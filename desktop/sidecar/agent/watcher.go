package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// MaxFileBytes, backend'in UBL yükleme sınırı (invoices/upload
// MaxUploadBytes, 200 MB).
const MaxFileBytes = 200 << 20

// settleFor: dosya bu süre değişmeden kalınca indirme bitmiş sayılır.
const settleFor = 3 * time.Second

// fileMark, bir önceki taramada görülen boyut ve değişiklik zamanı.
type fileMark struct {
	size  int64
	mtime time.Time
}

// UploadEvent bir dosyanın sonucu (kabuğa olay olarak gider).
type UploadEvent struct {
	Path           string `json:"path"`
	Name           string `json:"name"`
	EngagementUUID string `json:"engagement_uuid"`
	Outcome        string `json:"outcome"` // uploaded | duplicate | rejected | failed
	JobUUID        string `json:"job_uuid,omitempty"`
	Error          string `json:"error,omitempty"`
	// Reason kullanıcıya gösterilecek Türkçe açıklama (#206); Error ham
	// ayrıntıdır ve yalnızca günlükte / Gelişmiş bölümünde kullanılır.
	Reason string `json:"reason,omitempty"`
}

// Uploader, Client.UploadUBL ve Client.UploadDocument'in test edilebilir
// hâli.
type Uploader interface {
	UploadUBL(ctx context.Context, engagementUUID, fileName string, data []byte) (UploadJob, error)
	UploadDocument(ctx context.Context, agentUUID, folder, engagementUUID, fileName string, data []byte) (AgentDocument, error)
}

// MaxDocumentBytes, ajan belgesi ucunun sınırı (Luca dökümü / PDF, 20 MB).
const MaxDocumentBytes = 20 << 20

// Watcher izlenen klasörlerdeki yeni .xml ve .zip dosyalarını (e-fatura),
// Luca dökümlerini (.xlsx, .xls, .csv) ve yabancı fatura PDF'lerini
// (#780 e) yükler.
//
// Yoklama (polling) bilinçli seçildi: ağ sürücüsü ve eşitlenen klasörlerde
// (OneDrive vb.) dosya sistemi bildirimleri güvenilir değil. Dosyalar
// taşınmaz ya da silinmez; aynı içerik (SHA-256) bir kez yüklenir.
type Watcher struct {
	State    *State
	Uploader Uploader
	Emit     func(UploadEvent)
	// ConvertHEIC HEIC fotoğrafını JPEG'e çevirir (nil: heicToJPEG).
	ConvertHEIC func(ctx context.Context, path string) ([]byte, error)

	mu    sync.Mutex
	marks map[string]fileMark
	// known: bu boyut ve zamanla zaten işlenmiş dosyalar; yeniden okunmaz.
	known map[string]fileMark
	// Geçici hatada (ağ, 5xx) dosya bir sonraki taramada yeniden denenir.
	stats WatchStats
	// folders klasör başına son etkinlik (#206); bellekte tutulur.
	folders map[string]*FolderActivity
}

// maxFolderErrors klasör başına tutulan en fazla hata.
const maxFolderErrors = 20

// FileError bir dosyanın kullanıcıya gösterilecek hatası.
type FileError struct {
	Name    string    `json:"name"`
	Message string    `json:"message"`
	Retry   bool      `json:"retry"` // geçici hata: sonraki taramada yeniden denenir
	At      time.Time `json:"at"`
}

// FolderActivity bir klasörün son yükleme durumu ve hata listesi (#206).
type FolderActivity struct {
	LastFile     string      `json:"last_file,omitempty"`
	LastOutcome  string      `json:"last_outcome,omitempty"` // uploaded | rejected | failed
	LastAt       *time.Time  `json:"last_at,omitempty"`
	LastUploadAt *time.Time  `json:"last_upload_at,omitempty"`
	Uploaded     int         `json:"uploaded"`
	FolderError  string      `json:"folder_error,omitempty"`
	Errors       []FileError `json:"errors"`
}

// Activity klasörün etkinliğinin kopyası (yoksa boş).
func (w *Watcher) Activity(path string) FolderActivity {
	w.mu.Lock()
	defer w.mu.Unlock()
	a, ok := w.folders[path]
	if !ok {
		return FolderActivity{Errors: []FileError{}}
	}
	out := *a
	out.Errors = append([]FileError{}, a.Errors...)
	return out
}

