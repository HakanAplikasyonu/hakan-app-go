package agent

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Sürükle-bırak yükleme (tek program): kullanıcı masaüstü penceresine dosya
// ya da klasör bırakır, mükellefi, dönemi (ay) ve belge türünü seçer.
//
// Yol: seçilen tür ve dönem ofis panelindeki "Evrak yükle" ucuna
// (`POST /v1/tenant/engagements/{e}/document-uploads`, #827) gider; Z raporu
// ve masraf fotoğrafı orada mobil ile aynı fiş okumasına girer. Tür ve dönem
// taşımayan ajan belgesi ucu (`POST /v1/tenant/agents/{a}/documents`, #831
// yolu) iki durumda kullanılır: Luca dökümü (.xlsx / .xls / .csv; sunucu
// içeriğinden tanır) ve ofisin türlü evrak yüklemeyi kapattığı durum
// (DOCUMENT_UPLOADS_OFF). Dosyalar taşınmaz ya da silinmez.

// Drop belge türleri (backend docuploads.Types ile aynı).
const (
	DropInvoice       = "invoice"
	DropZReport       = "z_report"
	DropExpense       = "expense"
	DropBankStatement = "bank_statement"
	DropOther         = "other"
)

// DropDocTypes seçilebilen türler.
var DropDocTypes = []string{DropInvoice, DropZReport, DropExpense, DropBankStatement, DropOther}

// MaxDropFiles bir bırakmada en çok dosya.
const MaxDropFiles = 200

// DropFolder, ajan belgesi ucuna giden dosyaların "folder" alanı (sunucuda
// klasör yolu yerine görünür).
const DropFolder = "Sürükle-bırak"

var periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// DropRequest kullanıcının seçimi.
type DropRequest struct {
	Paths          []string `json:"paths"`
	EngagementUUID string   `json:"engagement_uuid"`
	Period         string   `json:"period"`
	DocType        string   `json:"doc_type"`
}

// Validate seçimi denetler; hata metni kullanıcıya gösterilir.
func (r DropRequest) Validate() error {
	switch {
	case len(r.Paths) == 0:
		return errors.New("yüklenecek dosya yok")
	case strings.TrimSpace(r.EngagementUUID) == "":
		return errors.New("mükellef seçilmeli")
	case !periodRe.MatchString(r.Period):
		return errors.New("dönem (ay) seçilmeli")
	}
	for _, t := range DropDocTypes {
		if r.DocType == t {
			return nil
		}
	}
	return errors.New("belge türü seçilmeli")
}

// dropAccepted: bu uzantılar yüklenebilir (e-fatura, PDF, fotoğraf, Luca
// dökümü).
func dropAccepted(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".xml" || ext == ".zip" || document(name)
}

// spreadsheet: Luca bordro dökümü / mizan; türlü uç almaz, ajan belgesi
// ucuna gider.
func spreadsheet(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx", ".xls", ".csv":
		return true
	}
	return false
}

// ExpandDrop bırakılan yolları dosya listesine çevirir: dosya olduğu gibi,
// klasörün yalnızca kendi dosyaları (alt klasörlere inilmez). Gizli ve
// geçici dosyalar (`.`, `~$`) atlanır. Sıra kararlıdır.
func ExpandDrop(paths []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		base := filepath.Base(p)
		if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "~$") || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, raw := range paths {
		p := filepath.Clean(raw)
		info, err := os.Stat(p)
		if err != nil {
			return nil, errors.New("bırakılan dosya bulunamadı: " + filepath.Base(p))
		}
		if info.Mode().IsRegular() {
			add(p)
			continue
		}
		if !info.IsDir() {
			continue
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, errors.New("klasör okunamadı: " + filepath.Base(p))
		}
		var files []string
		for _, e := range entries {
			if e.Type().IsRegular() {
				files = append(files, filepath.Join(p, e.Name()))
			}
		}
		sort.Strings(files)
		for _, f := range files {
			add(f)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("bırakılan klasörde dosya yok")
	}
	if len(out) > MaxDropFiles {
		return nil, errors.New("bir seferde en çok 200 dosya yüklenebilir")
	}
	return out, nil
}

