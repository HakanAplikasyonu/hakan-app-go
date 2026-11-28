package httpserver

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
)

// TestLucaExportsFromPhoneAndWhatsApp (Luca uçtan uca denetimi): mükellefin
// telefondan (mobil uygulamanın ucu POST /v1/taxpayer/receipts) ve ofisin
// WhatsApp uzantısıyla (POST .../whatsapp/captures) gönderdiği masraf fişi
// ve Z raporu Luca'ya iki yoldan da doğru gider:
//
//   - telefondan gelen masraf fişi (%1 / %10 / %20) ve iki parçalı Z raporu
//     ofiste onaylanır, yevmiye fişleri onaylanıp Luca fiş dosyasına
//     (journal-exports) aktarılır: her KDV oranı kendi satırında, borç =
//     alacak; ardından Masraflar / Z raporları listesinin "Luca'ya aktar
//     (Excel)" dosyası onları bir daha almaz (çift aktarım koruması);
//   - WhatsApp'tan gelen akaryakıt fişi (tek oran %20) ve Z raporu onay
//     beklerken listelerin Luca dosyasına girer (tek oranlı fişin KDV'si
//     %20'nin hesabında, "kırılım yok" işareti olmadan); ikinci çıktı onları
//     almaz; Z raporu sonra onaylanıp fiş dosyasına aktarılmak istenince
//     DOCUMENT_ALREADY_EXPORTED ile durur, yeniden dahil edilince gider.
//
// Yapay zekâ sahtesi (testfakes.AI) fişleri okur; veriler uydurmadır.
func TestLucaExportsFromPhoneAndWhatsApp(t *testing.T) {
	e := newSweepEnv(t)
	eng := e.param(t, "engagementUuid", "")
	// Gerçekçi hesap planı: demo planında KDV yalnız %20'nin alt hesabıyla
	// (191.01 / 391.01) açık. Fiş %1 / %10 / %20 KDV taşıdığından her oranın
	// 191 / 391 alt hesabı açılır; Luca grup hesabına (191, 391) kayıt almaz.
	if err := rls.System(context.Background(), e.d.Pool, "test: rate sub-accounts", func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO account_plan_accounts (organization_id, engagement_id, import_id, target, code, name, main_code, level, is_leaf)
			SELECT a.organization_id, a.engagement_id, a.import_id, a.target, v.code, v.name, left(v.code, 3), 2, true
			FROM account_plan_accounts a
			CROSS JOIN (VALUES ('191.10', 'İNDİRİLECEK KDV %10'), ('191.11', 'İNDİRİLECEK KDV %1'),
			                   ('391.10', 'HESAPLANAN KDV %10'), ('391.11', 'HESAPLANAN KDV %1')) AS v(code, name)
			WHERE a.organization_id = $1 AND a.engagement_id = $2 AND a.code = '391.01'
			ON CONFLICT DO NOTHING`, e.orgID, e.engID)
		if err == nil && tag.RowsAffected() != 4 {
			err = fmt.Errorf("rate sub-accounts: %d rows (no 391.01 in the demo plan?)", tag.RowsAffected())
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	base := "/v1/tenant/engagements/" + eng

	type env struct {
		Data  json.RawMessage `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Details []struct {
				Code string `json:"code"`
			} `json:"details"`
		} `json:"error"`
	}
	send := func(taxpayer bool, method, path, contentType string, body io.Reader) *httptest.ResponseRecorder {
		t.Helper()
		if body == nil {
			body = strings.NewReader("")
		}
		req := httptest.NewRequest(method, path, body)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if taxpayer {
			req.Header.Set("Authorization", "Bearer "+e.tpTok)
			req.Header.Set("X-Taxpayer", e.tpUUID)
		} else {
			req.Header.Set("Authorization", "Bearer "+e.ownerTok)
			req.Header.Set("X-Organization", "demo-smmm")
		}
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder, what string, want int, into any) env {
		t.Helper()
		var out env
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		if rec.Code != want {
			t.Fatalf("%s: %d (want %d) %s", what, rec.Code, want, rec.Body)
		}
		if into != nil {
			if err := json.Unmarshal(out.Data, into); err != nil {
				t.Fatalf("%s: %v %s", what, err, rec.Body)
			}
		}
		return out
	}
	postJSON := func(what, path, body string, want int, into any) env {
		t.Helper()
		return decode(send(false, http.MethodPost, path, "application/json", strings.NewReader(body)), what, want, into)
	}
	getJSON := func(what, path string, into any) {
		t.Helper()
		decode(send(false, http.MethodGet, path, "", nil), what, http.StatusOK, into)
	}
	form := func(fields map[string]string, files map[string]string) (string, io.Reader) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range fields {
			_ = mw.WriteField(k, v)
		}
		for _, field := range []string{"image", "image_2", "file"} {
			data, ok := files[field]
			if !ok {
				continue
			}
			fw, _ := mw.CreateFormFile(field, field+".png")
			_, _ = fw.Write([]byte(data))
		}
		_ = mw.Close()
		return mw.FormDataContentType(), &buf
	}
	// draftOf reads the expense / Z report a capture became.
	draftOf := func(capture, table, column string) string {
		t.Helper()
		if _, err := uuid.Parse(capture); err != nil {
			t.Fatalf("capture uuid %q", capture)
		}
		q := fmt.Sprintf(`SELECT x.uuid::text FROM receipt_captures c JOIN %s x ON x.id = c.%s
			WHERE c.uuid = '%s' AND c.organization_id = $1 AND c.engagement_id = $2 AND x.taxpayer_id = $3`, table, column, capture)
		return e.one(t, q, table)
	}
	type captureView struct {
		UUID   string `json:"uuid"`
		Status string `json:"status"`
		Draft  *struct {
			Type string `json:"type"`
			UUID string `json:"uuid"`
		} `json:"draft"`
	}
	type journalRef struct {
		UUID string `json:"uuid"`
	}
	type expenseView struct {
		UUID          string          `json:"uuid"`
		Period        *string         `json:"period"`
		ReceiptNumber *string         `json:"receipt_number"`
		TaxRate       json.RawMessage `json:"tax_rate"`
		Journal       *journalRef     `json:"journal"`
	}
	type zView struct {
		UUID    string      `json:"uuid"`
		Period  *string     `json:"period"`
		ZNumber *string     `json:"z_report_number"`
		Journal *journalRef `json:"journal"`
	}
	expense := func(id string) expenseView {
		t.Helper()
		var v expenseView
		getJSON("expense "+id, base+"/expenses/"+id, &v)
		if v.Period == nil || v.ReceiptNumber == nil {
			t.Fatalf("expense %s: %+v", id, v)
		}
		return v
	}
	zReport := func(id string) zView {
		t.Helper()
		var v zView
		getJSON("Z report "+id, base+"/retail-sales/"+id, &v)
		if v.Period == nil || v.ZNumber == nil {
			t.Fatalf("Z report %s: %+v", id, v)
		}
		return v
	}

	// 1) Telefondan: masraf fişi ve iki parçalı Z raporu (mobil uygulamanın
	// ucu, mükellef oturumu).
	ct, body := form(map[string]string{"kind": "expense_receipt"}, map[string]string{"image": pngOf(0x31)})
	var phoneExpCap captureView
	decode(send(true, http.MethodPost, "/v1/taxpayer/receipts", ct, body), "phone expense", http.StatusCreated, &phoneExpCap)
	if phoneExpCap.Status != "draft" || phoneExpCap.Draft == nil || phoneExpCap.Draft.Type != "expense" {
		t.Fatalf("phone expense capture: %+v", phoneExpCap)
	}
	ct, body = form(map[string]string{"kind": "z_report"}, map[string]string{"image": pngOf(0x32), "image_2": pngOf(0x33)})
	var phoneZCap captureView
	decode(send(true, http.MethodPost, "/v1/taxpayer/receipts", ct, body), "phone Z report", http.StatusCreated, &phoneZCap)
	if phoneZCap.Status != "draft" || phoneZCap.Draft == nil || phoneZCap.Draft.Type != "retail_sale" {
		t.Fatalf("phone Z capture: %+v", phoneZCap)
	}

	// 2) WhatsApp uzantısından: "Gider olarak kaydet" (akaryakıt fişi) ve
	// "Gelir olarak kaydet" (Z raporu).
	type waCapture struct {
		ReceiptUUID *string `json:"receipt_uuid"`
	}
	var waExpCap, waZCap waCapture
	chat := "905551112244@c.us" // WhatsApp sohbeti (uydurma numara)
	ct, body = form(map[string]string{"target": "expense", "chat_key": chat}, map[string]string{"file": pngOf(0x34)})
	decode(send(false, http.MethodPost, base+"/whatsapp/captures", ct, body), "WhatsApp expense", http.StatusCreated, &waExpCap)
	ct, body = form(map[string]string{"target": "income", "chat_key": chat}, map[string]string{"file": pngOf(0x35)})
	decode(send(false, http.MethodPost, base+"/whatsapp/captures", ct, body), "WhatsApp Z report", http.StatusCreated, &waZCap)
	if waExpCap.ReceiptUUID == nil || waZCap.ReceiptUUID == nil {
		t.Fatalf("WhatsApp captures without a receipt: %+v %+v", waExpCap, waZCap)
	}

	phoneExp := expense(phoneExpCap.Draft.UUID)
	phoneZ := zReport(phoneZCap.Draft.UUID)
	waExp := expense(draftOf(*waExpCap.ReceiptUUID, "expenses", "expense_id"))
	waZ := zReport(draftOf(*waZCap.ReceiptUUID, "retail_sales", "retail_sale_id"))
	if len(waExp.TaxRate) == 0 || string(waExp.TaxRate) == "null" {
		t.Fatalf("WhatsApp fuel receipt without its rate: %+v", waExp)
	}

	// 3) Telefondan gelenler: ofis onaylar, yevmiye fişleri onaylanır ve Luca
	// fiş dosyasına aktarılır.
	approveCapture := func(id string) {
		t.Helper()
		postJSON("approve capture "+id, base+"/receipts/"+id+"/approve", `{}`, http.StatusOK, nil)
	}
	approveDraft := func(id string) {
		t.Helper()
		path := base + "/journal-drafts/" + id + "/approve"
		rec := send(false, http.MethodPost, path, "application/json", strings.NewReader(`{"confirm_warnings":true,"note":"uçtan uca test"}`))
		if rec.Code == http.StatusConflict {
			rec = send(false, http.MethodPost, path, "application/json",
				strings.NewReader(`{"confirm_warnings":true,"note":"uçtan uca test","send_duplicate":true,"reason":"uçtan uca test"}`))
		}
		decode(rec, "approve journal draft "+id, http.StatusOK, nil)
	}
	approveCapture(phoneExpCap.UUID)
	approveCapture(phoneZCap.UUID)
	phoneExp, phoneZ = expense(phoneExp.UUID), zReport(phoneZ.UUID)
	if phoneExp.Journal == nil || phoneZ.Journal == nil {
		t.Fatalf("approved without a journal draft: %+v %+v", phoneExp.Journal, phoneZ.Journal)
	}
	approveDraft(phoneExp.Journal.UUID)
	approveDraft(phoneZ.Journal.UUID)
	type batch struct {
		UUID  string `json:"uuid"`
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	journalFile := func(b batch) []byte {
		t.Helper()
		if len(b.Files) != 1 {
			t.Fatalf("journal export files: %+v", b.Files)
		}
		rec := send(false, http.MethodGet, base+"/journal-exports/"+b.UUID+"/files/"+b.Files[0].Name, "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("journal file: %d %s", rec.Code, rec.Body)
		}
		return rec.Body.Bytes()
	}
	var phoneBatch batch
	postJSON("journal export (phone)", base+"/journal-exports",
		fmt.Sprintf(`{"driver":"luca","drafts":[%q,%q]}`, phoneExp.Journal.UUID, phoneZ.Journal.UUID), http.StatusCreated, &phoneBatch)
	fis := lucaVouchers(t, journalFile(phoneBatch))
	checkExpenseVoucher(t, "phone expense in the journal file", fis.voucher(t, *phoneExp.ReceiptNumber, "921.00"),
		[]string{"1.00", "20.00", "100.00"}, "921.00")
	checkZVoucher(t, "phone Z report in the journal file", fis.voucher(t, *phoneZ.ZNumber, zTotal))

	// 4) Listelerin Luca dosyası: WhatsApp'tan gelenler (onay bekliyor) girer;
	// fiş dosyasıyla gitmiş telefon kayıtları girmez.
	type lucaReport struct {
		AlreadyExported []struct {
			DocumentNo string `json:"document_no"`
		} `json:"already_exported"`
	}
	report := func(rec *httptest.ResponseRecorder) lucaReport {
		t.Helper()
		var r lucaReport
		raw, err := base64.RawURLEncoding.DecodeString(rec.Header().Get("X-Luca-Report"))
		if err != nil || json.Unmarshal(raw, &r) != nil {
			t.Fatalf("X-Luca-Report %q: %v", rec.Header().Get("X-Luca-Report"), err)
		}
		return r
	}
	alreadyIn := func(r lucaReport, no string) bool {
		for _, x := range r.AlreadyExported {
			if x.DocumentNo == no {
				return true
			}
		}
		return false
	}
	zList := func(period string) *httptest.ResponseRecorder {
		t.Helper()
		return send(false, http.MethodPost, base+"/luca-lists/z-reports", "application/json", strings.NewReader(`{"period":"`+period+`"}`))
	}
	expenseList := func(period string) *httptest.ResponseRecorder {
		t.Helper()
		return send(false, http.MethodPost, base+"/expenses/luca", "application/json", strings.NewReader(`{"period":"`+period+`"}`))
	}

	rec := zList(*waZ.Period)
	if rec.Code != http.StatusOK {
		t.Fatalf("Z list export: %d %s", rec.Code, rec.Body)
	}
	zFile := lucaVouchers(t, rec.Body.Bytes())
	checkZVoucher(t, "WhatsApp Z report in the Z list file", zFile.voucher(t, *waZ.ZNumber, zTotal))
	if zFile.has(*phoneZ.ZNumber, zTotal) {
		t.Errorf("phone Z report %s went into the Z list file after the journal file", *phoneZ.ZNumber)
	}
	if *phoneZ.Period == *waZ.Period && !alreadyIn(report(rec), *phoneZ.ZNumber) {
		t.Errorf("phone Z report %s not reported as already exported: %s", *phoneZ.ZNumber, rec.Header().Get("X-Luca-Report"))
	}

	rec = expenseList(*waExp.Period)
	if rec.Code != http.StatusOK {
		t.Fatalf("expense list export: %d %s", rec.Code, rec.Body)
	}
	expFile := lucaVouchers(t, rec.Body.Bytes())
	fuel := expFile.voucher(t, *waExp.ReceiptNumber, "1250.00")
	checkExpenseVoucher(t, "WhatsApp fuel receipt in the expense list file", fuel, []string{"208.33"}, "1250.00")
	for _, l := range fuel {
		if strings.Contains(l.detail, "kırılımı yok") {
			t.Errorf("read single rate marked as no breakdown: %+v", l)
		}
	}
	if *phoneExp.Period != *waExp.Period {
		rec = expenseList(*phoneExp.Period)
	} else {
		rec = expenseList(*waExp.Period)
	}
	switch rec.Code {
	case http.StatusOK:
		if lucaVouchers(t, rec.Body.Bytes()).has(*phoneExp.ReceiptNumber, "921.00") {
			t.Errorf("phone expense %s went into the expense list file after the journal file", *phoneExp.ReceiptNumber)
		}
		if rec.Header().Get("X-Expenses-Already-Exported") == "0" {
			t.Errorf("phone expense not counted as already exported")
		}
	case http.StatusUnprocessableEntity:
		// Dönemde aktarılacak başka masraf yok: koruma hepsini dışarıda bıraktı.
	default:
		t.Fatalf("expense list export (phone period): %d %s", rec.Code, rec.Body)
	}

	// 5) İkinci liste çıktısı WhatsApp kayıtlarını almaz.
	rec = zList(*waZ.Period)
	switch rec.Code {
	case http.StatusOK:
		if lucaVouchers(t, rec.Body.Bytes()).has(*waZ.ZNumber, zTotal) {
			t.Errorf("WhatsApp Z report exported twice")
		}
	case http.StatusUnprocessableEntity:
	default:
		t.Fatalf("second Z list export: %d %s", rec.Code, rec.Body)
	}
	if !alreadyIn(report(rec), *waZ.ZNumber) {
		t.Errorf("second Z list export does not report the WhatsApp Z report: %s", rec.Header().Get("X-Luca-Report"))
	}
	rec = expenseList(*waExp.Period)
	switch rec.Code {
	case http.StatusOK:
		if lucaVouchers(t, rec.Body.Bytes()).has(*waExp.ReceiptNumber, "1250.00") {
			t.Errorf("WhatsApp expense exported twice")
		}
	case http.StatusUnprocessableEntity:
	default:
		t.Fatalf("second expense list export: %d %s", rec.Code, rec.Body)
	}

	// 6) Liste dosyasıyla giden WhatsApp Z raporu onaylanıp fiş dosyasına
	// aktarılmak istenir: koruma durdurur, yeniden dahil edilince gider.
	approveCapture(*waZCap.ReceiptUUID)
	waZ = zReport(waZ.UUID)
	if waZ.Journal == nil {
		t.Fatalf("WhatsApp Z report approved without a journal draft")
	}
	approveDraft(waZ.Journal.UUID)
	blocked := postJSON("journal export (WhatsApp Z, already in a list file)", base+"/journal-exports",
		fmt.Sprintf(`{"driver":"luca","drafts":[%q]}`, waZ.Journal.UUID), http.StatusUnprocessableEntity, nil)
	found := false
	for _, d := range blocked.Error.Details {
		found = found || d.Code == "DOCUMENT_ALREADY_EXPORTED"
	}
	if blocked.Error.Code != "EXPORT_BLOCKED" || !found {
		t.Fatalf("journal export not stopped by the guard: %+v", blocked.Error)
	}
	var waBatch batch
	postJSON("journal export (WhatsApp Z, re-included)", base+"/journal-exports",
		fmt.Sprintf(`{"driver":"luca","drafts":[%q],"confirm_documents":["z:%s"]}`, waZ.Journal.UUID, waZ.UUID), http.StatusCreated, &waBatch)
	checkZVoucher(t, "WhatsApp Z report in the journal file", lucaVouchers(t, journalFile(waBatch)).voucher(t, *waZ.ZNumber, zTotal))
}