// ClearErrors klasörün hata listesini temizler.
func (w *Watcher) ClearErrors(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if a, ok := w.folders[path]; ok {
		a.Errors = nil
	}
}

func (w *Watcher) folderLocked(path string) *FolderActivity {
	if w.folders == nil {
		w.folders = map[string]*FolderActivity{}
	}
	a, ok := w.folders[path]
	if !ok {
		a = &FolderActivity{}
		w.folders[path] = a
	}
	return a
}

// note bir dosya sonucunu klasörün etkinliğine işler.
func (w *Watcher) note(folder string, ev UploadEvent, now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	a := w.folderLocked(folder)
	t := now
	a.LastFile, a.LastOutcome, a.LastAt = ev.Name, ev.Outcome, &t
	if ev.Outcome == "uploaded" {
		a.Uploaded++
		a.LastUploadAt = &t
		return
	}
	// Aynı dosyanın önceki hatası güncellenir; liste en yeni başta.
	errs := a.Errors[:0]
	for _, e := range a.Errors {
		if e.Name != ev.Name {
			errs = append(errs, e)
		}
	}
	fe := FileError{Name: ev.Name, Message: ev.Reason, Retry: ev.Outcome == "failed", At: t}
	a.Errors = append([]FileError{fe}, errs...)
	if len(a.Errors) > maxFolderErrors {
		a.Errors = a.Errors[:maxFolderErrors]
	}
}

func (w *Watcher) setFolderError(folder, msg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.folderLocked(folder).FolderError = msg
}

// WatchStats heartbeat'e eklenen sayaçlar.
type WatchStats struct {
	Folders   int       `json:"watched_folders"`
	Uploaded  int       `json:"uploaded"`
	Failed    int       `json:"failed"`
	LastScan  time.Time `json:"last_scan,omitzero"` // tarama yapılmadıysa gönderilmez (#284)
	LastError string    `json:"last_error,omitempty"`
}

func (w *Watcher) Stats() WatchStats {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stats
}

func candidate(name string) bool {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~$") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".xml" || ext == ".zip" || document(name)
}

// document: Luca bordro dökümü / mizan Excel'i, PDF ya da fiş / fatura
// fotoğrafı; ajan belgesi ucuna gider, sunucu içeriğinden tanır. Fotoğraf
// mobil ve WhatsApp ile aynı fiş okumasına gider.
func document(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx", ".xls", ".csv", ".pdf":
		return true
	}
	return photo(name)
}

// photo: fiş / fatura / Z raporu fotoğrafı ya da taranmış görüntü.
func photo(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return true
	}
	return heic(name)
}

// heic: iPhone fotoğrafı. Fiş okuma JPEG / PNG ister; macOS'ta yerleşik
// sips ile JPEG'e çevrilir, başka sistemde atlanır.
func heic(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".heic", ".heif":
		return true
	}
	return false
}

// heicToJPEG HEIC dosyasını macOS'un sips aracıyla JPEG'e çevirir.
func heicToJPEG(ctx context.Context, path string) ([]byte, error) {
	if runtime.GOOS != "darwin" {
		return nil, errHEICUnsupported
	}
	dir, err := os.MkdirTemp("", "hakan-heic-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	out := filepath.Join(dir, "fis.jpg")
	cmd := exec.CommandContext(ctx, "/usr/bin/sips", "-s", "format", "jpeg", path, "--out", out)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(out)
}

var errHEICUnsupported = errors.New("HEIC bu işletim sisteminde çevrilemiyor")

// Scan her izlenen klasörü bir kez tarar. now enjekte edilebilir (test).
func (w *Watcher) Scan(ctx context.Context, now time.Time) {
	snap := w.State.Snapshot()
	w.mu.Lock()
	if w.marks == nil {
		w.marks = map[string]fileMark{}
		w.known = map[string]fileMark{}
	}
	w.stats.Folders = len(snap.Watches)
	w.stats.LastScan = now
	w.mu.Unlock()

	for _, wt := range snap.Watches {
		entries, err := os.ReadDir(wt.Path)
		if err != nil {
			w.setError("klasör okunamadı: " + err.Error())
			w.setFolderError(wt.Path, folderReason(err))
			continue
		}
		w.setFolderError(wt.Path, "")
		for _, e := range entries {
			if ctx.Err() != nil {
				return
			}
			if e.IsDir() {
				w.scanSub(ctx, now, wt, filepath.Join(wt.Path, e.Name()), e.Name())
				continue
			}
			if !candidate(e.Name()) {
				continue
			}
			w.consider(ctx, now, wt, wt.Path, filepath.Join(wt.Path, e.Name()))
		}
	}
}

// scanSub izlenen klasörün bir alt klasörünü tarar (tek düzey). Alt klasör
// adı mükellefin unvanı ya da VKN'si olabilir ("Fişler/Örnek Gıda",
// "Fişler/1234567890"); sunucu mükellefi bu addan bulur, bulamazsa dosya
// ofiste "eşleştir" listesinde bekler. Yalnızca belge ucuna giden dosyalar
// (fotoğraf, PDF, Luca dökümü) alınır: e-fatura XML'i izlenen klasörün
// mükellefine gider, alt klasörde aranmaz.
func (w *Watcher) scanSub(ctx context.Context, now time.Time, wt Watch, dir, name string) {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~$") {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if ctx.Err() != nil {
			return
		}
		if e.IsDir() || !candidate(e.Name()) || !document(e.Name()) {
			continue
		}
		w.consider(ctx, now, wt, dir, filepath.Join(dir, e.Name()))
	}
}

