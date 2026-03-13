package llm

import (
	"sync"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"golang.org/x/time/rate"
)

// UsageByProvider 用于注入今日用量查询（便于测试）
type UsageByProvider interface {
	GetUsageByProviderToday(providers []string) (map[string]int64, error)
}

// ModelScheduler 模型渠道调度器：按日 tokens 上限、QPS 限制过滤后，负载均衡选择请求数最少的渠道
type ModelScheduler struct {
	mu       sync.RWMutex
	usage    map[string]int64 // key: channelID|apiBase，请求计数
	limiters map[string]*rate.Limiter
	db       UsageByProvider
}

// NewModelScheduler 创建模型调度器，db 可为 nil（不查日 tokens）
func NewModelScheduler(database UsageByProvider) *ModelScheduler {
	s := &ModelScheduler{
		usage:    make(map[string]int64),
		limiters: make(map[string]*rate.Limiter),
		db:       database,
	}
	return s
}

func (s *ModelScheduler) endpointKey(ep config.ChannelEndpoint) string {
	return ep.ChannelID + "|" + ep.APIBase
}

func (s *ModelScheduler) getLimiter(key string, qps float64) *rate.Limiter {
	if qps <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.limiters[key]; ok {
		return l
	}
	// burst 设为 qps*2，至少 1，避免突发被限
	burst := int(qps * 2)
	if burst < 1 {
		burst = 1
	}
	l := rate.NewLimiter(rate.Limit(qps), burst)
	s.limiters[key] = l
	return l
}

// Pick 从候选中选择渠道：优先过滤日 tokens 超限、QPS 超限的，再按负载最低选
func (s *ModelScheduler) Pick(model string, candidates []config.ChannelEndpoint) (config.ChannelEndpoint, bool) {
	if len(candidates) == 0 {
		return config.ChannelEndpoint{}, false
	}

	// 日 tokens 过滤
	var available []config.ChannelEndpoint
	if s.db != nil {
		providers := make([]string, 0, len(candidates))
		for _, ep := range candidates {
			if ep.DailyTokensLimit > 0 {
				providers = append(providers, ep.APIBase)
			}
		}
		usageMap := make(map[string]int64)
		if len(providers) > 0 {
			if m, err := s.db.GetUsageByProviderToday(providers); err == nil {
				usageMap = m
			}
		}
		for _, ep := range candidates {
			if ep.DailyTokensLimit <= 0 {
				available = append(available, ep)
				continue
			}
			used := usageMap[ep.APIBase]
			if used < ep.DailyTokensLimit {
				available = append(available, ep)
			}
		}
	} else {
		available = candidates
	}
	if len(available) == 0 {
		return config.ChannelEndpoint{}, false
	}

	// 按负载选最低，若有多个并列则按顺序尝试 QPS
	s.mu.Lock()
	var minLoad int64 = -1
	for _, ep := range available {
		n := s.usage[s.endpointKey(ep)]
		if minLoad < 0 || n < minLoad {
			minLoad = n
		}
	}
	var bestCandidates []config.ChannelEndpoint
	for _, ep := range available {
		if s.usage[s.endpointKey(ep)] == minLoad {
			bestCandidates = append(bestCandidates, ep)
		}
	}
	s.mu.Unlock()

	for _, ep := range bestCandidates {
		if ep.QPSLimit <= 0 {
			s.recordUsage(ep)
			return ep, true
		}
		l := s.getLimiter(s.endpointKey(ep), ep.QPSLimit)
		if l != nil && l.Allow() {
			s.recordUsage(ep)
			return ep, true
		}
	}
	// 全部 QPS 受限时仍选第一个（上游可能 429）
	s.recordUsage(bestCandidates[0])
	return bestCandidates[0], true
}

func (s *ModelScheduler) recordUsage(ep config.ChannelEndpoint) {
	s.mu.Lock()
	s.usage[s.endpointKey(ep)]++
	s.mu.Unlock()
}
