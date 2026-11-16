package tenant

import (
	"net/http"
	"strings"

	orgusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/response"
)

type roleList struct {
	Items []orgusecase.TenantRole `json:"items"`
}

type permissionList struct {
	Items []orgusecase.TenantPermission `json:"items"`
}

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	items, err := h.svc.ListRoles(r.Context(), s.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, roleList{Items: items})
}

func (h *Handler) listRolePermissions(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.ListRolePermissions(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, permissionList{Items: items})
}

type roleRequest struct {
	Key         string    `json:"key"`
	Name        *string   `json:"name"`
	Description *string   `json:"description"`
	Permissions *[]string `json:"permissions"`
}

func (h *Handler) createRole(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	var in roleRequest
	if !decode(w, r, &in, 16<<10) {
		return
	}
	role, err := h.svc.CreateRole(r.Context(), s.ID, orgusecase.RoleInput{
		Key: in.Key, Name: in.Name, Description: in.Description, Permissions: in.Permissions,
		Grantor: grantor(s),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, s, "organizations.role_created", map[string]any{"role_key": role.Key, "permissions": role.Permissions})
	response.JSON(w, r, http.StatusCreated, role)
}

func (h *Handler) patchRole(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	key := strings.TrimSpace(r.PathValue("roleKey"))
	var in struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Permissions *[]string `json:"permissions"`
	}
	if !decode(w, r, &in, 16<<10) {
		return
	}
	role, err := h.svc.UpdateRole(r.Context(), s.ID, orgusecase.RoleInput{
		Key: key, Name: in.Name, Description: in.Description, Permissions: in.Permissions,
		Grantor: grantor(s),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, s, "organizations.role_updated", map[string]any{"role_key": role.Key, "permissions": role.Permissions})
	response.JSON(w, r, http.StatusOK, role)
}

func (h *Handler) deleteRole(w http.ResponseWriter, r *http.Request) {
	s := orgctx.Must(r.Context())
	key := strings.TrimSpace(r.PathValue("roleKey"))
	if err := h.svc.DeleteRole(r.Context(), s.ID, key); err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, s, "organizations.role_deleted", map[string]any{"role_key": key})
	w.WriteHeader(http.StatusNoContent)
}
