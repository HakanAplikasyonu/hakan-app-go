package helper

// Görev çalıştırıcı: yardımcı açıkken sunucudaki Luca görevlerini 15 sn'de
// bir yoklar ve iş sırasına (run_uuid + sequence) göre tek tek çalıştırır.
// Aynı anda tek görev, tek sekme; dosyalar arasında en az 5 sn (§5.7).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Varsayılan aralıklar.
const (
	DefaultPollInterval = 15 * time.Second
	DefaultFileGap      = 5 * time.Second
	maxRecentTasks      = 20
)

// CurrentTask şu an çalışan görev (arayüz).
type CurrentTask struct {
	TaskUUID string `json:"task_uuid"`
	Kind     string `json:"kind"`
	Taxpayer string `json:"taxpayer"`
	Position int    `json:"position"`
	Total    int    `json:"total"`
	Step     int    `json:"step,omitempty"`
	Steps    int    `json:"steps,omitempty"`
	Message  string `json:"message"`
	Waiting  string `json:"waiting,omitempty"`
}

// TaskEvent biten bir görev (arayüz geçmişi).
type TaskEvent struct {
	TaskUUID string    `json:"task_uuid"`
	Kind     string    `json:"kind"`
	Taxpayer string    `json:"taxpayer"`
	Outcome  string    `json:"outcome"`
	Reason   string    `json:"reason,omitempty"`
	At       time.Time `json:"at"`
}

// MapInfo ekran haritasının sürümü.
type MapInfo struct {
	Version     string `json:"version"`
	Verified    bool   `json:"verified"`
	Placeholder bool   `json:"placeholder"`
}

// RunnerStatus arayüz için özet (helper.status).
type RunnerStatus struct {
	Enabled   bool         `json:"enabled"`
	Browser   State        `json:"browser"`
	Map       MapInfo      `json:"map"`
	Current   *CurrentTask `json:"current,omitempty"`
	Recent    []TaskEvent  `json:"recent"`
	Note      string       `json:"note,omitempty"`
	LastError string       `json:"last_error,omitempty"`
	LastRun   *time.Time   `json:"last_run,omitempty"`
}

// Runner görev çalıştırıcı.
type Runner struct {
	Helper    *Helper
	API       TaskClient
	State     *LocalState
	AgentUUID func() string
	Emit      func(RunnerStatus)
	// WorkDir geçici yükleme dosyaları için (uygulama veri dizini).
	WorkDir  string
	Interval time.Duration
	FileGap  time.Duration

	mu        sync.Mutex
	enabled   bool
	cancel    context.CancelFunc
	kick      chan struct{}
	current   *CurrentTask
	recent    []TaskEvent
	note      string
	lastError string
	lastRun   *time.Time
}

// NewRunner çalıştırıcıyı kurar ve yardımcının olaylarını bağlar.
func NewRunner(h *Helper, api TaskClient, st *LocalState, agentUUID func() string, workDir string, emit func(RunnerStatus)) *Runner {
	if emit == nil {
		emit = func(RunnerStatus) {}
	}
	r := &Runner{
		Helper: h, API: api, State: st, AgentUUID: agentUUID, Emit: emit, WorkDir: workDir,
		Interval: DefaultPollInterval, FileGap: DefaultFileGap, kick: make(chan struct{}, 1),
	}
	h.Notify = r.onHelperEvent
	return r
}

// Status özet.
func (r *Runner) Status() RunnerStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := RunnerStatus{
		Enabled: r.enabled, Browser: r.Helper.State(), Recent: append([]TaskEvent{}, r.recent...),
		Note: r.note, LastError: r.lastError,
		Map: MapInfo{Version: r.Helper.Map.Version, Verified: r.Helper.Map.Verified, Placeholder: r.Helper.Map.Placeholder},
	}
	if r.current != nil {
		c := *r.current
		st.Current = &c
	}
	if r.lastRun != nil {
		t := *r.lastRun
		st.LastRun = &t
	}
	return st
}

