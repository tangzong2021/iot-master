package iot

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

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

// firmwareUpgrade 设备侧升级检查接口
// 兼容合宙 libfota2：设备请求 /api/site/firmware_upgrade?imei=&version=&project_key=
// 需要升级：HTTP 200 + 固件二进制；无需升级/找不到：HTTP 300+（libfota2 视为已是最新）
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

	//设备版本与最新版本名一致：无需升级
	if version != "" && version == name {
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
		return row, nil
	}
	return nil, errors.New("该产品没有可用固件版本")
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
