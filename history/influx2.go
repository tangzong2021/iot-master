package history

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/god-jason/iot-master/pkg/config"
	"github.com/god-jason/iot-master/pkg/log"
	"github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

var client influxdb2.Client
var writer api.WriteAPI
var writerBlocking api.WriteAPIBlocking
var reader api.QueryAPI

type Point struct {
	Value any   `json:"value"`
	Time  int64 `json:"time"`
}

func Startup() error {
	if !config.GetBool(MODULE, "enable") {
		return nil
	}

	client = influxdb2.NewClient(config.GetString(MODULE, "url"), config.GetString(MODULE, "token"))
	//同步阻塞写入器: 异步WriteAPI在网络抖动后会让批次卡死/过期丢弃(实测丢数据),
	//改为逐点同步写入+失败重试, 保证数据不丢(写入量小, 性能足够)
	writerBlocking = client.WriteAPIBlocking(config.GetString(MODULE, "org"), config.GetString(MODULE, "bucket"))
	reader = client.QueryAPI(config.GetString(MODULE, "org"))

	return nil
}

func Shutdown() error {
	client.Close()
	return nil
}

func Client() influxdb2.Client {
	return client
}

func Write(table, id string, timestamp int64, values map[string]any) error {
	//log.Info("influxdb write", table, id, timestamp, values)

	if writerBlocking == nil {
		return nil
	}

	vs := make(map[string]any)
	for k, v := range values {
		k = strings.TrimSpace(k)

		//过滤无效字段名
		if k == "" {
			continue
		}
		//处理数据类型
		switch val := v.(type) {

		case int, int32, int64:
			vs[k] = val

		case float32:
			vs[k] = float64(val)

		case float64:
			if !math.IsNaN(val) && !math.IsInf(val, 0) {
				vs[k] = val
			}

		case bool:
			vs[k] = val

		case string:
			if val != "" {
				vs[k] = val
			}
		}
	}
	if len(vs) == 0 {
		log.Info("nothing to write", values, vs)
		return nil
	}
	//log.Info("influxdb write2", table, id, vs)

	pt := write.NewPoint(table, map[string]string{"id": id}, vs, time.UnixMilli(timestamp))
	//同步写入, 失败重试3次(间隔递增), 避免网络抖动导致数据丢失
	var err error
	for i := 0; i < 3; i++ {
		if err = writerBlocking.WritePoint(context.Background(), pt); err == nil {
			return nil
		}
		log.Error("influxdb write retry ", i+1, ": ", err)
		time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
	}
	return err
}

func Query(table, id, name, start, end, window, method string) ([]*Point, error) {
	if reader == nil {
		return nil, errors.New("influxdb未启用")
	}

	bucket := config.GetString(MODULE, "bucket")

	flux := "from(bucket: \"" + bucket + "\")\n"
	flux += "|> range(start: " + start + ", stop: " + end + ")\n"
	flux += "|> filter(fn: (r) => r[\"_measurement\"] == \"" + table + "\")\n"
	flux += "|> filter(fn: (r) => r[\"id\"] == \"" + id + "\")\n"
	flux += "|> filter(fn: (r) => r[\"_field\"] == \"" + name + "\")"
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

	var records []*Point
	for result.Next() {
		//result.TableChanged() 查询多个数值的情况？？？
		records = append(records, &Point{
			Value: result.Record().Value(),
			Time:  result.Record().Time().UnixMilli(),
		})
	}
	return records, result.Err()
}
