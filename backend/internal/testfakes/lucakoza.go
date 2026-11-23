package testfakes

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Luca Koza credentials the fake accepts. Made-up test values only.
const (
	LucaKozaOrgCode   = "1001"
	LucaKozaUser      = "muhasebe"
	LucaKozaPassword  = "koza-test-sifresi"
	LucaKozaCompanyID = "301"
)

// LucaKoza fakes the Luca Koza web server: Giris.do (code 0 / 99) opens a
// cookie session, YdlUserResponsibilityOrgSs.do lists the companies /
// branches, GuncelleYtkSirketSubeDegistir.do selects one and
// ListeleFtrSsFaturaBaslik.do?detayliListe=true (No-Paging: true) lists the
// selected company's invoices by parUstHareketTuru and document date. Any
// other endpoint answers 404 and is kept in Unknown, so a test sees a call
// the adapter must not make.
type LucaKoza struct {
	mu sync.Mutex
	// Docs are the invoice headers by movement type (16, 17, 18, 19).
	Docs map[int][]map[string]any
	// Logins counts accepted sign-ins.
	Logins int
	// Calls are the endpoints called, in order (path without the base).
	Calls []string
	// Unknown are calls to endpoints the fake does not serve.
	Unknown []string

	selected map[string]string // session → orgSirketSubeId
}

const lucaCookie = "JSESSIONID"

func lucaRefuse(w http.ResponseWriter, msg string) {
	writeJSON(w, map[string]any{"code": 99, "message": msg})
}

func (f *LucaKoza) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.selected == nil {
		f.selected = map[string]string{}
	}
	ep := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	f.Calls = append(f.Calls, ep)
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	var in map[string]any
	_ = dec.Decode(&in)
	if ep == "Giris.do" {
		if in["orgCode"] != LucaKozaOrgCode || in["userName"] != LucaKozaUser || in["userPassword"] != LucaKozaPassword {
			lucaRefuse(w, "Kullanıcı adı veya şifre hatalı.")
			return
		}
		f.Logins++
		http.SetCookie(w, &http.Cookie{Name: lucaCookie, Value: "s" + strconv.Itoa(f.Logins), Path: "/"})
		writeJSON(w, map[string]any{"code": 0, "message": ""})
		return
	}
	ck, err := r.Cookie(lucaCookie)
	if err != nil || ck.Value == "" {
		lucaRefuse(w, "Oturum bulunamadı.")
		return
	}
	switch ep {
	case "YdlUserResponsibilityOrgSs.do":
		writeJSON(w, []map[string]any{
			{"id": json.Number(LucaKozaCompanyID), "ack": "Atlas Gıda - Merkez", "kod": "M"},
			{"id": json.Number("302"), "ack": "Atlas Gıda - Kadıköy Şube", "kod": "S1"},
		})
	case "GuncelleYtkSirketSubeDegistir.do":
		id := ""
		switch v := in["orgSirketSubeId"].(type) {
		case json.Number:
			id = v.String()
		case string:
			id = v
		}
		if id != LucaKozaCompanyID && id != "302" {
			lucaRefuse(w, "Şirket/şube yetkiniz yok.")
			return
		}
		f.selected[ck.Value] = id
		writeJSON(w, map[string]any{"code": 0})
	case "ListeleFtrSsFaturaBaslik.do":
		if r.URL.Query().Get("detayliListe") != "true" || r.Header.Get("No-Paging") != "true" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if f.selected[ck.Value] != LucaKozaCompanyID {
			writeJSON(w, map[string]any{"list": []any{}})
			return
		}
		mv, _ := strconv.Atoi(scalarString(in["parUstHareketTuru"]))
		from, _ := time.Parse("02/01/2006", scalarString(in["belgeTarihiBas"]))
		to, _ := time.Parse("02/01/2006", scalarString(in["belgeTarihiBit"]))
		out := []map[string]any{}
		for _, d := range f.Docs[mv] {
			day, err := time.Parse("02/01/2006", scalarString(d["belgeTarihi"]))
			if err == nil && !day.Before(from) && !day.After(to) {
				out = append(out, d)
			}
		}
		writeJSON(w, map[string]any{"list": out, "toplamKayit": len(out)})
	default:
		f.Unknown = append(f.Unknown, ep)
		w.WriteHeader(http.StatusNotFound)
	}
}

func scalarString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	}
	return ""
}

// LucaKozaCari is an invoice's counterparty as Koza nests it.
func LucaKozaCari(vkn, name, office string) map[string]any {
	return map[string]any{"vergiKimlikNo": vkn, "cariTanim": name, "vergiDairesi": office}
}

// LucaKozaSample is an October set with one invoice of every movement type,
// amounts as strings (Turkish and dotted) and numbers, a withholding line
// and one record of an unknown movement type (skipped by the adapter).
func LucaKozaSample() *LucaKoza {
	return &LucaKoza{Docs: map[int][]map[string]any{
		18: { // satış
			{"ssFaturaBaslikId": json.Number("1001"), "belgeTarihi": "05/10/2026", "belgeSeriNo": "KZA2026000000001",
				"cari": LucaKozaCari("9100000021", "Yıldız Market Ltd. Şti.", "Kadıköy"), "yeniBirAlan": map[string]any{"x": 1},
				"detayList": []map[string]any{
					{"miktar": "3", "birimFiyat": "1.234,56", "kdvOran": json.Number("20"), "kdvTutar": "740,74", "netTutar": "3703,68", "bilinmeyen": true},
					{"miktar": json.Number("2"), "birimFiyat": json.Number("10.005"), "kdvOran": "10", "netTutar": json.Number("20.01")},
				}},
			{"ssFaturaBaslikId": json.Number("1005"), "ustHareketTuru": "99", "belgeTarihi": "06/10/2026", "belgeSeriNo": "KZA2026000000005",
				"cari":      LucaKozaCari("9100000021", "Yıldız Market Ltd. Şti.", "Kadıköy"),
				"detayList": []map[string]any{{"miktar": "1", "birimFiyat": "100", "kdvOran": "20", "kdvTutar": "20", "netTutar": "100"}}},
		},
		17: { // satış iade
			{"ssFaturaBaslikId": "1002", "belgeTarihi": "12/10/2026", "belgeSeriNo": "KZA2026000000002",
				"cari":      LucaKozaCari("12345678901", "Ayşe Demir", "Üsküdar"),
				"detayList": []map[string]any{{"miktar": "1", "birimFiyat": "500", "kdvOran": "20", "kdvTutar": "100", "netTutar": "500"}}},
		},
		16: { // alım, tevkifatlı
			{"ssFaturaBaslikId": json.Number("1003"), "belgeTarihi": "15/10/2026", "belgeSeriNo": "TDR2026000000123",
				"cari":      LucaKozaCari("9200000032", "Toros Danışmanlık A.Ş.", "Beşiktaş"),
				"detayList": []map[string]any{{"miktar": "1", "birimFiyat": "10000.00", "kdvOran": "20", "kdvTutar": "2000.00", "tevkifatTutar": "1000.00", "netTutar": "10000.00"}}},
		},
		19: { // alım iade
			{"ssFaturaBaslikId": json.Number("1004"), "belgeTarihi": "20/10/2026", "belgeSeriNo": "KZA2026000000004",
				"cari":      LucaKozaCari("9200000032", "Toros Danışmanlık A.Ş.", "Beşiktaş"),
				"detayList": []map[string]any{{"miktar": "2", "birimFiyat": "250,5", "kdvOran": "0.20", "kdvTutar": "100,20", "netTutar": "501,00"}}},
		},
	}}
}
