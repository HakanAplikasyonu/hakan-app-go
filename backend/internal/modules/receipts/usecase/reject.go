package usecase

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
)

// Ret gerekçeleri (#780): ofis mükellefin gönderdiği fişi, masrafı ya da Z
// raporunu reddederken birini seçer; mükellef Gönderdiklerim'de görür.
const (
	RejectUnreadable     = "okunaksiz"       // bulanık / okunmuyor
	RejectMissingInfo    = "eksik_bilgi"     // VKN / tarih / tutar eksik
	RejectPersonal       = "kisisel_harcama" // işletme gideri değil
	RejectDuplicate      = "mukerrer"        // daha önce gönderildi
	RejectWrongTaxpayer  = "yanlis_mukellef" // başka şirkete ait
	RejectOther          = "diger"           // açıklama zorunlu
	MaxRejectReasonRunes = 500
)

// RejectReasonCodes are the reason codes in the order the screens list them.
var RejectReasonCodes = []string{RejectUnreadable, RejectMissingInfo, RejectPersonal, RejectDuplicate, RejectWrongTaxpayer, RejectOther}

var rejectLabels = map[string]string{
	RejectUnreadable:    "Fotoğraf okunmuyor",
	RejectMissingInfo:   "Bilgi eksik (VKN, tarih ya da tutar)",
	RejectPersonal:      "İşletme gideri değil",
	RejectDuplicate:     "Daha önce gönderilmiş",
	RejectWrongTaxpayer: "Başka bir şirkete ait",
	RejectOther:         "Diğer",
}

// RejectReasonLabel is the Turkish label of a reason code ("" when unknown).
func RejectReasonLabel(code string) string { return rejectLabels[code] }

// Rejection is the office's optional reason for rejecting a receipt, an
// expense or a Z report (body of the reject endpoints).
type Rejection struct {
	Code string `json:"reason_code"`
	Text string `json:"reason_text"`
}

// Normalize trims and checks the reason: the code is one of
// RejectReasonCodes, "diger" needs a text, the text is at most 500
// characters. A text without a code counts as "diger". Both empty: no
// reason (older clients), which is allowed.
func (r Rejection) Normalize() (Rejection, error) {
	r.Code = strings.TrimSpace(r.Code)
	r.Text = strings.Join(strings.Fields(r.Text), " ")
	if r.Code == "" && r.Text != "" {
		r.Code = RejectOther
	}
	if r.Code != "" && rejectLabels[r.Code] == "" {
		return Rejection{}, fmt.Errorf("%w: unknown reason_code %q", ErrInvalid, r.Code)
	}
	if r.Code == RejectOther && r.Text == "" {
		return Rejection{}, fmt.Errorf("%w: reason_text is required when reason_code is diger", ErrInvalid)
	}
	if utf8.RuneCountInString(r.Text) > MaxRejectReasonRunes {
		return Rejection{}, fmt.Errorf("%w: reason_text is longer than %d characters", ErrInvalid, MaxRejectReasonRunes)
	}
	return r, nil
}

// Empty: no reason was given.
func (r Rejection) Empty() bool { return r.Code == "" }

// Label is the reason as the mükellef reads it (see RejectReasonText).
func (r Rejection) Label() string { return RejectReasonText(r.Code, r.Text) }

// RejectReasonText renders a stored reason: the label, and the office's own
// words after it ("Bilgi eksik (VKN, tarih ya da tutar): tarih yok"); for
// "diger" only the words. "" without a reason (older records).
func RejectReasonText(code, text string) string {
	label := rejectLabels[code]
	switch {
	case label == "":
		return text
	case code == RejectOther:
		if text == "" {
			return label
		}
		return text
	case text == "":
		return label
	}
	return label + ": " + text
}

// AuditPayload is the reason in the audit record.
func (r Rejection) AuditPayload() map[string]any {
	return map[string]any{"reason_code": r.Code, "reason_label": RejectReasonLabel(r.Code), "reason_text": r.Text}
}

func (r Rejection) dbCode() pgtype.Text { return text(r.Code) }
func (r Rejection) dbText() pgtype.Text { return text(r.Text) }

// decisionReason checks a rejection's reason; an approval carries none.
func decisionReason(approve bool, rej Rejection) (Rejection, error) {
	if approve {
		return Rejection{}, nil
	}
	return rej.Normalize()
}
