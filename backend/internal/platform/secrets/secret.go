package secrets

import (
	"fmt"
	"log/slog"
)

const redacted = "[REDACTED]"

// Secret wraps decrypted bytes so they cannot leak through fmt, slog or JSON
// by accident. Only Reveal returns the plaintext.
type Secret struct{ b []byte }

// NewSecret wraps plaintext (e.g. before Put).
func NewSecret(b []byte) Secret { return Secret{b: append([]byte(nil), b...)} }

// Reveal returns the plaintext. Do not log it.
func (s Secret) Reveal() []byte { return s.b }

// Empty reports whether there is no plaintext.
func (s Secret) Empty() bool { return len(s.b) == 0 }

func (Secret) String() string               { return redacted }
func (Secret) GoString() string             { return redacted }
func (Secret) LogValue() slog.Value         { return slog.StringValue(redacted) }
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }
func (Secret) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// Format makes every verb (%v, %+v, %#v, %s, %x, %q) print the placeholder.
func (Secret) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(redacted)) }
