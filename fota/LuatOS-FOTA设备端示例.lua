-- LuaTools需要PROJECT和VERSION这两个信息
-- =====================================================================
-- iot-master 平台 FOTA 设备端示例（合宙 LuatOS，Air780E 等系列通用）
-- 2026-08-31 修复版：旧版示例有三处致命错误（mqtt.EVENT常量不存在导致运行即崩、
--   mqtt.create第5参数实为isipv6而非websocket、subscribe把函数当qos传），
--   已按 LuatOS 官方源码与 API 文档修正；另将循环重建连接改为单连接+autoreconn。
-- 完整模块化测试工程（拆分mqtt_app/fota_app/version_app，含烧录联调README）：
--   C:\Users\13395\Desktop\小测试\780E功能测试\数采测试\Air780E\demo\fota2-iotmaster
--
-- 两条升级通道：
--   A. 平台点"下发升级" -> MQTT指令 device/{id}/upgrade -> libfota2按url下载
--   B. 开机自检 + 定时自检 -> HTTP /api/site/firmware_upgrade (libfota2直连)
-- 注意：MQTT走wss(WebSocket)需要2025.09.23之后的底层core；HTTP自检通道不受限
-- =====================================================================
PROJECT = "fotademo"
-- 平台按VERSION字符串精确匹配固件版本名：相同则304=已是最新
-- 采用合宙格式 1122.001.001：前两段对齐合宙约定, 最后一段为脚本版本, 递增最后一段发新版本
VERSION = "1122.001.001"

-- ===== 平台配置（按实际环境修改）=====
PLATFORM_HOST = "iot-master-jhykguet.sealosgzg.site"
-- 项目KEY = 平台上的产品ID（对应合宙模式的PRODUCT_KEY）:
--   ① register自动注册时设备自动归属该产品  ② 设备未注册时HTTP自检按产品拉固件兜底
PROJECT_KEY = "1234"
USE_IMEI  = true  -- true=联网后取模组真实IMEI作为设备ID(平台需以IMEI为ID建设备)
DEVICE_ID = ""    -- USE_IMEI=false时填固定设备ID，如"pc-test-002"
FOTA_CHECK_INTERVAL = 4 * 3600000 -- 定时自检升级周期(毫秒)，联调可改2*60*1000

sys = require "sys"
libfota2 = require "libfota2"

local mqttc = nil
local cur_msg_id = "" -- 当前升级任务的msg_id，上报时原样回传
local cur_target = "" -- 平台下发的目标版本号，success上报时回传

-- 按当前设备ID动态拼主题(USE_IMEI时设备ID联网后才确定，不能在文件加载时拼死)
local function topic(suffix)
    return "device/" .. DEVICE_ID .. "/" .. suffix
end

-- 设备日志上报：平台"设备日志"页可见，免串口看测试进度
local function devlog(msg)
    if mqttc ~= nil then
        mqttc:publish(topic("log"), msg, 1)
    end
end

-- 上报升级进度/结果给平台 (status: downloading/success/fail)
-- 平台按 msg_id+device_id 匹配升级记录；version非空时回写设备"固件版本"
local function report(status, error, version)
    if mqttc == nil then return end
    local body = { msg_id = cur_msg_id, status = status }
    if error then body.error = error end
    if version then body.version = version end
    mqttc:publish(topic("upgrade/response"), json.encode(body), 1)
end

-- 升级结果回调: 0成功 1连接失败 2url错误 3服务器断开 4报文错误(含304=已是最新) 5缺PROJECT_KEY
local function fota_cb(ret)
    log.info("fota", ret)
    if ret == 0 then
        log.info("升级包下载成功,1秒后重启刷写")
        report("success", nil, cur_target ~= "" and cur_target or nil)
        devlog("FOTA下载成功,重启刷写")
        cur_msg_id, cur_target = "", ""
        sys.wait(1000) -- 留1秒让上报报文发出
        rtos.reboot()
    elseif ret == 1 then
        log.error("连接失败", "请检查url拼写或服务器配置(是否为内网)")
        report("fail", "1 connect fail")
    elseif ret == 2 then
        log.error("url错误", "检查url拼写")
        report("fail", "2 url error")
    elseif ret == 3 then
        log.error("服务器断开", "检查服务器白名单配置")
        report("fail", "3 server closed")
    elseif ret == 4 then
        -- 304已是最新属正常PASS，不算失败
        log.info("304已是最新, 属正常")
        devlog("自检:已是最新版本")
    elseif ret == 5 then
        log.error("缺少必要的PROJECT_KEY参数")
        report("fail", "5 no project key")
    else
        log.info("不是上面几种情况 ret为", ret)
        report("fail", tostring(ret))
    end
