package iot

import (
	"time"

	"github.com/god-jason/iot-master/pkg/log"
	"github.com/god-jason/iot-master/pkg/mqtt"
	"github.com/spf13/viper"
)

func init() {
	//数据活跃超时判定离线，默认1小时。可在 iot-master.yaml 配置：offline_timeout: 1h
	viper.SetDefault("offline_timeout", time.Hour)
}

// touch 更新设备活跃时间（收到数据、注册、定位时调用）
func (d *Device) touch() {
	d.lastActive = time.Now().UnixMilli()
}

// offlineWatch 周期检查：在线设备超过 offline_timeout 无活跃则按离线处理。
// 复用 device/{id}/offline 事件通道：统一处理缓存、数据库、子设备联动与设备日志，
// 补齐"连接未断开但不再上数据"（如蜂窝网络半开连接）无法触发broker断连事件的场景
func offlineWatch() {
	for {
		time.Sleep(time.Minute)
		timeout := viper.GetDuration("offline_timeout")
		if timeout <= 0 {
			continue
		}
		now := time.Now().UnixMilli()
		devices.Range(func(name string, d *Device) bool {
			if d.Online && d.lastActive > 0 && now-d.lastActive > timeout.Milliseconds() {
				log.Info("device offline by timeout: ", name)
				mqtt.Publish("device/"+name+"/offline", nil)
			}
			return true
		})
	}
}
