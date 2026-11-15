package entegrator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

const ownVKN = "1288331521"

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

var (
	uuidRe = regexp.MustCompile(`(<UUID[^>]*>)[^<]+(</UUID>)`)
	idRe   = regexp.MustCompile(`(<ID xmlns="urn:oasis:names:specification:ubl:schema:xsd:CommonBasicComponents-2">)GIB2026[0-9]{9}(</ID>)`)
)

// outgoingXML is a sales invoice issued by the taxpayer; incomingXML swaps
// the parties so the taxpayer is the customer.
func invoiceXML(t *testing.T, n int, incoming bool, profile string) ([]byte, string) {
	t.Helper()
	b, err := os.ReadFile("testdata/temelfatura-satis.xml")
	if err != nil {
		t.Fatal(err)
	}
	ettn := uuid.NewString()
	b = uuidRe.ReplaceAll(b, []byte("${1}"+ettn+"${2}"))
	b = idRe.ReplaceAll(b, []byte(fmt.Sprintf("${1}TST2026%09d${2}", n)))
	if profile != "" {
		b = bytes.ReplaceAll(b, []byte(">TEMELFATURA<"), []byte(">"+profile+"<"))
	}
	if incoming {
		b = bytes.ReplaceAll(b, []byte(`schemeID="VKN">`+ownVKN), []byte(`schemeID="VKN">4840847211`))
		b = bytes.ReplaceAll(b, []byte(`schemeID="TCKN">12345678901`), []byte(`schemeID="VKN">`+ownVKN))
	}
	return b, ettn
}

// spy is the fake integrator with every write method counted (and refused).
type spy struct {
	*providertest.Fake
	writes  atomic.Int32
	listErr error
	minPoll time.Duration
}

func (s *spy) Send(context.Context, *provider.Document) (*provider.SendResult, error) {
	s.writes.Add(1)
	return nil, errors.New("write called")
}
func (s *spy) Respond(context.Context, provider.Response) error {
	s.writes.Add(1)
	return errors.New("write called")
}
func (s *spy) CancelEArchive(context.Context, string, string) error {
	s.writes.Add(1)
	return errors.New("write called")
}
func (s *spy) MarkTaken(context.Context, provider.Direction, ...string) error {
	s.writes.Add(1)
	return errors.New("write called")
}
func (s *spy) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.Fake.ListInbox(ctx, q)
}
func (s *spy) Capabilities() provider.Capabilities {
	c := s.Fake.Capabilities()
	c.MinPollInterval = s.minPoll
	return c
}

type env struct {
	pool *pgxpool.Pool
	q    *db.Queries
	tp   int64
	conn db.SourceConnection
	fake *providertest.Fake
	spy  *spy
	svc  *Service
	now  time.Time
}

