package apis

import "time"

// User 用户
type User struct {
	Id       string    `json:"id" xorm:"pk"`
	TenantId string    `json:"tenant_id,omitempty"` //多租户
	Name     string    `json:"name,omitempty"`
	Admin    bool      `json:"admin,omitempty"`
	Oem      string    `json:"oem,omitempty"`
	Disabled bool      `json:"disabled,omitempty"`
	//细粒度权限
	PrivDataView     bool `json:"priv_data_view,omitempty"`
	PrivDeviceManage bool `json:"priv_device_manage,omitempty"`
	PrivSystem       bool `json:"priv_system,omitempty"`
	Created  time.Time `json:"created,omitempty" xorm:"created"`
}

// Password 密码
type Password struct {
	Id       string `json:"id" xorm:"pk"`
	Password string `json:"password"`
}

type UserLog struct {
	Id      string    `json:"id"`
	Name    string    `json:"name,omitempty"`
	Action  string    `json:"action,omitempty"`
	Client  string    `json:"client,omitempty"`
	Ip      string    `json:"ip,omitempty"`
	Created time.Time `json:"created,omitempty" xorm:"created"`
}
