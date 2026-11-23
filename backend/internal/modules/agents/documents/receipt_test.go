package documents

import (
	"context"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	receipts "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
)

// Fiş / fatura fotoğrafı: masaüstü ajanının izlediği klasörden gelen JPEG,
// mobil ve WhatsApp ile aynı fiş okumasına (receipts CaptureKeyed) gider;
// mükellef alt klasör adından ya da yoldaki VKN'den bulunur, bulunamazsa
// "eşleştir" kutusunda bekler.

var jpeg = append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, make([]byte, 64)...)

const orgID = int64(7)

type fakeQueries struct {
	agent uuid.UUID
	cands []db.ListAgentDocumentCandidatesRow
	rows  map[int64]db.AgentDocument
	byEng map[int64]uuid.UUID
}

func (f *fakeQueries) GetDesktopAgent(_ context.Context, arg db.GetDesktopAgentParams) (db.DesktopAgent, error) {
	if arg.Uuid != f.agent {
		return db.DesktopAgent{}, pgx.ErrNoRows
	}
	return db.DesktopAgent{ID: 1, Uuid: f.agent, OrganizationID: orgID, Name: "Ofis bilgisayarı"}, nil
}

func (f *fakeQueries) GetAgentDocumentBySha(_ context.Context, arg db.GetAgentDocumentByShaParams) (db.AgentDocument, error) {
	for _, r := range f.rows {
		if r.Sha256 == arg.Sha256 {
			return r, nil
		}
	}
	return db.AgentDocument{}, pgx.ErrNoRows
}

func (f *fakeQueries) CreateAgentDocument(_ context.Context, arg db.CreateAgentDocumentParams) (db.AgentDocument, error) {
	id := int64(len(f.rows) + 1)
	r := db.AgentDocument{
		ID: id, Uuid: uuid.New(), OrganizationID: arg.OrganizationID, AgentID: arg.AgentID, Folder: arg.Folder,
		FileName: arg.FileName, Sha256: arg.Sha256, SizeBytes: arg.SizeBytes, ContentType: arg.ContentType,
		StorageKey: arg.StorageKey, Kind: arg.Kind, Period: arg.Period, Status: StatusNeedsMatch,
	}
	f.rows[id] = r
	return r, nil
}

func (f *fakeQueries) SetAgentDocumentOutcome(_ context.Context, arg db.SetAgentDocumentOutcomeParams) (db.AgentDocument, error) {
	r := f.rows[arg.ID]
	r.EngagementID, r.Kind, r.Period, r.MatchSource = arg.EngagementID, arg.Kind, arg.Period, arg.MatchSource
	r.Status, r.Reason, r.ResultUuid, r.ResolvedBy = arg.Status, arg.Reason, arg.ResultUuid, arg.ResolvedBy
	f.rows[arg.ID] = r
	return r, nil
}

func (f *fakeQueries) GetAgentDocument(_ context.Context, arg db.GetAgentDocumentParams) (db.AgentDocument, error) {
	for _, r := range f.rows {
		if r.Uuid == arg.Uuid {
			return r, nil
		}
	}
	return db.AgentDocument{}, pgx.ErrNoRows
}

func (f *fakeQueries) GetAgentDocumentView(_ context.Context, arg db.GetAgentDocumentViewParams) (db.GetAgentDocumentViewRow, error) {
	r := f.rows[arg.ID]
	v := db.GetAgentDocumentViewRow{
		ID: r.ID, Uuid: r.Uuid, OrganizationID: r.OrganizationID, Folder: r.Folder, FileName: r.FileName,
		Kind: r.Kind, Period: r.Period, MatchSource: r.MatchSource, Status: r.Status, Reason: r.Reason,
		ResultUuid: r.ResultUuid, EngagementID: r.EngagementID,
	}
	if r.EngagementID.Valid {
		v.EngagementUuid = pgtype.UUID{Bytes: f.byEng[r.EngagementID.Int64], Valid: true}
	}
	return v, nil
}

func (f *fakeQueries) ListAgentDocuments(context.Context, db.ListAgentDocumentsParams) ([]db.ListAgentDocumentsRow, error) {
	return nil, nil
}

func (f *fakeQueries) CountAgentDocuments(context.Context, db.CountAgentDocumentsParams) (int64, error) {
	return 0, nil
}