func setup(t *testing.T, cfg Config) *env {
	t.Helper()
	d := testdb.Open(t)
	org := d.Organization(t)
	tp := d.Taxpayer(t, "legal", ownVKN)
	d.Engagement(t, org, tp)
	q := db.New(d.Pool)
	conn, err := q.CreateSourceConnection(context.Background(), db.CreateSourceConnectionParams{
		TaxpayerID: tp, SourceSystem: "entegrator_fake", SetupState: "active", Meta: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	e := &env{pool: d.Pool, q: q, tp: tp, conn: conn, fake: providertest.NewFake(), now: mustTime(t, "2026-11-15T10:00:00+03:00")}
	e.fake.SetClock(func() time.Time { return mustTime(t, "2026-11-10T09:00:00+03:00") })
	e.spy = &spy{Fake: e.fake}
	if cfg.RequestGap == 0 {
		cfg.RequestGap = -1
	}
	e.svc = New(d.Pool, nil, storage.NewMemory(), cfg, nil).
		WithOpener(func(_ context.Context, c db.SourceConnection, own string) (*Client, error) {
			if c.ID != conn.ID || own != ownVKN {
				return nil, errors.New("unexpected connection")
			}
			return Wrap(e.spy), nil
		}).
		WithClock(func() time.Time { return e.now })
	return e
}

func (e *env) deliver(t *testing.T, n int, profile string, status provider.Status, created time.Time) string {
	t.Helper()
	xml, ettn := invoiceXML(t, n, true, profile)
	e.fake.Deliver(provider.DocumentInfo{Kind: provider.EInvoice, UUID: strings.ToUpper(ettn), ID: fmt.Sprintf("TST2026%09d", n),
		Profile: profile, Status: status, IssueDate: "2026-11-01", CreatedAt: created}, xml)
	return ettn
}

func (e *env) send(t *testing.T, n int) string {
	t.Helper()
	xml, ettn := invoiceXML(t, n, false, "")
	if _, err := e.fake.Send(context.Background(), &provider.Document{Kind: provider.EInvoice, XML: xml}); err != nil {
		t.Fatal(err)
	}
	return ettn
}

func (e *env) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, append([]any{e.tp}, args...)...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) doc(t *testing.T, ettn, dir string) db.EntegratorDocument {
	t.Helper()
	d, err := e.q.GetEntegratorDocument(context.Background(), db.GetEntegratorDocumentParams{TaxpayerID: e.tp, ConnectionID: e.conn.ID, Ettn: ettn, Direction: dir})
	if err != nil {
		t.Fatalf("document %s/%s: %v", ettn, dir, err)
	}
	return d
}

func (e *env) invoiceStatus(t *testing.T, ettn string) string {
	t.Helper()
	var s string
	if err := e.pool.QueryRow(context.Background(), `SELECT status_normalized FROM invoices WHERE taxpayer_id = $1 AND ettn::text = $2`, e.tp, ettn).Scan(&s); err != nil {
		t.Fatalf("invoice %s: %v", ettn, err)
	}
	return s
}

// Kabul: sahte adaptörle uçtan uca senkron. Liste → indeks → UBL → fatura;
// durum değişikliği sonraki çalışmada faturaya işlenir; entegratörde hiçbir
// yazma metodu çağrılmaz ve belgeler "alındı" işaretlenmez.
func TestSyncEndToEnd(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{})
	e.now = mustTime(t, "2026-11-11T10:00:00+03:00") // incremental window covers the fake's clock
	created := mustTime(t, "2026-11-10T09:00:00+03:00")
	in1 := e.deliver(t, 1, "TEMELFATURA", provider.StatusReceived, created)
	in2 := e.deliver(t, 2, "TICARIFATURA", provider.StatusReceived, created.Add(time.Hour))
	in3 := e.deliver(t, 3, "TEMELFATURA", provider.StatusReceived, mustTime(t, "2025-03-04T12:00:00+03:00"))
	out1 := e.send(t, 4)
	out2 := e.send(t, 5)
	if err := e.fake.Advance(out1, provider.StatusDelivered, 1300); err != nil {
		t.Fatal(err)
	}
	if err := e.fake.Advance(out2, provider.StatusRejected, 1230); err != nil {
		t.Fatal(err)
	}

	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunBackfill)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "success" || rep.Counters.New != 5 || rep.Counters.UBLFetched != 5 {
		t.Fatalf("report %+v", rep)
	}
	if n := e.count(t, `SELECT count(*) FROM entegrator_documents WHERE taxpayer_id = $1 AND ubl_status = 'ok' AND invoice_id IS NOT NULL`); n != 5 {
		t.Fatalf("indexed with UBL = %d, want 5", n)
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 5 {
		t.Fatalf("invoices = %d, want 5", n)
	}
	if n := e.count(t, `SELECT count(*) FROM invoice_sources WHERE taxpayer_id = $1 AND source_system = 'entegrator_fake' AND capture_method = 'api'`); n != 5 {
		t.Fatalf("invoice_sources = %d, want 5", n)
	}
	if n := e.count(t, `SELECT count(*) FROM attachments WHERE taxpayer_id = $1 AND kind = 'ubl_xml' AND owner_type = 'invoice'`); n != 5 {
		t.Fatalf("ubl attachments = %d, want 5", n)
	}
	if d := e.doc(t, in2, "incoming"); len(d.Warnings) != 1 || d.Warnings[0] != WarnPendingResponse {
		t.Fatalf("ticari warnings = %v", d.Warnings)
	}
	if d := e.doc(t, in3, "incoming"); d.CreatedAtSource.Time.Year() != 2025 {
		t.Fatalf("previous-year document not listed: %+v", d)
	}
	if s := e.invoiceStatus(t, out2); s != StatusCancelled {
		t.Fatalf("rejected invoice status = %s", s)
	}
	if s := e.invoiceStatus(t, in1); s != StatusActive {
		t.Fatalf("received invoice status = %s", s)
	}
	var caps string
	_ = e.pool.QueryRow(ctx, `SELECT capabilities::text FROM source_connections WHERE id = $1`, e.conn.ID).Scan(&caps)
	if !strings.Contains(caps, `"e_invoice"`) {
		t.Fatalf("capabilities = %s", caps)
	}

	// Okurken durum değiştirmeme kuralı (30 §5).
	if w := e.spy.writes.Load(); w != 0 {
		t.Fatalf("write methods called %d times", w)
	}
	page, err := e.fake.ListInbox(ctx, provider.ListQuery{OnlyNew: true})
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("inbox documents marked taken: %v %v", page, err)
	}
	for _, it := range page.Items {
		if it.Status != provider.StatusReceived {
			t.Fatalf("incoming %s status changed to %s", it.UUID, it.Status)
		}
	}

	// Durum değişikliği: kabul edilen giden fatura iptal edildi.
	if err := e.fake.Advance(out1, provider.StatusCancelled, 0); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(2 * time.Hour)
	rep, err = e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Counters.New != 0 || rep.Counters.StatusChanged != 1 || rep.Counters.UBLFetched != 0 {
		t.Fatalf("incremental report %+v", rep)
	}
	if s := e.invoiceStatus(t, out1); s != StatusCancelled {
		t.Fatalf("status change not applied: %s", s)
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 5 {
		t.Fatalf("invoices after incremental = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM sync_runs WHERE taxpayer_id = $1 AND status = 'success'`); n != 2 {
		t.Fatalf("successful runs = %d", n)
	}
	if w := e.spy.writes.Load(); w != 0 {
		t.Fatalf("write methods called %d times", w)
	}
}

// Pencere bölme (37 §5.3): sayfa sınırını aşan pencere ikiye bölünür, bölme
// 1 güne kadar iner; her belge bir kez indekslenir.
func TestWindowSplit(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{PageSize: 2, MaxPagesPerWindow: 1})
	base := mustTime(t, "2026-10-01T10:00:00+03:00")
	var ettns []string
	for i := range 7 {
		ettns = append(ettns, e.deliver(t, i+1, "TEMELFATURA", provider.StatusReceived, base.Add(time.Duration(i)*96*time.Hour)))
	}
	// Three on the same day: the 1-day window pages to the end.
	for i := range 3 {
		ettns = append(ettns, e.deliver(t, 20+i, "TEMELFATURA", provider.StatusReceived, base.Add(time.Duration(i)*time.Minute)))
	}
	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunBackfill)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Counters.Splits == 0 {
		t.Fatalf("no split: %+v", rep.Counters)
	}
	if n := e.count(t, `SELECT count(*) FROM entegrator_documents WHERE taxpayer_id = $1`); n != len(ettns) {
		t.Fatalf("indexed = %d, want %d", n, len(ettns))
	}
	if n := e.count(t, `SELECT count(*) FROM sync_windows WHERE taxpayer_id = $1 AND status <> 'done'`); n != 0 {
		t.Fatalf("unfinished windows = %d", n)
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != len(ettns) {
		t.Fatalf("invoices = %d", n)
	}
}

