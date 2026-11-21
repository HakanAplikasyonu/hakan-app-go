// Package automation is part C of the İletişim merkezi (#780): automation
// rules (Ayarlar > Otomasyonlar). A rule has a trigger (calendar days of the
// month or "X days before the due date", an event, or one taxpayer's
// payment date), a target (all taxpayers, a group, selected taxpayers;
// optionally only those with a payment / something missing), channels
// (SMS, e-mail, in-app) and a text with variables. It either sends by
// itself or leaves a draft for the office to approve.
//
// Safety: a rule runs once per taxpayer per period (automation_runs has a
// unique run key per rule), every run and every edit is kept, campaign
// wording is refused, and the office can switch all automations off.
//
// Sending goes through communication.Service (SendSMS / SendMail, test
// mode included) and the notifications service for in-app messages.
package automation

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication"
)

// Trigger kinds.
const (
	TriggerCalendar = "calendar"
	TriggerEvent    = "event"
	TriggerPayment  = "payment"
)

// Calendar modes: given days of the month, or N days before a due date.
const (
	ModeDays      = "days"
	ModeBeforeDue = "before_due"
)

// Targets.
const (
	TargetAll      = "all"
	TargetGroup    = "group"
	TargetSelected = "selected"
)

// Channels. WhatsApp is shown as "yakında" on the screen and refused here.
const (
	ChannelSMS   = "sms"
	ChannelEmail = "email"
	ChannelInapp = "inapp"
)

// Approval modes.
const (
	ApprovalAuto  = "auto"
	ApprovalDraft = "draft"
)

// Run statuses.
const (
	RunScheduled = "scheduled"
	RunDraft     = "draft"
	RunSent      = "sent"
	RunSkipped   = "skipped"
	RunFailed    = "failed"
	RunRejected  = "rejected"
)

// Taxes a calendar rule can be about (the :beyan, :son_gun and :tutar
// variables come from it).
const (
	TaxKDV      = "kdv"
	TaxMuhtasar = "muhtasar"
	TaxGecici   = "gecici"
)

// KDV events of K1 (#780, issue comment 7061061955). They reach the worker
// through the outbox and events.Bus. TODO: switch to the
// events.VATReportReady / VATThresholdExceeded / VATStatusChanged constants
// once they are on main (backend/internal/platform/events/catalog.go).
const (
	EventVATReportReady       = "vat.report_ready"
	EventVATThresholdExceeded = "vat.threshold_exceeded"
	EventVATStatusChanged     = "vat.status_changed"
)

// Events found in the database by the scheduler.
const (
	EventReturnFiled      = "tax_return.filed"
	EventReturnChecked    = "tax_return.checked"
	EventDocRequestLate   = "doc_request.overdue"
	EventBankStatementGap = "bank_statement.missing"
	EventMonthlyReport    = "report.monthly_ready"
)

// EventInfo describes an event trigger. Available false: listed on the
// screen as "yakında" and refused on save (there is no source yet).
type EventInfo struct {
	Code      string `json:"code"`
	Available bool   `json:"available"`
	// Payment: the event is about a payment, so "hâlâ ödenmediyse tekrar
	// gönder" applies.
	Payment bool `json:"payment"`
}

// Events is the event catalog of the rule dialog.
var Events = []EventInfo{
	{Code: EventReturnFiled, Available: true, Payment: true},
	{Code: EventReturnChecked, Available: true},
	{Code: EventDocRequestLate, Available: true},
	{Code: EventVATReportReady, Available: true, Payment: true},
	{Code: EventVATThresholdExceeded, Available: true, Payment: true},
	{Code: EventVATStatusChanged, Available: true, Payment: true},
	// No source yet: the bank statement gap and the monthly report do not
	// produce an event or a record the scheduler can find.
	{Code: EventBankStatementGap},
	{Code: EventMonthlyReport},
}

func eventInfo(code string) (EventInfo, bool) {
	for _, e := range Events {
		if e.Code == code {
			return e, true
		}
	}
	return EventInfo{}, false
}

