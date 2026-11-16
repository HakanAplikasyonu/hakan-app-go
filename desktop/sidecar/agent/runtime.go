package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultHeartbeat sunucu başka bir süre bildirmezse (02 §6: 5 dk).
	DefaultHeartbeat = 5 * time.Minute
	// ScanInterval klasör tarama sıklığı.
	ScanInterval = 15 * time.Second
)

// Emitter kabuğa olay yollar (rpc.Server.Emit).
type Emitter func(name string, data any)

// Runtime ajanın çalışan hâlidir: oturum, kayıt, heartbeat ve klasör izleme.
type Runtime struct {
	Client  *Client
	State   *State
	Watcher *Watcher
	Version string
	emit    Emitter

	mu        sync.Mutex
	cancel    context.CancelFunc
	lastBeat  time.Time
	beatError string
	interval  time.Duration
	// kick heartbeat'i beklemeden gönderir; rescan taramayı hemen başlatır
	// (#284). İkisi de tek elemanlı: üst üste gelen istekler birleşir.
	kick   chan struct{}
	rescan chan struct{}
}

// NewRuntime durumu dataDir'den yükler.
func NewRuntime(dataDir, version string, emit Emitter) (*Runtime, error) {
	st, err := LoadState(dataDir)
	if err != nil {
		return nil, err
	}
	if emit == nil {
		emit = func(string, any) {}
	}
	c := NewClient(nil)
	r := &Runtime{
		Client: c, State: st, Version: version, emit: emit, interval: DefaultHeartbeat,
		kick: make(chan struct{}, 1), rescan: make(chan struct{}, 1),
	}
	// Yenileme belirteci kabuğa gider; kabuk anahtar zincirine yazar. Ön
	// yüze iletilmez.
	c.OnRotate = func(rt string) { emit("auth.rotated", map[string]string{"refresh_token": rt}) }
	snap := st.Snapshot()
	if snap.BaseURL != "" {
		c.SetBase(snap.BaseURL)
	}
	if snap.Organization != nil {
		c.SetOrganization(snap.Organization.Slug)
	}
	r.Watcher = &Watcher{State: st, Uploader: c, Emit: func(ev UploadEvent) { emit("upload", ev) }}
	return r, nil
}

// WatchView izlenen klasör ve son etkinliği (#206).
type WatchView struct {
	Watch
	Activity FolderActivity `json:"activity"`
}

// Status kabuk ve arayüz için özet.
type Status struct {
	BaseURL      string      `json:"base_url"`
	SignedIn     bool        `json:"signed_in"`
	Organization *OrgRef     `json:"organization"`
	AgentUUID    string      `json:"agent_uuid,omitempty"`
	AgentName    string      `json:"agent_name,omitempty"`
	MachineID    string      `json:"machine_id"`
	Running      bool        `json:"running"`
	LastBeat     *time.Time  `json:"last_heartbeat,omitempty"`
	BeatError    string      `json:"heartbeat_error,omitempty"`
	Watches      []WatchView `json:"watches"`
	Stats        WatchStats  `json:"stats"`
	Uploaded     int         `json:"uploaded_total"`
}

func (r *Runtime) Status() Status {
	snap := r.State.Snapshot()
	r.mu.Lock()
	defer r.mu.Unlock()
	st := Status{
		BaseURL: snap.BaseURL, SignedIn: r.Client.SignedIn(), Organization: snap.Organization,
		AgentUUID: snap.AgentUUID, AgentName: snap.AgentName, MachineID: snap.MachineID,
		Running: r.cancel != nil, BeatError: r.beatError, Watches: []WatchView{},
		Stats: r.Watcher.Stats(), Uploaded: r.State.UploadedCount(),
	}
	for _, w := range snap.Watches {
		st.Watches = append(st.Watches, WatchView{Watch: w, Activity: r.Watcher.Activity(w.Path)})
	}
	if !r.lastBeat.IsZero() {
		t := r.lastBeat
		st.LastBeat = &t
	}
	return st
}

// SetBaseURL API adresini ayarlar ve kaydeder.
func (r *Runtime) SetBaseURL(raw string) (string, error) {
	base, err := NormalizeBase(raw)
	if err != nil {
		return "", err
	}
	r.Client.SetBase(base)
	return base, r.State.Update(func(d *stateData) {
		if d.BaseURL != base {
			// Başka sunucu: ofis ve ajan kaydı geçersiz.
			d.Organization, d.AgentUUID = nil, ""
		}
		d.BaseURL = base
	})
}

// Login oturum açar ve kullanıcının ofislerini döner.
func (r *Runtime) Login(ctx context.Context, baseURL, email, password, totp string) ([]Organization, error) {
	if _, err := r.SetBaseURL(baseURL); err != nil {
		return nil, err
	}
	if err := r.Client.Login(ctx, email, password, totp); err != nil {
		return nil, err
	}
	return r.Client.Organizations(ctx)
}

// Resume, kabuğun anahtar zincirinden verdiği yenileme belirteciyle oturumu
// sürdürür ve ofis seçiliyse ajanı başlatır.
func (r *Runtime) Resume(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return ErrSignedOut
	}
	if r.Client.Base() == "" {
		return errors.New("API adresi ayarlanmadı")
	}
	r.Client.SetRefreshToken(refreshToken)
	if _, err := r.Client.Organizations(ctx); err != nil {
		return err
	}
	if r.State.Snapshot().Organization != nil {
		return r.Start(ctx)
	}
	return nil
}

