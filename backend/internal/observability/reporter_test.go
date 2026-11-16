package observability

import (
	"context"
	"errors"
	"testing"
	"time"
)

type captured struct {
	errs []error
	msgs []string
	opts []Options
}

func (c *captured) CaptureError(_ context.Context, err error, o Options) {
	c.errs = append(c.errs, err)
	c.opts = append(c.opts, o)
}

func (c *captured) CaptureMessage(_ context.Context, m string, o Options) {
	c.msgs = append(c.msgs, m)
	c.opts = append(c.opts, o)
}

func (c *captured) Flush(time.Duration) bool { return true }

var errRoot = errors.New("root")

// Ek bir hata raporlayıcı yalnızca temizlenmiş veri alır (#179).
func TestAddReporterGetsScrubbedEvents(t *testing.T) {
	c := &captured{}
	remove := AddReporter(c)
	CaptureError(context.Background(), errors.Join(errRoot, errors.New("VKN 1234567890 için a@b.co")), Options{
		Extra: map[string]any{"iban": "TR330006100519786457841326", "note": "TCKN 12345678901"},
		Tags:  map[string]string{"route": "GET /x", "token": "abc"},
	})
	CaptureMessage(context.Background(), "IBAN TR330006100519786457841326 reddedildi", Options{})
	remove()
	CaptureMessage(context.Background(), "after remove", Options{})

	if len(c.errs) != 1 || len(c.msgs) != 1 {
		t.Fatalf("captured = %+v", c)
	}
	if got := c.errs[0].Error(); got != "root\nVKN [vkn] için [email]" {
		t.Fatalf("error = %q", got)
	}
	if !errors.Is(c.errs[0], errRoot) {
		t.Fatal("error chain lost")
	}
	o := c.opts[0]
	if o.Extra["iban"] != redacted || o.Extra["note"] != "TCKN [tckn]" || o.Tags["token"] != redacted || o.Tags["route"] != "GET /x" {
		t.Fatalf("options = %+v", o)
	}
	if c.msgs[0] != "IBAN [iban] reddedildi" {
		t.Fatalf("message = %q", c.msgs[0])
	}
	if !FlushAll(time.Millisecond) {
		t.Fatal("flush")
	}
}
