package frontend

import (
	"github.com/gorilla/mux"
)

// http middleware
type Middleware interface {
	SetupRoutes(m *mux.Router)
}
