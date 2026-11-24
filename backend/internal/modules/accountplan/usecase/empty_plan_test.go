package usecase

import (
	"context"
	"testing"
)

// planStore is a Store with only the calls the plan gate makes; the
// embedded nil interface panics on anything else.
type planStore struct {
	Store
	imp   Import
	codes []string
}

func (p planStore) Active(context.Context, Subject, string) (Import, error) { return p.imp, nil }
func (p planStore) Codes(context.Context, Subject, string) ([]string, error) {
	return p.codes, nil
}

func (p planStore) Find(_ context.Context, _ Subject, _ string, codes []string) ([]Found, error) {
	have := map[string]bool{}
	for _, c := range p.codes {
		have[c] = true
	}
	var out []Found
	for _, c := range codes {
		if have[c] {
			out = append(out, Found{Code: c, IsLeaf: true})
		}
	}
	return out, nil
}

// A plan import with no accounts (the demo seed's empty Luca plan) is no
// plan: the export is not blocked with every code "not in plan".
func TestEmptyPlanIsNoPlan(t *testing.T) {
	ctx := context.Background()
	empty := New(planStore{imp: Import{ID: 1, AccountCount: 0}})
	res, err := empty.Check(ctx, Subject{}, "", []string{"600.01", "191.20"})
	if err != nil || res.PlanLoaded || len(res.Missing) != 0 {
		t.Fatalf("empty plan check: %+v %v", res, err)
	}
	if set, err := empty.CodeSet(ctx, Subject{}, ""); err != nil || set != nil {
		t.Fatalf("empty plan code set: %v %v", set, err)
	}

	full := New(planStore{imp: Import{ID: 2, AccountCount: 1}, codes: []string{"600.01"}})
	res, err = full.Check(ctx, Subject{}, "", []string{"600.01", "191.20"})
	if err != nil || !res.PlanLoaded || len(res.Missing) != 1 || res.Missing[0] != "191.20" {
		t.Fatalf("full plan check: %+v %v", res, err)
	}
	// A plan whose count was not stored but has codes is still a plan.
	uncounted := New(planStore{imp: Import{ID: 3}, codes: []string{"600.01"}})
	if set, err := uncounted.CodeSet(ctx, Subject{}, ""); err != nil || !set["600.01"] {
		t.Fatalf("uncounted plan code set: %v %v", set, err)
	}
}