end

-- 平台升级参数：###开头=URL完全按此请求；version必须传当前脚本版本号
-- 设备ID联网后才确定(USE_IMEI)，每次请求时构造；https下载失败可改http
local function build_opts()
    return {
        url = "###https://" .. PLATFORM_HOST ..
              "/api/site/firmware_upgrade?imei=" .. DEVICE_ID ..
              "&project_key=" .. PROJECT_KEY .. "&version=" .. VERSION,
        imei = DEVICE_ID,
    }
end

-- 平台下发升级的下载task(libfota2内部会sys.wait，不能在订阅回调上下文直接跑)
local function download_task(url)
    report("downloading") -- 平台"升级记录"变"下载中"
    devlog("开始下载固件")
    libfota2.request(fota_cb, { url = "###" .. url })
end

-- MQTT事件回调：LuatOS只有单回调形式，事件是字符串("conack"/"recv")，
-- 没有mqtt.EVENT常量；conack不携带数据(没有rc字段)，收到即连接成功
local function mqtt_cb(mqtt_client, event, data, payload)
    log.info("mqtt", "event", event)
    if event == "conack" then
        mqtt_client:subscribe(topic("upgrade"))
        -- 注册上报：平台自动记录当前固件版本(设备详情"固件版本")
        -- product_id=项目KEY: 设备不存在时平台自动注册并归属该产品
        mqtt_client:publish(topic("register"),
            json.encode({ id = DEVICE_ID, firmware = VERSION, product_id = PROJECT_KEY }), 1)
        devlog("设备上线,脚本版本" .. VERSION)
    elseif event == "recv" then
        log.info("mqtt", "recv", data)
        if data == topic("upgrade") then
            local ok, cmd = pcall(json.decode, payload or "")
            if not ok or type(cmd) ~= "table" or not cmd.url then
                report("fail", "bad upgrade payload")
                return
            end
            cur_msg_id = cmd.msg_id or ""
            cur_target = cmd.version or ""
            sys.taskInit(download_task, cmd.url)
        end
    end
end

-- 联网+MQTT任务：只创建一次连接，重连交给autoreconn(重连成功会再次conack，
-- 自动重新订阅和register；不要循环重建连接，相同client_id会互踢下线)
local function mqtt_task()
    sys.waitUntil("IP_READY")
    log.info("4G网络链接成功")
    if USE_IMEI then
        DEVICE_ID = mobile.imei()
        log.info("device", "设备ID取IMEI", DEVICE_ID)
    end
    -- wss://前缀由LuatOS底层识别为WebSocket模式(第5个参数是isipv6，不是websocket!)
    mqttc = mqtt.create(nil, "wss://" .. PLATFORM_HOST .. "/mqtt", 443, true)
    mqttc:auth(DEVICE_ID) -- WebSocket通道匿名，用户名填设备ID即可
    mqttc:autoreconn(true, 10000)
    mqttc:on(mqtt_cb)
    mqttc:connect()
    while true do
        sys.wait(60000) -- 连接与重连由autoreconn维护，task保活即可
    end
end

-- 开机自检：等设备ID就绪(USE_IMEI模式下联网后填充)再检查
local function boot_check_task()
    while DEVICE_ID == nil or DEVICE_ID == "" do
        sys.wait(200)
    end
    log.info("开始检查升级")
    sys.wait(500)
    libfota2.request(fota_cb, build_opts())
end

sys.taskInit(mqtt_task)
sys.taskInit(boot_check_task)
-- 定时自检升级，周期见FOTA_CHECK_INTERVAL
sys.timerLoopStart(function()
    libfota2.request(fota_cb, build_opts())
end, FOTA_CHECK_INTERVAL)

-- 用户代码已结束---------------------------------------------
-- 结尾总是这一句
sys.run()
-- sys.run()之后后面不要加任何语句!!!!!