// lucaLine is one row of a Luca fiş file.
type lucaLine struct {
	file, fis, account, docNo, detail string
	debit, credit                     decimal.Decimal
}

type lucaFile []lucaLine

// lucaVouchers reads a Luca fiş file: one .xlsx, or a .zip of them (Luca's
// 50-voucher limit). Columns are found by their header.
func lucaVouchers(t *testing.T, data []byte) lucaFile {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a zip / xlsx: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "[Content_Types].xml" {
			return lucaSheet(t, "", data)
		}
	}
	var out lucaFile
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".xlsx") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, lucaSheet(t, f.Name, b)...)
	}
	return out
}

func lucaSheet(t *testing.T, name string, data []byte) lucaFile {
	t.Helper()
	x, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := x.GetRows(x.GetSheetName(0))
	if err != nil || len(rows) == 0 {
		t.Fatalf("sheet: %v", err)
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.TrimSpace(h)] = i
	}
	for _, h := range []string{"Fiş No", "Hesap Kodu", "Evrak No", "Detay Açıklama", "Borç", "Alacak"} {
		if _, ok := col[h]; !ok {
			t.Fatalf("Luca header without %q: %v", h, rows[0])
		}
	}
	cell := func(r []string, h string) string {
		if i := col[h]; i < len(r) {
			return strings.TrimSpace(r[i])
		}
		return ""
	}
	num := func(s string) decimal.Decimal {
		if s == "" {
			return decimal.Zero
		}
		d, err := decimal.NewFromString(s)
		if err != nil {
			t.Fatalf("amount %q: %v", s, err)
		}
		return d
	}
	var out lucaFile
	for _, r := range rows[1:] {
		if len(r) == 0 {
			continue
		}
		out = append(out, lucaLine{file: name, fis: cell(r, "Fiş No"), account: cell(r, "Hesap Kodu"), docNo: cell(r, "Evrak No"),
			detail: cell(r, "Detay Açıklama"), debit: num(cell(r, "Borç")), credit: num(cell(r, "Alacak"))})
	}
	return out
}

