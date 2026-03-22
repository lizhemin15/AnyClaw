package llm

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"golang.org/x/time/rate"
)

// UsageByProvider 用于注入今日用量查询（便于测试）
type UsageByProvider interface {
	GetUsageByProviderToday(providers []string) (map[string]int64, error)
}

// 渠道冷却时长（仅用于 proxy 计算 until）：
//   - CooldownTransient  瞬时错误，60 秒后恢复
//   - 配额用尽 / 429 等：冷却到北京时间次日 0 点
const CooldownTransient = 60 * time.Second

// ModelScheduler 模型渠道调度器。
// 选渠道逻辑：
//  1. 过滤日 tokens 超限的渠道
//  2. 过滤冷却期内的渠道（配额用尽等自动禁用至次日 0 点）
//  3. 负载 = token 用量/上限比例 + 进行中请求数；各渠道上限不同，用比例比较
//  4. 负载接近时轮转，避免只薅同一渠道
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

// RecordFailureUntil 标记渠道进入冷却期，until 为冷却到期时间（到期后可再次选用）。
// 配额用尽类错误应传入北京时间次日 0 点；瞬时错误传入 time.Now().Add(60*time.Second)。
func (s *ModelScheduler) RecordFailureUntil(ep config.ChannelEndpoint, until time.Time) {
	s.mu.Lock()
	s.failures[s.endpointKey(ep)] = until
	s.mu.Unlock()
}

// ClearFailure 清除渠道冷却，用于保活探测成功时恢复渠道。
func (s *ModelScheduler) ClearFailure(ep config.ChannelEndpoint) {
	s.mu.Lock()
	delete(s.failures, s.endpointKey(ep))
	s.mu.Unlock()
}

// ChannelStatus 渠道实时状态，供管理后台展示
type ChannelStatus struct {
	ChannelID       string `json:"channel_id"`
	TokenUsageToday int64  `json:"token_usage_today"`
	Available       bool   `json:"available"`
	CooldownUntil   string `json:"cooldown_until,omitempty"`
	InFlight        int64  `json:"in_flight"`
}

// GetChannelStatus 返回各渠道的当日 token 用量、可用性、冷却到期时间
func (s *ModelScheduler) GetChannelStatus(channels []config.Channel) []ChannelStatus {
	if len(channels) == 0 {
		return nil
	}
	providers := make([]string, 0, len(channels)*2)
	for _, ch := range channels {
		if ch.Name != "" {
			providers = append(providers, ch.Name)
		}
		base := strings.TrimSuffix(ch.APIBase, "/")
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		providers = append(providers, base)
	}
	usageMap := make(map[string]int64)
	if s.db != nil && len(providers) > 0 {
		if m, err := s.db.GetUsageByProviderToday(providers); err == nil {
			usageMap = m
		}
	}
	s.mu.Lock()
	now := time.Now()
	out := make([]ChannelStatus, 0, len(channels))
	for _, ch := range channels {
		base := strings.TrimSuffix(ch.APIBase, "/")
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		key := ch.ID + "|" + base
		until, inCooldown := s.failures[key]
		available := !inCooldown || now.After(until)
		cooldownUntil := ""
		if inCooldown && until.After(now) {
			cooldownUntil = until.Format("2006-01-02 15:04")
		}
		pk := ch.Name
		if pk == "" {
			pk = base
		}
		tokens := usageMap[pk]
		if tokens == 0 && ch.Name != "" {
			tokens = usageMap[base]
		}
		out = append(out, ChannelStatus{
			ChannelID:       ch.ID,
			TokenUsageToday: tokens,
			Available:       available,
			CooldownUntil:   cooldownUntil,
			InFlight:        s.usage[key],
		})
	}
	s.mu.Unlock()
	return out
}