func (w *Watcher) setError(msg string) {
	w.mu.Lock()
	w.stats.LastError = msg
	w.mu.Unlock()
}

func (w *Watcher) consider(ctx context.Context, now time.Time, wt Watch, folder, path string) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	cur := fileMark{size: info.Size(), mtime: info.ModTime()}
	w.mu.Lock()
	if k, ok := w.known[path]; ok && k == cur {
		w.mu.Unlock()
		return
	}
	prev, seen := w.marks[path]
	w.marks[path] = cur
	w.mu.Unlock()
	// İndirme sürüyor olabilir: iki taramada aynı ve yeterince eski olmalı.
	if !seen || prev != cur || now.Sub(cur.mtime) < settleFor {
		return
	}
	name := filepath.Base(path)
	ev := UploadEvent{Path: path, Name: name, EngagementUUID: wt.EngagementUUID}
	defer func() {
		if ev.Outcome != "" {
			w.note(wt.Path, ev, now)
		}
	}()
	if cur.size == 0 {
		return
	}
	if cur.size > MaxFileBytes {
		w.markKnown(path, cur)
		ev.Reason = "Dosya 200 MB sınırını aşıyor; yüklenmedi."
		ev = w.finish(ev, "", "rejected", "dosya 200 MB sınırını aşıyor", "")
		return
	}
	isDoc := document(name)
	if isDoc && cur.size > MaxDocumentBytes {
		w.markKnown(path, cur)
		ev.Reason = "Dosya 20 MB sınırını aşıyor; yüklenmedi."
		ev = w.finish(ev, "", "rejected", "dosya 20 MB sınırını aşıyor", "")
		return
	}
	agentUUID := ""
	if isDoc {
		// Belge ucu ajana bağlı: kayıt olmadan yüklenmez, sonra denenir.
		if agentUUID = w.State.Snapshot().AgentUUID; agentUUID == "" {
			w.mu.Lock()
			delete(w.marks, path)
			w.mu.Unlock()
			return
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	sha := sha256Sum(data)
	if _, done := w.State.Seen(sha); done {
		w.markKnown(path, cur)
		return
	}
	if heic(name) {
		// Fiş okuma JPEG / PNG ister: HEIC önce JPEG'e çevrilir. Aynı
		// içeriğin özeti (sha) asıl dosyanınki kalır.
		conv := w.ConvertHEIC
		if conv == nil {
			conv = heicToJPEG
		}
		jpg, cerr := conv(ctx, path)
		if cerr != nil || len(jpg) == 0 {
			w.markKnown(path, cur)
			ev.Reason = "HEIC fotoğrafı bu bilgisayarda JPEG'e çevrilemedi; fotoğrafı JPEG olarak kaydedip klasöre yeniden bırakın."
			msg := "heic çevrilemedi"
			if cerr != nil {
				msg += ": " + cerr.Error()
			}
			ev = w.finish(ev, sha, "rejected", msg, "")
			return
		}
		data = jpg
		name = strings.TrimSuffix(name, filepath.Ext(name)) + ".jpg"
	}
	var jobUUID string
	if isDoc {
		var doc AgentDocument
		doc, err = w.Uploader.UploadDocument(ctx, agentUUID, folder, wt.EngagementUUID, name, data)
		jobUUID, ev.Reason = doc.UUID, documentReason(doc)
	} else {
		var job UploadJob
		job, err = w.Uploader.UploadUBL(ctx, wt.EngagementUUID, name, data)
		jobUUID = job.UUID
	}
	var ae *APIError
	switch {
	case err == nil:
		w.markKnown(path, cur)
		ev = w.finish(ev, sha, "uploaded", "", jobUUID)
	case errors.As(err, &ae) && permanent(ae.Status):
		// Biçim / boyut / yetki hatası: yeniden denemek sonuç değiştirmez.
		w.markKnown(path, cur)
		ev.Reason = rejectReason(ae.Status)
		if isDoc {
			ev.Reason = documentRejectReason(ae.Status)
		}
		ev = w.finish(ev, sha, "rejected", ae.Error(), "")
	default:
		w.mu.Lock()
		w.stats.Failed++
		w.stats.LastError = err.Error()
		// Bir sonraki taramada yeniden denensin.
		delete(w.marks, path)
		w.mu.Unlock()
		ev.Outcome, ev.Error = "failed", err.Error()
		ev.Reason = "Sunucuya gönderilemedi; bir sonraki taramada yeniden denenecek."
		w.emit(ev)
	}
}

func (w *Watcher) markKnown(path string, m fileMark) {
	w.mu.Lock()
	w.known[path] = m
	w.mu.Unlock()
}

func permanent(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound,
		http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		return true
	}
	return false
}

