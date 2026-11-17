package secrets

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
)

// Field encryption (#168): sensitive JSONB columns (raw bank / invoice
// payloads) are stored as a sealed JSON object instead of plaintext. Each
// value gets a fresh data key (DEK); the DEK is wrapped by a field KEK. The
// field keyring is separate from APP_ENCRYPTION_KEY and from the credential
// vault's SECRETS_KEYRING: a leak of one does not open the other.
//
// Stored form (still valid JSON, so the column type stays JSONB):
//
//	{"$enc":"f1","kek":"<kek id>","dek":"<base64 nonce|GCM(KEK,DEK)>","ct":"<base64 nonce|GCM(DEK,plaintext)>"}
//
// The AAD binds the ciphertext to its purpose (e.g. "bank.raw_payload"), so a
// value copied into another kind of column fails to open. Values are not
// bound to a row id: rows are encrypted before the id exists, and the banking
// pipeline copies a source payload onto its movement.

// FieldBaseKEKID is the id of FIELD_ENCRYPTION_KEY in the field keyring.
const FieldBaseKEKID = "field"

// FieldFormatV1 marks a sealed field value.
const FieldFormatV1 = "f1"

// Purposes of the encrypted columns. Columns that copy values between each
// other share one purpose.
const (
	PurposeBankRawPayload       = "bank.raw_payload"        // bank_transactions, bank_transaction_sources
	PurposeInvoiceRawPayload    = "invoice.raw_payload"     // invoices, invoice_sources
	PurposeCustomerRawPayload   = "customer.raw_payload"    // customers, customer_source_links
	PurposeExpenseRawPayload    = "expense.raw_payload"     // expenses
	PurposeRetailSaleRawPayload = "retail_sale.raw_payload" // retail_sales
	PurposeEntegratorHeader     = "entegrator.header_raw"   // entegrator_documents
	// PurposeFinancialTxRawPayload: source_financial_transactions (issue #385).
	PurposeFinancialTxRawPayload = "source_financial_transaction.raw_payload"
)

var ErrFieldKeyReused = errors.New("secrets: field key must differ from APP_ENCRYPTION_KEY and the vault keys")

type sealedField struct {
	Enc string `json:"$enc"`
	KEK string `json:"kek"`
	DEK []byte `json:"dek"`
	CT  []byte `json:"ct"`
}

// FieldCipher seals and opens field values with the field keyring.
type FieldCipher struct{ kr *Keyring }

// ParseFieldKeyring builds the field keyring.
//
//	spec     FIELD_ENCRYPTION_KEYRING, "id:base64key,…" (rotation); may be empty
//	activeID FIELD_ENCRYPTION_ACTIVE_KEY_ID; empty means the last spec key, or "field"
//	base     FIELD_ENCRYPTION_KEY (base64 or raw 32 bytes); may be empty
//	forbid   keys the field keyring must not reuse (APP_ENCRYPTION_KEY, SECRETS_KEYRING)
//
// It returns (nil, nil) when no field key is configured at all: encryption is
// then off and values are stored as plaintext (the config guard refuses that
// outside development).
func ParseFieldKeyring(spec, activeID, base string, forbid ...*Keyring) (*Keyring, error) {
	if strings.TrimSpace(spec) == "" && strings.TrimSpace(base) == "" {
		return nil, nil
	}
	kr, err := parseKeyring(spec, activeID, base, FieldBaseKEKID, "FIELD_ENCRYPTION_KEY")
	if err != nil {
		return nil, err
	}
	for _, other := range forbid {
		if other == nil {
			continue
		}
		for _, a := range kr.keys {
			for _, b := range other.keys {
				if subtle.ConstantTimeCompare(a, b) == 1 {
					return nil, ErrFieldKeyReused
				}
			}
		}
	}
	return kr, nil
}

// NewFieldCipher wraps a field keyring; nil gives nil (encryption off).
func NewFieldCipher(kr *Keyring) *FieldCipher {
	if kr == nil {
		return nil
	}
	return &FieldCipher{kr: kr}
}

// ActiveID is the KEK new values are wrapped with.
func (c *FieldCipher) ActiveID() string { return c.kr.ActiveID() }

// Seal encrypts plaintext JSON. It always seals, even input that looks
// sealed, so a crafted payload cannot skip encryption.
func (c *FieldCipher) Seal(purpose string, plaintext []byte) ([]byte, error) {
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("secrets: dek: %w", err)
	}
	defer wipe(dek)
	ct, err := gcmSeal(dek, plaintext, fieldDataAAD(purpose))
	if err != nil {
		return nil, err
	}
	kekID := c.kr.ActiveID()
	wrapped, err := c.wrap(kekID, purpose, dek)
	if err != nil {
		return nil, err
	}
	return json.Marshal(sealedField{Enc: FieldFormatV1, KEK: kekID, DEK: wrapped, CT: ct})
}

