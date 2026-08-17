package balancer

import (
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

// item 状态机自检: 失败3次踢出、成功恢复+计数、filterEligible 过滤。
func TestItemPool(t *testing.T) {
	// 失败 3 次同类错误 → invalid
	MarkItemFail(1, ItemReasonRateLimited)
	MarkItemFail(1, ItemReasonRateLimited)
	if !isItemActive(1) {
		t.Fatal("want still active after 2 fails (threshold 3)")
	}
	MarkItemFail(1, ItemReasonRateLimited)
	if isItemActive(1) {
		t.Fatal("want inactive after 3 fails")
	}
	// 换错误类型重新计数
	MarkItemFail(1, ItemReasonUpstreamError)
	MarkItemFail(1, ItemReasonUpstreamError)
	if isItemActive(1) {
		t.Fatal("want still inactive (new reason counts from 1, needs 3)")
	}
	// 成功恢复 + 计数
	MarkItemOk(1)
	if !isItemActive(1) {
		t.Fatal("want active after ok")
	}
	MarkItemOk(1)
	if itemUseCount(1) != 2 {
		t.Fatalf("use count = %d, want 2", itemUseCount(1))
	}
	// ResetItem 清状态
	ResetItem(1)
	if !isItemActive(1) {
		t.Fatal("want active after reset")
	}
	if itemUseCount(1) != 0 {
		t.Fatalf("use count after reset = %d, want 0", itemUseCount(1))
	}
}

// filterEligible 自检: enabled + active 才进候选池。
func TestFilterEligible(t *testing.T) {
	ResetItem(10)
	ResetItem(11)
	MarkItemFail(11, ItemReasonUpstreamError)
	MarkItemFail(11, ItemReasonUpstreamError)
	MarkItemFail(11, ItemReasonUpstreamError) // 11 → invalid

	items := []model.GroupItem{
		{ID: 10, Enabled: true},  // active + enabled → 入选
		{ID: 11, Enabled: true},  // invalid → 出局
		{ID: 12, Enabled: false}, // 手动禁用 → 出局
		{ID: 13, Enabled: true},  // 从未使用 → active → 入选
	}
	got := filterEligible(items)
	if len(got) != 2 {
		t.Fatalf("want 2 eligible, got %d: %+v", len(got), got)
	}
	if got[0].ID != 10 || got[1].ID != 13 {
		t.Fatalf("unexpected selection: %+v", got)
	}
	ResetItem(10)
	ResetItem(11)
	ResetItem(12)
	ResetItem(13)
}
