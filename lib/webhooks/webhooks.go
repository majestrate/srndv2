package webhooks

import (
	"github.com/majestrate/srndv2/lib/config"
	"github.com/majestrate/srndv2/lib/nntp"
	"github.com/majestrate/srndv2/lib/nntp/message"
	"github.com/majestrate/srndv2/lib/store"
)

type Webhook interface {
	// implements nntp.EventHooks
	nntp.EventHooks
}

// create webhook multiplexing multiple web hooks
func NewWebhooks(conf []*config.WebhookConfig, st store.Storage) Webhook {
	h := message.NewHeaderIO()
	var hooks []Webhook
	for _, c := range conf {
		hooks = append(hooks, &httpWebhook{
			conf:    c,
			storage: st,
			hdr:     h,
		})
	}

	return &multiWebhook{
		hooks: hooks,
	}
}
