package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/majestrate/srndv2/lib/config"
	"github.com/majestrate/srndv2/lib/database"
	"github.com/majestrate/srndv2/lib/frontend"
	"github.com/majestrate/srndv2/lib/nntp"
	"github.com/majestrate/srndv2/lib/store"
	"net"
	"time"
)

func main() {
	log.Info("starting up nntpchan...")
	conf, err := config.Ensure("nntpchan.json")
	if err != nil {
		log.Fatal(err)
	}

	if conf.Log == "debug" {
		log.SetLevel(log.DebugLevel)
	}

	fconfig := conf.Frontend
	if fconfig == nil {
		log.Fatal("no frontend configured")
	}

	sconfig := conf.Store

	if sconfig == nil {
		log.Fatal("no article storage configured")
	}

	nconfig := conf.NNTP

	if nconfig == nil {
		log.Fatal("no nntp server configured")
	}

	dconfig := conf.Database

	if dconfig == nil {
		log.Fatal("no database configured")
	}

	db, err := database.FromConfig(dconfig)
	if err != nil {
		log.Fatalf("failed to create dabatase: %s", err.Error())
	}

	fserv, err := frontend.NewHTTPFrontend(fconfig, db)

	if err != nil {
		log.Fatalf("failed to create frontend: %s", err.Error())
	}

	// create nntp server
	nserv := &nntp.Server{
		Config: nconfig,
		Feeds:  conf.Feeds,
		Hooks:  fserv,
	}

	// create article storage
	nserv.Storage, err = store.NewFilesytemStorage(sconfig.Path)
	if err != nil {
		log.Fatal(err)
	}

	// frontent server loop
	go func() {
		for {
			faddr := fconfig.BindAddr
			time.Sleep(time.Second)
			log.Infof("Frontend bind to %s", faddr)
			fl, err := net.Listen("tcp", faddr)
			if err != nil {
				log.Errorf("frontend  net.Listen failed: %s", err.Error())
				continue
			}
			// run frontend
			err = fserv.Serve(fl)
			if err != nil {
				fl.Close()
				log.Errorf("frontend.serve() %s", err.Error())
			}
		}
	}()

	// nntp server loop
	go func() {
		for {
			time.Sleep(time.Second)
			naddr := conf.NNTP.Bind
			log.Infof("Bind nntp server to %s", naddr)
			nl, err := net.Listen("tcp", naddr)
			if err != nil {
				log.Errorf("nntp server net.Listen failed: %s", err.Error())
				continue
			}
			// run nntp
			err = nserv.Serve(nl)
			if err != nil {
				nl.Close()
				log.Errorf("nntpserver.serve() %s", err.Error())
			}
		}
	}()
	for {
		time.Sleep(time.Second)
	}
}
