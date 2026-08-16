package helper

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/bestruirui/octopus/internal/client"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/utils/log"
	"github.com/bestruirui/octopus/internal/utils/xstrings"
	"github.com/dlclark/regexp2"
)

// proxyPoolRR 代理池轮换计数器(非 sticky 时轮换出口 IP)。
var proxyPoolRR atomic.Uint64

// ChannelHttpClient 根据渠道代理配置创建 HTTP 客户端。
// 优先级: proxy_pool(多 IP 池) > channel_proxy(单代理) > 系统代理。
// sticky=true 时按渠道 ID 哈希固定代理(同渠道同出口); 否则轮换。
func ChannelHttpClient(channel *model.Channel) (*http.Client, error) {
	if !channel.Proxy {
		return client.GetHTTPClientSystemProxy(false)
	}
	if len(channel.ProxyPool) > 0 {
		pool := make([]string, 0, len(channel.ProxyPool))
		for _, p := range channel.ProxyPool {
			if strings.TrimSpace(p) != "" {
				pool = append(pool, strings.TrimSpace(p))
			}
		}
		if len(pool) > 0 {
			idx := proxyPoolRR.Add(1) - 1
			if channel.Sticky {
				idx = uint64(channel.ID)
			}
			return client.GetHTTPClientCustomProxy(pool[idx%uint64(len(pool))])
		}
	}
	if channel.ChannelProxy == nil || strings.TrimSpace(*channel.ChannelProxy) == "" {
		return client.GetHTTPClientSystemProxy(true)
	}
	return client.GetHTTPClientCustomProxy(strings.TrimSpace(*channel.ChannelProxy))
}

// ChannelAutoGroup 根据渠道模型和分组规则补充分组成员。
func ChannelAutoGroup(channel *model.Channel, ctx context.Context) {
	if channel == nil {
		return
	}
	if channel.AutoGroup == model.AutoGroupTypeNone {
		return
	}
	groups, err := op.GroupList(ctx)
	if err != nil {
		log.Warnf("get group list failed: %v", err)
		return
	}

	channelModelNames := xstrings.SplitTrimCompact(",", channel.Model, channel.CustomModel)
	if len(channelModelNames) == 0 {
		return
	}

	for _, group := range groups {
		matchedModelNames := make([]string, 0, len(channelModelNames))

		switch channel.AutoGroup {
		case model.AutoGroupTypeExact:
			for _, modelName := range channelModelNames {
				if strings.EqualFold(modelName, group.Name) {
					matchedModelNames = append(matchedModelNames, modelName)
				}
			}

		case model.AutoGroupTypeFuzzy:
			groupNameLower := strings.ToLower(strings.TrimSpace(group.Name))
			if groupNameLower == "" {
				continue
			}
			for _, modelName := range channelModelNames {
				if strings.Contains(strings.ToLower(modelName), groupNameLower) {
					matchedModelNames = append(matchedModelNames, modelName)
				}
			}

		case model.AutoGroupTypeRegex:
			if group.MatchRegex == "" {
				for _, modelName := range channelModelNames {
					if strings.EqualFold(modelName, group.Name) {
						matchedModelNames = append(matchedModelNames, modelName)
					}
				}
				break
			}

			re, err := regexp2.Compile(group.MatchRegex, regexp2.ECMAScript)
			if err != nil {
				log.Warnf("compile regex failed (channel=%d group=%d regex=%q): %v", channel.ID, group.ID, group.MatchRegex, err)
				continue
			}
			for _, modelName := range channelModelNames {
				matched, err := re.MatchString(modelName)
				if err != nil {
					log.Warnf("match regex failed (channel=%d group=%d regex=%q model=%q): %v", channel.ID, group.ID, group.MatchRegex, modelName, err)
					continue
				}
				if matched {
					matchedModelNames = append(matchedModelNames, modelName)
				}
			}
		}

		if len(matchedModelNames) > 0 {
			items := make([]model.GroupIDAndLLMName, 0, len(matchedModelNames))
			for _, modelName := range matchedModelNames {
				items = append(items, model.GroupIDAndLLMName{
					ChannelID: channel.ID,
					ModelName: modelName,
				})
			}
			if err := op.GroupItemBatchAdd(group.ID, items, ctx); err != nil {
				log.Warnf("group item batch add failed (channel=%d group=%d): %v", channel.ID, group.ID, err)
			}
		}
	}
}
