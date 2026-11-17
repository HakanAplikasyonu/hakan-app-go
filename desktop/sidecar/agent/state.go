package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// maxUploaded, mükerrer önleme dizinindeki en fazla kayıt; dolunca en eskiler
// düşer.
const maxUploaded = 50000

// Watch, izlenen bir klasör ve dosyalarının yükleneceği hizmet ilişkisi.
type Watch struct {
	Path           string `json:"path"`
	EngagementUUID string `json:"engagement_uuid"`
	Label          string `json:"label"`
	// TaxpayerTitle eklenirken seçilen mükellefin adı (#206); arayüz
	// çevrimdışıyken de adı gösterebilsin diye saklanır.
	TaxpayerTitle string `json:"taxpayer_title,omitempty"`
}

// Uploaded, yüklenmiş (ya da kalıcı olarak reddedilmiş) bir dosyanın kaydı.
// Anahtar SHA-256: aynı dosya başka adla ya da başka klasörde tekrar
// yüklenmez.
type Uploaded struct {
	Name           string    `json:"name"`
	EngagementUUID string    `json:"engagement_uuid"`
	JobUUID        string    `json:"job_uuid,omitempty"`
	Rejected       string    `json:"rejected,omitempty"`
	At             time.Time `json:"at"`
}

// Organization seçimi.
type OrgRef struct {
	UUID string `json:"uuid"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// stateData diske yazılan biçim. Sır içermez: oturum belirteci kabuğun
// anahtar zincirindedir.
type stateData struct {
	BaseURL      string  `json:"base_url"`
	Organization *OrgRef `json:"organization,omitempty"`
	MachineID    string  `json:"machine_id"`
	AgentUUID    string  `json:"agent_uuid,omitempty"`
	AgentName    string  `json:"agent_name,omitempty"`
	Watches      []Watch `json:"watches"`
	// Targets Luca / Zirve aktarım klasörleri (#391).
	Targets  []TransferTarget    `json:"transfer_targets,omitempty"`
	Uploaded map[string]Uploaded `json:"uploaded"`
}

// State, ajanın kalıcı durumu (`<veri dizini>/agent-state.json`, 0600).
type State struct {
	mu   sync.Mutex
	path string
	d    stateData
}

// LoadState dosyayı okur; yoksa yeni makine kimliğiyle boş durum kurar.
func LoadState(dir string) (*State, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &State{path: filepath.Join(dir, "agent-state.json")}
	raw, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(raw, &s.d); err != nil {
			return nil, err
		}
	}
	if s.d.Uploaded == nil {
		s.d.Uploaded = map[string]Uploaded{}
	}
	if s.d.MachineID == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		s.d.MachineID = "hk-" + hex.EncodeToString(b)
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *State) saveLocked() error {
	raw, err := json.MarshalIndent(s.d, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Update durumu değiştirip kaydeder.
func (s *State) Update(fn func(d *stateData)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.d)
	return s.saveLocked()
}

// Snapshot okunacak bir kopya döner.
func (s *State) Snapshot() stateData {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.d
	d.Watches = append([]Watch(nil), s.d.Watches...)
	d.Targets = append([]TransferTarget(nil), s.d.Targets...)
	d.Uploaded = nil
	return d
}

// Seen, dosyanın daha önce işlenip işlenmediğini söyler.
func (s *State) Seen(sha string) (Uploaded, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.d.Uploaded[sha]
	return u, ok
}

// Record bir dosyayı işlenmiş olarak kaydeder.
func (s *State) Record(sha string, u Uploaded) error {
	return s.Update(func(d *stateData) {
		d.Uploaded[sha] = u
		if len(d.Uploaded) <= maxUploaded {
			return
		}
		type kv struct {
			k string
			t time.Time
		}
		all := make([]kv, 0, len(d.Uploaded))
		for k, v := range d.Uploaded {
			all = append(all, kv{k, v.At})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].t.Before(all[j].t) })
		for _, e := range all[:len(all)-maxUploaded] {
			delete(d.Uploaded, e.k)
		}
	})
}

// UploadedCount kayıtlı dosya sayısı.
func (s *State) UploadedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.d.Uploaded)
}
