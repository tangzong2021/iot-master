package history

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/god-jason/iot-master/pkg/config"
	"github.com/god-jason/iot-master/pkg/log"
	"github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

var client influxdb2.Client
var writerBlocking api.WriteAPIBlocking
var reader api.QueryAPI

// 写入失败落盘队列: 保证 InfluxDB 故障/字段类型冲突/网络中断时不丢数据
// - 写入重试3次仍失败 → 追加一行JSON到队列文件(位于PVC持久卷)
// - 后台协程每15秒重试, 成功后删除; 重启后自动续传
// - 队列超过50MB时丢弃最旧一半, 防止磁盘打满
const (
	retryQueueFile     = "/app-data/influx_retry_queue.jsonl"
	retryQueueMaxBytes = 50 * 1024 * 1024
	retryInterval      = 15 * time.Second
)

var queueMutex sync.Mutex

type queueItem struct {
	Table     string         `json:"table"`
	Id        string         `json:"id"`
	Timestamp int64          `json:"ts"`
	Values    map[string]any `json:"values"`
}

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

	//启动落盘队列续传协程(含重启后恢复未写入的数据)
	go retryLoop()

	return nil
}

func Shutdown() error {
	client.Close()
	return nil
}

func Client() influxdb2.Client {
	return client
}

// buildPoint 把业务值转换为InfluxDB点(过滤无效字段与NaN/Inf)
func buildPoint(table, id string, timestamp int64, values map[string]any) *write.Point {
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
		return nil
	}
	return write.NewPoint(table, map[string]string{"id": id}, vs, time.UnixMilli(timestamp))
}

func Write(table, id string, timestamp int64, values map[string]any) error {
	//log.Info("influxdb write", table, id, timestamp, values)

	if writerBlocking == nil {
		return nil
	}

	pt := buildPoint(table, id, timestamp, values)
	if pt == nil {
		log.Info("nothing to write", values)
		return nil
	}

	//同步写入, 失败重试3次(间隔递增), 避免网络抖动导致数据丢失
	var err error
	for i := 0; i < 3; i++ {
		if err = writerBlocking.WritePoint(context.Background(), pt); err == nil {
			return nil
		}
		log.Error("influxdb write retry ", i+1, ": ", err)
		time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
	}

	//三次重试仍失败: 落盘持久化, 由后台协程续传, 确保数据不丢
	queueAppend(&queueItem{Table: table, Id: id, Timestamp: timestamp, Values: values})
	log.Error("influxdb write failed, queued to disk: ", err)
	return err
}

// queueAppend 追加一条待重试数据到队列文件
func queueAppend(item *queueItem) {
	queueMutex.Lock()
	defer queueMutex.Unlock()

	//队列过大时丢弃最旧一半, 防止磁盘打满
	if fi, err := os.Stat(retryQueueFile); err == nil && fi.Size() > retryQueueMaxBytes {
		if data, err := os.ReadFile(retryQueueFile); err == nil {
			var valid [][]byte
			for _, l := range bytes.Split(data, []byte("\n")) {
				if len(bytes.TrimSpace(l)) > 0 {
					cp := make([]byte, len(l))
					copy(cp, l)
					valid = append(valid, cp)
				}
			}
			keep := valid[len(valid)/2:]
			_ = os.WriteFile(retryQueueFile, bytes.Join(keep, []byte("\n")), 0644)
			log.Error("influx retry queue exceed limit, dropped oldest ", len(valid)-len(keep))
		}
	}

	data, err := json.Marshal(item)
	if err != nil {
		log.Error("influx retry queue marshal fail: ", err)
		return
	}
	f, err := os.OpenFile(retryQueueFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Error("influx retry queue open fail: ", err)
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

// retryLoop 后台续传: 定期重试队列中未写入成功的数据
func retryLoop() {
	time.Sleep(10 * time.Second) //等平台启动稳定
	for {
		retryQueueOnce()
		time.Sleep(retryInterval)
	}
}

// retryQueueOnce 尝试把队列中的全部数据写入, 成功的移除, 失败的保留等下次
func retryQueueOnce() {
	if writerBlocking == nil {
		return
	}

	queueMutex.Lock()
	defer queueMutex.Unlock()

	data, err := os.ReadFile(retryQueueFile)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return
	}

	var remain [][]byte
	okCount := 0
	failCount := 0
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var item queueItem
		if json.Unmarshal(line, &item) != nil {
			continue //损坏行直接丢弃
		}
		pt := buildPoint(item.Table, item.Id, item.Timestamp, item.Values)
		if pt != nil {
			if err := writerBlocking.WritePoint(context.Background(), pt); err == nil {
				okCount++
				continue
			}
		}
		failCount++
		cp := make([]byte, len(line))
		copy(cp, line)
		remain = append(remain, cp)
	}

	//写回剩余(全部成功则删除文件)
	if len(remain) == 0 {
		_ = os.Remove(retryQueueFile)
	} else {
		_ = os.WriteFile(retryQueueFile, bytes.Join(remain, []byte("\n")), 0644)
	}
	if okCount > 0 {
		log.Info("influx retry queue flushed ", okCount, ", remain ", len(remain))
	}
	if failCount > 0 {
		log.Error("influx retry queue still failing ", failCount, ", will retry in ", retryInterval)
	}
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
