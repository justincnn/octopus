package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/relay"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/gin-gonic/gin"
	"github.com/looplj/axonhub/llm"
)

// modelTestRequest 模型可用性测试请求: 渠道配置(与 fetch-model 同构) + 待测模型列表。
type modelTestRequest struct {
	model.Channel
	ModelNames []string `json:"model_names" binding:"required"`
	MaxTokens  int      `json:"max_tokens"`
}

// modelTestResult 单个模型的测试结果。
type modelTestResult struct {
	ModelName string `json:"model_name"`
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	Content   string `json:"content,omitempty"`
	Error     string `json:"error,omitempty"`
	KeyUsed   string `json:"key_used,omitempty"` // 测通的 key(掩码)
}

// keyProbeStatus key 池探测状态: 只标注被测过的 key, 未测 = unknown。
type keyProbeStatus struct {
	Key    string `json:"key"` // 掩码
	Status string `json:"status"` // ok / failed / unknown
	Error  string `json:"error,omitempty"`
}

func init() {
	router.NewGroupRouter("/api/v1/channel").
		Use(middleware.Auth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/test-model", http.MethodPost).
				Handle(testChannelModels),
		)
}

// testChannelModels 批量测试渠道已选模型可用性:
// 每个模型按 key 序(主key→keys池)短路测试——第一个 2xx 即成功; 探测结果回写 key 状态机。
func testChannelModels(c *gin.Context) {
	var req modelTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	if len(req.ModelNames) == 0 {
		resp.Error(c, http.StatusBadRequest, "model_names is required")
		return
	}
	// 前端传打码/空 key 时, 用渠道 ID 从缓存补明文(主 key 空则取 keys 池第一个)
	fillPlainKey(c.Request.Context(), &req.Channel)
	// 前端 body 不带 keys 池: 从缓存补齐供 key 短路切换
	fillChannelKeys(c.Request.Context(), &req.Channel)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 5
	}

	keys := buildChannelKeys(&req.Channel)
	keyProbes := make(map[string]*keyProbeStatus, len(keys))
	keyOrder := make([]string, 0, len(keys))
	for _, k := range keys {
		if _, ok := keyProbes[k]; ok {
			continue
		}
		keyProbes[k] = &keyProbeStatus{Key: maskKey(k), Status: "unknown"}
		keyOrder = append(keyOrder, k)
	}

	ctx := c.Request.Context()
	results := make([]modelTestResult, len(req.ModelNames))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // 4 并发, 防上游限流

	for i, name := range req.ModelNames {
		wg.Add(1)
		go func(idx int, modelName string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = probeModel(ctx, req.Channel, keyOrder, modelName, maxTokens, keyProbes)
		}(i, name)
	}
	wg.Wait()

	// key 状态按原顺序输出
	keyStatus := make([]keyProbeStatus, 0, len(keyOrder))
	for _, k := range keyOrder {
		keyStatus = append(keyStatus, *keyProbes[k])
	}

	resp.Success(c, gin.H{"results": results, "key_status": keyStatus})
}

