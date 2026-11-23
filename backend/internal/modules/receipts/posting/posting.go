// Package posting turns captured receipts into accounting (issue #384):
// expense categories (platform default → office template → taxpayer), the
// account code suggestion against the taxpayer's imported plan (mizan
// hafızası) and the journal draft lines of an approved expense or Z report.
// It is pure: the use case loads the plan, the memory and the rows.
package posting

import (
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/engine"
)

// Category scopes.
const (
	ScopePlatform = "platform"
	ScopeOffice   = "office"
	ScopeTaxpayer = "taxpayer"
)

// Account suggestion sources (receipt_postings.account_source).
const (
	SourceCategory     = "category"
	SourceVendorMemory = "vendor_memory"
	// SourceTaxpayerMemory: the taxpayer's last approved expense account
	// (#780), used when the vendor has no history and no keyword matched.
	SourceTaxpayerMemory = "taxpayer_memory"
	SourcePlan           = "plan"
	SourceManual         = "manual"
)

// FallbackCategory is used when nothing else matches.
const FallbackCategory = "genel"

// Category is one expense category.
type Category struct {
	UUID           *uuid.UUID `json:"uuid"`
	Code           string     `json:"code"`
	Name           string     `json:"name"`
	AccountCode    string     `json:"account_code"`
	VATAccountCode string     `json:"vat_account_code"`
	// Deductible "no": VAT is not deducted, it is added to the expense.
	Deductible   string   `json:"deductible"`
	ReceiptTypes []string `json:"receipt_types"`
	Keywords     []string `json:"keywords"`
	Active       bool     `json:"is_active"`
	Scope        string   `json:"scope"`
	// Overrides: the scope above has a category with this code.
	Overrides string `json:"overrides,omitempty"`
}

func def(code, name, account string, types []string, keywords ...string) Category {
	return Category{Code: code, Name: name, AccountCode: account, VATAccountCode: "191", Deductible: "yes",
		ReceiptTypes: nonNil(types), Keywords: nonNil(keywords), Active: true, Scope: ScopePlatform}
}

// DefaultCategories is the platform template (Tek Düzen Hesap Planı, genel
// yönetim giderleri). Offices override by code.
var DefaultCategories = []Category{
	def("akaryakit", "Akaryakıt", "770", []string{"fuel_receipt"}, "petrol", "opet", "shell", "akaryakıt", "aytemiz", "total"),
	def("yemek", "Yemek ve ikram", "770", nil, "lokanta", "restoran", "restaurant", "cafe", "kafe", "yemek", "pide", "kebap", "fırın", "pastane"),
	def("ulasim", "Ulaşım ve konaklama", "770", nil, "taksi", "otel", "hotel", "havayol", "otobüs", "seyahat", "turizm", "otopark"),
	def("kirtasiye", "Kırtasiye ve ofis malzemesi", "770", nil, "kırtasiye", "kitap", "ofis", "büro"),
	def("kargo", "Kargo ve posta", "770", nil, "kargo", "ptt", "kurye"),
	def("iletisim", "Telefon ve internet", "770", nil, "turkcell", "vodafone", "telekom", "internet"),
	def("bakim", "Bakım ve onarım", "770", nil, "servis", "lastik", "yedek parça", "tamir", "bakım"),
	def("temizlik", "Temizlik malzemesi", "770", nil, "temizlik", "deterjan"),
	def("banka", "Banka masrafı", "780", []string{"bank_fee"}, "banka"),
	func() Category {
		c := def("kkeg", "Kanunen kabul edilmeyen gider", "689", nil)
		c.Deductible = "no"
		return c
	}(),
	def(FallbackCategory, "Genel yönetim gideri", "770", []string{"okc_receipt", "expense_voucher", "smm", "other"}),
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Merge lays office and taxpayer rows over the platform defaults by code:
// taxpayer > office > platform. Inactive rows stay in the result (the
// screen shows them); Active filters them.
func Merge(defaults, office, taxpayer []Category) []Category {
	by := map[string]Category{}
	for _, layer := range [][]Category{defaults, office, taxpayer} {
		for _, c := range layer {
			if prev, ok := by[c.Code]; ok {
				c.Overrides = prev.Scope
			}
			by[c.Code] = c
		}
	}
	out := make([]Category, 0, len(by))
	for _, c := range by {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		// Fallback last, the rest by name.
		if (out[i].Code == FallbackCategory) != (out[j].Code == FallbackCategory) {
			return out[j].Code == FallbackCategory
		}
		return engine.Fold(out[i].Name) < engine.Fold(out[j].Name)
	})
	return out
}