// DocLateAfter is how long a document request waits before "belge istendi
// ama 3 gündür gelmedi" fires.
const DocLateAfter = 3 * 24 * time.Hour

// Trigger is the trigger_config of a rule. Unused fields stay zero.
type Trigger struct {
	// Calendar.
	Mode          string `json:"mode"`
	Days          []int  `json:"days"`
	Hour          int    `json:"hour"`
	BeforeDueDays int    `json:"before_due_days"`
	Tax           string `json:"tax"`
	// CurrentPeriod: :donem is the running month (e.g. the 29th KDV
	// report), not the month being declared.
	CurrentPeriod bool `json:"current_period"`
	// Event.
	Event           string `json:"event"`
	DelayMinHours   int    `json:"delay_min_hours"`
	DelayMaxHours   int    `json:"delay_max_hours"`
	ResendAfterDays int    `json:"resend_after_days"`
	// Payment (one taxpayer).
	PaymentDate       string `json:"payment_date"`
	PaymentAmount     string `json:"payment_amount"`
	PaymentDaysBefore int    `json:"payment_days_before"`
}

// Input is a rule as the dialog sends it.
type Input struct {
	Name               string   `json:"name"`
	TriggerKind        string   `json:"trigger_kind"`
	Trigger            Trigger  `json:"trigger"`
	TargetKind         string   `json:"target_kind"`
	TargetGroupUUID    *string  `json:"target_group_uuid"`
	TargetEngagements  []string `json:"target_engagements"`
	OnlyDue            bool     `json:"only_due"`
	Channels           []string `json:"channels"`
	SMSIfUnopenedHours *int     `json:"sms_if_unopened_hours"`
	Subject            string   `json:"subject"`
	Body               string   `json:"body"`
	ApprovalMode       string   `json:"approval_mode"`
	Active             bool     `json:"active"`
}

var (
	// ErrInvalid: a field of the rule is invalid (the message says which).
	ErrInvalid = errors.New("automation: invalid rule")
	// ErrNotFound: unknown rule or run.
	ErrNotFound = errors.New("automation: not found")
)

