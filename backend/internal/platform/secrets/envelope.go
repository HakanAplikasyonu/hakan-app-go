package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
)

// AlgA256GCM is the only algorithm today: AES-256-GCM for both the data and
// the wrapped data key.
const AlgA256GCM = "A256GCM"

var ErrDecrypt = errors.New("secrets: decryption failed")

// envelope is one sealed record as stored. It never holds plaintext.
type envelope struct {
	ID         uuid.UUID
	Purpose    string
	Alg        string
	KEKID      string
	WrappedDEK []byte // nonce | GCM(KEK, DEK)
	Ciphertext []byte // nonce | GCM(DEK, plaintext)
}

// AAD binds a ciphertext to its record id and purpose, so a row copied onto
// another record (or another purpose) fails to decrypt.
func dataAAD(id uuid.UUID, purpose string) []byte {
	return []byte("hakan/secret/v1/data|" + id.String() + "|" + purpose)
}

func dekAAD(id uuid.UUID, kekID string) []byte {
	return []byte("hakan/secret/v1/dek|" + id.String() + "|" + kekID)
}

func seal(kr *Keyring, id uuid.UUID, purpose string, plaintext []byte) (envelope, error) {
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return envelope{}, fmt.Errorf("secrets: dek: %w", err)
	}
	defer wipe(dek)
	ct, err := gcmSeal(dek, plaintext, dataAAD(id, purpose))
	if err != nil {
		return envelope{}, err
	}
	kekID := kr.ActiveID()
	wrapped, err := wrapDEK(kr, kekID, id, dek)
	if err != nil {
		return envelope{}, err
	}
	return envelope{ID: id, Purpose: purpose, Alg: AlgA256GCM, KEKID: kekID, WrappedDEK: wrapped, Ciphertext: ct}, nil
}

func open(kr *Keyring, e envelope) ([]byte, error) {
	if e.Alg != AlgA256GCM {
		return nil, fmt.Errorf("secrets: unsupported alg %q", e.Alg)
	}
	dek, err := unwrapDEK(kr, e)
	if err != nil {
		return nil, err
	}
	defer wipe(dek)
	return gcmOpen(dek, e.Ciphertext, dataAAD(e.ID, e.Purpose))
}

// rewrap re-encrypts only the data key with the active KEK.
func rewrap(kr *Keyring, e envelope) (envelope, error) {
	dek, err := unwrapDEK(kr, e)
	if err != nil {
		return envelope{}, err
	}
	defer wipe(dek)
	wrapped, err := wrapDEK(kr, kr.ActiveID(), e.ID, dek)
	if err != nil {
		return envelope{}, err
	}
	e.KEKID, e.WrappedDEK = kr.ActiveID(), wrapped
	return e, nil
}

func wrapDEK(kr *Keyring, kekID string, id uuid.UUID, dek []byte) ([]byte, error) {
	kek, err := kr.key(kekID)
	if err != nil {
		return nil, err
	}
	return gcmSeal(kek, dek, dekAAD(id, kekID))
}

func unwrapDEK(kr *Keyring, e envelope) ([]byte, error) {
	kek, err := kr.key(e.KEKID)
	if err != nil {
		return nil, err
	}
	return gcmOpen(kek, e.WrappedDEK, dekAAD(e.ID, e.KEKID))
}

func gcmSeal(key, plaintext, aad []byte) ([]byte, error) {
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("secrets: nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plaintext, aad), nil
}

func gcmOpen(key, sealed, aad []byte) ([]byte, error) {
	aead, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	ns := aead.NonceSize()
	if len(sealed) < ns+aead.Overhead() {
		return nil, ErrDecrypt
	}
	out, err := aead.Open(nil, sealed[:ns], sealed[ns:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return out, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secrets: cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