// Active returns the active categories.
func Active(cats []Category) []Category {
	out := make([]Category, 0, len(cats))
	for _, c := range cats {
		if c.Active {
			out = append(out, c)
		}
	}
	return out
}

// Find returns the active category with code.
func Find(cats []Category, code string) (Category, bool) {
	for _, c := range cats {
		if c.Code == code && c.Active {
			return c, true
		}
	}
	return Category{}, false
}

// Pick chooses a category for a receipt: a vendor-name keyword first (the
// most specific), then the receipt type, then the fallback. ok=false only
// when no active category exists at all.
func Pick(cats []Category, receiptType, vendorName, description string) (Category, bool) {
	active := Active(cats)
	if c, ok := PickKeyword(cats, vendorName, description); ok {
		return c, true
	}
	var fallback *Category
	for i, c := range active {
		for _, t := range c.ReceiptTypes {
			if t == receiptType && c.Code != FallbackCategory {
				return c, true
			}
		}
		if c.Code == FallbackCategory {
			fallback = &active[i]
		}
	}
	if fallback != nil {
		return *fallback, true
	}
	if len(active) > 0 {
		return active[len(active)-1], true
	}
	return Category{}, false
}

// PickKeyword is the category whose keyword starts a word of the vendor
// name or description ("petrol" in "Petrol Ofisi"); ok=false when none.
func PickKeyword(cats []Category, vendorName, description string) (Category, bool) {
	text := " " + strings.Join(strings.FieldsFunc(engine.Fold(vendorName+" "+description), notWordRune), " ")
	for _, c := range Active(cats) {
		for _, k := range c.Keywords {
			if k = engine.Fold(k); k != "" && strings.Contains(text, " "+k) {
				return c, true
			}
		}
	}
	return Category{}, false
}

// Resolution is an account code matched to the plan.
type Resolution struct {
	Code string
	// InPlan: the code is a leaf of the taxpayer's plan.
	InPlan     bool
	Confidence decimal.Decimal
}

var (
	confNoPlan = decimal.RequireFromString("0.6000")
	confGuess  = decimal.RequireFromString("0.6500")
	confByName = decimal.RequireFromString("0.8000")
	confExact  = decimal.RequireFromString("0.8500")
	confMemory = decimal.RequireFromString("0.9500")
	// confTaxpayer: same taxpayer, another vendor: a weaker hint.
	confTaxpayer    = decimal.RequireFromString("0.7000")
	confManual      = decimal.RequireFromString("1.0000")
	errNoAmount     = errors.New("posting: amount is required")
	errVATOverTotal = errors.New("posting: the VAT is more than the total")
)

// ResolveLeaf maps code (a main account such as "770" or a full code) to a
// leaf of the plan (the accounts under code's main code). An exact leaf
// wins; else the only leaf under code; else the leaf whose name shares the
// most words with hint; else the lowest leaf code. No plan (or nothing
// under code): code itself, unverified.
func ResolveLeaf(code, hint string, plan []engine.PlanAccount) Resolution {
	var under []engine.PlanAccount
	for _, a := range plan {
		if !a.IsLeaf {
			continue
		}
		if a.Code == code {
			return Resolution{Code: a.Code, InPlan: true, Confidence: confExact}
		}
		if strings.HasPrefix(a.Code, code+".") {
			under = append(under, a)
		}
	}
	switch len(under) {
	case 0:
		return Resolution{Code: code, Confidence: confNoPlan}
	case 1:
		return Resolution{Code: under[0].Code, InPlan: true, Confidence: confExact}
	}
	sort.Slice(under, func(i, j int) bool { return under[i].Code < under[j].Code })
	want := words(hint)
	best, bestN := under[0], 0
	for _, a := range under {
		n := 0
		for w := range words(a.Name) {
			if want[w] {
				n++
			}
		}
		if n > bestN {
			best, bestN = a, n
		}
	}
	if bestN > 0 {
		return Resolution{Code: best.Code, InPlan: true, Confidence: confByName}
	}
	return Resolution{Code: best.Code, InPlan: true, Confidence: confGuess}
}

