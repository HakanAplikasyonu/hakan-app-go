package helper

// Luca'nın kendi içe aktarma ekranına dosya yükleme (yöntem (a), tasarım §3;
// akış §2.3 adım 5–7). Yardımcı dosyayı dosya alanına verir ve haritadaki
// seçimleri yapar. Ekranda doğrulama kodu varsa kodu kullanıcı yazar,
// "Yükle"ye kullanıcı basar. Kod yoksa (Rahat Aktarım gibi) ve ofis izni
// varsa (AutoClick) "Yükle"ye yardımcı basar; izin yoksa kullanıcı basar.
// Sonra sonuç alanı okunur. "Yükle"ye basıldıktan sonra sonuç
// okunamazsa sonuç "unknown" olur: görev needs_check'e düşer ve asla
// otomatik tekrar denenmez (§2.1).

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// Ekranlar.
const (
	ScreenFis  = "fis"  // Excel Veri Aktarımı
	ScreenPlan = "plan" // Hesap Planı aktarımı
)

// Dosya sonucu durumları (tasarım §2.4).
const (
	FileOK      = "ok"
	FilePartial = "partial"
	FileFailed  = "failed"
	FileUnknown = "unknown"
)

// ImportRequest bir dosyanın yüklenmesi.
type ImportRequest struct {
	Screen string
	Path   string
	Params map[string]string
	// Message kod istenirken gösterilen bekleme metni ("Dosya 2/4 …").
	Message string
	// ConfirmMessage kod istenmeyen ekranda, AutoClick kapalıyken "Yükle"ye
	// basılması beklenirken gösterilen metin.
	ConfirmMessage string
	// AutoClick ofis izni var: ekranda doğrulama kodu yoksa "Yükle"ye
	// yardımcı basar.
	AutoClick bool
	// OnArmed "Yükle" düğmesi izlenmeye başlamadan hemen önce çağrılır;
	// çalıştırıcı yerel kayda "yükleme başlayabilir" yazar.
	OnArmed func() error
}

