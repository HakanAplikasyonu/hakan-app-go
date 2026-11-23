package documents

import (
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/parse"
)

// Candidate is an active engagement a file may belong to.
type Candidate struct {
	EngagementUUID uuid.UUID
	TaxID          string
	Title          string
}

// Match sources (agent_documents.match_source).
const (
	MatchContent = "content" // VKN / TCKN in the file
	MatchWatch   = "watch"   // the watched folder's engagement
	MatchFolder  = "folder"  // taxpayer title or VKN in the folder path
	MatchOffice  = "office"  // chosen by the office
)

// ByTaxID returns the one engagement whose taxpayer has one of ids. Two
// different taxpayers matched (an employer and a client VKN in one banner):
// no match.
func ByTaxID(ids []string, cands []Candidate) (Candidate, bool) {
	if len(ids) == 0 {
		return Candidate{}, false
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var found []Candidate
	for _, c := range cands {
		if c.TaxID != "" && want[c.TaxID] {
			found = append(found, c)
		}
	}
	return only(found)
}

// ByFolder matches the folder path, deepest segment first: a VKN / TCKN in
// the segment, then the taxpayer title ("Örnek Gıda Ltd. Şti." matches a
// folder "ornek gida" or "Örnek Gıda Ltd Şti"). A segment matching several
// taxpayers, or a generic one ("Luca", "Bordro", "Desktop"), says nothing.
func ByFolder(folder string, cands []Candidate) (Candidate, bool) {
	segs := segments(folder)
	for i := len(segs) - 1; i >= 0; i-- {
		if c, ok := bySegment(segs[i], cands); ok {
			return c, true
		}
	}
	return Candidate{}, false
}

// ByFolderLeaf matches only the deepest folder of the path. A subfolder per
// taxpayer under one watched folder ("Fişler/Örnek Gıda" or
// "Fişler/1234567890") names the taxpayer explicitly, so it wins over the
// watched folder's own engagement.
func ByFolderLeaf(folder string, cands []Candidate) (Candidate, bool) {
	segs := segments(folder)
	if len(segs) == 0 {
		return Candidate{}, false
	}
	return bySegment(segs[len(segs)-1], cands)
}

func segments(folder string) []string {
	return strings.FieldsFunc(folder, func(r rune) bool { return r == '/' || r == '\\' })
}

// bySegment matches one folder name: a VKN / TCKN in it, then the taxpayer
// title (the exact core, else a single prefix match).
func bySegment(seg string, cands []Candidate) (Candidate, bool) {
	if c, ok := ByTaxID(TaxIDsIn(seg), cands); ok {
		return c, true
	}
	core := titleCore(seg)
	if len(core) < 3 || generic[core] {
		return Candidate{}, false
	}
	var exact, prefix []Candidate
	for _, c := range cands {
		t := titleCore(c.Title)
		switch {
		case t == "":
		case t == core:
			exact = append(exact, c)
		case strings.HasPrefix(t+" ", core+" "):
			prefix = append(prefix, c)
		}
	}
	if c, ok := only(exact); ok {
		return c, true
	}
	if len(exact) == 0 {
		return only(prefix)
	}
	return Candidate{}, false
}

// only returns the single candidate (by engagement).
func only(cs []Candidate) (Candidate, bool) {
	if len(cs) == 0 {
		return Candidate{}, false
	}
	for _, c := range cs[1:] {
		if c.EngagementUUID != cs[0].EngagementUUID {
			return Candidate{}, false
		}
	}
	return cs[0], true
}

// formWords are company form words left out of a title ("Ltd. Şti.",
// "A.Ş.", "Tic. San.").
var formWords = map[string]bool{
	"ltd": true, "sti": true, "limited": true, "sirketi": true, "sirket": true, "anonim": true, "as": true,
	"tic": true, "ticaret": true, "san": true, "sanayi": true, "ve": true, "ltdsti": true, "kollektif": true,
}

// generic folder names that are not a taxpayer.
var generic = map[string]bool{
	"luca": true, "zirve": true, "logo": true, "bordro": true, "bordrolar": true, "mizan": true, "mizanlar": true,
	"dokum": true, "dokumler": true, "export": true, "exports": true, "downloads": true, "indirilenler": true,
	"desktop": true, "masaustu": true, "belgeler": true, "documents": true, "users": true, "yabanci": true,
	"faturalar": true, "fatura": true, "yabanci faturalar": true, "hakan": true, "onedrive": true,
	"fis": true, "fisler": true, "gider": true, "giderler": true, "gelir": true, "makbuz": true, "makbuzlar": true,
	"masraf": true, "masraflar": true, "fotograflar": true, "resimler": true, "pictures": true, "tarama": true,
	"taramalar": true, "scan": true, "scans": true, "raporu": true, "raporlari": true, "gelen": true,
}

// titleCore normalizes a title or a folder name: Turkish letters folded,
// punctuation and company form words dropped, single letters dropped.
func titleCore(s string) string {
	ws := strings.Fields(parse.Normalize(s))
	out := ws[:0]
	for _, w := range ws {
		if len(w) < 2 || formWords[w] {
			continue
		}
		out = append(out, w)
	}
	return strings.Join(out, " ")
}
