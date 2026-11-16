package petitions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
)

// ErrNotFound: unknown petition in this engagement.
var ErrNotFound = errors.New("petitions: not found")

// Service fills templates from an engagement and stores petitions. Every
// database call runs under Postgres RLS (rls.FromRequest), so a petition
// can only be read in the engagement that made it.
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// New builds the service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool, now: time.Now} }

// Petition is a stored petition.
type Petition struct {
	UUID            uuid.UUID `json:"uuid"`
	TemplateKey     string    `json:"template_key"`
	TemplateVersion int       `json:"template_version"`
	Draft           bool      `json:"draft"`
	CreatedAt       time.Time `json:"created_at"`
	CreatedBy       *string   `json:"created_by"`
	Rendered        Rendered  `json:"rendered"`
}

// Input is a petition request.
type Input struct {
	TemplateKey string            `json:"template_key"`
	Fields      map[string]string `json:"fields"`
}

func (s *Service) scoped(ctx context.Context, fn func(q *db.Queries) error) error {
	return rls.WithScope(ctx, s.pool, rls.FromRequest(ctx), func(ctx context.Context, tx pgx.Tx) error {
		return fn(db.New(tx))
	})
}

// Preview fills the template without storing anything.
func (s *Service) Preview(ctx context.Context, e engagementctx.Scope, userID int64, in Input) (Rendered, error) {
	var out Rendered
	err := s.scoped(ctx, func(q *db.Queries) error {
		base, err := s.base(ctx, q, e, userID)
		if err != nil {
			return err
		}
		out, _, err = Render(in.TemplateKey, in.Fields, base, s.now())
		return err
	})
	return out, err
}

// Create fills the template and stores the petition with the exact text.
func (s *Service) Create(ctx context.Context, e engagementctx.Scope, userID int64, in Input) (Petition, error) {
	var out Petition
	err := s.scoped(ctx, func(q *db.Queries) error {
		base, err := s.base(ctx, q, e, userID)
		if err != nil {
			return err
		}
		r, fields, err := Render(in.TemplateKey, in.Fields, base, s.now())
		if err != nil {
			return err
		}
		fj, _ := json.Marshal(fields)
		rj, _ := json.Marshal(r)
		row, err := q.CreatePetition(ctx, db.CreatePetitionParams{
			OrganizationID: e.OrganizationID, EngagementID: e.ID, TemplateKey: r.TemplateKey,
			TemplateVersion: intconv.Int32(r.TemplateVersion), Fields: fj, Rendered: rj, Draft: r.Draft,
			CreatedBy: pgtype.Int8{Int64: userID, Valid: userID > 0},
		})
		if err != nil {
			return err
		}
		out = Petition{UUID: row.Uuid, TemplateKey: row.TemplateKey, TemplateVersion: int(row.TemplateVersion),
			Draft: row.Draft, CreatedAt: row.CreatedAt.Time, Rendered: r}
		return nil
	})
	return out, err
}

// List returns the engagement's petitions, newest first.
func (s *Service) List(ctx context.Context, e engagementctx.Scope, limit, offset int32) ([]Petition, error) {
	var out []Petition
	err := s.scoped(ctx, func(q *db.Queries) error {
		rows, err := q.ListPetitions(ctx, db.ListPetitionsParams{
			OrganizationID: e.OrganizationID, EngagementID: e.ID, LimitCount: limit, OffsetCount: offset,
		})
		if err != nil {
			return err
		}
		out = make([]Petition, 0, len(rows))
		for _, r := range rows {
			p := Petition{UUID: r.Uuid, TemplateKey: r.TemplateKey, TemplateVersion: int(r.TemplateVersion),
				Draft: r.Draft, CreatedAt: r.CreatedAt.Time}
			_ = json.Unmarshal(r.Rendered, &p.Rendered)
			if n := strings.TrimSpace(r.CreatorName.String + " " + r.CreatorSurname.String); n != "" {
				p.CreatedBy = &n
			}
			out = append(out, p)
		}
		return nil
	})
	return out, err
}

// Get returns one petition of the engagement.
func (s *Service) Get(ctx context.Context, e engagementctx.Scope, id uuid.UUID) (Petition, error) {
	var out Petition
	err := s.scoped(ctx, func(q *db.Queries) error {
		row, err := q.GetPetition(ctx, db.GetPetitionParams{OrganizationID: e.OrganizationID, EngagementID: e.ID, Uuid: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		out = Petition{UUID: row.Uuid, TemplateKey: row.TemplateKey, TemplateVersion: int(row.TemplateVersion),
			Draft: row.Draft, CreatedAt: row.CreatedAt.Time}
		return json.Unmarshal(row.Rendered, &out.Rendered)
	})
	return out, err
}

// base loads the mükellef, office and signer fields.
func (s *Service) base(ctx context.Context, q *db.Queries, e engagementctx.Scope, userID int64) (Data, error) {
	c, err := q.GetPetitionContext(ctx, db.GetPetitionContextParams{OrganizationID: e.OrganizationID, EngagementID: e.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Data{}, ErrNotFound
	}
	if err != nil {
		return Data{}, err
	}
	d := Data{
		Taxpayer: Taxpayer{Title: c.Title, TaxID: c.TaxID, TaxIDType: "VKN", TaxOffice: c.TaxOffice},
		Office:   Office{Name: c.OfficeName},
	}
	if len(c.TaxID) == 11 {
		d.Taxpayer.TaxIDType = "TCKN"
	}
	if userID > 0 {
		if u, err := q.GetUserByID(ctx, userID); err == nil {
			d.Signer = Signer{Name: strings.TrimSpace(u.Name + " " + u.Surname), SMMMRegistryNo: u.SmmmRegistryNo.String}
		}
	}
	return d, nil
}
