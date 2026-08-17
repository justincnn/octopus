package price

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
)

// ---- 全量目录候选匹配 ----

// GetCanonicalPrice 在全量 models.dev 目录里查规范模型名的成本。
// 目录尚未拉取(首次启动未跑 UpdateLLMPrice)时, 回退到白名单价格表。
func GetCanonicalPrice(name string) (model.LLMPrice, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	llmPriceFullLock.RLock()
	p, ok := llmPriceFull[name]
	llmPriceFullLock.RUnlock()
	if ok {
		return p, nil
	}
	llmPriceLock.RLock()
	p, ok = llmPrice[name]
	llmPriceLock.RUnlock()
	if ok {
		return p, nil
	}
	return model.LLMPrice{}, fmt.Errorf("not in catalog")
}

// aliasExactMappings 内置常见"虚拟/别名模型名 → 规范模型名"的语义映射。
// 覆盖 models.dev 里没有、但实际用常见别名跑的模型。能命中就不再做前缀/包含猜测。
var aliasExactMappings = map[string]string{
	"gpt-oss-120b":           "grok-ossb-120b",
	"gpt-oss:120b":           "grok-ossb-120b",
	"grok-chat-fast":         "grok-4-fast",
	"deepseek-v4-flash":      "deepseek-chat",
	"deepseek-v4-pro":        "deepseek-reasoner",
	"glm-5.2":                "glm-4.6",
	"glm-5.1":                "glm-4.6",
	"glm-5-2":                "glm-4.6",
	"zai-glm-5-2":            "glm-4.6",
	"gpt-5-5":                "gpt-5",
	"gpt-5.5":                "gpt-5",
	"gpt-5.4-mini":           "gpt-5-mini",
	"gemini-3.6-flash":       "gemini-2.5-flash",
	"qwen3.8-max":            "qwen3-max",
	"qwen3.7-plus":           "qwen-plus",
	"kimi-k3":                "kimi-k2-thinking",
	"moonshotai/kimi-k3":     "kimi-k2-thinking",
	"coder-ds4-0731":         "deepseek-chat",
	"deepseek-v4-flash-0731": "deepseek-chat",
	"deepseek-v4-flash-free": "deepseek-chat",
	"mimo-v2.5-free":         "kimi-k2-thinking",
	"grok-4.5":               "grok-4",
	"grok-4.6":               "grok-4",
}

// stripDerive 把渠道模型名"剥壳"成候选: 去命名空间前缀、去版本/尺寸后缀、去 -free 等。
func deriveCandidates(name string) []string {
	name = strings.ToLower(strings.TrimSpace(name))
	out := []string{name}
	// 去命名空间前缀: xx/yyy → yyy, 全名保留在 out 里一起尝试
	base := name
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
		out = append(out, base)
	}
	// 去 :size 与 -size ("gpt-oss:120b" → "gpt-oss", "gpt-oss-120b" → "gpt-oss")
	cand := base
	if i := strings.Index(cand, ":"); i >= 0 {
		cand = cand[:i]
		out = append(out, cand)
	}
	cand = strings.TrimSuffix(strings.TrimSuffix(cand, "-120b"), "-120b:free")
	out = append(out, cand)
	// 去掉尾部日期/版本号 token (形如 -0731, -20240513, -latest)
	segs := strings.Split(cand, "-")
	for len(segs) > 1 {
		last := segs[len(segs)-1]
		if !isWeakSuffix(last) {
			break
		}
		segs = segs[:len(segs)-1]
	}
	if s := strings.Join(segs, "-"); s != cand {
		out = append(out, s)
	}
	// 去掉 -free
	if s := strings.TrimSuffix(base, "-free"); s != base {
		out = append(out, s)
	}
	return out
}

func isWeakSuffix(tok string) bool {
	return tok == "latest" || tok == "free" ||
		(len(tok) >= 4 && allDigits(tok[:4])) ||
		(len(tok) == 5 && tok[0] >= '0' && tok[0] <= '9' && allDigits(tok))
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// MatchCandidate 一个模糊匹配候选。
type MatchCandidate struct {
	CanonicalID string         `json:"canonical_id"`
	Price       model.LLMPrice `json:"price"`
	Reason      string         `json:"reason"` // exact / prefix_mapping / derive / contains
}

// BatchMatchResult 批量匹配返回: 一个未匹配模型名 + 它的候选列表。
type BatchMatchResult struct {
	Name       string           `json:"name"`
	Candidates []MatchCandidate `json:"candidates"`
}

// MatchCandidates 对渠道模型名返回模糊匹配候选(按优先级: 别名表 → 全量目录精配 → 剥壳派生 → 包含)。
// 返回空 = 无匹配。
func MatchCandidates(name string) []MatchCandidate {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil
	}
	// 1) 内置语义别名表
	if canon, ok := aliasExactMappings[name]; ok {
		if p, err := GetCanonicalPrice(canon); err == nil {
			return []MatchCandidate{{CanonicalID: canon, Price: p, Reason: "alias"}}
		}
	}
	llmPriceFullLock.RLock()
	full := llmPriceFull
	llmPriceFullLock.RUnlock()
	return candidatesFromMap(name, full)
}

