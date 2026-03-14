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
	mu     sync.Mutex // 保护 usage 的选择+记录，Pick 整体原子执行以消除并发竞态
	limMu  sync.RWMutex
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
	s.limMu.RLock()
	l, ok := s.limiters[key]
	s.limMu.RUnlock()
	if ok {
		return l
	}
	s.limMu.Lock()
	defer s.limMu.Unlock()
	if l, ok = s.limiters[key]; ok {
		return l
	}
	burst := int(qps * 2)
	if burst < 1 {
		burst = 1
	}
	l = rate.NewLimiter(rate.Limit(qps), burst)
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

	// 「读取负载 → 选择渠道 → 记录请求」必须在同一把锁内完成，否则并发请求会同时
	// 读到相同的最低负载，全部选中同一个渠道（通常是列表第一个），导致负载均衡失效。
	s.mu.Lock()
	defer s.mu.Unlock()

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

	for _, ep := range bestCandidates {
		if ep.QPSLimit <= 0 {
			s.usage[s.endpointKey(ep)]++
			return ep, true
		}
		// getLimiter 使用独立的 limMu，不会和外层 mu 死锁
		l := s.getLimiter(s.endpointKey(ep), ep.QPSLimit)
		if l != nil && l.Allow() {
			s.usage[s.endpointKey(ep)]++
			return ep, true
		}
	}
	// 全部 QPS 受限时仍选第一个（上游可能 429）
	s.usage[s.endpointKey(bestCandidates[0])]++
	return bestCandidates[0], true
}

// recordUsage is kept for external callers (e.g. tests), but Pick now inlines
// the increment inside its own critical section.
func (s *ModelScheduler) recordUsage(ep config.ChannelEndpoint) {
	s.mu.Lock()
	s.usage[s.endpointKey(ep)]++
	s.mu.Unlock()
}
