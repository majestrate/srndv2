package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/majestrate/srndv2/lib/config"
	"github.com/majestrate/srndv2/lib/database"
	"github.com/majestrate/srndv2/lib/frontend"
	"github.com/majestrate/srndv2/lib/nntp"
	"github.com/majestrate/srndv2/lib/store"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.Info("starting up nntpchan...")
	cfg_fname := "nntpchan.json"
	conf, err := config.Ensure(cfg_fname)
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
			log.Infof("Frontend bind to %s", faddr)
			fl, err := net.Listen("tcp", faddr)
			if err == nil {
				// run frontend
				err = fserv.Serve(fl)
				if err != nil {
					fl.Close()
					log.Errorf("frontend.serve() %s", err.Error())
				}
			} else {
				log.Errorf("frontend  net.Listen failed: %s", err.Error())
			}
			time.Sleep(time.Second)
		}
	}()

	// nntp server loop
	go func() {
		for {
			naddr := conf.NNTP.Bind
			log.Infof("Bind nntp server to %s", naddr)
			nl, err := net.Listen("tcp", naddr)
			if err == nil {
				err = nserv.Serve(nl)
				if err != nil {
					nl.Close()
					log.Errorf("nntpserver.serve() %s", err.Error())
				}
			} else {
				log.Errorf("nntp server net.Listen failed: %s", err.Error())
			}
			time.Sleep(time.Second)
		}
	}()
	sigchnl := make(chan os.Signal, 1)
	signal.Notify(sigchnl, syscall.SIGTERM, syscall.SIGHUP)
	for {
		s := <-sigchnl
		if s == syscall.SIGHUP {
			conf, err := config.Ensure(cfg_fname)
			if err == nil {
				log.Infof("reloading config: %s", cfg_fname)
				fserv.Reload(conf.Frontend)

				nserv.ReloadServer(conf.NNTP)
				nserv.ReloadFeeds(conf.Feeds)
			}
		}
	}
}