func (f *fakeQueries) ListAgentDocumentCandidates(context.Context, int64) ([]db.ListAgentDocumentCandidatesRow, error) {
	return f.cands, nil
}

type memStore struct{ n int }

func (m *memStore) Upload(context.Context, storage.File, string) error { m.n++; return nil }
func (m *memStore) Download(context.Context, string) (io.ReadCloser, int64, error) {
	return nil, 0, io.EOF
}

type fakeResolver struct{ ids map[uuid.UUID]int64 }

func (r fakeResolver) ResolveEngagement(_ context.Context, org int64, eng uuid.UUID, _ int64) (engagementctx.Access, error) {
	id, ok := r.ids[eng]
	if !ok {
		return engagementctx.Access{}, pgx.ErrNoRows
	}
	return engagementctx.Access{ID: id, UUID: eng, OrganizationID: org, TaxpayerID: id * 10, Status: engagementctx.StatusActive, Assigned: true}, nil
}

// fakeCapture stands in for the receipts use case (the mobile / WhatsApp
// capture): it records the call and answers a capture.
type fakeCapture struct {
	calls []captureCall
	id    uuid.UUID
}

type captureCall struct {
	scope     engagementctx.Scope
	name      string
	kindHint  string
	clientKey string
	size      int
}

func (c *fakeCapture) CaptureKeyed(_ context.Context, sc engagementctx.Scope, _ int64, fileName string, data []byte, kindHint, clientKey string) (receipts.View, error) {
	c.calls = append(c.calls, captureCall{scope: sc, name: fileName, kindHint: kindHint, clientKey: clientKey, size: len(data)})
	return receipts.View{UUID: c.id, Status: receipts.StatusDraft}, nil
}

type fixture struct {
	svc     *Service
	q       *fakeQueries
	capture *fakeCapture
	agent   uuid.UUID
	ornek   uuid.UUID // Örnek Gıda Ltd. Şti., VKN 1234567890
	deniz   uuid.UUID // Deniz Yapı A.Ş., VKN 9876543210
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{agent: uuid.New(), ornek: uuid.New(), deniz: uuid.New(), capture: &fakeCapture{id: uuid.New()}}
	f.q = &fakeQueries{
		agent: f.agent, rows: map[int64]db.AgentDocument{},
		cands: []db.ListAgentDocumentCandidatesRow{
			{EngagementID: 1, EngagementUuid: f.ornek, TaxID: "1234567890", Title: "Örnek Gıda Ltd. Şti."},
			{EngagementID: 2, EngagementUuid: f.deniz, TaxID: "9876543210", Title: "Deniz Yapı A.Ş."},
		},
		byEng: map[int64]uuid.UUID{1: f.ornek, 2: f.deniz},
	}
	res := fakeResolver{ids: map[uuid.UUID]int64{f.ornek: 1, f.deniz: 2}}
	f.svc = New(nil, f.q, &memStore{}, res, nil, nil, nil, nil).WithReceipts(f.capture)
	return f
}

func receiptActor() Actor {
	return Actor{OrganizationID: orgID, UserID: 3, Has: func(p string) bool { return p == rbac.PermTenantReceiptsWrite }}
}

func TestPhotoClassifiedAsReceipt(t *testing.T) {
	for name, data := range map[string][]byte{
		"fis.jpg":  jpeg,
		"fis.png":  []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
		"fis.heic": []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"),
	} {
		if c := Classify(name, data); c.Kind != KindReceipt {
			t.Errorf("%s: kind %q, want receipt", name, c.Kind)
		}
	}
	if !ValidKind(KindReceipt) {
		t.Fatal("the office must be able to resolve a photo as a receipt")
	}
}

