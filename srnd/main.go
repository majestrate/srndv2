package main

import (
	"log"
)


func main() {
	var daemon NNTPDaemon
	err := daemon.LoadConfig("srnd.ini")
	if err != nil {
		log.Println("could not load config", err)
	} else {
		daemon.Run()
	}
}
