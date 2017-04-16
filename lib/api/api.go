package api

import (
	"github.com/majestrate/srndv2/lib/model"
)

// json api
type API interface {
	MakePost(p model.Post)
}
