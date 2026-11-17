package statement

import (
	"context"
	"errors"
	"testing"
)

func TestOfficeTemplateImport(t *testing.T) {
	f := setupImport(t)
	ctx := context.Background()
	file := []byte("Firma Hesap Dökümü\nIBAN;" + ibanZiraat + "\n" + oddCSV[len("Firma Hesap Dökümü\n"):])
	req := Request{TaxpayerID: f.tp, OrganizationID: f.org, EngagementID: f.eng, FileName: "odd.csv", Data: file}
	if _, err := f.im.Import(ctx, req); !errors.Is(err, ErrLayoutUnknown) {
		t.Fatalf("err = %v, want ErrLayoutUnknown", err)
	}
	a, err := f.im.Analyze(ctx, f.org, 0, "odd.csv", file)
	if err != nil {
		t.Fatal(err)
	}
	if a.BankID != "ziraat" {
		t.Fatalf("bank from IBAN = %q", a.BankID)
	}
	in := TemplateInput{BankID: "ziraat", Name: "Ziraat şube dökümü", Format: a.Format, Columns: []TemplateColumn{
		{"Gün", FieldDate}, {"Hareket", FieldDescription}, {"Giriş", FieldCredit}, {"Çıkış", FieldDebit}, {"Kalan", FieldBalance}}}
	if _, err := f.im.SaveTemplate(ctx, f.org, 0, TemplateInput{Format: "csv", Columns: in.Columns[:2]}); !errors.Is(err, ErrInvalidTemplate) {
		t.Fatalf("err = %v", err)
	}
	v, err := f.im.SaveTemplate(ctx, f.org, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	// Saving the same layout again replaces it.
	in.Name = "Ziraat dökümü"
	v2, err := f.im.SaveTemplate(ctx, f.org, 0, in)
	if err != nil || *v2.UUID != *v.UUID || v2.Name != "Ziraat dökümü" {
		t.Fatalf("v2 %+v err %v", v2, err)
	}
	rep, err := f.im.Import(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Template == nil || rep.Template.Source != SourceOffice || rep.BankID != "ziraat" || rep.Accounts[0].Inserted != 2 {
		t.Fatalf("report %+v", rep)
	}
	list, err := f.im.Templates(ctx, f.org)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Source != SourceOffice || list[0].Hits != 1 || list[0].LastUsedAt == nil || len(list) != 1+len(builtinTemplates) {
		t.Fatalf("list[0] %+v (%d)", list[0], len(list))
	}
	// Another office does not see or use it.
	other := f.d.Organization(t)
	if l, _ := f.im.Templates(ctx, other); l[0].Source != SourceBuiltin {
		t.Fatalf("other office sees %+v", l[0])
	}
	if err := f.im.DeleteTemplate(ctx, other, *v.UUID); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("err = %v", err)
	}
	if err := f.im.DeleteTemplate(ctx, f.org, *v.UUID); err != nil {
		t.Fatal(err)
	}
}