func TestPhotoInTaxpayerSubfolderGoesToReceiptCapture(t *testing.T) {
	f := newFixture(t)
	// The watched folder belongs to Deniz Yapı; the subfolder names Örnek Gıda.
	watch := f.deniz
	doc, err := f.svc.Upload(context.Background(), receiptActor(), UploadInput{
		AgentUUID: f.agent, Folder: `C:\Fişler\Örnek Gıda`, FileName: "market.jpg", Data: jpeg, EngagementUUID: &watch,
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Kind != KindReceipt || doc.Status != StatusImported || doc.MatchSource != MatchFolder {
		t.Fatalf("doc = %+v", doc)
	}
	if doc.EngagementUUID == nil || *doc.EngagementUUID != f.ornek {
		t.Fatalf("engagement = %v, want Örnek Gıda", doc.EngagementUUID)
	}
	if doc.ResultUUID == nil || *doc.ResultUUID != f.capture.id {
		t.Fatalf("result = %v, want the receipt capture", doc.ResultUUID)
	}
	if len(f.capture.calls) != 1 {
		t.Fatalf("capture calls = %d", len(f.capture.calls))
	}
	c := f.capture.calls[0]
	if c.scope.ID != 1 || c.scope.TaxpayerID != 10 || c.name != "market.jpg" || c.kindHint != "" || c.size != len(jpeg) {
		t.Fatalf("capture = %+v", c)
	}
	if len(c.clientKey) != len("agent:")+64 || c.clientKey[:6] != "agent:" {
		t.Fatalf("client key = %q", c.clientKey)
	}
}

func TestPhotoMatchedByVKNInPath(t *testing.T) {
	f := newFixture(t)
	doc, err := f.svc.Upload(context.Background(), receiptActor(), UploadInput{
		AgentUUID: f.agent, Folder: "/Users/ofis/Fisler/9876543210", FileName: "z_raporu.png",
		Data: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != StatusImported || doc.EngagementUUID == nil || *doc.EngagementUUID != f.deniz {
		t.Fatalf("doc = %+v", doc)
	}
	if len(f.capture.calls) != 1 || f.capture.calls[0].scope.ID != 2 {
		t.Fatalf("capture = %+v", f.capture.calls)
	}
}

func TestPhotoWithoutTaxpayerWaitsUnmatched(t *testing.T) {
	f := newFixture(t)
	doc, err := f.svc.Upload(context.Background(), receiptActor(), UploadInput{
		AgentUUID: f.agent, Folder: `C:\Fişler\Gelen`, FileName: "fis.jpg", Data: jpeg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Kind != KindReceipt || doc.Status != StatusNeedsMatch || doc.EngagementUUID != nil || doc.Reason != reasonNoTaxpayer {
		t.Fatalf("doc = %+v", doc)
	}
	if len(f.capture.calls) != 0 {
		t.Fatalf("an unmatched photo must not be read: %+v", f.capture.calls)
	}
}

func TestPhotoWithoutReceiptPermissionWaits(t *testing.T) {
	f := newFixture(t)
	doc, err := f.svc.Upload(context.Background(), Actor{OrganizationID: orgID, UserID: 3, Has: func(string) bool { return false }},
		UploadInput{AgentUUID: f.agent, Folder: `C:\Fişler\Örnek Gıda`, FileName: "fis.jpg", Data: jpeg})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != StatusNeedsMatch || doc.Reason != reasonNoPerm || len(f.capture.calls) != 0 {
		t.Fatalf("doc = %+v calls = %d", doc, len(f.capture.calls))
	}
}

func TestByFolderLeaf(t *testing.T) {
	ornek, deniz := uuid.New(), uuid.New()
	cands := []Candidate{
		{EngagementUUID: ornek, TaxID: "1234567890", Title: "Örnek Gıda Ltd. Şti."},
		{EngagementUUID: deniz, TaxID: "9876543210", Title: "Deniz Yapı A.Ş."},
	}
	for folder, want := range map[string]uuid.UUID{
		`C:\Fişler\ornek gida`:          ornek,
		"/fisler/Deniz Yapı":            deniz,
		`D:\Tarama\9876543210 Deniz`:    deniz,
		`C:\Örnek Gıda\Fişler`:          uuid.Nil, // leaf is generic
		`C:\Fişler\Gelen`:               uuid.Nil,
		`C:\Fişler\Bilinmeyen Firma AŞ`: uuid.Nil,
	} {
		c, ok := ByFolderLeaf(folder, cands)
		if want == uuid.Nil {
			if ok {
				t.Errorf("%s: matched %v, want none", folder, c.Title)
			}
			continue
		}
		if !ok || c.EngagementUUID != want {
			t.Errorf("%s: got %v %v", folder, c.Title, ok)
		}
	}
}
