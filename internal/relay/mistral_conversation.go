package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/streams"
	"github.com/looplj/axonhub/llm/transformer"
	"github.com/looplj/axonhub/llm/transformer/openai"
)

// mistralConversationOutbound 适配 Mistral 官方 /v1/conversations(Agent 格式)。
// 与 OpenAI /chat/completions 的差异:
//   - 请求: messages → inputs; 响应: choices → outputs[0].content
//   - 流式: event: message.output.delta + data:{"type":"message.output.delta","content":"..."}
//   - 结束事件: conversation.response.done
//
// 内嵌 OpenAI outbound 复用 TransformResponse/Stream 的 OpenAI 格式转换,
// 只重写 TransformRequest 做 messages→inputs 转换, 响应侧由 openai 适配器
// 处理不了 outputs 结构, 所以 TransformResponse/Stream 也要重写。
type mistralConversationOutbound struct {
	openaiOutbound transformer.Outbound
	baseURL        string
	apiKey         string
}

func NewMistralConversationOutbound(baseURL, apiKey string) (transformer.Outbound, error) {
	// 复用 OpenAI outbound 构造完整请求(headers/auth/错误处理)
	o, err := openai.NewOutboundTransformer(baseURL, apiKey)
	if err != nil {
		return nil, err
	}
	return &mistralConversationOutbound{
		openaiOutbound: o,
		baseURL:        baseURL,
		apiKey:         apiKey,
	}, nil
}

func (m *mistralConversationOutbound) APIFormat() llm.APIFormat {
	return llm.APIFormatOpenAIChatCompletion
}

// TransformRequest 把统一请求转成 Mistral conversations 格式:
// {"model":..., "inputs":[{"role":..., "content":...}], "stream":...}
func (m *mistralConversationOutbound) TransformRequest(ctx context.Context, llmReq *llm.Request) (*httpclient.Request, error) {
	if llmReq == nil {
		return nil, fmt.Errorf("llm request is nil")
	}
	if llmReq.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	inputs := make([]map[string]any, 0, len(llmReq.Messages))
	for _, msg := range llmReq.Messages {
		content := msg.Content
		inputs = append(inputs, map[string]any{
			"role":    msg.Role,
			"content": content,
		})
	}

	body := map[string]any{
		"model":  llmReq.Model,
		"inputs": inputs,
	}
	stream := false
	if llmReq.Stream != nil {
		stream = *llmReq.Stream
	}
	body["stream"] = stream
	// ⚠️ 官方 /v1/conversations 禁止 max_tokens/temperature(extra_forbidden 422), 只发 model/inputs/stream

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mistral conversation request: %w", err)
	}

	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	if stream {
		headers.Set("Accept", "text/event-stream")
	} else {
		headers.Set("Accept", "application/json")
	}

	return &httpclient.Request{
		Method:    http.MethodPost,
		URL:       m.baseURL + "/conversations",
		Headers:   headers,
		Body:      raw,
		Auth:      &httpclient.AuthConfig{Type: "bearer", APIKey: m.apiKey},
		APIFormat: string(llm.APIFormatOpenAIChatCompletion),
	}, nil
}

// TransformResponse 处理非流式响应: outputs[0].content → OpenAI choices。
func (m *mistralConversationOutbound) TransformResponse(ctx context.Context, httpResp *httpclient.Response) (*llm.Response, error) {
	if httpResp == nil {
		return nil, fmt.Errorf("http response is nil")
	}
	if httpResp.StatusCode >= 400 {
		return m.openaiOutbound.TransformResponse(ctx, httpResp)
	}
	var r struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Outputs []struct {
			Content string `json:"content"`
			Role    string `json:"role"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(httpResp.Body, &r); err != nil {
		return nil, fmt.Errorf("failed to unmarshal mistral conversation response: %w", err)
	}
	text := ""
	role := "assistant"
	if len(r.Outputs) > 0 {
		text = r.Outputs[0].Content
		if r.Outputs[0].Role != "" {
			role = r.Outputs[0].Role
		}
	}
	finish := "stop"
	return &llm.Response{
		ID:      r.ID,
		Object:  "chat.completion",
		Model:   r.Model,
		Choices: []llm.Choice{{Index: 0, Message: &llm.Message{Role: role, Content: llm.MessageContent{Content: &text}}, FinishReason: &finish}},
	}, nil
}

// TransformStream 处理流式响应: event: message.output.delta → OpenAI delta chunk。
func (m *mistralConversationOutbound) TransformStream(ctx context.Context, req *httpclient.Request, stream streams.Stream[*httpclient.StreamEvent]) (streams.Stream[*llm.Response], error) {
	return streams.MapErr(stream, func(event *httpclient.StreamEvent) (*llm.Response, error) {
		if event == nil || len(event.Data) == 0 {
			return nil, nil
		}
		var d struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return nil, nil // 非 JSON 帧(如 [DONE])跳过
		}
		// 结束事件: conversation.response.done / completed / failed
		if d.Type == "conversation.response.done" || d.Type == "conversation.response.completed" || d.Type == "conversation.response.failed" {
			return llm.DoneResponse, nil
		}
		// 增量帧: message.output.delta
		if d.Type == "message.output.delta" {
			return &llm.Response{
				Object: "chat.completion.chunk",
				Choices: []llm.Choice{{
					Index: 0,
					Delta: &llm.Message{Role: "assistant", Content: llm.MessageContent{Content: &d.Content}},
				}},
			}, nil
		}
		return nil, nil // message_start 等无关帧
	}), nil
}

// TransformError 复用 OpenAI 错误转换。
func (m *mistralConversationOutbound) TransformError(ctx context.Context, err *httpclient.Error) *llm.ResponseError {
	return m.openaiOutbound.TransformError(ctx, err)
}

// AggregateStreamChunks 复用 OpenAI 聚合(下游是 OpenAI 格式)。
func (m *mistralConversationOutbound) AggregateStreamChunks(ctx context.Context, req *httpclient.Request, chunks []*httpclient.StreamEvent) ([]byte, llm.ResponseMeta, error) {
	return m.openaiOutbound.AggregateStreamChunks(ctx, req, chunks)
}
