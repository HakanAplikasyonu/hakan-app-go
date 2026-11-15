package sources

import (
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

// AllowedParams, sorgu metninde geçebilecek tek parametreler (15 §2.4:
// dönem başlangıç ve bitiş tarihi).
var AllowedParams = []string{"from", "to"}

// forbiddenWords, sorgu metninde (dize ve köşeli parantez dışındaki) tek
// başına bir kelime olarak geçerse sorguyu reddeden anahtar kelimeler.
// Liste bilerek geniştir: salt okuma sorgusunda hiçbirine gerek yok.
var forbiddenWords = map[string]bool{
	"INSERT": true, "UPDATE": true, "DELETE": true, "MERGE": true,
	"EXEC": true, "EXECUTE": true, "DROP": true, "ALTER": true,
	"CREATE": true, "TRUNCATE": true, "GRANT": true, "REVOKE": true,
	"DENY": true, "INTO": true, "OPENROWSET": true, "OPENQUERY": true,
	"OPENDATASOURCE": true, "OPENXML": true, "BULK": true, "WAITFOR": true,
	"SHUTDOWN": true, "DBCC": true, "BACKUP": true, "RESTORE": true,
	"KILL": true, "USE": true, "DECLARE": true, "SET": true, "GO": true,
	"RECONFIGURE": true, "CHECKPOINT": true, "BEGIN": true, "COMMIT": true,
	"ROLLBACK": true, "SAVE": true, "TRAN": true, "TRANSACTION": true,
	"PRINT": true, "RAISERROR": true, "THROW": true, "OUTPUT": true,
	"READTEXT": true, "WRITETEXT": true, "UPDATETEXT": true, "SETUSER": true,
}

// aliasStop, FROM/JOIN'den sonra takma ad olamayacak kelimeler.
var aliasStop = map[string]bool{
	"WITH": true, "WHERE": true, "JOIN": true, "INNER": true, "LEFT": true,
	"RIGHT": true, "FULL": true, "OUTER": true, "CROSS": true, "ON": true,
	"GROUP": true, "ORDER": true, "UNION": true, "HAVING": true, "AS": true,
	"EXCEPT": true, "INTERSECT": true, "APPLY": true,
}

type tokKind int

const (
	tokWord   tokKind = iota // tanımlayıcı / anahtar kelime
	tokQuoted                // [köşeli] veya "çift tırnaklı" tanımlayıcı
	tokString                // 'dize' (N'dize' dahil)
	tokNumber
	tokParam // @ad
	tokPunct
)

type token struct {
	kind tokKind
	text string // tokWord için büyük harf; tokParam için '@' olmadan küçük harf
}

func rejectf(format string, args ...any) error {
	return rpc.Errorf(CodeQueryRejected, format, args...)
}

func isWordStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }
func isWordPart(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// tokenize T-SQL metnini kaba olarak parçalar. Yorumlar, ';', '@@',
// geçici tablo (#) ve tanınmayan karakterler reddedilir.
func tokenize(q string) ([]token, error) {
	rs := []rune(q)
	var out []token
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '-' && i+1 < len(rs) && rs[i+1] == '-',
			r == '/' && i+1 < len(rs) && rs[i+1] == '*':
			return nil, rejectf("sorguda yorum olamaz")
		case r == ';':
			return nil, rejectf("sorgu tek bir deyim olmalı (';' olamaz)")
		case r == '\'' || ((r == 'N' || r == 'n') && i+1 < len(rs) && rs[i+1] == '\''):
			if r != '\'' {
				i++
			}
			j := i + 1
			for {
				if j >= len(rs) {
					return nil, rejectf("kapanmamış dize")
				}
				if rs[j] == '\'' {
					if j+1 < len(rs) && rs[j+1] == '\'' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			out = append(out, token{tokString, string(rs[i : j+1])})
			i = j + 1
		case r == '[' || r == '"':
			end := ']'
			if r == '"' {
				end = '"'
			}
			j := i + 1
			for j < len(rs) && rs[j] != end {
				j++
			}
			if j >= len(rs) {
				return nil, rejectf("kapanmamış tanımlayıcı")
			}
			out = append(out, token{tokQuoted, string(rs[i+1 : j])})
			i = j + 1
		case r == '@':
			if i+1 < len(rs) && rs[i+1] == '@' {
				return nil, rejectf("sistem değişkeni (@@) kullanılamaz")
			}
			j := i + 1
			for j < len(rs) && isWordPart(rs[j]) {
				j++
			}
			if j == i+1 {
				return nil, rejectf("geçersiz parametre")
			}
			out = append(out, token{tokParam, strings.ToLower(string(rs[i+1 : j]))})
			i = j
		case isWordStart(r):
			j := i
			for j < len(rs) && isWordPart(rs[j]) {
				j++
			}
			out = append(out, token{tokWord, strings.ToUpper(string(rs[i:j]))})
			i = j
		case unicode.IsDigit(r):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.') {
				j++
			}
			out = append(out, token{tokNumber, string(rs[i:j])})
			i = j
		case strings.ContainsRune(",().*=<>!+-/%", r):
			out = append(out, token{tokPunct, string(r)})
			i++
		default:
			return nil, rejectf("sorguda izin verilmeyen karakter: %q", r)
		}
	}
	return out, nil
}

