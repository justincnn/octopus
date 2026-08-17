package model

type LLMPrice struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

type LLMInfo struct {
	Name string `json:"name" gorm:"primaryKey;not null"`
	LLMPrice
}

// ModelAlias 记录渠道模型名 → models.dev 规范模型名的价格映射。
// 用户在「未匹配模型」弹窗里对某个渠道模型名做了模糊匹配后, 持久化这条映射,
// 以后每次拉取 models.dev 价格都会用 canonical_id 的成本自动刷新 src_name 的价格,
// 实现"匹配后自动同步"。
type ModelAlias struct {
	SrcName     string `json:"src_name" gorm:"primaryKey;not null"` // 渠道里的模型名(小写)
	CanonicalID string `json:"canonical_id" gorm:"not null;index"`  // models.dev 目录里的规范模型名(小写)
}

type LLMChannel struct {
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
}

type GeminiModel struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

type GeminiModelList struct {
	Models        []GeminiModel `json:"models"`
	NextPageToken string        `json:"nextPageToken"`
}

type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type OpenAIModelList struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}
type AnthropicModel struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"created_at"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

type AnthropicModelList struct {
	Data    []AnthropicModel `json:"data"`
	FirstID string           `json:"first_id"`
	HasMore bool             `json:"has_more"`
	LastID  string           `json:"last_id"`
}
