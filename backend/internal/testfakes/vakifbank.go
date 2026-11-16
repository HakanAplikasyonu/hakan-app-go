package testfakes

import (
	"bytes"
	_ "embed"
	"net/http"
	"sync"
)

//go:embed testdata/vakifbank-getirhareket.xml
var vakifbankFixture []byte

// VakifbankFixture is a GetirHareket answer of the Vakıfbank test system
// (two accounts, movements on 2026-11-03; docs/kaynak 22).
func VakifbankFixture() []byte { return append([]byte(nil), vakifbankFixture...) }

// Vakifbank fakes the Vakıfbank online statement service (SOAP 1.2
// GetirHareket): every call answers the fixture, or a result code.
type Vakifbank struct {
	mu       sync.Mutex
	Code     string // "" → the fixture; else <IslemKodu>Code</IslemKodu>
	Requests int
	LastBody string
}

func (f *Vakifbank) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Requests++
	var b bytes.Buffer
	_, _ = b.ReadFrom(r.Body)
	f.LastBody = b.String()
	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	if f.Code == "" {
		_, _ = w.Write(vakifbankFixture)
		return
	}
	_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GetirHareketResponse><GetirHareketResult>` +
		`<IslemKodu>` + f.Code + `</IslemKodu><IslemAciklamasi>x</IslemAciklamasi></GetirHareketResult></GetirHareketResponse></s:Body></s:Envelope>`))
}
