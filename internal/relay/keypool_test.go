package relay

import (
	"net/http"
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

// RecordKeyProbe 自检: 2xx 恢复、401/403 直接踢出、429/5xx 记失败计数。
func TestRecordKeyProbe(t *testing.T) {
	ch := &model.Channel{ID: 999, Keys: []string{"k1", "k2", "k3"}}

	// 初始全 active
	// 401 → 直接 invalid
	RecordKeyProbe(ch, "k1", http.StatusForbidden)
	st := keyStateOf(ch, "k1")
	if st.Status != KeyStatusInvalid || st.Reason != KeyReasonInvalid {
		t.Fatalf("k1 want invalid/invalid, got %s/%s", st.Status, st.Reason)
	}

	// 429 一次 → 仍 active(连 3 次才踢)
	RecordKeyProbe(ch, "k2", http.StatusTooManyRequests)
	st = keyStateOf(ch, "k2")
	if st.Status != KeyStatusActive || st.FailCount != 1 {
		t.Fatalf("k2 want active/fail=1, got %s/%d", st.Status, st.FailCount)
	}

	// 5xx 同类错误连 3 次 → 踢出
	RecordKeyProbe(ch, "k2", http.StatusBadGateway)
	RecordKeyProbe(ch, "k2", http.StatusBadGateway)
	RecordKeyProbe(ch, "k2", http.StatusBadGateway)
	st = keyStateOf(ch, "k2")
	if st.Status != KeyStatusInvalid || st.Reason != KeyReasonUpstreamError {
		t.Fatalf("k2 want invalid/upstream_error, got %s/%s", st.Status, st.Reason)
	}

	// 2xx → 恢复 active 清计数
	RecordKeyProbe(ch, "k2", http.StatusOK)
	st = keyStateOf(ch, "k2")
	if st.Status != KeyStatusActive || st.FailCount != 0 {
		t.Fatalf("k2 want active/0, got %s/%d", st.Status, st.FailCount)
	}
}

func keyStateOf(ch *model.Channel, key string) *keyState {
	ck := loadKeyStates(ch)
	for _, st := range ck.states {
		if st.Key == key {
			return st
		}
	}
	return nil
}
