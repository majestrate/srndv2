//
// util.go
//

package main

import (
	"os"
)

func CheckFile(fname string) bool {
	if _, err := os.Stat(fname) ; os.IsNotExist(err) {
		return false
	}
	return true
}
