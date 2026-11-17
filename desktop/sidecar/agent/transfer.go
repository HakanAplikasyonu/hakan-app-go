package agent

// Muhasebe programına aktarım (issue #391,
// docs/arastirma/luca-zirve-aktarim.md §4): ofis bir Luca / Zirve çıktısını
// "masaüstü ajanına gönder" deyince ajan, o mükellef ve program için seçilmiş
// aktarım klasörüne dosyaları bırakır ve sonucu sunucuya bildirir.
// Programın içinde içe aktarmayı kullanıcı yapar. Ajan programın
// veritabanına ya da web sitesine dokunmaz, Luca / Zirve parolası istemez.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TransferInterval bekleyen aktarımların yoklanma sıklığı.
const TransferInterval = time.Minute

// TransferDrivers ajanın klasöre bırakabildiği programlar.
var TransferDrivers = map[string]string{"luca": "Luca", "zirve": "Zirve"}

// TransferTarget bir mükellef + program için aktarım klasörü.
type TransferTarget struct {
	EngagementUUID string `json:"engagement_uuid"`
	Driver         string `json:"driver"` // luca | zirve
	Path           string `json:"path"`
	TaxpayerTitle  string `json:"taxpayer_title,omitempty"`
}

// RemoteFile çıktıdaki bir dosya (sunucunun bildirdiği).
type RemoteFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// RemoteTransfer sunucudaki aktarım görevi.
type RemoteTransfer struct {
	UUID      string       `json:"uuid"`
	BatchUUID string       `json:"batch_uuid"`
	Period    string       `json:"period"`
	Driver    string       `json:"driver"`
	Status    string       `json:"status"`
	Files     []RemoteFile `json:"files"`
}

// DeliveryReport ajanın sonucu (POST …/delivery).
type DeliveryReport struct {
	AgentUUID    string       `json:"agent_uuid"`
	Outcome      string       `json:"outcome"` // delivered | failed
	Files        []RemoteFile `json:"files,omitempty"`
	FolderLabel  string       `json:"folder_label,omitempty"`
	ErrorCode    string       `json:"error_code,omitempty"`
	ErrorMessage string       `json:"error_message,omitempty"`
}

// TransferAPI Client'ın test edilebilir alt kümesi.
type TransferAPI interface {
	ListTransfers(ctx context.Context, engagementUUID, status string) ([]RemoteTransfer, error)
	ClaimTransfer(ctx context.Context, engagementUUID, transferUUID, agentUUID string) (RemoteTransfer, error)
	ReportDelivery(ctx context.Context, engagementUUID, transferUUID string, rep DeliveryReport) error
	DownloadExportFile(ctx context.Context, engagementUUID, batchUUID, name string) ([]byte, error)
}

// TransferEvent bir aktarımın ajan tarafındaki sonucu (arayüz geçmişi).
type TransferEvent struct {
	TransferUUID   string    `json:"transfer_uuid"`
	EngagementUUID string    `json:"engagement_uuid"`
	TaxpayerTitle  string    `json:"taxpayer_title,omitempty"`
	Driver         string    `json:"driver"`
	Period         string    `json:"period,omitempty"`
	Outcome        string    `json:"outcome"` // delivered | failed | retry
	Files          []string  `json:"files,omitempty"`
	Folder         string    `json:"folder"`
	Reason         string    `json:"reason,omitempty"`
	At             time.Time `json:"at"`
}

// maxTransferEvents bellekte tutulan son aktarım sonucu sayısı.
const maxTransferEvents = 20

// Transferrer bekleyen aktarımları alır ve klasörlere yazar.
type Transferrer struct {
	State *State
	API   TransferAPI
	Emit  func(TransferEvent)
	Now   func() time.Time

	mu     sync.Mutex
	recent []TransferEvent
	// lastError yoklamanın kendisi başarısız olduysa (ağ, oturum).
	lastError string
	lastRun   *time.Time
}

// TransferActivity arayüz için özet.
type TransferActivity struct {
	Recent    []TransferEvent `json:"recent"`
	LastRun   *time.Time      `json:"last_run,omitempty"`
	LastError string          `json:"last_error,omitempty"`
}

// Activity son sonuçların kopyası.
func (t *Transferrer) Activity() TransferActivity {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := TransferActivity{Recent: append([]TransferEvent{}, t.recent...), LastError: t.lastError}
	if t.lastRun != nil {
		v := *t.lastRun
		out.LastRun = &v
	}
	return out
}