// zTotal is the fake's Z report total.
const zTotal = "5430.00"

// vouchers groups the file's rows by voucher (file + Fiş No) and returns
// the vouchers with this Evrak No and this total (debit sum): the demo
// office's own documents may share a number.
func (f lucaFile) vouchers(docNo, total string) [][]lucaLine {
	byKey := map[string][]lucaLine{}
	var keys []string
	for _, l := range f {
		k := l.file + "|" + l.fis
		if _, ok := byKey[k]; !ok {
			keys = append(keys, k)
		}
		byKey[k] = append(byKey[k], l)
	}
	var out [][]lucaLine
	for _, k := range keys {
		lines := byKey[k]
		match := false
		var dr decimal.Decimal
		for _, l := range lines {
			match = match || l.docNo == docNo
			dr = dr.Add(l.debit)
		}
		if match && dr.StringFixed(2) == total {
			out = append(out, lines)
		}
	}
	return out
}

// has reports whether the file holds the voucher.
func (f lucaFile) has(docNo, total string) bool { return len(f.vouchers(docNo, total)) > 0 }

// voucher is the one voucher with this Evrak No and total.
func (f lucaFile) voucher(t *testing.T, docNo, total string) []lucaLine {
	t.Helper()
	v := f.vouchers(docNo, total)
	if len(v) != 1 {
		t.Fatalf("%d vouchers with Evrak No %s and total %s in the file (%d rows)", len(v), docNo, total, len(f))
	}
	return v[0]
}

