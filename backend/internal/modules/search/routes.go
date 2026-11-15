package search

import (
	searchhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/search/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
)

// Module mounts the command-palette search endpoints (any signed-in session).
type Module struct{ H *searchhandler.Handler }

func (Module) Name() string { return "search" }

func (m Module) Routes(r *module.Router) {
	r.Session("GET /v1/search/specs", m.H.Specs)
	r.Session("GET /v1/search", m.H.Search)
}
