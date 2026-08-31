package apis

import (
	"github.com/gin-gonic/gin"
	"github.com/god-jason/iot-master/pkg/api"
	"github.com/god-jason/iot-master/pkg/db"
	"github.com/god-jason/iot-master/pkg/log"
	"github.com/god-jason/iot-master/pkg/table"
)

func init() {
	//行级过滤：非管理员在 device 表只查看被授权（绑定）的站点
	table.RowFilter = func(ctx *gin.Context, name string) map[string]any {
		log.Info("RowFilter called: table=", name, " admin=", ctx.GetBool("admin"), " uid=", ctx.GetString("user"))
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
		log.Info("RowFilter ids for ", uid, ": ", ids)
		if len(ids) == 0 {
			//未绑定任何站点：返回不可能匹配的值，保证结果为空
			return map[string]any{"id": "__no_site__"}
		}
		//数组filter是AND语义，改用$or实现 id=a OR id=b
		orCondition := map[string]any{}
		for _, sid := range ids {
			orCondition[sid] = sid
		}
		return map[string]any{"$or": map[string]any{"id": orCondition}}
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