func balanced(t *testing.T, what string, lines []lucaLine) decimal.Decimal {
	t.Helper()
	var dr, cr decimal.Decimal
	for _, l := range lines {
		dr, cr = dr.Add(l.debit), cr.Add(l.credit)
	}
	if !dr.Equal(cr) {
		t.Errorf("%s: debit %s ≠ credit %s %+v", what, dr, cr, lines)
	}
	return dr
}

// checkExpenseVoucher: one 191 line per VAT rate with these amounts, the
// total on credit, balanced.
func checkExpenseVoucher(t *testing.T, what string, lines []lucaLine, vat []string, total string) {
	t.Helper()
	var got []string
	for _, l := range lines {
		if strings.HasPrefix(l.account, "191") {
			got = append(got, l.debit.StringFixed(2))
			if !strings.HasPrefix(l.account, "191.") {
				t.Errorf("%s: VAT on the group account %s (want the rate's sub-account)", what, l.account)
			}
		}
	}
	if strings.Join(got, " ") != strings.Join(vat, " ") {
		t.Errorf("%s: VAT lines %v, want %v (%+v)", what, got, vat, lines)
	}
	if sum := balanced(t, what, lines); sum.StringFixed(2) != total {
		t.Errorf("%s: total %s, want %s", what, sum.StringFixed(2), total)
	}
}

