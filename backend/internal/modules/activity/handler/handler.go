package handler

import (
	"net/http"

	activityusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/activity/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/resourcemeta"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/apiquery"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

type Handler struct {
	svc *activityusecase.Service
}

func New(svc *activityusecase.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Meta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, resourcemeta.Activity())
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := apiquery.Parse(r.URL.Query())
	var actorID *int64
	items, total, err := h.svc.List(r.Context(), q, actorID, r.URL.Query().Get("resource"), r.URL.Query().Get("action"), q.Q)
	if err != nil {
		response.InternalErr(w, r, err, "failed to list activity")
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}
