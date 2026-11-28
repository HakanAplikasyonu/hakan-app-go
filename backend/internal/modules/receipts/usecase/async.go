package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/dbtx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// Background read of a mobile receipt (#780). The request only stores the
// photos and records the capture as StatusProcessing; the AI read, the draft
// and the checks run here, after the request's transaction has committed,
// in the same office scope the request used (rls.ForOffice). A read that
// fails, times out or panics leaves the capture failed (Okunamayan fiş), so
// the office still sees it and asks for a new photo.

// BackgroundReadTimeout bounds one background read (every part of it).
const BackgroundReadTimeout = 5 * time.Minute

// MaxBackgroundReads bounds the AI reads running at once; later uploads wait
// for a free slot.
const MaxBackgroundReads = 8

// recordTimeout bounds writing a background read's result (draft, checks).
const recordTimeout = 2 * time.Minute

// failTimeout bounds marking a capture failed after its read went wrong.
const failTimeout = 30 * time.Second

// ReadUnfinished is the error kept on a capture whose background read did
// not finish (the office shows it as an unreadable photo).
const ReadUnfinished = "receipt read did not finish"

// readJob is one background read.
type readJob struct {
	sc     engagementctx.Scope
	userID int64
	pre    prepared
	id     int64
	uuid   uuid.UUID
	// notify: a mükellef upload, told to the office once read (#780
	// Senaryo B); off for an office user's own upload (WhatsApp).
	notify bool
}

// background holds the running reads (Service field).
type background struct {
	// timeout: WithReadTimeout; 0 = BackgroundReadTimeout.
	timeout time.Duration
	wg      sync.WaitGroup
	once    sync.Once
	slots   chan struct{}
}

func (b *background) slot() chan struct{} {
	b.once.Do(func() { b.slots = make(chan struct{}, MaxBackgroundReads) })
	return b.slots
}

// WithReadTimeout bounds one background read (default
// BackgroundReadTimeout); tests shorten it.
func (s *Service) WithReadTimeout(d time.Duration) *Service { s.bg.timeout = d; return s }

func (s *Service) backgroundReadTimeout() time.Duration {
	if s.bg.timeout > 0 {
		return s.bg.timeout
	}
	return BackgroundReadTimeout
}

// WaitReads waits for the background reads started so far, or until ctx
// ends (tests; server shutdown).
func (s *Service) WaitReads(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.bg.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// asyncCapture is the capture of CapturePartsAsync / CaptureKeyedAsync.
func (s *Service) asyncCapture(notify bool) captureFunc {
	return func(ctx context.Context, sc engagementctx.Scope, userID int64, parts []Part, kindHint, clientKey string) (View, error) {
		return s.captureAsync(ctx, sc, userID, parts, kindHint, clientKey, notify)
	}
}

func (s *Service) captureAsync(ctx context.Context, sc engagementctx.Scope, userID int64, parts []Part, kindHint, clientKey string, notify bool) (View, error) {
	pre, v, done, err := s.prepare(ctx, sc, userID, parts, kindHint, clientKey)
	if err != nil || done {
		return v, err
	}
	p := pre.base
	p.Status = StatusProcessing
	c, err := s.q.InsertReceiptCapture(ctx, p)
	if err != nil {
		return View{}, err
	}
	if err := s.saveParts(ctx, sc, c.Uuid, pre.stored); err != nil {
		return View{}, err
	}
	v = s.view(c, nil)
	v.Parts = partViews(pre.stored)
	job := readJob{sc: sc, userID: userID, pre: pre, id: c.ID, uuid: c.Uuid, notify: notify}
	start := func(context.Context) { s.startRead(job) }
	// The capture is written in the request's transaction: the read starts
	// once it is committed (visible to the read's own transaction). Without
	// a transaction the row is already committed.
	if !dbtx.AfterCommit(ctx, start) {
		start(ctx)
	}
	return v, nil
}

// officeScope is the scope rls.ForOffice gives a mükellef upload: the
// office narrowed to the taxpayer, as the uploading user.
func officeScope(sc engagementctx.Scope, userID int64) rls.Scope {
	return rls.Scope{Role: rls.RoleApp, OrganizationID: sc.OrganizationID, UserID: userID,
		TaxpayerIDs: []int64{sc.TaxpayerID}, EngagementsReadAll: true}
}

func (s *Service) startRead(j readJob) {
	s.bg.wg.Add(1)
	go func() { //nolint:gosec // G118: the read outlives the request on purpose; the request ctx carries a finished tx
		defer s.bg.wg.Done()
		slots := s.bg.slot()
		slots <- struct{}{}
		defer func() { <-slots }()
		s.runRead(j)
	}()
}

func (s *Service) runRead(j readJob) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("receipt_background_read_panic", "capture", j.uuid, "panic", fmt.Sprint(r))
			s.failRead(j)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), s.backgroundReadTimeout())
	defer cancel()
	// The model reads outside any transaction; only the result is written
	// in the office scope.
	reads, readErr := s.readParts(ctx, j.sc, j.userID, j.pre)
	// The result gets its own time: a read that ran out of time is still
	// recorded as failed with the reader's error.
	wctx, wcancel := context.WithTimeout(context.Background(), recordTimeout)
	defer wcancel()
	var v View
	err := rls.WithScope(wctx, s.pool, officeScope(j.sc, j.userID), func(ctx context.Context, _ pgx.Tx) error {
		var err error
		v, err = s.record(ctx, j.sc, j.pre, j.id, reads, readErr)
		return err
	})
	if errors.Is(err, ErrNotProcessing) {
		return
	}
	if err != nil {
		s.log.Warn("receipt_background_read_failed", "capture", j.uuid, "taxpayer", j.sc.TaxpayerID, "error", err)
		var ok bool
		if v, ok = s.failRead(j); !ok {
			return
		}
	}
	s.notifyRead(j, v)
}

