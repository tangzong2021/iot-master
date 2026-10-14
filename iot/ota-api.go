package iot

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/god-jason/iot-master/apis"
	"github.com/god-jason/iot-master/pkg/api"
	"github.com/god-jason/iot-master/pkg/db"
	"github.com/god-jason/iot-master/pkg/log"
	"github.com/god-jason/iot-master/pkg/mqtt"
	"github.com/god-jason/iot-master/pkg/table"
	"github.com/rs/xid"
	"github.com/spf13/cast"
)

func init() {
	//设备自助升级检查（兼容合宙 libfota2/update.lua 协议）
	api.RegisterUnAuthorized("GET", "site/firmware_upgrade", firmwareUpgrade)

	//创建升级任务（支持批量设备）
	api.Register("POST", "upgrade/create", upgradeCreate)
	api.Register("POST", "device/:id/upgrade", deviceUpgrade)
	//创建固件版本（名称留空时自动从升级包文件名提取版本号）
	api.Register("POST", "version/create", versionCreate)
	//手动获取设备物模型（下发指令让设备上报完整模型，自动保存为所属产品模型）
	api.Register("POST", "device/:id/model/get", deviceModelGet)
}

// checkDeviceManage 校验设备管理权限（管理员放行）
func checkDeviceManage(ctx *gin.Context) bool {
	if ctx.GetBool("admin") {
		return true
	}
	uid := ctx.GetString("user")
	if uid == "" {
		return false
	}
	type row struct {
		PrivDeviceManage bool `json:"priv_device_manage"`
	}
	var r row
	if has, err := db.Engine().Table("user").Where("id=?", uid).Get(&r); err == nil && has {
		return r.PrivDeviceManage
	}
	return false
}

// parseVersion 将版本号解析为数字段："001.999.083" → [1,999,83]。
// 含非数字段或空串返回 nil，表示无法数值比对（调用方退回字符串精确比较）
func parseVersion(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ".")
	segs := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		segs = append(segs, n)
	}
	return segs
}

