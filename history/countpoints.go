package history

import (
	"context"

	"github.com/god-jason/iot-master/pkg/config"
)

// countPoints 统计时间段内原始点数(不带聚合, 长范围也稳定), 用于区分
// "确实无数据" 与 "聚合缺陷返回空" —— 后者才需要走分片兜底
func countPoints(table, id, start, end string) (int64, error) {
	bucket := config.GetString(MODULE, "bucket")
	flux := "from(bucket: \"" + bucket + "\") |> range(start: " + start + ", stop: " + end + ")\n"
	flux += "|> filter(fn: (r) => r[\"_measurement\"] == \"" + table + "\")\n"
	flux += "|> filter(fn: (r) => r[\"id\"] == \"" + id + "\")\n"
	flux += "|> count()"

	result, err := reader.Query(context.Background(), flux)
	if err != nil {
		return 0, err
	}
	var total int64
	for result.Next() {
		switch v := result.Record().Value().(type) {
		case int64:
			total += v
		case float64:
			total += int64(v)
		}
	}
	return total, result.Err()
}
