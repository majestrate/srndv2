package main

import (
	"github.com/majestrate/srnd"
	"log"
)


func main() {
	var daemon srnd.NNTPDaemon
	err := daemon.LoadConfig("srnd.ini")
	if err != nil {
		log.Println("could not load config", err)
	} else {
		daemon.Run()
	}
}
