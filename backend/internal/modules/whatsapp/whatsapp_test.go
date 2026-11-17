package whatsapp

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/model"
	authusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/usecase"
)

func TestNormalizeChatKey(t *testing.T) {
	for in, want := range map[string]string{
		"905551112233@c.us":           "905551112233",
		"905551112233@s.whatsapp.net": "905551112233",
		"+90 (555) 111-22-33":         "905551112233",
		"120363041234567890@g.us":     "g:120363041234567890",
		"g:120363041234567890":        "g:120363041234567890",
		"98765432101234@lid":          "l:98765432101234",
	} {
		got, ok := NormalizeChatKey(in)
		if !ok || got != want {
			t.Errorf("NormalizeChatKey(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "merhaba", "12345", "9055a5112233", "g:ab", "x@g.us", "1234567890123456789012"} {
		if got, ok := NormalizeChatKey(in); ok {
			t.Errorf("NormalizeChatKey(%q) = %q, want rejected", in, got)
		}
	}
}

func TestNormalizeCode(t *testing.T) {
	if got, ok := NormalizeCode(" k7qm-3xpa "); !ok || got != "K7QM3XPA" {
		t.Fatalf("got %q %v", got, ok)
	}
	for _, in := range []string{"K7QM3XP", "K7QM3XPAA", "K7QM-3XP0", "K7QM-3XPI", "K7QM;3XPA"} {
		if _, ok := NormalizeCode(in); ok {
			t.Errorf("NormalizeCode(%q) accepted", in)
		}
	}
}

type fakeAccounts struct {
	calls int
	meta  model.SessionMeta
	err   error
}

func (f *fakeAccounts) IssueQRLoginSession(_ context.Context, id uuid.UUID, meta model.SessionMeta) (model.Tokens, authusecase.QRLoginUser, error) {
	f.calls++
	f.meta = meta
	if f.err != nil {
		return model.Tokens{}, authusecase.QRLoginUser{}, f.err
	}
	return model.Tokens{AccessToken: "a", RefreshToken: "r"}, authusecase.QRLoginUser{UUID: id, Email: "u@example.test", Name: "U"}, nil
}

func TestPairingIsOneTime(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	acc := &fakeAccounts{}
	p := NewPairings(rdb, "test", acc)
	ctx := context.Background()
	user := uuid.New()

	if _, err := p.Create(ctx, user, true, Office{Slug: "ofis"}); !errors.Is(err, ErrImpersonating) {
		t.Fatalf("impersonating: %v", err)
	}
	pr, err := p.Create(ctx, user, false, Office{UUID: uuid.NewString(), Slug: "ofis", Name: "Ofis"})
	if err != nil {
		t.Fatal(err)
	}
	if len(mr.Keys()) != 1 || mr.TTL(mr.Keys()[0]) != PairingTTL {
		t.Fatalf("keys %v ttl %v", mr.Keys(), mr.TTL(mr.Keys()[0]))
	}
	for _, k := range mr.Keys() {
		if code, _ := NormalizeCode(pr.Code); k == code || len(k) < 40 {
			t.Fatalf("raw code in key %q", k)
		}
	}
	out, err := p.Redeem(ctx, pr.Code, "Chrome\x00 · Ofis", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if out.Organization.Slug != "ofis" || out.User.UUID != user.String() || out.AccessToken != "a" {
		t.Fatalf("paired = %+v", out)
	}
	if acc.meta.UserAgent != SessionUserAgent+" · Chrome · Ofis" || acc.meta.IP != "10.0.0.1" {
		t.Fatalf("meta = %+v", acc.meta)
	}
	if _, err := p.Redeem(ctx, pr.Code, "", ""); !errors.Is(err, ErrPairingNotFound) {
		t.Fatalf("second redeem: %v", err)
	}
	if _, err := p.Redeem(ctx, "nope", "", ""); !errors.Is(err, ErrPairingNotFound) {
		t.Fatalf("malformed: %v", err)
	}
	pr2, _ := p.Create(ctx, user, false, Office{Slug: "ofis"})
	mr.FastForward(PairingTTL + 1)
	if _, err := p.Redeem(ctx, pr2.Code, "", ""); !errors.Is(err, ErrPairingNotFound) {
		t.Fatalf("expired: %v", err)
	}
	if acc.calls != 1 {
		t.Fatalf("sessions issued: %d", acc.calls)
	}
}

func TestSniffAndNames(t *testing.T) {
	if ct, kind, ext, ok := sniff([]byte("%PDF-1.7\n")); !ok || ct != "application/pdf" || kind != "pdf" || ext != ".pdf" {
		t.Fatalf("pdf: %s %s %s %v", ct, kind, ext, ok)
	}
	if _, _, _, ok := sniff([]byte("merhaba")); ok {
		t.Fatal("text accepted")
	}
	if got := cleanName(`C:\fake\..\fiş.jpeg`, ".jpg"); got != "fiş.jpeg.jpg" {
		t.Fatalf("cleanName = %q", got)
	}
	if got := cleanName("", ".pdf"); got != "whatsapp.pdf" {
		t.Fatalf("cleanName empty = %q", got)
	}
}
