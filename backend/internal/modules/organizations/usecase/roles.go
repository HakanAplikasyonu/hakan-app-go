package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/organizations/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Office role errors (issue #5).
var (
	ErrRoleIsSystem = errors.New("system roles cannot be deleted")
	ErrRoleInUse    = errors.New("role is held by a member or an open invitation")
	ErrRoleKeyTaken = errors.New("role key already exists in this organization")
	ErrRolesOff     = errors.New("office roles are not configured")
)

// RoleRepository is the storage of office roles. *repository.Postgres
// implements it.
type RoleRepository interface {
	ListOrganizationRoles(ctx context.Context, orgID int64) ([]db.ListOrganizationRolesRow, error)
	GetOrganizationRoleByKey(ctx context.Context, q *db.Queries, orgID int64, key string) (db.Role, error)
	ListTenantPermissions(ctx context.Context) ([]db.ListTenantPermissionsRow, error)
	CreateOrganizationRole(ctx context.Context, q *db.Queries, params db.CreateOrganizationRoleParams) (db.Role, error)
	UpdateOrganizationRole(ctx context.Context, q *db.Queries, params db.UpdateOrganizationRoleParams) (db.Role, error)
	ReplaceRolePermissions(ctx context.Context, q *db.Queries, orgID, roleID int64, slugs []string) error
	CountOrganizationRoleUsage(ctx context.Context, q *db.Queries, orgID, roleID int64) (int64, error)
	DeleteOrganizationRole(ctx context.Context, q *db.Queries, orgID, roleID int64) (int64, error)
}

// TenantRole is an office role as the API shows it.
type TenantRole struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IsSystem    bool     `json:"is_system"`
	Permissions []string `json:"permissions"`
	MemberCount int64    `json:"member_count"`
}

// TenantPermission is a permission an office role can grant.
type TenantPermission struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// RoleInput creates or patches an office role. Nil fields stay unchanged on
// patch.
type RoleInput struct {
	Key         string
	Name        *string
	Description *string
	Permissions *[]string
}

var roleKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

func (s *Service) roleRepo() (RoleRepository, error) {
	r, ok := s.repo.(RoleRepository)
	if !ok {
		return nil, ErrRolesOff
	}
	return r, nil
}

// ListRoles returns the office's roles (template copies first).
func (s *Service) ListRoles(ctx context.Context, orgID int64) ([]TenantRole, error) {
	rr, err := s.roleRepo()
	if err != nil {
		return nil, err
	}
	rows, err := rr.ListOrganizationRoles(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]TenantRole, 0, len(rows))
	for _, r := range rows {
		perms := r.Permissions
		if perms == nil {
			perms = []string{}
		}
		out = append(out, TenantRole{
			Key: r.Slug, Name: r.Name, Description: r.Description, IsSystem: r.IsSystem,
			Permissions: perms, MemberCount: r.MemberCount,
		})
	}
	return out, nil
}

// ListRolePermissions returns every tenant.* permission.
func (s *Service) ListRolePermissions(ctx context.Context) ([]TenantPermission, error) {
	rr, err := s.roleRepo()
	if err != nil {
		return nil, err
	}
	rows, err := rr.ListTenantPermissions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TenantPermission, 0, len(rows))
	for _, r := range rows {
		out = append(out, TenantPermission{Slug: r.Slug, Name: r.Name})
	}
	return out, nil
}

// CreateRole adds a custom role to the office.
func (s *Service) CreateRole(ctx context.Context, orgID int64, in RoleInput) (TenantRole, error) {
	rr, err := s.roleRepo()
	if err != nil {
		return TenantRole{}, err
	}
	key := strings.TrimSpace(in.Key)
	if !roleKeyRe.MatchString(key) {
		return TenantRole{}, fmt.Errorf("%w: key must match ^[a-z][a-z0-9_]{1,31}$", ErrInvalidRequest)
	}
	name, desc, err := roleText(in)
	if err != nil {
		return TenantRole{}, err
	}
	if name == "" {
		return TenantRole{}, fmt.Errorf("%w: name is required", ErrInvalidRequest)
	}
	if in.Permissions == nil {
		return TenantRole{}, fmt.Errorf("%w: permissions is required", ErrInvalidRequest)
	}
	perms, err := s.checkPermissions(ctx, rr, *in.Permissions)
	if err != nil {
		return TenantRole{}, err
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		role, err := rr.CreateOrganizationRole(ctx, q, db.CreateOrganizationRoleParams{
			Name: name, RoleKey: key, Description: pgtype.Text{String: desc, Valid: true},
			OrganizationID: pgtype.Int8{Int64: orgID, Valid: true},
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return ErrRoleKeyTaken
			}
			return err
		}
		return rr.ReplaceRolePermissions(ctx, q, orgID, role.ID, perms)
	})
	if err != nil {
		return TenantRole{}, err
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, orgID)
	return s.getRole(ctx, orgID, key)
}

