package history

import (
	"sync"
	"time"
)

// 历史查询结果短TTL缓存 + 按键单飞:
// 前端数据页一次并发请求全部因子(同设备同范围同窗口), 逐因子独立查询会把
// 同一份influxdb查询重复执行N次, 造成曲线加载缓慢;
// 以(设备,范围,窗口,方法)为键缓存30秒, 同键并发请求只穿透一次influxdb。
// 锁按key独立(而非全局锁), 不同设备/不同范围互不阻塞。
type pointCacheEntry struct {
	points []*Point
	exp    time.Time
}

var (
	pcMu    sync.Mutex
	pcData  = map[string]pointCacheEntry{}
	pcLocks = map[string]*keyLock{}
	pcTTL   = 30 * time.Second
	pcLimit = 64
)

// keyLock 按key的互斥锁: refs计数用于无引用时回收, 避免锁表无限增长
type keyLock struct {
	mu   sync.Mutex
	refs int
}

func pointCacheKey(table, id, start, end, window, method string) string {
	return table + "|" + id + "|" + start + "|" + end + "|" + window + "|" + method
}

func acquireKeyLock(key string) *keyLock {
	pcMu.Lock()
	kl, ok := pcLocks[key]
	if !ok {
		kl = &keyLock{}
		pcLocks[key] = kl
	}
	kl.refs++
	pcMu.Unlock()
	kl.mu.Lock()
	return kl
}

func releaseKeyLock(key string, kl *keyLock) {
	kl.mu.Unlock()
	pcMu.Lock()
	kl.refs--
	if kl.refs <= 0 {
		delete(pcLocks, key)
	}
	pcMu.Unlock()
}

// cachedQueryFields 带短TTL缓存的全因子查询; 同键并发只穿透一次influxdb
func cachedQueryFields(table, id, name, start, end, window, method string) ([]*Point, error) {
	key := pointCacheKey(table, id, start, end, window, method)

	kl := acquireKeyLock(key)
	defer releaseKeyLock(key, kl)

	//双检: 拿到key锁后先看缓存(前一个同键请求可能刚填充完)
	pcMu.Lock()
	if e, ok := pcData[key]; ok && time.Now().Before(e.exp) {
		pcMu.Unlock()
		return e.points, nil
	}
	//顺手清理过期项与新锁表
	for k, e := range pcData {
		if time.Now().After(e.exp) {
			delete(pcData, k)
			if kl2, ok := pcLocks[k]; ok && kl2.refs <= 0 {
				delete(pcLocks, k)
			}
		}
	}
	if len(pcData) >= pcLimit {
		pcData = map[string]pointCacheEntry{} //超限直接清空(简单防膨胀)
	}
	pcMu.Unlock()

	all, err := QueryFields(table, id, name, start, end, window, method)
	if err != nil {
		return nil, err
	}

	pcMu.Lock()
	pcData[key] = pointCacheEntry{points: all, exp: time.Now().Add(pcTTL)}
	pcMu.Unlock()
	return all, nil
}