// words are the folded words of s longer than two letters.
func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(engine.Fold(s), notWordRune) {
		if len([]rune(w)) > 2 && !stopWords[w] {
			out[w] = true
		}
	}
	return out
}

// notWordRune: ASCII other than letters and digits separates words;
// Turkish letters are part of a word.
func notWordRune(r rune) bool {
	return r < 128 && (r < 'a' || r > 'z') && (r < '0' || r > '9')
}

var stopWords = map[string]bool{"ve": true, "ile": true, "gider": true, "giderleri": true, "gideri": true, "hesabı": true}

// Suggestion is the proposed category and account of an expense.
type Suggestion struct {
	CategoryCode string          `json:"category_code"`
	AccountCode  string          `json:"account_code"`
	Source       string          `json:"source"`
	Confidence   decimal.Decimal `json:"confidence"`
}

// Memory is the vendor's last approved posting (VKN memory).
type Memory struct {
	CategoryCode string
	AccountCode  string
}

// Suggest proposes the category and expense account: the vendor memory
// first (the accountant already decided for this vendor), else the picked
// category's account matched to the plan.
func Suggest(cats []Category, receiptType, vendorName, description string, mem *Memory, plan []engine.PlanAccount) (Suggestion, bool) {
	return SuggestWith(cats, receiptType, vendorName, description, mem, nil, plan)
}

// SuggestWith is Suggest with the taxpayer memory (#780): the order is the
// vendor's last account (same VKN), a vendor-name keyword's category, the
// taxpayer's last approved account, then the receipt type's / fallback
// category.
func SuggestWith(cats []Category, receiptType, vendorName, description string, mem, taxpayer *Memory, plan []engine.PlanAccount) (Suggestion, bool) {
	if taxpayer != nil && taxpayer.AccountCode != "" && (mem == nil || mem.AccountCode == "") {
		if c, ok := PickKeyword(cats, vendorName, description); ok {
			return ForCategory(c, plan), true
		}
		cat := taxpayer.CategoryCode
		if _, ok := Find(cats, cat); !ok {
			if c, ok := Pick(cats, receiptType, vendorName, description); ok {
				cat = c.Code
			}
		}
		return Suggestion{CategoryCode: cat, AccountCode: taxpayer.AccountCode, Source: SourceTaxpayerMemory, Confidence: confTaxpayer}, true
	}
	if mem != nil && mem.AccountCode != "" {
		cat := mem.CategoryCode
		if _, ok := Find(cats, cat); !ok {
			if c, ok := Pick(cats, receiptType, vendorName, description); ok {
				cat = c.Code
			}
		}
		return Suggestion{CategoryCode: cat, AccountCode: mem.AccountCode, Source: SourceVendorMemory, Confidence: confMemory}, true
	}
	c, ok := Pick(cats, receiptType, vendorName, description)
	if !ok {
		return Suggestion{}, false
	}
	return ForCategory(c, plan), true
}

// ForCategory is the suggestion for a chosen category.
func ForCategory(c Category, plan []engine.PlanAccount) Suggestion {
	r := ResolveLeaf(c.AccountCode, c.Name, plan)
	src := SourceCategory
	if r.InPlan {
		src = SourcePlan
	}
	return Suggestion{CategoryCode: c.Code, AccountCode: r.Code, Source: src, Confidence: r.Confidence}
}

// ManualConfidence is the confidence of a code the accountant typed.
func ManualConfidence() decimal.Decimal { return confManual }
