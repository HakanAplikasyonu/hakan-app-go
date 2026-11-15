package engagements

import (
	"context"
	"errors"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database/db"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/orgctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeStore keeps engagements and assignments in memory and enforces the
// one-active-engagement-per-taxpayer rule like uq_engagements_active_taxpayer.
type fakeStore struct {
	taxpayers   map[uuid.UUID]db.Taxpayer
	engagements []db.Engagement
	members     map[int64]map[uuid.UUID]int64 // org -> user uuid -> user id
	assignments map[[2]int64]bool             // (engagement id, user id)
	nextID      int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		taxpayers:   map[uuid.UUID]db.Taxpayer{},
		members:     map[int64]map[uuid.UUID]int64{},
		assignments: map[[2]int64]bool{},
	}
}

func (f *fakeStore) addTaxpayer() db.Taxpayer {
	f.nextID++
	t := db.Taxpayer{ID: f.nextID, Uuid: uuid.New()}
	f.taxpayers[t.Uuid] = t
	return t
}

func (f *fakeStore) addMember(orgID, userID int64) uuid.UUID {
	if f.members[orgID] == nil {
		f.members[orgID] = map[uuid.UUID]int64{}
	}
	u := uuid.New()
	f.members[orgID][u] = userID
	return u
}

func (f *fakeStore) GetTaxpayerByUUID(_ context.Context, id uuid.UUID) (db.Taxpayer, error) {
	t, ok := f.taxpayers[id]
	if !ok {
		return db.Taxpayer{}, errNoRows
	}
	return t, nil
}

func (f *fakeStore) CreateEngagement(_ context.Context, orgID, taxpayerID int64) (db.Engagement, error) {
	for _, e := range f.engagements {
		if e.TaxpayerID == taxpayerID && !e.EndedAt.Valid {
			return db.Engagement{}, errActiveExists
		}
	}
	f.nextID++
	e := db.Engagement{ID: f.nextID, Uuid: uuid.New(), OrganizationID: orgID, TaxpayerID: taxpayerID, Status: "active"}
	f.engagements = append(f.engagements, e)
	return e, nil
}

func (f *fakeStore) find(orgID int64, id uuid.UUID) (int, bool) {
	for i, e := range f.engagements {
		if e.OrganizationID == orgID && e.Uuid == id {
			return i, true
		}
	}
	return 0, false
}

func (f *fakeStore) GetEngagement(_ context.Context, orgID int64, id uuid.UUID) (db.GetEngagementByUUIDRow, error) {
	i, ok := f.find(orgID, id)
	if !ok {
		return db.GetEngagementByUUIDRow{}, errNoRows
	}
	e := f.engagements[i]
	return db.GetEngagementByUUIDRow{ID: e.ID, Uuid: e.Uuid, OrganizationID: e.OrganizationID, TaxpayerID: e.TaxpayerID, Status: e.Status, EndedAt: e.EndedAt}, nil
}

