package price

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/client"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/utils/log"
)

const llmPriceUrl = "https://models.dev/api.json"

var Provider = []string{
	"openai",     // GPT 系列
	"anthropic",  // Claude 系列
	"google",     // Gemini 系列
	"deepseek",   // DeepSeek 系列
	"xai",        // Grok 系列
	"alibaba",    // Qwen 系列
	"zhipuai",    // GLM 系列
	"minimax",    // MiniMax 系列
	"moonshotai", // Kimi/Moonshot
	"v0",         // v0 系列
}

var lastUpdateTime time.Time

// llmPriceFull 保存 models.dev 全量目录(所有 provider 的所有模型), 不裁剪。
// 它是「未匹配模型」模糊匹配的候选池; llmPrice 仍只保留 Provider 白名单(用于定时落库)。
var (
	llmPriceFullLock sync.RWMutex
	llmPriceFull     = make(map[string]model.LLMPrice)
)

func UpdateLLMPrice(ctx context.Context) error {
	log.Debugf("update LLM price task started")
	startTime := time.Now()
	defer func() {
		log.Debugf("update LLM price task finished, update time: %s", time.Since(startTime))
	}()
	// 定时任务 ctx 是 Background() 无限期, 加超时防 models.dev 挂起卡死后台。
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client, err := client.GetHTTPClientSystemProxy(false)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, llmPriceUrl, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch LLM info: %s", resp.Status)
	}
	var rawPrice map[string]struct {
		Models map[string]struct {
			ID   string         `json:"id"`
			Cost model.LLMPrice `json:"cost"`
		} `json:"models"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	if err := json.Unmarshal(body, &rawPrice); err != nil {
		return fmt.Errorf("failed to parse LLM info: %w", err)
	}
	llmPriceLock.Lock()
	full := make(map[string]model.LLMPrice, 512)
	infos := make([]model.LLMInfo, 0, 512)
	providerAllowed := make(map[string]struct{}, len(Provider))
	for _, p := range Provider {
		providerAllowed[p] = struct{}{}
	}
	for srcProvider, src := range rawPrice {
		for _, m := range src.Models {
			m.ID = strings.ToLower(m.ID)
			if m.ID == "" {
				continue
			}
			full[m.ID] = m.Cost // 全量目录
			if _, ok := providerAllowed[srcProvider]; !ok {
				continue // 非白名单 provider 只进 full, 不进定时落库
			}
			llmPrice[m.ID] = m.Cost
			infos = append(infos, model.LLMInfo{Name: m.ID, LLMPrice: m.Cost})
		}
	}
	llmPriceLock.Unlock()
	llmPriceFullLock.Lock()
	llmPriceFull = full
	llmPriceFullLock.Unlock()
	lastUpdateTime = time.Now()

	// 新增: 落库持久化, 让设置页列表(读 DB)显示最新价格且重启不丢。
	if err := op.LLMUpsertAll(infos, cctx); err != nil {
		log.Warnf("failed to upsert llm price to db: %v", err)
	}
	// 新增: 自动同步已记录的模型别名(src→canonical), 让匹配过的模型价格保持最新。
	if err := syncAliases(cctx); err != nil {
		log.Warnf("failed to sync model aliases: %v", err)
	}
	return nil
}

func GetLastUpdateTime() time.Time {
	return lastUpdateTime
}

func GetLLMPrice(modelName string) *model.LLMPrice {
	modelName = strings.ToLower(modelName)
	price, err := op.LLMGet(modelName)
	if err == nil {
		return &price
	}
	llmPriceLock.RLock()
	defer llmPriceLock.RUnlock()
	price, ok := llmPrice[modelName]
	if !ok {
		return nil
	}
	return &price
}