func invalid(msg string) error { return fmt.Errorf("%w: %s", ErrInvalid, msg) }

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Validate normalizes and checks a rule. Campaign wording, WhatsApp, the
// "yakında" events and group targets (until #780 B) are refused.
func Validate(in *Input) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Subject = strings.TrimSpace(in.Subject)
	in.Body = strings.TrimSpace(in.Body)
	if in.ApprovalMode == "" {
		in.ApprovalMode = ApprovalAuto
	}
	if in.TargetKind == "" {
		in.TargetKind = TargetAll
	}
	switch n := utf8.RuneCountInString(in.Name); {
	case n == 0:
		return invalid("kural adı gerekli")
	case n > 120:
		return invalid("kural adı en fazla 120 karakter")
	}
	if err := validateTrigger(in); err != nil {
		return err
	}
	switch in.TargetKind {
	case TargetAll:
		in.TargetEngagements = nil
	case TargetSelected:
		if len(in.TargetEngagements) == 0 {
			return invalid("en az bir mükellef seçin")
		}
		if len(in.TargetEngagements) > 500 {
			return invalid("en fazla 500 mükellef seçilebilir")
		}
	case TargetGroup:
		// TODO(#780 B): mükellef grupları paralel PR'da geliyor; grup tablosu
		// bu dalda yok, grup hedefi o zamana kadar kabul edilmez.
		return invalid("grup hedefi mükellef grupları geldiğinde açılacak")
	default:
		return invalid("hedef tüm mükellefler, grup ya da seçili mükellefler olmalı")
	}
	if in.TriggerKind == TriggerPayment && (in.TargetKind != TargetSelected || len(in.TargetEngagements) != 1) {
		return invalid("kişiye özel ödeme kuralında tek bir mükellef seçilir")
	}
	seen := map[string]bool{}
	chans := []string{}
	for _, c := range in.Channels {
		switch c {
		case ChannelSMS, ChannelEmail, ChannelInapp:
			if !seen[c] {
				seen[c] = true
				chans = append(chans, c)
			}
		case "whatsapp":
			return invalid("WhatsApp kanalı yakında")
		default:
			return invalid("kanal SMS, e-posta ya da uygulama içi olmalı")
		}
	}
	if len(chans) == 0 {
		return invalid("en az bir kanal seçin")
	}
	in.Channels = chans
	if in.SMSIfUnopenedHours != nil {
		if !seen[ChannelSMS] || !seen[ChannelEmail] {
			return invalid("\"e-posta açılmadıysa SMS\" için SMS ve e-posta birlikte seçilir")
		}
		if h := *in.SMSIfUnopenedHours; h < 1 || h > 168 {
			return invalid("e-posta açılmadıysa SMS süresi 1 ile 168 saat arasında olmalı")
		}
	}
	switch n := utf8.RuneCountInString(in.Body); {
	case n == 0:
		return invalid("mesaj metni gerekli")
	case n > 2000:
		return invalid("mesaj metni en fazla 2000 karakter")
	}
	if seen[ChannelEmail] && in.Subject == "" {
		return invalid("e-posta için konu gerekli")
	}
	if utf8.RuneCountInString(in.Subject) > 200 {
		return invalid("konu en fazla 200 karakter")
	}
	if w := communication.CampaignWords(in.Subject + "\n" + in.Body); len(w) > 0 {
		return fmt.Errorf("%w: %s", communication.ErrCampaignText, strings.Join(w, ", "))
	}
	if seen[ChannelSMS] {
		if l := SMSPreviewLength(in.Body); l.Parts > 6 {
			return invalid("SMS metni örnek mükellefle 6 SMS'i geçiyor; kısaltın")
		}
	}
	if in.ApprovalMode != ApprovalAuto && in.ApprovalMode != ApprovalDraft {
		return invalid("onay modu otomatik ya da taslak olmalı")
	}
	return nil
}

