//
// daemon.go
//
package srnd

import (
	"log"
	"os"
)

type NNTPDaemon struct {
	instance_name string
	api_caller *API
}

// TODO implement
func (*NNTPDaemon) Run() {
	
}

// TODO implement
func (*NNTPDaemon) Bind(addr string) error {
	return nil
}

// TODO implement
func (*NNTPDaemon) LoadConfig(fname string) error {
	if _, err := os.Stat(fname) ; os.IsNotExist(err) {
		log.Println("no such config file", fname)
		log.Println("create config", fname)
		GenConfig(fname)
	}
	log.Println("load config file", fname)
	return nil
}
