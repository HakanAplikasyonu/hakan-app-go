package usecase

import (
	"errors"
	"strings"
	"testing"
)

func TestRejectionNormalize(t *testing.T) {
	ok := []struct {
		in   Rejection
		want Rejection
	}{
		{Rejection{}, Rejection{}},
		{Rejection{Code: " okunaksiz "}, Rejection{Code: RejectUnreadable}},
		{Rejection{Code: "eksik_bilgi", Text: "  tarih   yok "}, Rejection{Code: RejectMissingInfo, Text: "tarih yok"}},
		{Rejection{Code: "diger", Text: "Ekim ayına ait değil"}, Rejection{Code: RejectOther, Text: "Ekim ayına ait değil"}},
		{Rejection{Text: "Şirket kartıyla ödenmemiş"}, Rejection{Code: RejectOther, Text: "Şirket kartıyla ödenmemiş"}},
		{Rejection{Code: "mukerrer", Text: strings.Repeat("ş", MaxRejectReasonRunes)}, Rejection{Code: RejectDuplicate, Text: strings.Repeat("ş", MaxRejectReasonRunes)}},
	}
	for _, c := range ok {
		got, err := c.in.Normalize()
		if err != nil || got != c.want {
			t.Errorf("%+v = %+v %v, want %+v", c.in, got, err, c.want)
		}
	}
	bad := []Rejection{
		{Code: "bilinmeyen"},
		{Code: "diger"},
		{Code: "diger", Text: "   "},
		{Code: "kisisel_harcama", Text: strings.Repeat("a", MaxRejectReasonRunes+1)},
	}
	for _, in := range bad {
		if _, err := in.Normalize(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: err = %v, want ErrInvalid", in, err)
		}
	}
	for _, code := range RejectReasonCodes {
		if RejectReasonLabel(code) == "" {
			t.Errorf("label missing for %s", code)
		}
	}
	if r, err := decisionReason(true, Rejection{Code: "bilinmeyen"}); err != nil || !r.Empty() {
		t.Fatalf("approval must drop the reason: %+v %v", r, err)
	}
}

func TestRejectReasonText(t *testing.T) {
	cases := map[[2]string]string{
		{"", ""}:                          "",
		{"okunaksiz", ""}:                 "Fotoğraf okunmuyor",
		{"eksik_bilgi", "tarih yok"}:      "Bilgi eksik (VKN, tarih ya da tutar): tarih yok",
		{"diger", "Ekim ayına ait değil"}: "Ekim ayına ait değil",
		{"yanlis_mukellef", ""}:           "Başka bir şirkete ait",
	}
	for in, want := range cases {
		if got := RejectReasonText(in[0], in[1]); got != want {
			t.Errorf("%v = %q, want %q", in, got, want)
		}
	}
	if p := (Rejection{Code: RejectPersonal, Text: "özel alışveriş"}).AuditPayload(); p["reason_code"] != "kisisel_harcama" ||
		p["reason_label"] != "İşletme gideri değil" || p["reason_text"] != "özel alışveriş" {
		t.Fatalf("audit payload = %v", p)
	}
}
