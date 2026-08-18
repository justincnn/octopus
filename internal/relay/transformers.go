package relay

import (
	"fmt"

	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
	"github.com/looplj/axonhub/llm/transformer/anthropic"
	"github.com/looplj/axonhub/llm/transformer/gemini"
	"github.com/looplj/axonhub/llm/transformer/openai"
	"github.com/looplj/axonhub/llm/transformer/openai/responses"
)

func newInbound(format llm.APIFormat) transformer.Inbound {
	switch format {
	case llm.APIFormatOpenAIChatCompletion:
		return openai.NewInboundTransformer()
	case llm.APIFormatOpenAIResponse:
		return responses.NewInboundTransformer()
	case llm.APIFormatOpenAIEmbedding:
		return openai.NewEmbeddingInboundTransformer()
	case llm.APIFormatOpenAIImageGeneration:
		return openai.NewImageGenerationInboundTransformer()
	case llm.APIFormatOpenAIImageEdit:
		return openai.NewImageEditInboundTransformer()
	case llm.APIFormatOpenAIImageVariation:
		return openai.NewImageVariationInboundTransformer()
	case llm.APIFormatAnthropicMessage:
		return anthropic.NewInboundTransformer()
	default:
		return nil
	}
}

// NewOutbound 根据渠道提供方(ChannelProvider)选择上游适配器。扁平化后渠道只存
// ChannelProvider, 不再持有 llm.APIFormat, 因此这里统一按 provider 分发。
// (relay 内部与渠道模型测试共用)
func NewOutbound(provider dbmodel.ChannelProvider, request *llm.Request, baseURL, key string) (transformer.Outbound, error) {
	// 兼容旧值: 某些历史数据可能把 provider 写成 APIFormat 字符串
	switch provider {
	case dbmodel.ChannelProviderOpenAI, dbmodel.ChannelProviderOpenAIResponses:
		// OpenAI 兼容渠道走标准 /chat/completions 或 /responses。
		// openai 从 gpt-5 起拒绝 max_tokens(Unsupported parameter), 官方已统一 max_completion_tokens
		// (gpt-4o 等旧模型同样支持, 第三方兼容网关实测兼容) → 入站 max_tokens 统一映射。
		if request.MaxTokens != nil && request.MaxCompletionTokens == nil {
			request.MaxCompletionTokens = request.MaxTokens
			request.MaxTokens = nil
		}
		return openai.NewOutboundTransformer(baseURL, key)
	case dbmodel.ChannelProviderMistral:
		// Mistral 官方 /v1/conversations(Agent 格式) 支持 glm-5-2/zai-glm-5-2,
		// 走自定义 outbound(messages→inputs)。
		return NewMistralConversationOutbound(baseURL, key)
	case dbmodel.ChannelProviderAnthropic:
		return anthropic.NewOutboundTransformer(baseURL, key)
	case dbmodel.ChannelProviderGemini:
		return gemini.NewOutboundTransformer(baseURL, key)
	default:
		return nil, fmt.Errorf("channel provider %s not supported", provider)
	}
}