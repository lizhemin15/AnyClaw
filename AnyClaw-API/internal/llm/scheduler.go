package llm

import (
	"sync"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"golang.org/x/time/rate"
)

// UsageByProvider 用于注入今日用量查询（便于测试）
type UsageByProvider interface {
	GetUsageByProviderToday(providers []string) (map[string]int64, error)
}

// 渠道冷却时长：
//   - CooldownTransient  瞬时错误（随机 5xx / 网络抖动），60 秒后恢复
//   - CooldownDailyLimit 配额用尽 / 429 / 余额不足，冷却到当天结束
const (
	CooldownTransient  = 60 * time.Second
	CooldownDailyLimit = 24 * time.Hour
)

// ModelScheduler 模型渠道调度器。
// 选渠道逻辑：
//  1. 过滤日 tokens 超限的渠道
//  2. 优先使用不在故障冷却期内的健康渠道
//  3. 在健康渠道中按当前进行中请求数（负载）最小的为准
//  4. 负载相同时用 counter 轮转，避免永远选同一个渠道
//  5. 通过 QPS 令牌桶做最后过滤；全部 QPS 受限时仍选一个（允许上游 429）
type ModelScheduler struct {
	mu       sync.Mutex
	limMu    sync.RWMutex
	usage    map[string]int64     // 进行中请求计数，key: channelID|apiBase
	failures map[string]time.Time // 渠道冷却到期时间（到期前不选用）
	counter  uint64               // 等负载轮转计数器
	limiters map[string]*rate.Limiter
	db       UsageByProvider
}

// NewModelScheduler 创建模型调度器，db 可为 nil（不查日 tokens）
func NewModelScheduler(database UsageByProvider) *ModelScheduler {
	return &ModelScheduler{
		usage:    make(map[string]int64),
		failures: make(map[string]time.Time),
		limiters: make(map[string]*rate.Limiter),
		db:       database,
	}
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

// RecordFailure 标记渠道进入冷却期，dur 决定冷却时长。
//   - CooldownTransient  用于瞬时 5xx / 网络错误
//   - CooldownDailyLimit 用于 429 / 配额用尽 / 余额不足
func (s *ModelScheduler) RecordFailure(ep config.ChannelEndpoint, dur time.Duration) {
	s.mu.Lock()
	s.failures[s.endpointKey(ep)] = time.Now().Add(dur)
	s.mu.Unlock()
}

// Pick 从候选列表中选择最优渠道。
func (s *ModelScheduler) Pick(model string, candidates []config.ChannelEndpoint) (config.ChannelEndpoint, bool) {
	if len(candidates) == 0 {
		return config.ChannelEndpoint{}, false
	}

	available := s.filterByDailyTokens(candidates)
	if len(available) == 0 {
		return config.ChannelEndpoint{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// 过滤冷却期内的故障渠道；若全部都在冷却，则降级使用全部可用渠道
	healthy := make([]config.ChannelEndpoint, 0, len(available))
	for _, ep := range available {
		if until, failed := s.failures[s.endpointKey(ep)]; !failed || now.After(until) {
			healthy = append(healthy, ep)
		}
	}
	if len(healthy) == 0 {
		healthy = available
	}

	// 找当前最低负载
	var minLoad int64 = -1
	for _, ep := range healthy {
		n := s.usage[s.endpointKey(ep)]
		if minLoad < 0 || n < minLoad {
			minLoad = n
		}
	}

	// 收集负载相同的候选渠道
	best := make([]config.ChannelEndpoint, 0, len(healthy))
	for _, ep := range healthy {
		if s.usage[s.endpointKey(ep)] == minLoad {
			best = append(best, ep)
		}
	}

	// 用 counter 轮转起始位，避免等负载时永远选同一个渠道
	n := uint64(len(best))
	start := int(s.counter % n)
	s.counter++

	for i := 0; i < len(best); i++ {
		ep := best[(start+i)%len(best)]
		if ep.QPSLimit <= 0 {
			s.usage[s.endpointKey(ep)]++
			return ep, true
		}
		l := s.getLimiter(s.endpointKey(ep), ep.QPSLimit)
		if l != nil && l.Allow() {
			s.usage[s.endpointKey(ep)]++
			return ep, true
		}
	}

	// 全部 QPS 受限时仍选轮转位的第一个（允许上游 429）
	ep := best[start]
	s.usage[s.endpointKey(ep)]++
	return ep, true
}

// Done 在请求完成后递减负载计数（成功或失败均需调用）。
func (s *ModelScheduler) Done(ep config.ChannelEndpoint) {
	s.mu.Lock()
	k := s.endpointKey(ep)
	if s.usage[k] > 0 {
		s.usage[k]--
	}
	s.mu.Unlock()
}

func (s *ModelScheduler) filterByDailyTokens(candidates []config.ChannelEndpoint) []config.ChannelEndpoint {
	if s.db == nil {
		return candidates
	}
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
	out := make([]config.ChannelEndpoint, 0, len(candidates))
	for _, ep := range candidates {
		if ep.DailyTokensLimit <= 0 || usageMap[ep.APIBase] < ep.DailyTokensLimit {
			out = append(out, ep)
		}
	}
	return out
}

// recordUsage kept for external callers / tests.
func (s *ModelScheduler) recordUsage(ep config.ChannelEndpoint) {
	s.mu.Lock()
	s.usage[s.endpointKey(ep)]++
	s.mu.Unlock()
}