func (r *Runner) emit() { r.Emit(r.Status()) }

// Enable yardımcıyı açar: tarayıcı (Luca giriş penceresi) arka planda açılır
// ve yoklama başlar.
func (r *Runner) Enable() {
	r.mu.Lock()
	if r.enabled {
		r.mu.Unlock()
		r.Kick()
		go r.openBrowser()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.enabled, r.cancel = true, cancel
	r.mu.Unlock()
	go r.openBrowser()
	go r.loop(ctx)
	r.emit()
}

func (r *Runner) openBrowser() {
	err := r.Helper.Open()
	r.mu.Lock()
	if err != nil {
		r.lastError = "Yardımcı tarayıcı açılamadı: " + err.Error()
	}
	r.mu.Unlock()
	r.emit()
}

// Disable yoklamayı durdurur ve tarayıcıyı kapatır. Çalışan görev
// bağlamı iptal olur; "Yükle"ye basıldıysa görev needs_check olur.
func (r *Runner) Disable() {
	r.mu.Lock()
	if r.cancel != nil {
		r.cancel()
	}
	r.enabled, r.cancel = false, nil
	r.mu.Unlock()
	r.Helper.Close()
	r.emit()
}

// Kick yoklamayı beklemeden başlatır.
func (r *Runner) Kick() {
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

func (r *Runner) loop(ctx context.Context) {
	for {
		r.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.Interval):
		case <-r.kick:
		}
	}
}

func (r *Runner) setNote(note, lastErr string) {
	now := time.Now().UTC()
	r.mu.Lock()
	r.note, r.lastError, r.lastRun = note, lastErr, &now
	r.mu.Unlock()
	r.emit()
}

// RunOnce bir yoklama turu.
func (r *Runner) RunOnce(ctx context.Context) {
	agent := r.AgentUUID()
	if agent == "" {
		r.setNote("Önce oturum açıp ofis seçin.", "")
		return
	}
	if r.Helper.Map.Placeholder {
		r.setNote("Luca ekran haritası henüz keşifle doldurulmadı; yardımcı Luca görevi almıyor.", "")
		return
	}
	if err := r.flush(ctx); err != nil {
		r.setNote("", "Sonuçlar sunucuya gönderilemedi; yeniden denenecek.")
		return
	}
	tasks, err := r.API.Open(ctx)
	if err != nil {
		r.setNote("", "Görevler alınamadı: sunucuya ulaşılamadı ya da oturum kapandı.")
		return
	}
	todo := r.eligible(tasks, agent)
	if len(todo) == 0 {
		r.setNote("Bekleyen Luca görevi yok.", "")
		return
	}
	r.setNote("", "")
	for i, t := range todo {
		if ctx.Err() != nil {
			return
		}
		if stop := r.process(ctx, agent, t, i+1, len(todo)); stop {
			return
		}
		if i < len(todo)-1 {
			if sleep(ctx, r.FileGap) != nil {
				return
			}
		}
	}
}

// eligible ajanın alabileceği görevler: kendi açık görevleri önce, sonra
// onaylı ve sırada olanlar; iş sırası run_uuid (sunucunun sırası) + sequence.
func (r *Runner) eligible(tasks []Task, agent string) []Task {
	runOrder := map[string]int{}
	var out []Task
	for _, t := range tasks {
		if t.Kind != KindCariSync && t.Kind != KindBulkExport {
			continue
		}
		mine := t.Agent != nil && t.Agent.UUID == agent &&
			(t.Status == StatusClaimed || t.Status == StatusWaitingUser || t.Status == StatusRunning)
		if !mine && (t.Status != StatusQueued || !t.Approved) {
			continue
		}
		if r.State.HasOutbox(t.UUID) {
			continue
		}
		if _, ok := runOrder[t.run()]; !ok {
			runOrder[t.run()] = len(runOrder)
		}
		out = append(out, t)
	}
	isMine := func(t Task) bool { return t.Status != StatusQueued }
	sort.SliceStable(out, func(i, j int) bool {
		if isMine(out[i]) != isMine(out[j]) {
			return isMine(out[i])
		}
		if ri, rj := runOrder[out[i].run()], runOrder[out[j].run()]; ri != rj {
			return ri < rj
		}
		return out[i].Sequence < out[j].Sequence
	})
	return out
}

