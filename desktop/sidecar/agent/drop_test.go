package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// dropAPI, sürükle-bırakın kullandığı iki ucu taklit eder: ofis evrak
// yükleme (#827, tür + dönem) ve ajan belgesi (#831 yolu).
type dropAPI struct {
	mu    sync.Mutex
	typed []string // mükellef|tür|dönem|dosya
	docs  []string // ajan|klasör|mükellef|dosya
	// off: ofis türlü evrak yüklemeyi kapattı.
	off bool
}

func (f *dropAPI) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/auth/refresh", func(w http.ResponseWriter, _ *http.Request) {
		ok(w, 200, map[string]any{"access_token": "acc", "refresh_token": "ref", "expires_in": 900})
	})
	mux.HandleFunc("POST /v1/tenant/engagements/{eng}/document-uploads", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer acc" || r.Header.Get("X-Organization") != "atlas-ymm" {
			fail(w, 401, "UNAUTHENTICATED")
			return
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			fail(w, 400, "VALIDATION_ERROR")
			return
		}
		_, hdr, err := r.FormFile("file")
		if err != nil {
			fail(w, 400, "VALIDATION_ERROR")
			return
		}
		f.mu.Lock()
		off := f.off
		if !off {
			f.typed = append(f.typed, strings.Join([]string{r.PathValue("eng"), r.FormValue("doc_type"), r.FormValue("period"), hdr.Filename}, "|"))
		}
		f.mu.Unlock()
		switch {
		case off:
			fail(w, 409, "DOCUMENT_UPLOADS_OFF")
		case strings.HasPrefix(hdr.Filename, "kilitli"):
			fail(w, 409, "PERIOD_LOCKED")
		default:
			ok(w, 201, map[string]any{"uuid": "up-1", "doc_type": r.FormValue("doc_type"), "period": r.FormValue("period"), "status": "processed"})
		}
	})
	mux.HandleFunc("POST /v1/tenant/agents/{agent}/documents", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			fail(w, 400, "VALIDATION_ERROR")
			return
		}
		_, hdr, err := r.FormFile("file")
		if err != nil {
			fail(w, 400, "VALIDATION_ERROR")
			return
		}
		f.mu.Lock()
		f.docs = append(f.docs, strings.Join([]string{r.PathValue("agent"), r.FormValue("folder"), r.FormValue("engagement_uuid"), hdr.Filename}, "|"))
		f.mu.Unlock()
		ok(w, 201, map[string]any{"uuid": "doc-1", "kind": "payroll", "status": "imported"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func dropClient(srv *httptest.Server) *Client {
	c := NewClient(nil)
	c.SetBase(srv.URL)
	c.SetRefreshToken("ref-0")
	c.SetOrganization("atlas-ymm")
	return c
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDropRequestValidate(t *testing.T) {
	good := DropRequest{Paths: []string{"/a.pdf"}, EngagementUUID: "eng-1", Period: "2026-10", DocType: DropInvoice}
	if err := good.Validate(); err != nil {
		t.Fatalf("geçerli istek: %v", err)
	}
	for name, mut := range map[string]func(*DropRequest){
		"dosya yok":    func(r *DropRequest) { r.Paths = nil },
		"mükellef yok": func(r *DropRequest) { r.EngagementUUID = " " },
		"dönem boş":    func(r *DropRequest) { r.Period = "" },
		"13. ay":       func(r *DropRequest) { r.Period = "2026-13" },
		"gün var":      func(r *DropRequest) { r.Period = "2026-10-01" },
		"tür boş":      func(r *DropRequest) { r.DocType = "" },
		"tür bilinmez": func(r *DropRequest) { r.DocType = "payroll" },
	} {
		r := good
		mut(&r)
		if err := r.Validate(); err == nil {
			t.Errorf("%s: hata beklenirdi", name)
		}
	}
	for _, typ := range DropDocTypes {
		r := good
		r.DocType = typ
		if err := r.Validate(); err != nil {
			t.Errorf("%s geçerli olmalı: %v", typ, err)
		}
	}
}

// Klasör bırakılınca yalnız kendi dosyaları alınır; gizli / geçici dosya
// ve alt klasör atlanır, aynı dosya bir kez sayılır.
func TestExpandDrop(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "Ekim")
	if err := os.MkdirAll(filepath.Join(folder, "alt"), 0o700); err != nil {
		t.Fatal(err)
	}
	b := writeFile(t, folder, "b.pdf", "%PDF")
	a := writeFile(t, folder, "a.jpg", "jpg")
	writeFile(t, folder, ".DS_Store", "x")
	writeFile(t, folder, "~$mizan.xlsx", "x")
	writeFile(t, filepath.Join(folder, "alt"), "derin.pdf", "%PDF")
	single := writeFile(t, dir, "tek.pdf", "%PDF")

	got, err := ExpandDrop([]string{single, folder, a})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{single, a, b}; !reflect.DeepEqual(got, want) {
		t.Errorf("dosyalar: %v, beklenen %v", got, want)
	}
	if _, err := ExpandDrop([]string{filepath.Join(dir, "yok.pdf")}); err == nil {
		t.Error("olmayan dosya hata vermeli")
	}
	empty := filepath.Join(dir, "bos")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ExpandDrop([]string{empty}); err == nil {
		t.Error("boş klasör hata vermeli")
	}
}

// Seçilen mükellef, dönem ve tür türlü evrak ucuna gider; Luca dökümü ajan
// belgesi ucuna (mükellefle) gider; desteklenmeyen dosya hiç gönderilmez;
// kilitli ay Türkçe nedenle reddedilir.
func TestDropperUploadRequest(t *testing.T) {
	api := &dropAPI{}
	srv := api.server(t)
	dir := t.TempDir()
	files := []string{
		writeFile(t, dir, "fatura.pdf", "%PDF-1.7"),
		writeFile(t, dir, "fis.heic", "heic"),
		writeFile(t, dir, "mizan.xlsx", "PK\x03\x04"),
		writeFile(t, dir, "not.txt", "x"),
		writeFile(t, dir, "kilitli.pdf", "%PDF-1.7 kilit"),
	}
	var events []UploadEvent
	d := &Dropper{
		API:         dropClient(srv),
		Emit:        func(ev UploadEvent) { events = append(events, ev) },
		ConvertHEIC: func(context.Context, string) ([]byte, error) { return []byte("jpeg"), nil },
	}
	req := DropRequest{Paths: []string{dir}, EngagementUUID: "eng-1", Period: "2026-10", DocType: DropExpense}
	out := d.Upload(context.Background(), "agent-1", req, files)

	wantTyped := []string{
		"eng-1|expense|2026-10|fatura.pdf",
		"eng-1|expense|2026-10|fis.jpg",
		"eng-1|expense|2026-10|kilitli.pdf",
	}
	if !reflect.DeepEqual(api.typed, wantTyped) {
		t.Errorf("türlü yükleme: %v", api.typed)
	}
	if want := []string{"agent-1|" + DropFolder + "|eng-1|mizan.xlsx"}; !reflect.DeepEqual(api.docs, want) {
		t.Errorf("ajan belgesi: %v", api.docs)
	}
	outcomes := map[string]string{}
	for _, ev := range out {
		outcomes[ev.Name] = ev.Outcome
		if ev.Source != "drop" || ev.EngagementUUID != "eng-1" {
			t.Errorf("%s: kaynak / mükellef: %+v", ev.Name, ev)
		}
		if ev.Outcome != "uploaded" && ev.Reason == "" {
			t.Errorf("%s: Türkçe neden boş", ev.Name)
		}
	}
	want := map[string]string{"fatura.pdf": "uploaded", "fis.heic": "uploaded", "mizan.xlsx": "uploaded", "not.txt": "rejected", "kilitli.pdf": "rejected"}
	if !reflect.DeepEqual(outcomes, want) {
		t.Errorf("sonuçlar: %v", outcomes)
	}
	if len(events) != len(files) {
		t.Errorf("her dosya için olay: %d", len(events))
	}
	if !strings.Contains(out[4].Reason, "kilitli") {
		t.Errorf("kilitli ay nedeni: %q", out[4].Reason)
	}
	// Olay kabuğa JSON olarak gider; kaynak alanı taşınır.
	b, _ := json.Marshal(out[0])
	if !strings.Contains(string(b), `"source":"drop"`) {
		t.Errorf("olay: %s", b)
	}
}

// Ofis türlü evrak yüklemeyi kapattıysa dosya ajan belgesi ucuna (#831
// yolu) mükellefiyle gider.
func TestDropperFallsBackWhenTypedUploadsOff(t *testing.T) {
	api := &dropAPI{off: true}
	srv := api.server(t)
	dir := t.TempDir()
	p := writeFile(t, dir, "z.jpg", "jpg")
	d := &Dropper{API: dropClient(srv)}
	out := d.Upload(context.Background(), "agent-1", DropRequest{EngagementUUID: "eng-2", Period: "2026-09", DocType: DropZReport}, []string{p})
	if out[0].Outcome != "uploaded" {
		t.Fatalf("sonuç: %+v", out[0])
	}
	if want := []string{"agent-1|" + DropFolder + "|eng-2|z.jpg"}; !reflect.DeepEqual(api.docs, want) {
		t.Errorf("ajan belgesi: %v", api.docs)
	}
	// Ajan kaydı yoksa belge ucuna gidilmez.
	out = d.Upload(context.Background(), "", DropRequest{EngagementUUID: "eng-2", Period: "2026-09", DocType: DropZReport}, []string{p})
	if out[0].Outcome != "rejected" || len(api.docs) != 1 {
		t.Errorf("kayıtsız ajan: %+v, %v", out[0], api.docs)
	}
}

// Oturum ve ofis yokken sürükle-bırak kabul edilmez.
func TestRuntimeDropNeedsRunningAgent(t *testing.T) {
	rt, _, _, _ := setup(t)
	dir := t.TempDir()
	p := writeFile(t, dir, "a.pdf", "%PDF")
	if _, err := rt.Drop(DropRequest{Paths: []string{p}, EngagementUUID: "eng-1", Period: "2026-10", DocType: DropInvoice}); err == nil {
		t.Error("çalışmayan ajan hata vermeli")
	}
	if _, err := rt.Drop(DropRequest{Paths: []string{p}}); err == nil {
		t.Error("eksik seçim hata vermeli")
	}
}
