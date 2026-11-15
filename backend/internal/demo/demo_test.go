package demo

import (
	"context"
	"strings"
	"testing"
	"time"

	ubltr "github.com/furkanmeclis/go-ubltr"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/model"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/invoices/ublmap"
)

// want describes what each demo invoice must map to; together the set covers
// the cases issue #17 asks for: temel, ticari, tevkifat, istisna, iade,
// döviz and e-arşiv.
var want = map[string]struct {
	owner     string
	direction model.Direction
	typ       string
	profile   string
	currency  string
}{
	"atlas-01-temel-satis.xml":  {"atlas", model.DirectionOutgoing, model.TypeSatis, model.ProfileTemel, "TRY"},
	"atlas-02-ticari-satis.xml": {"atlas", model.DirectionOutgoing, model.TypeSatis, model.ProfileTicari, "TRY"},
	"atlas-03-tevkifat.xml":     {"atlas", model.DirectionOutgoing, model.TypeTevkifat, model.ProfileTemel, "TRY"},
	"atlas-04-istisna.xml":      {"atlas", model.DirectionOutgoing, model.TypeIstisna, model.ProfileTemel, "TRY"},
	"atlas-05-iade-alis.xml":    {"atlas", model.DirectionIncoming, model.TypeIade, model.ProfileTemel, "TRY"},
	"atlas-06-doviz-usd.xml":    {"atlas", model.DirectionOutgoing, model.TypeSatis, model.ProfileTicari, "USD"},
	"atlas-07-earsiv-satis.xml": {"atlas", model.DirectionOutgoing, model.TypeSatis, model.ProfileEArsiv, "TRY"},
	"selin-01-temel-alis.xml":   {"selin", model.DirectionIncoming, model.TypeAlis, model.ProfileTemel, "TRY"},
	"selin-02-earsiv-alis.xml":  {"selin", model.DirectionIncoming, model.TypeAlis, model.ProfileEArsiv, "TRY"},
}

func taxIDOf(key string) string {
	for _, tp := range Taxpayers {
		if tp.Key == key {
			return tp.TaxID
		}
	}
	return ""
}

func TestDemoUBLMapsForItsTaxpayer(t *testing.T) {
	files, err := UBL("")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(want) {
		t.Fatalf("demo UBL files = %d, want %d", len(files), len(want))
	}
	seen := map[string]string{}
	for _, f := range files {
		w, ok := want[f.Name]
		if !ok {
			t.Errorf("%s: not described in want", f.Name)
			continue
		}
		if !strings.HasPrefix(f.Name, w.owner+"-") {
			t.Errorf("%s: owner prefix", f.Name)
		}
		r, err := ublmap.MapXML(f.Data, ublmap.Options{OwnTaxID: taxIDOf(w.owner)})
		if err != nil {
			t.Errorf("%s: map: %v", f.Name, err)
			continue
		}
		inv := r.Invoice
		if inv.Direction != w.direction || inv.Type != w.typ || inv.Profile == nil || *inv.Profile != w.profile || inv.Currency != w.currency {
			t.Errorf("%s: got %s/%s/%v/%s, want %+v", f.Name, inv.Direction, inv.Type, inv.Profile, inv.Currency, w)
		}
		if inv.Period != "2026-10" {
			t.Errorf("%s: period %s", f.Name, inv.Period)
		}
		for _, wr := range r.Warnings {
			t.Errorf("%s: unexpected warning %s %v", f.Name, wr.Code, wr.Params)
		}
		if other, dup := seen[inv.UUID]; dup {
			t.Errorf("%s: ETTN %s also used by %s", f.Name, inv.UUID, other)
		}
		seen[inv.UUID] = f.Name
	}
}

// The derivation only touches identity fields, so the GİB samples must stay
// XSD and Schematron valid.
func TestDemoUBLStaysValid(t *testing.T) {
	if testing.Short() {
		t.Skip("schema validation is slow")
	}
	files, err := UBL("")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, f := range files {
		if err := ubltr.ValidateXSD(ctx, f.Data); err != nil {
			t.Errorf("%s: xsd: %v", f.Name, err)
		}
		docType := ubltr.TypeEFatura
		if strings.Contains(f.Name, "earsiv") {
			docType = ubltr.TypeEArchive
		}
		if err := ubltr.ValidateSchematronType(ctx, f.Data, docType); err != nil {
			t.Errorf("%s: schematron: %v", f.Name, err)
		}
	}
}
