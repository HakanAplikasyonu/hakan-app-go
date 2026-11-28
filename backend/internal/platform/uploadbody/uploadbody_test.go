package uploadbody

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"timeout", fmt.Errorf("multipart: NextPart: %w", timeoutErr{}), http.StatusRequestTimeout},
		{"cut body", fmt.Errorf("multipart: NextPart: %w", io.ErrUnexpectedEOF), http.StatusRequestTimeout},
		{"deadline", fmt.Errorf("read: %w", os.ErrDeadlineExceeded), http.StatusRequestTimeout},
		{"closed", fmt.Errorf("read: %w", net.ErrClosed), http.StatusRequestTimeout},
		{"too large", fmt.Errorf("multipart: %w", &http.MaxBytesError{Limit: 10}), http.StatusRequestEntityTooLarge},
		{"not multipart", http.ErrNotMultipart, http.StatusBadRequest},
		{"other", errors.New("boom"), http.StatusBadRequest},
	}
	for _, c := range cases {
		if got := Status(c.err); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

// A recorder has no connection: Extend must not panic and keeps defaults.
func TestExtendWithoutConnection(t *testing.T) {
	Extend(httptest.NewRecorder())
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }
