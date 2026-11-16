package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
)

// Migration'daki ilk sürümler: dört tür yayımlanmış, hepsi hukuk onayı
// bekleyen taslak olarak işaretli, içerik özeti metinle tutarlı.
func TestSeededDocuments(t *testing.T) {
	d := testdb.Open(t)
	svc := NewPostgres(d.Pool)
	ctx := context.Background()
	docs, err := svc.ListCurrent(ctx, "tr")
	if err != nil || len(docs) != 4 {
		t.Fatalf("liste: %d %v", len(docs), err)
	}
	for _, doc := range docs {
		if doc.Version != 1 || doc.Status != "published" || !doc.LegalReviewPending {
			t.Errorf("%s: %+v", doc.Kind, doc)
		}
		if doc.ContentSHA256 != ContentHash(doc.Title, doc.Body) {
			t.Errorf("%s: içerik özeti tutmuyor", doc.Kind)
		}
	}
	// İngilizce yoksa bağlayıcı Türkçe metin döner.
	en, err := svc.Current(ctx, KindPrivacyNotice, "en-US")
	if err != nil || en.Language != "tr" || !strings.Contains(en.Body, "6698") {
		t.Fatalf("dil geri dönüşü: %+v %v", en.Language, err)
	}
	if _, err := svc.Current(ctx, "kvkk", "tr"); !errors.Is(err, ErrNotFound) {
		t.Errorf("bilinmeyen tür: %v", err)
	}
}

func TestVersioningAcceptanceAndDPA(t *testing.T) {
	d := testdb.Open(t)
	svc := NewPostgres(d.Pool)
	ctx := context.Background()
	org, user, admin := d.Organization(t), d.User(t), d.User(t)

	st, err := svc.DPA(ctx, org, "tr")
	if err != nil || st.Accepted != nil || st.UpToDate {
		t.Fatalf("başlangıç: %+v %v", st, err)
	}
	if _, err := svc.Accept(ctx, AcceptInput{Kind: KindDPA, Version: 1, UserID: user, Context: ContextOfficeDPA, OrganizationID: org, IP: "10.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	st, _ = svc.DPA(ctx, org, "tr")
	if st.Accepted == nil || st.Accepted.Version != 1 || !st.UpToDate {
		t.Fatalf("kabul sonrası: %+v", st)
	}

	// Yeni sürüm: taslak görünmez; yayımlanınca eski emekliye ayrılır ve
	// ofisin onayı güncel sayılmaz.
	draft, err := svc.CreateDraft(ctx, admin, DraftInput{Kind: KindDPA, Language: "tr", Title: "Veri İşleyen Sözleşmesi", Body: "v2 metni"})
	if err != nil || draft.Version != 2 || draft.Status != "draft" {
		t.Fatalf("taslak: %+v %v", draft, err)
	}
	if cur, _ := svc.Current(ctx, KindDPA, "tr"); cur.Version != 1 {
		t.Fatalf("taslak yayımlanmadan güncel olmamalı: %d", cur.Version)
	}
	if _, err := svc.Version(ctx, KindDPA, "tr", 2); !errors.Is(err, ErrNotFound) {
		t.Errorf("taslak sürüm herkese açık okunmamalı: %v", err)
	}
	pub, err := svc.Publish(ctx, draft.UUID, true)
	if err != nil || pub.Status != "published" || pub.LegalReviewPending || pub.PublishedAt == nil {
		t.Fatalf("yayım: %+v %v", pub, err)
	}
	if _, err := svc.Publish(ctx, draft.UUID, true); !errors.Is(err, ErrState) {
		t.Errorf("ikinci yayım: %v", err)
	}
	if _, err := svc.Publish(ctx, uuid.New(), true); !errors.Is(err, ErrNotFound) {
		t.Errorf("olmayan belge: %v", err)
	}
	old, err := svc.Version(ctx, KindDPA, "tr", 1)
	if err != nil || old.Status != "retired" {
		t.Fatalf("eski sürüm okunabilir ve emekli olmalı: %+v %v", old, err)
	}
	st, _ = svc.DPA(ctx, org, "tr")
	if st.Document.Version != 2 || st.Accepted.Version != 1 || st.UpToDate {
		t.Fatalf("yeni sürüm sonrası: %+v", st)
	}

	// Eski sürüme onay reddedilir; güncel sürüm kabul edilir.
	if _, err := svc.Accept(ctx, AcceptInput{Kind: KindDPA, Version: 1, UserID: user, Context: ContextOfficeDPA, OrganizationID: org}); !errors.Is(err, ErrOutdated) {
		t.Fatalf("eski sürüm onayı: %v", err)
	}
	if _, err := svc.Accept(ctx, AcceptInput{Kind: KindDPA, Version: 2, UserID: user, Context: ContextOfficeDPA, OrganizationID: org}); err != nil {
		t.Fatal(err)
	}
	if st, _ = svc.DPA(ctx, org, "tr"); !st.UpToDate {
		t.Fatalf("güncel onay: %+v", st)
	}

	// Yayımlanmış metin değiştirilemez (veritabanı tetikleyicisi).
	if _, err := d.Pool.Exec(ctx, `UPDATE legal_documents SET body = 'değişti' WHERE uuid = $1`, pub.UUID); err == nil {
		t.Fatal("yayımlanmış metin değiştirilebildi")
	}

	// Kullanıcının kendi onay geçmişi; geri alma.
	subj := uuid.New()
	if _, err := svc.Accept(ctx, AcceptInput{Kind: KindHistoryConsent, Version: 1, UserID: user, Context: ContextHistory, SubjectUUID: &subj}); err != nil {
		t.Fatal(err)
	}
	if err := svc.WithdrawWith(ctx, svc.q, subj); err != nil {
		t.Fatal(err)
	}
	mine, err := svc.Mine(ctx, user)
	if err != nil || len(mine) != 3 {
		t.Fatalf("onay geçmişi: %d %v", len(mine), err)
	}
	var withdrawn, withOrg int
	for _, m := range mine {
		if m.WithdrawnAt != nil {
			withdrawn++
		}
		if m.Organization != nil {
			withOrg++
		}
	}
	if withdrawn != 1 || withOrg != 2 {
		t.Errorf("geri alınan %d, ofisli %d", withdrawn, withOrg)
	}

	if _, err := svc.CreateDraft(ctx, admin, DraftInput{Kind: KindDPA, Language: "de", Title: "x", Body: "y"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("bilinmeyen dil: %v", err)
	}
}
