package balancer

import (
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

// 分组 item 状态机: 与渠道 key 池(keypool)同构——失败连 failThreshold 次踢出轮询,
// 由定时任务(itemValidator)恢复, 策略可选(round_robin/random/least_used/priority)。
const (
	ItemStatusActive  = "active"
	ItemStatusInvalid = "invalid"

	ItemReasonInvalid       = "invalid"        // 401/403 认证失败
	ItemReasonRateLimited   = "rate_limited"   // 429 限流
	ItemReasonUpstreamError = "upstream_error" // 5xx/网络错误
)

// 轮询策略(与渠道 key 池一致)。
const (
	StrategyRoundRobin = "round_robin"
	StrategyRandom     = "random"
	StrategyLeastUsed  = "least_used"
	StrategyPriority   = "priority"
	StrategyDefault    = StrategyRoundRobin
)

// itemFailThreshold 同类错误连续失败次数, 达到即标记 invalid 踢出轮询池。
// 每渠道可用其 max_failures 覆盖(经 MarkItemFailWithThreshold); itemFailThreshold 是全局限量默认。
const itemFailThreshold = 3

// itemState 单个分组 item 的内存状态。
type itemState struct {
	ItemID     int
	Status     string // active / invalid
	Reason     string
	FailCount  int // 同类错误连续次数
	LastFailAt time.Time
	UseCount   uint64 // 累计使用次数(least_used 用; 内存态, 重启清零)
}

// itemStates 全局: groupItemID -> *itemState。
var itemStates sync.Map

// getItemState 懒创建 item 状态。
func getItemState(itemID int) *itemState {
	if v, ok := itemStates.Load(itemID); ok {
		return v.(*itemState)
	}
	st := &itemState{ItemID: itemID, Status: ItemStatusActive}
	actual, _ := itemStates.LoadOrStore(itemID, st)
	return actual.(*itemState)
}

// isItemActive 查询 item 是否在轮询池中(非 invalid)。
func isItemActive(itemID int) bool {
	return getItemState(itemID).Status == ItemStatusActive
}

// itemUseCount 返回 item 累计使用次数(least_used 用)。
func itemUseCount(itemID int) uint64 {
	return getItemState(itemID).UseCount
}

// MarkItemFail 记录 item 失败; 同类错误连续 itemFailThreshold 次 → invalid + 原因。
// 使用默认阈值; 需要按渠道覆盖时用 MarkItemFailWithThreshold。
func MarkItemFail(itemID int, reason string) {
	MarkItemFailWithThreshold(itemID, reason, itemFailThreshold)
}

// MarkItemFailWithThreshold 同 MarkItemFail, 但允许按渠道指定熔断阈值(>0), 0 用全局默认。
func MarkItemFailWithThreshold(itemID int, reason string, threshold int) {
	if threshold <= 0 {
		threshold = itemFailThreshold
	}
	st := getItemState(itemID)
	if reason == st.Reason {
		st.FailCount++
	} else {
		st.Reason = reason
		st.FailCount = 1
	}
	st.LastFailAt = time.Now()
	if st.FailCount >= threshold {
		st.Status = ItemStatusInvalid
	}
}

// MarkItemOk 记录 item 成功: 清空失败计数、恢复 active(成功一次即复活)并累计使用次数。
func MarkItemOk(itemID int) {
	st := getItemState(itemID)
	st.FailCount = 0
	st.Reason = ""
	st.Status = ItemStatusActive
	st.UseCount++
}

// ResetItem 重置 item 状态(手动启用/禁用时调用; 禁用清计数, 启用恢复 active)。
func ResetItem(itemID int) {
	itemStates.Delete(itemID)
}

// InvalidItemIDs 返回当前 invalid 且非手动禁用的 item id(供恢复任务探测)。
func InvalidItemIDs() []int {
	var out []int
	itemStates.Range(func(k, v any) bool {
		st := v.(*itemState)
		if st.Status == ItemStatusInvalid {
			out = append(out, k.(int))
		}
		return true
	})
	return out
}

// RecoverItem 手动恢复 item 为 active。
func RecoverItem(itemID int) {
	getItemState(itemID).Status = ItemStatusActive
}

// filterEligible 过滤可参与轮询的 item: enabled(DB 态) && active(内存态)。
func filterEligible(items []model.GroupItem) []model.GroupItem {
	out := make([]model.GroupItem, 0, len(items))
	for _, it := range items {
		if it.Enabled && isItemActive(it.ID) {
			out = append(out, it)
		}
	}
	return out
}
