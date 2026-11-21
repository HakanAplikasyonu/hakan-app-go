package communication

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Elle toplu gönderimin kuralları (#780 B). BuildPlan saf bir işlevdir:
// hedefteki mükelleflerden kime ne gideceğini, elenen tekrarları, atlanan
// mükellefleri ve gönderimi durduran uzunluk aşımlarını çıkarır.

// Bulk channels (ChannelSMS: history.go).
const ChannelMail = "mail"

const (
	// MaxBulkRecipients caps one bulk send (hedefteki mükellef sayısı).
	MaxBulkRecipients = 500
	// MaxBulkSMSParts: kişiselleştirilmiş bir SMS en fazla bu kadar SMS boyu
	// olabilir (TR kodlamasında 3 × 148 = 444 karakter). Aşan varsa gönderim
	// yapılmaz.
	MaxBulkSMSParts = 3
	// MaxBulkMailChars caps an e-mail body.
	MaxBulkMailChars = 20000
)

var (
	// ErrTooManyRecipients: more than MaxBulkRecipients taxpayers.
	ErrTooManyRecipients = errors.New("communication: too many recipients")
	// ErrNoRecipients: nobody left to send to.
	ErrNoRecipients = errors.New("communication: no recipients")
	// ErrBulkBlocked: the plan has a blocker (too long SMS, campaign words).
	ErrBulkBlocked = errors.New("communication: bulk send blocked")
	// ErrInvalidInput: a request field is invalid; the text after ": " is the
	// screen message.
	ErrInvalidInput = errors.New("communication: invalid input")
)

// Skip reasons.
const (
	SkipNoContact    = "no_contact"
	SkipInvalidPhone = "invalid_phone"
	SkipInvalidEmail = "invalid_email"
	SkipCampaign     = "campaign_words"
)

// Contact is one target taxpayer with its contact details.
type Contact struct {
	EngagementID   int64
	EngagementUUID uuid.UUID
	Title          string
	Phone          string
	Email          string
}

// Vars are the office-wide values of the message variables. The taxpayer's
// own title fills :mukellef. Empty values leave the variable empty.
type Vars struct {
	Office  string
	Period  string
	Amount  string
	DueDate string
	Link    string
}

// Variables the text may carry.
var Variables = []string{":mukellef", ":ofis", ":donem", ":tutar", ":son_gun", ":link"}

// Personalize fills the variables of a text for one taxpayer.
func Personalize(text, taxpayer string, v Vars) string {
	return strings.NewReplacer(
		":mukellef", taxpayer,
		":ofis", v.Office,
		":donem", v.Period,
		":tutar", v.Amount,
		":son_gun", v.DueDate,
		":link", v.Link,
	).Replace(text)
}

var trMonths = [...]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

// DeclarationPeriod is the period being declared now (the previous month),
// e.g. "Ekim 2026" in November 2026.
func DeclarationPeriod(now time.Time) string {
	p := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -1, 0)
	return fmt.Sprintf("%s %d", trMonths[p.Month()-1], p.Year())
}

// PlanItem is one message to send.
type PlanItem struct {
	EngagementID int64
	Title        string
	To           string
	Subject      string
	Text         string
	Length       Length
}

// Skip is a taxpayer that gets nothing, and why.
type Skip struct {
	EngagementUUID string `json:"engagement_uuid"`
	Title          string `json:"title"`
	Reason         string `json:"reason"`
}

// Duplicate is a taxpayer whose phone / e-mail already gets the message of
// an earlier taxpayer: one send per number or address.
type Duplicate struct {
	Title   string `json:"title"`
	SameAs  string `json:"same_as"`
	Contact string `json:"contact"`
}

// TooLong is a personalized SMS beyond MaxBulkSMSParts.
type TooLong struct {
	Title string `json:"title"`
	Chars int    `json:"chars"`
	Parts int    `json:"parts"`
}

// Plan is what a bulk send would do.
type Plan struct {
	Channel    string
	Targets    int
	Items      []PlanItem
	Skipped    []Skip
	Duplicates []Duplicate
	TooLong    []TooLong
	// Campaign: campaign wording found in the template itself.
	Campaign []string
	MaxParts int
}

