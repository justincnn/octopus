package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// testChannelModels 批量测试渠道已选模型可用性: 走真实 outbound 转换路径
// (openai→chat/completions, mistral→conversations 等), 2xx=可用。
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
	// 前端传打码 key 时, 用渠道 ID 从缓存取明文(无需先点眼睛显示密钥)
	fillPlainKey(c.Request.Context(), &req.Channel)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 5
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
			results[idx] = probeModel(ctx, req.Channel, modelName, maxTokens)
		}(i, name)
	}
	wg.Wait()

	resp.Success(c, gin.H{"results": results})
}

// probeModel 用渠道配置对单个模型发最小对话请求, 判定可用性。
func probeModel(ctx context.Context, ch model.Channel, modelName string, maxTokens int) modelTestResult {
	mt := int64(maxTokens)
	prompt := "回复:ok"
	llmReq := &llm.Request{
		Model:     modelName,
		Messages:  []llm.Message{{Role: "user", Content: llm.MessageContent{Content: &prompt}}},
		MaxTokens: &mt,
	}
	out, err := relay.NewOutbound(ch.Type, llmReq, ch.BaseURL, ch.Key)
	if err != nil {
		return modelTestResult{ModelName: modelName, OK: false, Error: err.Error()}
	}
	outReq, err := out.TransformRequest(ctx, llmReq)
	if err != nil {
		return modelTestResult{ModelName: modelName, OK: false, Error: err.Error()}
	}

	httpClient, err := helper.ChannelHttpClient(&ch)
	if err != nil {
		return modelTestResult{ModelName: modelName, OK: false, Error: err.Error()}
	}

	httpReq := &http.Request{
		Method: outReq.Method,
		Header: outReq.Headers,
		Body:   io.NopCloser(bytes.NewReader(outReq.Body)),
	}
	if u, err := url.Parse(outReq.URL); err == nil {
		httpReq.URL = u
	} else {
		return modelTestResult{ModelName: modelName, OK: false, Error: "invalid url: " + outReq.URL}
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
		return modelTestResult{ModelName: modelName, OK: false, LatencyMS: latency, Error: err.Error()}
	}
	defer resp2.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp2.Body, 4096))

	if resp2.StatusCode >= 200 && resp2.StatusCode < 300 {
		return modelTestResult{
			ModelName: modelName,
			OK:        true,
			LatencyMS: latency,
			Content:   extractReplyContent(body),
		}
	}
	return modelTestResult{
		ModelName: modelName,
		OK:        false,
		LatencyMS: latency,
		Error:     fmt.Sprintf("status %d: %s", resp2.StatusCode, truncate(string(body), 300)),
	}
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
