package entegrator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	ubltr "github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/repository"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/ublmap"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// UBL failure codes (entegrator_documents.ubl_error.code, 37 §5.4).
const (
	CodeFetch          = "FETCH_FAILED"
	CodePayloadUnknown = "PAYLOAD_UNKNOWN"
	CodeUUIDMismatch   = "UUID_MISMATCH"
	CodePartyMismatch  = model.WarnPartyMismatch
	CodeMapping        = "MAPPING_FAILED"
	CodeStore          = "STORE_FAILED"
)

// ublBatch is how many queue rows are read at a time.
const ublBatch = 50

// maxCandidates caps the invoice ids listed in a POSSIBLE_DUPLICATE warning.
const maxCandidates = 10

type ublFailure struct {
	code string
	err  error
}

func (f *ublFailure) Error() string { return f.code + ": " + f.err.Error() }

// ublQueue downloads the UBL of every queued document once per run, maps it
// (`31`) and stores the invoice with the XML as attachment. Invoice content
// always comes from the UBL, never from the listing header (30 §6).
func (j *job) ublQueue(ctx context.Context) error {
	var after int64
	for {
		rows, err := j.s.q.ListEntegratorUBLQueue(ctx, db.ListEntegratorUBLQueueParams{
			TaxpayerID: j.conn.TaxpayerID, ConnectionID: j.conn.ID, AfterID: after,
			MaxAttempts: int32(j.s.cfg.MaxUBLAttempts), Lim: ublBatch,
		})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, d := range rows {
			after = d.ID
			if err := j.fetchUBL(ctx, d); err != nil {
				return err
			}
		}
	}
}

// fetchUBL handles one queued document. Only errors that end the run are
// returned; a document-level failure is recorded on the row.
func (j *job) fetchUBL(ctx context.Context, d db.EntegratorDocument) error {
	mark := func(status string, f *ublFailure) error {
		var errJSON []byte
		if f != nil {
			m := map[string]any{"code": f.code, "message": f.err.Error()}
			if k := kindOf(f.err); k != "" {
				m["kind"] = string(k)
			}
			errJSON, _ = json.Marshal(m)
		}
		_, err := j.s.q.SetEntegratorDocumentUBL(ctx, db.SetEntegratorDocumentUBLParams{
			TaxpayerID: d.TaxpayerID, ID: d.ID, UblStatus: status, UblError: errJSON,
		})
		return err
	}
	kind, err := providerKind(d.DocKind)
	if err != nil {
		return err
	}
	if !j.c.Supports(kind, provider.OpFetch) {
		j.cnt.UBLMissing++
		return mark("not_available", nil)
	}
	if err := j.throttle(ctx); err != nil {
		return err
	}
	// The integrator gets the ETTN as it listed it (the index keeps it lower-case).
	ref := d.Ettn
	var listed provider.DocumentInfo
	if json.Unmarshal(d.HeaderRaw, &listed) == nil && strings.EqualFold(listed.UUID, d.Ettn) {
		ref = listed.UUID
	}
	res, err := j.c.Fetch(ctx, provider.FetchQuery{Kind: kind, Direction: providerDirection(d.Direction), UUID: ref, Format: provider.FormatUBL})
	if err != nil {
		if abortsRun(err) {
			return err
		}
		if kindOf(err) == provider.KindNotFound || kindOf(err) == provider.KindUnsupported {
			j.cnt.UBLMissing++
			return mark("not_available", &ublFailure{CodeFetch, err})
		}
		j.cnt.UBLFailed++
		return mark("failed", &ublFailure{CodeFetch, err})
	}
	if f := j.storeUBL(ctx, d, res.Data); f != nil {
		j.cnt.UBLFailed++
		return mark("failed", f)
	}
	j.cnt.UBLFetched++
	return nil
}

// storeUBL maps and stores the downloaded payload; the row is marked ok in
// the same transaction as the invoice.
func (j *job) storeUBL(ctx context.Context, d db.EntegratorDocument, payload []byte) *ublFailure {
	items, err := ublmap.Extract(d.Ettn+".xml", payload, ublmap.DefaultLimits)
	if err != nil {
		return &ublFailure{CodePayloadUnknown, err}
	}
	var xml []byte
	for _, it := range items {
		if it.Err == nil && it.Kind == ubltr.KindInvoice {
			xml = it.XML
			break
		}
	}
	if xml == nil {
		return &ublFailure{CodePayloadUnknown, errors.New("no invoice in the downloaded payload")}
	}
	dir := model.DirectionIncoming
	if d.Direction == "outgoing" {
		dir = model.DirectionOutgoing
	}
	mapped, err := ublmap.MapXML(xml, ublmap.Options{OwnTaxID: j.ownTaxID, Direction: dir, SourceSystem: j.conn.SourceSystem, CaptureMethod: model.CaptureAPI})
	var pm *ublmap.PartyMismatchError
	if errors.As(err, &pm) {
		return &ublFailure{CodePartyMismatch, errors.New("own VKN/TCKN is not on the invoice")}
	}
	if err != nil {
		return &ublFailure{CodeMapping, err}
	}
	inv := mapped.Invoice
	if !strings.EqualFold(strings.TrimSpace(inv.UUID), d.Ettn) {
		return &ublFailure{CodeUUIDMismatch, fmt.Errorf("UBL UUID %s differs from the listed ETTN", inv.UUID)}
	}
	if err := j.save(ctx, d, inv, xml, mapped.Warnings); err != nil {
		return &ublFailure{CodeStore, err}
	}
	return nil
}

