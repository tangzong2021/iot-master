package history

import (
	"time"
	"github.com/gin-gonic/gin"
	"github.com/god-jason/iot-master/pkg/api"
	"github.com/god-jason/iot-master/pkg/db"
)

func init() {
	api.Register("GET", "device/:id/history/:point", deviceHistory)
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
	window := ctx.Query("window") //留空=原始采样点(不聚合, 时间为设备真实采样时间)
	method := ctx.DefaultQuery("method", "last") //last

	//范围对齐到窗口网格: 起点向下取整、终点向上取整, 保证聚合时间标签均为窗口整数倍
	//否则首桶被查询起点截断、末桶被终点截断, 标签会落在非网格时刻(如16:14:49)
	if d, err := time.ParseDuration(window); err == nil && d > 0 {
		if st, e1 := time.Parse(time.RFC3339, start); e1 == nil {
			start = st.Truncate(d).Format(time.RFC3339Nano)
		}
		if en, e2 := time.Parse(time.RFC3339, end); e2 == nil {
			end = en.Truncate(d).Add(d).Format(time.RFC3339Nano)
		}
	}

	points, err := Query(dev.ProductId, dev.Id, key, start, end, window, method)
	if err != nil {
		api.Error(ctx, err)
		return
	}

	api.OK(ctx, points)
}
