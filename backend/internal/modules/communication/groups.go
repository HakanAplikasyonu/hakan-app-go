package communication

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

// Mükellef grupları (#780 B): ofisin ad verdiği sabit mükellef listeleri.
// Toplu gönderimde hedef olarak seçilir.

// MaxGroupMembers caps a group (a bulk send takes at most 500 too).
const MaxGroupMembers = MaxBulkRecipients

// Group is a saved taxpayer list.
type Group struct {
	UUID        uuid.UUID     `json:"uuid"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	MemberCount int           `json:"member_count"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	Members     []GroupMember `json:"members"`
}

// GroupMember is one taxpayer of a group.
type GroupMember struct {
	EngagementUUID uuid.UUID `json:"engagement_uuid"`
	Title          string    `json:"title"`
	TaxID          string    `json:"tax_id"`
}

// GroupInput creates or replaces a group (members replaced as a whole).
type GroupInput struct {
	Name            string      `json:"name"`
	Description     string      `json:"description"`
	EngagementUUIDs []uuid.UUID `json:"engagement_uuids"`
}

func (in *GroupInput) clean() error {
	in.Name, in.Description = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description)
	switch {
	case in.Name == "":
		return fmt.Errorf("%w: grup adı boş olamaz", ErrInvalidInput)
	case len([]rune(in.Name)) > 80:
		return fmt.Errorf("%w: grup adı en fazla 80 karakter olabilir", ErrInvalidInput)
	case len([]rune(in.Description)) > 500:
		return fmt.Errorf("%w: açıklama en fazla 500 karakter olabilir", ErrInvalidInput)
	case len(in.EngagementUUIDs) > MaxGroupMembers:
		return fmt.Errorf("%w: bir grupta en fazla %d mükellef olabilir", ErrInvalidInput, MaxGroupMembers)
	}
	if in.EngagementUUIDs == nil {
		in.EngagementUUIDs = []uuid.UUID{}
	}
	return nil
}

func groupOf(g db.TaxpayerGroup, count int) Group {
	return Group{UUID: g.Uuid, Name: g.Name, Description: g.Description, MemberCount: count,
		CreatedAt: g.CreatedAt.Time, UpdatedAt: g.UpdatedAt.Time, Members: []GroupMember{}}
}

func duplicateName(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

// ListGroups lists the office's groups with their active member counts.
func (s *Service) ListGroups(ctx context.Context, org int64) ([]Group, error) {
	rows, err := s.q.ListTaxpayerGroups(ctx, org)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	out := make([]Group, 0, len(rows))
	for _, r := range rows {
		out = append(out, groupOf(db.TaxpayerGroup{Uuid: r.Uuid, Name: r.Name, Description: r.Description,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}, int(r.MemberCount)))
	}
	return out, nil
}

// GetGroup returns one group with its members.
func (s *Service) GetGroup(ctx context.Context, org int64, id uuid.UUID) (Group, error) {
	g, err := s.q.GetTaxpayerGroup(ctx, db.GetTaxpayerGroupParams{OrganizationID: org, Uuid: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrNotFound
	} else if err != nil {
		return Group{}, fmt.Errorf("get group: %w", err)
	}
	return s.withMembers(ctx, s.q, g)
}

func (s *Service) withMembers(ctx context.Context, q *db.Queries, g db.TaxpayerGroup) (Group, error) {
	rows, err := q.ListTaxpayerGroupMembers(ctx, db.ListTaxpayerGroupMembersParams{OrganizationID: g.OrganizationID, GroupID: g.ID})
	if err != nil {
		return Group{}, fmt.Errorf("group members: %w", err)
	}
	out := groupOf(g, len(rows))
	for _, r := range rows {
		out.Members = append(out.Members, GroupMember{EngagementUUID: r.EngagementUuid, Title: r.Title, TaxID: r.TaxID})
	}
	return out, nil
}

// CreateGroup saves a new group; unknown or ended engagements are ignored.
func (s *Service) CreateGroup(ctx context.Context, org, actor int64, in GroupInput) (Group, error) {
	if err := in.clean(); err != nil {
		return Group{}, err
	}
	var out Group
	err := database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		g, err := q.CreateTaxpayerGroup(ctx, db.CreateTaxpayerGroupParams{OrganizationID: org, Name: in.Name,
			Description: in.Description, CreatedBy: actorID(actor)})
		if err != nil {
			return err
		}
		if _, err := q.AddTaxpayerGroupMembers(ctx, db.AddTaxpayerGroupMembersParams{GroupID: g.ID, OrganizationID: org,
			EngagementUuids: in.EngagementUUIDs}); err != nil {
			return err
		}
		out, err = s.withMembers(ctx, q, g)
		return err
	})
	if duplicateName(err) {
		return Group{}, fmt.Errorf("%w: bu adla bir grup zaten var", ErrInvalidInput)
	}
	if err != nil {
		return Group{}, fmt.Errorf("create group: %w", err)
	}
	return out, nil
}

