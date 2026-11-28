package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/statement"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

// fakeImporter records the request and answers err.
type fakeImporter struct {
	got statement.Request
	err error
}

func (f *fakeImporter) Import(_ context.Context, req statement.Request) (statement.Report, error) {
	f.got = req
	return statement.Report{}, f.err
}

const csvStatement = "Tarih;Açıklama;Tutar;Bakiye\n03.11.2026;FAST GELEN;2.500,00;12.500,00\n"

// "Banka ekstresi olarak kaydet" (#780): the file goes to the panel's
// statement import; its errors keep the import's codes.
func TestCaptureBankStatementRouting(t *testing.T) {
	ctx := context.Background()
	sc := engagementctx.Scope{ID: 7, UUID: uuid.New(), OrganizationID: 3, TaxpayerID: 11}

	// Not wired: a clear error, nothing else touched.
	if _, err := New(nil, nil, nil, nil, nil).Capture(ctx, sc, 1, CaptureInput{Target: TargetBankStatement, FileName: "e.csv", Data: []byte(csvStatement)}); err == nil {
		t.Fatal("unconfigured importer accepted the file")
	}

	im := &fakeImporter{err: statement.ErrLayoutUnknown}
	svc := New(nil, nil, nil, nil, nil).WithStatements(im, nil)
	task := uuid.New()
	box := uuid.New()
	for name, in := range map[string]CaptureInput{
		"photo":             {Target: TargetBankStatement, FileName: "ekstre.jpg", Data: []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00")},
		"task":              {Target: TargetBankStatement, FileName: "e.csv", Data: []byte(csvStatement), TaskUUID: &task},
		"box on a document": {Target: TargetDocument, FileName: "e.pdf", Data: []byte("%PDF-1.4\n"), CashBoxUUID: &box},
		"bad chat":          {Target: TargetBankStatement, ChatKey: "merhaba", FileName: "e.csv", Data: []byte(csvStatement)},
		"empty":             {Target: TargetBankStatement, FileName: "e.csv"},
	} {
		if _, err := svc.Capture(ctx, sc, 1, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if im.got.FileName != "" {
		t.Fatalf("importer called for a refused file: %+v", im.got)
	}

	_, err := svc.Capture(ctx, sc, 1, CaptureInput{Target: TargetBankStatement, ChatKey: "905551112233@c.us", FileName: " ekstre.csv ", Data: []byte(csvStatement)})
	var se *StatementError
	if !errors.As(err, &se) || !errors.Is(err, statement.ErrLayoutUnknown) {
		t.Fatalf("err = %v, want a StatementError of ErrLayoutUnknown", err)
	}
	if im.got.TaxpayerID != 11 || im.got.OrganizationID != 3 || im.got.EngagementID != 7 || im.got.FileName != "ekstre.csv" || string(im.got.Data) != csvStatement || im.got.Preview {
		t.Fatalf("request = %+v", im.got)
	}

	// The handler answers it like the panel's upload.
	rec := httptest.NewRecorder()
	writeError(rec, httptest.NewRequest(http.MethodPost, "/", nil), err)
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusUnprocessableEntity || body.Error.Code != "STATEMENT_LAYOUT_UNKNOWN" {
		t.Fatalf("answer = %d %s", rec.Code, rec.Body)
	}
}

func TestStatementResult(t *testing.T) {
	id := uuid.New()
	gap := "kapanış tutmuyor"
	rep := statement.Report{ImportID: id, Format: "csv", Accounts: []statement.AccountResult{
		{Lines: 3, Inserted: 2, Skipped: 1, BankID: "akbank"},
		{Lines: 2, Merged: 1, PossibleDuplicates: 1, Gaps: []string{gap}},
	}}
	r := statementResult(rep)
	if r.ImportID != id || r.Accounts != 2 || r.Lines != 5 || r.Inserted != 2 || r.Merged != 1 || r.Skipped != 1 ||
		r.PossibleDuplicates != 1 || !r.Gaps || r.BankID != "akbank" || r.replayed() {
		t.Fatalf("result = %+v", r)
	}
	again := statementResult(statement.Report{Accounts: []statement.AccountResult{{Lines: 3, Skipped: 3}}})
	if !again.replayed() {
		t.Fatalf("all skipped should be a replay: %+v", again)
	}
	if statementResult(statement.Report{}).replayed() {
		t.Fatal("an empty file is not a replay")
	}
}
