package price

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/client"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/utils/log"
)

const llmPriceUrl = "https://models.dev/api.json"

// developerFamilies 定义研发商及其自研模型系列前缀(云平台托管的第三方模型不收录)。
var developerFamilies = map[string][]string{
	"openai":     {"gpt", "o"},
	"anthropic":  {"claude"},
	"google":     {"gemini", "gemma", "lyria", "veo"},
	"deepseek":   {"deepseek"},
	"xai":        {"grok"},
	"alibaba":    {"qwen", "qvq"},
	"zhipuai":    {"glm"},
	"minimax":    {"minimax"},
	"moonshotai": {"kimi"},
	"v0":         {"v0"},
}

var lastUpdateTime time.Time

// llmPriceFull 保存 models.dev 全量目录(所有 provider 的所有模型), 不裁剪。
// 它是「未匹配模型」模糊匹配的候选池; llmPrice 仍只保留自研文本系列(用于定时落库)。
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
	var body []byte
	httpClient, err := client.GetHTTPClientSystemProxy(false)
	if err == nil {
		req, requestErr := http.NewRequestWithContext(cctx, http.MethodGet, llmPriceUrl, nil)
		if requestErr != nil {
			return requestErr
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		resp, requestErr := httpClient.Do(req)
		if requestErr != nil {
			err = requestErr
		} else {
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("failed to fetch LLM info: %s", resp.Status)
			} else {
				body, err = io.ReadAll(resp.Body)
				if err != nil {
					err = fmt.Errorf("failed to read response body: %w", err)
				}
			}
			resp.Body.Close()
		}
	}
	// 直连失败时走系统代理重试(自部署机器直连 models.dev 可能被限)。
	if err != nil {
		log.Warnf("direct price fetch failed, retrying via system proxy: %v", err)
		httpClient, err = client.GetHTTPClientSystemProxy(true)
		if err != nil {
			return err
		}
		req, requestErr := http.NewRequestWithContext(cctx, http.MethodGet, llmPriceUrl, nil)
		if requestErr != nil {
			return requestErr
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		resp, requestErr := httpClient.Do(req)
		if requestErr != nil {
			return requestErr
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to fetch LLM info: %s", resp.Status)
		}
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}
	}
	var rawPrice map[string]struct {
		Models map[string]struct {
			ID         string `json:"id"`
			Family     string `json:"family"`
			Modalities struct {
				Output []string `json:"output"`
			} `json:"modalities"`
			Cost model.LLMPrice `json:"cost"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &rawPrice); err != nil {
		return fmt.Errorf("failed to parse LLM info: %w", err)
	}
	llmPriceLock.Lock()
	full := make(map[string]model.LLMPrice, 512)
	infos := make([]model.LLMInfo, 0, 512)
	for srcProvider, src := range rawPrice {
		familyPrefixes, isDeveloper := developerFamilies[srcProvider]
		for _, m := range src.Models {
			modelID := strings.ToLower(m.ID)
			if modelID == "" {
				continue
			}
			full[modelID] = m.Cost // 全量目录: 不裁剪, 供未匹配模型匹配候选
			if !isDeveloper {
				continue // 非自研研发商只进 full, 不进定时落库
			}
			// 仅保留文本输出的非嵌入自研模型, 排除云平台托管的第三方模型。
			modelFamily := strings.ToLower(m.Family)
			if !slices.Contains(m.Modalities.Output, "text") ||
				strings.Contains(modelID, "embed") || strings.Contains(modelFamily, "embed") {
				continue
			}
			isSelfDev := false
			for _, prefix := range familyPrefixes {
				if strings.HasPrefix(modelFamily, prefix) {
					isSelfDev = true
					break
				}
			}
			if !isSelfDev {
				continue
			}
			llmPrice[modelID] = m.Cost
			infos = append(infos, model.LLMInfo{Name: modelID, LLMPrice: m.Cost})
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