func (t *Transferrer) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now().UTC()
}

// Run her aktarım klasörünün mükellefi için bekleyen görevleri işler.
func (t *Transferrer) Run(ctx context.Context) {
	snap := t.State.Snapshot()
	if snap.AgentUUID == "" || len(snap.Targets) == 0 {
		return
	}
	byEng := map[string][]TransferTarget{}
	order := []string{}
	for _, tg := range snap.Targets {
		if _, ok := byEng[tg.EngagementUUID]; !ok {
			order = append(order, tg.EngagementUUID)
		}
		byEng[tg.EngagementUUID] = append(byEng[tg.EngagementUUID], tg)
	}
	var runErr error
	for _, eng := range order {
		for _, status := range []string{"queued", "claimed"} {
			items, err := t.API.ListTransfers(ctx, eng, status)
			if err != nil {
				runErr = err
				continue
			}
			for _, it := range items {
				tg, ok := targetFor(byEng[eng], it.Driver)
				if !ok {
					continue // bu program için klasör yok: başka ajan alsın
				}
				t.handle(ctx, snap.AgentUUID, tg, it)
			}
		}
	}
	now := t.now()
	t.mu.Lock()
	t.lastRun = &now
	t.lastError = ""
	if runErr != nil {
		t.lastError = transferReason(runErr)
	}
	t.mu.Unlock()
}

func targetFor(ts []TransferTarget, driver string) (TransferTarget, bool) {
	for _, tg := range ts {
		if tg.Driver == driver {
			return tg, true
		}
	}
	return TransferTarget{}, false
}

// handle bir görevi üstlenir, dosyaları yazar ve sonucu bildirir. Ağ hatası
// sonucu bildirilmez: görev üstlenilmiş kalır, sonraki turda yeniden denenir.
func (t *Transferrer) handle(ctx context.Context, agentUUID string, tg TransferTarget, it RemoteTransfer) {
	claimed, err := t.API.ClaimTransfer(ctx, tg.EngagementUUID, it.UUID, agentUUID)
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.Status == 409 {
			return // başka ajan aldı ya da kapandı
		}
		t.note(tg, it, "retry", nil, transferReason(err))
		return
	}
	if len(claimed.Files) > 0 {
		it = claimed
	}
	written, code, reason, err := t.deliver(ctx, tg, it)
	if err != nil && code == "" {
		// Geçici (indirme / ağ): görev üstlenilmiş kalır.
		t.note(tg, it, "retry", nil, reason)
		return
	}
	rep := DeliveryReport{AgentUUID: agentUUID, FolderLabel: folderLabel(tg.Path)}
	if code != "" {
		rep.Outcome, rep.ErrorCode, rep.ErrorMessage = "failed", code, reason
	} else {
		rep.Outcome, rep.Files = "delivered", written
	}
	if err := t.API.ReportDelivery(ctx, tg.EngagementUUID, it.UUID, rep); err != nil {
		t.note(tg, it, "retry", nil, transferReason(err))
		return
	}
	names := make([]string, 0, len(written))
	for _, f := range written {
		names = append(names, f.Name)
	}
	t.note(tg, it, rep.Outcome, names, reason)
}

// deliver dosyaları indirip klasöre yazar. code doluysa kalıcı hata
// (sunucuya failed bildirilir); code boş ve err doluysa geçici hata.
func (t *Transferrer) deliver(ctx context.Context, tg TransferTarget, it RemoteTransfer) ([]RemoteFile, string, string, error) {
	info, err := os.Stat(tg.Path)
	if err != nil || !info.IsDir() {
		return nil, "FOLDER_MISSING", "Aktarım klasörü bulunamadı. Klasör taşınmış ya da ağ sürücüsü bağlı değil.", errors.New("folder missing")
	}
	if len(it.Files) == 0 {
		return nil, "NO_FILES", "Çıktıda aktarılacak dosya yok.", errors.New("no files")
	}
	out := make([]RemoteFile, 0, len(it.Files))
	for _, f := range it.Files {
		data, err := t.API.DownloadExportFile(ctx, tg.EngagementUUID, it.BatchUUID, f.Name)
		if err != nil {
			var ae *APIError
			if errors.As(err, &ae) && ae.Status == 404 {
				return nil, "FILE_MISSING", "Çıktı dosyası sunucuda bulunamadı.", err
			}
			return nil, "", transferReason(err), err
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if f.SHA256 != "" && !strings.EqualFold(got, f.SHA256) {
			return nil, "CHECKSUM_MISMATCH", "İndirilen dosya çıktıdaki dosyayla aynı değil (özet tutmadı).", errors.New("checksum")
		}
		name := TransferFileName(f.Name, it.BatchUUID)
		if code, reason, err := writeInto(tg.Path, name, data, got); err != nil {
			return nil, code, reason, err
		}
		out = append(out, RemoteFile{Name: name, SHA256: got, Size: int64(len(data))})
	}
	return out, "", "", nil
}

// TransferFileName klasöre yazılan ad: çıktı adı + çıktının kısa kimliği.
// Aynı dönemin iki çıktısı (ör. geri alma sonrası) birbirinin üstüne
// yazılmaz; aynı çıktının yeniden teslimi aynı adı kullanır.
func TransferFileName(name, batchUUID string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = "cikti.xlsx"
	}
	short := strings.ReplaceAll(batchUUID, "-", "")
	if len(short) > 8 {
		short = short[:8]
	}
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext) + "_" + short + ext
}

