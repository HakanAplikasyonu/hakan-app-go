package egress

import (
	"net/http"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

// StatusHandler serves GET /v1/platform/egress: the fixed-egress setup and
// the last IP check the egress worker stored.
func StatusHandler(cfg Config, store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, err := Describe(r.Context(), cfg, store, time.Now())
		if err != nil {
			response.InternalErr(w, r, err, "")
			return
		}
		response.JSON(w, r, http.StatusOK, v)
	})
}
