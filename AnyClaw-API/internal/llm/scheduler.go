package llm

import (
	"sync"

	"github.com/anyclaw/anyclaw-api/internal/config"
)

// ModelScheduler 模型渠道调度器：负载均衡选择请求数最少的渠道
type ModelScheduler struct {
	mu    sync.RWMutex
	usage map[string]int64 // key: channelID|apiBase
}

// NewModelScheduler 创建模型调度器
func NewModelScheduler() *ModelScheduler {
	return &ModelScheduler{usage: make(map[string]int64)}
}

func (s *ModelScheduler) endpointKey(ep config.ChannelEndpoint) string {
	return ep.ChannelID + "|" + ep.APIBase
}

// Pick 从候选中选择负载最低的渠道，并增加其计数
func (s *ModelScheduler) Pick(model string, candidates []config.ChannelEndpoint) (config.ChannelEndpoint, bool) {
	if len(candidates) == 0 {
		return config.ChannelEndpoint{}, false
	}
	if len(candidates) == 1 {
		s.recordUsage(candidates[0])
		return candidates[0], true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var best config.ChannelEndpoint
	var minUsage int64 = -1
	for _, ep := range candidates {
		key := s.endpointKey(ep)
		n := s.usage[key]
		if minUsage < 0 || n < minUsage {
			minUsage = n
			best = ep
		}
	}
	s.usage[s.endpointKey(best)]++
	return best, true
}

func (s *ModelScheduler) recordUsage(ep config.ChannelEndpoint) {
	s.mu.Lock()
	s.usage[s.endpointKey(ep)]++
	s.mu.Unlock()
}
