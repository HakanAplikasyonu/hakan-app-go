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

// TaxpayerUser is the taxpayer-side account (G4). It is created as a plain
// user; linking it to Atlas Gıda needs taxpayer_members from issue #4.
var TaxpayerUser = Member{Email: "mukellef@" + EmailDomain, Name: "Okan", Surname: "Atlas", Title: "Atlas Gıda yetkilisi"}

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