// writeInto dosyayı geçici adla yazıp yeniden adlandırır. Aynı adla aynı
// içerik varsa yazılmış sayılır; farklı içerik varsa üstüne yazılmaz.
func writeInto(dir, name string, data []byte, sum string) (string, string, error) {
	dst := filepath.Join(dir, name)
	if old, err := os.ReadFile(dst); err == nil { //nolint:gosec // klasör kullanıcının seçtiği aktarım klasörü
		s := sha256.Sum256(old)
		if hex.EncodeToString(s[:]) == sum {
			return "", "", nil
		}
		return "FILE_EXISTS", fmt.Sprintf("Klasörde aynı adla farklı bir dosya var: %s. Dosyayı taşıyıp yeniden deneyin.", name), errors.New("exists")
	}
	tmp := filepath.Join(dir, "."+name+".part")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return writeFailure(err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return writeFailure(err)
	}
	return "", "", nil
}

func writeFailure(err error) (string, string, error) {
	if errors.Is(err, fs.ErrPermission) {
		return "WRITE_DENIED", "Aktarım klasörüne yazma izni yok.", err
	}
	return "WRITE_FAILED", "Dosya aktarım klasörüne yazılamadı (disk dolu ya da klasör kilitli olabilir).", err
}

// folderLabel kullanıcıya gösterilen klasör adı: tam yol sunucuya gitmez.
func folderLabel(path string) string {
	b := filepath.Base(path)
	if len([]rune(b)) > 200 {
		b = string([]rune(b)[:200])
	}
	return b
}

func transferReason(err error) string {
	var ae *APIError
	switch {
	case errors.Is(err, ErrSignedOut):
		return "Oturum kapandı; yeniden oturum açın."
	case errors.As(err, &ae) && ae.Status == 403:
		return "Bu mükellef için aktarım yetkiniz yok."
	case errors.As(err, &ae) && ae.Status >= 500:
		return "Sunucu şu an yanıt vermiyor; bir dakika sonra yeniden denenecek."
	case errors.As(err, &ae):
		return "Sunucu isteği kabul etmedi; bir dakika sonra yeniden denenecek."
	}
	return "Sunucuya ulaşılamadı; bir dakika sonra yeniden denenecek."
}

func (t *Transferrer) note(tg TransferTarget, it RemoteTransfer, outcome string, files []string, reason string) {
	ev := TransferEvent{
		TransferUUID: it.UUID, EngagementUUID: tg.EngagementUUID, TaxpayerTitle: tg.TaxpayerTitle,
		Driver: it.Driver, Period: it.Period, Outcome: outcome, Files: files, Folder: folderLabel(tg.Path),
		Reason: reason, At: t.now(),
	}
	t.mu.Lock()
	if len(t.recent) > 0 && t.recent[0].TransferUUID == ev.TransferUUID && t.recent[0].Outcome == "retry" {
		t.recent = t.recent[1:] // aynı görevin yinelenen geçici hatası tek satır
	}
	t.recent = append([]TransferEvent{ev}, t.recent...)
	if len(t.recent) > maxTransferEvents {
		t.recent = t.recent[:maxTransferEvents]
	}
	t.mu.Unlock()
	if t.Emit != nil {
		t.Emit(ev)
	}
}
