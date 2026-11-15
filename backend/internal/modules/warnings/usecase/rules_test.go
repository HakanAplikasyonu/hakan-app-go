package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
)

type fakeStore struct {
	office, eng map[string]engine.Layer
	upserts     int
}

func (f *fakeStore) Layers(_ context.Context, _, engID int64, codes []string) ([]repository.CodeLayers, error) {
	if len(codes) == 0 {
		for _, c := range catalog.Codes {
			codes = append(codes, c.Code)
		}
	}
	w := catalog.SeverityWarning
	var out []repository.CodeLayers
	for _, c := range codes {
		l := repository.CodeLayers{Code: c, Section: "3.1", Default: engine.Layer{Severity: &w, Params: json.RawMessage(`{"tolerance":"0.05"}`)}}
		if o, ok := f.office[c]; ok {
			l.Office = &o
		}
		if e, ok := f.eng[c]; ok && engID > 0 {
			l.Engagement = &e
		}
		out = append(out, l)
	}
	return out, nil
}

func (f *fakeStore) Upsert(_ context.Context, _, engID int64, code string, l engine.Layer, _ int64) error {
	f.upserts++
	if engID > 0 {
		f.eng[code] = l
	} else {
		f.office[code] = l
	}
	return nil
}

func (f *fakeStore) Delete(_ context.Context, _, engID int64, code string) (bool, error) {
	m := f.office
	if engID > 0 {
		m = f.eng
	}
	_, ok := m[code]
	delete(m, code)
	return ok, nil
}

func newRules() (*Rules, *fakeStore) {
	f := &fakeStore{office: map[string]engine.Layer{}, eng: map[string]engine.Layer{}}
	return NewRules(f), f
}

func strp(s string) *string { return &s }

func TestSetValidates(t *testing.T) {
	r, f := newRules()
	ctx := context.Background()
	bad := []SetInput{
		{Severity: strp("critical")},
		{Params: map[string]any{"unknown": "1"}},
		{Params: map[string]any{"tolerance": "abc"}},
		{Params: map[string]any{"tolerance": true}},
	}
	for _, in := range bad {
		if _, err := r.Set(ctx, 1, 0, "TAX_TOTAL_MISMATCH", in, 1); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: err = %v", in, err)
		}
	}
	if _, err := r.Set(ctx, 1, 0, "NOPE", SetInput{}, 1); !errors.Is(err, ErrUnknownCode) {
		t.Errorf("unknown code err = %v", err)
	}
	if f.upserts != 0 {
		t.Fatal("invalid input reached the store")
	}
}

func TestSetAndListEffective(t *testing.T) {
	r, _ := newRules()
	ctx := context.Background()
	v, err := r.Set(ctx, 1, 0, "TAX_TOTAL_MISMATCH", SetInput{Severity: strp("blocker"), Params: map[string]any{"tolerance": "0.10"}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v.Effective.Severity != "blocker" || v.Effective.Params["tolerance"] != "0.10" || v.Office == nil || v.Engagement != nil {
		t.Fatalf("office view = %+v", v)
	}
	off := false
	v, err = r.Set(ctx, 1, 9, "TAX_TOTAL_MISMATCH", SetInput{Enabled: &off, Params: map[string]any{"tolerance": nil}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v.Effective.Enabled || v.Effective.Severity != "blocker" || v.Effective.Params["tolerance"] != nil || v.Engagement == nil {
		t.Fatalf("engagement view = %+v", v)
	}
	items, _ := r.List(ctx, 1, 0)
	if len(items) != len(catalog.Codes) {
		t.Fatalf("items = %d", len(items))
	}
	for i := 1; i < len(items); i++ {
		if items[i-1].Code > items[i].Code {
			t.Fatal("list not sorted")
		}
	}
	if err := r.Reset(ctx, 1, 9, "TAX_TOTAL_MISMATCH"); err != nil {
		t.Fatal(err)
	}
	if err := r.Reset(ctx, 1, 9, "TAX_TOTAL_MISMATCH"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second reset err = %v", err)
	}
}
