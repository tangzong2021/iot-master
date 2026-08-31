-- =============================================================================
-- LuatOS FOTA 设备端参考脚本（对接 iot-master 平台固件升级）
-- 适用模组：合宙 Air780E / Air780EP / Air780EPM / Air724UG 等LuatOS模组
-- 参考：https://docs.openluat.com/air780e/luatos/app/base/fotathird/
--       若API与你的core版本有出入，以 LuatOS demo/mqtt、demo/fota 为准
--
-- 功能：
--   1. MQTT连上平台后，发布 device/{id}/register 上报当前固件版本
--   2. 订阅 device/{id}/upgrade，收到平台"下发升级"指令后：
--      上报"下载中" → libfota2 下载固件 → 成功上报success → 自动重启应用升级
--      失败上报 fail(error=错误码)
--   3. 断网重连自动重新注册
-- =============================================================================

PROJECT = "iot-master-demo"   -- 项目名，升级包要与旧版保持一致
VERSION = "1.0.0"             -- 当前脚本版本号，升级时必须大于旧版

-- ===== 平台连接配置（按实际环境修改）=====
local PLATFORM_HOST = "iot-master-jhykguet.sealosgzg.site" -- 平台域名
local DEVICE_ID     = "pc-test-002"                        -- 设备ID（imei参数也用它）
local PRODUCT_ID    = ""                                   -- 产品ID，首次注册自动建设备时必填

-- 连接方式A：WebSocket（走平台443/Ingress，匿名，与MQTTX的ws连接一致）
local WS_MODE  = true
-- 连接方式B：TCP直连broker 1883，密码=md5(用户名+broker.key)，broker.key见平台系统设置
local TCP_PORT    = 1883
local BROKER_KEY  = "efeqwkehztrvafjm"

-- 主题
local T_REGISTER = "device/" .. DEVICE_ID .. "/register"
local T_UPGRADE  = "device/" .. DEVICE_ID .. "/upgrade"
local T_RESPONSE = "device/" .. DEVICE_ID .. "/upgrade/response"

local mqttc = nil
local cur_msg_id = ""   -- 当前升级指令ID，用于平台匹配升级记录

local libnet = require "libnet"
local libfota2 = require "libfota2"
local crypto = require "crypto"

-- MQTT发布
local function publish(topic, tbl)
    if mqttc then
        mqttc:publish(topic, json.encode(tbl), 1)
    end
end

-- 上报升级进度/结果
local function report(status, error, version)
    local body = { msg_id = cur_msg_id, status = status }
    if error then body.error = error end
    if version then body.version = version end
    publish(T_RESPONSE, body)
end

-- FOTA回调：ret=0 下载成功，其他为失败
local function fota_cb(ret)
    log.info("fota", "result", ret)
    if ret == 0 then
        report("success", nil, VERSION)
        sys.wait(500)           -- 等待报文发出
        rtos.reboot()           -- 重启应用升级（重启过程自动校验刷写，失败自动回滚）
    else
        report("fail", tostring(ret))
    end
end

-- 执行升级：url 为平台下发的固件完整下载地址
local function do_upgrade(url)
    report("downloading")
    -- "###" 前缀 = 完全按该URL请求，不追加合宙iot平台参数
    libfota2.request(fota_cb, { url = "###" .. url })
end

-- MQTT消息处理
local function on_mqtt_message(sc, data)
    log.info("mqtt", "recv", data.topic)
    if data.topic == T_UPGRADE then
        local ok, cmd = pcall(json.decode, data.payload or "")
        if not ok or type(cmd) ~= "table" then return end
        cur_msg_id = cmd.msg_id or ""
        if cmd.url and cmd.url ~= "" then
            do_upgrade(cmd.url)
        else
            report("fail", "url is empty")
        end
    end
end

-- MQTT连接成功后：订阅升级主题 + 注册上报固件版本
local function on_connected()
    mqttc:subscribe(T_UPGRADE, function()
        log.info("mqtt", "subscribed", T_UPGRADE)
    end)
    publish(T_REGISTER, {
        id         = DEVICE_ID,
        product_id = PRODUCT_ID ~= "" and PRODUCT_ID or nil,
        firmware   = VERSION,
    })
end

-- 连接MQTT（阻塞式任务，断线自动重连）
local function mqtt_task()
    while true do
        local ok, err
        if WS_MODE then
            -- mqtt.create(adapter, host, port, ssl, iswebsocket) → wss://host/mqtt
            mqttc = mqtt.create(nil, PLATFORM_HOST, 443, true, true)
            mqttc:auth(DEVICE_ID)                       -- WebSocket通道匿名
        else
            mqttc = mqtt.create(nil, PLATFORM_HOST, TCP_PORT)
            mqttc:auth(DEVICE_ID, crypto.md5(DEVICE_ID .. BROKER_KEY))
        end
        mqttc:on(mqtt.EVENT.CONNACK, function(sc, connack)
            if connack.rc == 0 then
                log.info("mqtt", "connected")
                on_connected()
            else
                log.warn("mqtt", "connack rc", connack.rc)
            end
        end)
        mqttc:on(mqtt.EVENT.RECV, on_mqtt_message)
        mqttc:autoreconn(true, 10000)
        ok, err = mqttc:connect()
        if not ok then
            log.warn("mqtt", "connect fail", err)
        end
        sys.waitUntil("MQTT_CLOSED", 300000)
        sys.wait(5000)
    end
end

-- 开机/定时自检升级（不依赖平台下发，直连HTTP接口）
-- 平台约定：HTTP 200=有固件(200内容即固件)，304=已是最新
-- 如需此模式，取消下面的注释即可
-- sys.taskInit(function()
--     sys.wait(15000)  -- 等网络就绪
--     while true do
--         local url = "https://" .. PLATFORM_HOST ..
--                     "/api/site/firmware_upgrade?imei=" .. DEVICE_ID ..
--                     "&version=" .. VERSION
--         libfota2.request(fota_cb, { url = "###" .. url })
--         sys.wait(4 * 3600 * 1000)  -- 每4小时检查一次
--     end
-- end)

sys.taskInit(mqtt_task)
