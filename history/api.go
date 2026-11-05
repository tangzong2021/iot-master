package history

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/god-jason/iot-master/pkg/api"
	"github.com/god-jason/iot-master/pkg/db"
)

func init() {
	api.Register("GET", "device/:id/history/:point", deviceHistory)
	//批量接口: 一次返回该设备全部因子(前端曲线页单请求加载, 避免逐因子N倍查询放大)
	api.Register("GET", "device/:id/history-all", deviceHistoryAll)
	//api.Register("GET", "influxdb/device/:id/history/:point", deviceHistory)
}

type Device struct {
	Id        string `json:"id" xorm:"pk"`
	ProductId string `json:"product_id" xorm:"index"`
}

func deviceHistory(ctx *gin.Context) {
	var dev Device
	has, err := db.Engine().ID(ctx.Param("id")).Get(&dev)
	if err != nil {
		api.Error(ctx, err)
		return
	}
	if !has {
		api.Fail(ctx, "找不到设备")
		return
	}

	key := ctx.Param("point")
	start := ctx.DefaultQuery("start", "-5h")
	end := ctx.DefaultQuery("end", "0h")
	window := ctx.Query("window")                //留空=原始采样点(不聚合, 时间为设备真实采样时间)
	method := ctx.DefaultQuery("method", "last") //last

	//不带_field过滤一次取全部因子后提取目标因子:
	//规避influxdb 2.7长时间范围+字段过滤静默返回空的缺陷
	all, err := QueryFields(dev.ProductId, dev.Id, key, start, end, window, method)
	if err != nil {
		api.Error(ctx, err)
		return
	}

	api.OK(ctx, all)
}

// deviceHistoryAll 返回设备全部因子的历史数据(按因子名分组的map)。
// 前端曲线页改为单请求加载后, influxdb查询从N因子×分片数次降为分片数次
func deviceHistoryAll(ctx *gin.Context) {
	var dev Device
	has, err := db.Engine().ID(ctx.Param("id")).Get(&dev)
	if err != nil {
		api.Error(ctx, err)
		return
	}
	if !has {
		api.Fail(ctx, "找不到设备")
		return
	}
	start := ctx.DefaultQuery("start", "-5h")
	end := ctx.DefaultQuery("end", "0h")
	window := ctx.Query("window")
	method := ctx.DefaultQuery("method", "last")

	st, err1 := time.Parse(time.RFC3339Nano, start)
	en, err2 := time.Parse(time.RFC3339Nano, end)
	if err1 != nil || err2 != nil {
		api.Error(ctx, err1)
		return
	}
	fields, err := queryAllChunked(dev.ProductId, dev.Id, st, en, window, method)
	if err != nil {
		api.Error(ctx, err)
		return
	}
	api.OK(ctx, fields)
}
