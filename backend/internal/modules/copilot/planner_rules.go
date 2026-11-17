package copilot

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// RulesPlanner reads a command with keyword rules: the taxpayer by its
// title, the month by its Turkish name and the action by its verbs. It
// works without an AI provider (and is the fallback when the provider
// fails).
type RulesPlanner struct{}

// Fold lowercases Turkish text and drops its diacritics ("Şubat'ı" →
// "subat'i").
func Fold(s string) string {
	s = strings.ToLowerSpecial(unicode.TurkishCase, s)
	return strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u", "â", "a", "î", "i", "û", "u").Replace(s)
}

var wordRe = regexp.MustCompile(`[a-z0-9]+`)

func words(s string) []string { return wordRe.FindAllString(Fold(s), -1) }

var months = map[string]int{
	"ocak": 1, "subat": 2, "mart": 3, "nisan": 4, "mayis": 5, "haziran": 6,
	"temmuz": 7, "agustos": 8, "eylul": 9, "ekim": 10, "kasim": 11, "aralik": 12,
}

var (
	isoPeriodRe   = regexp.MustCompile(`\b(20[0-9]{2})-(0[1-9]|1[0-2])\b`)
	slashPeriodRe = regexp.MustCompile(`\b(0?[1-9]|1[0-2])[/.](20[0-9]{2})\b`)
	yearRe        = regexp.MustCompile(`^20[0-9]{2}$`)
)

// ParsePeriod finds the month of a command; "" when none is named.
func ParsePeriod(command string, now time.Time) string {
	f := Fold(command)
	if m := isoPeriodRe.FindStringSubmatch(f); m != nil {
		return m[1] + "-" + m[2]
	}
	if m := slashPeriodRe.FindStringSubmatch(f); m != nil {
		mo, _ := strconv.Atoi(m[1])
		return m[2] + "-" + twoDigits(mo)
	}
	t := now.In(istanbul)
	ws := words(command)
	for i, w := range ws {
		if strings.HasPrefix(w, "gecen") && i+1 < len(ws) && strings.HasPrefix(ws[i+1], "ay") {
			return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, istanbul).AddDate(0, -1, 0).Format("2006-01")
		}
		if w == "bu" && i+1 < len(ws) && strings.HasPrefix(ws[i+1], "ay") {
			return t.Format("2006-01")
		}
		mo := monthOf(w)
		if mo == 0 {
			continue
		}
		year := t.Year()
		if i+1 < len(ws) && yearRe.MatchString(ws[i+1]) {
			year, _ = strconv.Atoi(ws[i+1])
		} else if i > 0 && yearRe.MatchString(ws[i-1]) {
			year, _ = strconv.Atoi(ws[i-1])
		} else if mo > int(t.Month()) {
			// "aralık" in November means last December.
			year--
		}
		return strconv.Itoa(year) + "-" + twoDigits(mo)
	}
	return ""
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// monthOf matches a month name with Turkish suffixes ("ekim", "ekimin",
// "eylül'ü" → eylul, u).
func monthOf(w string) int {
	for name, mo := range months {
		if w == name || (strings.HasPrefix(w, name) && len(w)-len(name) <= 4) {
			return mo
		}
	}
	return 0
}

// Intents found in a command.
type intent struct {
	actions []string
	// next: only the next step of each engagement; close: every remaining
	// step of the period.
	next, close bool
	driver      string
	all         bool
}

func hasPrefix(ws []string, prefixes ...string) bool {
	for _, w := range ws {
		for _, p := range prefixes {
			if strings.HasPrefix(w, p) {
				return true
			}
		}
	}
	return false
}

