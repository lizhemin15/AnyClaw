package anyclaw_bridge

import (
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
)

func init() {
	channels.RegisterFactory("anyclaw_bridge", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewBridgeChannel(cfg.Channels.AnyClawBridge, b)
	})
}
