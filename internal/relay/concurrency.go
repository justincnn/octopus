package relay

import (
	"sync"
	"sync/atomic"
)

// 渠道级并发闸: 每渠道独立在途计数, 超限的候选渠道被跳过(下一候选接管)。
// 仿 gptload keypool 的无锁 CAS 模式; 渠道 max_concurrency=0 表示不限制。
// ponytail: 全局 sync.Map 单表, 渠道数少(<100)完全够用; 若未来渠道上千可换 sharded map。
var channelInFlight sync.Map // channelID(int64) -> *atomic.Int32

// tryAcquireChannel 尝试占用渠道一个并发槽, 满则返回 false。
func tryAcquireChannel(channelID int, maxConcurrency int) bool {
	if maxConcurrency <= 0 {
		return true
	}
	v, _ := channelInFlight.LoadOrStore(int64(channelID), &atomic.Int32{})
	counter := v.(*atomic.Int32)
	for {
		cur := counter.Load()
		if cur >= int32(maxConcurrency) {
			return false
		}
		if counter.CompareAndSwap(cur, cur+1) {
			return true
		}
	}
}

// releaseChannel 释放渠道一个并发槽。
func releaseChannel(channelID int) {
	if v, ok := channelInFlight.Load(int64(channelID)); ok {
		v.(*atomic.Int32).Add(-1)
	}
}
