package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/pkg/intconv"
)

// RefPrefix marks a reference produced by this vault. A different backend
// (e.g. "hcv:" for HashiCorp Vault) would use its own prefix.
const RefPrefix = "vault:"

var (
	ErrNotFound   = errors.New("secrets: not found")
	ErrInvalidRef = errors.New("secrets: invalid reference")
	ErrPurpose    = errors.New("secrets: purpose mismatch")
	ErrEmpty      = errors.New("secrets: empty secret")
)

var purposeRe = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)

// Ref is an opaque pointer to a stored secret ("vault:<uuid>"). It is safe to
// store and to log.
type Ref string

// ParseRef validates a reference read from another table.
func ParseRef(s string) (Ref, error) {
	if _, err := refID(Ref(s)); err != nil {
		return "", err
	}
	return Ref(s), nil
}

func refFor(id uuid.UUID) Ref { return Ref(RefPrefix + id.String()) }

func refID(r Ref) (uuid.UUID, error) {
	rest, ok := strings.CutPrefix(string(r), RefPrefix)
	if !ok {
		return uuid.Nil, ErrInvalidRef
	}
	id, err := uuid.Parse(rest)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, ErrInvalidRef
	}
	return id, nil
}

// Vault is what callers depend on. Purpose names the owner kind
// ("source_connection", "bank_connection" …); Get checks it so a reference
// copied into the wrong table cannot be read there.
type Vault interface {
	Put(ctx context.Context, purpose string, plaintext Secret) (Ref, error)
	Get(ctx context.Context, purpose string, ref Ref) (Secret, error)
	Replace(ctx context.Context, purpose string, ref Ref, plaintext Secret) error
	Delete(ctx context.Context, ref Ref) error
}

// Store persists sealed envelopes. PGStore and MemoryStore implement it.
type Store interface {
	Insert(ctx context.Context, e envelope) error
	Get(ctx context.Context, id uuid.UUID) (envelope, error) // ErrNotFound when missing
	Replace(ctx context.Context, e envelope) (bool, error)
	Rewrap(ctx context.Context, e envelope, oldKEK string) (bool, error)
	ListNotOnKEK(ctx context.Context, activeKEK string, limit int32) ([]envelope, error)
	CountByKEK(ctx context.Context) (map[string]int64, error)
	Delete(ctx context.Context, id uuid.UUID) (bool, error)
}

// EnvelopeVault is the built-in Vault: envelope encryption in Postgres.
type EnvelopeVault struct {
	kr    *Keyring
	store Store
}

var _ Vault = (*EnvelopeVault)(nil)

// New returns a vault over store using keyring kr.
func New(kr *Keyring, store Store) *EnvelopeVault {
	return &EnvelopeVault{kr: kr, store: store}
}

func (v *EnvelopeVault) Put(ctx context.Context, purpose string, plaintext Secret) (Ref, error) {
	if !purposeRe.MatchString(purpose) {
		return "", fmt.Errorf("%w: invalid purpose", ErrPurpose)
	}
	if plaintext.Empty() {
		return "", ErrEmpty
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	e, err := seal(v.kr, id, purpose, plaintext.Reveal())
	if err != nil {
		return "", err
	}
	if err := v.store.Insert(ctx, e); err != nil {
		return "", fmt.Errorf("secrets: store: %w", err)
	}
	return refFor(id), nil
}

func (v *EnvelopeVault) Get(ctx context.Context, purpose string, ref Ref) (Secret, error) {
	id, err := refID(ref)
	if err != nil {
		return Secret{}, err
	}
	e, err := v.store.Get(ctx, id)
	if err != nil {
		return Secret{}, err
	}
	if e.Purpose != purpose {
		return Secret{}, ErrPurpose
	}
	pt, err := open(v.kr, e)
	if err != nil {
		return Secret{}, err
	}
	return Secret{b: pt}, nil
}

// Replace stores new plaintext under the same reference (new data key).
func (v *EnvelopeVault) Replace(ctx context.Context, purpose string, ref Ref, plaintext Secret) error {
	id, err := refID(ref)
	if err != nil {
		return err
	}
	if plaintext.Empty() {
		return ErrEmpty
	}
	e, err := seal(v.kr, id, purpose, plaintext.Reveal())
	if err != nil {
		return err
	}
	ok, err := v.store.Replace(ctx, e)
	if err != nil {
		return fmt.Errorf("secrets: store: %w", err)
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func (v *EnvelopeVault) Delete(ctx context.Context, ref Ref) error {
	id, err := refID(ref)
	if err != nil {
		return err
	}
	ok, err := v.store.Delete(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// PutJSON marshals v (e.g. a credentials struct) and stores it.
func PutJSON(ctx context.Context, vault Vault, purpose string, v any) (Ref, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	defer wipe(b)
	return vault.Put(ctx, purpose, Secret{b: b})
}

// GetJSON reads a reference and unmarshals it into out.
func GetJSON(ctx context.Context, vault Vault, purpose string, ref Ref, out any) error {
	s, err := vault.Get(ctx, purpose, ref)
	if err != nil {
		return err
	}
	defer wipe(s.b)
	return json.Unmarshal(s.b, out)
}

// RotationReport summarises one Rotate call. It holds counts only.
type RotationReport struct {
	ActiveKEK string
	Rewrapped int
	Failed    int
	Remaining map[string]int64 // records per KEK after the run
}

// Rotate re-wraps every data key that is not on the active KEK. Ciphertexts
// are untouched, so it is cheap and safe to re-run; a record whose KEK is
// missing from the keyring counts as Failed and is left as it was.
func (v *EnvelopeVault) Rotate(ctx context.Context, batch int32) (RotationReport, error) {
	if batch <= 0 {
		batch = 200
	}
	rep := RotationReport{ActiveKEK: v.kr.ActiveID()}
	skip := map[uuid.UUID]bool{}
	for {
		list, err := v.store.ListNotOnKEK(ctx, v.kr.ActiveID(), batch+intconv.Int32(len(skip)))
		if err != nil {
			return rep, err
		}
		progressed := false
		for _, e := range list {
			if skip[e.ID] {
				continue
			}
			next, err := rewrap(v.kr, e)
			if err != nil {
				skip[e.ID] = true
				rep.Failed++
				continue
			}
			ok, err := v.store.Rewrap(ctx, next, e.KEKID)
			if err != nil {
				return rep, err
			}
			if ok {
				rep.Rewrapped++
			}
			progressed = true
		}
		if !progressed {
			break
		}
	}
	counts, err := v.store.CountByKEK(ctx)
	if err != nil {
		return rep, err
	}
	rep.Remaining = counts
	return rep, nil
}