// TypedUpload, ofis evrak yükleme ucunun yanıtı (#827).
type TypedUpload struct {
	UUID             string `json:"uuid"`
	DocType          string `json:"doc_type"`
	Period           string `json:"period"`
	Status           string `json:"status"`
	CorrectionNeeded bool   `json:"correction_needed"`
	Error            string `json:"error"`
}

// UploadTyped dosyayı seçilen tür ve dönemle ofis evrak yükleme ucuna
// gönderir.
func (c *Client) UploadTyped(ctx context.Context, engagementUUID, docType, period, fileName string, data []byte) (TypedUpload, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range [][2]string{{"doc_type", docType}, {"period", period}} {
		if err := mw.WriteField(f[0], f[1]); err != nil {
			return TypedUpload{}, err
		}
	}
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		return TypedUpload{}, err
	}
	if _, err := fw.Write(data); err != nil {
		return TypedUpload{}, err
	}
	if err := mw.Close(); err != nil {
		return TypedUpload{}, err
	}
	var out TypedUpload
	path := "/v1/tenant/engagements/" + url.PathEscape(engagementUUID) + "/document-uploads"
	err = c.send(ctx, http.MethodPost, path, mw.FormDataContentType(), buf.Bytes(), &out, true)
	return out, err
}

// DropAPI, Client'ın sürükle-bırak için kullandığı uçlar (test edilebilir).
type DropAPI interface {
	UploadTyped(ctx context.Context, engagementUUID, docType, period, fileName string, data []byte) (TypedUpload, error)
	UploadDocument(ctx context.Context, agentUUID, folder, engagementUUID, fileName string, data []byte) (AgentDocument, error)
}

// Dropper bırakılan dosyaları sırayla yükler; her dosya için `upload`
// olayı yollar (geçmiş listesi ve tepsi aynı olayı okur).
type Dropper struct {
	API  DropAPI
	Emit func(UploadEvent)
	// ConvertHEIC HEIC fotoğrafını JPEG'e çevirir (nil: heicToJPEG).
	ConvertHEIC func(ctx context.Context, path string) ([]byte, error)
}

// Upload dosyaları yükler ve sonuçları döner. Bir dosyanın hatası
// diğerlerini durdurmaz.
func (d *Dropper) Upload(ctx context.Context, agentUUID string, req DropRequest, files []string) []UploadEvent {
	out := make([]UploadEvent, 0, len(files))
	for _, p := range files {
		ev := d.one(ctx, agentUUID, req, p)
		if d.Emit != nil {
			d.Emit(ev)
		}
		out = append(out, ev)
	}
	return out
}

