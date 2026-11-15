// Package secrets is the credential vault (G9, #18): envelope encryption with a
// fresh data key (DEK) per record and a key-encryption key (KEK) from the
// environment. Plaintext never reaches the database or the logs.
//
// Callers keep only an opaque reference (Ref, e.g. source_connection.credentials_ref)
// and go through the Vault interface, so the backend can later move to an
// external KMS / HashiCorp Vault without touching the callers.
package secrets

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// LegacyKEKID is the id of the key derived from APP_ENCRYPTION_KEY. It is
// always in the keyring (when that key is set) so records written before a
// dedicated SECRETS_KEYRING existed stay readable.
const LegacyKEKID = "app"

var (
	ErrInvalidKeyring = errors.New("secrets: invalid keyring")
	ErrUnknownKEK     = errors.New("secrets: unknown key-encryption key")
)

var kekIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// Keyring holds every KEK that may still wrap stored data keys, plus the one
// new records (and rotation) use.
type Keyring struct {
	keys   map[string][]byte
	active string
}

// ParseKeyring builds a keyring from configuration.
//
//	spec     SECRETS_KEYRING, "id:base64key,id2:base64key" (each key 32 bytes); may be empty
//	activeID SECRETS_ACTIVE_KEY_ID; empty means the last key in spec, or LegacyKEKID
//	legacy   APP_ENCRYPTION_KEY (raw 32 bytes or base64 of 32 bytes); may be empty
func ParseKeyring(spec, activeID, legacy string) (*Keyring, error) {
	kr := &Keyring{keys: map[string][]byte{}}
	if strings.TrimSpace(legacy) != "" {
		k, err := decodeKey(legacy)
		if err != nil {
			return nil, fmt.Errorf("%w: APP_ENCRYPTION_KEY: %v", ErrInvalidKeyring, err)
		}
		kr.keys[LegacyKEKID] = k
	}
	last := ""
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, material, ok := strings.Cut(part, ":")
		id = strings.TrimSpace(id)
		if !ok || !kekIDRe.MatchString(id) {
			return nil, fmt.Errorf("%w: entry must be <id>:<base64 key> with a lowercase id", ErrInvalidKeyring)
		}
		if id == LegacyKEKID {
			return nil, fmt.Errorf("%w: id %q is reserved for APP_ENCRYPTION_KEY", ErrInvalidKeyring, id)
		}
		if _, dup := kr.keys[id]; dup {
			return nil, fmt.Errorf("%w: duplicate id %q", ErrInvalidKeyring, id)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(material))
		if err != nil || len(raw) != 32 {
			return nil, fmt.Errorf("%w: key %q must be base64 of 32 bytes", ErrInvalidKeyring, id)
		}
		kr.keys[id] = raw
		last = id
	}
	switch {
	case strings.TrimSpace(activeID) != "":
		kr.active = strings.TrimSpace(activeID)
	case last != "":
		kr.active = last
	default:
		kr.active = LegacyKEKID
	}
	if _, ok := kr.keys[kr.active]; !ok {
		return nil, fmt.Errorf("%w: active key %q is not in the keyring", ErrInvalidKeyring, kr.active)
	}
	return kr, nil
}

// ActiveID is the KEK new records are wrapped with.
func (k *Keyring) ActiveID() string { return k.active }

// IDs lists every KEK id, sorted.
func (k *Keyring) IDs() []string {
	out := make([]string, 0, len(k.keys))
	for id := range k.keys {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (k *Keyring) key(id string) ([]byte, error) {
	b, ok := k.keys[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKEK, id)
	}
	return b, nil
}

func decodeKey(material string) ([]byte, error) {
	if raw, err := base64.StdEncoding.DecodeString(material); err == nil && len(raw) == 32 {
		return raw, nil
	}
	if len(material) == 32 {
		return []byte(material), nil
	}
	return nil, errors.New("key must be 32 bytes (raw or base64)")
}
