package history

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/god-jason/iot-master/pkg/config"
)

// QueryFields 查询设备全部因子(不带_field过滤)后按因子名分组, 返回目标因子的点位。
// 规避influxdb 2.7.12实测缺陷: aggregateWindow在长查询范围(数小时以上)会静默返回空结果
// (短范围正常, 且无报错; 同flux经CLI/HTTP直查行为也不一致)。
// 处理: 按2小时分片逐段查询后合并——分片边界与聚合窗口边界对齐(UTC偶数小时),
// 不产生跨片截断的窗口; 各分片结果按时间排序合并。
// 附带收益: 不带字段过滤一次取全部因子, 前端逐因子查询合并为单次influxdb往返
func QueryFields(table, id, name, start, end, window, method string) ([]*Point, error) {
	if reader == nil {
		return nil, errors.New("influxdb未启用")
	}

	st, err1 := time.Parse(time.RFC3339Nano, start)
	en, err2 := time.Parse(time.RFC3339Nano, end)
	if err1 != nil || err2 != nil || !en.After(st) {
		//时间解析失败或范围异常: 退回单次全因子查询
		all, err := queryChunk(table, id, start, end, window, method)
		if err != nil {
			return nil, err
		}
		return all[name], nil
	}

	fields := make(map[string][]*Point)
	const chunk = 2 * time.Hour
	for cur := st; cur.Before(en); {
		next := cur.Add(chunk)
		if next.After(en) {
			next = en
		}
		s := cur.UTC().Format(time.RFC3339Nano)
		e := next.UTC().Format(time.RFC3339Nano)
		part, err := queryChunk(table, id, s, e, window, method)
		if err != nil {
			return nil, err
		}
		for fname, pts := range part {
			fields[fname] = append(fields[fname], pts...)
		}
		cur = next
	}

	//目标因子按时间升序排序后返回
	pts := fields[name]
	sort.Slice(pts, func(i, j int) bool { return pts[i].Time < pts[j].Time })
	return pts, nil
}

// rec_field 从点位值推断因子名(合并分片时需要): Point不含字段名,
// 因此合并逻辑放在查询循环内按result.Record().Field()分组, 这里仅排序用
func rec_field(p *Point) string { return "" }

// queryChunk 查询单个时间分片(不带_field过滤, 返回全因子点位并按因子名分组)
func queryChunk(table, id, start, end, window, method string) (map[string][]*Point, error) {
	bucket := config.GetString(MODULE, "bucket")

	flux := "from(bucket: \"" + bucket + "\")\n"
	flux += "|> range(start: " + start + ", stop: " + end + ")\n"
	flux += "|> filter(fn: (r) => r[\"_measurement\"] == \"" + table + "\")\n"
	flux += "|> filter(fn: (r) => r[\"id\"] == \"" + id + "\")"
	if window != "" {
		agg := "|> aggregateWindow(every: " + window + ", fn: " + method + ", createEmpty: false"
		switch method {
		case "last", "first", "min", "max":
			agg += ", timeSrc: \"_time\", timeDst: \"_time\""
		}
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
		name := rec.Field()
		fields[name] = append(fields[name], &Point{
			Value: rec.Value(),
			Time:  rec.Time().UnixMilli(),
		})
	}
	return fields, result.Err()
}