func (d *Dropper) one(ctx context.Context, agentUUID string, req DropRequest, path string) UploadEvent {
	name := filepath.Base(path)
	ev := UploadEvent{Path: path, Name: name, EngagementUUID: req.EngagementUUID, Source: "drop"}
	reject := func(reason, detail string) UploadEvent {
		ev.Outcome, ev.Reason, ev.Error = "rejected", reason, detail
		return ev
	}
	if !dropAccepted(name) {
		return reject("Bu dosya türü yüklenemez. PDF, fotoğraf, e-fatura (XML / ZIP) ya da Luca dökümü bırakın.", "uzantı desteklenmiyor")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return reject("Dosya okunamadı; yüklenmedi.", "dosya yok")
	}
	if info.Size() == 0 {
		return reject("Dosya boş; yüklenmedi.", "boş dosya")
	}
	if info.Size() > MaxDocumentBytes {
		return reject("Dosya 20 MB sınırını aşıyor; yüklenmedi.", "dosya 20 MB sınırını aşıyor")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return reject("Dosya okunamadı; yüklenmedi.", err.Error())
	}
	if heic(name) {
		conv := d.ConvertHEIC
		if conv == nil {
			conv = heicToJPEG
		}
		jpg, cerr := conv(ctx, path)
		if cerr != nil || len(jpg) == 0 {
			return reject("HEIC fotoğrafı bu bilgisayarda JPEG'e çevrilemedi; fotoğrafı JPEG olarak kaydedip yeniden bırakın.", "heic çevrilemedi")
		}
		data, name = jpg, strings.TrimSuffix(name, filepath.Ext(name))+".jpg"
	}

	viaDocument := spreadsheet(name)
	if !viaDocument {
		up, err := d.API.UploadTyped(ctx, req.EngagementUUID, req.DocType, req.Period, name, data)
		var ae *APIError
		switch {
		case err == nil:
			ev.Outcome, ev.JobUUID, ev.Reason = "uploaded", up.UUID, typedReason(up)
			return ev
		case errors.As(err, &ae) && ae.Code == "DOCUMENT_UPLOADS_OFF":
			viaDocument = true
		case errors.As(err, &ae) && ae.Status >= 400 && ae.Status < 500:
			return reject(typedRejectReason(ae), ae.Error())
		default:
			ev.Outcome, ev.Error = "failed", err.Error()
			ev.Reason = "Sunucuya gönderilemedi; dosyayı yeniden bırakın."
			return ev
		}
	}
	if agentUUID == "" {
		return reject("Bu bilgisayar ofise kayıtlı değil; uygulamayı yeniden bağlayın.", "ajan kaydı yok")
	}
	doc, err := d.API.UploadDocument(ctx, agentUUID, DropFolder, req.EngagementUUID, name, data)
	var ae *APIError
	switch {
	case err == nil:
		ev.Outcome, ev.JobUUID, ev.Reason = "uploaded", doc.UUID, documentReason(doc)
	case errors.As(err, &ae) && permanent(ae.Status):
		return reject(documentRejectReason(ae.Status), ae.Error())
	default:
		ev.Outcome, ev.Error = "failed", err.Error()
		ev.Reason = "Sunucuya gönderilemedi; dosyayı yeniden bırakın."
	}
	return ev
}

// typedReason başarılı türlü yüklemenin kullanıcıya gösterilecek sonucu.
func typedReason(u TypedUpload) string {
	switch {
	case u.Status == "rejected":
		return "Ay kilitli olduğu için kayda alınmadı."
	case u.CorrectionNeeded:
		return "Kilitli aya düzeltme olarak alındı."
	case u.Period != "":
		return "Yüklendi (" + u.Period + ")."
	}
	return "Yüklendi."
}

// typedRejectReason türlü yükleme ucunun kalıcı hatası.
func typedRejectReason(ae *APIError) string {
	switch ae.Code {
	case "PERIOD_LOCKED", "TARGET_PERIOD_LOCKED":
		return "Seçilen ay kilitli. Belgeyi ofis panelinden yükleyip kilitli ay seçeneklerinden birini seçin."
	case "IMAGE_REQUIRED":
		return "Z raporu ve masraf fotoğraftan okunur; JPEG ya da PNG bırakın."
	case "UNSUPPORTED_FILE":
		return "Bu türde PDF, JPEG ya da PNG yüklenebilir (e-fatura: XML ya da ZIP)."
	case "DOCUMENT_EXISTS":
		return "Aynı dosya daha önce yüklenmiş."
	case "NO_OWN_TAX_ID":
		return "Mükellefin VKN / TCKN bilgisi kayıtlı değil."
	}
	switch ae.Status {
	case http.StatusForbidden:
		return "Bu mükellefe belge yükleme yetkiniz yok."
	case http.StatusNotFound:
		return "Mükellef ilişkisi bulunamadı ya da sona ermiş."
	case http.StatusRequestEntityTooLarge:
		return "Dosya çok büyük; yüklenmedi."
	}
	return "Dosya yüklenemedi; türü ve dönemi denetleyin."
}
