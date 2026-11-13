package history

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/god-jason/iot-master/pkg/config"
)

// QueryFields 查询目标因子的历史点位。
// 内部不带_field过滤一次取全部因子后按因子名分组提取, 规避influxdb 2.7.12实测缺陷:
// aggregateWindow在长查询范围(数小时以上)会静默返回空结果(短范围正常、无报错,
// 同flux经CLI/HTTP/Go客户端行为也不一致); 处理: 按2小时分片逐段查询后合并——
// 分片边界与聚合窗口边界对齐(UTC偶数小时), 不产生跨片截断的窗口; 各分片按时间排序合并。
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

	all, err := queryAllChunked(table, id, st, en, window, method)
	if err != nil {
		return nil, err
	}

	//目标因子按时间升序排序后返回
	pts := all[name]
	sort.Slice(pts, func(i, j int) bool { return pts[i].Time < pts[j].Time })
	return pts, nil
}

// queryAllChunked 查询时间段内全因子点位。
// 快路径: 单次查询(TSI索引修复后长范围聚合已验证稳定, 30天约300~600ms, 一次往返);
// 兜底: 若长范围(>4h)单次查询返回空, 判定为influxdb长范围聚合缺陷复发, 退回2小时分片查询。
// 分片查询为并行执行(并发上限chunkWorkers), 结果按下标回填保证合并顺序稳定。
func queryAllChunked(table, id string, st, en time.Time, window, method string) (map[string][]*Point, error) {
	s := st.UTC().Format(time.RFC3339Nano)
	e := en.UTC().Format(time.RFC3339Nano)

	fields, err := queryChunk(table, id, s, e, window, method)
	if err != nil {
		return nil, err
	}
	//范围内有数据就直接返回; 空结果且范围超过4小时时才走分片兜底(短范围空=确实无数据)
	if len(fields) > 0 || en.Sub(st) <= 4*time.Hour {
		return fields, nil
	}

	const chunk = 2 * time.Hour
	const chunkWorkers = 4

	type span struct{ s, e string }
	var spans []span
	for cur := st; cur.Before(en); {
		next := cur.Add(chunk)
		if next.After(en) {
			next = en
		}
		spans = append(spans, span{cur.UTC().Format(time.RFC3339Nano), next.UTC().Format(time.RFC3339Nano)})
		cur = next
	}

	parts := make([]map[string][]*Point, len(spans))
	errs := make([]error, len(spans))

	var wg sync.WaitGroup
	sem := make(chan struct{}, chunkWorkers)
	for i := range spans {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			parts[idx], errs[idx] = queryChunk(table, id, spans[idx].s, spans[idx].e, window, method)
		}(i)
	}
	wg.Wait()

	merged := make(map[string][]*Point)
	for i := range spans {
		if errs[i] != nil {
			return nil, errs[i]
		}
		for fname, pts := range parts[i] {
			merged[fname] = append(merged[fname], pts...)
		}
	}
	return merged, nil
}

// queryChunk 查询单个时间分片(不带_field过滤, 返回全部因子点位并按因子名分组)
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
