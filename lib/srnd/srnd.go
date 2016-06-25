package srnd

import (
	log "github.com/Sirupsen/logrus"
	"github.com/majestrate/srndv2/lib/config"
	"github.com/majestrate/srndv2/lib/nntp"
)

// entry point for srndv2
func Driver() {
	log.Info("srndv2 starting up")
	fname := "srnd.json"
	// initialize configs
	log.Debugf("load config %s", fname)
	cfg, err := config.EnsureJSON(fname)
	if err != nil {
		log.Errorf("failed to load config: %s", err.Error())
		return
	}
	log.Debugf("begin init nntp")
	serv := new(nntp.Server)
	serv.Feeds = append(serv.Feeds, cfg.Feeds...)
	serv.Config = cfg.NNTP
}