func (r *Runner) setCurrent(c *CurrentTask) {
	r.mu.Lock()
	r.current = c
	r.mu.Unlock()
	r.emit()
}

func (r *Runner) updateCurrent(fn func(c *CurrentTask)) *CurrentTask {
	r.mu.Lock()
	var out *CurrentTask
	if r.current != nil {
		fn(r.current)
		c := *r.current
		out = &c
	}
	r.mu.Unlock()
	r.emit()
	return out
}

// onHelperEvent yardımcının bekleme durumunu sunucuya ve arayüze taşır.
func (r *Runner) onHelperEvent(ev Event) {
	if ev.Kind == "closed" {
		r.emit()
		return
	}
	c := r.updateCurrent(func(c *CurrentTask) {
		c.Waiting = ""
		if ev.Kind == "waiting_user" {
			c.Waiting = ev.Reason
			c.Message = ev.Message
		}
	})
	if c == nil {
		return
	}
	status := StatusRunning
	if ev.Kind == "waiting_user" {
		status = StatusWaitingUser
	}
	agent := r.AgentUUID()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = r.API.Progress(ctx, c.TaskUUID, ProgressReport{AgentUUID: agent, Status: status, Step: c.Step, Steps: c.Steps, Message: clip(c.Message, 200)})
	}()
}