func (w *Watcher) finish(ev UploadEvent, sha, outcome, errMsg, job string) UploadEvent {
	if sha != "" {
		rec := Uploaded{Name: ev.Name, EngagementUUID: ev.EngagementUUID, JobUUID: job, At: time.Now().UTC()}
		if outcome == "rejected" {
			rec.Rejected = errMsg
		}
		if err := w.State.Record(sha, rec); err != nil {
			w.setError("durum yazılamadı: " + err.Error())
		}
	}
	w.mu.Lock()
	if outcome == "uploaded" {
		w.stats.Uploaded++
	}
	w.mu.Unlock()
	ev.Outcome, ev.Error, ev.JobUUID = outcome, errMsg, job
	w.emit(ev)
	return ev
}

// rejectReason kalıcı yükleme hatasının kullanıcıya gösterilecek metni.
func rejectReason(status int) string {
	switch status {
	case http.StatusUnsupportedMediaType:
		return "Dosya e-fatura (UBL-TR) değil; yüklenmedi."
	case http.StatusRequestEntityTooLarge:
		return "Dosya çok büyük; yüklenmedi."
	case http.StatusForbidden:
		return "Bu mükellefe yükleme yetkiniz yok."
	case http.StatusNotFound:
		return "Mükellef ilişkisi bulunamadı ya da sona ermiş."
	default:
		return "Dosya okunamadı ya da geçerli bir e-fatura değil; yüklenmedi."
	}
}

// documentReason yüklenen belgenin sunucudaki sonucu (#780 e).
func documentReason(d AgentDocument) string {
	switch d.Status {
	case "imported":
		if d.Kind == "receipt" {
			return "Fiş okundu; mükellefin gider / gelir listesine düştü."
		}
		return "İçe aktarıldı."
	case "needs_match", "failed":
		if d.Reason != "" {
			return "Ofiste eşleştirme bekliyor: " + d.Reason
		}
		return "Ofiste eşleştirme bekliyor."
	}
	return ""
}

// documentRejectReason belge ucunun kalıcı hatası.
func documentRejectReason(status int) string {
	switch status {
	case http.StatusRequestEntityTooLarge:
		return "Dosya çok büyük; yüklenmedi."
	case http.StatusForbidden:
		return "Bu ofise belge yükleme yetkiniz yok."
	case http.StatusNotFound:
		return "Ajan kaydı bulunamadı; uygulamayı yeniden bağlayın."
	default:
		return "Dosya okunamadı; yüklenmedi."
	}
}

// folderReason klasör okuma hatasının kullanıcıya gösterilecek metni.
func folderReason(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "Klasör bulunamadı. Taşınmış ya da silinmiş olabilir."
	case errors.Is(err, os.ErrPermission):
		return "Klasöre erişim izni yok."
	default:
		return "Klasör okunamadı."
	}
}

func (w *Watcher) emit(ev UploadEvent) {
	if w.Emit != nil {
		w.Emit(ev)
	}
}

func sha256Sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