func parseIntent(command string) intent {
	ws := words(command)
	var in intent
	if hasPrefix(ws, "luca") {
		in.driver = "luca"
	} else if hasPrefix(ws, "zirve") {
		in.driver = "zirve"
	}
	in.all = hasPrefix(ws, "hepsi", "tumu", "herkes") ||
		(hasPrefix(ws, "tum", "butun", "her") && hasPrefix(ws, "mukellef", "musteri", "firma", "sirket"))
	approve := hasPrefix(ws, "onay")
	bank := hasPrefix(ws, "banka", "hareket", "ekstre", "eslestir", "eslesme")
	fis := hasPrefix(ws, "fis", "yevmiye", "mahsup")
	switch {
	case hasPrefix(ws, "kapat", "kapanis", "bitir", "tamamla"):
		in.close = true
	case hasPrefix(ws, "sirada", "sonraki", "devam"):
		in.next = true
	}
	if bank && !fis {
		if approve {
			in.actions = append(in.actions, ActionBankApprove)
		} else if hasPrefix(ws, "oner", "esle", "hesap") {
			in.actions = append(in.actions, ActionBankSuggest)
		}
	}
	transfer := in.driver != "" || hasPrefix(ws, "aktar", "program")
	if fis && !transfer {
		switch {
		case hasPrefix(ws, "olustur", "hazirla", "kes", "uret", "cikar"):
			in.actions = append(in.actions, ActionBankDrafts)
		case approve:
			in.actions = append(in.actions, ActionJournalsApprove)
		}
	}
	if transfer {
		in.actions = append(in.actions, ActionExportTransfer)
	}
	if hasPrefix(ws, "rapor") {
		in.actions = append(in.actions, ActionReportSend)
	}
	return in
}

// Words of a title that do not name the taxpayer.
var titleNoise = map[string]bool{
	"a": true, "s": true, "as": true, "ltd": true, "sti": true, "sirketi": true, "ve": true, "tic": true,
	"ticaret": true, "san": true, "sanayi": true, "limited": true, "anonim": true, "ins": true, "insaat": true,
}

// stopWords never name a taxpayer: months, verbs and objects of commands.
func stopWord(w string) bool {
	if monthOf(w) != 0 || len(w) < 3 {
		return true
	}
	for _, p := range []string{"fis", "rapor", "banka", "hareket", "onay", "aktar", "luca", "zirve", "gonder",
		"olustur", "kapat", "donem", "mukellef", "tum", "butun", "hepsi", "sirada", "sonraki", "yevmiye", "mahsup", "program"} {
		if strings.HasPrefix(w, p) {
			return true
		}
	}
	return false
}

// matchEngagements finds the engagements a command names: the longest run
// of a title's words that the command contains (word prefixes, so
// "Atlas'ın" matches "Atlas"). The best score wins; ties are all kept.
func matchEngagements(command string, refs []EngagementRef) []EngagementRef {
	cmd := words(command)
	best := 0
	var out []EngagementRef
	for _, e := range refs {
		var tw []string
		for _, w := range words(e.TaxpayerTitle) {
			if !titleNoise[w] {
				tw = append(tw, w)
			}
		}
		score := 0
		for i := range cmd {
			if stopWord(cmd[i]) {
				continue
			}
			for j := range tw {
				n := 0
				for i+n < len(cmd) && j+n < len(tw) && wordMatch(cmd[i+n], tw[j+n]) {
					n++
				}
				// The first word counts more: "Atlas Gıda" beats "Gıda Pazarı".
				s := n * 2
				if j == 0 && n > 0 {
					s++
				}
				if s > score {
					score = s
				}
			}
		}
		if e.TaxpayerTaxID != "" && strings.Contains(command, e.TaxpayerTaxID) {
			score = 100
		}
		switch {
		case score == 0:
		case score > best:
			best, out = score, []EngagementRef{e}
		case score == best:
			out = append(out, e)
		}
	}
	return out
}

func wordMatch(cmdWord, titleWord string) bool {
	if len(titleWord) < 3 {
		return cmdWord == titleWord
	}
	return strings.HasPrefix(cmdWord, titleWord) && len(cmdWord)-len(titleWord) <= 5
}

// Remaining write actions of a chain in order, with what each needs first.
func remaining(r Row) []string {
	c := r.counts
	var out []string
	if c.BankUnassigned > 0 {
		out = append(out, ActionBankSuggest)
	}
	if c.BankUnassigned+c.BankSuggested > 0 {
		out = append(out, ActionBankApprove)
	}
	drafts := c.BankApproved > 0 || c.BankUnassigned+c.BankSuggested > 0
	if drafts {
		out = append(out, ActionBankDrafts)
	}
	if drafts || c.JournalDraft > 0 {
		out = append(out, ActionJournalsApprove)
	}
	if (drafts || c.JournalDraft+c.JournalApproved > 0) && c.TransferFailed == 0 {
		out = append(out, ActionExportTransfer)
	}
	if c.ReportSent+c.ReportPending == 0 {
		out = append(out, ActionReportSend)
	}
	return out
}