// UBL'deki UUID listedeki ETTN'den farklıysa kayıt yazılmaz (37 §5.4).
func TestUUIDMismatch(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{})
	xml, _ := invoiceXML(t, 1, true, "")
	listed := uuid.NewString()
	e.fake.Deliver(provider.DocumentInfo{Kind: provider.EInvoice, UUID: listed, CreatedAt: mustTime(t, "2026-11-14T09:00:00+03:00")}, xml)
	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "partial" || rep.Counters.UBLFailed != 1 {
		t.Fatalf("report %+v", rep)
	}
	d := e.doc(t, listed, "incoming")
	if d.UblStatus != "failed" || !strings.Contains(string(d.UblError), CodeUUIDMismatch) || d.InvoiceID.Valid {
		t.Fatalf("document %+v / %s", d, d.UblError)
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 0 {
		t.Fatalf("invoices = %d", n)
	}
}

// Kimlik hatası çalışmayı durdurur ve bağlantıyı auth_failed yapar (37 §4).
func TestAuthFailure(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{})
	e.spy.listErr = &provider.Error{Kind: provider.KindAuth, Provider: "fake", Message: "bad password"}
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); kindOf(err) != provider.KindAuth {
		t.Fatalf("err = %v", err)
	}
	c, err := e.q.GetSourceConnection(ctx, db.GetSourceConnectionParams{TaxpayerID: e.tp, Uuid: e.conn.Uuid})
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "auth_failed" {
		t.Fatalf("connection status = %s", c.Status)
	}
	if n := e.count(t, `SELECT count(*) FROM sync_runs WHERE taxpayer_id = $1 AND status = 'failed'`); n != 1 {
		t.Fatalf("failed runs = %d", n)
	}
}