// UpdateRole patches name, description and/or permissions of an office role.
// The owner role always keeps every permission.
func (s *Service) UpdateRole(ctx context.Context, orgID int64, in RoleInput) (TenantRole, error) {
	rr, err := s.roleRepo()
	if err != nil {
		return TenantRole{}, err
	}
	key := strings.TrimSpace(in.Key)
	name, desc, err := roleText(in)
	if err != nil {
		return TenantRole{}, err
	}
	if in.Name != nil && name == "" {
		return TenantRole{}, fmt.Errorf("%w: name cannot be empty", ErrInvalidRequest)
	}
	var perms []string
	if in.Permissions != nil {
		if key == orgctx.RoleOwner {
			return TenantRole{}, fmt.Errorf("%w: the owner role keeps every permission", ErrInvalidRequest)
		}
		if perms, err = s.checkPermissions(ctx, rr, *in.Permissions); err != nil {
			return TenantRole{}, err
		}
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		params := db.UpdateOrganizationRoleParams{OrganizationID: pgtype.Int8{Int64: orgID, Valid: true}, RoleKey: key}
		if in.Name != nil {
			params.Name = pgtype.Text{String: name, Valid: true}
		}
		if in.Description != nil {
			params.Description = pgtype.Text{String: desc, Valid: true}
		}
		role, err := rr.UpdateOrganizationRole(ctx, q, params)
		if err != nil {
			return mapRepoNotFound(err, ErrRoleNotFound)
		}
		if in.Permissions != nil {
			return rr.ReplaceRolePermissions(ctx, q, orgID, role.ID, perms)
		}
		return nil
	})
	if err != nil {
		return TenantRole{}, err
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, orgID)
	return s.getRole(ctx, orgID, key)
}

// DeleteRole removes a custom role nobody holds.
func (s *Service) DeleteRole(ctx context.Context, orgID int64, key string) error {
	rr, err := s.roleRepo()
	if err != nil {
		return err
	}
	err = s.repo.WithTx(ctx, func(ctx context.Context, q *db.Queries, _ pgx.Tx) error {
		if err := s.repo.LockOrganizationRow(ctx, q, orgID); err != nil {
			return err
		}
		role, err := rr.GetOrganizationRoleByKey(ctx, q, orgID, strings.TrimSpace(key))
		if err != nil {
			return mapRepoNotFound(err, ErrRoleNotFound)
		}
		if role.IsSystem {
			return ErrRoleIsSystem
		}
		n, err := rr.CountOrganizationRoleUsage(ctx, q, orgID, role.ID)
		if err != nil {
			return err
		}
		if n > 0 {
			return ErrRoleInUse
		}
		_, err = rr.DeleteOrganizationRole(ctx, q, orgID, role.ID)
		return err
	})
	if err != nil {
		return err
	}
	_ = orgctx.Invalidate(ctx, s.invalidator, orgID)
	return nil
}

func (s *Service) getRole(ctx context.Context, orgID int64, key string) (TenantRole, error) {
	roles, err := s.ListRoles(ctx, orgID)
	if err != nil {
		return TenantRole{}, err
	}
	for _, r := range roles {
		if r.Key == key {
			return r, nil
		}
	}
	return TenantRole{}, ErrRoleNotFound
}

// checkPermissions keeps unique tenant.* slugs that exist.
func (s *Service) checkPermissions(ctx context.Context, rr RoleRepository, in []string) ([]string, error) {
	known, err := rr.ListTenantPermissions(ctx)
	if err != nil {
		return nil, err
	}
	valid := make(map[string]bool, len(known))
	for _, p := range known {
		valid[p.Slug] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if !valid[p] {
			return nil, fmt.Errorf("%w: unknown tenant permission %q", ErrInvalidRequest, p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

func roleText(in RoleInput) (name, desc string, err error) {
	if in.Name != nil {
		name = strings.Join(strings.Fields(*in.Name), " ")
		if utf8.RuneCountInString(name) > 100 {
			return "", "", fmt.Errorf("%w: name is too long (max 100)", ErrInvalidRequest)
		}
	}
	if in.Description != nil {
		desc = strings.TrimSpace(*in.Description)
		if utf8.RuneCountInString(desc) > 500 {
			return "", "", fmt.Errorf("%w: description is too long (max 500)", ErrInvalidRequest)
		}
	}
	return name, desc, nil
}

var _ RoleRepository = (*repository.Postgres)(nil)
