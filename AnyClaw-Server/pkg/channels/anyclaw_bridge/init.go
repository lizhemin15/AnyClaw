package anyclaw_bridge

import (
	"github.com/anyclaw/anyclaw-server/pkg/bus"
	"github.com/anyclaw/anyclaw-server/pkg/channels"
	"github.com/anyclaw/anyclaw-server/pkg/config"
)

func init() {
	channels.RegisterFactory("anyclaw_bridge", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewBridgeChannel(cfg.Channels.AnyClawBridge, b)
	})
}
