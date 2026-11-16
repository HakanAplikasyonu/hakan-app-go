package entegrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
)

// dateFieldCreated: go-ubltr listings bound the creation / receive time
// (provider.ListQuery.From/To), which 30 §7 prefers over the invoice date.
const dateFieldCreated = "created"

// CodeLimitReached marks a one-day window whose listing still filled the
// LIMIT of an integrator without paging (36 §5.3).
const CodeLimitReached = "LIMIT_REACHED"

// istanbul is the calendar month boundaries are drawn in.
var istanbul = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return l
	}
	return time.FixedZone("TRT", 3*60*60)
}()

// window is one planned listing range.
type window struct {
	Kind     provider.DocumentKind
	Dir      provider.Direction
	From, To time.Time
}

// item is the sync_windows.item value: "<doc_kind>:<direction>". The
// direction is part of it because the table's unique key has no direction.
func (w window) item() string { return docKind(w.Kind) + ":" + direction(w.Dir) }

func parseItem(s string) (provider.DocumentKind, provider.Direction, error) {
	k, d, ok := strings.Cut(s, ":")
	if !ok || (d != "incoming" && d != "outgoing") {
		return "", "", fmt.Errorf("entegrator: bad window item %q", s)
	}
	kind, err := providerKind(k)
	return kind, providerDirection(d), err
}

// plan builds the windows of a run kind for every listable kind/direction.
func (s *Service) plan(c *Client, kind string, now time.Time) []window {
	now = now.Truncate(time.Minute)
	var ranges [][2]time.Time
	switch kind {
	case RunBackfill:
		local := now.In(istanbul)
		ranges = months(time.Date(local.Year()-1, 1, 1, 0, 0, 0, 0, istanbul), now)
	case RunFullScan:
		local := now.In(istanbul)
		ranges = months(time.Date(local.Year(), local.Month()-1, 1, 0, 0, 0, 0, istanbul), now)
	default: // incremental, manual
		ranges = [][2]time.Time{{now.Add(-s.cfg.IncrementalLookback), now}}
	}
	var out []window
	for _, k := range []provider.DocumentKind{provider.EInvoice, provider.EArchive} {
		for _, d := range []provider.Direction{provider.Inbound, provider.Outbound} {
			op := provider.OpListInbox
			if d == provider.Outbound {
				op = provider.OpListOutbox
			}
			if !c.Supports(k, op) {
				continue
			}
			for _, r := range ranges {
				out = append(out, window{Kind: k, Dir: d, From: r[0], To: r[1]})
			}
		}
	}
	return out
}

// months splits [from, to) at calendar month starts.
func months(from, to time.Time) [][2]time.Time {
	var out [][2]time.Time
	for start := from; start.Before(to); {
		end := time.Date(start.Year(), start.Month()+1, 1, 0, 0, 0, 0, start.Location())
		if end.After(to) {
			end = to
		}
		out = append(out, [2]time.Time{start, end})
		start = end
	}
	return out
}

// job is the state of one run.
type job struct {
	s        *Service
	c        *Client
	conn     db.SourceConnection
	ownTaxID string
	cnt      *Counters
	planned  []window
	last     time.Time
	lastList time.Time
}

// throttle keeps RequestGap between integrator calls.
func (j *job) throttle(ctx context.Context) error {
	gap := j.s.cfg.RequestGap
	if gap > 0 && !j.last.IsZero() {
		if wait := gap - time.Since(j.last); wait > 0 {
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
		}
	}
	j.last = time.Now()
	return nil
}

// throttleList keeps the profile's ListGap between listing calls. A
// negative Config.RequestGap (tests) turns all pacing off.
func (j *job) throttleList(ctx context.Context, gap time.Duration) error {
	if gap <= 0 || j.s.cfg.RequestGap < 0 {
		return nil
	}
	if !j.lastList.IsZero() {
		if wait := gap - time.Since(j.lastList); wait > 0 {
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
		}
	}
	j.lastList = time.Now()
	return nil
}

