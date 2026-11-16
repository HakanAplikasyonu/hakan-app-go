package entegrator

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

type sovosDoc struct {
	uuid, number string
	incoming     bool
	created      time.Time
	xml          []byte
}

// fakeSovos answers getUBLList (one day per call) and getUBL (ZIP) the way
// the go-ubltr sovos module reads them, and records every operation.
type fakeSovos struct {
	mu   sync.Mutex
	docs []sovosDoc
	ops  []string
}

var (
	sovosOp   = regexp.MustCompile(`<(?:[A-Za-z0-9]+:)?(get[A-Za-z]+Request|send[A-Za-z]+|[a-z][A-Za-z]+Request)[ >]`)
	sovosType = regexp.MustCompile(`<(?:[A-Za-z0-9]+:)?Type>([A-Z]+)<`)
	sovosFrom = regexp.MustCompile(`<(?:[A-Za-z0-9]+:)?FromDate>([^<]+)<`)
	sovosTo   = regexp.MustCompile(`<(?:[A-Za-z0-9]+:)?ToDate>([^<]+)<`)
	sovosUUID = regexp.MustCompile(`<(?:[A-Za-z0-9]+:)?UUID>([^<]+)<`)
)

const sovosTime = "2006-01-02T15:04:05.000-07:00"

func (f *fakeSovos) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	b := string(body)
	f.mu.Lock()
	defer f.mu.Unlock()
	op := sovosSub(sovosOp, b, "?")
	f.ops = append(f.ops, op)
	if u, _, ok := r.BasicAuth(); !ok || u != "user" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	inbound := sovosSub(sovosType, b, "") == "INBOUND"
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	var out strings.Builder
	switch op {
	case "getUBLListRequest":
		from, _ := time.Parse(sovosTime, sovosSub(sovosFrom, b, ""))
		to, _ := time.Parse(sovosTime, sovosSub(sovosTo, b, ""))
		out.WriteString(`<getUBLListResponse xmlns="http:/fitcons.com/eInvoice/">`)
		for _, d := range f.docs {
			if d.incoming != inbound || d.created.Before(from) || d.created.After(to) {
				continue
			}
			fmt.Fprintf(&out, `<UBLList><UUID>%s</UUID><VKN_TCKN>4840847211</VKN_TCKN><ID>%s</ID><InsertDateTime>%s</InsertDateTime></UBLList>`,
				d.uuid, d.number, d.created.In(istanbul).Format(sovosTime))
		}
		out.WriteString(`</getUBLListResponse>`)
	case "getUBLRequest":
		id := sovosSub(sovosUUID, b, "")
		out.WriteString(`<getUBLResponse xmlns="http:/fitcons.com/eInvoice/">`)
		for _, d := range f.docs {
			if strings.EqualFold(d.uuid, id) {
				var buf bytes.Buffer
				zw := zip.NewWriter(&buf)
				fw, _ := zw.Create(d.uuid + ".xml")
				_, _ = fw.Write(d.xml)
				_ = zw.Close()
				out.WriteString(`<DocData>` + base64.StdEncoding.EncodeToString(buf.Bytes()) + `</DocData>`)
			}
		}
		out.WriteString(`</getUBLResponse>`)
	default:
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>unexpected</faultstring></s:Fault></s:Body></s:Envelope>`)
		return
	}
	_, _ = io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>`+out.String()+`</s:Body></s:Envelope>`)
}

// Sovos (#59, 30 §9): gün gün liste (Page.Next ertesi gün), ZIP'li UBL;
// yalnızca getUBLList / getUBL çağrılır; e-arşiv listelenemez
// (not_listable). Bir günde sayfa sınırından fazla belge olsa da hiçbiri
// kesilmez.
func TestSovosSyncAgainstFakeServer(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{PageSize: 2})
	f := &fakeSovos{}
	created := mustTime(t, "2026-11-10T09:00:00+03:00")
	for i := range 4 { // four on one day, more than PageSize
		xml, ettn := invoiceXML(t, 900+i, true, "")
		f.docs = append(f.docs, sovosDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 900+i), incoming: true, created: created.Add(time.Duration(i) * time.Minute), xml: xml})
	}
	xml, out := invoiceXML(t, 950, false, "")
	f.docs = append(f.docs, sovosDoc{uuid: out, number: "TST2026000000950", created: created.Add(24 * time.Hour), xml: xml})
	srv := httptest.NewServer(f)
	defer srv.Close()
	open := func(own string) (*Client, error) {
		return Open("sovos", provider.Config{Username: "user", Password: "pass", VKN: own, Endpoint: srv.URL,
			Options: map[string]string{"postbox_alias": "urn:mail:defaultpk@example.invalid", "sender_alias": "urn:mail:defaultgb@example.invalid"}})
	}
	c, err := open(ownVKN)
	if err != nil {
		t.Fatal(err)
	}
	// Why the profile leaves the limit open: with Limit the module drops the
	// rest of the day and gives no next page.
	day := mustTime(t, "2026-11-10T00:00:00+03:00")
	cut, err := c.List(ctx, provider.Inbound, ListQuery{Kind: provider.EInvoice, From: day, To: day.Add(23 * time.Hour), Limit: 2})
	if err != nil || len(cut.Items) != 2 || cut.Next != "" {
		t.Fatalf("limited listing = %+v, %v (expected the silent cut this profile avoids)", cut, err)
	}
	if got := NotListable(c); strings.Join(got, ",") != "e_archive:outgoing" {
		t.Fatalf("NotListable = %v", got)
	}

	e.now = mustTime(t, "2026-11-12T10:00:00+03:00")
	e.svc.WithOpener(func(_ context.Context, _ db.SourceConnection, own string) (*Client, error) { return open(own) })
	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "success" || rep.Counters.New != 5 || rep.Counters.UBLFetched != 5 {
		t.Fatalf("report %+v", rep)
	}
	if n := e.count(t, `SELECT count(*) FROM invoices WHERE taxpayer_id = $1`); n != 5 {
		t.Fatalf("invoices = %d", n)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, op := range f.ops {
		if op != "getUBLListRequest" && op != "getUBLRequest" {
			t.Errorf("unexpected operation %s", op)
		}
	}
}

func sovosSub(re *regexp.Regexp, s, def string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return def
}