// RowError Luca'nın bir hata satırı.
type RowError struct {
	Line    string `json:"line,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Message string `json:"message"`
}

// ImportResult sonuç ekranından okunan.
type ImportResult struct {
	Status       string     `json:"status"`
	Clicked      bool       `json:"clicked"`
	SuccessCount int        `json:"success_count"`
	Errors       []RowError `json:"errors,omitempty"`
	Message      string     `json:"message,omitempty"`
}

func (h *Helper) screen(name string) (ImportMap, error) {
	switch name {
	case ScreenFis:
		return h.Map.Import, nil
	case ScreenPlan:
		return h.Map.PlanImport, nil
	}
	return ImportMap{}, fmt.Errorf("bilinmeyen içe aktarma ekranı: %s", name)
}

// checkRate saatlik yükleme sınırı.
func (h *Helper) checkRate(now time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	keep := h.uploads[:0]
	for _, t := range h.uploads {
		if now.Sub(t) < time.Hour {
			keep = append(keep, t)
		}
	}
	h.uploads = keep
	if len(h.uploads) >= h.Opts.MaxUploadsPerHour {
		return ErrRateLimit
	}
	return nil
}

func (h *Helper) noteUpload(now time.Time) {
	h.mu.Lock()
	h.uploads = append(h.uploads, now)
	h.mu.Unlock()
}

// Import dosyayı Luca ekranına verir, kullanıcının kodu girip "Yükle"ye
// basmasını bekler ve sonucu okur.
//
// Dönüş: err doluysa dosya Luca'ya gitmedi (Clicked=false). err boşsa
// sonuç Status'ta; FileUnknown "basıldı ama sonuç okunamadı" demektir.
func (h *Helper) Import(ctx context.Context, req ImportRequest) (ImportResult, error) {
	im, err := h.screen(req.Screen)
	if err != nil {
		return ImportResult{}, err
	}
	if err := h.checkRate(time.Now()); err != nil {
		return ImportResult{}, err
	}
	var res ImportResult
	err = h.withOp(ctx, func(ctx context.Context) error {
		r, err := h.importOn(ctx, im, req)
		res = r
		return err
	})
	if res.Clicked {
		// "Yükle"ye basıldıysa hata da olsa sonuç belirsizdir: needs_check.
		if err != nil || res.Status == "" {
			res.Status = FileUnknown
		}
		return res, nil
	}
	return res, err
}

func (h *Helper) importOn(ctx context.Context, im ImportMap, req ImportRequest) (ImportResult, error) {
	ready := im.Ready
	if ready == "" {
		ready = im.FileInput
	}
	if err := h.visit(ctx, im.URL, ready); err != nil {
		return ImportResult{}, err
	}
	// Dosya alanı gerçekten dosya alanı mı (başka bir alana değer yazılmaz).
	ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(function(){var e=document.querySelector(`+js(im.FileInput)+
		`);return !!e&&e.tagName==="INPUT"&&e.type==="file"})()`))
	if err != nil {
		return ImportResult{}, err
	}
	if !ok {
		return ImportResult{}, fmt.Errorf("%w (%s)", ErrScreenChanged, im.FileInput)
	}
	if err := h.pace(ctx); err != nil {
		return ImportResult{}, err
	}
	fctx, cancel := context.WithTimeout(ctx, h.Opts.ReadyTimeout)
	err = chromedp.Do(fctx, chromedp.SetUploadFiles(chromedp.CSS(im.FileInput), []string{req.Path}))
	cancel()
	if err != nil {
		return ImportResult{}, fmt.Errorf("dosya Luca ekranına verilemedi: %w", err)
	}
	for _, s := range im.Selects {
		if err := h.choose(ctx, s, req.Params); err != nil {
			return ImportResult{}, err
		}
	}
	before, _ := textOf(ctx, im.Result)
	if req.OnArmed != nil {
		if err := req.OnArmed(); err != nil {
			return ImportResult{}, err
		}
	}
	// Doğrulama kodu alanı haritada yoksa ya da ekranda görünmüyorsa bu
	// aktarım kod istemiyor.
	needCode := false
	if im.CodeInput != "" {
		needCode, _ = exists(ctx, im.CodeInput)
	}
	h.arm(true)
	defer h.arm(false)
	// "Yükle" düğmesine ve formun gönderilmesine dinleyici; doğrulama kodu
	// alanına yalnızca odaklanılır.
	hooked, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(function(){var b=document.querySelector(`+js(im.UploadButton)+
		`);if(!b)return false;var f=function(){try{if(window.`+exposeName+`)window.`+exposeName+`()}catch(e){}};`+
		`b.addEventListener("click",f,true);var fm=b.form||b.closest("form");if(fm)fm.addEventListener("submit",f,true);`+
		`var c=`+js(im.CodeInput)+`?document.querySelector(`+js(im.CodeInput)+`):null;if(c&&c.type!=="password")c.focus();return true})()`))
	if err != nil {
		return ImportResult{}, err
	}
	if !hooked {
		return ImportResult{}, fmt.Errorf("%w (%s)", ErrScreenChanged, im.UploadButton)
	}
	switch {
	case needCode:
		msg := req.Message
		if msg == "" {
			msg = "Luca'da doğrulama kodunu yazıp Yükle'ye basın."
		}
		h.setWaiting("upload_code", msg)
	case req.AutoClick:
		// Kod yok, ofis izni var: "Yükle"ye yardımcı basar. Basıldı kaydı
		// tıklamadan ÖNCE düşülür; sonuç okunamazsa dosya needs_check olur,
		// asla ikinci kez yüklenmez.
		if err := h.pace(ctx); err != nil {
			return ImportResult{}, err
		}
		h.markClicked()
		pressed, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(function(){var b=document.querySelector(`+js(im.UploadButton)+
			`);if(!b)return false;b.click();return true})()`))
		if err == nil && !pressed {
			h.arm(false)
			return ImportResult{}, fmt.Errorf("%w (%s)", ErrScreenChanged, im.UploadButton)
		}
	default:
		msg := req.ConfirmMessage
		if msg == "" {
			msg = "Luca'da Yükle'ye basın."
		}
		h.setWaiting("upload_confirm", msg)
	}
	defer h.setWaiting("", "Sonuç okunuyor.")

	deadline := time.Now().Add(h.Opts.UserTimeout)
	for {
		clicked := h.wasClicked()
		if err := h.guardErr(); err != nil {
			return ImportResult{Clicked: clicked}, err
		}
		cur, err := textOf(ctx, im.Result)
		if err == nil && cur != nil && strings.TrimSpace(*cur) != "" && (clicked || before == nil || *cur != *before) {
			r, perr := h.readResult(ctx, im, *cur)
			r.Clicked = true
			h.noteUpload(time.Now())
			return r, perr
		}
		if ctx.Err() != nil {
			// Tarayıcı kapandı ya da görev iptal edildi.
			if clicked {
				h.noteUpload(time.Now())
				return ImportResult{Clicked: true, Status: FileUnknown}, nil
			}
			return ImportResult{}, ErrNotUploaded
		}
		if time.Now().After(deadline) {
			if clicked {
				h.noteUpload(time.Now())
				return ImportResult{Clicked: true, Status: FileUnknown}, nil
			}
			return ImportResult{}, ErrNotUploaded
		}
		if err := sleep(ctx, h.Opts.PollEvery); err != nil && !h.wasClicked() {
			return ImportResult{}, ErrNotUploaded
		}
	}
}

