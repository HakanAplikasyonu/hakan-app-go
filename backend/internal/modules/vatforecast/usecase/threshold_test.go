package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/catalog"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/engine"
	warningsrepository "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/repository"
)

type fakeThresholds struct {
	layers []warningsrepository.CodeLayers
	saved  *engine.Layer
	org    int64
	eng    int64
	code   string
}

func (f *fakeThresholds) Layers(context.Context, int64, int64, []string) ([]warningsrepository.CodeLayers, error) {
	return f.layers, nil
}

func (f *fakeThresholds) Upsert(_ context.Context, org, eng int64, code string, l engine.Layer, _ int64) error {
	f.org, f.eng, f.code, f.saved = org, eng, code, &l
	return nil
}

func params(t *testing.T, l engine.Layer) map[string]any {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(l.Params, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

// The threshold goes to the office layer of VAT_PAYABLE_THRESHOLD; the
// layer's severity is kept, a switched-off code comes back on.
func TestSavePayableLimit(t *testing.T) {
	sev := catalog.Severity("blocker")
	off := false
	ts := &fakeThresholds{layers: []warningsrepository.CodeLayers{{
		Code:   CodePayableThreshold,
		Office: &engine.Layer{Severity: &sev, Enabled: &off, Params: json.RawMessage(`{"limit":"5000.00"}`)},
	}}}
	svc := New(&fakeStore{}, nil, nil).WithThresholds(ts)
	limit := d("10000")
	if err := svc.SavePayableLimit(context.Background(), 7, 1, &limit); err != nil {
		t.Fatal(err)
	}
	if ts.org != 7 || ts.eng != 0 || ts.code != CodePayableThreshold || ts.saved == nil {
		t.Fatalf("upsert = %d/%d/%s", ts.org, ts.eng, ts.code)
	}
	if p := params(t, *ts.saved); p["limit"] != "10000.00" {
		t.Errorf("params = %v", p)
	}
	if ts.saved.Severity == nil || *ts.saved.Severity != sev || ts.saved.Enabled == nil || !*ts.saved.Enabled {
		t.Errorf("layer = %+v", ts.saved)
	}
	if err := svc.SavePayableLimit(context.Background(), 7, 1, nil); err != nil {
		t.Fatal(err)
	}
	if p := params(t, *ts.saved); p["limit"] != nil {
		t.Errorf("cleared params = %v", p)
	}
	for _, bad := range []string{"0", "-5", "1000000000000000"} {
		v := d(bad)
		if err := svc.SavePayableLimit(context.Background(), 7, 1, &v); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v", bad, err)
		}
	}
}

func TestPayableLimitLayerWithoutOfficeRow(t *testing.T) {
	limit := d("2500.5")
	l, err := PayableLimitLayer(nil, &limit)
	if err != nil {
		t.Fatal(err)
	}
	if p := params(t, l); p["limit"] != "2500.50" || l.Severity != nil {
		t.Errorf("layer = %+v params %v", l, p)
	}
}