// GetVoiceAPIStatus 返回语音 API 端点的实时状态
func (s *ModelScheduler) GetVoiceAPIStatus(endpoints []config.VoiceAPIEndpoint) []ChannelStatus {
	if len(endpoints) == 0 {
		return nil
	}
	providers := make([]string, 0, len(endpoints)*2)
	for _, ep := range endpoints {
		if ep.Name != "" {
			providers = append(providers, ep.Name)
		}
		base := strings.TrimSuffix(ep.Endpoint, "/")
		if base != "" {
			providers = append(providers, base)
		}
	}
	usageMap := make(map[string]int64)
	if s.db != nil && len(providers) > 0 {
		if m, err := s.db.GetUsageByProviderToday(providers); err == nil {
			usageMap = m
		}
	}
	s.mu.Lock()
	now := time.Now()
	out := make([]ChannelStatus, 0, len(endpoints))
	for _, ep := range endpoints {
		base := strings.TrimSuffix(ep.Endpoint, "/")
		key := ep.ID + "|" + base
		until, inCooldown := s.failures[key]
		available := !inCooldown || now.After(until)
		cooldownUntil := ""
		if inCooldown && until.After(now) {
			cooldownUntil = until.Format("2006-01-02 15:04")
		}
		pk := ep.Name
		if pk == "" {
			pk = base
		}
		tokens := usageMap[pk]
		if tokens == 0 && ep.Name != "" {
			tokens = usageMap[base]
		}
		out = append(out, ChannelStatus{
			ChannelID:       ep.ID,
			TokenUsageToday: tokens,
			Available:       available,
			CooldownUntil:   cooldownUntil,
			InFlight:        s.usage[key],
		})
	}
	s.mu.Unlock()
	return out
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

	// 过滤冷却期内的渠道：配额用尽等自动禁用至次日 0 点，0 点后自动恢复
	healthy := make([]config.ChannelEndpoint, 0, len(available))
	for _, ep := range available {
		if until, failed := s.failures[s.endpointKey(ep)]; !failed || now.After(until) {
			healthy = append(healthy, ep)
		}
	}
	if len(healthy) == 0 {
		// 全部在冷却常见于网关/飞书绑定后重启：并发 429/5xx 给每个渠道打了短时冷却，
		// 若此处直接失败会立刻 502。仍从「未超日配额」的候选里按原负载逻辑选一条尝试，
		// 由上游决定是否继续限流；真正配额用尽时上游仍会失败并再次写入长冷却。
		healthy = slices.Clone(available)
	}

	// 负载 = token 用量/上限比例 + 进行中请求数；各渠道上限不同，用比例才能比较
	tokenUsage := s.getTokenUsageToday(healthy)
	var minLoad float64 = -1
	for _, ep := range healthy {
		used := tokenUsage[providerKey(ep)]
		limit := ep.DailyTokensLimit
		if limit <= 0 {
			limit = 1e12 // 无上限视为超大，比例趋近 0
		}
		ratio := float64(used) / float64(limit)
		inFlight := s.usage[s.endpointKey(ep)]
		load := ratio + float64(inFlight)*0.1 // 比例为主，进行中请求为辅
		if minLoad < 0 || load < minLoad {
			minLoad = load
		}
	}

	// 收集负载接近最低的候选（允许差距 0.2，避免只薅同一渠道）
	loadTolerance := 0.2
	best := make([]config.ChannelEndpoint, 0, len(healthy))
	for _, ep := range healthy {
		used := tokenUsage[providerKey(ep)]
		limit := ep.DailyTokensLimit
		if limit <= 0 {
			limit = 1e12
		}
		ratio := float64(used) / float64(limit)
		inFlight := s.usage[s.endpointKey(ep)]
		load := ratio + float64(inFlight)*0.1
		if load <= minLoad+loadTolerance {
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

// providerKey 与 usage_log 的 provider 列一致：优先渠道名，否则 apiBase
func providerKey(ep config.ChannelEndpoint) string {
	if ep.ChannelName != "" {
		return ep.ChannelName
	}
	return ep.APIBase
}

func (s *ModelScheduler) getTokenUsageToday(candidates []config.ChannelEndpoint) map[string]int64 {
	out := make(map[string]int64)
	if s.db == nil || len(candidates) == 0 {
		return out
	}
	providers := make([]string, 0, len(candidates))
	for _, ep := range candidates {
		providers = append(providers, providerKey(ep))
	}
	if m, err := s.db.GetUsageByProviderToday(providers); err == nil {
		return m
	}
	return out
}

func (s *ModelScheduler) filterByDailyTokens(candidates []config.ChannelEndpoint) []config.ChannelEndpoint {
	if s.db == nil {
		return candidates
	}
	providers := make([]string, 0, len(candidates))
	for _, ep := range candidates {
		if ep.DailyTokensLimit > 0 {
			providers = append(providers, providerKey(ep))
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
		if ep.DailyTokensLimit <= 0 || usageMap[providerKey(ep)] < ep.DailyTokensLimit {
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