func (r *Runner) progress(ctx context.Context, agent string, step, steps int, msg string) {
	c := r.updateCurrent(func(c *CurrentTask) { c.Step, c.Steps, c.Message, c.Waiting = step, steps, msg, "" })
	if c == nil {
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_ = r.API.Progress(pctx, c.TaskUUID, ProgressReport{AgentUUID: agent, Status: StatusRunning, Step: step, Steps: steps, Message: clip(msg, 200)})
}

func (r *Runner) record(t Task, outcome, reason string) {
	ev := TaskEvent{TaskUUID: t.UUID, Kind: t.Kind, Taxpayer: t.Taxpayer.Title, Outcome: outcome, Reason: reason, At: time.Now().UTC()}
	r.mu.Lock()
	r.recent = append([]TaskEvent{ev}, r.recent...)
	if len(r.recent) > maxRecentTasks {
		r.recent = r.recent[:maxRecentTasks]
	}
	r.mu.Unlock()
}

// process bir görevi çalıştırır. stop=true ise bu turda başka görev
// denenmez (kullanıcı yok, ayar kapalı, ağ sorunu).
func (r *Runner) process(ctx context.Context, agent string, t Task, pos, total int) (stop bool) {
	// Uygulama "Yükle" izlenirken kapandıysa: sonuç belirsiz, asla yeniden
	// yüklenmez.
	if a, ok := r.State.Armed(t.UUID); ok {
		if a.File != "" {
			r.send(ctx, t.UUID, FileResultPath(t.UUID, a.File), FileResult{AgentUUID: agent, Status: FileUnknown,
				LucaMessage: "Yükle'ye basılmış olabilir; uygulama kapandığı için sonuç okunamadı.", Entries: []EntryResult{}})
		}
		r.finish(ctx, agent, t, TaskResult{Outcome: OutcomeNeedsCheck, ErrorCode: CodeUnread,
			ErrorMessage: "Yükleme sırasında uygulama kapandı; Luca'ya bakıp görevi kapatın."})
		_ = r.State.Disarm(t.UUID)
		return false
	}
	// Tarayıcı görev üstlenilmeden açılır: açılamıyorsa görev alınmaz.
	if err := r.Helper.Open(); err != nil {
		r.setNote("", "Yardımcı tarayıcı açılamadı: "+err.Error())
		return true
	}
	claimed, err := r.API.Claim(ctx, t.UUID, agent)
	if err != nil {
		status, code := apiStatus(err)
		switch {
		case status == 409 && code == "HELPER_DISABLED":
			r.setNote("Ofis ayarlarında Luca yardımcısı kapalı. Ofis panelinden açılınca görevler alınır.", "")
			return true
		case status == 409 && (code == "TASK_NOT_READY" || code == "TASK_STATE" || code == "TASK_NOT_APPROVED"):
			return false
		case status == 409:
			return true
		}
		r.setNote("", "Görev üstlenilemedi; yeniden denenecek.")
		return true
	}
	switch claimed.Status {
	case StatusClaimed, StatusWaitingUser, StatusRunning:
	default:
		// ör. NOTHING_TO_EXPORT: sunucu görevi bitirdi.
		r.record(claimed, claimed.Status, claimed.ErrorCode)
		return false
	}
	t = claimed
	r.setCurrent(&CurrentTask{TaskUUID: t.UUID, Kind: t.Kind, Taxpayer: t.Taxpayer.Title, Position: pos, Total: total,
		Message: fmt.Sprintf("Mükellef %d/%d · hazırlanıyor", pos, total)})
	defer r.setCurrent(nil)
	r.progress(ctx, agent, 0, 0, fmt.Sprintf("Mükellef %d/%d · hazırlanıyor", pos, total))

	r.Helper.ResetGuard()
	if err := r.Helper.EnsureSession(ctx); err != nil {
		res := failure(err, r.Helper)
		r.finish(ctx, agent, t, res)
		return res.ErrorCode == CodeLoginTimeout || res.ErrorCode == CodeBrowser
	}
	if _, err := r.Helper.VerifyCompany(ctx, t.Taxpayer.TaxID); err != nil {
		r.finish(ctx, agent, t, failure(err, r.Helper))
		return false
	}
	var res TaskResult
	var retry bool
	switch t.Kind {
	case KindCariSync:
		res = r.cariSync(ctx, agent, t, pos, total)
	case KindBulkExport:
		res, retry = r.uploadFiles(ctx, agent, t, pos, total)
	}
	if retry {
		// İndirme / ağ sorunu: görev üstlenilmiş kalır, sonraki turda sürer.
		r.setNote("", "Dosya sunucudan indirilemedi; yeniden denenecek.")
		return true
	}
	r.finish(ctx, agent, t, res)
	return res.ErrorCode == CodeBrowser || res.ErrorCode == CodeRate
}

// failure hatayı sözleşmedeki koda çevirir.
func failure(err error, h *Helper) TaskResult {
	code, msg := errCode(err, h)
	return TaskResult{Outcome: OutcomeFailed, ErrorCode: code, ErrorMessage: msg}
}

func errCode(err error, h *Helper) (string, string) {
	switch {
	case errors.Is(err, ErrLoginTimeout):
		return CodeLoginTimeout, "Luca'ya süre içinde giriş yapılmadı."
	case errors.Is(err, ErrCompanyMismatch):
		return CodeFirmMismatch, "Luca'da seçili firma mükellefin VKN'siyle tutmuyor; hiçbir şey yüklenmedi."
	case errors.Is(err, ErrCompanyNotFound):
		return CodeFirmNotFound, "Luca firma listesinde mükellefin VKN'si bulunamadı."
	case errors.Is(err, ErrScreenChanged):
		return CodeScreen, "Luca ekranı değişmiş olabilir; yardımcı tahmin etmeden durdu."
	case errors.Is(err, ErrOutsideHost):
		return CodeDomain, "Tarayıcı Luca dışındaki bir adrese gitti; yardımcı durdu."
	case errors.Is(err, ErrRateLimit), errors.Is(err, ErrThrottled):
		return CodeRate, "Luca'yı yormamak için yükleme durduruldu; biraz sonra yeniden deneyin."
	case errors.Is(err, ErrNotUploaded):
		return CodeCancelled, "Doğrulama kodu girilip Yükle'ye basılmadı; dosya yüklenmedi."
	}
	if !h.State().Open || errors.Is(err, ErrNotOpen) || errors.Is(err, context.Canceled) {
		return CodeBrowser, "Yardımcı tarayıcı kapandı."
	}
	return CodeScreen, "Luca ekranında beklenmeyen bir durum oluştu; yardımcı durdu."
}

// finish görevin sonucunu gönderir.
func (r *Runner) finish(ctx context.Context, agent string, t Task, res TaskResult) {
	res.AgentUUID = agent
	res.ErrorMessage = clip(res.ErrorMessage, 500)
	r.send(ctx, t.UUID, ResultPath(t.UUID), res)
	outcome := res.Outcome
	reason := res.ErrorMessage
	r.record(t, outcome, reason)
}

// send sonucu gönderir; gönderilemezse sırayla yeniden denenmek üzere yerel
// kuyruğa yazar. Aynı görevin önceki sonucu kuyruktaysa sıra bozulmasın diye
// doğrudan kuyruğa eklenir.
func (r *Runner) send(ctx context.Context, taskUUID, path string, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		return
	}
	if !r.State.HasOutbox(taskUUID) {
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		err := r.API.Post(sctx, path, json.RawMessage(raw))
		cancel()
		if err == nil || isFinal(err) {
			return
		}
	}
	_ = r.State.Enqueue(OutboxCall{TaskUUID: taskUUID, Path: path, Body: raw})
}