func (j *job) save(ctx context.Context, d db.EntegratorDocument, inv model.Invoice, xml []byte, warnings []model.Warning) error {
	sum := sha256.Sum256(xml)
	sha := hex.EncodeToString(sum[:])
	key := fmt.Sprintf("taxpayers/%d/ubl/%s.xml", d.TaxpayerID, sha)
	exists, err := j.s.store.Exists(ctx, key)
	if err != nil {
		return err
	}
	if !exists {
		if err := j.s.store.Upload(ctx, storage.File{Body: bytes.NewReader(xml), Size: int64(len(xml)), Filename: d.Ettn + ".xml", ContentType: "application/xml"}, key); err != nil {
			return err
		}
	}
	tx, err := j.s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	att, err := q.UpsertAttachment(ctx, db.UpsertAttachmentParams{
		TaxpayerID: d.TaxpayerID, Kind: "ubl_xml", StorageKey: key, Sha256: sha, SizeBytes: int64(len(xml)),
		ContentType: repository.Text("application/xml"), FileName: repository.Text(d.Ettn + ".xml"),
	})
	if err != nil {
		return err
	}
	saved, err := repository.SaveInvoice(ctx, tx, d.TaxpayerID, inv, att.ID)
	if err != nil {
		return err
	}
	if _, err := q.SetAttachmentOwner(ctx, db.SetAttachmentOwnerParams{TaxpayerID: d.TaxpayerID, ID: att.ID, OwnerType: repository.Text("invoice"), OwnerID: pgtype.Int8{Int64: saved.InvoiceID, Valid: true}}); err != nil {
		return err
	}
	// The integrator status decides whether the invoice counts (30 §8).
	if _, err := q.SetInvoiceStatusNormalized(ctx, db.SetInvoiceStatusNormalizedParams{TaxpayerID: d.TaxpayerID, ID: saved.InvoiceID, StatusNormalized: d.StatusNormalized}); err != nil {
		return err
	}
	if _, err := q.SetEntegratorDocumentUBL(ctx, db.SetEntegratorDocumentUBLParams{
		TaxpayerID: d.TaxpayerID, ID: d.ID, UblStatus: "ok",
		UblAttachmentID: pgtype.Int8{Int64: att.ID, Valid: true}, InvoiceID: pgtype.Int8{Int64: saved.InvoiceID, Valid: true},
	}); err != nil {
		return err
	}
	if saved.Written {
		if err := j.warn(ctx, q, d, inv, saved, warnings); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// warn records the mapping warnings for the office that serves the
// taxpayer now. Without an active engagement there is no office to warn;
// the invoice is still stored.
func (j *job) warn(ctx context.Context, q *db.Queries, d db.EntegratorDocument, inv model.Invoice, saved repository.SaveResult, warnings []model.Warning) error {
	if len(saved.PossibleDuplicates) > 0 {
		ids := make([]string, 0, maxCandidates)
		for i, id := range saved.PossibleDuplicates {
			if i == maxCandidates {
				break
			}
			ids = append(ids, id.String())
		}
		warnings = append(warnings, model.Warning{Code: model.WarnPossibleDuplicate, EntityType: model.EntityInvoice,
			Params: map[string]any{"candidates": ids, "count": len(saved.PossibleDuplicates)}})
	}
	if len(warnings) == 0 {
		return nil
	}
	eng, err := q.GetActiveEngagementForTaxpayer(ctx, d.TaxpayerID)
	if err != nil {
		return nil //nolint:nilerr // no active office: nothing to warn
	}
	for _, w := range warnings {
		setting, err := q.ResolveWarningSetting(ctx, db.ResolveWarningSettingParams{
			Fallback: model.Severity(w.Code), Code: w.Code, OrganizationID: eng.OrganizationID,
			EngagementID: pgtype.Int8{Int64: eng.ID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("warning %s: %w", w.Code, err)
		}
		if !setting.Enabled {
			continue
		}
		entityID := saved.InvoiceID
		if w.EntityType == model.EntityInvoiceItem {
			if id, ok := saved.ItemIDs[w.LineNo]; ok {
				entityID = id
			}
		}
		params := w.Params
		if params == nil {
			params = map[string]any{}
		}
		if w.LineNo > 0 {
			params["line_no"] = w.LineNo
		}
		raw, _ := json.Marshal(params)
		if _, err := q.UpsertOpenDataWarning(ctx, db.UpsertOpenDataWarningParams{
			OrganizationID: eng.OrganizationID, EngagementID: eng.ID, TaxpayerID: d.TaxpayerID,
			Period: repository.Text(inv.Period), EntityType: w.EntityType, EntityID: pgtype.Int8{Int64: entityID, Valid: true},
			Code: w.Code, Severity: setting.Severity, Params: raw, Producer: Producer,
		}); err != nil {
			return fmt.Errorf("warning %s: %w", w.Code, err)
		}
	}
	return nil
}
