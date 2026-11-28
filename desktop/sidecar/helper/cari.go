package helper

// Senaryo 1 (tasarım §0.1): Luca'da cari sorgula, yoksa sıradaki kodla aç,
// kodu HAKAN'a yaz. Bu dosya ekransız kısmı içerir: eşleştirme, sıradaki
// kodun üretimi ve hesap planı aktarım dosyası.

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

// CariItem sunucunun gönderdiği, Luca'da aranacak / açılacak cari.
type CariItem struct {
	VKN       string `json:"vkn"`
	Name      string `json:"name"`
	Parent    string `json:"parent"` // 120, 320 …
	TaxOffice string `json:"tax_office,omitempty"`
	Address   string `json:"address,omitempty"`
}

// CodeRule bir ana hesabın alt hesap kod deseni (ofis / mükellef ayarı,
// keşifte Luca'daki gerçek kodlardan çıkarılır). Prefix "320.01.E", Width 3
// → 320.01.E001, 320.01.E002 …
type CodeRule struct {
	Prefix string `json:"prefix"`
	Width  int    `json:"width"`
}

// Cari sonuç durumları (tasarım §2.4).
const (
	CariFound   = "found"
	CariCreated = "created"
	CariFailed  = "failed"
)

// CariResult bir carinin sonucu.
type CariResult struct {
	VKN    string `json:"vkn"`
	Parent string `json:"parent"`
	Code   string `json:"code,omitempty"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// PlannedCari açılacak cari ve ona verilen kod.
type PlannedCari struct {
	Item CariItem
	Code string
}

// PlanCari her cariyi Luca hesap planında VKN ile arar. Bulunanlar
// "found"; bulunamayanlara o ana hesabın sıradaki kodu verilir.
func PlanCari(items []CariItem, accounts []Account, rules map[string]CodeRule) (found []CariResult, create []PlannedCari, failed []CariResult) {
	taken := map[string]bool{}
	for _, a := range accounts {
		taken[a.Code] = true
	}
	seen := map[string]bool{}
	for _, it := range items {
		vkn := digits(it.VKN)
		parent := strings.TrimSpace(it.Parent)
		key := parent + "|" + vkn
		if seen[key] {
			continue
		}
		seen[key] = true
		if len(vkn) != 10 && len(vkn) != 11 {
			failed = append(failed, CariResult{VKN: it.VKN, Parent: parent, Status: CariFailed, Reason: "VKN/TCKN geçersiz"})
			continue
		}
		if parent == "" {
			failed = append(failed, CariResult{VKN: vkn, Status: CariFailed, Reason: "Ana hesap (120 / 320) belirtilmemiş"})
			continue
		}
		if a, ok := findAccount(accounts, parent, vkn); ok {
			found = append(found, CariResult{VKN: vkn, Parent: parent, Code: a.Code, Name: a.Name, Status: CariFound})
			continue
		}
		code, err := NextCode(parent, taken, rules[parent])
		if err != nil {
			failed = append(failed, CariResult{VKN: vkn, Parent: parent, Status: CariFailed, Reason: err.Error()})
			continue
		}
		taken[code] = true
		it.VKN = vkn
		it.Parent = parent
		create = append(create, PlannedCari{Item: it, Code: code})
	}
	return found, create, failed
}

// findAccount ana hesabın altında VKN'si tutan hesap.
func findAccount(accounts []Account, parent, vkn string) (Account, bool) {
	for _, a := range accounts {
		if a.VKN == vkn && strings.HasPrefix(a.Code, parent+".") {
			return a, true
		}
	}
	return Account{}, false
}

var trailingDigits = regexp.MustCompile(`^(.*?)(\d+)$`)

// NextCode ana hesap altındaki en büyük kodun bir fazlası (E10 → E11,
// 001 → 002). Kural varsa yalnızca kuralın önekiyle başlayan kodlara bakar;
// hiç alt hesap yoksa kuralın ilk kodu, kural da yoksa backend hesap planı
// şablonunun varsayılan düzeni (120.01.001) kullanılır.
func NextCode(parent string, taken map[string]bool, rule CodeRule) (string, error) {
	type cand struct {
		prefix string
		n      int
		width  int
		depth  int
		code   string
	}
	var cands []cand
	for code := range taken {
		if !strings.HasPrefix(code, parent+".") {
			continue
		}
		if rule.Prefix != "" && !strings.HasPrefix(code, rule.Prefix) {
			continue
		}
		m := trailingDigits.FindStringSubmatch(code)
		if m == nil {
			continue
		}
		if rule.Prefix != "" && m[1] != rule.Prefix {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		cands = append(cands, cand{prefix: m[1], n: n, width: len(m[2]), depth: strings.Count(code, "."), code: code})
	}
	var prefix string
	var n, width int
	if len(cands) == 0 {
		switch {
		case rule.Prefix != "":
			prefix, n, width = rule.Prefix, 0, max(rule.Width, 1)
		default:
			prefix, n, width = parent+".01.", 0, 3
		}
	} else {
		// En derin seviye (cari alt hesapları), sonra en büyük kodun öneki,
		// o önekte en büyük sayı.
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].depth != cands[j].depth {
				return cands[i].depth > cands[j].depth
			}
			return cands[i].code > cands[j].code
		})
		prefix = cands[0].prefix
		for _, c := range cands {
			if c.depth == cands[0].depth && c.prefix == prefix && c.n >= n {
				n, width = c.n, max(width, c.width)
			}
		}
		if rule.Width > width {
			width = rule.Width
		}
	}
	for i := 0; i < 100000; i++ {
		n++
		code := fmt.Sprintf("%s%0*d", prefix, width, n)
		if !taken[code] {
			return code, nil
		}
	}
	return "", errors.New("sıradaki cari kodu üretilemedi")
}

// BuildPlanFile yeni carileri Luca hesap planı aktarım dosyası olarak yazar
// (biçim haritada; backend hesap_plani şablonuyla aynı varsayılanlar).
func BuildPlanFile(pf PlanFileMap, rows []PlannedCari) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	if pf.Separator != "" {
		w.Comma = []rune(pf.Separator)[0]
	}
	w.UseCRLF = pf.LineEnding != "lf"
	if pf.Header {
		var head []string
		for _, c := range pf.Columns {
			head = append(head, c.Title)
		}
		if err := w.Write(head); err != nil {
			return nil, err
		}
	}
	for _, r := range rows {
		vals := map[string]string{"code": r.Code, "name": clip(r.Item.Name, 250), "tax_office": r.Item.TaxOffice, "address": clip(r.Item.Address, 250)}
		if len(r.Item.VKN) == 11 {
			vals["tckn"] = r.Item.VKN
		} else {
			vals["tax_no"] = r.Item.VKN
		}
		var rec []string
		for _, c := range pf.Columns {
			rec = append(rec, vals[c.Key])
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	out := buf.Bytes()
	if strings.EqualFold(pf.Encoding, "utf-8") {
		return out, nil
	}
	enc, err := encoding.ReplaceUnsupported(charmap.Windows1254.NewEncoder()).Bytes(out)
	if err != nil {
		return nil, fmt.Errorf("dosya Türkçe Windows kodlamasına çevrilemedi: %w", err)
	}
	return enc, nil
}
