package relay

import (
	"fmt"

	dbmodel "github.com/bestruirui/octopus/internal/model"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer"
	"github.com/looplj/axonhub/llm/transformer/anthropic"
	"github.com/looplj/axonhub/llm/transformer/doubao"
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

// newOutbound 根据渠道提供方(ChannelProvider)选择上游适配器。扁平化后渠道只存
// ChannelProvider, 不再持有 llm.APIFormat, 因此这里统一按 provider 分发。
func newOutbound(provider dbmodel.ChannelProvider, request *llm.Request, baseURL, key string) (transformer.Outbound, error) {
	// 兼容旧值: 某些历史数据可能把 provider 写成 APIFormat 字符串
	switch provider {
	case dbmodel.ChannelProviderOpenAI, dbmodel.ChannelProviderOpenAIResponses, dbmodel.ChannelProviderMistral:
		// Mistral 官方 API (https://api.mistral.ai/v1) 原生就是 OpenAI 兼容的
		// /chat/completions 接口, 因此复用 OpenAI outbound, baseURL 填官方地址即可。
		return openai.NewOutboundTransformer(baseURL, key)
	case dbmodel.ChannelProviderAnthropic:
		return anthropic.NewOutboundTransformer(baseURL, key)
	case dbmodel.ChannelProviderGemini:
		return gemini.NewOutboundTransformer(baseURL, key)
	case dbmodel.ChannelProviderVolcengine:
		return doubao.NewOutboundTransformer(baseURL, key)
	default:
		return nil, fmt.Errorf("channel provider %s not supported", provider)
	}
}