// listWindows records the planned windows, then lists every resumable
// window of the connection (planned now or left over by earlier runs).
func (j *job) listWindows(ctx context.Context, planned []window) error {
	j.planned = planned
	q := j.s.q
	for _, w := range planned {
		if _, err := q.PlanSyncWindow(ctx, db.PlanSyncWindowParams{
			TaxpayerID: j.conn.TaxpayerID, ConnectionID: j.conn.ID, Item: w.item(),
			Direction: repository.Text(direction(w.Dir)), DateField: dateFieldCreated,
			RangeStart: ts(w.From), RangeEnd: ts(w.To),
		}); err != nil {
			return err
		}
	}
	queue, err := q.ListResumableSyncWindows(ctx, db.ListResumableSyncWindowsParams{
		TaxpayerID: j.conn.TaxpayerID, ConnectionID: j.conn.ID, MaxAttempts: int32(j.s.cfg.MaxWindowAttempts),
	})
	if err != nil {
		return err
	}
	for len(queue) > 0 {
		w := queue[0]
		queue = queue[1:]
		children, err := j.runWindow(ctx, w)
		if err != nil {
			if abortsRun(ctx, err) {
				return err
			}
			j.s.log.Warn("entegrator: window failed", "connection", j.conn.Uuid, "item", w.Item, "err", err)
			continue
		}
		queue = append(children, queue...)
	}
	return nil
}

// runWindow lists one window page by page. A window that is still paging
// after MaxPagesPerWindow pages and is wider than MinWindow is split in two
// halves, which are returned to be listed next (37 §5.3); what was already
// indexed stays (upserts are idempotent).
func (j *job) runWindow(ctx context.Context, w db.SyncWindow) ([]db.SyncWindow, error) {
	q := j.s.q
	kind, dir, err := parseItem(w.Item)
	if err != nil {
		return nil, err
	}
	setStatus := func(status string, cursor pgtype.Text, errJSON []byte) error {
		_, err := q.SetSyncWindowStatus(ctx, db.SetSyncWindowStatusParams{
			TaxpayerID: w.TaxpayerID, ID: w.ID, Status: status, PageCursor: cursor, LastError: errJSON,
		})
		return err
	}
	if err := setStatus("running", w.PageCursor, nil); err != nil {
		return nil, err
	}
	from, to := w.RangeStart.Time, w.RangeEnd.Time
	prof := listProfile(j.c.Name())
	limit := j.s.cfg.PageSize
	if prof.PageSize > 0 {
		limit = prof.PageSize
	}
	// Windows are [from, to); listings take inclusive bounds, often as
	// dates, so the end moves back by a nanosecond and both are given in
	// Turkish time.
	qFrom, qTo := from.In(istanbul), to.Add(-time.Nanosecond).In(istanbul)
	if prof.EndDateOnly {
		qTo = time.Date(qTo.Year(), qTo.Month(), qTo.Day()+1, 0, 0, 0, 0, istanbul)
	}
	cursor := w.PageCursor.String
	for pages := 1; ; pages++ {
		if err := j.throttleList(ctx, prof.ListGap); err != nil {
			return nil, err
		}
		if err := j.throttle(ctx); err != nil {
			return nil, err
		}
		cctx, cancel := context.WithTimeout(ctx, j.s.cfg.CallTimeout)
		page, err := j.c.List(cctx, dir, ListQuery{Kind: kind, From: qFrom, To: qTo, Cursor: cursor, Limit: limit})
		cancel()
		if err != nil {
			j.cnt.WindowsFailed++
			_ = setStatus("failed", repository.Text(cursor), errorJSON(err))
			return nil, err
		}
		j.cnt.Pages++
		for _, it := range page.Items {
			if err := j.index(ctx, kind, dir, it); err != nil {
				return nil, err
			}
		}
		wide := to.Sub(from) > j.s.cfg.MinWindow
		full := prof.NoCursor && page.Next == "" && limit > 0 && len(page.Items) >= limit
		switch {
		case full && wide:
			return j.split(ctx, w, setStatus)
		case full:
			// One day still fills the LIMIT: documents may be missing.
			j.cnt.Windows++
			j.cnt.LimitReached++
			info, _ := json.Marshal(map[string]any{"code": CodeLimitReached, "limit": limit})
			return nil, setStatus("done", pgtype.Text{}, info)
		case page.Next == "":
			j.cnt.Windows++
			return nil, setStatus("done", pgtype.Text{}, nil)
		case pages >= j.s.cfg.MaxPagesPerWindow && wide:
			return j.split(ctx, w, setStatus)
		}
		cursor = page.Next
		if _, err := q.SetSyncWindowCursor(ctx, db.SetSyncWindowCursorParams{TaxpayerID: w.TaxpayerID, ID: w.ID, PageCursor: repository.Text(cursor)}); err != nil {
			return nil, err
		}
	}
}

