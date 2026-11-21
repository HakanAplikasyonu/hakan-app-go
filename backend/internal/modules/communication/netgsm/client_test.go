package netgsm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Sahte NetGSM sunucusu: testlerde gerçek API'ye hiç istek gitmez.
func fake(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL)
	c.Wait = time.Millisecond
	return c
}

var cred = Credentials{Usercode: "8501234567", Password: "gizli"}

func TestHeaders(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != cred.Usercode || p != cred.Password || r.URL.Path != "/sms/rest/v2/msgheader" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"code":"00","msgheaders":["OFISADI","HAKAN MMM"]}`)
	})
	got, err := c.Headers(context.Background(), cred)
	if err != nil || len(got) != 2 || got[0] != "OFISADI" {
		t.Fatalf("headers = %v, %v", got, err)
	}
	_, err = c.Headers(context.Background(), Credentials{Usercode: "1", Password: "yanlis"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "30" {
		t.Fatalf("wrong password = %v", err)
	}
}

func TestSend(t *testing.T) {
	var got sendBody
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"code":"00","jobid":"17723","description":"queued"}`)
	})
	job, err := c.Send(context.Background(), cred, SendRequest{Header: "OFISADI", Encoding: "TR", Items: []SendItem{{Phone: "5321234567", Text: "Merhaba\n"}}})
	if err != nil || job != "17723" {
		t.Fatalf("send = %q, %v", job, err)
	}
	if got.IYSFilter != "0" || got.MsgHeader != "OFISADI" || len(got.Messages) != 1 || got.Messages[0].No != "5321234567" {
		t.Fatalf("body = %+v", got)
	}
}

func TestSendErrorCode(t *testing.T) {
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":"40","description":"header"}`)
	})
	_, err := c.Send(context.Background(), cred, SendRequest{Header: "X", Items: []SendItem{{Phone: "5321234567", Text: "a"}}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "40" || apiErr.Message() == "" {
		t.Fatalf("code 40 = %v", err)
	}
}

// Cevap beklerken bağlantı koparsa yeniden denenmez: sonuç belirsiz.
func TestSendConnectionLostIsUncertain(t *testing.T) {
	var calls atomic.Int32
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.ReadAll(r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	})
	_, err := c.Send(context.Background(), cred, SendRequest{Header: "X", Items: []SendItem{{Phone: "5321234567", Text: "a"}}})
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("err = %v, want uncertain", err)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("calls = %d, want 1 (no retry)", n)
	}
}

// İstek hiç ulaşmadıysa (bağlantı kurulamadı) yeniden denenir, sonra
// "ulaşılamadı" döner.
func TestUnreachableIsRetried(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	c := New("http://" + addr)
	c.Wait = time.Millisecond
	_, err = c.Headers(context.Background(), cred)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v, want unreachable", err)
	}
}