func isIdent(t token) bool { return t.kind == tokWord || t.kind == tokQuoted }

// ValidateQuery, sorgu metninin salt okunur, tek bir SELECT deyimi olduğunu
// denetler ve kullandığı parametreleri (küçük harf, '@' olmadan, sıralı)
// döner. Kurallar (02 §6, 14 §9, 15 §2.4):
//
//   - SELECT ile başlar; ';', yorum, '@@', geçici tablo yok
//   - yazma / yürütme anahtar kelimeleri yok (INSERT, UPDATE, DELETE, MERGE,
//     EXEC, DROP, ALTER, CREATE, TRUNCATE, GRANT, INTO, OPENROWSET, ...);
//     sp_ / xp_ ile başlayan adlar yok
//   - her FROM / JOIN tablosunun hemen ardından WITH (NOLOCK) gelir;
//     türetilmiş tablo, tablo fonksiyonu ve virgülle birleştirme yok
//   - parametreler yalnızca @from ve @to
func ValidateQuery(q string) ([]string, error) {
	toks, err := tokenize(q)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 || toks[0].kind != tokWord || toks[0].text != "SELECT" {
		return nil, rejectf("sorgu SELECT ile başlamalı")
	}
	used := map[string]bool{}
	for i, t := range toks {
		switch t.kind {
		case tokWord:
			if forbiddenWords[t.text] {
				return nil, rejectf("sorguda %s kullanılamaz", t.text)
			}
			if strings.HasPrefix(t.text, "SP_") || strings.HasPrefix(t.text, "XP_") {
				return nil, rejectf("sorguda saklı yordam çağrılamaz (%s)", t.text)
			}
			if t.text == "FROM" || t.text == "JOIN" {
				if err := checkTableRef(toks, i+1, t.text); err != nil {
					return nil, err
				}
			}
		case tokParam:
			if !slices.Contains(AllowedParams, t.text) {
				return nil, rejectf("izin verilmeyen parametre: @%s (yalnızca @from, @to)", t.text)
			}
			used[t.text] = true
		}
	}
	out := make([]string, 0, len(used))
	for p := range used {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// checkTableRef, toks[i]'den başlayan tablo başvurusunu denetler:
// ad[.ad...] [AS] [takma_ad] WITH ( NOLOCK )
func checkTableRef(toks []token, i int, kw string) error {
	at := func(j int) token {
		if j < len(toks) {
			return toks[j]
		}
		return token{kind: tokPunct, text: ""}
	}
	if !isIdent(at(i)) {
		return rejectf("%s sonrasında tablo adı bekleniyordu (türetilmiş tablo desteklenmez)", kw)
	}
	name := at(i).text
	i++
	for at(i).kind == tokPunct && at(i).text == "." {
		if !isIdent(at(i + 1)) {
			return rejectf("geçersiz tablo adı: %s", name)
		}
		name += "." + at(i+1).text
		i += 2
	}
	if at(i).kind == tokPunct && at(i).text == "(" {
		return rejectf("tablo fonksiyonu kullanılamaz: %s", name)
	}
	if at(i).kind == tokWord && at(i).text == "AS" {
		i++
		if !isIdent(at(i)) {
			return rejectf("%s için takma ad bekleniyordu", name)
		}
		i++
	} else if t := at(i); t.kind == tokQuoted || (t.kind == tokWord && !aliasStop[t.text]) {
		i++
	}
	noLock := at(i).kind == tokWord && at(i).text == "WITH" &&
		at(i+1).kind == tokPunct && at(i+1).text == "(" &&
		at(i+2).kind == tokWord && at(i+2).text == "NOLOCK" &&
		at(i+3).kind == tokPunct && at(i+3).text == ")"
	if !noLock {
		return rejectf("%s tablosu WITH (NOLOCK) ile okunmalı", name)
	}
	if t := at(i + 4); t.kind == tokPunct && t.text == "," {
		return rejectf("virgülle tablo birleştirme desteklenmez; JOIN kullanın")
	}
	return nil
}

// renderTemplate, Logo tablo adı şablonundaki {FFF} / {PP} yer tutucularını
// doğrulanmış firma / dönem no ile doldurur. Logo dışı kaynakta yer tutucu
// bulunursa hata döner.
func renderTemplate(text string, cfg Config) (string, error) {
	has := strings.Contains(text, "{FFF}") || strings.Contains(text, "{PP}")
	if !has {
		return text, nil
	}
	if !cfg.Kind.IsLogo() {
		return "", rejectf("%s kaynağında {FFF}/{PP} yer tutucusu kullanılamaz", cfg.Kind)
	}
	if err := ValidateFirmPeriod(cfg.FirmNo, cfg.PeriodNo); err != nil {
		return "", err
	}
	return strings.NewReplacer("{FFF}", cfg.FirmNo, "{PP}", cfg.PeriodNo).Replace(text), nil
}
