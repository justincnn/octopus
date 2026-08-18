package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/relay"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/task"
	"github.com/gin-gonic/gin"
)

func init() {
	router.NewGroupRouter("/api/v1/channel").
		Use(middleware.Auth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/list", http.MethodGet).
				Handle(listChannel),
		).
		AddRoute(
			router.NewRoute("/create", http.MethodPost).
				Handle(createChannel),
		).
		AddRoute(
			router.NewRoute("/update", http.MethodPost).
				Handle(updateChannel),
		).
		AddRoute(
			router.NewRoute("/enable", http.MethodPost).
				Handle(enableChannel),
		).
		AddRoute(
			router.NewRoute("/delete/:id", http.MethodDelete).
				Handle(deleteChannel),
		).
		AddRoute(
			router.NewRoute("/fetch-model", http.MethodPost).
				Handle(fetchModel),
		)
	router.NewGroupRouter("/api/v1/channel").
		Use(middleware.Auth()).
		AddRoute(
			router.NewRoute("/sync", http.MethodPost).
				Handle(syncChannel),
		).
		AddRoute(
			router.NewRoute("/last-sync-time", http.MethodGet).
				Handle(getLastSyncTime),
		).
		AddRoute(
			router.NewRoute("/key/:id", http.MethodGet).
				Handle(getChannelKey),
		).
		AddRoute(
			router.NewRoute("/keys/status", http.MethodGet).
				Handle(getChannelKeysStatus),
		).
		AddRoute(
			router.NewRoute("/keys/disable", http.MethodPost).
				Handle(disableChannelKey),
		).
		AddRoute(
			router.NewRoute("/keys/recover", http.MethodPost).
				Handle(recoverChannelKey),
		)
}

func listChannel(c *gin.Context) {
	channels, err := op.ChannelList(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	for i, channel := range channels {
		stats := op.StatsChannelGet(channel.ID)
		channels[i].Stats = &stats
		// key 打码: 列表只回显首尾, 编辑时前端不改就不提交(update 是 patch 语义)
		channels[i].Key = maskKey(channel.Key)
	}
	resp.Success(c, channels)
}

// maskKey 渠道 key 打码: 保留前3后4, 中间用 ****。
func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:3] + "****" + key[len(key)-4:]
}

// getChannelKey 返回渠道完整 key(仅供管理端眼睛切换显示用)。
func getChannelKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	channel, err := op.ChannelGet(id, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "channel not found")
		return
	}
	resp.Success(c, gin.H{"key": channel.Key})
}

// getChannelKeysStatus 返回渠道多 key 池状态列表(管理端展示: key/状态/原因/上次失败时间)。
func getChannelKeysStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("channel_id"))
	if err != nil || id <= 0 {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	channel, err := op.ChannelGet(id, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "channel not found")
		return
	}
	type keyStatus struct {
		Key        string `json:"key"`
		Status     string `json:"status"`
		Reason     string `json:"reason"`
		Disabled   bool   `json:"disabled"`
		FailCount  int    `json:"fail_count"`
		LastFailAt int64  `json:"last_fail_at"`
	}
	states := relay.KeyStateSnapshot(channel)
	out := make([]keyStatus, 0, len(states))
	for _, st := range states {
		var lastFail int64
		if !st.LastFailAt.IsZero() {
			lastFail = st.LastFailAt.Unix()
		}
		out = append(out, keyStatus{
			Key:        st.Key,
			Status:     st.Status,
			Reason:     st.Reason,
			Disabled:   st.Disabled,
			FailCount:  st.FailCount,
			LastFailAt: lastFail,
		})
	}
	resp.Success(c, out)
}

// disableChannelKey 手动禁用/启用渠道 key。
func disableChannelKey(c *gin.Context) {
	var req struct {
		ChannelID int    `json:"channel_id"`
		Key       string `json:"key"`
		Disabled  bool   `json:"disabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ChannelID <= 0 || req.Key == "" {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	channel, err := op.ChannelGet(req.ChannelID, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "channel not found")
		return
	}
	relay.DisableKey(channel, req.Key, req.Disabled)
	resp.Success(c, nil)
}

// recoverChannelKey 手动恢复渠道 key(清失败计数, 重新参与轮询)。
func recoverChannelKey(c *gin.Context) {
	var req struct {
		ChannelID int    `json:"channel_id"`
		Key       string `json:"key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ChannelID <= 0 || req.Key == "" {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	channel, err := op.ChannelGet(req.ChannelID, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusNotFound, "channel not found")
		return
	}
	relay.RecoverKey(channel, req.Key)
	resp.Success(c, nil)
}

func createChannel(c *gin.Context) {
	var channel model.Channel
	if err := c.ShouldBindJSON(&channel); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	if err := op.ChannelCreate(&channel, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	stats := op.StatsChannelGet(channel.ID)
	channel.Stats = &stats
	go func(channel *model.Channel) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		modelStr := channel.Model + "," + channel.CustomModel
		modelArray := strings.Split(modelStr, ",")
		helper.LLMPriceAddToDB(modelArray, ctx)
		helper.ChannelAutoGroup(channel, ctx)
	}(&channel)
	resp.Success(c, channel)
}

func updateChannel(c *gin.Context) {
	var req model.ChannelUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	channel, err := op.ChannelUpdate(&req, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	stats := op.StatsChannelGet(channel.ID)
	channel.Stats = &stats
	go func(channel *model.Channel) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		modelStr := channel.Model + "," + channel.CustomModel
		modelArray := strings.Split(modelStr, ",")
		helper.LLMPriceAddToDB(modelArray, ctx)
		helper.ChannelAutoGroup(channel, ctx)
	}(channel)
	resp.Success(c, channel)
}

func enableChannel(c *gin.Context) {
	var request struct {
		ID      int  `json:"id"`
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	if err := op.ChannelEnabled(request.ID, request.Enabled, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, nil)
}

func deleteChannel(c *gin.Context) {
	id := c.Param("id")
	idNum, err := strconv.Atoi(id)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidParam)
		return
	}
	if err := op.ChannelDel(idNum, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, nil)
}
func fetchModel(c *gin.Context) {
	var request model.Channel
	if err := c.ShouldBindJSON(&request); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.ErrInvalidJSON)
		return
	}
	// 前端传打码 key 时, 用渠道 ID 从缓存取明文(无需先点眼睛显示密钥)
	fillPlainKey(c.Request.Context(), &request)
	models, err := helper.FetchModels(c.Request.Context(), request)
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, models)
}

// fillPlainKey 请求体 key 是打码值(前3****后4)时, 用渠道 ID 从缓存取明文替换。
func fillPlainKey(ctx context.Context, ch *model.Channel) {
	if ch == nil || !strings.Contains(ch.Key, "****") || ch.ID <= 0 {
		return
	}
	if cached, err := op.ChannelGet(ch.ID, ctx); err == nil && cached.Key != "" {
		ch.Key = cached.Key
	}
}

func syncChannel(c *gin.Context) {
	task.SyncModelsTask()
	resp.Success(c, nil)
}

func getLastSyncTime(c *gin.Context) {
	time := task.GetLastSyncModelsTime()
	resp.Success(c, time)
}
