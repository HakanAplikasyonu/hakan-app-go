package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeTransfers sunucunun aktarım uçlarını bellekte taklit eder.
type fakeTransfers struct {
	mu        sync.Mutex
	items     map[string]*RemoteTransfer // uuid → görev
	eng       map[string]string          // uuid → engagement
	files     map[string][]byte          // batch/name → içerik
	reports   map[string]DeliveryReport
	claims    int
	downErr   error
	claimedBy map[string]string
}

func newFakeTransfers() *fakeTransfers {
	return &fakeTransfers{items: map[string]*RemoteTransfer{}, eng: map[string]string{}, files: map[string][]byte{},
		reports: map[string]DeliveryReport{}, claimedBy: map[string]string{}}
}

func sum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func (f *fakeTransfers) add(eng, id, batch, driver string, files map[string][]byte) {
	t := &RemoteTransfer{UUID: id, BatchUUID: batch, Period: "2026-10", Driver: driver, Status: "queued"}
	for n, b := range files {
		t.Files = append(t.Files, RemoteFile{Name: n, SHA256: sum(b), Size: int64(len(b))})
		f.files[batch+"/"+n] = b
	}
	f.items[id], f.eng[id] = t, eng
}

func (f *fakeTransfers) ListTransfers(_ context.Context, eng, status string) ([]RemoteTransfer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []RemoteTransfer
	for id, t := range f.items {
		if f.eng[id] == eng && t.Status == status {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (f *fakeTransfers) ClaimTransfer(_ context.Context, _, id, agent string) (RemoteTransfer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claims++
	t := f.items[id]
	if t.Status != "queued" && (t.Status != "claimed" || f.claimedBy[id] != agent) {
		return RemoteTransfer{}, &APIError{Status: 409, Code: "TRANSFER_STATE"}
	}
	t.Status, f.claimedBy[id] = "claimed", agent
	return *t, nil
}

func (f *fakeTransfers) ReportDelivery(_ context.Context, _, id string, rep DeliveryReport) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reports[id] = rep
	f.items[id].Status = rep.Outcome
	return nil
}

func (f *fakeTransfers) DownloadExportFile(_ context.Context, _, batch, name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.downErr != nil {
		return nil, f.downErr
	}
	b, ok := f.files[batch+"/"+name]
	if !ok {
		return nil, &APIError{Status: 404, Code: "NOT_FOUND"}
	}
	return b, nil
}

func transferSetup(t *testing.T) (*Transferrer, *fakeTransfers, string) {
	t.Helper()
	st, err := LoadState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "Luca aktarım")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = st.Update(func(d *stateData) {
		d.AgentUUID = "a-1"
		d.Targets = []TransferTarget{{EngagementUUID: "e-1", Driver: "luca", Path: dir, TaxpayerTitle: "Atlas Gıda"}}
	})
	api := newFakeTransfers()
	return &Transferrer{State: st, API: api}, api, dir
}

// #391: bekleyen Luca görevi klasöre yazılır, özetle bildirilir; ikinci tur
// bir şey yapmaz. Zirve görevi (klasör yok) ve başka mükellefinki alınmaz.
func TestTransferDeliversToFolder(t *testing.T) {
	tr, api, dir := transferSetup(t)
	content := []byte("PK luca dosyası")
	api.add("e-1", "t-1", "0f1e2d3c-aaaa-bbbb-cccc-000000000001", "luca", map[string][]byte{"luca_2026-10.xlsx": content})
	api.add("e-1", "t-2", "b-2", "zirve", map[string][]byte{"zirve_2026-10.xlsx": []byte("z")})
	api.add("e-9", "t-3", "b-3", "luca", map[string][]byte{"luca_2026-10.xlsx": []byte("x")})

	tr.Run(context.Background())

	name := "luca_2026-10_0f1e2d3c.xlsx"
	got, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || string(got) != string(content) {
		t.Fatalf("file %q %v", got, err)
	}
	rep := api.reports["t-1"]
	if rep.Outcome != "delivered" || rep.AgentUUID != "a-1" || rep.FolderLabel != "Luca aktarım" ||
		len(rep.Files) != 1 || rep.Files[0].Name != name || rep.Files[0].SHA256 != sum(content) {
		t.Fatalf("report %+v", rep)
	}
	if strings.Contains(rep.FolderLabel, string(filepath.Separator)) {
		t.Fatalf("full path sent: %q", rep.FolderLabel)
	}
	if api.items["t-2"].Status != "queued" || api.items["t-3"].Status != "queued" {
		t.Fatal("tasks without a folder were taken")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("folder has %d entries (temp file left?)", len(entries))
	}
	claims := api.claims
	tr.Run(context.Background())
	if api.claims != claims {
		t.Fatal("delivered task claimed again")
	}
	if a := tr.Activity(); len(a.Recent) != 1 || a.Recent[0].Outcome != "delivered" || a.Recent[0].TaxpayerTitle != "Atlas Gıda" {
		t.Fatalf("activity %+v", a)
	}
}

// Kalıcı hatalar Türkçe nedenle bildirilir; ağ hatası bildirilmez, görev
// üstlenilmiş kalır ve sonraki turda aynı ajan sürdürür.
func TestTransferFailures(t *testing.T) {
	tr, api, dir := transferSetup(t)

	api.add("e-1", "t-1", "b-1", "luca", map[string][]byte{"a.xlsx": []byte("yeni")})
	if err := os.WriteFile(filepath.Join(dir, TransferFileName("a.xlsx", "b-1")), []byte("eski"), 0o600); err != nil {
		t.Fatal(err)
	}
	tr.Run(context.Background())
	if r := api.reports["t-1"]; r.Outcome != "failed" || r.ErrorCode != "FILE_EXISTS" || r.ErrorMessage == "" {
		t.Fatalf("existing file %+v", r)
	}

	api.add("e-1", "t-2", "b-2", "luca", map[string][]byte{"b.xlsx": []byte("b")})
	api.downErr = &APIError{Status: 503, Code: "UNAVAILABLE"}
	tr.Run(context.Background())
	if _, ok := api.reports["t-2"]; ok || api.items["t-2"].Status != "claimed" {
		t.Fatalf("transient error reported: %+v", api.reports["t-2"])
	}
	if a := tr.Activity(); a.Recent[0].Outcome != "retry" || a.Recent[0].Reason == "" {
		t.Fatalf("retry event %+v", a.Recent[0])
	}
	api.downErr = nil
	tr.Run(context.Background())
	if r := api.reports["t-2"]; r.Outcome != "delivered" {
		t.Fatalf("resumed claim %+v", r)
	}

	api.add("e-1", "t-3", "b-3", "luca", map[string][]byte{"c.xlsx": []byte("c")})
	api.files["b-3/c.xlsx"] = []byte("değişmiş")
	tr.Run(context.Background())
	if r := api.reports["t-3"]; r.ErrorCode != "CHECKSUM_MISMATCH" {
		t.Fatalf("checksum %+v", r)
	}

	api.add("e-1", "t-4", "b-4", "luca", map[string][]byte{"d.xlsx": []byte("d")})
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	tr.Run(context.Background())
	if r := api.reports["t-4"]; r.ErrorCode != "FOLDER_MISSING" || !strings.Contains(r.ErrorMessage, "klasörü bulunamadı") {
		t.Fatalf("folder missing %+v", r)
	}
}

func TestTransferFileName(t *testing.T) {
	for in, want := range map[string]string{
		"luca_2026-10.xlsx":    "luca_2026-10_12345678.xlsx",
		"../../etc/passwd":     "passwd_12345678",
		`..\..\windows\x.xlsx`: "x_12345678.xlsx",
		"zirve_2026-10_2.xlsx": "zirve_2026-10_2_12345678.xlsx",
	} {
		if got := TransferFileName(in, "12345678-9abc-def0-0000-000000000000"); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

// İstemci uçları: liste, üstlenme, teslim ve zarfsız dosya indirme.
func TestTransferClient(t *testing.T) {
	var gotReport map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/tenant/engagements/e-1/journal-transfers", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "queued" || r.Header.Get("X-Organization") != "ofis" {
			fail(w, 400, "BAD")
			return
		}
		ok(w, 200, map[string]any{"items": []map[string]any{{"uuid": "t-1", "batch_uuid": "b-1", "driver": "luca", "status": "queued",
			"files": []map[string]any{{"name": "a.xlsx", "sha256": "x", "size": 1}}}}, "total": 1})
	})
	mux.HandleFunc("POST /v1/tenant/engagements/e-1/journal-transfers/t-1/claim", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, 200, map[string]any{"uuid": "t-1", "status": "claimed"})
	})
	mux.HandleFunc("POST /v1/tenant/engagements/e-1/journal-transfers/t-1/delivery", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReport)
		ok(w, 200, map[string]any{"uuid": "t-1", "status": "delivered"})
	})
	mux.HandleFunc("GET /v1/tenant/engagements/e-1/journal-exports/b-1/files/a.xlsx", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			fail(w, 401, "UNAUTHENTICATED")
			return
		}
		_, _ = w.Write([]byte("PKdata"))
	})
	mux.HandleFunc("GET /v1/tenant/engagements/e-1/journal-exports/b-1/files/yok.xlsx", func(w http.ResponseWriter, _ *http.Request) {
		fail(w, 404, "NOT_FOUND")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewClient(nil)
	c.SetBase(srv.URL)
	c.SetOrganization("ofis")
	c.setTokens(Tokens{AccessToken: "tok", RefreshToken: "r", ExpiresIn: 900})
	ctx := context.Background()

	items, err := c.ListTransfers(ctx, "e-1", "queued")
	if err != nil || len(items) != 1 || items[0].Files[0].Name != "a.xlsx" {
		t.Fatalf("list %+v %v", items, err)
	}
	if _, err := c.ClaimTransfer(ctx, "e-1", "t-1", "a-1"); err != nil {
		t.Fatal(err)
	}
	if err := c.ReportDelivery(ctx, "e-1", "t-1", DeliveryReport{AgentUUID: "a-1", Outcome: "delivered"}); err != nil || gotReport["outcome"] != "delivered" {
		t.Fatalf("report %v %v", gotReport, err)
	}
	data, err := c.DownloadExportFile(ctx, "e-1", "b-1", "a.xlsx")
	if err != nil || string(data) != "PKdata" {
		t.Fatalf("download %q %v", data, err)
	}
	var ae *APIError
	if _, err := c.DownloadExportFile(ctx, "e-1", "b-1", "yok.xlsx"); err == nil || !asAPI(err, &ae) || ae.Status != 404 {
		t.Fatalf("missing file err = %v", err)
	}
}

func asAPI(err error, target **APIError) bool {
	e, ok := err.(*APIError)
	if ok {
		*target = e
	}
	return ok
}

// Runtime: hedef ekleme doğrulaması ve durumda görünmesi.
func TestRuntimeTargets(t *testing.T) {
	rt, _, _, _ := setup(t)
	dir := t.TempDir()
	if _, err := rt.AddTarget("goreli/yol", "e-1", "luca", ""); err == nil {
		t.Fatal("relative path accepted")
	}
	if _, err := rt.AddTarget(dir, "e-1", "mikro", ""); err == nil {
		t.Fatal("unknown program accepted")
	}
	if _, err := rt.AddTarget(dir, "e-1", "luca", "Atlas"); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if _, err := rt.AddTarget(other, "e-1", "luca", "Atlas"); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.AddTarget(dir, "e-1", "zirve", "Atlas"); err != nil {
		t.Fatal(err)
	}
	st := rt.Status()
	if len(st.Targets) != 2 || st.Targets[0].Path != other {
		t.Fatalf("targets %+v", st.Targets)
	}
	if err := rt.RemoveTarget("e-1", "luca"); err != nil {
		t.Fatal(err)
	}
	if st := rt.Status(); len(st.Targets) != 1 || st.Targets[0].Driver != "zirve" {
		t.Fatalf("after remove %+v", st.Targets)
	}
}
