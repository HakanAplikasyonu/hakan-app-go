package helper

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LocalState yardımcının yerel kaydı (helper-state.json). Sır içermez.
//   - Uploaded: aynı dosyanın (sha256) aynı firmaya ikinci yüklemesini
//     engeller (tasarım §2.5 madde 3).
//   - Armed: "Yükle" izlenirken uygulama kapanırsa görev yeniden açılışta
//     needs_check olarak bildirilir; asla yeniden yüklenmez.
//   - AlwaysOn: "bu bilgisayar aktarıcı" ayarı (sır değil).
//   - Outbox: sunucuya gönderilemeyen sonuçlar; sırayla, sonraki turda
//     yeniden gönderilir (dosya sonuçları görev sonucundan önce).
type LocalState struct {
	path string
	mu   sync.Mutex
	data localData
}

// ArmedFile "Yükle"ye basılmış olabilecek dosya.
type ArmedFile struct {
	TaskUUID string    `json:"task_uuid"`
	File     string    `json:"file"`
	SHA256   string    `json:"sha256,omitempty"`
	At       time.Time `json:"at"`
}

type localData struct {
	// AlwaysOn bu bilgisayar aktarıcı: uygulama açılınca (pencere
	// gizliyken de) yardımcı görev yoklamasını kendisi başlatır.
	AlwaysOn bool                 `json:"always_on,omitempty"`
	Uploaded map[string]time.Time `json:"uploaded"`
	Armed    map[string]ArmedFile `json:"armed"`
	Outbox   []OutboxCall         `json:"outbox,omitempty"`
}

// OutboxCall gönderilemeyen bir sonuç isteği.
type OutboxCall struct {
	TaskUUID string          `json:"task_uuid"`
	Path     string          `json:"path"`
	Body     json.RawMessage `json:"body"`
}

// maxLocalUploaded kayıt sınırı; dolunca en eskiler düşer.
const maxLocalUploaded = 20000

// LoadLocalState dosyayı okur (yoksa boş başlar).
func LoadLocalState(dir string) (*LocalState, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &LocalState{path: filepath.Join(dir, "helper-state.json")}
	raw, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(raw, &s.data); err != nil {
			return nil, err
		}
	}
	if s.data.Uploaded == nil {
		s.data.Uploaded = map[string]time.Time{}
	}
	if s.data.Armed == nil {
		s.data.Armed = map[string]ArmedFile{}
	}
	return s, nil
}

func (s *LocalState) saveLocked() error {
	for len(s.data.Uploaded) > maxLocalUploaded {
		oldestKey, oldest := "", time.Time{}
		for k, t := range s.data.Uploaded {
			if oldestKey == "" || t.Before(oldest) {
				oldestKey, oldest = k, t
			}
		}
		delete(s.data.Uploaded, oldestKey)
	}
	raw, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func uploadKey(vkn, sha string) string { return digits(vkn) + "|" + sha }

// WasUploaded bu dosya bu firmaya daha önce yüklendi mi.
func (s *LocalState) WasUploaded(vkn, sha string) bool {
	if sha == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.data.Uploaded[uploadKey(vkn, sha)]
	return ok
}

// MarkUploaded dosyayı yüklendi olarak kaydeder.
func (s *LocalState) MarkUploaded(vkn, sha string, at time.Time) error {
	if sha == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Uploaded[uploadKey(vkn, sha)] = at
	return s.saveLocked()
}

// Arm görev için "Yükle"ye basılabilir kaydını yazar.
func (s *LocalState) Arm(a ArmedFile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Armed[a.TaskUUID] = a
	return s.saveLocked()
}

// Disarm kaydı siler (sonuç okundu ya da yükleme yapılmadı).
func (s *LocalState) Disarm(taskUUID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Armed[taskUUID]; !ok {
		return nil
	}
	delete(s.data.Armed, taskUUID)
	return s.saveLocked()
}

// Armed görevin açık kaydı.
func (s *LocalState) Armed(taskUUID string) (ArmedFile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.data.Armed[taskUUID]
	return a, ok
}

// Enqueue gönderilemeyen isteği kuyruğa ekler.
func (s *LocalState) Enqueue(c OutboxCall) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Outbox = append(s.data.Outbox, c)
	return s.saveLocked()
}

// Outbox kuyruğun kopyası.
func (s *LocalState) Outbox() []OutboxCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]OutboxCall{}, s.data.Outbox...)
}

// DropFirst kuyruğun başındaki n isteği siler (gönderildi).
func (s *LocalState) DropFirst(n int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.data.Outbox) {
		n = len(s.data.Outbox)
	}
	s.data.Outbox = append([]OutboxCall{}, s.data.Outbox[n:]...)
	return s.saveLocked()
}

// HasOutbox görevin gönderilmemiş sonucu var mı.
func (s *LocalState) HasOutbox(taskUUID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.data.Outbox {
		if c.TaskUUID == taskUUID {
			return true
		}
	}
	return false
}

// AlwaysOn "bu bilgisayar aktarıcı" ayarı.
func (s *LocalState) AlwaysOn() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.AlwaysOn
}

// SetAlwaysOn ayarı kaydeder.
func (s *LocalState) SetAlwaysOn(on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.AlwaysOn = on
	return s.saveLocked()
}