// semverCmp 数值段比较：a>b 返回 1，a<b 返回 -1，相等返回 0；缺段按 0 补齐
func semverCmp(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

// needUpgrade 判断目标固件版本是否比设备当前版本新（true=需要下发）。
// 兼容三种情况，任一命中"不比设备新"即返回 false：
//  1. 字符串一致
//  2. 数值段一致或目标更低（忽略前导零/缺段，如 1.999.83 == 001.999.083）
//  3. 合宙内核命名风格：目标首段≥1000 时视为"内核.脚本段1.脚本段3"（如
//     2052.001.083 对应脚本版本 001.*.083），按(目标段2,目标段3)与(设备段1,设备段3)比对
//
// 两侧任一版本无法数值解析时，仅按字符串一致判定
func needUpgrade(name, version string) bool {
	if name == version {
		return false
	}
	tgt, dev := parseVersion(name), parseVersion(version)
	if tgt == nil || dev == nil {
		return true // 无法比对，保持原下发行为
	}
	if semverCmp(tgt, dev) <= 0 {
		return false
	}
	// 合宙风格目标（首段为内核版本号，如 2052.001.083）与脚本版本(001.999.083)同方案折算
	if len(tgt) >= 3 && tgt[0] >= 1000 && len(dev) >= 3 {
		if semverCmp([]int{tgt[1], tgt[2]}, []int{dev[0], dev[len(dev)-1]}) <= 0 {
			return false
		}
	}
	return true
}

// fotaServeLast 每设备最近一次FOTA固件下发时刻（HTTP自检拉取与MQTT指令下发共用）。
// 同一设备两次下发间隔必须大于10分钟：设备侧异常循环拉包时，服务器不再跟着反复下发大文件。
// 内存态，平台重启后限频窗口清零（设备自检周期为小时级，重启丢失影响可忽略）
var fotaServeMu sync.Mutex
var fotaServeLast = map[string]time.Time{}

const fotaServeInterval = 10 * time.Minute

// fotaServeAllowed 判断该设备当前是否允许下发固件；允许则记录本次时刻并返回true
func fotaServeAllowed(deviceId string) bool {
	if deviceId == "" {
		return true //无设备标识无法限频，保持放行
	}
	fotaServeMu.Lock()
	defer fotaServeMu.Unlock()
	if t, ok := fotaServeLast[deviceId]; ok && time.Since(t) < fotaServeInterval {
		return false
	}
	fotaServeLast[deviceId] = time.Now()
	return true
}

// firmwareUpgrade 设备侧升级检查接口
// 兼容合宙 libfota2：设备请求 /api/site/firmware_upgrade?imei=&version=&project_key=
// 需要升级：HTTP 200 + 固件二进制；无需升级/找不到：HTTP 300+（libfota2 视为已是最新）
// 版本比对按 needUpgrade：目标固件不比设备 version 参数新时一律 304，
// 否则设备会拉取同版固件重启后版本不变，随自检周期反复下载反复重启
func firmwareUpgrade(ctx *gin.Context) {
	imei := ctx.Query("imei")
	version := strings.TrimSpace(ctx.Query("version"))
	projectKey := ctx.Query("project_key")

	//定位产品：优先按设备ID(imei)查设备，取其产品；否则 project_key 视为产品ID
	productId := projectKey
	if imei != "" {
		if tab, err := table.Get("device"); err == nil {
			doc, err := tab.Get(imei, []string{"product_id"})
			if err == nil && doc != nil {
				if pid, ok := doc["product_id"].(string); ok && pid != "" {
					productId = pid
				}
			}
		}
	}
	if productId == "" {
		ctx.String(http.StatusNotFound, "device not found")
		return
	}

	//产品最新固件版本
	ver, err := latestVersion(productId)
	if err != nil {
		ctx.String(http.StatusNotFound, err.Error())
		return
	}
	name, _ := ver["name"].(string)
	url, _ := ver["url"].(string)

	//设备报了版本且目标不比它新：无需升级（防反复下载反复重启）
	if version != "" && !needUpgrade(name, version) {
		log.Info("firmware_upgrade 无需升级，跳过下发",
			"imei", imei, "target", name, "device", version)
		ctx.Status(http.StatusNotModified)
		return
	}

	//限频：同一设备两次FOTA下发间隔必须大于10分钟
	if !fotaServeAllowed(imei) {
		log.Info("firmware_upgrade 距上次下发不足10分钟，跳过: ", imei)
		ctx.Status(http.StatusNotModified)
		return
	}

	if err := serveFirmware(ctx, url); err != nil {
		log.Error("serve firmware fail", err)
		if !ctx.Writer.Written() {
			ctx.String(http.StatusNotFound, "firmware not found")
		}
		return
	}

	//记录HTTP自助升级：设备拉到固件即产生升级记录（不论设备后续是否回报）
	recordUpgrade(productId, imei, ver, version)
}

// latestVersion 查询产品最新启用固件版本（version.id 为 xid，按序即时间序）
// 注意：API创建的记录 disabled 可能为 NULL，SQL过滤匹配不上，取回后在Go侧判断
// 名称(版本号)为空的记录视为无效跳过：空名称永远无法与设备版本匹配，会造成设备反复下载
func latestVersion(productId string) (map[string]any, error) {
	tab, err := table.Get("version")
	if err != nil {
		return nil, err
	}
	rows, err := tab.Find(&table.ParamSearch{
		Limit:  50,
		Filter: map[string]any{"product_id": productId},
		Sort:   map[string]int{"id": -1},
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if cast.ToBool(row["disabled"]) {
			continue
		}
		if name, _ := row["name"].(string); strings.TrimSpace(name) == "" {
			continue
		}
		return row, nil
	}
	return nil, errors.New("该产品没有可用固件版本")
}

// versionCreate 创建固件版本，名称(版本号)留空时自动从升级包文件名提取。
// Luatools生成的升级包文件名格式: 项目名_版本号_底层标识.bin，如 fotademo_1122.001.001_LuatOS-SoC_EC618.bin
func versionCreate(ctx *gin.Context) {
	if !checkDeviceManage(ctx) {
		api.Fail(ctx, "无权限：需要设备管理权限")
		return
	}
	var doc map[string]any
	if err := ctx.ShouldBindJSON(&doc); err != nil {
		api.Error(ctx, err)
		return
	}
	if name, _ := doc["name"].(string); strings.TrimSpace(name) == "" {
		url, _ := doc["url"].(string)
		if v := versionFromFilename(url); v != "" {
			doc["name"] = v
		}
	}
	//说明留空时自动从文件名提取固件标识，如 fotademo_LuatOS-SoC_EC618
	if desc, _ := doc["description"].(string); strings.TrimSpace(desc) == "" {
		url, _ := doc["url"].(string)
		if d := describeFromFilename(url); d != "" {
			doc["description"] = d
		}
	}
	if id, _ := doc["id"].(string); id == "" {
		doc["id"] = xid.New().String()
	}
	tab, err := table.Get("version")
	if err != nil {
		api.Error(ctx, err)
		return
	}
	if _, err := tab.Insert(doc); err != nil {
		api.Error(ctx, err)
		return
	}
	api.OK(ctx, doc["id"])
}

// versionFromFilename 从升级包文件名(或URL)中提取版本号：取第一个形如 1122.001.001 的段
func versionFromFilename(url string) string {
	base := url
	if i := strings.LastIndex(url, "/"); i >= 0 {
		base = url[i+1:]
	}
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	for _, part := range strings.Split(base, "_") {
		if isVersionLike(part) {
			return part
		}
	}
	return ""
}

// describeFromFilename 从升级包文件名提取固件标识(去掉版本号段的剩余部分)：
// fotademo_1122.001.001_LuatOS-SoC_EC618.bin → fotademo_LuatOS-SoC_EC618
func describeFromFilename(url string) string {
	base := url
	if i := strings.LastIndex(url, "/"); i >= 0 {
		base = url[i+1:]
	}
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	var parts []string
	for _, part := range strings.Split(base, "_") {
		if part == "" || isVersionLike(part) {
			continue
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "_")
}

// isVersionLike 判断是否为版本号段：2段以上、每段非空纯数字，如 1122.001.001
func isVersionLike(s string) bool {
	segs := strings.Split(s, ".")
	if len(segs) < 2 {
		return false
	}
	for _, seg := range segs {
		if seg == "" {
			return false
		}
		for _, r := range seg {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// serveFirmware 输出固件文件：平台上传的本地文件直接读取，外部地址代理转发
func serveFirmware(ctx *gin.Context, url string) error {
	if url == "" {
		return errors.New("固件地址为空")
	}
	ctx.Header("Content-Type", "application/octet-stream")
	ctx.Header("Content-Disposition", "attachment; filename=\"firmware.bin\"")

	//本平台上传的文件：/api/download/2026/8/xxx.bin → 本地 UploadPath/2026/8/xxx.bin
	if i := strings.Index(url, "/api/download/"); i >= 0 {
		fn := apis.UploadPath + "/" + url[i+len("/api/download/"):]
		f, err := os.Open(fn)
		if err != nil {
			return err
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			return err
		}
		ctx.Header("Content-Length", strconv.FormatInt(st.Size(), 10))
		_, err = io.Copy(ctx.Writer, f)
		return err
	}

	//外部 HTTP 地址：服务端代理下载
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		resp, err := http.Get(url)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		ctx.Status(resp.StatusCode)
		_, err = io.Copy(ctx.Writer, resp.Body)
		return err
	}

	//相对路径，按本地文件处理
	f, err := os.Open(apis.UploadPath + "/" + url)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(ctx.Writer, f)
	return err
}

// deviceModelGet 手动获取设备物模型: 下发指令让设备上报完整模型,
// 设备上报后经 model/report 处理器自动保存为所属产品的物模型(管理员手动触发, 防误操作)
func deviceModelGet(ctx *gin.Context) {
	if !checkDeviceManage(ctx) {
		api.Fail(ctx, "无权限：需要设备管理权限")
		return
	}
	id := ctx.Param("id")
	d := devices.Load(id)
	if d == nil {
		var err error
		if d, err = LoadDevice(id); err != nil {
			api.Fail(ctx, "设备不存在")
			return
		}
	}
	mqtt.Publish("device/"+id+"/model/get", nil)
	api.OK(ctx, "指令已下发，设备上报后将自动保存为所属产品的物模型")
}

// deviceUpgrade 单设备下发升级 POST device/:id/upgrade {version_id?}
func deviceUpgrade(ctx *gin.Context) {
	if !checkDeviceManage(ctx) {
		api.Fail(ctx, "无权限：需要设备管理权限")
		return
	}
	var body struct {
		VersionId string `json:"version_id"`
	}
	_ = ctx.ShouldBindJSON(&body)
	rid, err := sendUpgrade(ctx.Param("id"), body.VersionId)
	if err != nil {
		api.Fail(ctx, err.Error())
		return
	}
	api.OK(ctx, rid)
}

// upgradeCreate 批量下发升级 POST upgrade/create {device_id: string|[], version_id?}
func upgradeCreate(ctx *gin.Context) {
	if !checkDeviceManage(ctx) {
		api.Fail(ctx, "无权限：需要设备管理权限")
		return
	}
	var body struct {
		DeviceId  any    `json:"device_id"`
		VersionId string `json:"version_id"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		api.Error(ctx, err)
		return
	}

	var ids []string
	switch v := body.DeviceId.(type) {
	case string:
		if v != "" {
			ids = append(ids, v)
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				ids = append(ids, s)
			}
		}
	case []string:
		ids = v
	}
	if len(ids) == 0 {
		api.Fail(ctx, "请选择设备")
		return
	}

	var result []map[string]any
	for _, id := range ids {
		rid, err := sendUpgrade(id, body.VersionId)
		if err != nil {
			result = append(result, map[string]any{"device_id": id, "error": err.Error()})
		} else {
			result = append(result, map[string]any{"device_id": id, "id": rid})
		}
	}
	api.OK(ctx, result)
}

// sendUpgrade 向设备下发升级指令并记录升级任务
func sendUpgrade(deviceId, versionId string) (any, error) {
	d := devices.Load(deviceId)
	if d == nil || !d.Online {
		return nil, errors.New("设备未上线")
	}

	var ver map[string]any
	var err error
	if versionId != "" {
		tab, err := table.Get("version")
		if err != nil {
			return nil, err
		}
		ver, err = tab.Get(versionId, nil)
		if err != nil {
			return nil, errors.New("固件版本不存在")
		}
	} else {
		ver, err = latestVersion(d.ProductId)
		if err != nil {
			return nil, err
		}
	}

	url, _ := ver["url"].(string)
	if url == "" {
		return nil, errors.New("固件文件为空，请先上传固件")
	}
	name, _ := ver["name"].(string)
	//设备当前版本已不低于目标版本时拒绝下发（与自检接口同一比对规则），
	//防止重复下发造成设备反复升级重启
	if cur := deviceFirmware(d.Id); cur != "" && !needUpgrade(name, cur) {
		return nil, errors.New("设备当前版本 " + cur + " 已不低于目标版本 " + name + "，无需升级")
	}
	//限频：同一设备两次FOTA下发间隔必须大于10分钟
	if !fotaServeAllowed(d.Id) {
		return nil, errors.New("距上次FOTA下发不足10分钟，请稍后再试")
	}
	vid, _ := ver["id"].(string)
	msgId := xid.New().String()

	//下发升级指令
	mqtt.Publish("device/"+d.Id+"/upgrade", map[string]any{
		"msg_id":  msgId,
		"version": name,
		"url":     url,
	})

	//记录升级任务
	tab, err := table.Get("upgrade")
	if err != nil {
		return nil, err
	}
	return tab.Insert(map[string]any{
		"product_id":   d.ProductId,
		"device_id":    d.Id,
		"version_id":   vid,
		"msg_id":       msgId,
		"from_version": deviceFirmware(d.Id),
		"status":       "已下发",
	})
}

// deviceFirmware 查询设备当前固件版本
func deviceFirmware(id string) string {
	tab, err := table.Get("device")
	if err != nil {
		return ""
	}
	doc, err := tab.Get(id, []string{"firmware"})
	if err != nil || doc == nil {
		return ""
	}
	s, _ := doc["firmware"].(string)
	return s
}

// recordUpgrade 记录一次固件下发（HTTP自助拉取）。
// 同设备同版本存在未完成记录时直接复用，避免设备周期轮询产生重复记录
func recordUpgrade(productId, deviceId string, ver map[string]any, fromVersion string) {
	tab, err := table.Get("upgrade")
	if err != nil {
		return
	}
	vid, _ := ver["id"].(string)
	rows, _ := tab.Find(&table.ParamSearch{
		Limit:  1,
		Filter: map[string]any{"device_id": deviceId, "version_id": vid, "status": "已下发"},
	})
	if len(rows) > 0 {
		return
	}
	_, _ = tab.Insert(map[string]any{
		"product_id":   productId,
		"device_id":    deviceId,
		"version_id":   vid,
		"from_version": fromVersion,
		"status":       "已下发",
	})
}

// closeUpgradeRecords 设备上报固件版本后，关闭目标版本一致且未完成的升级记录（记为成功）。
// 覆盖两种场景：HTTP自助升级拉取后设备重启上来新版本；MQTT下发升级设备只回报拉取成功但未回response
func closeUpgradeRecords(deviceId, version string) {
	if deviceId == "" || version == "" {
		return
	}
	tab, err := table.Get("upgrade")
	if err != nil {
		return
	}
	rows, err := tab.Find(&table.ParamSearch{
		Limit:  50,
		Filter: map[string]any{"device_id": deviceId, "status": "已下发"},
	})
	if err != nil || len(rows) == 0 {
		return
	}
	vtab, err := table.Get("version")
	if err != nil {
		return
	}
	for _, row := range rows {
		vid, _ := row["version_id"].(string)
		if vid == "" {
			continue
		}
		ver, err := vtab.Get(vid, []string{"name"})
		if err != nil || ver == nil {
			continue
		}
		if name, _ := ver["name"].(string); name == version {
			_, _ = tab.UpdateById(row["id"], map[string]any{"status": "成功"})
		}
	}
}

// UpgradeResponse 设备升级进度/结果上报
type UpgradeResponse struct {
	MsgId   string `json:"msg_id,omitempty"`
	Status  string `json:"status,omitempty"` //downloading / success / fail
	Error   string `json:"error,omitempty"`
	Version string `json:"version,omitempty"` //升级成功后的版本
}

func mqttSubscribeUpgrade() {
	mqtt.SubscribeStruct[UpgradeResponse]("device/+/upgrade/response", func(topic string, resp *UpgradeResponse) {
		id := strings.Split(topic, "/")[1]
		tab, err := table.Get("upgrade")
		if err != nil {
			log.Error(err)
			return
		}
		rows, err := tab.Find(&table.ParamSearch{
			Limit:  1,
			Filter: map[string]any{"msg_id": resp.MsgId, "device_id": id},
		})
		if err != nil || len(rows) == 0 {
			log.Error("upgrade record not found: ", resp.MsgId)
			return
		}

		status := map[string]string{
			"downloading": "下载中",
			"success":     "成功",
			"fail":        "失败",
		}[resp.Status]
		if status == "" {
			status = resp.Status
		}
		_, _ = tab.UpdateById(rows[0]["id"], map[string]any{
			"status": status,
			"error":  resp.Error,
		})

		//升级成功后的版本回写设备表
		if resp.Version != "" {
			if dtab, err := table.Get("device"); err == nil {
				_, _ = dtab.UpdateById(id, map[string]any{"firmware": resp.Version})
			}
		}
	})
}