// prerequisites of an action that the chain still needs.
func prerequisites(action string, r Row) []string {
	c := r.counts
	switch action {
	case ActionBankApprove:
		if c.BankUnassigned > 0 {
			return []string{ActionBankSuggest}
		}
	case ActionJournalsApprove:
		if c.BankApproved > 0 {
			return []string{ActionBankDrafts}
		}
	case ActionExportTransfer:
		var out []string
		if c.BankApproved > 0 {
			out = append(out, ActionBankDrafts)
		}
		if c.BankApproved > 0 || c.JournalDraft > 0 {
			out = append(out, ActionJournalsApprove)
		}
		return out
	}
	return nil
}

// relevant reports whether an action has anything to do in the chain
// (counting the prerequisites added before it).
func relevant(action string, r Row) (ok bool, done bool) {
	c := r.counts
	switch action {
	case ActionBankSuggest:
		return c.BankUnassigned > 0, c.BankTotal > 0
	case ActionBankApprove:
		return c.BankUnassigned+c.BankSuggested > 0, c.BankTotal > 0
	case ActionBankDrafts:
		return c.BankApproved > 0, c.BankTotal > 0 && c.BankApproved == 0
	case ActionJournalsApprove:
		return c.JournalDraft > 0 || c.BankApproved > 0, c.JournalApproved+c.JournalExported > 0
	case ActionExportTransfer:
		return c.JournalApproved+c.JournalDraft+c.BankApproved > 0, c.JournalExported > 0
	case ActionReportSend:
		return c.ReportSent+c.ReportPending == 0, c.ReportSent+c.ReportPending > 0
	}
	return false, false
}

// Plan implements Planner.
func (RulesPlanner) Plan(ctx context.Context, in PlanInput) (Draft, error) {
	d := Draft{Planner: PlannerRules, Notes: []Note{}}
	d.Period = ParsePeriod(in.Command, in.Now)
	if d.Period == "" {
		d.Period = in.Period
	}
	if d.Period == "" {
		d.Period = DefaultPeriod(in.Now)
	}
	it := parseIntent(in.Command)
	if len(it.actions) == 0 && !it.next && !it.close {
		d.Notes = append(d.Notes, Note{Code: NoteNotUnderstood})
		return d, nil
	}
	targets := in.Engagements
	if !it.all {
		targets = matchEngagements(in.Command, in.Engagements)
		if len(targets) == 0 {
			d.Notes = append(d.Notes, Note{Code: NoteEngagementNotFound})
			return d, nil
		}
		if len(targets) > 1 {
			d.Notes = append(d.Notes, Note{Code: NoteAmbiguous})
		}
	}
	status, err := in.Status(ctx, targets, d.Period)
	if err != nil {
		return Draft{}, err
	}
	add := func(e EngagementRef, action string) {
		d.Steps = append(d.Steps, PlanStep{Action: action, EngagementUUID: e.UUID, TaxpayerTitle: e.TaxpayerTitle,
			Period: d.Period, Driver: it.driver})
	}
	for _, e := range targets {
		row := status[e.UUID]
		if it.next || it.close {
			rest := remaining(row)
			if len(rest) == 0 {
				d.Notes = append(d.Notes, Note{Code: NoteAlreadyDone, TaxpayerTitle: e.TaxpayerTitle})
				continue
			}
			if it.next {
				rest = rest[:1]
			}
			for _, a := range rest {
				add(e, a)
			}
			continue
		}
		planned := map[string]bool{}
		for _, a := range it.actions {
			ok, done := relevant(a, row)
			if !ok {
				code := NoteNothingToDo
				if done {
					code = NoteAlreadyDone
				}
				d.Notes = append(d.Notes, Note{Code: code, Action: a, TaxpayerTitle: e.TaxpayerTitle})
				continue
			}
			for _, pre := range prerequisites(a, row) {
				if !planned[pre] && !contains(it.actions, pre) {
					planned[pre] = true
					add(e, pre)
					d.Notes = append(d.Notes, Note{Code: NotePrerequisite, Action: pre, TaxpayerTitle: e.TaxpayerTitle})
				}
			}
			if !planned[a] {
				planned[a] = true
				add(e, a)
			}
		}
	}
	return d, nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
