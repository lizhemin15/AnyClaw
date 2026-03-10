package pico

import (
	"github.com/anyclaw/anyclaw-server/pkg/bus"
	"github.com/anyclaw/anyclaw-server/pkg/channels"
	"github.com/anyclaw/anyclaw-server/pkg/config"
)

func init() {
	channels.RegisterFactory("pico", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewPicoChannel(cfg.Channels.Pico, b)
	})
}
