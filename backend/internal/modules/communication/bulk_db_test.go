package communication

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

type bulkFixture struct {
	svc   *Service
	d     *testdb.DB
	org   int64
	actor int64
	// engagement uuids by name, and internal ids.
	eng map[string]uuid.UUID
	ids map[string]int64
}

func newBulkFixture(t *testing.T) bulkFixture {
	t.Helper()
	ctx := context.Background()
	svc, d, org, _ := setup(t, false)
	f := bulkFixture{svc: svc, d: d, org: org, actor: d.User(t), eng: map[string]uuid.UUID{}, ids: map[string]int64{}}
	d.Member(t, org, f.actor, "owner")
	add := func(name, phone, email string) {
		tp := d.Taxpayer(t, "legal", fmt.Sprintf("77%08d", testdb.Next()))
		if _, err := d.Pool.Exec(ctx, `UPDATE taxpayers SET title = $2, contact_phone = NULLIF($3, ''), contact_email = NULLIF($4, '') WHERE id = $1`,
			tp, name, phone, email); err != nil {
			t.Fatal(err)
		}
		id := d.Engagement(t, org, tp)
		var u uuid.UUID
		if err := d.Pool.QueryRow(ctx, `SELECT uuid FROM engagements WHERE id = $1`, id).Scan(&u); err != nil {
			t.Fatal(err)
		}
		f.eng[name], f.ids[name] = u, id
	}
	add("Atlas", "0532 111 22 33", "atlas@example.test")
	add("Beta", "+90 532 111 22 33", "beta@example.test")
	add("Ceren", "", "")
	add("Deniz", "5320000004", "deniz@example.test")
	d.EndEngagement(t, f.ids["Deniz"])
	if _, err := svc.Save(ctx, org, 0, SettingsInput{Enabled: true, TestMode: true, Usercode: "8501234567", Password: ptr("p"), MsgHeader: "OFISADI"}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f bulkFixture) sel(names ...string) Target {
	t := Target{Kind: TargetSelection}
	for _, n := range names {
		t.EngagementUUIDs = append(t.EngagementUUIDs, f.eng[n])
	}
	return t
}

// Kabul (#780 B): seçili mükelleflere toplu SMS; aynı numara tek gönderim,
// iletişim bilgisi olmayan atlanır, gönderim arka planda biter ve her mesaj
// bulk_id ile kaydedilir (deneme modu).
func TestBulkSMSSelection(t *testing.T) {
	ctx := context.Background()
	f := newBulkFixture(t)
	in := BulkInput{Channel: ChannelSMS, Body: "Sayın :mukellef, :donem beyanınız hazır.", Target: f.sel("Atlas", "Beta", "Ceren", "Deniz")}
	pv, err := f.svc.Preview(ctx, f.org, f.actor, in)
	if err != nil {
		t.Fatal(err)
	}
	// Deniz'in ilişkisi bitti: hedefte yok.
	if pv.Targets != 3 || pv.Recipients != 1 || len(pv.Duplicates) != 1 || len(pv.Skipped) != 1 || !pv.TestMode || pv.Blocked != "" || pv.Sample == nil {
		t.Fatalf("preview = %+v", pv)
	}
	if !strings.HasPrefix(pv.Sample.Text, "Sayın Atlas, ") {
		t.Fatalf("sample = %q", pv.Sample.Text)
	}
	v, err := f.svc.StartBulk(ctx, f.org, f.actor, in)
	if err != nil || v.Status != BulkRunning || v.Total != 1 || v.Duplicates != 1 {
		t.Fatalf("start = %+v, %v", v, err)
	}
	f.svc.WaitBulk()
	got, err := f.svc.GetBulk(ctx, f.org, v.UUID)
	if err != nil || got.Status != BulkDone || got.Processed != 1 || got.Sent != 1 || got.Failed != 0 || len(got.Skipped) != 1 || got.Skipped[0].Title != "Ceren" {
		t.Fatalf("done = %+v, %v", got, err)
	}
	var n int
	var status string
	if err := f.d.Pool.QueryRow(ctx, `SELECT count(*), max(status) FROM sms_messages WHERE organization_id = $1 AND bulk_id = $2`, f.org, v.UUID).Scan(&n, &status); err != nil {
		t.Fatal(err)
	}
	if n != 1 || status != StatusDryRun {
		t.Fatalf("records = %d %s", n, status)
	}
	// Son gönderilenler: kullanıcının son gönderimi, hedef olarak yeniden.
	recent, err := f.svc.RecentBulk(ctx, f.org, f.actor)
	if err != nil || len(recent) != 1 || recent[0].UUID != v.UUID {
		t.Fatalf("recent = %+v, %v", recent, err)
	}
	pv, err = f.svc.Preview(ctx, f.org, f.actor, BulkInput{Channel: ChannelSMS, Body: "Tekrar", Target: Target{Kind: TargetRecent, BulkUUID: &v.UUID}})
	if err != nil || pv.Targets != 3 || !strings.HasPrefix(pv.TargetLabel, "Tekrar") {
		t.Fatalf("recent target = %+v, %v", pv, err)
	}
	// Başka kullanıcının gönderimi hedef olamaz.
	if _, err := f.svc.Preview(ctx, f.org, f.d.User(t), BulkInput{Channel: ChannelSMS, Body: "x", Target: Target{Kind: TargetRecent, BulkUUID: &v.UUID}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user's bulk: %v", err)
	}
}

// Kabul: kişiselleştirmeden sonra uzun SMS gönderimi durdurur, kayıt
// yazılmaz; 500'den fazla mükellef reddedilir; SMS kapalı ofis gönderemez.
func TestBulkRefusals(t *testing.T) {
	ctx := context.Background()
	f := newBulkFixture(t)
	long := BulkInput{Channel: ChannelSMS, Body: strings.Repeat("ş", 300), Target: f.sel("Atlas")}
	if _, err := f.svc.StartBulk(ctx, f.org, f.actor, long); !errors.Is(err, ErrBulkBlocked) {
		t.Fatalf("too long: %v", err)
	}
	if _, err := f.svc.StartBulk(ctx, f.org, f.actor, BulkInput{Channel: ChannelSMS, Body: "Bilgi", Target: f.sel("Ceren")}); !errors.Is(err, ErrNoRecipients) {
		t.Fatalf("no recipients: %v", err)
	}
	var n int
	if err := f.d.Pool.QueryRow(ctx, `SELECT count(*) FROM bulk_sends WHERE organization_id = $1`, f.org).Scan(&n); err != nil || n != 0 {
		t.Fatalf("bulk rows = %d, %v", n, err)
	}
	many := Target{Kind: TargetSelection}
	for i := 0; i <= MaxBulkRecipients; i++ {
		many.EngagementUUIDs = append(many.EngagementUUIDs, uuid.New())
	}
	if _, err := f.svc.Preview(ctx, f.org, f.actor, BulkInput{Channel: ChannelSMS, Body: "x", Target: many}); !errors.Is(err, ErrTooManyRecipients) {
		t.Fatalf("501: %v", err)
	}
	other := f.d.Organization(t)
	if _, err := f.svc.Preview(ctx, other, f.actor, BulkInput{Channel: ChannelSMS, Body: "x", Target: f.sel("Atlas")}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("not configured: %v", err)
	}
}

// Kabul: kayıtlı grup ve dinamik süzgeç (sorumlu çalışan) hedef olur; grup
// adı ofiste tekildir; bitmiş ilişki gruba girmez.
func TestGroupsAndFilters(t *testing.T) {
	ctx := context.Background()
	f := newBulkFixture(t)
	g, err := f.svc.CreateGroup(ctx, f.org, f.actor, GroupInput{Name: "KDV'ye hassas", EngagementUUIDs: []uuid.UUID{f.eng["Atlas"], f.eng["Ceren"], f.eng["Deniz"]}})
	if err != nil || g.MemberCount != 2 || len(g.Members) != 2 {
		t.Fatalf("create = %+v, %v", g, err)
	}
	if _, err := f.svc.CreateGroup(ctx, f.org, f.actor, GroupInput{Name: "kdv'ye HASSAS"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate name: %v", err)
	}
	g, err = f.svc.UpdateGroup(ctx, f.org, g.UUID, GroupInput{Name: "KDV'ye hassas", Description: "Takip", EngagementUUIDs: []uuid.UUID{f.eng["Beta"]}})
	if err != nil || g.MemberCount != 1 || g.Members[0].Title != "Beta" {
		t.Fatalf("update = %+v, %v", g, err)
	}
	list, err := f.svc.ListGroups(ctx, f.org)
	if err != nil || len(list) != 1 || list[0].MemberCount != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	v, err := f.svc.StartBulk(ctx, f.org, f.actor, BulkInput{Channel: ChannelMail, Subject: ":ofis bilgilendirme", Body: "Sayın :mukellef",
		Target: Target{Kind: TargetGroup, GroupUUID: &g.UUID}})
	if err != nil || v.TargetLabel != "Grup: KDV'ye hassas" {
		t.Fatalf("group send = %+v, %v", v, err)
	}
	f.svc.WaitBulk()
	got, err := f.svc.GetBulk(ctx, f.org, v.UUID)
	if err != nil || got.Status != BulkDone || got.Sent != 1 {
		t.Fatalf("group done = %+v, %v", got, err)
	}
	var to string
	if err := f.d.Pool.QueryRow(ctx, `SELECT to_email::text FROM mail_messages WHERE organization_id = $1 AND bulk_id = $2`, f.org, v.UUID).Scan(&to); err != nil || to != "beta@example.test" {
		t.Fatalf("mail record = %q, %v", to, err)
	}

	// Sorumlu çalışan süzgeci.
	f.d.Assign(t, f.org, f.ids["Atlas"], f.actor)
	opts, err := f.svc.Filters(ctx, f.org)
	if err != nil || len(opts.Assignees) != 1 || opts.Assignees[0].Count != 1 || len(opts.TaxKinds) != 2 {
		t.Fatalf("filters = %+v, %v", opts, err)
	}
	pv, err := f.svc.Preview(ctx, f.org, f.actor, BulkInput{Channel: ChannelSMS, Body: "Bilgi",
		Target: Target{Kind: TargetFilter, AssigneeUUID: &opts.Assignees[0].UUID, Label: "Sorumlu: Test"}})
	if err != nil || pv.Targets != 1 || pv.Recipients != 1 || pv.TargetLabel != "Sorumlu: Test" {
		t.Fatalf("filter preview = %+v, %v", pv, err)
	}
	if _, err := f.svc.Preview(ctx, f.org, f.actor, BulkInput{Channel: ChannelSMS, Body: "x", Target: Target{Kind: TargetFilter, TaxKind: "x"}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad tax kind: %v", err)
	}
	if err := f.svc.DeleteGroup(ctx, f.org, g.UUID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.DeleteGroup(ctx, f.org, g.UUID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}
