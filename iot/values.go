package iot

import (
	"sync"
	"time"
)

type Values struct {
	values map[string]any
	lock   sync.RWMutex
	//Updated time.Time `json:"updated"`
}

func (v *Values) Put(values map[string]any) {
	v.PutTime(0, values)
}

// PutTime 保存数据。ts为数据时间戳（毫秒），0表示使用服务器当前时间
func (v *Values) PutTime(ts int64, values map[string]any) {
	if len(values) == 0 {
		return
	}

	v.lock.Lock()
	defer v.lock.Unlock()

	if v.values == nil {
		v.values = make(map[string]any)
	}

	//逐一复制
	for key, value := range values {
		v.values[key] = value
	}

	//更新时间（断网补发时为设备携带的采集时间）
	if ts <= 0 {
		ts = time.Now().UnixMilli()
	}
	v.values["_update"] = time.UnixMilli(ts)
}

func (v *Values) Get() map[string]any {
	return v.values
}
