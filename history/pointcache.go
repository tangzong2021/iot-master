package history

import (
	"sync"
	"time"
)

// 历史查询结果短TTL缓存:
// 前端数据页一次并发请求全部因子(同设备同范围同窗口), 逐因子独立查询会把
// 同一份influxdb查询重复执行N次(分片模式下放大为N×分片数), 造成曲线加载缓慢;
// 以(设备,范围,窗口)为键缓存结果30秒, 并发请求只穿透一次influxdb
type pointCacheEntry struct {
	points []*Point
	exp    time.Time
}

var (
	pcMu    sync.Mutex
	pcData  = map[string]pointCacheEntry{}
	pcTTL   = 30 * time.Second
	pcLimit = 64
)

func pointCacheKey(table, id, start, end, window, method string) string {
	return table + "|" + id + "|" + start + "|" + end + "|" + window + "|" + method
}

// cachedQueryFields 带短TTL缓存的全因子查询; 结果为只读共享, 调用方不得修改
func cachedQueryFields(table, id, name, start, end, window, method string) ([]*Point, error) {
	key := pointCacheKey(table, id, start, end, window, method)

	pcMu.Lock()
	if e, ok := pcData[key]; ok && time.Now().Before(e.exp) {
		pcMu.Unlock()
		return e.points, nil
	}
	//顺手清理全部过期项
	for k, e := range pcData {
		if time.Now().After(e.exp) {
			delete(pcData, k)
		}
	}
	if len(pcData) >= pcLimit {
		pcData = map[string]pointCacheEntry{} //超限直接清空(简单防膨胀)
	}

	//单飞: 查询全程持锁, 并发同键请求(前端一次拉N个因子)只穿透一次influxdb,
	//其余等待后直接命中缓存; 全局串行化也顺带保护influxdb不被历史查询突发打满
	all, err := QueryFields(table, id, name, start, end, window, method)
	if err != nil {
		pcMu.Unlock()
		return nil, err
	}

	pcData[key] = pointCacheEntry{points: all, exp: time.Now().Add(pcTTL)}
	pcMu.Unlock()
	return all, nil
}
