package entegrator

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/go-ubltr/provider"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
)

type miDoc struct {
	uuid, number, status string
	incoming             bool
	created              time.Time
	xml                  []byte
}

// fakeMuhasebeizleme answers the BasicIntegration operations the go-ubltr
// uyumsoft module's muhasebeizleme preset reads (list pages, invoice data)
// and records each SOAPAction. The API key travels in userInfo.
type fakeMuhasebeizleme struct {
	mu      sync.Mutex
	docs    []miDoc
	actions []string
}

var (
	miPage  = regexp.MustCompile(`PageIndex="([0-9]+)"`)
	miSize  = regexp.MustCompile(`PageSize="([0-9]+)"`)
	miStart = regexp.MustCompile(`<(?:[A-Za-z0-9]+:)?CreateStartDate>([^<]+)<`)
	miEnd   = regexp.MustCompile(`<(?:[A-Za-z0-9]+:)?CreateEndDate>([^<]+)<`)
	miID    = regexp.MustCompile(`<(?:[A-Za-z0-9]+:)?invoiceId>([^<]+)<`)
)

func miSub(re *regexp.Regexp, s, def string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return def
}

func (f *fakeMuhasebeizleme) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	b := string(body)
	action := strings.Trim(r.Header.Get("SOAPAction"), `"`)
	op := action[strings.LastIndexByte(action, '/')+1:]
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actions = append(f.actions, op)
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	if !strings.Contains(b, "test-key") {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>api key</faultstring></s:Fault></s:Body></s:Envelope>`)
		return
	}
	var out string
	switch op {
	case "GetInboxInvoiceList", "GetOutboxInvoiceList":
		out = f.list(op, b)
	case "GetInboxInvoiceData", "GetOutboxInvoiceData":
		id := miSub(miID, b, "")
		data := ""
		for _, d := range f.docs {
			if strings.EqualFold(d.uuid, id) {
				data = base64.StdEncoding.EncodeToString(d.xml)
			}
		}
		out = fmt.Sprintf(`<%sResponse xmlns="http://tempuri.org/"><%sResult IsSucceded="true"><Value><Data>%s</Data></Value></%sResult></%sResponse>`, op, op, data, op, op)
	default:
		w.WriteHeader(http.StatusInternalServerError)
		out = `<s:Fault><faultcode>s:Client</faultcode><faultstring>unexpected</faultstring></s:Fault>`
	}
	_, _ = io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>`+out+`</s:Body></s:Envelope>`)
}

func (f *fakeMuhasebeizleme) list(op, b string) string {
	incoming := op == "GetInboxInvoiceList"
	page, _ := strconv.Atoi(miSub(miPage, b, "0"))
	size, _ := strconv.Atoi(miSub(miSize, b, "20"))
	const layout = "2006-01-02T15:04:05"
	from, _ := time.ParseInLocation(layout, miSub(miStart, b, ""), istanbul)
	to, _ := time.ParseInLocation(layout, miSub(miEnd, b, ""), istanbul)
	var hit []miDoc
	for _, d := range f.docs {
		c := d.created.Truncate(time.Second)
		if d.incoming != incoming || !from.IsZero() && c.Before(from) || !to.IsZero() && c.After(to) {
			continue
		}
		hit = append(hit, d)
	}
	pages := (len(hit) + size - 1) / size
	start := min(page*size, len(hit))
	end := min(start+size, len(hit))
	var items strings.Builder
	for _, d := range hit[start:end] {
		fmt.Fprintf(&items, `<Items><InvoiceId>%s</InvoiceId><DocumentId>%s</DocumentId><TargetTcknVkn>4840847211</TargetTcknVkn><TargetTitle>Karşı Taraf</TargetTitle>`+
			`<Status>%s</Status><StatusCode>1000</StatusCode><EnvelopeStatusCode>1300</EnvelopeStatusCode><CreateDateUtc>%s</CreateDateUtc><ExecutionDate>2026-11-01T00:00:00</ExecutionDate>`+
			`<PayableAmount>1.00</PayableAmount><DocumentCurrencyCode>TRY</DocumentCurrencyCode><Profile>TEMELFATURA</Profile><InvoiceTipType>Sales</InvoiceTipType></Items>`,
			d.number, d.uuid, d.status, d.created.In(istanbul).Format(layout))
	}
	return fmt.Sprintf(`<%sResponse xmlns="http://tempuri.org/"><%sResult IsSucceded="true"><Value PageIndex="%d" PageSize="%d" TotalCount="%d" TotalPages="%d">%s</Value></%sResult></%sResponse>`,
		op, op, page, size, len(hit), pages, items.String(), op, op)
}

// muhasebeizleme (#59, 30 §9): uyumsoft modülünün BasicIntegration ön
// ayarı. Sayfalama TotalPages ile ilerler; yalnızca liste ve veri
// çağrıları yapılır, SetInvoicesTaken çağrılmaz.
func TestMuhasebeizlemeSyncAgainstFakeServer(t *testing.T) {
	ctx := context.Background()
	e := setup(t, Config{PageSize: 2})
	f := &fakeMuhasebeizleme{}
	created := mustTime(t, "2026-11-10T09:00:00+03:00")
	for i := range 3 {
		xml, ettn := invoiceXML(t, 1000+i, true, "")
		f.docs = append(f.docs, miDoc{uuid: ettn, number: fmt.Sprintf("TST2026%09d", 1000+i), status: "Approved", incoming: true,
			created: created.Add(time.Duration(i) * time.Hour), xml: xml})
	}
	xml, out := invoiceXML(t, 1100, false, "")
	f.docs = append(f.docs, miDoc{uuid: out, number: "TST2026000001100", status: "Declined", created: created, xml: xml})
	srv := httptest.NewServer(f)
	defer srv.Close()

	e.now = mustTime(t, "2026-11-11T10:00:00+03:00")
	e.svc.WithOpener(func(_ context.Context, _ db.SourceConnection, own string) (*Client, error) {
		return Open("muhasebeizleme", provider.Config{APIKey: "test-key", Password: "test-key", Endpoint: srv.URL, VKN: own})
	})
	rep, err := e.svc.Run(ctx, e.tp, e.conn.Uuid, RunIncremental)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "success" || rep.Counters.New != 4 || rep.Counters.UBLFetched != 4 || rep.Counters.Pages < 3 {
		t.Fatalf("report %+v", rep)
	}
	if s := e.invoiceStatus(t, out); s != StatusCancelled {
		t.Fatalf("declined outgoing invoice = %s", s)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, op := range f.actions {
		switch op {
		case "GetInboxInvoiceList", "GetOutboxInvoiceList", "GetInboxInvoiceData", "GetOutboxInvoiceData":
		default:
			t.Errorf("unexpected operation %s", op)
		}
	}
}
