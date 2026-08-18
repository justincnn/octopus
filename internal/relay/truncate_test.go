package relay

import (
	"testing"

	"github.com/looplj/axonhub/llm"
)

func msg(role, content string) llm.Message {
	return llm.Message{Role: role, Content: llm.MessageContent{Content: &content}}
}

func TestTruncateMsgsKeepsHeadAndTail(t *testing.T) {
	big := ""
	for i := 0; i < 200; i++ { // 中文填充, ~800 token
		big += "这是一段用于填充的测试中文文本内容。"
	}
	msgs := []llm.Message{
		msg("system", "system prompt 很少"),
		msg("user", big), msg("user", big), msg("user", big),
		msg("user", big), msg("user", big),
		msg("user", "最后一条很短的最终指令，必须在结果里"),
	}
	out := truncateMsgs(msgs, 3000)

	if len(out) < 3 {
		t.Fatalf("expected >=3 msgs (system+tail+note), got %d", len(out))
	}
	if out[0].Role != "system" {
		t.Fatalf("expected first msg system, got %s", out[0].Role)
	}
	if total := countMsgTokens(out); total > 3000 {
		t.Fatalf("expected tokens <= 3000, got %d", total)
	}
	// 最后一条真实消息必须保留
	last := out[len(out)-2].Content
	if last.Content == nil || *last.Content != "最后一条很短的最终指令，必须在结果里" {
		t.Fatalf("expected last user msg preserved, got %v", out[len(out)-2].Content)
	}
}

func TestTruncateMsgsSkipsWhenWithinWindow(t *testing.T) {
	msgs := []llm.Message{msg("user", "短"), msg("user", "短二")}
	out := truncateMsgs(msgs, 100000)
	if len(out) != 2 {
		t.Fatalf("expected no change, got %d msgs", len(out))
	}
}

func TestTruncateMsgsDisabledWhenZero(t *testing.T) {
	big := ""
	for i := 0; i < 3000; i++ {
		big += "超长内容填充。"
	}
	msgs := []llm.Message{msg("user", big), msg("user", big)}
	if out := truncateMsgs(msgs, 0); len(out) != 2 {
		t.Fatalf("maxContextTokens=0 should disable truncation, got %d", len(out))
	}
}