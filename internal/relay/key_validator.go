package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/task"
)

func init() {
	// 多 key 失效自动恢复: 每 1 小时一轮, 逐个 key 探测, 每个测完随机停 10~30s
	task.Register("key_validator", time.Hour, false, validateInvalidKeys)
	// 分组模型失效自动恢复: 每 1 小时一轮, 直接复活(下次失败会重新计数)
	task.Register("item_validator", time.Hour, false, validateInvalidItems)
}

// validateInvalidItems 把所有 invalid 的分组 item 重置回 active。
// 不做探测: 分组 item 失败多为渠道级问题(key 层已探测恢复), item 复活后失败会重新计数。
func validateInvalidItems() {
	for _, itemID := range balancer.InvalidItemIDs() {
		balancer.RecoverItem(itemID)
	}
}

// validateInvalidKeys 探测所有 invalid 且非 disabled 的 key, 2xx 则恢复 active。
func validateInvalidKeys() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	channels, err := op.ChannelList(ctx)
	if err != nil {
		return
	}

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		invalid := invalidKeys(&ch)
		for _, st := range invalid {
			select {
			case <-ctx.Done():
				return
			default:
			}
			ok := probeKey(ctx, &ch, st.Key)
			if ok {
				RecoverKey(&ch, st.Key)
			}
			// 每个 key 测完随机停 10~30s 再测下一个(防上游限流)
			time.Sleep(time.Duration(10+rand.Intn(21)) * time.Second)
		}
	}
}

// probeKey 用渠道最近成功模型发最小探测请求, 2xx 视为可用。
func probeKey(ctx context.Context, ch *model.Channel, key string) bool {
	modelName := lastSuccessModel(ch.ID)
	if modelName == "" {
		// 无历史模型时跳过(避免探测模型本身 404 误判)
		return false
	}

	client, err := helper.ChannelHttpClient(ch)
	if err != nil {
		return false
	}

	path := "/chat/completions"
	body := map[string]any{
		"model":      modelName,
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 1,
	}
	if ch.Type == model.ChannelProviderMistral {
		// Mistral conversations 端点: inputs 格式, 无 max_tokens
		path = "/conversations"
		body = map[string]any{
			"model":  modelName,
			"inputs": []map[string]string{{"role": "user", "content": "hi"}},
		}
	}

	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(ch.BaseURL, "/")+path, strings.NewReader(string(raw)))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}
	// 401/403/429/5xx 都保持 invalid
	return false
}

// lastSuccessModel 查该渠道最近一次成功调用的模型(避免探测模型不存在误判)。
func lastSuccessModel(channelID int) string {
	var modelName string
	_ = op.RelayLogLastSuccessModel(channelID, &modelName)
	return modelName
}

var _ = fmt.Sprintf // keep fmt import if unused later