// Open returns the plaintext JSON of a stored value. Plaintext (written
// before encryption was on) is returned unchanged.
func (c *FieldCipher) Open(purpose string, stored []byte) ([]byte, error) {
	s, ok := parseSealed(stored)
	if !ok {
		return stored, nil
	}
	dek, err := c.unwrap(s, purpose)
	if err != nil {
		return nil, err
	}
	defer wipe(dek)
	return gcmOpen(dek, s.CT, fieldDataAAD(purpose))
}

// Rewrap re-wraps a sealed value's DEK with the active KEK; the ciphertext is
// unchanged. changed is false for plaintext and for values already on the
// active key.
func (c *FieldCipher) Rewrap(purpose string, stored []byte) (out []byte, changed bool, err error) {
	s, ok := parseSealed(stored)
	if !ok || s.KEK == c.kr.ActiveID() {
		return stored, false, nil
	}
	dek, err := c.unwrap(s, purpose)
	if err != nil {
		return nil, false, err
	}
	defer wipe(dek)
	if s.DEK, err = c.wrap(c.kr.ActiveID(), purpose, dek); err != nil {
		return nil, false, err
	}
	s.KEK = c.kr.ActiveID()
	out, err = json.Marshal(s)
	return out, err == nil, err
}

// FieldKEK reports the KEK id of a sealed value ("" for plaintext).
func FieldKEK(stored []byte) string {
	if s, ok := parseSealed(stored); ok {
		return s.KEK
	}
	return ""
}

// IsSealedField reports whether a stored value is a sealed field.
func IsSealedField(stored []byte) bool {
	_, ok := parseSealed(stored)
	return ok
}

func (c *FieldCipher) wrap(kekID, purpose string, dek []byte) ([]byte, error) {
	kek, err := c.kr.key(kekID)
	if err != nil {
		return nil, err
	}
	return gcmSeal(kek, dek, fieldDEKAAD(purpose, kekID))
}

func (c *FieldCipher) unwrap(s sealedField, purpose string) ([]byte, error) {
	kek, err := c.kr.key(s.KEK)
	if err != nil {
		return nil, err
	}
	return gcmOpen(kek, s.DEK, fieldDEKAAD(purpose, s.KEK))
}

func fieldDataAAD(purpose string) []byte { return []byte("hakan/field/v1/data|" + purpose) }

func fieldDEKAAD(purpose, kekID string) []byte {
	return []byte("hakan/field/v1/dek|" + purpose + "|" + kekID)
}

// parseSealed recognises a sealed value. JSONB reorders keys, so this
// decodes instead of matching a prefix; plaintext objects fail the marker.
func parseSealed(b []byte) (sealedField, bool) {
	t := bytes.TrimSpace(b)
	if len(t) == 0 || t[0] != '{' || !bytes.Contains(t, []byte(`"$enc"`)) {
		return sealedField{}, false
	}
	var s sealedField
	if json.Unmarshal(t, &s) != nil || s.Enc != FieldFormatV1 || s.KEK == "" || len(s.DEK) == 0 || len(s.CT) == 0 {
		return sealedField{}, false
	}
	return s, true
}

// The process-wide field cipher. Write paths call SealField and read paths
// OpenField; with no cipher installed both pass values through, so tests and
// development without a field key keep working.
var fieldCipher atomic.Pointer[FieldCipher]

// InstallFieldCipher sets the process-wide field cipher (nil turns it off).
func InstallFieldCipher(c *FieldCipher) { fieldCipher.Store(c) }

// InstalledFieldCipher is the process-wide field cipher, or nil.
func InstalledFieldCipher() *FieldCipher { return fieldCipher.Load() }

// SealField encrypts a JSON value with the installed cipher (pass-through
// when none is installed).
func SealField(purpose string, plaintext []byte) ([]byte, error) {
	c := fieldCipher.Load()
	if c == nil {
		return plaintext, nil
	}
	return c.Seal(purpose, plaintext)
}

// OpenField decrypts a stored JSON value with the installed cipher. A sealed
// value without a cipher is an error, never returned as if it were data.
func OpenField(purpose string, stored []byte) ([]byte, error) {
	c := fieldCipher.Load()
	if c == nil {
		if IsSealedField(stored) {
			return nil, fmt.Errorf("%w: no field key configured", ErrUnknownKEK)
		}
		return stored, nil
	}
	return c.Open(purpose, stored)
}
