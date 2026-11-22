package entegrator

import (
	"context"
	"reflect"
	"testing"

	"github.com/furkanmeclis/go-ubltr/provider"
	"github.com/furkanmeclis/go-ubltr/provider/providertest"
)

// The read rule (30 §5) at compile time: a *Client must not satisfy any
// write interface of go-ubltr, and must not hand out the provider it wraps.
func TestClientIsReadOnly(t *testing.T) {
	var c any = Wrap(providertest.NewFake())
	if _, ok := c.(provider.Sender); ok {
		t.Error("Client implements provider.Sender")
	}
	if _, ok := c.(provider.Responder); ok {
		t.Error("Client implements provider.Responder")
	}
	if _, ok := c.(provider.EArchiveCanceller); ok {
		t.Error("Client implements provider.EArchiveCanceller")
	}
	if _, ok := c.(provider.Acknowledger); ok {
		t.Error("Client implements provider.Acknowledger")
	}
	if _, ok := c.(provider.CreditChecker); ok {
		t.Error("Client implements provider.CreditChecker")
	}

	providerType := reflect.TypeFor[provider.Provider]()
	typ := reflect.TypeFor[*Client]()
	allowed := map[string]bool{"Name": true, "Capabilities": true, "Supports": true, "List": true, "Fetch": true, "Status": true, "CheckUser": true,
		// Configuration of the read calls themselves (retry of a passing failure).
		"WithRetry": true,
		// Logout of a session-based integrator: ends the session, changes no document.
		"Close": true}
	for i := range typ.NumMethod() {
		m := typ.Method(i)
		if !allowed[m.Name] {
			t.Errorf("Client exposes %s; only read methods are allowed", m.Name)
		}
		for o := range m.Type.NumOut() {
			out := m.Type.Out(o)
			if out.Kind() == reflect.Interface && out.Implements(providerType) {
				t.Errorf("Client.%s returns a provider", m.Name)
			}
		}
	}
	st := typ.Elem()
	for i := range st.NumField() {
		f := st.Field(i)
		if f.IsExported() {
			t.Errorf("Client has exported field %s", f.Name)
		}
		if f.Type == providerType {
			t.Errorf("Client keeps the provider value in %s", f.Name)
		}
	}
	// No write operation is reported as supported.
	cl := Wrap(providertest.NewFake())
	for _, op := range []provider.Operation{provider.OpSend, provider.OpRespond, provider.OpCancelEArchive, provider.OpMarkTaken, provider.OpCredits} {
		if cl.Supports(provider.EInvoice, op) {
			t.Errorf("Supports(%s) = true", op)
		}
	}
}

type onlyNewSpy struct {
	*providertest.Fake
	onlyNew int
}

func (s *onlyNewSpy) ListInbox(ctx context.Context, q provider.ListQuery) (*provider.Page, error) {
	if q.OnlyNew {
		s.onlyNew++
	}
	return s.Fake.ListInbox(ctx, q)
}

func TestListNeverAsksOnlyNew(t *testing.T) {
	s := &onlyNewSpy{Fake: providertest.NewFake()}
	c := Wrap(s)
	if _, err := c.List(context.Background(), provider.Inbound, ListQuery{Kind: provider.EInvoice}); err != nil {
		t.Fatal(err)
	}
	if s.onlyNew != 0 {
		t.Fatalf("OnlyNew sent %d times", s.onlyNew)
	}
}

func TestProviderName(t *testing.T) {
	for in, want := range map[string]string{"entegrator_izibiz": "izibiz", "entegrator_": "", "parasut": ""} {
		got, ok := ProviderName(in)
		if got != want || ok != (want != "") {
			t.Errorf("ProviderName(%q) = %q, %v", in, got, ok)
		}
	}
}

func TestNormalizeStatus(t *testing.T) {
	cases := []struct {
		st      provider.Status
		profile string
		want    string
		warn    string
	}{
		{provider.StatusAccepted, "TICARIFATURA", StatusActive, ""},
		{provider.StatusReceived, "TICARIFATURA", StatusActive, WarnPendingResponse},
		{provider.StatusReceived, "TEMELFATURA", StatusActive, ""},
		{provider.StatusDelivered, "TEMELFATURA", StatusActive, ""},
		{provider.StatusRejected, "TICARIFATURA", StatusCancelled, WarnRejected},
		{provider.StatusCancelled, "EARSIVFATURA", StatusCancelled, ""},
		{provider.StatusQueued, "TEMELFATURA", StatusDraft, ""},
		{provider.StatusFailed, "TEMELFATURA", StatusDraft, WarnDeliveryError},
		{provider.StatusUnknown, "TEMELFATURA", StatusActive, WarnStatusUnknown},
	}
	for _, c := range cases {
		n := NormalizeStatus("none", provider.DocumentInfo{Status: c.st, Profile: c.profile})
		warn := ""
		if len(n.Warnings) > 0 {
			warn = n.Warnings[0]
		}
		if n.Status != c.want || warn != c.warn {
			t.Errorf("%s/%s = %+v, want %s %s", c.st, c.profile, n, c.want, c.warn)
		}
	}
	RegisterStatusTable("tabled", func(d provider.DocumentInfo) (Normalized, bool) {
		if d.ProviderCode == "IADE" {
			return Normalized{Status: StatusCancelled, Warnings: []string{WarnReturned}}, true
		}
		return Normalized{}, false
	})
	if n := NormalizeStatus("tabled", provider.DocumentInfo{Status: provider.StatusAccepted, ProviderCode: "IADE"}); n.Status != StatusCancelled {
		t.Errorf("table not used: %+v", n)
	}
	if n := NormalizeStatus("tabled", provider.DocumentInfo{Status: provider.StatusAccepted}); n.Status != StatusActive {
		t.Errorf("fallback not used: %+v", n)
	}
	if got := RawStatus(provider.DocumentInfo{Status: provider.StatusAccepted, GIBCode: 1300, ProviderCode: "5"}); got != "accepted;gib=1300;code=5" {
		t.Errorf("RawStatus = %q", got)
	}
}

func TestMonths(t *testing.T) {
	from := mustTime(t, "2025-01-01T00:00:00+03:00")
	to := mustTime(t, "2026-11-15T10:00:00+03:00")
	got := months(from, to)
	if len(got) != 23 {
		t.Fatalf("months = %d, want 23", len(got))
	}
	if !got[22][1].Equal(to) || !got[0][0].Equal(from) {
		t.Fatalf("bounds %v .. %v", got[0][0], got[22][1])
	}
	for i := 1; i < len(got); i++ {
		if !got[i][0].Equal(got[i-1][1]) {
			t.Fatalf("gap at %d", i)
		}
	}
}