func validateTrigger(in *Input) error {
	t := &in.Trigger
	if t.Hour < 0 || t.Hour > 23 {
		return invalid("saat 0 ile 23 arasında olmalı")
	}
	switch in.TriggerKind {
	case TriggerCalendar:
		if t.Tax != "" && t.Tax != TaxKDV && t.Tax != TaxMuhtasar && t.Tax != TaxGecici {
			return invalid("beyan KDV, muhtasar ya da geçici vergi olmalı")
		}
		switch t.Mode {
		case ModeDays:
			if len(t.Days) == 0 || len(t.Days) > 10 {
				return invalid("ayın 1 ile 10 arası günü seçilir")
			}
			seen := map[int]bool{}
			days := []int{}
			for _, d := range t.Days {
				if d < 1 || d > 31 {
					return invalid("gün 1 ile 31 arasında olmalı")
				}
				if !seen[d] {
					seen[d] = true
					days = append(days, d)
				}
			}
			t.Days = days
			t.BeforeDueDays = 0
		case ModeBeforeDue:
			if t.Tax == "" {
				return invalid("son gün öncesi için beyan seçilir (KDV, muhtasar ya da geçici vergi)")
			}
			if t.BeforeDueDays < 0 || t.BeforeDueDays > 20 {
				return invalid("son günden 0 ile 20 gün önce seçilebilir")
			}
			t.Days = nil
		default:
			return invalid("takvim tetikleyicisi ayın günleri ya da son gün öncesi olmalı")
		}
		t.Event, t.DelayMinHours, t.DelayMaxHours, t.ResendAfterDays = "", 0, 0, 0
		t.PaymentDate, t.PaymentAmount, t.PaymentDaysBefore = "", "", 0
	case TriggerEvent:
		info, ok := eventInfo(t.Event)
		if !ok {
			return invalid("bilinmeyen olay")
		}
		if !info.Available {
			return invalid("bu olay yakında")
		}
		if t.DelayMinHours < 0 || t.DelayMinHours > 720 || t.DelayMaxHours < 0 || t.DelayMaxHours > 720 {
			return invalid("gecikme 0 ile 720 saat arasında olmalı")
		}
		if t.DelayMaxHours > 0 && t.DelayMaxHours < t.DelayMinHours {
			return invalid("en fazla gecikme en az gecikmeden küçük olamaz")
		}
		if t.ResendAfterDays < 0 || t.ResendAfterDays > 60 {
			return invalid("tekrar gönderim 0 ile 60 gün arasında olmalı")
		}
		if t.ResendAfterDays > 0 && !info.Payment {
			return invalid("\"hâlâ ödenmediyse tekrar gönder\" yalnız ödeme olaylarında seçilir")
		}
		t.Mode, t.Days, t.BeforeDueDays, t.Tax, t.CurrentPeriod = "", nil, 0, "", false
		t.PaymentDate, t.PaymentAmount, t.PaymentDaysBefore = "", "", 0
	case TriggerPayment:
		t.PaymentDate = strings.TrimSpace(t.PaymentDate)
		if !datePattern.MatchString(t.PaymentDate) {
			return invalid("ödeme tarihi gerekli")
		}
		if _, err := time.Parse("2006-01-02", t.PaymentDate); err != nil {
			return invalid("ödeme tarihi geçersiz")
		}
		t.PaymentAmount = strings.TrimSpace(t.PaymentAmount)
		if t.PaymentAmount != "" {
			if _, err := strconv.ParseFloat(strings.ReplaceAll(t.PaymentAmount, ",", "."), 64); err != nil {
				return invalid("ödeme tutarı sayı olmalı")
			}
		}
		if t.PaymentDaysBefore < 0 || t.PaymentDaysBefore > 30 {
			return invalid("ödemeden 0 ile 30 gün önce seçilebilir")
		}
		t.Mode, t.Days, t.BeforeDueDays, t.Tax, t.CurrentPeriod = "", nil, 0, "", false
		t.Event, t.DelayMinHours, t.DelayMaxHours, t.ResendAfterDays = "", 0, 0, 0
	default:
		return invalid("tetikleyici takvim, olay ya da kişiye özel ödeme olmalı")
	}
	if t.Days == nil {
		t.Days = []int{}
	}
	return nil
}

// Istanbul is the office clock: calendar rules fire on Turkish dates.
var Istanbul = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*60*60)
}()

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, Istanbul) }

func dayOnly(t time.Time) time.Time {
	t = t.In(Istanbul)
	return date(t.Year(), t.Month(), t.Day())
}

func lastDay(y int, m time.Month) int { return date(y, m+1, 0).Day() }

// shiftWeekend moves a due date that falls on a weekend to Monday (VUK:
// the last day passes to the next business day). Public holidays are not
// known here.
func shiftWeekend(d time.Time) time.Time {
	switch d.Weekday() {
	case time.Saturday:
		return d.AddDate(0, 0, 2)
	case time.Sunday:
		return d.AddDate(0, 0, 1)
	}
	return d
}

// Period is YYYY-MM.
func periodOf(t time.Time) string { return t.Format("2006-01") }

func parsePeriod(p string) (int, time.Month, bool) {
	t, err := time.Parse("2006-01", p)
	if err != nil {
		return 0, 0, false
	}
	return t.Year(), t.Month(), true
}

// DueDate is the last day of the tax return of a period (YYYY-MM):
// KDV the 28th and muhtasar the 26th of the next month; geçici vergi (the
// quarter ending in March, June, September) the 17th of the second month
// after the quarter. Weekends move to Monday.
func DueDate(tax, period string) (time.Time, bool) {
	y, m, ok := parsePeriod(period)
	if !ok {
		return time.Time{}, false
	}
	switch tax {
	case TaxKDV:
		return shiftWeekend(date(y, m+1, 28)), true
	case TaxMuhtasar:
		return shiftWeekend(date(y, m+1, 26)), true
	case TaxGecici:
		if m != 3 && m != 6 && m != 9 {
			return time.Time{}, false
		}
		return shiftWeekend(date(y, m+2, 17)), true
	}
	return time.Time{}, false
}

