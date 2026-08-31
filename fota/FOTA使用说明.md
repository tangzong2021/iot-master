# FOTA 固件升级使用说明（合宙 Luat 模组对接）

平台固件升级已打通合宙 LuatOS 设备的 FOTA 流程，分两条通道：

| 通道 | 触发方 | 设备要求 | 适用场景 |
|---|---|---|---|
| **MQTT 下发升级** | 管理员在平台点按钮 | 设备在线（MQTT已连接，订阅 `device/{id}/upgrade`） | 主动升级、批量升级 |
| **HTTP 自助检查** | 设备开机/定时自检 | 能 HTTP 访问平台 | 兼容合宙 libfota2 标准库，无需平台干预 |

---

## 一、整体流程

```
[平台] 产品详情 → 固件版本 → 上传固件（Luatools生成的升级包）
        → 点行操作"下发升级" → 弹窗选设备（可多选）→ 确认
        → 平台发 MQTT 指令 device/{id}/upgrade  {msg_id, url, version}
        → 设备收到指令 → libfota2 下载 url 固件 → 上报进度
        → device/{id}/upgrade/response  {msg_id, status, error?, version?}
        → 平台"升级记录"页实时更新状态：已下发→下载中→成功/失败
        → 设备重启应用固件 → register 上报新版本 → 设备详情显示"固件版本"
```

## 二、平台侧操作

1. **上传固件**：产品库 → 产品详情 → 固件版本 tab → 创建：
   - 名称 = 版本号（如 `1.0.1`），**必须与设备端 `VERSION` 一致**（设备请求接口上报的 version 与它相同时，平台返回304=无需升级）
   - 固件 = 上传 Luatools 生成的升级包文件（`.bin` / `.dfota.bin`）
2. **下发升级**：固件版本列表 → 行操作 🡅（下发升级）→ 勾选在线设备 → 提交
   - 也可在 设备详情 右上角点"升级"直接对单台设备下发最新固件
3. **查看进度**：物联网 → 升级记录（状态：已下发/下载中/成功/失败，含错误信息）

## 三、设备侧（LuatOS Lua 脚本）

参考 [LuatOS-FOTA设备端示例.lua](./LuatOS-FOTA设备端示例.lua)（2026-08-31 修复版：修正了旧示例中 `mqtt.EVENT` 常量不存在、`mqtt.create` 第5参数误用、`subscribe` 传回调函数三处致命错误）。完整模块化测试工程（mqtt_app/fota_app/version_app 拆分 + 烧录联调 README）：
`C:\Users\13395\Desktop\小测试\780E功能测试\数采测试\Air780E\demo\fota2-iotmaster`

要点：

```lua
-- 1) 收到升级指令后，用 libfota2 下载（"###"=URL完全按平台下发的用）
report("downloading")
libfota2.request(fota_cb, { url = "###" .. cmd.url })

-- 2) 回调 ret=0 表示下载成功 → 上报成功 → 重启（重启过程自动校验刷写）
local function fota_cb(ret)
    if ret == 0 then
        report("success", nil, VERSION)
        rtos.reboot()
    else
        report("fail", tostring(ret))
    end
end

-- 3) 每次MQTT连上后注册，平台自动记录当前固件版本
publish("device/"..DEVICE_ID.."/register", { id=DEVICE_ID, firmware=VERSION })
```

设备 ID 与触发方式：

- **设备 ID 默认取模组真实 IMEI**（`USE_IMEI=true`，联网后 `mobile.imei()`）：平台须以设备标签上的 IMEI 为 ID 创建设备并归属产品；一批设备共用一份脚本。设 `USE_IMEI=false` 可改用固定设备 ID（如 MQTTX 联调用的 `pc-test-002`）；
- **开机自检**：设备就绪后自动 GET 升级接口，有新版即下载重启，无新版返回 304；升级完成重启后再次自检因版本已相同返回 304，不会循环升级；
- **定时自检**：周期由脚本内 `FOTA_CHECK_INTERVAL` 配置（默认 4 小时，联调可改 2 分钟）。

升级脚本注意事项（合宙官方要求）：
- 新旧脚本 `PROJECT` 必须一致，`VERSION` 必须**大于**旧版（不支持降级）
- **MQTT 走 wss（WebSocket）通道需要 2025.09.23 之后的底层 core**（Luatools 在线选最新 core）；HTTP 自检通道不受 core 限制
- 仅升级脚本：Luatools 勾选"升级文件包含脚本"；含底层core升级：需用合宙差分工具生成差分包
- 异常断电不会变砖：下载完成后要**重启过程**才校验刷写，失败自动回滚
- 连续6次循环升级失败，core会禁止升级（防变砖保护）

## 四、HTTP 自助检查接口（libfota2 兼容）

设备直接 HTTP GET：

```
GET https://{平台域名}/api/site/firmware_upgrade?imei={设备ID}&version={当前版本}
```

| 参数 | 说明 |
|---|---|
| `imei` | 设备ID（平台找不到该设备时，用 `project_key` 参数直接指定产品ID） |
| `version` | 设备当前固件版本，与平台最新版本名相同 → 返回304 |
| `firmware_name` / `core_version` / `dfota` | 合宙旧 update.lua 附加的参数，平台忽略，可不上传 |

返回约定（与合宙 libfota2 完全兼容）：
- **HTTP 200**：body 即固件文件内容（平台上传的文件直接读，外部URL则代理转发）
- **HTTP 304/404**：无需升级或无可用固件，libfota2 视为"已是最新"

快速验证：

```bash
# 有新版本：返回固件内容
curl "https://xxx/api/site/firmware_upgrade?imei=pc-test-002&version=1.0.0" -o f.bin
# 已是最新：返回304
curl -v "https://xxx/api/site/firmware_upgrade?imei=pc-test-002&version=9.9.9"
```

## 五、MQTTX 模拟设备联调（无真实模组时）

1. MQTTX 连接平台（wss://域名/mqtt，匿名），订阅 `device/pc-test-002/#`
2. 平台固件版本页点"下发升级"→ MQTTX 收到 `device/pc-test-002/upgrade` 指令（含 url、version、msg_id）
3. 模拟下载成功：向 `device/pc-test-002/upgrade/response` 发布
   `{"msg_id":"复制收到的msg_id","status":"success","version":"1.0.1"}`
4. 平台"升级记录"状态变"成功"；再发 `device/pc-test-002/register` `{"id":"pc-test-002","firmware":"1.0.1"}`，设备详情"固件版本"即更新

## 六、升级记录表结构

| 字段 | 说明 |
|---|---|
| 设备/设备ID | 目标设备 |
| 目标版本 | 下发的固件版本（version表） |
| 原版本 | 下发时设备自报的固件版本 |
| 状态 | 已下发 → 下载中 → 成功/失败 |
| 错误 | 设备回报的错误码/原因 |
| 下发时间/更新时间 | 任务时间线 |
