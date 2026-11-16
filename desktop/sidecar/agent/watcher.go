package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
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
}

// Uploader, Client.UploadUBL'in test edilebilir hâli.
type Uploader interface {
	UploadUBL(ctx context.Context, engagementUUID, fileName string, data []byte) (UploadJob, error)
}

// Watcher izlenen klasörlerdeki yeni .xml ve .zip dosyalarını yükler.
//
// Yoklama (polling) bilinçli seçildi: ağ sürücüsü ve eşitlenen klasörlerde
// (OneDrive vb.) dosya sistemi bildirimleri güvenilir değil. Dosyalar
// taşınmaz ya da silinmez; aynı içerik (SHA-256) bir kez yüklenir.
type Watcher struct {
	State    *State
	Uploader Uploader
	Emit     func(UploadEvent)

	mu    sync.Mutex
	marks map[string]fileMark
	// known: bu boyut ve zamanla zaten işlenmiş dosyalar; yeniden okunmaz.
	known map[string]fileMark
	// Geçici hatada (ağ, 5xx) dosya bir sonraki taramada yeniden denenir.
	stats WatchStats
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
	return ext == ".xml" || ext == ".zip"
}

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
			continue
		}
		for _, e := range entries {
			if ctx.Err() != nil {
				return
			}
			if e.IsDir() || !candidate(e.Name()) {
				continue
			}
			w.consider(ctx, now, wt, filepath.Join(wt.Path, e.Name()))
		}
	}
}

func (w *Watcher) setError(msg string) {
	w.mu.Lock()
	w.stats.LastError = msg
	w.mu.Unlock()
}

func (w *Watcher) consider(ctx context.Context, now time.Time, wt Watch, path string) {
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
	if cur.size == 0 {
		return
	}
	if cur.size > MaxFileBytes {
		w.markKnown(path, cur)
		w.finish(ev, "", "rejected", "dosya 200 MB sınırını aşıyor", "")
		return
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
	job, err := w.Uploader.UploadUBL(ctx, wt.EngagementUUID, name, data)
	var ae *APIError
	switch {
	case err == nil:
		w.markKnown(path, cur)
		w.finish(ev, sha, "uploaded", "", job.UUID)
	case errors.As(err, &ae) && permanent(ae.Status):
		// Biçim / boyut / yetki hatası: yeniden denemek sonuç değiştirmez.
		w.markKnown(path, cur)
		w.finish(ev, sha, "rejected", ae.Error(), "")
	default:
		w.mu.Lock()
		w.stats.Failed++
		w.stats.LastError = err.Error()
		// Bir sonraki taramada yeniden denensin.
		delete(w.marks, path)
		w.mu.Unlock()
		ev.Outcome, ev.Error = "failed", err.Error()
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

func (w *Watcher) finish(ev UploadEvent, sha, outcome, errMsg, job string) {
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
