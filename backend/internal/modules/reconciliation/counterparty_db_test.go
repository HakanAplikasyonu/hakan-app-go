package reconciliation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	reviewrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/repository"
	reviewusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/usecase"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	platformmail "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/mail"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/jackc/pgx/v5"
)

type fakeMail struct {
	mu   sync.Mutex
	sent []platformmail.Message
}

func (f *fakeMail) Send(_ context.Context, m platformmail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

// #389: cari mutabakat (mizan 120/320 bakiyesi ↔ fatura + eşleşen ödeme),
// BA/BS beyanı içe aktarma ve karşılaştırma, mutabakat mektubu: oluştur →
// worker e-postası → bağlantıdan yanıt → cari satırı çözüldü.
func TestCounterpartyReconciliationAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t).Pool
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		must(err)
	}
	tag := time.Now().Format("150405.000000")
	var org, tp, eng, user int64
	must(pool.QueryRow(ctx, `INSERT INTO organizations (slug, name) VALUES ($1, 'Demo Müşavirlik') RETURNING id`, "cp-"+tag).Scan(&org))
	must(pool.QueryRow(ctx, `INSERT INTO taxpayers (kind, tax_id, title) VALUES ('legal', '4730074556', 'Atlas Gıda A.Ş.') RETURNING id`).Scan(&tp))
	must(pool.QueryRow(ctx, `INSERT INTO engagements (organization_id, taxpayer_id) VALUES ($1, $2) RETURNING id`, org, tp).Scan(&eng))
	must(pool.QueryRow(ctx, `INSERT INTO users (email, password_hash, name, surname) VALUES ($1, 'x', 'A', 'B') RETURNING id`, tag+"@example.test").Scan(&user))

	var imp int64
	must(pool.QueryRow(ctx, `INSERT INTO account_plan_imports (organization_id, engagement_id, source_format, source_kind, sha256, period, file_name)
		VALUES ($1, $2, 'xlsx', 'trial_balance', repeat('c', 64), '2026-10', 'mizan.xlsx') RETURNING id`, org, eng).Scan(&imp))
	for _, l := range [][3]string{{"120.01", "12000", "2000"}, {"320.05", "0", "5500"}, {"320.09", "0", "750"}} {
		exec(`INSERT INTO trial_balance_lines (organization_id, engagement_id, import_id, code, name, main_code, level, is_leaf, debit, credit, balance)
			VALUES ($1, $2, $3, $4::text, $4::text, left($4::text, 3), 2, true, $5::numeric, $6::numeric, $5::numeric - $6::numeric)`, org, eng, imp, l[0], l[1], l[2])
	}
	for _, a := range [][3]string{{"1111111111", "120", "120.01"}, {"2222222222", "320", "320.05"}, {"4444444444", "320", "320.09"}} {
		exec(`INSERT INTO counterparty_accounts (organization_id, engagement_id, taxpayer_id, counterparty_tax_id, main_code, account_code, account_name)
			VALUES ($1, $2, $3, $4, $5, $6, 'Cari')`, org, eng, tp, a[0], a[1], a[2])
	}
	invoice := func(src, dir, taxID, name, issue, net, tax string) int64 {
		var id int64
		must(pool.QueryRow(ctx, `INSERT INTO invoices (taxpayer_id, source_system, source_id, capture_method, invoice_number, direction, type,
				issue_date, period, currency, customer_tax_number, customer_name, tax_exclusive_amount, tax_amount, tax_inclusive_amount,
				payable_amount, tax_summary_source)
			VALUES ($1, 'test', $2::text, 'manual', $2::text, $3, 'SATIS', $4::date, to_char($4::date, 'YYYY-MM'), 'TRY', $5, $6, $7::numeric, $8::numeric,
				$7::numeric + $8::numeric, $7::numeric + $8::numeric, 'header_estimate') RETURNING id`,
			tp, src, dir, issue, taxID, name, net, tax).Scan(&id))
		return id
	}
	sale := invoice("S1", "outgoing", "1111111111", "Alıcı A.Ş.", "2026-10-10", "10000", "2000")
	invoice("P1", "incoming", "2222222222", "Satıcı Ltd.", "2026-10-12", "3000", "600")
	invoice("P2", "incoming", "2222222222", "Satıcı Ltd.", "2026-10-15", "2000", "400")
	invoice("P0", "incoming", "2222222222", "Satıcı Ltd.", "2026-09-01", "1000", "0") // eski dönem: bakiye var, BA yok
	// Alıcının 2.000 TL ödemesi faturaya eşleşmiş.
	var box, txn int64
	must(pool.QueryRow(ctx, `INSERT INTO cash_boxes (taxpayer_id, name, currency) VALUES ($1, 'Banka', 'TRY') RETURNING id`, tp).Scan(&box))
	must(pool.QueryRow(ctx, `INSERT INTO bank_transactions (taxpayer_id, cash_box_id, source_system, source_id, capture_method, booked_at,
			transaction_date, debit_credit, amount, currency, description, period)
		VALUES ($1, $2, 'bank_test', 'b1', 'api', '2026-10-20', '2026-10-20', 'A', 2000, 'TRY', 'tahsilat', '2026-10') RETURNING id`, tp, box).Scan(&txn))
	exec(`INSERT INTO bank_transaction_matches (taxpayer_id, bank_transaction_id, target_type, target_id, amount, score, method)
		VALUES ($1, $2, 'invoice', $3, 2000, 100, 'manual')`, tp, txn, sale)
	exec(`INSERT INTO customers (taxpayer_id, source_system, source_id, capture_method, name, tax_number, email) VALUES ($1, 'test', 'c1', 'manual', 'Alıcı A.Ş.', '1111111111', 'muhasebe@alici.example')`, tp)
	// E-postası olmayan cari kartı: kişi listesi NULL e-posta okumamalı (#452).
	exec(`INSERT INTO customers (taxpayer_id, source_system, source_id, capture_method, name, tax_number) VALUES ($1, 'test', 'c2', 'manual', 'Satıcı Ltd.', '2222222222')`, tp)

	reviewSvc := reviewusecase.New(reviewrepository.New(pool, db.New(pool))).Register(SuggestionKind, Applier{})
	svc := New(database.Ambient{Pool: pool}, reviewSvc).WithLetters(LetterConfig{Secret: "gizli", FrontendURL: "https://app.test"})
	sc := engagementctx.Scope{ID: eng, OrganizationID: org, TaxpayerID: tp}

	// BA beyanı: satıcı 5.000 / 2 belge; fazladan bir satır; bozuk satır.
	csv := "Form Ba - Ekim 2026\nVKN/TCKN;Adı Soyadı/Unvanı;Belge Sayısı;Mal ve Hizmet Bedeli\n" +
		"2222222222;Satıcı Ltd.;2;5.000\n5555555555;Fazla A.Ş.;1;7.500\n12;Bozuk;1;10\n"
	res, err := svc.ImportDeclarations(ctx, sc, user, "2026-10", "ba", "ba.csv", []byte(csv))
	must(err)
	if res.Imported != 2 || len(res.Errors) != 1 || res.Errors[0].Line != 5 {
		t.Fatalf("import %+v", res)
	}
	if _, err := svc.ImportDeclarations(ctx, sc, user, "2026-10", "ba", "x.csv", []byte("a;b\n1;2\n")); !errors.Is(err, ErrNoBABSHeader) {
		t.Fatalf("no header: %v", err)
	}
	decl, err := svc.Declarations(ctx, sc, "2026-10", "")
	must(err)
	if len(decl) != 2 || decl[1].Amount != "5000.00" || decl[1].Name != "Satıcı Ltd." {
		t.Fatalf("declarations %+v", decl)
	}

	det, err := svc.Run(ctx, sc, user, RunInput{Period: "2026-10"})
	must(err)
	if det.Run.Summary.Counterparties != 4 || det.Run.Summary.BABSIssues != 1 || det.Run.Summary.CounterpartyDifferences != 2 {
		t.Fatalf("summary %+v", det.Run.Summary)
	}
	page, err := svc.Counterparties(ctx, sc, "latest", "")
	must(err)
	by := map[string]CounterpartyView{}
	for _, c := range page.Items {
		by[c.TaxID] = c
	}
	a := by["1111111111"]
	if a.Result != CPMatched || a.SourceBalance != "10000.00" || *a.LedgerBalance != "10000.00" || a.BSResult != BABSNoDeclaration || a.Email == nil {
		t.Fatalf("alıcı %+v", a)
	}
	s := by["2222222222"]
	// Kaynak −7.000 (3.600 + 2.400 + 1.000), mizan −5.500 → fark 1.500; BA tutuyor.
	if s.Result != CPDifference || *s.Difference != "1500.00" || s.BAResult != BABSOK || s.BAAmount != "5000.00" || s.BACount != 2 {
		t.Fatalf("satıcı %+v", s)
	}
	if f := by["5555555555"]; f.BAResult != BABSExtraInForm || f.Result != CPNoLedgerAccount || f.Name != "Fazla A.Ş." {
		t.Fatalf("fazla %+v", f)
	}
	if _, err := svc.SendCounterpartyToReview(ctx, sc, a.UUID, user, nil); !errors.Is(err, ErrMatched) {
		t.Fatalf("matched to review: %v", err)
	}
	v, err := svc.SendCounterpartyToReview(ctx, sc, s.UUID, user, nil)
	must(err)
	if v.Status != StatusInReview || v.Review == nil {
		t.Fatalf("review %+v", v)
	}
	sub := reviewusecase.Subject{OrganizationID: org, EngagementID: eng, TaxpayerID: tp}
	if _, err := reviewSvc.Accept(ctx, sub, v.Review.UUID, user, ""); err != nil {
		t.Fatal(err)
	}
	page, err = svc.Counterparties(ctx, sc, "latest", "")
	must(err)
	for _, c := range page.Items {
		if c.TaxID == "2222222222" && (c.Status != StatusResolved || c.Note == nil || !strings.Contains(*c.Note, "Cari bakiye")) {
			t.Fatalf("accepted %+v", c)
		}
	}
	note := "Eski tedarikçi, kapanacak"
	if _, err := svc.DecideCounterparty(ctx, sc, by["4444444444"].UUID, user, DecisionInput{Status: StatusIgnored, Note: &note}); err != nil {
		t.Fatal(err)
	}

	// Mektup: e-postasız gönderilmez; satıcıya taslak, alıcıya kuyruk.
	if _, err := svc.CreateLetter(ctx, sc, user, LetterInput{CounterpartyUUID: s.UUID, Send: true}); !errors.Is(err, ErrNoRecipient) {
		t.Fatalf("no recipient: %v", err)
	}
	bad := "olmaz"
	if _, err := svc.CreateLetter(ctx, sc, user, LetterInput{CounterpartyUUID: s.UUID, Email: &bad}); !errors.Is(err, ErrBadRecipient) {
		t.Fatalf("bad email: %v", err)
	}
	email := "muhasebe@alici.example"
	letter, err := svc.CreateLetter(ctx, sc, user, LetterInput{CounterpartyUUID: a.UUID, Email: &email, Send: true})
	must(err)
	if letter.Status != "queued" || letter.Balance != "10000.00" || letter.AsOfDate != "2026-10-31" || letter.Snapshot.Office != "Demo Müşavirlik" || letter.ExpiresAt == nil {
		t.Fatalf("letter %+v", letter)
	}
	pdf, name, err := svc.LetterPDF(ctx, sc, letter.UUID)
	must(err)
	if len(pdf) < 1000 || !strings.HasSuffix(name, ".pdf") {
		t.Fatalf("pdf %d %s", len(pdf), name)
	}

	mailer := &fakeMail{}
	sender := &LetterSender{Q: db.New(pool), Mail: mailer, Config: LetterConfig{Secret: "gizli", FrontendURL: "https://app.test"}}
	var sent int
	must(rls.System(ctx, pool, "test: letters", func(ctx context.Context, _ pgx.Tx) error {
		var err error
		sent, err = sender.SendDue(ctx)
		return err
	}))
	token := LetterToken("gizli", letter.UUID)
	if sent != 1 || len(mailer.sent) != 1 || mailer.sent[0].To[0] != email || !strings.Contains(mailer.sent[0].Body, "https://app.test/mutabakat/"+token) ||
		len(mailer.sent[0].Attachments) != 1 {
		t.Fatalf("sent %d %+v", sent, mailer.sent)
	}

	// Bağlantı: yanlış jeton bulunmaz; doğru jetonla görünür ve yanıtlanır.
	if _, err := svc.PublicLetterByToken(ctx, strings.Repeat("0", 64)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad token: %v", err)
	}
	pub, err := svc.PublicLetterByToken(ctx, token)
	must(err)
	if pub.CounterpartyName != "Alıcı A.Ş." || pub.Balance != "10000.00" || pub.Response != nil {
		t.Fatalf("public %+v", pub)
	}
	if _, err := svc.PublicRespond(ctx, token, ResponseInput{Response: "disputed"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("dispute without balance: %v", err)
	}
	who := "  Ayşe   Yılmaz "
	pub, err = svc.PublicRespond(ctx, token, ResponseInput{Response: "agreed", ResponderName: &who})
	must(err)
	if pub.Response == nil || *pub.Response != "agreed" {
		t.Fatalf("responded %+v", pub)
	}
	if _, err := svc.PublicRespond(ctx, token, ResponseInput{Response: "agreed"}); !errors.Is(err, ErrLetterAnswered) {
		t.Fatalf("answer twice: %v", err)
	}
	letters, err := svc.Letters(ctx, sc, "1111111111")
	must(err)
	if len(letters) != 1 || letters[0].Status != "responded" || *letters[0].ResponderName != "Ayşe Yılmaz" || *letters[0].ResponseChannel != "link" || letters[0].SentAt == nil {
		t.Fatalf("letters %+v", letters)
	}

	// Elle yanıt: satıcı mutabık değil.
	draft, err := svc.CreateLetter(ctx, sc, user, LetterInput{CounterpartyUUID: s.UUID})
	must(err)
	if draft.Status != "draft" || draft.Balance != "-5500.00" {
		t.Fatalf("draft %+v", draft)
	}
	bal := "-6600,00"
	got, err := svc.RecordResponse(ctx, sc, draft.UUID, ResponseInput{Response: "disputed", Balance: &bal})
	must(err)
	if *got.ResponseBalance != "-6600.00" || *got.ResponseChannel != "manual" {
		t.Fatalf("manual %+v", got)
	}
	if _, err := svc.SendLetter(ctx, sc, draft.UUID, &email); !errors.Is(err, ErrLetterAnswered) {
		t.Fatalf("send answered: %v", err)
	}

	// Yeniden çalıştırma: kararlar taşınır.
	_, err = svc.Run(ctx, sc, user, RunInput{Period: "2026-10"})
	must(err)
	page, err = svc.Counterparties(ctx, sc, "latest", "")
	must(err)
	for _, c := range page.Items {
		switch c.TaxID {
		case "4444444444":
			if c.Status != StatusIgnored || c.Note == nil {
				t.Fatalf("carried %+v", c)
			}
		case "1111111111":
			if c.LastLetter == nil || c.LastLetter.Response == nil || *c.LastLetter.Response != "agreed" {
				t.Fatalf("last letter %+v", c)
			}
		}
	}
}