// isFinal sunucu isteği kesin olarak reddetti (yeniden gönderilmez).
func isFinal(err error) bool {
	status, _ := apiStatus(err)
	return status == 404 || status == 409 || status == 400 || status == 422
}

// flush kuyruktaki sonuçları sırayla gönderir.
func (r *Runner) flush(ctx context.Context) error {
	for _, c := range r.State.Outbox() {
		sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := r.API.Post(sctx, c.Path, c.Body)
		cancel()
		if err != nil && !isFinal(err) {
			return err
		}
		if err := r.State.DropFirst(1); err != nil {
			return err
		}
	}
	return nil
}

// tempFile yükleme için geçici dosya (0600); Luca'da görünen ad korunur.
func (r *Runner) tempFile(name string, data []byte) (string, error) {
	dir := filepath.Join(r.WorkDir, "yardimci-gecici")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = "hakan.xlsx"
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return "", err
	}
	return p, nil
}

// uploadFiles luca_bulk_export: dosyaları sırayla Excel Veri Aktarımı
// ekranına yükler; her dosyanın sonucunu hemen bildirir.
func (r *Runner) uploadFiles(ctx context.Context, agent string, t Task, pos, total int) (TaskResult, bool) {
	files := append([]TaskFile{}, t.Files...)
	sort.SliceStable(files, func(i, j int) bool { return files[i].Sequence < files[j].Sequence })
	steps := len(files)
	vkn := t.Taxpayer.TaxID
	for i, f := range files {
		if f.Status != "" && f.Status != "pending" {
			continue // önceki turda bildirildi
		}
		label := fmt.Sprintf("Mükellef %d/%d · Dosya %d/%d", pos, total, i+1, steps)
		r.progress(ctx, agent, i+1, steps, label)
		if r.State.WasUploaded(vkn, f.SHA256) {
			r.send(ctx, t.UUID, FileResultPath(t.UUID, f.UUID), FileResult{AgentUUID: agent, Status: FileOK,
				LucaMessage: "Bu dosya bu firmaya daha önce yüklendi; ikinci kez yüklenmedi.", Entries: []EntryResult{}})
			continue
		}
		data, err := r.API.DownloadFile(ctx, t.UUID, f)
		if errors.Is(err, ErrChecksum) {
			r.send(ctx, t.UUID, FileResultPath(t.UUID, f.UUID), FileResult{AgentUUID: agent, Status: FileFailed,
				LucaMessage: "İndirilen dosya görevdeki dosyayla aynı değil; Luca'ya yüklenmedi.", Entries: []EntryResult{}})
			continue
		}
		if err != nil {
			return TaskResult{}, true
		}
		path, err := r.tempFile(f.Name, data)
		if err != nil {
			return TaskResult{Outcome: OutcomeFailed, ErrorCode: CodeBrowser, ErrorMessage: "Geçici dosya yazılamadı."}, false
		}
		fileUUID, sha := f.UUID, f.SHA256
		res, err := r.Helper.Import(ctx, ImportRequest{
			Screen: ScreenFis, Path: path, Params: f.Luca,
			Message: label + ": Luca'da doğrulama kodunu yazıp Yükle'ye basın.",
			OnArmed: func() error {
				return r.State.Arm(ArmedFile{TaskUUID: t.UUID, File: fileUUID, SHA256: sha, At: time.Now().UTC()})
			},
		})
		_ = os.Remove(path)
		if err != nil {
			_ = r.State.Disarm(t.UUID)
			return failure(err, r.Helper), false
		}
		status := res.Status
		if status == FilePartial {
			status = FileFailed // sözleşme: ok | failed | unknown; satırlar entries'te
		}
		fr := FileResult{AgentUUID: agent, Status: status, LucaMessage: res.Message, Entries: entryResults(res.Errors)}
		r.send(ctx, t.UUID, FileResultPath(t.UUID, f.UUID), fr)
		if res.Status == FileOK || res.Status == FilePartial {
			_ = r.State.MarkUploaded(vkn, f.SHA256, time.Now().UTC())
		}
		_ = r.State.Disarm(t.UUID)
		if res.Status == FileUnknown {
			return TaskResult{Outcome: OutcomeNeedsCheck, ErrorCode: CodeUnread,
				ErrorMessage: "Yükle'ye basıldı ama Luca'nın sonucu okunamadı; Luca'ya bakıp görevi kapatın."}, false
		}
		if i < len(files)-1 && sleep(ctx, r.FileGap) != nil {
			return failure(ctx.Err(), r.Helper), false
		}
	}
	return TaskResult{Outcome: OutcomeDone}, false
}

