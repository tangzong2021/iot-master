package history

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/god-jason/iot-master/pkg/config"
)

// QueryFields 查询设备全部因子(不带_field过滤)后按因子名分组返回。
// 规避influxdb 2.7 TSI索引在"长时间范围+_field过滤"组合下静默返回空结果的缺陷:
// 实测24小时范围带字段过滤返回空, 同范围不带字段过滤数据完整; 一次取全部因子
// 还能把前端逐因子查询合并为单次查询, 减少influxdb查询压力
func QueryFields(table, id, name, start, end, window, method string) ([]*Point, error) {
	if reader == nil {
		return nil, errors.New("influxdb未启用")
	}

	bucket := config.GetString(MODULE, "bucket")

	flux := "from(bucket: \"" + bucket + "\")\n"
	flux += "|> range(start: " + start + ", stop: " + end + ")\n"
	flux += "|> filter(fn: (r) => r[\"_measurement\"] == \"" + table + "\")\n"
	flux += "|> filter(fn: (r) => r[\"id\"] == \"" + id + "\")"
	//window留空=原始采样点查询, 保留设备真实采样时间; 传了窗口才做桶聚合
	if window != "" {
		agg := "|> aggregateWindow(every: " + window + ", fn: " + method + ", createEmpty: false"
		//选择器算子(last/first/min/max)保留被选中样本的真实采样时间(设备_time入库时间):
		//断网补发的数据按原采集时间入库, 标签跟随真实时间才不会错乱; 均值类算子无单一采样时刻, 用桶边界
		switch method {
		case "last", "first", "min", "max":
			agg += ", timeSrc: \"_time\", timeDst: \"_time\""
		}
		//聚合桶按服务器本地时区零点对齐(默认按UTC对齐, 1天窗口在东八区会按早上8点切分,
		//导致用户选一天的范围却出现两组数据); offset=本地零点相对UTC零点的偏移
		_, sec := time.Now().Zone()
		off := (86400 - (sec % 86400)) % 86400
		if off > 0 {
			agg += fmt.Sprintf(", offset: %ds", off)
		}
		agg += ")\n"
		flux += agg
		flux += "|> yield(name: \"" + method + "\")"
	}

	result, err := reader.Query(context.Background(), flux)
	if err != nil {
		return nil, err
	}

	fields := make(map[string][]*Point)
	for result.Next() {
		rec := result.Record()
		fields[rec.Field()] = append(fields[rec.Field()], &Point{
			Value: rec.Value(),
			Time:  rec.Time().UnixMilli(),
		})
	}
	return fields[name], result.Err()
}