// checkZVoucher: the fake's Z report (5.430,00; nakit 2.130, kart 3.300;
// %1 / %10 / %20) as 100 + 108 debit, 600 and 391 credit per rate.
func checkZVoucher(t *testing.T, what string, lines []lucaLine) {
	t.Helper()
	sums := map[string][]string{}
	for _, l := range lines {
		main := l.account
		if i := strings.IndexAny(main, ".-"); i > 0 {
			main = main[:i]
		}
		amt := l.debit
		if amt.IsZero() {
			amt = l.credit
		}
		sums[main] = append(sums[main], amt.StringFixed(2))
		if main == "391" && main == l.account {
			t.Errorf("%s: VAT on the group account 391 (want the rate's sub-account)", what)
		}
	}
	want := map[string]string{"100": "2130.00", "108": "3300.00", "600": "1000.00 1000.00 2618.00", "391": "10.00 100.00 702.00"}
	for main, w := range want {
		if got := strings.Join(sortedAmounts(sums[main]), " "); got != w {
			t.Errorf("%s: %s lines %s, want %s (%+v)", what, main, got, w, lines)
		}
	}
	if sum := balanced(t, what, lines); sum.StringFixed(2) != zTotal {
		t.Errorf("%s: total %s", what, sum.StringFixed(2))
	}
}

func sortedAmounts(v []string) []string {
	out := append([]string(nil), v...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && decimal.RequireFromString(out[j]).LessThan(decimal.RequireFromString(out[j-1])); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
