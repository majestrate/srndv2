package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/majestrate/srndv2/lib/config"
	"github.com/majestrate/srndv2/lib/nntp"
	"net"
)

func main() {

	log.Info("starting up nntp server")
	conf, err := config.Ensure("settings.json")
	if err != nil {
		log.Fatal(err)
	}

	if conf.Log == "debug" {
		log.SetLevel(log.DebugLevel)
	}

	serv := new(nntp.Server)
	l, err := net.Listen("tcp", conf.NNTP.Bind)
	if err != nil {
		log.Fatal(err)
	}
	log.Info("listening on ", l.Addr())
	err = serv.Serve(l)
	if err != nil {
		log.Fatal(err)
	}
}