// UpdateGroup renames a group and replaces its members.
func (s *Service) UpdateGroup(ctx context.Context, org int64, id uuid.UUID, in GroupInput) (Group, error) {
	if err := in.clean(); err != nil {
		return Group{}, err
	}
	var out Group
	err := database.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		g, err := q.UpdateTaxpayerGroup(ctx, db.UpdateTaxpayerGroupParams{Name: in.Name, Description: in.Description,
			OrganizationID: org, Uuid: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if err := q.ClearTaxpayerGroupMembers(ctx, db.ClearTaxpayerGroupMembersParams{OrganizationID: org, GroupID: g.ID}); err != nil {
			return err
		}
		if _, err := q.AddTaxpayerGroupMembers(ctx, db.AddTaxpayerGroupMembersParams{GroupID: g.ID, OrganizationID: org,
			EngagementUuids: in.EngagementUUIDs}); err != nil {
			return err
		}
		out, err = s.withMembers(ctx, q, g)
		return err
	})
	switch {
	case errors.Is(err, ErrNotFound):
		return Group{}, ErrNotFound
	case duplicateName(err):
		return Group{}, fmt.Errorf("%w: bu adla bir grup zaten var", ErrInvalidInput)
	case err != nil:
		return Group{}, fmt.Errorf("update group: %w", err)
	}
	return out, nil
}

// DeleteGroup removes a group (the taxpayers stay).
func (s *Service) DeleteGroup(ctx context.Context, org int64, id uuid.UUID) error {
	n, err := s.q.DeleteTaxpayerGroup(ctx, db.DeleteTaxpayerGroupParams{OrganizationID: org, Uuid: id})
	if err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Tax return kinds a dynamic filter can target, with their screen names.
// Geçici vergi has no tax return records yet, so it is not offered.
var filterTaxKinds = []struct{ Code, Label string }{
	{"kdv1", "KDV beyannamesi veren mükellefler"},
	{"muhsgk", "Muhtasar ve prim hizmet beyannamesi veren mükellefler"},
}

func validTaxKind(k string) bool {
	for _, x := range filterTaxKinds {
		if x.Code == k {
			return true
		}
	}
	return false
}

// FilterOption is one dynamic filter value.
type FilterOption struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// AssigneeOption is one office member taxpayers are assigned to.
type AssigneeOption struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Count int       `json:"count"`
}

// FilterOptions are the dynamic filters the office can target.
//
// TODO(#780 B): defter türü (bilanço / işletme / serbest meslek) süzgeci,
// defter türü sütunu ana dala girince eklenecek; etiket süzgeci de mükellef
// etiketleri olmadığı için yok.
type FilterOptions struct {
	TaxKinds  []FilterOption   `json:"tax_kinds"`
	Assignees []AssigneeOption `json:"assignees"`
}

// sincePeriod: tax returns of the last 12 months count for a tax kind.
func (s *Service) sincePeriod() string {
	return s.now().AddDate(-1, 0, 0).Format("2006-01")
}

// Filters returns the dynamic filter values with taxpayer counts.
func (s *Service) Filters(ctx context.Context, org int64) (FilterOptions, error) {
	kinds, err := s.q.CountEngagementsByTaxKind(ctx, db.CountEngagementsByTaxKindParams{OrganizationID: org, SincePeriod: s.sincePeriod()})
	if err != nil {
		return FilterOptions{}, fmt.Errorf("filter tax kinds: %w", err)
	}
	counts := map[string]int{}
	for _, k := range kinds {
		counts[k.Kind] = int(k.Engagements)
	}
	out := FilterOptions{TaxKinds: []FilterOption{}, Assignees: []AssigneeOption{}}
	for _, k := range filterTaxKinds {
		out.TaxKinds = append(out.TaxKinds, FilterOption{Code: k.Code, Label: k.Label, Count: counts[k.Code]})
	}
	people, err := s.q.CountEngagementsByAssignee(ctx, org)
	if err != nil {
		return FilterOptions{}, fmt.Errorf("filter assignees: %w", err)
	}
	for _, p := range people {
		name := strings.TrimSpace(p.Name + " " + p.Surname)
		if name == "" {
			name = p.Email
		}
		out.Assignees = append(out.Assignees, AssigneeOption{UUID: p.UserUuid, Name: name, Count: int(p.Engagements)})
	}
	return out, nil
}
