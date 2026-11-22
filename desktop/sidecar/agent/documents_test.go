package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Luca dökümleri ve yabancı fatura PDF'i (#780 e) ajan belgesi ucuna gider;
// ajan kaydı yokken beklenir, aynı içerik bir kez yüklenir.
func TestWatcherUploadsLucaDocuments(t *testing.T) {
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
	old := time.Now().Add(-time.Minute)
	write := func(name, body string) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	write("bordro_dokumu.xlsx", "PK\x03\x04bordro")
	write("mizan.csv", "Hesap Kodu;Hesap Adı;Borç\n100;KASA;1\n")
	write("aws.pdf", "%PDF-1.7 fatura")
	write("~$mizan.xlsx", "kilit")

	now := time.Now()
	rt.Watcher.Scan(ctx, now)
	rt.Watcher.Scan(ctx, now.Add(ScanInterval))
	if len(api.docs) != 0 {
		t.Fatalf("ajan kaydı yokken belge yüklenmemeli: %v", api.docs)
	}
	if err := rt.State.Update(func(d *stateData) { d.AgentUUID = "a-1" }); err != nil {
		t.Fatal(err)
	}
	for i := 2; i < 5; i++ {
		rt.Watcher.Scan(ctx, now.Add(time.Duration(i)*ScanInterval))
	}
	if len(api.docs) != 3 {
		t.Fatalf("belgeler: %v", api.docs)
	}
	for _, d := range api.docs {
		if !strings.HasPrefix(d, "eng-1|") {
			t.Errorf("izlenen klasörün mükellefi gönderilmeli: %q", d)
		}
	}
	if len(api.uploads) != 0 {
		t.Fatalf("belgeler UBL ucuna gitmemeli: %v", api.uploads)
	}
	// Aynı içerik başka adla: yüklenmez.
	write("mizan-kopya.csv", "Hesap Kodu;Hesap Adı;Borç\n100;KASA;1\n")
	rt.Watcher.Scan(ctx, now.Add(6*ScanInterval))
	rt.Watcher.Scan(ctx, now.Add(7*ScanInterval))
	if len(api.docs) != 3 {
		t.Fatalf("mükerrer belge yüklendi: %v", api.docs)
	}
	waiting := false
	for _, e := range *events {
		if strings.HasPrefix(e, "upload ") && strings.Contains(e, "aws.pdf") && strings.Contains(e, "eşleştirme bekliyor") {
			waiting = true
		}
	}
	if !waiting {
		t.Errorf("eşleştirme bekleyen PDF olayı: %v", *events)
	}
}
