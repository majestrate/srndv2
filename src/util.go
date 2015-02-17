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

// ensure a directory exists
func EnsureDir(dirname string) {
	stat, err := os.Stat(dirname)
	if os.IsNotExist(err) {
		os.Mkdir(dirname, 0755)
	} else if ! stat.IsDir() {
		os.Remove(dirname)
		os.Mkdir(dirname, 0755)
	}
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
