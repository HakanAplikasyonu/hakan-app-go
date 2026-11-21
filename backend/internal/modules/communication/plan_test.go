package communication

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func contact(id int64, title, phone, email string) Contact {
	return Contact{EngagementID: id, EngagementUUID: uuid.New(), Title: title, Phone: phone, Email: email}
}

// Kabul (#780 B): aynı numara tek gönderim; kaç tekrarın elendiği görünür;
// numarası olmayan ve hatalı olan atlanır.
func TestBuildPlanDedupesAndSkips(t *testing.T) {
	cs := []Contact{
		contact(1, "Atlas", "0532 111 22 33", ""),
		contact(2, "Beta", "+90 532 111 22 33", ""), // Atlas'ın numarası
		contact(3, "Ceren", "5320000000", ""),
		contact(4, "Deniz", "", ""),
		contact(5, "Ege", "0212 123 45 67", ""), // sabit hat
	}
	p, err := BuildPlan(ChannelSMS, "TR", "", "Sayın :mukellef, :donem beyanınız hazır. :ofis", cs, Vars{Office: "Demo SMMM", Period: "Ekim 2026"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 2 || p.Items[0].To != "5321112233" || p.Items[1].Title != "Ceren" {
		t.Fatalf("items = %+v", p.Items)
	}
	if len(p.Duplicates) != 1 || p.Duplicates[0].Title != "Beta" || p.Duplicates[0].SameAs != "Atlas" {
		t.Fatalf("duplicates = %+v", p.Duplicates)
	}
	if len(p.Skipped) != 2 || p.Skipped[0].Reason != SkipNoContact || p.Skipped[1].Reason != SkipInvalidPhone {
		t.Fatalf("skipped = %+v", p.Skipped)
	}
	if p.Items[0].Text != "Sayın Atlas, Ekim 2026 beyanınız hazır. Demo SMMM\n" {
		t.Fatalf("text = %q", p.Items[0].Text)
	}
	if p.Blocked() != "" {
		t.Fatalf("blocked = %q", p.Blocked())
	}

	// E-posta: büyük/küçük harf farkı aynı adres sayılır.
	cs = []Contact{contact(1, "Atlas", "", "Bilgi@Atlas.com"), contact(2, "Beta", "", "bilgi@atlas.com"), contact(3, "Ceren", "", "bozuk")}
	p, err = BuildPlan(ChannelMail, "TR", "Konu :mukellef", "Merhaba :mukellef", cs, Vars{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 1 || len(p.Duplicates) != 1 || len(p.Skipped) != 1 || p.Skipped[0].Reason != SkipInvalidEmail || p.Items[0].Subject != "Konu Atlas" {
		t.Fatalf("mail plan = %+v", p)
	}
}

// Kabul: kişiselleştirme sonrası 3 SMS boyunu aşan varsa gönderim durur.
func TestBuildPlanBlocksTooLongSMS(t *testing.T) {
	body := ":mukellef " + strings.Repeat("a", 420)
	cs := []Contact{
		contact(1, "Kısa", "5321112233", ""),
		contact(2, strings.Repeat("Uzun Unvan ", 6), "5321112234", ""),
	}
	p, err := BuildPlan(ChannelSMS, "TR", "", body, cs, Vars{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.TooLong) != 1 || p.TooLong[0].Parts <= MaxBulkSMSParts || !strings.HasPrefix(p.TooLong[0].Title, "Uzun") {
		t.Fatalf("too long = %+v", p.TooLong)
	}
	if !strings.Contains(p.Blocked(), "3 SMS") {
		t.Fatalf("blocked = %q", p.Blocked())
	}
	// Kampanya dili şablonda: gönderim durur.
	p, err = BuildPlan(ChannelSMS, "TR", "", "Bu ay %20 indirim", cs[:1], Vars{})
	if err != nil || len(p.Campaign) == 0 || p.Blocked() == "" {
		t.Fatalf("campaign = %+v, %v", p.Campaign, err)
	}
	// Unvanında kampanya kelimesi olan mükellef atlanır, şablon temizse durmaz.
	p, err = BuildPlan(ChannelSMS, "TR", "", "Sayın :mukellef", []Contact{contact(1, "Fırsat Market", "5321112233", "")}, Vars{})
	if err != nil || len(p.Skipped) != 1 || p.Skipped[0].Reason != SkipCampaign || len(p.Campaign) != 0 {
		t.Fatalf("title campaign = %+v, %v", p, err)
	}
}

// Kabul: en fazla 500 mükellef.
func TestBuildPlanLimit(t *testing.T) {
	cs := make([]Contact, MaxBulkRecipients+1)
	for i := range cs {
		cs[i] = contact(int64(i+1), "M", "5320000000", "")
	}
	if _, err := BuildPlan(ChannelSMS, "TR", "", "Bilgi", cs, Vars{}); !errors.Is(err, ErrTooManyRecipients) {
		t.Fatalf("501: %v", err)
	}
	p, err := BuildPlan(ChannelSMS, "TR", "", "Bilgi", cs[:MaxBulkRecipients], Vars{})
	if err != nil || len(p.Items) != 1 || len(p.Duplicates) != MaxBulkRecipients-1 {
		t.Fatalf("500: items=%d dup=%d %v", len(p.Items), len(p.Duplicates), err)
	}
	if _, err := BuildPlan(ChannelMail, "TR", "", "Metin", cs[:1], Vars{}); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("mail without subject: %v", err)
	}
	if _, err := BuildPlan("fax", "TR", "", "Metin", nil, Vars{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("channel: %v", err)
	}
}

func TestPersonalizeAndPeriod(t *testing.T) {
	got := Personalize(":mukellef :ofis :donem :tutar:son_gun :link", "Atlas", Vars{Office: "Ofis", Period: "Ekim 2026", Link: "https://x/m"})
	if got != "Atlas Ofis Ekim 2026  https://x/m" {
		t.Fatalf("personalize = %q", got)
	}
	if p := DeclarationPeriod(time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)); p != "Aralık 2025" {
		t.Fatalf("period = %q", p)
	}
}
