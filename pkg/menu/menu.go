package menu

import (
	"encoding/json"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/god-jason/iot-master/pkg/api"
	"github.com/god-jason/iot-master/pkg/db"
)

type Item struct {
	Name       string   `json:"name"`
	Title      string   `json:"title,omitempty"`
	Icon       string   `json:"icon,omitempty"`
	Url        string   `json:"url,omitempty"`
	External   bool     `json:"external,omitempty"`
	Privileges []string `json:"privileges,omitempty"`
	Admin      bool     `json:"admin,omitempty"` //管理员
}

type Menu struct {
	Name       string   `json:"name"`
	Title      string   `json:"title,omitempty"`
	NzIcon     string   `json:"nz_icon,omitempty"` //ant.design图标库
	Items      []*Item  `json:"items,omitempty"`
	Index      int      `json:"index,omitempty"`
	Privileges []string `json:"privileges,omitempty"`
	Admin      bool     `json:"admin,omitempty"` //管理员
	//Domain     []string `json:"domain"` //域 admin project 或 dealer等
}

var menus []Menu
var filename string = "menu.json" //默认menu.json

func init() {
	api.RegisterUnAuthorized("GET", "menu", func(ctx *gin.Context) {
		//加载菜单定义（首次访问时从文件加载并缓存）
		if menus == nil {
			if buf, err := os.ReadFile(filename); err == nil {
				_ = json.Unmarshal(buf, &menus)
			}
		}
		if menus == nil {
			ctx.File(filename)
			return
		}

		//未登录或管理员：返回全部
		uid := ctx.GetString("user")
		if uid == "" || ctx.GetBool("admin") {
			ctx.JSON(200, menus)
			return
		}

		//普通用户：按权限过滤菜单（用户表 priv_* 布尔字段）
		privs := loadUserPrivileges(uid)
		ctx.JSON(200, filterMenus(menus, privs))
	})
}

// loadUserPrivileges 读取用户权限集合
func loadUserPrivileges(uid string) map[string]bool {
	privs := make(map[string]bool)
	type row struct {
		PrivDataView     bool `json:"priv_data_view"`
		PrivDeviceManage bool `json:"priv_device_manage"`
		PrivSystem       bool `json:"priv_system"`
	}
	var r row
	if has, err := db.Engine().Table("user").ID(uid).Get(&r); err == nil && has {
		if r.PrivDataView {
			privs["data_view"] = true
		}
		if r.PrivDeviceManage {
			privs["device_manage"] = true
		}
		if r.PrivSystem {
			privs["system"] = true
		}
	}
	return privs
}

// hasPrivilege 菜单项无权限标签 = 所有人可见；有标签 = 用户需持有其一
func hasPrivilege(privs map[string]bool, tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	for _, t := range tags {
		if privs[t] {
			return true
		}
	}
	return false
}

func filterMenus(all []Menu, privs map[string]bool) []Menu {
	var result []Menu
	for _, m := range all {
		if !hasPrivilege(privs, m.Privileges) {
			continue
		}
		var items []*Item
		for _, it := range m.Items {
			if hasPrivilege(privs, it.Privileges) {
				items = append(items, it)
			}
		}
		if len(items) > 0 {
			m.Items = items
			result = append(result, m)
		}
	}
	return result
}

func Content(buf []byte) error {
	return json.Unmarshal(buf, &menus)
}

func File(fn string) {
	filename = fn
}