// failRead marks the capture failed (unreadable) when its read did not
// finish. ok: the capture was still waiting and is now failed.
func (s *Service) failRead(j readJob) (View, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), failTimeout)
	defer cancel()
	var c db.ReceiptCapture
	err := rls.WithScope(ctx, s.pool, officeScope(j.sc, j.userID), func(ctx context.Context, _ pgx.Tx) error {
		var err error
		c, err = s.q.CompleteReceiptCapture(ctx, db.CompleteReceiptCaptureParams{
			Status: StatusFailed, Kind: j.pre.base.Kind, Fields: []byte("{}"), Error: text(ReadUnfinished),
			OrganizationID: j.sc.OrganizationID, EngagementID: j.sc.ID, ID: j.id,
		})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, false
	}
	if err != nil {
		s.log.Error("receipt_background_fail_failed", "capture", j.uuid, "error", err)
		return View{}, false
	}
	v := s.view(c, nil)
	v.Parts = partViews(j.pre.stored)
	return v, true
}

// notifyRead tells the office about the receipt once it is read (#780
// Senaryo B; the request answered before the read). Best effort.
func (s *Service) notifyRead(j readJob, v View) {
	if !j.notify || s.notifier == nil || (v.Status != StatusDraft && v.Status != StatusFailed) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), failTimeout)
	defer cancel()
	if err := rls.WithScope(ctx, s.pool, officeScope(j.sc, j.userID), func(ctx context.Context, _ pgx.Tx) error {
		s.NotifyTaxpayerUpload(ctx, j.sc, j.userID, v)
		return nil
	}); err != nil {
		s.log.Warn("receipt_submitted_notify_failed", "capture", j.uuid, "error", err)
	}
}

// StaleReadAfter: a capture still StatusProcessing this long after the
// upload lost its background read (server restart, crash). Longer than
// BackgroundReadTimeout plus a wait for a read slot.
const StaleReadAfter = 10 * time.Minute

// whatsappKeyPrefix marks a WhatsApp extension capture (CaptureKeyedAsync's
// Idempotency-Key): an office user's own upload, not announced to the office.
const whatsappKeyPrefix = "whatsapp:"

// SweepStaleReads fails the captures whose background read did not finish
// within StaleReadAfter (#780), with the error of a read that timed out, and
// tells the office about a mükellef's receipt as a failed background read
// does. It scans every office: a deliberate row level security bypass
// (rls.System), the failures in one transaction and each notice in its own,
// so a notice that fails keeps the rows failed. The read is not tried again:
// the capture keeps no attempt count, and the office's "Okunamayan fiş" flow
// asks for a new photo.
func (s *Service) SweepStaleReads(ctx context.Context, now time.Time) (int, error) {
	var rows []db.ReceiptCapture
	err := rls.System(ctx, s.pool, "receipts: fail stale background reads across offices", func(ctx context.Context, _ pgx.Tx) error {
		var err error
		rows, err = s.q.FailStaleReceiptCaptures(ctx, db.FailStaleReceiptCapturesParams{
			Error:  ReadUnfinished,
			Before: pgtype.Timestamptz{Time: now.Add(-StaleReadAfter), Valid: true},
		})
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("receipts: fail stale reads: %w", err)
	}
	for _, c := range rows {
		s.log.Warn("receipt_read_stale", "capture", c.Uuid, "taxpayer", c.TaxpayerID, "organization_id", c.OrganizationID)
		if s.notifier == nil || !c.UploadedBy.Valid || strings.HasPrefix(c.ClientKey.String, whatsappKeyPrefix) {
			continue
		}
		sc := engagementctx.Scope{ID: c.EngagementID, OrganizationID: c.OrganizationID, TaxpayerID: c.TaxpayerID}
		v, user := s.view(c, nil), c.UploadedBy.Int64
		if err := rls.System(ctx, s.pool, "receipts: tell the office about a receipt whose read did not finish", func(ctx context.Context, _ pgx.Tx) error {
			s.NotifyTaxpayerUpload(ctx, sc, user, v)
			return nil
		}); err != nil {
			s.log.Warn("receipt_stale_read_notify_failed", "capture", c.Uuid, "error", err)
		}
	}
	return len(rows), nil
}
