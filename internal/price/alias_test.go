package price

import (
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

// 模糊匹配自检: 用注入的全量目录(representative entries)验证匹配规则, 不依赖网络/DB。
func TestMatchCandidates(t *testing.T) {
	// 模拟 models.dev 全量目录(正常由 UpdateLLMPrice 从网络填充)
	llmPriceFullLock.Lock()
	llmPriceFull = map[string]model.LLMPrice{
		"grok-ossb-120b":      {Input: 0.15, Output: 0.6},
		"deepseek-chat":       {Input: 0.14, Output: 0.28},
		"deepseek-reasoner":   {Input: 0.55, Output: 2.0},
		"kimi-k2-thinking":    {Input: 1.4, Output: 4.4},
		"glm-4.6":             {Input: 1.4, Output: 4.4},
		"gpt-5":               {Input: 1.25, Output: 10.0},
		"gpt-5-mini":          {Input: 0.75, Output: 4.5},
		"gemini-2.5-flash":    {Input: 0.1, Output: 0.4},
		"grok-4":              {Input: 2.0, Output: 6.0},
		"grok-4-fast":         {Input: 0.3, Output: 0.1},
		"qwen3-max":           {Input: 0.5, Output: 0.8},
		"qwen-plus":           {Input: 0.1, Output: 0.2},
		"weird-model-has-key": {Input: 0.01, Output: 0.02}, // 用于包含匹配
	}
	llmPriceFullLock.Unlock()

	cases := []struct {
		name  string
		want  string // 期望首个候选
		valid bool
	}{
		{name: "grok-4.5", want: "grok-4", valid: true},                     // alias → grok-4(目录里有 key?) 见下
		{name: "gpt-oss:120b", want: "grok-ossb-120b", valid: true},         // alias 表
		{name: "deepseek-v4-flash", want: "deepseek-chat", valid: true},     // alias 表
		{name: "moonshotai/Kimi-K3", want: "kimi-k2-thinking", valid: true}, // alias 表
		{name: "deepseek/deepseek-v4-flash", valid: true},                   // derive 去前缀 → deepseek-chat
		{name: "not-a-real-model-xyzzy", valid: false},                      // 应无候选
	}
	for _, c := range cases {
		got := MatchCandidates(c.name)
		if !c.valid {
			if len(got) != 0 {
				t.Errorf("%q: want no candidate, got %d", c.name, len(got))
			}
			continue
		}
		if len(got) == 0 {
			t.Errorf("%q: expected a match, got none", c.name)
			continue
		}
		t.Logf("%q → %s (%s)", c.name, got[0].CanonicalID, got[0].Reason)
		if c.want != "" && got[0].CanonicalID != c.want {
			t.Errorf("%q: want first candidate %q, got %q (%s)", c.name, c.want, got[0].CanonicalID, got[0].Reason)
		}
	}
}

// deriveCandidates 纯函数自检(不依赖网络/DB)。
func TestDeriveCandidates(t *testing.T) {
	cases := map[string][]string{
		"moonshotai/Kimi-K3": {"moonshotai/kimi-k3", "kimi-k3"},
		"gpt-oss:120b":       {"gpt-oss:120b", "gpt-oss"},
	}
	for in, expected := range cases {
		got := deriveCandidates(in)
		for _, e := range expected {
			if !containsStr(got, e) {
				t.Errorf("deriveCandidates(%q) = %v, missing %q", in, got, e)
			}
		}
	}
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