// choose haritadaki bir seçim kutusunu ayarlar. Seçenek yoksa tahmin etmez.
func (h *Helper) choose(ctx context.Context, s SelectStep, params map[string]string) error {
	v := s.Value
	if s.Param != "" {
		if pv, ok := params[s.Param]; ok {
			v = pv
		}
	}
	if v == "" {
		if s.Optional {
			return nil
		}
		return fmt.Errorf("görevde %q değeri yok", s.Param)
	}
	if err := h.pace(ctx); err != nil {
		return err
	}
	res, err := chromedp.Run(ctx, chromedp.Evaluate[string](`(function(){var e=document.querySelector(`+js(s.Selector)+
		`);if(!e||e.tagName!=="SELECT")return "missing";var v=`+js(v)+
		`;if(!Array.from(e.options).some(function(o){return o.value===v}))return "nooption";`+
		`e.value=v;e.dispatchEvent(new Event("input",{bubbles:true}));e.dispatchEvent(new Event("change",{bubbles:true}));return "ok"})()`))
	if err != nil {
		return err
	}
	switch res {
	case "ok":
		return nil
	case "missing":
		if s.Optional {
			return nil
		}
	}
	return fmt.Errorf("%w (%s = %s)", ErrScreenChanged, s.Selector, v)
}

var firstInt = regexp.MustCompile(`\d+`)

// readResult sonuç alanından başarı sayısını ve hata satırlarını okur.
func (h *Helper) readResult(ctx context.Context, im ImportMap, text string) (ImportResult, error) {
	r := ImportResult{Message: mask(text, 500)}
	countFound := false
	if im.SuccessCount != "" {
		if t, err := textOf(ctx, im.SuccessCount); err == nil && t != nil {
			if m := firstInt.FindString(*t); m != "" {
				r.SuccessCount, _ = strconv.Atoi(m)
				countFound = true
			}
		}
	}
	if !countFound && im.SuccessPattern != "" {
		if re, err := regexp.Compile("(?i)" + im.SuccessPattern); err == nil {
			if m := re.FindStringSubmatch(text); len(m) > 1 {
				r.SuccessCount, _ = strconv.Atoi(m[1])
				countFound = true
			}
		}
	}
	if im.ErrorRow != "" {
		rows, err := chromedp.Run(ctx, chromedp.Evaluate[[]RowError](`(function(){function t(r,s){if(!s)return "";var e=r.querySelector(s);return e?(e.innerText||e.textContent||"").trim():""}`+
			`return Array.from(document.querySelectorAll(`+js(im.ErrorRow)+`)).map(function(r){var m=t(r,`+js(im.ErrorMessage)+
			`);return {line:t(r,`+js(im.ErrorLine)+`),ref:t(r,`+js(im.ErrorRef)+`),message:m||(r.innerText||"").trim()}})})()`))
		if err == nil {
			for _, e := range rows {
				r.Errors = append(r.Errors, RowError{Line: clip(e.Line, 20), Ref: clip(e.Ref, 64), Message: mask(e.Message, 300)})
			}
		}
	}
	failed := false
	if im.FailurePattern != "" {
		if re, err := regexp.Compile("(?i)" + im.FailurePattern); err == nil && re.MatchString(text) {
			failed = true
		}
	}
	r.Status = resultStatus(countFound, r.SuccessCount, len(r.Errors), failed)
	return r, nil
}

// resultStatus sonuç ekranının yorumu. Ne başarı sayısı ne hata okunabildiyse
// sonuç belirsizdir (needs_check).
func resultStatus(countFound bool, ok, errs int, failureText bool) string {
	switch {
	case countFound && ok > 0 && errs == 0:
		return FileOK
	case ok > 0 && errs > 0:
		return FilePartial
	case errs > 0 || (countFound && ok == 0 && failureText):
		return FileFailed
	}
	return FileUnknown
}

// IsUserCancel kullanıcı yüklemeyi yapmadı mı.
func IsUserCancel(err error) bool { return errors.Is(err, ErrNotUploaded) }