// UseOrganization ofisi seçer, makineyi ajan olarak kaydeder ve başlatır.
func (r *Runtime) UseOrganization(ctx context.Context, slug, agentName string) (Status, error) {
	orgs, err := r.Client.Organizations(ctx)
	if err != nil {
		return Status{}, err
	}
	var org *Organization
	for i := range orgs {
		if orgs[i].Slug == slug {
			org = &orgs[i]
		}
	}
	if org == nil {
		return Status{}, errors.New("bu ofise üyeliğiniz yok")
	}
	r.Client.SetOrganization(org.Slug)
	if strings.TrimSpace(agentName) == "" {
		agentName, _ = os.Hostname()
	}
	snap := r.State.Snapshot()
	info, err := r.Client.RegisterAgent(ctx, snap.MachineID, agentName, Platform(), r.Version)
	if err != nil {
		return Status{}, err
	}
	if err := r.State.Update(func(d *stateData) {
		d.Organization = &OrgRef{UUID: org.UUID, Slug: org.Slug, Name: org.Name}
		d.AgentUUID, d.AgentName = info.UUID, info.Name
	}); err != nil {
		return Status{}, err
	}
	r.setInterval(info.HeartbeatSeconds)
	if err := r.Start(ctx); err != nil {
		return Status{}, err
	}
	return r.Status(), nil
}

// Platform sunucunun beklediği adla.
func Platform() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows":
		return "windows"
	}
	return "linux"
}

func (r *Runtime) setInterval(sec int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sec >= 30 {
		r.interval = time.Duration(sec) * time.Second
	}
}

// Start heartbeat ve tarama döngülerini başlatır (zaten çalışıyorsa bir şey
// yapmaz).
func (r *Runtime) Start(_ context.Context) error {
	snap := r.State.Snapshot()
	if snap.AgentUUID == "" || snap.Organization == nil {
		return errors.New("önce ofis seçilmeli")
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.mu.Unlock()

	go r.beatLoop(ctx)
	go r.scanLoop(ctx)
	return nil
}

// Stop döngüleri durdurur.
func (r *Runtime) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}

// Logout oturumu kapatır, döngüleri durdurur. İzlenen klasörler ve yükleme
// geçmişi kalır.
func (r *Runtime) Logout() {
	r.Stop()
	r.Client.SignOut()
	r.emit("auth.signed_out", nil)
}

func (r *Runtime) beatLoop(ctx context.Context) {
	for {
		r.Beat(ctx)
		r.mu.Lock()
		d := r.interval
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		case <-r.kick:
		}
	}
}

// signal tek elemanlı kanala bekletmeden yazar; dolusa istek zaten sırada.
func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// Beat tek bir heartbeat gönderir.
func (r *Runtime) Beat(ctx context.Context) {
	snap := r.State.Snapshot()
	if snap.AgentUUID == "" {
		return
	}
	info, err := r.Client.Heartbeat(ctx, snap.AgentUUID, r.Version, r.Watcher.Stats())
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == 404 {
		// Ajan ofisten geri alındı: yeniden kayıt kullanıcı girişini bekler.
		_ = r.State.Update(func(d *stateData) { d.AgentUUID = "" })
		r.emit("agent.revoked", nil)
		go r.Stop()
	}
	if errors.Is(err, ErrSignedOut) {
		r.emit("auth.signed_out", nil)
		go r.Stop()
	}
	r.mu.Lock()
	if err != nil {
		r.beatError = err.Error()
	} else {
		r.beatError = ""
		r.lastBeat = time.Now().UTC()
	}
	r.mu.Unlock()
	if err == nil {
		r.setInterval(info.HeartbeatSeconds)
		r.emit("agent.heartbeat", map[string]string{"status": info.Status})
	}
}

func (r *Runtime) scanLoop(ctx context.Context) {
	t := time.NewTicker(ScanInterval)
	defer t.Stop()
	prev := r.Watcher.Stats()
	for {
		r.Watcher.Scan(ctx, time.Now())
		// Ofisteki görünüm sonraki heartbeat'i (5 dk) beklemesin: izlenen
		// klasör, yüklenen ya da hatalı sayısı değişince hemen bildir (#284).
		if cur := r.Watcher.Stats(); statsChanged(prev, cur) {
			signal(r.kick)
			prev = cur
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-r.rescan:
		}
	}
}

// AddWatch bir klasörü izlemeye alır.
func (r *Runtime) AddWatch(path, engagementUUID, label, taxpayerTitle string) (Watch, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(path) {
		return Watch{}, errors.New("klasör yolu mutlak olmalı")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return Watch{}, errors.New("klasör bulunamadı")
	}
	if strings.TrimSpace(engagementUUID) == "" {
		return Watch{}, errors.New("mükellef seçilmeli")
	}
	w := Watch{Path: path, EngagementUUID: engagementUUID, Label: strings.TrimSpace(label), TaxpayerTitle: strings.TrimSpace(taxpayerTitle)}
	err = r.State.Update(func(d *stateData) {
		for i := range d.Watches {
			if d.Watches[i].Path == path {
				d.Watches[i] = w
				return
			}
		}
		d.Watches = append(d.Watches, w)
	})
	if err == nil {
		signal(r.rescan)
	}
	return w, err
}

// RemoveWatch klasörü izlemeden çıkarır.
func (r *Runtime) RemoveWatch(path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	err := r.State.Update(func(d *stateData) {
		out := d.Watches[:0]
		for _, w := range d.Watches {
			if w.Path != path {
				out = append(out, w)
			}
		}
		d.Watches = out
	})
	if err == nil {
		signal(r.rescan)
	}
	return err
}

func statsChanged(a, b WatchStats) bool {
	return a.Folders != b.Folders || a.Uploaded != b.Uploaded || a.Failed != b.Failed || a.LastError != b.LastError
}
