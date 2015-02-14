package main

import (
	"srnd/daemon"
)


func main() {
	var daemon := srnd.MakeDaemon()
	err := daemon.LoadConfig("srnd.ini")
	if err != nil {
		fmt.Println("could not load config", err)
	} else {
		daemon.Run()
	}
}
