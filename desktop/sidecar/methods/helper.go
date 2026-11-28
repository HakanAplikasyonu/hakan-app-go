package methods

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/helper"
	"github.com/HakanAplikasyonu/hakan-app-go/desktop/sidecar/rpc"
)

// RegisterHelper Luca yardımcısının yöntemlerini bağlar (tasarım notu
// masaustu-luca-sgk, iş planı adım 2). Yardımcı Luca şifresini, çerezini ya
// da sayfa içeriğini ön yüze ve sunucuya taşımaz; bu yöntemler yalnızca
// durum ve sonuç döner.
func RegisterHelper(s *rpc.Server, r *helper.Runner) {
	s.Handle("helper.status", func(context.Context, json.RawMessage) (any, error) {
		return r.Status(), nil
	})
	// helper.start: yardımcı tarayıcıyı (Luca giriş penceresi) açar ve görev
	// yoklamasını başlatır.
	s.Handle("helper.start", func(context.Context, json.RawMessage) (any, error) {
		r.Enable()
		return r.Status(), nil
	})
	s.Handle("helper.stop", func(context.Context, json.RawMessage) (any, error) {
		r.Disable()
		return r.Status(), nil
	})
	s.Handle("helper.run_now", func(context.Context, json.RawMessage) (any, error) {
		r.Kick()
		return r.Status(), nil
	})
	s.Handle("helper.check_session", func(ctx context.Context, _ json.RawMessage) (any, error) {
		ok, err := r.Helper.SessionOpen(ctx)
		switch {
		case errors.Is(err, helper.ErrNotOpen):
			return map[string]any{"open": false, "logged_in": false}, nil
		case err != nil:
			return nil, helperErr(err)
		}
		return map[string]any{"open": true, "logged_in": ok}, nil
	})
	s.Handle("helper.verify_company", func(ctx context.Context, params json.RawMessage) (any, error) {
		var p struct {
			TaxID string `json:"tax_id"`
		}
		if err := rpc.Decode(params, &p); err != nil {
			return nil, err
		}
		info, err := r.Helper.VerifyCompany(ctx, p.TaxID)
		if err != nil {
			return nil, helperErr(err)
		}
		return info, nil
	})
	// Kayıtlı Luca girişi: yalnızca kabuk çağırır (ALLOWED_METHODS'ta yok).
	// Kabuk değeri OS şifre kasasından okuyup buraya verir; sidecar yalnızca
	// bellekte tutar, yanıt değer içermez.
	s.Handle("helper.set_login", func(_ context.Context, params json.RawMessage) (any, error) {
		var c helper.Credentials
		if err := rpc.Decode(params, &c); err != nil {
			return nil, rpc.Errorf(rpc.CodeInvalidParams, "giriş bilgisi okunamadı")
		}
		if err := r.Helper.SetCredentials(c); err != nil {
			return nil, rpc.Errorf(rpc.CodeInvalidParams, "%v", err)
		}
		r.Kick()
		return r.Helper.LoginInfo(), nil
	})
	s.Handle("helper.clear_login", func(context.Context, json.RawMessage) (any, error) {
		r.Helper.ClearCredentials()
		return r.Helper.LoginInfo(), nil
	})
	// Ofis ayarı ve izni (sözleşme 1.3 §1).
	s.Handle("helper.office", func(ctx context.Context, _ json.RawMessage) (any, error) {
		r.RefreshOffice(ctx)
		return r.Status(), nil
	})
	s.Handle("helper.consent", func(ctx context.Context, _ json.RawMessage) (any, error) {
		if _, err := r.GiveConsent(ctx); err != nil {
			return nil, rpc.Errorf(rpc.CodeUnavailable, "Ofis izni kaydedilemedi: %v", err)
		}
		return r.Status(), nil
	})
	s.Handle("helper.withdraw_consent", func(ctx context.Context, _ json.RawMessage) (any, error) {
		if _, err := r.WithdrawConsent(ctx); err != nil {
			return nil, rpc.Errorf(rpc.CodeUnavailable, "Ofis izni geri alınamadı: %v", err)
		}
		return r.Status(), nil
	})
	// Aktarıcı bilgisayar: uygulama açılınca (pencere kapalıyken de)
	// yoklama kendiliğinden başlar.
	s.Handle("helper.set_always_on", func(_ context.Context, params json.RawMessage) (any, error) {
		var p struct {
			On bool `json:"on"`
		}
		if err := rpc.Decode(params, &p); err != nil {
			return nil, err
		}
		if err := r.SetAlwaysOn(p.On); err != nil {
			return nil, rpc.Errorf(rpc.CodeInternal, "ayar kaydedilemedi: %v", err)
		}
		return r.Status(), nil
	})
	s.Handle("helper.logout_luca", func(ctx context.Context, _ json.RawMessage) (any, error) {
		if err := r.Helper.LogoutLuca(ctx); err != nil {
			return nil, helperErr(err)
		}
		return r.Status(), nil
	})
}

// helperErr yardımcı hatasını arayüzün tanıdığı koda çevirir.
func helperErr(err error) error {
	codes := []struct {
		err  error
		code string
	}{
		{helper.ErrNoBrowser, "NO_BROWSER"},
		{helper.ErrNotOpen, "HELPER_CLOSED"},
		{helper.ErrBusy, "HELPER_BUSY"},
		{helper.ErrCompanyMismatch, helper.CodeFirmMismatch},
		{helper.ErrCompanyNotFound, helper.CodeFirmNotFound},
		{helper.ErrScreenChanged, helper.CodeScreen},
		{helper.ErrOutsideHost, helper.CodeDomain},
	}
	for _, c := range codes {
		if errors.Is(err, c.err) {
			return rpc.Errorf(c.code, "%v", err)
		}
	}
	return rpc.Errorf(rpc.CodeUnavailable, "%v", err)
}
