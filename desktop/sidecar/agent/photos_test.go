package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestPhotoFilter(t *testing.T) {
	for _, n := range []string{"fis.jpg", "FIS.JPEG", "z.png", "a.webp", "iphone.HEIC", "b.heif", "tarama.pdf"} {
		if !candidate(n) || !document(n) {
			t.Errorf("%s alınmalı ve belge ucuna gitmeli", n)
		}
	}
	for _, n := range []string{"notlar.txt", "rapor.docx", "video.mov", ".gizli.jpg", "~$fis.jpg"} {
		if candidate(n) {
			t.Errorf("%s alınmamalı", n)
		}
	}
}

// Fiş / fatura fotoğrafı izlenen klasörden ve mükellef alt klasöründen
// belge ucuna gider; alt klasördeki dosyanın "folder" alanı alt klasördür
// (sunucu mükellefi bu addan bulur). HEIC önce JPEG'e çevrilir; alt
// klasördeki e-fatura XML'i ve desteklenmeyen dosya alınmaz.
func TestWatcherUploadsReceiptPhotos(t *testing.T) {
	rt, api, srv, events := setup(t)
	ctx := context.Background()
	if _, err := rt.Login(ctx, srv.URL, "a@b.c", "dogru", ""); err != nil {
		t.Fatal(err)
	}
	rt.Client.SetOrganization("atlas-ymm")
	dir := t.TempDir()
	if _, err := rt.AddWatch(dir, "eng-1", "Luca", "Atlas Gıda A.Ş."); err != nil {
		t.Fatal(err)
	}
	if err := rt.State.Update(func(d *stateData) { d.AgentUUID = "a-1" }); err != nil {
		t.Fatal(err)
	}
	var converted []string
	rt.Watcher.ConvertHEIC = func(_ context.Context, p string) ([]byte, error) {
		converted = append(converted, filepath.Base(p))
		if strings.Contains(p, "bozuk") {
			return nil, errors.New("çevrilemedi")
		}
		return []byte("\xFF\xD8\xFFjpeg"), nil
	}
	old := time.Now().Add(-time.Minute)
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	write("market.jpg", "\xFF\xD8\xFFmarket")
	write(filepath.Join("Örnek Gıda", "z_raporu.png"), "\x89PNGz")
	write(filepath.Join("1234567890", "iphone.heic"), "\x00\x00\x00\x18ftypheic")
	write(filepath.Join("1234567890", "bozuk.heic"), "\x00\x00\x00\x18ftypheic-bozuk")
	write(filepath.Join("Örnek Gıda", "fatura.xml"), "<Invoice/>")
	write(filepath.Join("Örnek Gıda", "notlar.txt"), "not")

	now := time.Now()
	for i := 0; i < 3; i++ {
		rt.Watcher.Scan(ctx, now.Add(time.Duration(i)*ScanInterval))
	}
	got := map[string]string{}
	for i, d := range api.docs {
		name := d[strings.Index(d, "|")+1:]
		got[name] = api.docFolders[i]
		if name == "iphone.jpg" && api.docBodies[i] != "\xFF\xD8\xFFjpeg" {
			t.Errorf("HEIC çevrilmiş JPEG olarak gitmeli: %q", api.docBodies[i])
		}
	}
	want := map[string]string{
		"market.jpg":   dir,
		"z_raporu.png": filepath.Join(dir, "Örnek Gıda"),
		"iphone.jpg":   filepath.Join(dir, "1234567890"),
	}
	if len(got) != len(want) {
		keys := make([]string, 0, len(got))
		for k := range got {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Fatalf("yüklenenler %v, beklenen %v", keys, want)
	}
	for n, f := range want {
		if got[n] != f {
			t.Errorf("%s klasörü %q, beklenen %q", n, got[n], f)
		}
	}
	if len(api.uploads) != 0 {
		t.Fatalf("alt klasördeki XML UBL ucuna gitmemeli: %v", api.uploads)
	}
	rejected := false
	for _, e := range *events {
		if strings.Contains(e, "bozuk.heic") && strings.Contains(e, "JPEG olarak kaydedip") {
			rejected = true
		}
	}
	if !rejected {
		t.Errorf("çevrilemeyen HEIC için durum iletisi: %v", *events)
	}
	sort.Strings(converted)
	if strings.Join(converted, ",") != "bozuk.heic,iphone.heic" {
		t.Errorf("çevrilen HEIC: %v", converted)
	}
}