// Capabilities.MinPollInterval ve elle çekim alt sınırı uygulanır.
func TestPollInterval(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{})
	e.spy.minPoll = time.Hour
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil {
		t.Fatal(err)
	}
	// sync_runs.started_at is the database clock; the service clock must be
	// ahead of it by less than the interval.
	e.now = time.Now().Add(30 * time.Minute)
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); !errors.Is(err, ErrTooSoon) {
		t.Fatalf("err = %v, want ErrTooSoon", err)
	}
	e.now = time.Now().Add(61 * time.Minute)
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil {
		t.Fatalf("after interval: %v", err)
	}
	// Backfill is not throttled.
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunBackfill); err != nil {
		t.Fatal(err)
	}
}

// Tam tarama: önceden görülüp bu taramada görülmeyen belge silinmez,
// missing_since alır.
func TestFullScanMarksMissing(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{})
	ettn := e.deliver(t, 1, "TEMELFATURA", provider.StatusReceived, mustTime(t, "2026-11-10T09:00:00+03:00"))
	kept := e.deliver(t, 2, "TEMELFATURA", provider.StatusReceived, mustTime(t, "2026-11-11T09:00:00+03:00"))
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunFullScan); err != nil {
		t.Fatal(err)
	}
	keptXML, _ := e.fake.Fetch(ctx, provider.FetchQuery{Kind: provider.EInvoice, Direction: provider.Inbound, UUID: strings.ToUpper(kept)})
	// The document disappears at the source.
	e.fake = providertest.NewFake()
	e.spy.Fake = e.fake
	e.fake.Deliver(provider.DocumentInfo{Kind: provider.EInvoice, UUID: strings.ToUpper(kept), Status: provider.StatusReceived,
		CreatedAt: mustTime(t, "2026-11-11T09:00:00+03:00")}, keptXML.Data)
	// Same windows as the first scan: they must be listed again.
	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunFullScan)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Counters.Missing != 1 || rep.Counters.Windows == 0 {
		t.Fatalf("report %+v", rep.Counters)
	}
	if d := e.doc(t, ettn, "incoming"); !d.MissingSince.Valid || d.UblStatus != "ok" {
		t.Fatalf("document %+v", d)
	}
	if d := e.doc(t, kept, "incoming"); d.MissingSince.Valid {
		t.Fatalf("document still at the source marked missing: %+v", d)
	}
}

