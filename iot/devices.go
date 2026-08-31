package iot

import (
	"errors"
	"time"

	"github.com/god-jason/iot-master/pkg/db"
	"github.com/god-jason/iot-master/pkg/lib"
)

var devices lib.Map[Device]

func GetDevice(id string) *Device {
	return devices.Load(id)
}

func LoadDevice(id string) (*Device, error) {
	var d Device
	has, err := db.Engine().ID(id).Get(&d)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, errors.New("device not exist")
	}
	err = d.Open()
	if err != nil {
		return nil, err
	}
	//挂载即视为活跃，之后由offlineWatch按超时判定
	d.lastActive = time.Now().UnixMilli()
	devices.Store(id, &d)

	return &d, nil
}

func UnloadDevice(id string) error {
	//close?
	devices.Delete(id)
	return nil
}