// Blocked reports why the plan must not be sent ("" = fine).
func (p Plan) Blocked() string {
	switch {
	case len(p.Campaign) > 0:
		return "Bilgilendirme kanalından kampanya içerikli mesaj gönderilemez (" + strings.Join(p.Campaign, ", ") + ")."
	case len(p.TooLong) > 0:
		return fmt.Sprintf("%d mükellefin mesajı kişiselleştirmeden sonra %d SMS boyunu aşıyor. Metni kısaltın.", len(p.TooLong), MaxBulkSMSParts)
	case len(p.Items) == 0:
		return "Gönderilecek mükellef yok: hedefteki mükelleflerin iletişim bilgisi yok."
	}
	return ""
}

// BuildPlan personalizes the text for every contact and applies the bulk
// rules: at most MaxBulkRecipients contacts; one send per phone (SMS) or
// address (e-mail), later duplicates dropped; contacts without a usable
// phone / address skipped; an SMS longer than MaxBulkSMSParts after
// personalization is a blocker (Plan.TooLong). Contacts come in the order
// they should be served (the first of a duplicate group wins).
func BuildPlan(channel, encoding, subject, body string, contacts []Contact, v Vars) (Plan, error) {
	if channel != ChannelSMS && channel != ChannelMail {
		return Plan{}, fmt.Errorf("%w: kanal SMS ya da e-posta olmalı", ErrInvalidInput)
	}
	if len(contacts) > MaxBulkRecipients {
		return Plan{}, ErrTooManyRecipients
	}
	body = strings.TrimSpace(body)
	subject = strings.TrimSpace(subject)
	if body == "" || (channel == ChannelMail && subject == "") {
		return Plan{}, ErrEmptyText
	}
	if channel == ChannelMail && len([]rune(body)) > MaxBulkMailChars {
		return Plan{}, fmt.Errorf("%w: e-posta metni en fazla %d karakter olabilir", ErrInvalidInput, MaxBulkMailChars)
	}
	if channel == ChannelMail && len([]rune(subject)) > 200 {
		return Plan{}, fmt.Errorf("%w: konu en fazla 200 karakter olabilir", ErrInvalidInput)
	}
	p := Plan{Channel: channel, Targets: len(contacts), Items: []PlanItem{}, Skipped: []Skip{}, Duplicates: []Duplicate{},
		TooLong: []TooLong{}, Campaign: []string{}}
	if channel == ChannelSMS {
		p.Campaign = CampaignWords(body)
	}
	seen := map[string]string{}
	for _, c := range contacts {
		skip := func(reason string) {
			p.Skipped = append(p.Skipped, Skip{EngagementUUID: c.EngagementUUID.String(), Title: c.Title, Reason: reason})
		}
		var key, to string
		if channel == ChannelSMS {
			if strings.TrimSpace(c.Phone) == "" {
				skip(SkipNoContact)
				continue
			}
			phone, err := NormalizePhone(c.Phone)
			if err != nil {
				skip(SkipInvalidPhone)
				continue
			}
			key, to = phone, phone
		} else {
			mail := strings.TrimSpace(c.Email)
			if mail == "" {
				skip(SkipNoContact)
				continue
			}
			if !strings.Contains(mail, "@") || len(mail) > 254 {
				skip(SkipInvalidEmail)
				continue
			}
			key, to = strings.ToLower(mail), mail
		}
		if first, ok := seen[key]; ok {
			p.Duplicates = append(p.Duplicates, Duplicate{Title: c.Title, SameAs: first, Contact: to})
			continue
		}
		seen[key] = c.Title
		item := PlanItem{EngagementID: c.EngagementID, Title: c.Title, To: to}
		if channel == ChannelSMS {
			text, err := NormalizeText(Personalize(body, c.Title, v))
			if err != nil {
				return Plan{}, err
			}
			if len(p.Campaign) == 0 && len(CampaignWords(text)) > 0 {
				// The template is fine; the taxpayer's own name carries the
				// wording (e.g. "Fırsat Market"). The send would be refused.
				skip(SkipCampaign)
				continue
			}
			item.Text, item.Length = text, SMSLength(text, encoding)
			if item.Length.Parts > MaxBulkSMSParts {
				p.TooLong = append(p.TooLong, TooLong{Title: c.Title, Chars: item.Length.Chars, Parts: item.Length.Parts})
			}
			if item.Length.Parts > p.MaxParts {
				p.MaxParts = item.Length.Parts
			}
		} else {
			item.Subject = Personalize(subject, c.Title, v)
			item.Text = Personalize(body, c.Title, v)
		}
		p.Items = append(p.Items, item)
	}
	return p, nil
}
