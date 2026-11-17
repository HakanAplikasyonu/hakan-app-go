// Package whatsapp serves the WhatsApp Web extension (issue #388): device
// pairing with the office user's session, the chat → taxpayer mapping and
// the upload endpoint that sends one clicked file to receipt capture (gider /
// gelir, AI okuma → taslak) or to the document tasks (belge).
//
// Pairing reuses the session model of QR sign-in (qrlogin): the signed-in
// office user creates a short-lived one-time code in the panel, the
// extension trades it for a normal session (access + refresh token). The
// session shows up in the user's device list (/v1/auth/sessions) and is
// revoked there like any other; no extension-specific secret is kept.
package whatsapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/model"
	authusecase "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/auth/usecase"
)

const (
	// PairingTTL is how long a pairing code is valid.
	PairingTTL = 10 * time.Minute
	// codeLen is the code length without the separator (31^8 ≈ 8.5e11).
	codeLen = 8
	// codeAlphabet leaves out 0/O, 1/I/L: the code is read off the screen.
	codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	// maxDeviceName bounds the name the extension sends.
	maxDeviceName = 60
	// SessionUserAgent prefixes the session's device label.
	SessionUserAgent = "HAKAN WhatsApp Web uzantısı"
)

var (
	// ErrPairingNotFound covers unknown, expired and already used codes.
	ErrPairingNotFound = errors.New("whatsapp: pairing code not found")
	// ErrPairingDisabled: no Redis configured.
	ErrPairingDisabled = errors.New("whatsapp: pairing is not configured")
	// ErrImpersonating: an impersonated session cannot hand out a session.
	ErrImpersonating = errors.New("whatsapp: stop impersonating before pairing")
)

// Accounts is the slice of the auth use case pairing needs (the same one
// QR sign-in uses: issue a session for a user who proved it elsewhere).
type Accounts interface {
	IssueQRLoginSession(ctx context.Context, userUUID uuid.UUID, meta model.SessionMeta) (model.Tokens, authusecase.QRLoginUser, error)
}

// Pairings creates and redeems pairing codes. State lives in Redis only.
type Pairings struct {
	rdb      *redis.Client
	env      string
	accounts Accounts
	now      func() time.Time
}

// NewPairings builds the pairing service. rdb nil disables pairing.
func NewPairings(rdb *redis.Client, appEnv string, accounts Accounts) *Pairings {
	if appEnv == "" {
		appEnv = "dev"
	}
	return &Pairings{rdb: rdb, env: appEnv, accounts: accounts, now: time.Now}
}

// Pairing is the code shown in the panel.
type Pairing struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
	ExpiresIn int64     `json:"expires_in"`
}

type pending struct {
	UserUUID string `json:"user_uuid"`
	OrgSlug  string `json:"org_slug"`
	OrgName  string `json:"org_name"`
	OrgUUID  string `json:"org_uuid"`
}

// Office is the office the paired extension works for.
type Office struct {
	UUID string `json:"uuid"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// PairedUser identifies the signed-in account.
type PairedUser struct {
	UUID  string `json:"uuid"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// Paired is the extension's session.
type Paired struct {
	model.Tokens
	User         PairedUser `json:"user"`
	Organization Office     `json:"organization"`
}

// Create issues a one-time code for the signed-in user and office.
func (p *Pairings) Create(ctx context.Context, userUUID uuid.UUID, impersonating bool, office Office) (Pairing, error) {
	if p.rdb == nil {
		return Pairing{}, ErrPairingDisabled
	}
	if impersonating {
		return Pairing{}, ErrImpersonating
	}
	code, err := newCode()
	if err != nil {
		return Pairing{}, err
	}
	raw, _ := json.Marshal(pending{UserUUID: userUUID.String(), OrgSlug: office.Slug, OrgName: office.Name, OrgUUID: office.UUID})
	if err := p.rdb.Set(ctx, p.key(code), raw, PairingTTL).Err(); err != nil {
		return Pairing{}, fmt.Errorf("whatsapp: store pairing: %w", err)
	}
	exp := p.now().Add(PairingTTL).UTC()
	return Pairing{Code: code[:4] + "-" + code[4:], ExpiresAt: exp, ExpiresIn: int64(PairingTTL.Seconds())}, nil
}

// Redeem trades a code for a session, exactly once.
func (p *Pairings) Redeem(ctx context.Context, code, deviceName, ip string) (Paired, error) {
	if p.rdb == nil {
		return Paired{}, ErrPairingDisabled
	}
	code, ok := NormalizeCode(code)
	if !ok {
		return Paired{}, ErrPairingNotFound
	}
	raw, err := p.rdb.GetDel(ctx, p.key(code)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Paired{}, ErrPairingNotFound
	}
	if err != nil {
		return Paired{}, fmt.Errorf("whatsapp: redeem pairing: %w", err)
	}
	var pd pending
	if json.Unmarshal(raw, &pd) != nil {
		return Paired{}, ErrPairingNotFound
	}
	userUUID, err := uuid.Parse(pd.UserUUID)
	if err != nil {
		return Paired{}, ErrPairingNotFound
	}
	ua := SessionUserAgent
	if name := cleanDeviceName(deviceName); name != "" {
		ua += " · " + name
	}
	tokens, user, err := p.accounts.IssueQRLoginSession(ctx, userUUID, model.SessionMeta{UserAgent: ua, IP: ip})
	if err != nil {
		return Paired{}, err
	}
	return Paired{
		Tokens:       tokens,
		User:         PairedUser{UUID: user.UUID.String(), Email: user.Email, Name: user.Name},
		Organization: Office{UUID: pd.OrgUUID, Slug: pd.OrgSlug, Name: pd.OrgName},
	}, nil
}

// key never holds the raw code.
func (p *Pairings) key(code string) string {
	sum := sha256.Sum256([]byte(code))
	return fmt.Sprintf("app:%s:whatsapp_pair:%s", p.env, hex.EncodeToString(sum[:]))
}

// NormalizeCode upper-cases the code and drops spaces and dashes; ok is false
// for anything that cannot be a code.
func NormalizeCode(s string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case r == '-' || r == ' ':
			continue
		case strings.ContainsRune(codeAlphabet, r):
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	out := b.String()
	return out, len(out) == codeLen
}

func newCode() (string, error) {
	var b strings.Builder
	max := big.NewInt(int64(len(codeAlphabet)))
	for range codeLen {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("whatsapp: random: %w", err)
		}
		b.WriteByte(codeAlphabet[n.Int64()])
	}
	return b.String(), nil
}

func cleanDeviceName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > maxDeviceName {
		s = string(r[:maxDeviceName])
	}
	return s
}
