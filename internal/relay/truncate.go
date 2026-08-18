package relay

import (
	"github.com/looplj/axonhub/llm"
	"github.com/sirupsen/logrus"
)

// contentTokens: 粗略估算一条消息内容的 token 数。不做精确 tokenizer:
// 中文按 1 token≈1 字、非中文按 1 token≈3-4 字符, 内部后台够用。
// ponytail: 粗估折线; 若要精确官方窗口判断需集成 tiktoken, 当前无此必要。
func contentTokens(c llm.MessageContent) int {
	if c.Content != nil {
		return approxLen(*c.Content)
	}
	n := 0
	for _, p := range c.MultipleContent {
		if p.Text != nil {
			n += approxLen(*p.Text)
		}
	}
	return n
}

func approxLen(s string) int {
	if len(s) == 0 {
		return 0
	}
	var cjk, other int
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF { // CJK 常用区
			cjk++
		} else {
			other++
		}
	}
	return cjk + (other+3)/4
}

func countMsgTokens(msgs []llm.Message) int {
	n := 0
	for _, m := range msgs {
		n += contentTokens(m.Content)
	}
	return n
}

func contentNote(s string) llm.MessageContent {
	return llm.MessageContent{Content: &s}
}

// truncateContextIfNeeded 在转发上游前对超长上下文做自动裁剪:
// 保留 system + 最晚的若干条消息, 裁掉中段最老轮次, 并把 model 的窗口当作硬上限。
func (ra *relayAttempt) truncateContextIfNeeded(maxContextTokens int) {
	if ra.internalRequest == nil {
		return
	}
	ra.internalRequest.Messages = truncateMsgs(ra.internalRequest.Messages, maxContextTokens)
}

// truncateMsgs 纯函数: 超窗时裁剪, maxContextTokens<=0 或未超窗则原样返回。
func truncateMsgs(msgs []llm.Message, maxContextTokens int) []llm.Message {
	if maxContextTokens <= 0 || len(msgs) <= 1 {
		return msgs
	}
	limit := maxContextTokens * 9 / 10 // 给输出/reasoning 留 10%
	total := countMsgTokens(msgs)
	if total <= limit {
		return msgs
	}

	// 保留: index0(通常是 system) 和最后一条。中间尽量保留最近的, 从最老开始丢。
	keep := []int{0}
	acc := contentTokens(msgs[0].Content)
	for i := 1; i < len(msgs)-1; i++ {
		cost := contentTokens(msgs[i].Content)
		if acc+cost <= limit {
			acc += cost
			keep = append(keep, i)
		}
		// 超出则不保留更早中间轮(keep 只含最近已加入的)
	}
	keep = append(keep, len(msgs)-1) // 始终保留最后一条

	out := make([]llm.Message, 0, len(keep)+1)
	for _, i := range keep {
		out = append(out, msgs[i])
	}
	out = append(out, llm.Message{
		Role: "user",
		Content: contentNote("[system 自动裁剪] 因超出模型上下文窗口，较早的对话轮次已被截断。请基于最新消息继续。"),
	})

	logrus.Infof("relay: context truncated %d -> ~%d tokens (channel window %d)",
		total, countMsgTokens(out), maxContextTokens)
	return out
}