// noCursor behaves like İzibiz GetInvoice: LIMIT caps the result and there
// is no next page.
type noCursor struct{ *spy }

func (noCursor) Name() string { return "fake-nocursor" }

func (n noCursor) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	p, err := n.spy.ListInbox(ctx, q)
	if p != nil {
		p.Next = ""
	}
	return p, err
}

// İzibiz kuralı (36 §5.3): dönen kayıt sayısı LIMIT'e eşitse pencere bölünür;
// 1 günlük pencere hâlâ doluysa LIMIT_REACHED yazılır.
func TestNoCursorFullPageSplits(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{})
	RegisterListProfile("fake-nocursor", ListProfile{PageSize: 2, NoCursor: true})
	e.svc.WithOpener(func(context.Context, db.SourceConnection, string) (*Client, error) {
		return Wrap(noCursor{e.spy}), nil
	})
	base := mustTime(t, "2026-10-01T10:00:00+03:00")
	for i := range 6 {
		e.deliver(t, i+1, "TEMELFATURA", provider.StatusReceived, base.Add(time.Duration(i)*96*time.Hour))
	}
	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunBackfill)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Counters.Splits == 0 || rep.Counters.LimitReached != 0 || rep.Status != "success" {
		t.Fatalf("report %+v", rep)
	}
	if n := e.count(t, `SELECT count(*) FROM entegrator_documents WHERE taxpayer_id = $1`); n != 6 {
		t.Fatalf("indexed = %d, want 6", n)
	}

	// Three documents on one day with LIMIT 2: the day cannot be split.
	for i := range 3 {
		e.deliver(t, 30+i, "TEMELFATURA", provider.StatusReceived, mustTime(t, "2026-11-03T10:00:00+03:00").Add(time.Duration(i)*time.Minute))
	}
	rep, err = e.svc.Run(ctx, e.tp, e.conn.Uuid, RunFullScan)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Counters.LimitReached != 1 || rep.Status != "partial" {
		t.Fatalf("report %+v", rep)
	}
	if n := e.count(t, `SELECT count(*) FROM sync_windows WHERE taxpayer_id = $1 AND last_error->>'code' = $2`, CodeLimitReached); n != 1 {
		t.Fatalf("LIMIT_REACHED windows = %d", n)
	}
}

// endDateOnly records the listing bounds it receives.
type endDateOnly struct {
	*spy
	mu  sync.Mutex
	tos []time.Time
}

func (*endDateOnly) Name() string { return "fake-enddate" }

func (e *endDateOnly) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	e.mu.Lock()
	e.tos = append(e.tos, q.To)
	e.mu.Unlock()
	return e.spy.ListInbox(ctx, q)
}

// Uyumsoft CreateEndDate yalnızca tarih (33 §7.2): bitiş ertesi gün 00:00
// gönderilir, böylece pencerenin son günündeki belgeler kaçmaz.
func TestEndDateOnlyProfile(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{})
	RegisterListProfile("fake-enddate", ListProfile{EndDateOnly: true})
	p := &endDateOnly{spy: e.spy}
	e.svc.WithOpener(func(context.Context, db.SourceConnection, string) (*Client, error) { return Wrap(p), nil })
	e.now = mustTime(t, "2026-11-15T19:26:00+03:00")
	if _, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental); err != nil {
		t.Fatal(err)
	}
	want := mustTime(t, "2026-11-16T00:00:00+03:00")
	if len(p.tos) == 0 {
		t.Fatal("no inbox listing")
	}
	for _, to := range p.tos {
		if !to.Equal(want) {
			t.Fatalf("To = %s, want %s", to, want)
		}
	}
}