// geciciFilingMonth: geçici vergi is filed in May, August and November.
func geciciFilingMonth(m time.Month) bool { return m == 5 || m == 8 || m == 11 }

// PeriodFor is the period a reminder on day `on` is about: the previous
// month (KDV, muhtasar, statements), the quarter filed in that month
// (geçici vergi), or the running month when current is set.
func PeriodFor(tax string, on time.Time, current bool) string {
	on = dayOnly(on)
	if current {
		return periodOf(on)
	}
	if tax == TaxGecici {
		// The latest quarter end at least one month back: in May → March.
		q := date(on.Year(), on.Month(), 1).AddDate(0, -1, 0)
		for q.Month() != 3 && q.Month() != 6 && q.Month() != 9 {
			q = q.AddDate(0, -1, 0)
		}
		return periodOf(q)
	}
	return periodOf(date(on.Year(), on.Month(), 1).AddDate(0, -1, 0))
}

// Occurrence is one firing of a calendar or payment rule on a day.
type Occurrence struct {
	Period string
	// RunKey together with the rule and the taxpayer makes the run unique.
	RunKey string
	// Due is the due date the message is about (zero when none).
	Due time.Time
}

// MatchCalendar reports whether a calendar trigger fires at now (office
// clock): the day matches and the hour has come. Day 31 (or any day past
// the month's end) means the last day of the month. A geçici vergi rule
// with days fires only in the filing months (May, August, November).
func MatchCalendar(t Trigger, now time.Time) (Occurrence, bool) {
	now = now.In(Istanbul)
	if now.Hour() < t.Hour {
		return Occurrence{}, false
	}
	today := dayOnly(now)
	switch t.Mode {
	case ModeDays:
		if t.Tax == TaxGecici && !geciciFilingMonth(today.Month()) {
			return Occurrence{}, false
		}
		last := lastDay(today.Year(), today.Month())
		hit := false
		for _, d := range t.Days {
			if d == today.Day() || (d > last && today.Day() == last) {
				hit = true
				break
			}
		}
		if !hit {
			return Occurrence{}, false
		}
		period := PeriodFor(t.Tax, today, t.CurrentPeriod)
		occ := Occurrence{Period: period, RunKey: fmt.Sprintf("cal:%s:d%02d", periodOf(today), today.Day())}
		if due, ok := DueDate(t.Tax, period); ok {
			occ.Due = due
		}
		return occ, true
	case ModeBeforeDue:
		target := today.AddDate(0, 0, t.BeforeDueDays)
		// The due date of the target month, or of the month before when a
		// weekend pushed it over (28 February on a Saturday → 2 March).
		for _, back := range []int{1, 2, 3} {
			p := date(target.Year(), target.Month(), 1).AddDate(0, -back, 0)
			period := periodOf(p)
			due, ok := DueDate(t.Tax, period)
			if ok && due.Equal(target) {
				return Occurrence{Period: period, RunKey: "due:" + t.Tax + ":" + period, Due: due}, true
			}
		}
	}
	return Occurrence{}, false
}

// MatchPayment reports whether a payment trigger fires at now: on the
// payment date minus the days before, once the hour has come.
func MatchPayment(t Trigger, now time.Time) (Occurrence, bool) {
	now = now.In(Istanbul)
	if now.Hour() < t.Hour {
		return Occurrence{}, false
	}
	pay, err := time.ParseInLocation("2006-01-02", t.PaymentDate, Istanbul)
	if err != nil {
		return Occurrence{}, false
	}
	if !dayOnly(now).Equal(pay.AddDate(0, 0, -t.PaymentDaysBefore)) {
		return Occurrence{}, false
	}
	return Occurrence{Period: periodOf(pay), RunKey: "pay:" + t.PaymentDate, Due: pay}, true
}

