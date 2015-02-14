//
// api.go
//
package srnd


import (
	"log"
)

type API struct {
	
}

type API_File struct {
	mime string
	extension string
	name string
	data string
}

type API_Article struct {
	id string
	newsgroup string
	op bool
	thread string
	frontend string
	sage bool
	subject string
	comment string
}

// TODO: implement
func (*API) sync(newsgroup string)  {
	log.Println("request sync for newsgroup", newsgroup)
}
