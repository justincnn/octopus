package relay

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

// Key 状态: active 参与轮询, invalid 不参与(由 Validator 每小时恢复), disabled 手动禁用。
const (
	KeyStatusActive  = "active"
	KeyStatusInvalid = "invalid"

	KeyReasonInvalid       = "invalid"        // 401/403 认证失败
	KeyReasonRateLimited   = "rate_limited"   // 429 限流
	KeyReasonUpstreamError = "upstream_error" // 5xx/网络错误
	KeyReasonDisabled      = "disabled"       // 手动禁用
)

// failThreshold 同类错误连续失败次数, 达到即标记 invalid 踢出轮询池。
const failThreshold = 3

// keyState 单个 key 的内存状态。
type keyState struct {
	Key        string
	Status     string    // active / invalid
	Reason     string    // 失败原因
	FailCount  int       // 同类错误连续次数
	Disabled   bool      // 手动禁用
	LastFailAt time.Time // 上次失败时间(前端展示用)
}

// channelKeys 每渠道的 key 池(含轮询游标)。
type channelKeys struct {
	states []*keyState
	rr     atomic.Uint64
}

// keyStates 全局: channelID -> *channelKeys。
var keyStates sync.Map

// loadKeyStates 从渠道加载/刷新 key 池。
// 渠道 keys 数组为空时回退到单 key(Key 字段), 保证旧渠道行为不变。
func loadKeyStates(ch *model.Channel) *channelKeys {
	keys := ch.Keys
	if len(keys) == 0 {
		if ch.Key == "" {
			keys = nil
		} else {
			keys = []string{ch.Key}
		}
	}

	if v, ok := keyStates.Load(ch.ID); ok {
		existing := v.(*channelKeys)
		// 数组一致则复用(保留状态); 变了才重建(重置状态)
		if sameKeyList(existing.states, keys) {
			return existing
		}
	}

	ck := &channelKeys{}
	for _, k := range keys {
		ck.states = append(ck.states, &keyState{Key: k, Status: KeyStatusActive})
	}
	keyStates.Store(ch.ID, ck)
	return ck
}

func sameKeyList(old []*keyState, new []string) bool {
	if len(old) != len(new) {
		return false
	}
	for i := range old {
		if old[i].Key != new[i] {
			return false
		}
	}
	return true
}

// nextKey 轮询选择下一个可用 key: sticky 命中优先, 否则 RR 跳过 invalid/disabled。
// 返回 (key, 下标); 全部不可用时回退单 key 字段。
func nextKey(ch *model.Channel, stickyIdx int) (string, int) {
	ck := loadKeyStates(ch)
	if len(ck.states) == 0 {
		return ch.Key, -1
	}

	// sticky: 会话记住的 key 下标仍可用则直接用
	if stickyIdx >= 0 && stickyIdx < len(ck.states) {
		st := ck.states[stickyIdx]
		if st.Status == KeyStatusActive && !st.Disabled {
			return st.Key, stickyIdx
		}
	}

	n := ck.rr.Add(1) - 1
	for i := 0; i < len(ck.states); i++ {
		idx := int((n + uint64(i)) % uint64(len(ck.states)))
		st := ck.states[idx]
		if st.Status == KeyStatusActive && !st.Disabled {
			return st.Key, idx
		}
	}
	// 全部 invalid/disabled: 回退单 key(可能为空, 上层处理)
	return ch.Key, -1
}

// markKeyFail 记录 key 失败; 同类错误连续 failThreshold 次 → invalid + 原因。
func markKeyFail(ch *model.Channel, key, reason string) {
	ck := loadKeyStates(ch)
	for _, st := range ck.states {
		if st.Key != key {
			continue
		}
		if reason == st.Reason {
			st.FailCount++
		} else {
			st.FailCount = 1
			st.Reason = reason
		}
		st.LastFailAt = time.Now()
		if st.FailCount >= failThreshold {
			st.Status = KeyStatusInvalid
		}
		return
	}
}

// markKeySuccess 记录 key 成功, 复位失败计数并恢复 active。
func markKeySuccess(ch *model.Channel, key string) {
	ck := loadKeyStates(ch)
	for _, st := range ck.states {
		if st.Key != key {
			continue
		}
		st.FailCount = 0
		st.Reason = ""
		if !st.Disabled {
			st.Status = KeyStatusActive
		}
		return
	}
}

// DisableKey 手动禁用/启用 key(disabled 不参与轮询, Validator 不碰)。
func DisableKey(ch *model.Channel, key string, disabled bool) {
	ck := loadKeyStates(ch)
	for _, st := range ck.states {
		if st.Key != key {
			continue
		}
		st.Disabled = disabled
		if disabled {
			st.Status = KeyStatusInvalid
			st.Reason = KeyReasonDisabled
		} else {
			st.Status = KeyStatusActive
			st.Reason = ""
			st.FailCount = 0
		}
		return
	}
}

// KeyStateSnapshot 返回渠道 key 池状态快照(管理端展示用)。
func KeyStateSnapshot(ch *model.Channel) []keyState {
	ck := loadKeyStates(ch)
	snap := make([]keyState, 0, len(ck.states))
	for _, st := range ck.states {
		snap = append(snap, *st)
	}
	return snap
}

// invalidKeys 返回该渠道所有 invalid 且非 disabled 的 key(Validator 用)。
func invalidKeys(ch *model.Channel) []*keyState {
	ck := loadKeyStates(ch)
	var out []*keyState
	for _, st := range ck.states {
		if st.Status == KeyStatusInvalid && !st.Disabled {
			out = append(out, st)
		}
	}
	return out
}

// RecoverKey 手动/自动恢复 key。
func RecoverKey(ch *model.Channel, key string) {
	ck := loadKeyStates(ch)
	for _, st := range ck.states {
		if st.Key != key {
			continue
		}
		st.Status = KeyStatusActive
		st.Reason = ""
		st.FailCount = 0
		return
	}
}
