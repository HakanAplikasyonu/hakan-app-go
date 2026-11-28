package handler

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/middleware"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/uploadbody"
)

// A phone on a mobile network sends a receipt photo slower than the server's
// general ReadTimeout. Before #780's fix the body was cut, the request
// answered 400 and the mobile queue gave up on the receipt for good, so it
// never reached the office. The upload routes now lift the deadline.
func TestReadImagesOutlivesServerReadTimeout(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts, ok := ReadImages(w, r)
		if !ok {
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, "%d:%d", len(parts), len(parts[0].Data))
	})
	// middleware.ServerErrors wraps the writer like the real server does: the
	// deadline must reach the connection through it.
	srv := httptest.NewUnstartedServer(middleware.ServerErrors(slog.New(slog.NewTextHandler(io.Discard, nil)))(h))
	srv.Config.ReadTimeout = 300 * time.Millisecond
	srv.Config.WriteTimeout = 300 * time.Millisecond
	srv.Start()
	defer srv.Close()

	status, body := slowUpload(t, srv.URL, 900*time.Millisecond)
	if status != http.StatusCreated {
		t.Fatalf("slow upload: status %d, body %q; want 201 (the upload deadline must outlive ReadTimeout)", status, body)
	}
	if body != "1:4096" {
		t.Fatalf("slow upload: body %q, want 1 part of 4096 bytes", body)
	}
}

// Without the lifted deadline the same slow upload is cut: the guard that the
// test above really exercises the server's ReadTimeout.
func TestSlowUploadIsCutWithoutLiftedDeadline(t *testing.T) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			w.WriteHeader(uploadbody.Status(err))
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ReadTimeout = 300 * time.Millisecond
	srv.Start()
	defer srv.Close()

	status, _ := slowUpload(t, srv.URL, 900*time.Millisecond)
	if status == http.StatusCreated {
		t.Fatal("slow upload passed without the lifted deadline; the test does not reproduce the timeout")
	}
	if status != 0 && status != http.StatusRequestTimeout {
		t.Fatalf("cut upload answered %d, want 408 (retryable), not 400", status)
	}
}

// slowUpload posts one 4 KB PNG-headed photo as multipart, pausing `pause`
// in the middle of the body. It returns 0 when the server dropped the
// connection without an answer.
func slowUpload(t *testing.T, url string, pause time.Duration) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("image", "fis.png")
	if err != nil {
		t.Fatal(err)
	}
	img := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 4096-8)...)
	_, _ = fw.Write(img)
	_ = mw.WriteField("kind", "expense_receipt")
	_ = mw.Close()
	data := buf.Bytes()
	half := len(data) / 2

	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write(data[:half])
		time.Sleep(pause)
		_, _ = pw.Write(data[half:])
		_ = pw.Close()
	}()
	req, err := http.NewRequest(http.MethodPost, url, pr)
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, strings.TrimSpace(string(b))
}