func entryResults(rows []RowError) []EntryResult {
	out := []EntryResult{}
	for _, e := range rows {
		er := EntryResult{DocumentNo: e.Ref, Status: "failed", Reason: clip(e.Message, 300)}
		if n, err := strconv.Atoi(strings.TrimSpace(e.Line)); err == nil {
			er.Row = n
		} else {
			er.EntryNo = e.Line
		}
		out = append(out, er)
	}
	return out
}

// cariSync luca_cari_sync: carileri Luca hesap planında arar; yoksa
// sıradaki kodla tek hesap planı dosyasıyla açar ve yeniden okuyarak
// doğrular (tasarım §0.1 Senaryo 1, adım 2–5).
func (r *Runner) cariSync(ctx context.Context, agent string, t Task, pos, total int) TaskResult {
	var items []CariItem
	for _, c := range t.Counterparties {
		if c.Status == "" || c.Status == "pending" {
			items = append(items, CariItem{VKN: c.TaxID, Name: c.Title, Parent: c.MainCode})
		}
	}
	if len(items) == 0 {
		return TaskResult{Outcome: OutcomeDone}
	}
	label := fmt.Sprintf("Mükellef %d/%d · Cariler", pos, total)
	r.progress(ctx, agent, 1, 3, label+": Luca hesap planı okunuyor")
	accounts, err := r.Helper.ReadAccounts(ctx)
	if err != nil {
		return failure(err, r.Helper)
	}
	found, create, failed := PlanCari(items, accounts, nil)
	out := cariResults(found, failed)
	if len(create) == 0 {
		return TaskResult{Outcome: OutcomeDone, Counterparties: out}
	}
	// Çakışma koruması: yüklemeden hemen önce yeniden okunur.
	accounts, err = r.Helper.ReadAccounts(ctx)
	if err != nil {
		return withCari(failure(err, r.Helper), out)
	}
	found2, create, failed2 := PlanCari(plannedItems(create), accounts, nil)
	out = append(out, cariResults(found2, failed2)...)
	if len(create) == 0 {
		return TaskResult{Outcome: OutcomeDone, Counterparties: out}
	}
	data, err := BuildPlanFile(r.Helper.Map.PlanFile, create)
	if err != nil {
		return withCari(TaskResult{Outcome: OutcomeFailed, ErrorCode: CodeScreen, ErrorMessage: "Hesap planı dosyası hazırlanamadı."}, out)
	}
	ext := r.Helper.Map.PlanFile.Extension
	if ext == "" {
		ext = ".csv"
	}
	path, err := r.tempFile("hakan-yeni-cariler"+ext, data)
	if err != nil {
		return withCari(TaskResult{Outcome: OutcomeFailed, ErrorCode: CodeBrowser, ErrorMessage: "Geçici dosya yazılamadı."}, out)
	}
	r.progress(ctx, agent, 2, 3, fmt.Sprintf("%s: %d yeni cari Luca'da açılacak", label, len(create)))
	res, err := r.Helper.Import(ctx, ImportRequest{
		Screen: ScreenPlan, Path: path,
		Message: fmt.Sprintf("%s: %d yeni cari için Luca'da doğrulama kodunu yazıp Yükle'ye basın.", label, len(create)),
		OnArmed: func() error {
			return r.State.Arm(ArmedFile{TaskUUID: t.UUID, At: time.Now().UTC()})
		},
	})
	_ = os.Remove(path)
	if err != nil {
		_ = r.State.Disarm(t.UUID)
		return withCari(failure(err, r.Helper), out)
	}
	// Yüklemeden sonra yeniden okunur ve "açıldı" doğrulanır.
	r.progress(ctx, agent, 3, 3, label+": açılan cariler doğrulanıyor")
	after, rerr := r.Helper.ReadAccounts(ctx)
	_ = r.State.Disarm(t.UUID)
	if rerr != nil {
		return withCari(TaskResult{Outcome: OutcomeNeedsCheck, ErrorCode: CodeUnread,
			ErrorMessage: "Cariler yüklendi ama Luca hesap planı yeniden okunamadı; Luca'ya bakıp görevi kapatın."}, out)
	}
	reasons := map[string]string{}
	for _, e := range res.Errors {
		reasons[e.Ref] = e.Message
	}
	for _, c := range create {
		if a, ok := findAccount(after, c.Item.Parent, c.Item.VKN); ok {
			out = append(out, CounterpartyResult{TaxID: c.Item.VKN, MainCode: c.Item.Parent, Status: CariCreated, AccountCode: a.Code, Title: clip(a.Name, 250)})
			continue
		}
		msg := reasons[c.Code]
		if msg == "" {
			msg = "Luca'da açılmadı (" + c.Code + ")."
		}
		out = append(out, CounterpartyResult{TaxID: c.Item.VKN, MainCode: c.Item.Parent, Status: CariFailed, Message: clip(msg, 300)})
	}
	return TaskResult{Outcome: OutcomeDone, Counterparties: out}
}

func plannedItems(p []PlannedCari) []CariItem {
	out := make([]CariItem, 0, len(p))
	for _, c := range p {
		out = append(out, c.Item)
	}
	return out
}

func cariResults(found, failed []CariResult) []CounterpartyResult {
	var out []CounterpartyResult
	for _, list := range [][]CariResult{found, failed} {
		for _, c := range list {
			out = append(out, CounterpartyResult{TaxID: c.VKN, MainCode: c.Parent, Status: c.Status, AccountCode: c.Code, Title: clip(c.Name, 250), Message: c.Reason})
		}
	}
	return out
}

func withCari(res TaskResult, c []CounterpartyResult) TaskResult {
	res.Counterparties = c
	return res
}