// EventRunKey is the run key of an event: its dedupe_key when it has one
// (KDV events), else the event name and the source record.
func EventRunKey(event, dedupe, ref string) string {
	if strings.TrimSpace(dedupe) != "" {
		return "evt:" + strings.TrimSpace(dedupe)
	}
	return "evt:" + event + ":" + ref
}

// Variables of the message text.
var Variables = []string{"mukellef", "ofis", "donem", "tutar", "son_gun", "beyan", "banka", "link"}

var varPattern = regexp.MustCompile(`:(mukellef|ofis|donem|tutar|son_gun|beyan|banka|link)\b`)

// Render places the variables into a text. A variable without a value
// becomes empty; doubled spaces left behind are folded.
func Render(text string, vars map[string]string) string {
	out := varPattern.ReplaceAllStringFunc(text, func(m string) string {
		return vars[m[1:]]
	})
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	out = strings.ReplaceAll(out, " ,", ",")
	out = strings.ReplaceAll(out, " .", ".")
	return strings.TrimSpace(out)
}

// SampleVars is the sample taxpayer of the preview and the length check.
var SampleVars = map[string]string{
	"mukellef": "Örnek Gıda Ticaret Ltd. Şti.",
	"ofis":     "Demo Mali Müşavirlik",
	"donem":    "Ekim 2026",
	"tutar":    "12.450,00 TL",
	"son_gun":  "28 Kasım 2026",
	"beyan":    "KDV",
	"banka":    "Ziraat Bankası",
	"link":     "https://app.ornek.com/m/abc",
}

// SMSPreviewLength is the SMS length of a text with the sample taxpayer
// (TR encoding).
func SMSPreviewLength(body string) communication.Length {
	return communication.SMSLength(Render(body, SampleVars), "TR")
}

var monthsTR = [...]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

// FormatPeriod writes a period for the taxpayer: "Ekim 2026"; geçici
// vergi "2026 3. dönem".
func FormatPeriod(tax, period string) string {
	y, m, ok := parsePeriod(period)
	if !ok {
		return period
	}
	if tax == TaxGecici {
		return fmt.Sprintf("%d %d. dönem", y, (int(m)+2)/3)
	}
	return monthsTR[m-1] + " " + strconv.Itoa(y)
}

// FormatDate writes a date as "28 Kasım 2026".
func FormatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.In(Istanbul)
	return fmt.Sprintf("%d %s %d", t.Day(), monthsTR[t.Month()-1], t.Year())
}

// FormatAmount writes a decimal text ("12450.5") as "12.450,50 TL"; ""
// stays empty.
func FormatAmount(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return ""
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	neg := f < 0
	if neg {
		f = -f
	}
	raw := strconv.FormatFloat(f, 'f', 2, 64)
	intPart, frac, _ := strings.Cut(raw, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := b.String() + "," + frac + " TL"
	if neg {
		out = "-" + out
	}
	return out
}

// TaxLabel is the :beyan text of a tax.
func TaxLabel(tax string) string {
	switch tax {
	case TaxKDV:
		return "KDV"
	case TaxMuhtasar:
		return "Muhtasar ve prim hizmet"
	case TaxGecici:
		return "Geçici vergi"
	}
	return ""
}

// returnKind maps a tax to the tax_returns kind (geçici vergi has none).
func returnKind(tax string) string {
	switch tax {
	case TaxKDV:
		return "kdv1"
	case TaxMuhtasar:
		return "muhsgk"
	}
	return ""
}

// taxOfReturn maps a tax_returns kind back to a tax.
func taxOfReturn(kind string) string {
	switch kind {
	case "kdv1":
		return TaxKDV
	case "muhsgk":
		return TaxMuhtasar
	}
	return ""
}

func returnLabel(kind string) string {
	switch kind {
	case "kdv1":
		return "KDV"
	case "muhsgk":
		return "Muhtasar ve prim hizmet"
	case "babs":
		return "Form Ba/Bs"
	}
	return kind
}
