package office

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/activity"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/authctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
)

func addMonths(t *testing.T, period string, n int) string {
	t.Helper()
	p, err := time.Parse("2006-01", period)
	if err != nil {
		t.Fatal(err)
	}
	return p.AddDate(0, n, 0).Format("2006-01")
}

// KDV indirim dönemi (#640): yalnız alış faturası, yalnız ileri bir ay,
// en geç izleyen yılın Aralık'ı; kilitli ay değişmez; kim, ne zaman ve
// etkinlik kaydı tutulur; geri alınca belge ayına döner.
func TestOfficeSetsVATDeductionPeriod(t *testing.T) {
	e := setup(t)
	e.h.WithActivity(activity.NewRecorder(e.q, nil))
	ctx := context.Background()
	uid := e.d.User(t)
	if _, err := e.d.Pool.Exec(ctx, `UPDATE users SET name = 'Ayşe', surname = 'Kurcu' WHERE id = $1`, uid); err != nil {
		t.Fatal(err)
	}
	req := func(method, body string) *http.Request {
		r := httptest.NewRequest(method, "/x", strings.NewReader(body))
		r.SetPathValue("invoiceUuid", e.inv)
		ctx := authctx.WithPrincipal(engagementctx.WithScope(r.Context(), e.scope), authctx.Principal{UserInternal: uid})
		return r.WithContext(ctx)
	}
	put := func(period string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		e.h.setVATDeductionPeriod(w, req(http.MethodPut, `{"period":"`+period+`"}`))
		return w
	}
	undo := func() *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		e.h.clearVATDeductionPeriod(w, req(http.MethodDelete, ""))
		return w
	}
	detail := func() Detail {
		t.Helper()
		w := httptest.NewRecorder()
		e.h.get(w, req(http.MethodGet, ""))
		var env struct {
			Data Detail `json:"data"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &env) != nil {
			t.Fatalf("get: %d %s", w.Code, w.Body)
		}
		return env.Data
	}
	expect := func(w *httptest.ResponseRecorder, code int, errCode string) {
		t.Helper()
		if w.Code != code || !strings.Contains(w.Body.String(), errCode) {
			t.Fatalf("want %d %s, got %d %s", code, errCode, w.Code, w.Body)
		}
	}

	var period string
	if err := e.d.Pool.QueryRow(ctx, `SELECT period FROM invoices WHERE uuid = $1`, e.inv).Scan(&period); err != nil {
		t.Fatal(err)
	}
	next := addMonths(t, period, 1)
	// Satış faturası: KDV indirimi yok.
	expect(put(next), http.StatusUnprocessableEntity, "NOT_PURCHASE_INVOICE")
	if _, err := e.d.Pool.Exec(ctx, `UPDATE invoices SET direction = 'incoming' WHERE uuid = $1`, e.inv); err != nil {
		t.Fatal(err)
	}
	if d := detail(); d.VATDeductionPeriod != nil || d.VATDeductionEntry != nil {
		t.Fatalf("fresh invoice has a deduction period %+v %+v", d.VATDeductionPeriod, d.VATDeductionEntry)
	}
	for _, bad := range []string{"2026-13", "abc", ""} {
		expect(put(bad), http.StatusBadRequest, "INVALID_PERIOD")
	}
	expect(put(period), http.StatusUnprocessableEntity, "VAT_PERIOD_NOT_FORWARD")
	expect(put(addMonths(t, period, -1)), http.StatusUnprocessableEntity, "VAT_PERIOD_NOT_FORWARD")
	lastAllowed := period[:4]
	lastAllowed = addMonths(t, lastAllowed+"-12", 12)
	expect(put(addMonths(t, lastAllowed, 1)), http.StatusUnprocessableEntity, "VAT_PERIOD_OUT_OF_WINDOW")

	// Hedef ay kilitliyse değişmez.
	lock := func(p string) {
		t.Helper()
		if _, err := e.d.Pool.Exec(ctx, `INSERT INTO period_locks (organization_id, engagement_id, taxpayer_id, period)
			VALUES ($1, $2, $3, $4)`, e.scope.OrganizationID, e.scope.ID, e.scope.TaxpayerID, p); err != nil {
			t.Fatal(err)
		}
	}
	unlock := func() {
		t.Helper()
		if _, err := e.d.Pool.Exec(ctx, `DELETE FROM period_locks WHERE engagement_id = $1`, e.scope.ID); err != nil {
			t.Fatal(err)
		}
	}
	lock(next)
	expect(put(next), http.StatusConflict, "PERIOD_LOCKED")
	unlock()
	// Belge ayı kilitliyse de değişmez (o ayın beyanı etkilenir).
	lock(period)
	expect(put(next), http.StatusConflict, "PERIOD_LOCKED")
	unlock()

	if w := put(lastAllowed); w.Code != http.StatusOK {
		t.Fatalf("set: %d %s", w.Code, w.Body)
	}
	d := detail()
	if d.VATDeductionPeriod == nil || *d.VATDeductionPeriod != lastAllowed {
		t.Fatalf("deduction period %v, want %s", d.VATDeductionPeriod, lastAllowed)
	}
	if d.VATDeductionEntry == nil || d.VATDeductionEntry.SetByName != "Ayşe Kurcu" || d.VATDeductionEntry.SetAt.IsZero() {
		t.Fatalf("deduction entry %+v", d.VATDeductionEntry)
	}
	var docPeriod string
	if err := e.d.Pool.QueryRow(ctx, `SELECT period FROM invoices WHERE uuid = $1`, e.inv).Scan(&docPeriod); err != nil {
		t.Fatal(err)
	}
	if docPeriod != period {
		t.Fatalf("document period changed: %s -> %s", period, docPeriod)
	}
	// Mevcut indirim ayı kilitliyse başka aya alınamaz, geri de alınamaz.
	lock(lastAllowed)
	expect(put(next), http.StatusConflict, "PERIOD_LOCKED")
	expect(undo(), http.StatusConflict, "PERIOD_LOCKED")
	unlock()

	if w := put(next); w.Code != http.StatusOK {
		t.Fatalf("move: %d %s", w.Code, w.Body)
	}
	// Geri al: belge ayına döner, kim/ne zaman temizlenir.
	if w := undo(); w.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", w.Code, w.Body)
	}
	if d := detail(); d.VATDeductionPeriod != nil || d.VATDeductionEntry != nil {
		t.Fatalf("after clear %+v %+v", d.VATDeductionPeriod, d.VATDeductionEntry)
	}

	var actions []string
	rows, err := e.d.Pool.Query(ctx, `SELECT action || ':' || (payload->>'from') || '>' || (payload->>'to')
		FROM activity_events WHERE resource = 'invoice' AND resource_uuid = $1 ORDER BY id`, e.inv)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, a)
	}
	rows.Close()
	want := []string{
		"invoice.vat_deduction_period.set:" + period + ">" + lastAllowed,
		"invoice.vat_deduction_period.set:" + lastAllowed + ">" + next,
		"invoice.vat_deduction_period.clear:" + next + ">" + period,
	}
	if strings.Join(actions, ",") != strings.Join(want, ",") {
		t.Fatalf("activity %v, want %v", actions, want)
	}
}

func TestVATDeductionAllowed(t *testing.T) {
	cases := []struct{ dir, doc, target, want string }{
		{"outgoing", "2026-03", "2026-04", "NOT_PURCHASE_INVOICE"},
		{"incoming", "2026-03", "2026-03", "VAT_PERIOD_NOT_FORWARD"},
		{"incoming", "2026-03", "2026-02", "VAT_PERIOD_NOT_FORWARD"},
		{"incoming", "2026-03", "2026-04", ""},
		{"incoming", "2026-03", "2027-12", ""},
		{"incoming", "2026-12", "2027-12", ""},
		{"incoming", "2026-03", "2028-01", "VAT_PERIOD_OUT_OF_WINDOW"},
	}
	for _, c := range cases {
		if got := VATDeductionAllowed(c.dir, c.doc, c.target); got != c.want {
			t.Errorf("%+v: got %q", c, got)
		}
	}
}
