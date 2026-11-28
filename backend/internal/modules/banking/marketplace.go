package banking

import "strings"

// Marketplace is a sales channel whose payouts reach the bank account with
// its name (or its company's name) in the description. A payout belongs to
// the channel's cari (120.xx), not to the end customer.
type Marketplace struct {
	Key  string // stable id: "trendyol"
	Name string // shown to the accountant: "Trendyol"
	// Patterns are written as an accountant reads them (Turkish letters,
	// upper case); they are matched on the normalized description word by
	// word like the keyword table (containsPhrase), so case, Turkish letters
	// and İ/I/ı do not matter and "DSM" does not match "DSMX".
	Patterns []string
}

// Marketplaces is the channel table, first match wins. Add a row (or a
// pattern) to support another channel; the company names are the legal
// entities the payouts come from (Trendyol: DSM Grup Danışmanlık;
// Hepsiburada: D-Market Elektronik; n11: Doğuş Planet).
var Marketplaces = []Marketplace{
	{Key: "trendyol", Name: "Trendyol", Patterns: []string{"TRENDYOL", "DSM GRUP", "DSM"}},
	{Key: "hepsiburada", Name: "Hepsiburada", Patterns: []string{"HEPSİBURADA", "HEPSİ BURADA", "D-MARKET", "DMARKET", "HEPSİPAY"}},
	{Key: "amazon", Name: "Amazon", Patterns: []string{"AMAZON"}},
	{Key: "n11", Name: "n11", Patterns: []string{"N11", "DOĞUŞ PLANET"}},
	{Key: "ciceksepeti", Name: "Çiçeksepeti", Patterns: []string{"ÇİÇEKSEPETİ", "ÇİÇEK SEPETİ"}},
	{Key: "pazarama", Name: "Pazarama", Patterns: []string{"PAZARAMA"}},
}

// TemlikPattern marks an assigned receivable: the channel named next to it
// is the one that assigned (temlik eden) the payout.
const TemlikPattern = "TEMLİK"

// MarketplaceHit is a channel found in a description.
type MarketplaceHit struct {
	Marketplace Marketplace
	// Keyword is the table pattern that matched, as written in the table.
	Keyword string
	// Temlik: the description also says TEMLİK.
	Temlik bool
}

// match returns the first pattern of m found in the normalized words.
func (m Marketplace) match(words []string) (string, bool) {
	for _, p := range m.Patterns {
		if containsPhrase(words, NormalizeDescription(p)) {
			return p, true
		}
	}
	return "", false
}

// NameMatches reports whether s (a plan account name, a raw description)
// names the channel.
func (m Marketplace) NameMatches(s string) bool {
	_, ok := m.match(strings.Fields(NormalizeDescription(s)))
	return ok
}

// DetectMarketplace finds the sales channel a raw bank description names.
func DetectMarketplace(desc string) (MarketplaceHit, bool) {
	words := strings.Fields(NormalizeDescription(desc))
	for _, m := range Marketplaces {
		if kw, ok := m.match(words); ok {
			return MarketplaceHit{Marketplace: m, Keyword: kw,
				Temlik: containsPhrase(words, NormalizeDescription(TemlikPattern))}, true
		}
	}
	return MarketplaceHit{}, false
}
