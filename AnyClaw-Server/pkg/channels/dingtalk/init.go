package dingtalk

import (
	"github.com/anyclaw/anyclaw-server/pkg/bus"
	"github.com/anyclaw/anyclaw-server/pkg/channels"
	"github.com/anyclaw/anyclaw-server/pkg/config"
)

func init() {
	channels.RegisterFactory("dingtalk", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewDingTalkChannel(cfg.Channels.DingTalk, b)
	})
}