func (j *job) split(ctx context.Context, w db.SyncWindow, setStatus func(string, pgtype.Text, []byte) error) ([]db.SyncWindow, error) {
	from, to := w.RangeStart.Time, w.RangeEnd.Time
	mid := from.Add(to.Sub(from) / 2).Truncate(time.Hour)
	if !mid.After(from) {
		mid = from.Add(to.Sub(from) / 2)
	}
	var out []db.SyncWindow
	for _, r := range [][2]time.Time{{from, mid}, {mid, to}} {
		child, err := j.s.q.PlanSyncWindow(ctx, db.PlanSyncWindowParams{
			TaxpayerID: w.TaxpayerID, ConnectionID: w.ConnectionID, Item: w.Item, Direction: w.Direction,
			DateField: w.DateField, RangeStart: ts(r[0]), RangeEnd: ts(r[1]),
		})
		if err != nil {
			return nil, err
		}
		out = append(out, child)
	}
	j.cnt.Splits++
	j.cnt.Windows++
	split, _ := json.Marshal(map[string]any{"split_at": mid})
	return out, setStatus("done", pgtype.Text{}, split)
}

// index writes one listed document to entegrator_documents (37 §5.2) and
// carries a status change to its invoice.
func (j *job) index(ctx context.Context, kind provider.DocumentKind, dir provider.Direction, it provider.DocumentInfo) error {
	j.cnt.Listed++
	if it.Kind != "" {
		kind = it.Kind
	}
	dk := docKind(kind)
	ettn := strings.ToLower(strings.TrimSpace(it.UUID))
	if dk == "" || ettn == "" {
		j.cnt.Skipped++
		return nil
	}
	n := NormalizeStatus(j.c.Name(), it)
	header, _ := json.Marshal(it)
	var issue pgtype.Date
	if d, err := time.Parse(time.DateOnly, it.IssueDate); err == nil {
		issue = pgtype.Date{Time: d, Valid: true}
	}
	var created pgtype.Timestamptz
	if !it.CreatedAt.IsZero() {
		created = ts(it.CreatedAt)
	}
	warnings := n.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	raw := RawStatus(it)
	row, err := j.s.q.UpsertEntegratorDocument(ctx, db.UpsertEntegratorDocumentParams{
		TaxpayerID: j.conn.TaxpayerID, ConnectionID: j.conn.ID, Ettn: ettn, DocKind: dk, Direction: direction(dir),
		InvoiceNumber: repository.Text(it.ID), IssueDate: issue, CreatedAtSource: created,
		StatusRaw: raw, StatusNormalized: n.Status, Warnings: warnings, HeaderRaw: header,
	})
	if err != nil {
		return err
	}
	if !row.Existed {
		j.cnt.New++
		return nil
	}
	if row.PrevStatusRaw != raw {
		j.cnt.StatusChanged++
		if row.InvoiceID.Valid {
			if _, err := j.s.q.SetInvoiceStatusNormalized(ctx, db.SetInvoiceStatusNormalizedParams{
				TaxpayerID: j.conn.TaxpayerID, ID: row.InvoiceID.Int64, StatusNormalized: n.Status,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// markMissing flags documents of the scanned windows that this full scan
// did not see (37 §5.5). Nothing is deleted. Skipped when a window failed,
// since its documents were not listed.
func (j *job) markMissing(ctx context.Context, startedAt time.Time) error {
	if j.cnt.WindowsFailed > 0 {
		return nil
	}
	for _, w := range j.planned {
		n, err := j.s.q.MarkEntegratorDocumentsMissing(ctx, db.MarkEntegratorDocumentsMissingParams{
			TaxpayerID: j.conn.TaxpayerID, ConnectionID: j.conn.ID, Direction: direction(w.Dir), DocKind: docKind(w.Kind),
			RangeStart: ts(w.From), RangeEnd: ts(w.To), SeenBefore: ts(startedAt),
		})
		if err != nil {
			return err
		}
		j.cnt.Missing += int(n)
	}
	return nil
}
