package vakifbank

// Contract test (#196), on top of vakifbank_test.go (recorded GetirHareket
// answer, VBH codes, firewall page): a hung bank, a 5xx without a SOAP
// envelope, and the live sandbox switch. CONTRACT_LIVE=1 with
// CONTRACT_VAKIFBANK_CUSTOMER_NO, _USERNAME, _PASSWORD (and optional
// _ENDPOINT, _ACCOUNT) runs one read-only GetirHareket for yesterday. The
// bank only answers the allowlisted fixed egress IP (20 §4).
// See docs/entegrator-sozlesme-testleri.md.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/banking/webservice"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testutil/contract"
)

func TestVakifbankContract(t *testing.T) {
	ctx := context.Background()
	if env, ok := contract.Live("VAKIFBANK", "CUSTOMER_NO", "USERNAME", "PASSWORD"); ok {
		t.Run("live", func(t *testing.T) {
			c := New(contract.Optional("VAKIFBANK", "ENDPOINT"), nil)
			day := time.Now().In(istanbul).AddDate(0, 0, -1)
			w := webservice.Window{
				Start:   time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, istanbul),
				End:     time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 0, 0, istanbul),
				Account: contract.Optional("VAKIFBANK", "ACCOUNT"),
			}
			res, err := c.Fetch(ctx, webservice.Credentials{"customer_no": env["CUSTOMER_NO"], "username": env["USERNAME"], "password": env["PASSWORD"]}, w)
			if err != nil {
				t.Fatalf("live: %v", err)
			}
			t.Logf("vakifbank live: %d accounts", len(res.Accounts))
		})
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<x/>")) })

	t.Run("server_error_is_bank_error", func(t *testing.T) {
		srv := httptest.NewServer(&contract.Fault{Next: ok, Status: http.StatusBadGateway, Times: -1})
		defer srv.Close()
		_, err := New(srv.URL, srv.Client()).Fetch(ctx, creds, window())
		if be := webservice.AsError(err); be.Reason != webservice.ReasonBankError || be.Code != "HTTP 502" {
			t.Fatalf("got %+v", be)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(&contract.Fault{Next: ok, Delay: time.Second, Times: -1})
		defer srv.Close()
		cctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := New(srv.URL, srv.Client()).Fetch(cctx, creds, window())
		if be := webservice.AsError(err); err == nil || be.Reason != webservice.ReasonBankError {
			t.Fatalf("got %v", err)
		}
		if d := time.Since(start); d > time.Second {
			t.Fatalf("returned after %s", d)
		}
	})
}
