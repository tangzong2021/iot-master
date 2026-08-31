-- LuaTools需要PROJECT和VERSION这两个信息
-- =====================================================================
-- 官方 fota2 demo 的 iot-master 平台对接版（结构保留自官方demo）
--   原版: demo/fota2/main.lua (合宙IoT平台模式)
--   本版改动:
--     1. ota_opts.url 指向 iot-master 平台 /api/site/firmware_upgrade
--        (### 开头 = url已带全部参数, libfota2不再追加合宙参数)
--     2. 增加 MQTT 任务: 平台点"下发升级"立即触发, 不只靠定时自检
--     3. 升级进度/结果回传平台, 平台"升级记录"页可见
--     4. MQTT连上后发 register 上报当前固件版本, 平台设备详情可见
--
-- 升级测试流程:
--   ① 本目录 Luatools 烧录(底层+脚本), 设备开机自动连平台, 定时自检升级
--   ② 改 VERSION(如 1.0.0 -> 1.0.1), Luatools 勾"升级文件包含脚本"打包 .bin
--   ③ 平台 产品详情→固件版本→创建: 名称填新版本号(必须与VERSION一致), 上传.bin
--   ④ 平台点该行"下发升级"(或等设备4小时自检) -> 自动下载重启完成升级
-- =====================================================================
PROJECT = "fotademo"
-- 平台按脚本VERSION字符串匹配: 与平台固件版本名称相同时, 平台返回304=已是最新
-- 因此建议版本号一路递增: 1.0.0 -> 1.0.1 -> 1.0.2 ...
VERSION = "1.0.0"

-- ===== 对接 iot-master 平台配置（按实际环境修改）=====
local PLATFORM_HOST = "iot-master-jhykguet.sealosgzg.site"
local DEVICE_ID     = "pc-test-002" -- 平台上的设备ID, 升级接口用它匹配设备/产品

sys = require "sys"
libfota2 = require "libfota2"

local T_UPGRADE  = "device/" .. DEVICE_ID .. "/upgrade"
local T_RESPONSE = "device/" .. DEVICE_ID .. "/upgrade/response"
local cur_msg_id = ""
local mqttc = nil

-- 联网函数, 可自行删减
sys.taskInit(function()
    -- 默认都等到联网成功
    sys.waitUntil("IP_READY")
    log.info("4G网络链接成功")
    sys.publish("net_ready")
end)

-- 循环打印版本号, 方便看版本号变化, 非必须
sys.taskInit(function()
    while 1 do
        sys.wait(5000)
        log.info("fota", "脚本版本号", VERSION, "core版本号", rtos.version())
    end
end)

-- 上报升级进度/结果给平台 (status: downloading/success/fail)
local function report(status, error, version)
    if mqttc == nil then return end
    local body = { msg_id = cur_msg_id, status = status }
    if error then body.error = error end
    if version then body.version = version end
    mqttc:publish(T_RESPONSE, json.encode(body), 1)
end

-- 升级结果的回调函数
-- result: 0成功 1连接失败 2url错误 3服务器断开 4接收报文错误(含服务器返回304=已是最新) 5缺PROJECT_KEY
local function fota_cb(ret)
    log.info("fota", ret)
    if ret == 0 then
        log.info("升级包下载成功,重启模块")
        report("success", nil, VERSION)
        sys.timerStart(rtos.reboot, 1000) -- 留1秒让上报报文发出
    elseif ret == 1 then
        log.info("连接失败", "请检查url拼写或服务器配置(是否为内网)")
        report("fail", "1 connect fail")
    elseif ret == 2 then
        log.info("url错误", "检查url拼写")
        report("fail", "2 url error")
    elseif ret == 3 then
        log.info("服务器断开", "检查服务器白名单配置")
        report("fail", "3 server closed")
    elseif ret == 4 then
        log.info("接收报文错误", "通常是服务器返回304=已是最新, 或升级包损坏")
        -- 已是最新不算失败, 不上报fail
    elseif ret == 5 then
        log.info("缺少必要的PROJECT_KEY参数")
        report("fail", "5 no project key")
    else
        log.info("不是上面几种情况 ret为", ret)
        report("fail", tostring(ret))
    end
end

-- 平台升级参数 (官方demo注释段的启用版, 指向 iot-master)
-- 平台契约: 200=返回固件内容, 304=已是最新
-- 如设备上https下载失败(回调1/4), 可把 https 改成 http 再试
local ota_opts = {
    -- ### 开头 = 完全按此url请求; version参数必须传当前脚本版本号
    url = "###https://" .. PLATFORM_HOST ..
          "/api/site/firmware_upgrade?imei=" .. DEVICE_ID .. "&version=" .. VERSION,
    imei = DEVICE_ID, -- 覆盖模块默认IMEI, 平台按设备ID匹配
}

-- 开机检查一次升级
sys.taskInit(function()
    sys.waitUntil("net_ready")
    log.info("开始检查升级")
    sys.wait(500)
    libfota2.request(fota_cb, ota_opts)
end)

-- 演示定时自动升级, 每隔4小时自动检查一次
sys.timerLoopStart(libfota2.request, 4 * 3600000, fota_cb, ota_opts)

-- ===== MQTT 任务: 平台"下发升级"立即触发 =====
-- 走 WebSocket(与MQTTX一致): wss://域名/mqtt, 平台侧此通道匿名
local function mqtt_task()
    while true do
        mqttc = mqtt.create(nil, PLATFORM_HOST, 443, true, true) -- ssl + websocket
        mqttc:auth(DEVICE_ID) -- WebSocket通道匿名, 用户名填设备ID即可
        mqttc:on(mqtt.EVENT.CONNACK, function(sc, connack)
            if connack.rc == 0 then
                log.info("mqtt", "connected")
                sc:subscribe(T_UPGRADE, function()
                    log.info("mqtt", "subscribed", T_UPGRADE)
                end)
                -- 注册上报: 平台自动记录当前固件版本(设备详情"固件版本")
                sc:publish("device/" .. DEVICE_ID .. "/register",
                    json.encode({ id = DEVICE_ID, firmware = VERSION }), 1)
            else
                log.warn("mqtt", "connack rc", connack.rc)
            end
        end)
        mqttc:on(mqtt.EVENT.RECV, function(sc, data)
            log.info("mqtt", "recv", data.topic)
            if data.topic == T_UPGRADE then
                local ok, cmd = pcall(json.decode, data.payload or "")
                if not ok or type(cmd) ~= "table" or not cmd.url then
                    report("fail", "bad upgrade payload")
                    return
                end
                cur_msg_id = cmd.msg_id or ""
                -- 升级下载放在task里执行
                sys.taskInit(function(u)
                    report("downloading")
                    libfota2.request(fota_cb, { url = "###" .. u })
                end, cmd.url)
            end
        end)
        mqttc:autoreconn(true, 10000)
        mqttc:connect()
        sys.wait(30000) -- 兜底重连周期
    end
end
sys.taskInit(mqtt_task)

-- 用户代码已结束---------------------------------------------
-- 结尾总是这一句
sys.run()
-- sys.run()之后后面不要加任何语句!!!!!
