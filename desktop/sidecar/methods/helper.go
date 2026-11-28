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
