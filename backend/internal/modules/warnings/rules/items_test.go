package rules

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
)

type itemsConfig struct{ office map[string]engine.Layer }

func (c itemsConfig) Resolve(_ context.Context, _ engine.Subject, codes []string) (map[string]engine.Effective, error) {
	w := catalog.SeverityWarning
	def := map[string]string{
		CodeFixedAsset:    `{"keywords":["bilgisayar","masa","araç"],"amount_limit":null}`,
		CodeUnusualRecord: `{"min_history":3,"min_overlap":"0.34"}`,
	}
	out := map[string]engine.Effective{}
	for _, code := range codes {
		out[code] = engine.Merge(code, engine.Layer{Severity: &w, Params: json.RawMessage(def[code])}, c.office[code])
	}
	return out, nil
}

func runItems(t *testing.T, cfg itemsConfig, in ItemInput) map[string]int {
	t.Helper()
	store := engine.NewMemoryStore()
	in.ItemID, in.Period = newID(), "2026-10"
	if _, err := engine.Run(context.Background(), engine.New(store, cfg), subj, Items{}, in); err != nil {
		t.Fatal(err)
	}
	return openCodes(store)
}

func TestItemsFixedAsset(t *testing.T) {
	cfg := itemsConfig{}
	hist := []string{"Dizüstü bilgisayar", "Masa", "Araç bakımı"}
	if got := runItems(t, cfg, ItemInput{Incoming: true, Name: "HP Dizüstü Bilgisayar 15\"", Amount: d("30000"), History: hist, HistoryLines: 5}); got[CodeFixedAsset] != 1 {
		t.Fatalf("keyword: %v", got)
	}
	// Suffixed forms match ("masası").
	if got := runItems(t, cfg, ItemInput{Incoming: true, Name: "Ofis masası", History: hist, HistoryLines: 5}); got[CodeFixedAsset] != 1 {
		t.Fatalf("suffix: %v", got)
	}
	if got := runItems(t, cfg, ItemInput{Incoming: true, Name: "Mermer işçiliği", History: []string{"Mermer işçiliği"}, HistoryLines: 5}); got[CodeFixedAsset] != 0 {
		t.Fatalf("no keyword: %v", got)
	}
	// Sales are never fixed-asset candidates.
	if got := runItems(t, cfg, ItemInput{Incoming: false, Name: "Bilgisayar satışı", History: []string{"Bilgisayar satışı"}, HistoryLines: 5}); got[CodeFixedAsset] != 0 {
		t.Fatalf("outgoing: %v", got)
	}
	// Office sets an amount limit.
	cfg.office = map[string]engine.Layer{CodeFixedAsset: {Params: json.RawMessage(`{"amount_limit":"10000"}`)}}
	if got := runItems(t, cfg, ItemInput{Incoming: true, Name: "Lazer kesim makinesi parçası", Amount: d("12000"), History: []string{"Lazer kesim"}, HistoryLines: 5}); got[CodeFixedAsset] != 1 {
		t.Fatalf("limit: %v", got)
	}
}

func TestItemsUnusualRecordType(t *testing.T) {
	cfg := itemsConfig{}
	hist := []string{"Danışmanlık hizmeti", "Yazılım danışmanlığı"}
	if got := runItems(t, cfg, ItemInput{Name: "Fiat Egea araç satışı", History: hist, HistoryLines: 10}); got[CodeUnusualRecord] != 1 {
		t.Fatalf("unusual: %v", got)
	}
	// Suffix variants share a stem.
	if got := runItems(t, cfg, ItemInput{Name: "Danışmanlık bedeli Ekim", History: hist, HistoryLines: 10}); got[CodeUnusualRecord] != 0 {
		t.Fatalf("usual: %v", got)
	}
	// Too little history: no verdict.
	if got := runItems(t, cfg, ItemInput{Name: "Fiat Egea araç satışı", History: hist, HistoryLines: 2}); got[CodeUnusualRecord] != 0 {
		t.Fatalf("thin history: %v", got)
	}
}

func TestTokens(t *testing.T) {
	got := Tokens("2 ADET Dizüstü BİLGİSAYAR (i7) - KDV dahil")
	if len(got) != 2 || got[0] != "dizustu" || got[1] != "bilgisayar" {
		t.Fatalf("tokens %v", got)
	}
}
