package apis

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/god-jason/iot-master/pkg/api"
	"github.com/god-jason/iot-master/pkg/db"
	"github.com/god-jason/iot-master/pkg/table"
)

func init() {
	//行级过滤：非管理员在 device 表只查看被授权（绑定）的站点
	table.RowFilter = func(ctx *gin.Context, name string) map[string]any {
		if name != "device" {
			return nil
		}
		if ctx.GetBool("admin") {
			return nil
		}
		uid := ctx.GetString("user")
		if uid == "" {
			return nil
		}
		ids := GetUserSiteIds(uid)
		if len(ids) == 0 {
			//未绑定任何站点：返回不可能匹配的值，保证结果为空
			return map[string]any{"id": map[string]any{"$in": []string{"__no_site__"}}}
		}
		//同字段多值用 $in：id IN (绑定的站点)
		return map[string]any{"id": map[string]any{"$in": ids}}
	}

	//写操作守卫：数据查看=只读；设备管理才能写业务表；系统表仅系统权限
	table.WriteGuard = func(ctx *gin.Context, name string, op string) error {
		if ctx.GetBool("admin") {
			return nil
		}
		uid := ctx.GetString("user")
		if uid == "" {
			return errors.New("未授权")
		}
		priv := loadPrivs(uid)
		systemTables := map[string]bool{"user": true, "user_site": true, "group": true, "member": true, "password": true}
		if systemTables[name] {
			if !priv["system"] {
				return errors.New("无权限：需要系统管理权限")
			}
			return nil
		}
		if !priv["device_manage"] {
			return errors.New("无权限：数据查看为只读，需要设备管理权限才能修改")
		}
		return nil
	}

	//当前用户被授权的站点列表（供前端展示）
	api.Register("GET", "site/my", func(ctx *gin.Context) {
		uid := ctx.GetString("user")
		type row struct {
			SiteId   string `json:"site_id"`
			SiteName string `json:"site_name"`
		}
		var rows []row
		err := db.Engine().Table("user_site").Where("user_id=?", uid).Cols("site_id", "site_name").Find(&rows)
		if err != nil {
			api.Error(ctx, err)
			return
		}
		api.OK(ctx, rows)
	})
}

// loadPrivs 读取用户权限集合
func loadPrivs(uid string) map[string]bool {
	priv := map[string]bool{}
	type row struct {
		PrivDataView     bool `json:"priv_data_view"`
		PrivDeviceManage bool `json:"priv_device_manage"`
		PrivSystem       bool `json:"priv_system"`
	}
	var r row
	if has, err := db.Engine().Table("user").Where("id=?", uid).Get(&r); err == nil && has {
		if r.PrivDataView {
			priv["data_view"] = true
		}
		if r.PrivDeviceManage {
			priv["device_manage"] = true
		}
		if r.PrivSystem {
			priv["system"] = true
		}
	}
	return priv
}

// GetUserSiteIds 查询用户被授权的站点ID列表
func GetUserSiteIds(uid string) []string {
	type row struct {
		SiteId string `json:"site_id"`
	}
	var rows []row
	err := db.Engine().Table("user_site").Where("user_id=?", uid).Cols("site_id").Find(&rows)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.SiteId)
	}
	return ids
}
