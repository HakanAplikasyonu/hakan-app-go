package connections

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/entegrator"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
)

// Queries is the db subset used here.
type Queries interface {
	ListSourceConnections(ctx context.Context, taxpayerID int64) ([]db.SourceConnection, error)
	AutoResolveOtherConnectionWarnings(ctx context.Context, arg db.AutoResolveOtherConnectionWarningsParams) (int64, error)
	ListConnectionWarningSubjects(ctx context.Context) ([]db.ListConnectionWarningSubjectsRow, error)
	ListOpenConnectionFailedWarnings(ctx context.Context, arg db.ListOpenConnectionFailedWarningsParams) ([]db.ListOpenConnectionFailedWarningsRow, error)
}

// Checker applies the rule to a taxpayer's connections.
type Checker struct {
	q      Queries
	engine *engine.Engine
	now    func() time.Time
}

// New builds a checker.
func New(q Queries, e *engine.Engine) *Checker {
	return &Checker{q: q, engine: e, now: time.Now}
}

// WithClock overrides the clock (tests).
func (c *Checker) WithClock(now func() time.Time) *Checker { c.now = now; return c }

var istanbul = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*60*60)
}()

// Period is the month a warning seen at t is written to.
func Period(t time.Time) string { return t.In(istanbul).Format("2006-01") }

// Check refreshes the connection warnings of one engagement: each
// connection's finding is raised or refreshed in the current month and a
// cleared condition auto-resolves; open warnings of earlier months, and of
// connections that no longer exist, are closed.
func (c *Checker) Check(ctx context.Context, s engine.Subject) (engine.Report, error) {
	rows, err := c.q.ListSourceConnections(ctx, s.TaxpayerID)
	if err != nil {
		return engine.Report{}, fmt.Errorf("connections: list: %w", err)
	}
	now := c.now()
	in := Input{Period: Period(now), Now: now, Connections: make([]Connection, 0, len(rows))}
	open, err := c.q.ListOpenConnectionFailedWarnings(ctx, db.ListOpenConnectionFailedWarningsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, Period: in.Period,
	})
	if err != nil {
		return engine.Report{}, fmt.Errorf("connections: open warnings: %w", err)
	}
	openParams := make(map[int64]map[string]any, len(open))
	for _, w := range open {
		if !w.EntityID.Valid {
			continue
		}
		p := map[string]any{}
		if len(w.Params) > 0 {
			if err := json.Unmarshal(w.Params, &p); err != nil {
				return engine.Report{}, fmt.Errorf("connections: warning params: %w", err)
			}
		}
		openParams[w.EntityID.Int64] = p
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		conn := FromRow(r)
		conn.OpenFailed = openParams[r.ID]
		in.Connections = append(in.Connections, conn)
		ids = append(ids, r.ID)
	}
	rep, err := engine.Run(ctx, c.engine, s, Rule{}, in)
	if err != nil {
		return rep, err
	}
	n, err := c.q.AutoResolveOtherConnectionWarnings(ctx, db.AutoResolveOtherConnectionWarningsParams{
		OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, ConnectionIds: ids, Period: in.Period,
	})
	if err != nil {
		return rep, fmt.Errorf("connections: resolve others: %w", err)
	}
	rep.AutoResolved += int(n)
	return rep, nil
}

// Sweep checks every active engagement whose taxpayer has a connection (or
// still has an open connection warning) across offices; the caller runs it
// with the row level security bypass (rls.System). Returns how many warnings
// were opened. One engagement's failure does not stop the others.
func (c *Checker) Sweep(ctx context.Context) (int, error) {
	subjects, err := c.q.ListConnectionWarningSubjects(ctx)
	if err != nil {
		return 0, fmt.Errorf("connections: subjects: %w", err)
	}
	created := 0
	var firstErr error
	for _, s := range subjects {
		rep, err := c.Check(ctx, engine.Subject{OrganizationID: s.OrganizationID, EngagementID: s.EngagementID, TaxpayerID: s.TaxpayerID})
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("engagement %d: %w", s.EngagementID, err)
			}
			continue
		}
		created += rep.Created
	}
	return created, firstErr
}

// FromRow maps a source_connections row.
func FromRow(r db.SourceConnection) Connection {
	c := Connection{
		ID: r.ID, SourceSystem: r.SourceSystem, SetupState: r.SetupState, Status: r.Status,
		LastError: r.LastError, CreatedAt: r.CreatedAt.Time,
	}
	if r.LastSuccessAt.Valid {
		t := r.LastSuccessAt.Time
		c.LastSuccessAt = &t
	}
	return c
}

var sourceNames = map[string]string{
	"parasut":        "Paraşüt",
	"kolaybi":        "KolayBi",
	"easytrade":      "Easytrade",
	"bizimhesap":     "BizimHesap",
	"logo_isbasi":    "Logo İşbaşı",
	"logo_tiger":     "Logo Tiger",
	"logo_go":        "Logo GO",
	"mikro":          "Mikro",
	"bank_vakifbank": "Vakıfbank",
}

// SourceName is the name the warning text shows for a source system: the
// integrator's name from its catalog, a known program or bank, else the raw
// system id.
func SourceName(system string) string {
	if n, ok := sourceNames[system]; ok {
		return n
	}
	if id, ok := strings.CutPrefix(system, entegrator.SourcePrefix); ok {
		for _, i := range entegrator.Catalog() {
			if i.ID == id {
				return i.Name
			}
		}
	}
	return system
}