func (f *fakeStore) ListEngagements(_ context.Context, orgID int64, _ pgtype.Text, _, _ int32) ([]db.ListEngagementsByOrgRow, int64, error) {
	var out []db.ListEngagementsByOrgRow
	for _, e := range f.engagements {
		if e.OrganizationID == orgID {
			out = append(out, db.ListEngagementsByOrgRow{ID: e.ID, Uuid: e.Uuid, Status: e.Status})
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeStore) ListAssignedEngagements(_ context.Context, orgID, userID int64, _ pgtype.Text, _, _ int32) ([]db.ListAssignedEngagementsRow, int64, error) {
	var out []db.ListAssignedEngagementsRow
	for _, e := range f.engagements {
		if e.OrganizationID == orgID && f.assignments[[2]int64{e.ID, userID}] {
			out = append(out, db.ListAssignedEngagementsRow{ID: e.ID, Uuid: e.Uuid, Status: e.Status})
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeStore) EndEngagement(_ context.Context, orgID int64, id uuid.UUID) (db.Engagement, error) {
	i, ok := f.find(orgID, id)
	if !ok || f.engagements[i].EndedAt.Valid {
		return db.Engagement{}, errNoRows
	}
	f.engagements[i].Status = "ended"
	f.engagements[i].EndedAt = pgtype.Timestamptz{Valid: true}
	return f.engagements[i], nil
}

func (f *fakeStore) ResolveEngagementAccess(_ context.Context, orgID int64, id uuid.UUID, userID int64) (db.ResolveEngagementAccessRow, error) {
	i, ok := f.find(orgID, id)
	if !ok {
		return db.ResolveEngagementAccessRow{}, errNoRows
	}
	e := f.engagements[i]
	return db.ResolveEngagementAccessRow{
		ID: e.ID, Uuid: e.Uuid, OrganizationID: e.OrganizationID, Status: e.Status,
		Assigned: f.assignments[[2]int64{e.ID, userID}],
	}, nil
}

func (f *fakeStore) GetMemberUserID(_ context.Context, orgID int64, userUUID uuid.UUID) (int64, error) {
	id, ok := f.members[orgID][userUUID]
	if !ok {
		return 0, errNoRows
	}
	return id, nil
}

func (f *fakeStore) AddAssignment(_ context.Context, _, engagementID, userID int64) error {
	f.assignments[[2]int64{engagementID, userID}] = true
	return nil
}

func (f *fakeStore) DeleteAssignment(_ context.Context, _, engagementID, userID int64) (int64, error) {
	k := [2]int64{engagementID, userID}
	if !f.assignments[k] {
		return 0, nil
	}
	delete(f.assignments, k)
	return 1, nil
}

func (f *fakeStore) ListAssignments(context.Context, int64, int64) ([]db.ListEngagementAssignmentsRow, error) {
	return nil, nil
}

type countingInvalidator struct{ orgs []int64 }

func (c *countingInvalidator) InvalidateOrganization(_ context.Context, orgID int64) error {
	c.orgs = append(c.orgs, orgID)
	return nil
}

func TestCreateRejectsSecondActiveEngagement(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	svc := New(st, nil)
	tp := st.addTaxpayer()

	first, err := svc.Create(ctx, 1, tp.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	// Same office and another office: both refused while one is active (D1).
	for _, org := range []int64{1, 2} {
		if _, err := svc.Create(ctx, org, tp.Uuid); !errors.Is(err, ErrAlreadyActive) {
			t.Fatalf("org %d: err = %v, want ErrAlreadyActive", org, err)
		}
	}
	if _, err := svc.End(ctx, 1, first.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, 2, tp.Uuid); err != nil {
		t.Fatalf("after ending, a new engagement must open: %v", err)
	}
}

func TestCreateUnknownTaxpayer(t *testing.T) {
	svc := New(newFakeStore(), nil)
	if _, err := svc.Create(context.Background(), 1, uuid.New()); !errors.Is(err, ErrTaxpayerNotFound) {
		t.Fatalf("err = %v", err)
	}
	if _, err := svc.Create(context.Background(), 1, uuid.Nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v", err)
	}
}

func TestAssignUnassignInvalidatesAndChangesAccess(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	inv := &countingInvalidator{}
	svc := New(st, inv)
	tp := st.addTaxpayer()
	e, err := svc.Create(ctx, 1, tp.Uuid)
	if err != nil {
		t.Fatal(err)
	}
	ayse := st.addMember(1, 42)
	access, _ := svc.ResolveEngagement(ctx, 1, e.UUID, 42)
	if access.Assigned {
		t.Fatal("not assigned yet")
	}

	if err := svc.Assign(ctx, 1, access.ID, ayse); err != nil {
		t.Fatal(err)
	}
	if access, _ = svc.ResolveEngagement(ctx, 1, e.UUID, 42); !access.Assigned {
		t.Fatal("assignment not visible")
	}
	if err := svc.Unassign(ctx, 1, access.ID, ayse); err != nil {
		t.Fatal(err)
	}
	if access, _ = svc.ResolveEngagement(ctx, 1, e.UUID, 42); access.Assigned {
		t.Fatal("unassignment not visible")
	}
	if len(inv.orgs) != 2 || inv.orgs[0] != 1 || inv.orgs[1] != 1 {
		t.Fatalf("invalidations = %v, want org 1 twice", inv.orgs)
	}
	if err := svc.Unassign(ctx, 1, access.ID, ayse); !errors.Is(err, ErrAssignmentNotFound) {
		t.Fatalf("second unassign err = %v", err)
	}
	if len(inv.orgs) != 2 {
		t.Fatal("a no-op unassign must not invalidate")
	}
}

func TestAssignRejectsNonMember(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	inv := &countingInvalidator{}
	svc := New(st, inv)
	tp := st.addTaxpayer()
	e, _ := svc.Create(ctx, 1, tp.Uuid)
	other := st.addMember(2, 99) // member of another office
	access, _ := svc.ResolveEngagement(ctx, 1, e.UUID, 99)
	if err := svc.Assign(ctx, 1, access.ID, other); !errors.Is(err, ErrMemberNotFound) {
		t.Fatalf("err = %v", err)
	}
	if len(inv.orgs) != 0 {
		t.Fatal("failed assign must not invalidate")
	}
}

func TestEndInvalidates(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	inv := &countingInvalidator{}
	svc := New(st, inv)
	e, _ := svc.Create(ctx, 1, st.addTaxpayer().Uuid)
	out, err := svc.End(ctx, 1, e.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != engagementctx.StatusEnded {
		t.Fatalf("status = %s", out.Status)
	}
	if len(inv.orgs) != 1 {
		t.Fatalf("invalidations = %v", inv.orgs)
	}
	if _, err := svc.End(ctx, 1, e.UUID); !errors.Is(err, ErrEnded) {
		t.Fatalf("second end err = %v", err)
	}
	if _, err := svc.End(ctx, 2, e.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other office end err = %v", err)
	}
}

func TestListRespectsReadAll(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	svc := New(st, nil)
	e1, _ := svc.Create(ctx, 1, st.addTaxpayer().Uuid)
	_, _ = svc.Create(ctx, 1, st.addTaxpayer().Uuid)
	_, _ = svc.Create(ctx, 2, st.addTaxpayer().Uuid) // another office
	u := st.addMember(1, 42)
	a, _ := svc.ResolveEngagement(ctx, 1, e1.UUID, 42)
	_ = svc.Assign(ctx, 1, a.ID, u)

	member := orgctx.Scope{ID: 1, Permissions: []string{rbac.PermTenantEngagementsRead}}
	items, total, err := svc.List(ctx, member, 42, "", 50, 0)
	if err != nil || total != 1 || len(items) != 1 || items[0].UUID != e1.UUID {
		t.Fatalf("member list = %v %d %v", items, total, err)
	}
	owner := orgctx.Scope{ID: 1, Permissions: []string{rbac.PermTenantEngagementsRead, rbac.PermTenantEngagementsReadAll}}
	if _, total, _ := svc.List(ctx, owner, 7, "", 50, 0); total != 2 {
		t.Fatalf("read_all list total = %d, want 2 (own office only)", total)
	}
	if _, _, err := svc.List(ctx, owner, 7, "bogus", 50, 0); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("bad status err = %v", err)
	}
}