// SearchModels 在全量 models.dev 目录里按关键词模糊搜索, 返回最多 20 条候选。
// 精配永远排最前(包含它的 id 必然更长), 其余按名字长度升序。只读不写库。
func SearchModels(q string) []MatchCandidate {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	llmPriceFullLock.RLock()
	defer llmPriceFullLock.RUnlock()
	out := make([]MatchCandidate, 0, 20)
	for id, p := range llmPriceFull {
		if strings.Contains(id, q) {
			out = append(out, MatchCandidate{CanonicalID: id, Price: p, Reason: "search"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].CanonicalID) < len(out[j].CanonicalID)
	})
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

func candidatesFromMap(name string, full map[string]model.LLMPrice) []MatchCandidate {
	// 精配
	if p, ok := full[name]; ok {
		return []MatchCandidate{{CanonicalID: name, Price: p, Reason: "exact"}}
	}
	seen := map[string]bool{}
	out := make([]MatchCandidate, 0, 4)
	// 逐个候选(含剥壳派生后的 base): 先查内置别名表, 再查目录精配
	for _, cand := range append(deriveCandidates(name), name) {
		if seen[cand] {
			continue
		}
		seen[cand] = true
		resolved := cand
		reason := "derive"
		if canon, ok := aliasExactMappings[cand]; ok {
			resolved = canon // 别名表把派生名映射成规范名
			if p, ok := full[resolved]; ok {
				if c := firstMatch(out, resolved); c == nil {
					out = append(out, MatchCandidate{CanonicalID: resolved, Price: p, Reason: "alias"})
				}
			}
		}
		if p, ok := full[resolved]; ok {
			if c := firstMatch(out, resolved); c == nil {
				out = append(out, MatchCandidate{CanonicalID: resolved, Price: p, Reason: reason})
			}
		}
	}
	// 包含匹配
	out = append(out, containsCandidates(name, full)...)
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func firstMatch(list []MatchCandidate, id string) *MatchCandidate {
	for i := range list {
		if list[i].CanonicalID == id {
			return &list[i]
		}
	}
	return nil
}

// containsCandidates 用 base 的核心片段在全量目录里做包含匹配, 返回最多 3 个。
func containsCandidates(name string, full map[string]model.LLMPrice) []MatchCandidate {
	base := name
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	key := shortestStableKey(strings.Split(base, "-"))
	contains := make([]MatchCandidate, 0, 3)
	for id, p := range full {
		if key != "" && strings.Contains(id, key) && id != name {
			contains = append(contains, MatchCandidate{CanonicalID: id, Price: p, Reason: "contains"})
			if len(contains) >= 3 {
				break
			}
		}
	}
	sort.SliceStable(contains, func(i, j int) bool {
		return len(contains[i].CanonicalID) < len(contains[j].CanonicalID)
	})
	return contains
}

// shortestStableKey 取模型名中较稳定的核心片段用于包含匹配(取第一个>=4位非纯数字的片段)。
func shortestStableKey(parts []string) string {
	for _, p := range parts {
		if len(p) >= 4 && !allDigits(p) {
			return p
		}
	}
	return ""
}

// syncAliases 自动同步所有已记录别名: 用 canonical 在全量目录的成本刷新 src 的价格。
// 在 UpdateLLMPrice 末尾调用, 保证匹配过的模型每次拉价后保持最新。
func syncAliases(ctx context.Context) error {
	aliases, err := op.AliasListAll(ctx)
	if err != nil {
		return err
	}
	if len(aliases) == 0 {
		return nil
	}
	infos := make([]model.LLMInfo, 0, len(aliases))
	llmPriceFullLock.RLock()
	for src, canon := range aliases {
		if p, ok := llmPriceFull[canon]; ok {
			infos = append(infos, model.LLMInfo{Name: src, LLMPrice: p})
		}
	}
	llmPriceFullLock.RUnlock()
	if len(infos) == 0 {
		return nil
	}
	return op.LLMUpsertAll(infos, ctx)
}

// SetAlias 把渠道模型名 src 匹配到规范模型名 canonical: 保存别名 + 立即写入 src 的价格。
// canonical 必须在目录/别名表里能找到, 否则报错(防存死映射)。
func SetAlias(src, canonical string, ctx context.Context) error {
	p, err := GetCanonicalPrice(canonical)
	if err != nil {
		return fmt.Errorf("canonical model %q not in catalog: %w", canonical, err)
	}
	if err := op.AliasSave(src, canonical, ctx); err != nil {
		return err
	}
	infos := []model.LLMInfo{{Name: strings.ToLower(strings.TrimSpace(src)), LLMPrice: p}}
	return op.LLMUpsertAll(infos, ctx)
}

// UnmatchedModels 返回所有渠道里、当前没有价格(成本全 0)且未设置别名的模型名(去重小写)。
func UnmatchedModels(ctx context.Context) ([]string, error) {
	channels, err := op.ChannelList(ctx)
	if err != nil {
		return nil, err
	}
	aliases, err := op.AliasListAll(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var out []string
	// 有价格的模型名(从 llm_infos 成本>0 收集, 等价于"已匹配")
	priced, err := op.LLMList(ctx)
	if err != nil {
		return nil, err
	}
	pricedSet := make(map[string]model.LLMPrice, len(priced))
	for _, m := range priced {
		pricedSet[m.Name] = m.LLMPrice
	}
	for _, ch := range channels {
		for _, m := range splitModels(ch.Model) {
			m = strings.ToLower(m)
			if m == "" || seen[m] {
				continue
			}
			seen[m] = true
			p, ok := pricedSet[m]
			if ok && (p.Input != 0 || p.Output != 0 || p.CacheRead != 0) {
				continue // 已有价格
			}
			if _, hasAlias := aliases[m]; hasAlias {
				continue // 已设别名(同步会补价)
			}
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out, nil
}

func splitModels(s string) []string {
	var out []string
	for _, m := range strings.Split(s, ",") {
		if t := strings.TrimSpace(m); t != "" {
			out = append(out, t)
		}
	}
	return out
}