// buildChannelKeys 组装测试用 key 列表: 主 key + keys 池, 去重保序。
func buildChannelKeys(ch *model.Channel) []string {
	seen := make(map[string]bool)
	var out []string
	if ch.Key != "" && !seen[ch.Key] {
		out = append(out, ch.Key)
		seen[ch.Key] = true
	}
	for _, k := range ch.Keys {
		if !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	return out
}

// probeModel 对单个模型按 key 序短路测试: 第一个 2xx 即成功(key_used 标注);
// 全部失败返回最后一次错误; 每个被测 key 回写状态机并更新 keyProbes。
func probeModel(ctx context.Context, ch model.Channel, keys []string, modelName string, maxTokens int, keyProbes map[string]*keyProbeStatus) modelTestResult {
	var last modelTestResult
	for _, k := range keys {
		r, code := probeModelWithKey(ctx, ch, k, modelName, maxTokens)
		relay.RecordKeyProbe(&ch, k, code)
		if p, ok := keyProbes[k]; ok {
			if r.OK {
				p.Status = "ok"
			} else {
				p.Status = "failed"
				p.Error = r.Error
			}
		}
		if r.OK {
			r.KeyUsed = maskKey(k)
			return r
		}
		last = r
	}
	return last
}

// probeModelWithKey 用单个 key 对模型发最小对话请求, 返回结果 + 上游状态码。
func probeModelWithKey(ctx context.Context, ch model.Channel, key, modelName string, maxTokens int) (modelTestResult, int) {
	mt := int64(maxTokens)
	prompt := "回复:ok"
	llmReq := &llm.Request{
		Model:     modelName,
		Messages:  []llm.Message{{Role: "user", Content: llm.MessageContent{Content: &prompt}}},
		MaxTokens: &mt,
	}
	out, err := relay.NewOutbound(ch.Type, llmReq, ch.BaseURL, key)
	if err != nil {
		return modelTestResult{ModelName: modelName, OK: false, Error: err.Error()}, 0
	}
	outReq, err := out.TransformRequest(ctx, llmReq)
	if err != nil {
		return modelTestResult{ModelName: modelName, OK: false, Error: err.Error()}, 0
	}

	httpClient, err := helper.ChannelHttpClient(&ch)
	if err != nil {
		return modelTestResult{ModelName: modelName, OK: false, Error: err.Error()}, 0
	}

	httpReq := &http.Request{
		Method:        outReq.Method,
		Header:        outReq.Headers,
		Body:          io.NopCloser(bytes.NewReader(outReq.Body)),
		ContentLength: int64(len(outReq.Body)), // 必须定长: 腾讯等上游拒绝 chunked(412)
	}
	if u, err := url.Parse(outReq.URL); err == nil {
		httpReq.URL = u
	} else {
		return modelTestResult{ModelName: modelName, OK: false, Error: "invalid url: " + outReq.URL}, 0
	}
	if httpReq.Header == nil {
		httpReq.Header = make(http.Header)
	}
	if outReq.Auth != nil && outReq.Auth.Type == "bearer" && outReq.Auth.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+outReq.Auth.APIKey)
	}

	start := time.Now()
	resp2, err := httpClient.Do(httpReq)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return modelTestResult{ModelName: modelName, OK: false, LatencyMS: latency, Error: err.Error()}, 0
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp2.Body, 4096))

	if resp2.StatusCode >= 200 && resp2.StatusCode < 300 {
		return modelTestResult{
			ModelName: modelName,
			OK:        true,
			LatencyMS: latency,
			Content:   extractReplyContent(body),
		}, resp2.StatusCode
	}
	return modelTestResult{
		ModelName: modelName,
		OK:        false,
		LatencyMS: latency,
		Error:     fmt.Sprintf("status %d: %s", resp2.StatusCode, truncate(string(body), 300)),
	}, resp2.StatusCode
}

// extractReplyContent 从非流式响应里提取模型回复(openai: choices[0].message.content;
// mistral conversations: outputs[0].content), 提取不到返回原文截断。
func extractReplyContent(body []byte) string {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Outputs []struct {
			Content string `json:"content"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if len(parsed.Choices) > 0 && parsed.Choices[0].Message.Content != "" {
			return truncate(parsed.Choices[0].Message.Content, 120)
		}
		if len(parsed.Outputs) > 0 && parsed.Outputs[0].Content != "" {
			return truncate(parsed.Outputs[0].Content, 120)
		}
	}
	return truncate(strings.TrimSpace(string(body)), 120)
}

// fetchErrorStatus 从 FetchModels 错误文本提取上游状态码("fetch models failed: status 403: ...")。
var fetchErrorStatusRe = regexp.MustCompile(`status (\d{3})`)

func fetchErrorStatus(err error) int {
	if err == nil {
		return 200
	}
	if m := fetchErrorStatusRe.FindStringSubmatch(err.Error()); len(m) == 2 {
		var code int
		if _, e := fmt.Sscanf(m[1], "%d", &code); e == nil {
			return code
		}
	}
	return 0 // 未知错误(连接失败等), 回写时走 upstream_error
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
