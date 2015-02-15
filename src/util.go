//
// util.go
//

package main

import (
	"log"
	"os"
	"strings"
)

func CheckFile(fname string) bool {
	if _, err := os.Stat(fname) ; os.IsNotExist(err) {
		return false
	}
	return true
}

// TODO make this work better
func ValidMessageID(id string) bool {
	
	if id[0] != '<' || id[len(id)-1] != '>' {
		log.Println(id[0], id[len(id)-1])
		return false
	}
	if strings.Count(id, "@") != 1 {
		return false
	}
	if strings.Count(id, "/") > 0 {
		return false
	}
	return true
}
