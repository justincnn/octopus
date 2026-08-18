package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay/balancer"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/gin-gonic/gin"
)

// RerankHandler 中转 Cohere 格式 rerank 请求(POST /v1/rerank)。
// 透传模式: 解析 model 走分组路由 + key 池, body 原样转发到上游 {base}/rerank, 响应原样返回。
// 401/403 → 换 key 重试; 其他 4xx → 上游业务错误原样返回; 5xx/网络 → 换渠道重试。
func RerankHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, "failed to read body")
			return
		}
		var req struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
			resp.Error(c, http.StatusBadRequest, "model is required")
			return
		}

		group, err := op.GroupGetEnabledMap(req.Model, ctx)
		if err != nil {
			resp.Error(c, http.StatusNotFound, "model not found")
			return
		}
		apiKeyID := c.GetInt("api_key_id")
		iter := balancer.NewIterator(group, apiKeyID, req.Model)
		if iter.Len() == 0 {
			resp.Error(c, http.StatusServiceUnavailable, "no available channel")
			return
		}

		var lastErr error
		for iter.Next() {
			item := iter.Item()
			channel, err := op.ChannelGet(item.ChannelID, ctx)
			if err != nil || !channel.Enabled {
				iter.Skip(item.ChannelID, item.ChannelID, "rerank", "channel unavailable")
				continue
			}
			key := channel.Key
			if len(channel.Keys) > 0 || channel.Key != "" {
				key, _ = nextKey(channel, iter.StickyKeyIdx())
			}
			if key == "" {
				iter.Skip(item.ChannelID, item.ChannelID, channel.Name, "no available key")
				continue
			}
			if !tryAcquireChannel(channel.ID, channel.MaxConcurrency) {
				iter.Skip(item.ChannelID, item.ChannelID, channel.Name, "channel concurrency limit reached")
				continue
			}

			upstreamURL := strings.TrimRight(channel.BaseURL, "/") + "/rerank"
			upReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
			if err != nil {
				releaseChannel(channel.ID)
				lastErr = err
				continue
			}
			upReq.Header.Set("Content-Type", "application/json")
			upReq.Header.Set("Authorization", "Bearer "+key)

			client, err := helper.ChannelHttpClient(channel)
			if err != nil {
				releaseChannel(channel.ID)
				lastErr = err
				continue
			}
			upResp, err := client.Do(upReq)
			if err != nil {
				releaseChannel(channel.ID)
				RecordKeyProbe(channel, key, 0)
				lastErr = err
				continue
			}

			code := upResp.StatusCode
			switch {
			case code >= 200 && code < 300:
				// 成功: 原样透传响应
				RecordKeyProbe(channel, key, code)
				c.Status(code)
				c.Writer.Header().Set("Content-Type", upResp.Header.Get("Content-Type"))
				io.Copy(c.Writer, upResp.Body)
				upResp.Body.Close()
				releaseChannel(channel.ID)
				return
			case code == http.StatusUnauthorized || code == http.StatusForbidden:
				// key 失效: 回写状态机, 换下一个 key/渠道重试
				RecordKeyProbe(channel, key, code)
				iter.Skip(item.ChannelID, item.ChannelID, channel.Name, "auth failed")
			default:
				// 其他 4xx/5xx: 上游业务错误或临时故障; 5xx 回写失败计数并重试
				RecordKeyProbe(channel, key, code)
				if code < 500 {
					// 业务错误(模型不支持/参数错误)原样返回, 不重试
					c.Status(code)
					c.Writer.Header().Set("Content-Type", upResp.Header.Get("Content-Type"))
					io.Copy(c.Writer, upResp.Body)
					upResp.Body.Close()
					releaseChannel(channel.ID)
					return
				}
				iter.Skip(item.ChannelID, item.ChannelID, channel.Name, "upstream error")
			}
			upResp.Body.Close()
			releaseChannel(channel.ID)
			lastErr = err
		}

		if lastErr == nil {
			lastErr = errors.New("all channels failed")
		}
		resp.Error(c, http.StatusBadGateway, lastErr.Error())
	}
}
