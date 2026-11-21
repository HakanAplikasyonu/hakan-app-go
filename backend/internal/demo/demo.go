// Package demo holds the reproducible demo environment for the internal
// review (D17, issue #17): the office, its members, taxpayers and a UBL set
// derived from the GİB samples in go-ubltr (scripts/demo/derive-ubl.py).
//
// Every identity here is invented. Tax ids are test values that belong to no
// real taxpayer; the frontend demo store (features/office/data/demo.ts) uses
// the same ones so the screens and the seed tell the same story.
package demo

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

// OrgSlug is the demo office. The seed refuses to touch any other org.
const (
	OrgSlug = "demo-smmm"
	OrgName = "Demir Mali Müşavirlik (demo)"
	// EmailDomain keeps demo accounts recognisable and out of real domains.
	EmailDomain = "demo.hakan.local"
)

// Member is a demo office member. RoleKey is an organization role key; the
// template roles today are owner / admin / member (office-specific roles such
// as "stajyer" arrive with issue #5).
type Member struct {
	Email   string
	Name    string
	Surname string
	RoleKey string
	// Title is the office role the person plays in the demo story.
	Title string
}

// Taxpayer is a demo taxpayer. Tax ids pass the VKN / TCKN checksum
// (internal/platform/taxid) but belong to no real taxpayer.
type Taxpayer struct {
	Key string
	// Kind is "legal" or "individual" (taxpayers.kind).
	Kind      string
	TaxID     string
	Title     string
	TaxOffice string
	// Assignees are Member emails assigned to the engagement.
	Assignees []string
	// UBLPrefix selects this taxpayer's files in UBL().
	UBLPrefix string
}

var Members = []Member{
	{Email: "sahip@" + EmailDomain, Name: "Mert", Surname: "Demir", RoleKey: "owner", Title: "Sahip (SMMM)"},
	{Email: "musavir@" + EmailDomain, Name: "Ayşe", Surname: "Demir", RoleKey: "admin", Title: "Müşavir"},
	{Email: "stajyer@" + EmailDomain, Name: "Can", Surname: "Kaya", RoleKey: "member", Title: "Stajyer"},
}

// TaxpayerUser is the taxpayer-side account (G4). The seed makes it a
// TaxpayerUserRole member (taxpayer_members, issue #4) of TaxpayerUserOf.
var TaxpayerUser = Member{Email: "mukellef@" + EmailDomain, Name: "Okan", Surname: "Atlas", Title: "Atlas Gıda yetkilisi"}

// TaxpayerUserOf is the Taxpayer.Key the taxpayer-side account belongs to.
const (
	TaxpayerUserOf   = "atlas"
	TaxpayerUserRole = "owner"
)

// WhatsAppChatKey / WhatsAppChatLabel are the demo taxpayer user's WhatsApp
// chat the office remembered (issue #388). The number is in the 0532 123
// 45 67 style but invented.
const (
	WhatsAppChatKey   = "905321234567"
	WhatsAppChatLabel = "Okan Atlas (Atlas Gıda)"
)

var Taxpayers = []Taxpayer{
	{
		Key: "atlas", Kind: "legal", TaxID: "9000000017", Title: "Atlas Gıda Ticaret A.Ş.", TaxOffice: "Kadıköy",
		Assignees: []string{"musavir@" + EmailDomain}, UBLPrefix: "atlas-",
	},
	{
		Key: "selin", Kind: "individual", TaxID: "10000000146", Title: "Selin Yılmaz", TaxOffice: "Çankaya",
		Assignees: []string{"musavir@" + EmailDomain, "stajyer@" + EmailDomain}, UBLPrefix: "selin-",
	},
}

// OperatingTaxpayer is a demo taxpayer that keeps an işletme defteri or a
// serbest meslek kazanç defteri (#780): no chart of accounts, an
// income/expense book instead.
type OperatingTaxpayer struct {
	Taxpayer
	// LedgerType is engagements.ledger_type: "isletme" or "serbest_meslek".
	LedgerType string
}

// OperatingTaxpayers are seeded after Taxpayers with a few months of
// invoices, expenses and Z reports (cmd/demo-seed/isletme.go). TCKNs pass
// the checksum but belong to no real person.
var OperatingTaxpayers = []OperatingTaxpayer{
	{
		Taxpayer: Taxpayer{
			Key: "kaya", Kind: "individual", TaxID: "20780000120", Title: "Hüseyin Kaya - Kaya Kırtasiye", TaxOffice: "Üsküdar",
			Assignees: []string{"musavir@" + EmailDomain},
		},
		LedgerType: "isletme",
	},
	{
		Taxpayer: Taxpayer{
			Key: "deniz", Kind: "individual", TaxID: "20780000298", Title: "Deniz Aydın - Deniz Market", TaxOffice: "Bornova",
			Assignees: []string{"musavir@" + EmailDomain},
		},
		LedgerType: "isletme",
	},
	{
		Taxpayer: Taxpayer{
			Key: "arslan", Kind: "individual", TaxID: "20780000366", Title: "Av. Elif Arslan", TaxOffice: "Kadıköy",
			Assignees: []string{"musavir@" + EmailDomain},
		},
		LedgerType: "serbest_meslek",
	},
}

//go:embed ubl/*.xml
var ublFS embed.FS

// UBLFile is one demo invoice document.
type UBLFile struct {
	Name string
	Data []byte
}

// UBL returns the demo invoices whose file name starts with prefix ("" = all),
// sorted by name.
func UBL(prefix string) ([]UBLFile, error) {
	entries, err := fs.ReadDir(ublFS, "ubl")
	if err != nil {
		return nil, err
	}
	var out []UBLFile
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		b, err := ublFS.ReadFile("ubl/" + e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, UBLFile{Name: e.Name(), Data: b})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
