package secrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func newTestFieldCipher(t *testing.T, spec, active string) *FieldCipher {
	t.Helper()
	kr, err := ParseFieldKeyring(spec, active, key(9))
	if err != nil {
		t.Fatal(err)
	}
	return NewFieldCipher(kr)
}

func TestFieldSealOpen(t *testing.T) {
	c := newTestFieldCipher(t, "", "")
	if c.ActiveID() != FieldBaseKEKID {
		t.Fatalf("active = %q", c.ActiveID())
	}
	plain := []byte(`{"iban":"TR000000000000000000000001","aciklama":"kira odemesi"}`)
	sealed, err := c.Seal(PurposeBankRawPayload, plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("TR0000")) || bytes.Contains(sealed, []byte("kira")) {
		t.Fatal("plaintext visible in sealed value")
	}
	if !json.Valid(sealed) || !IsSealedField(sealed) || FieldKEK(sealed) != FieldBaseKEKID {
		t.Fatalf("sealed value is not a valid envelope: %s", sealed)
	}
	again, _ := c.Seal(PurposeBankRawPayload, plain)
	if bytes.Equal(again, sealed) {
		t.Fatal("two seals of the same value must differ (fresh DEK and nonce)")
	}
	got, err := c.Open(PurposeBankRawPayload, sealed)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("open = %s, %v", got, err)
	}
	// Another purpose (column kind) cannot open it.
	if _, err := c.Open(PurposeInvoiceRawPayload, sealed); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("wrong purpose: %v", err)
	}
	// Plaintext from before encryption passes through.
	if got, err := c.Open(PurposeBankRawPayload, plain); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("plaintext passthrough = %s, %v", got, err)
	}
	// A tampered ciphertext fails.
	var s sealedField
	_ = json.Unmarshal(sealed, &s)
	s.CT[len(s.CT)-1] ^= 1
	bad, _ := json.Marshal(s)
	if _, err := c.Open(PurposeBankRawPayload, bad); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("tampered: %v", err)
	}
}

func TestFieldSealsLookalikeInput(t *testing.T) {
	c := newTestFieldCipher(t, "", "")
	fake := []byte(`{"$enc":"f1","kek":"field","dek":"AAAA","ct":"AAAA","iban":"TR11"}`)
	sealed, err := c.Seal(PurposeBankRawPayload, fake)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("TR11")) {
		t.Fatal("input that looks sealed must still be encrypted")
	}
	got, err := c.Open(PurposeBankRawPayload, sealed)
	if err != nil || !bytes.Equal(got, fake) {
		t.Fatalf("open = %s, %v", got, err)
	}
}

func TestFieldRotation(t *testing.T) {
	old := newTestFieldCipher(t, "", "")
	plain := []byte(`{"a":1}`)
	sealed, _ := old.Seal(PurposeInvoiceRawPayload, plain)

	rotated := newTestFieldCipher(t, "f2027:"+key(7), "f2027")
	out, changed, err := rotated.Rewrap(PurposeInvoiceRawPayload, sealed)
	if err != nil || !changed || FieldKEK(out) != "f2027" {
		t.Fatalf("rewrap: changed=%v kek=%q err=%v", changed, FieldKEK(out), err)
	}
	var a, b sealedField
	_ = json.Unmarshal(sealed, &a)
	_ = json.Unmarshal(out, &b)
	if !bytes.Equal(a.CT, b.CT) {
		t.Fatal("rotation must not touch the ciphertext")
	}
	if _, changed, _ := rotated.Rewrap(PurposeInvoiceRawPayload, out); changed {
		t.Fatal("rewrap on the active key must be a no-op")
	}
	// The new key alone opens it once the old one is dropped.
	kr, err := ParseFieldKeyring("f2027:"+key(7), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := NewFieldCipher(kr).Open(PurposeInvoiceRawPayload, out); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("open after dropping old key = %s, %v", got, err)
	}
	if _, err := NewFieldCipher(kr).Open(PurposeInvoiceRawPayload, sealed); !errors.Is(err, ErrUnknownKEK) {
		t.Fatalf("value still on the dropped key: %v", err)
	}
}

func TestParseFieldKeyring(t *testing.T) {
	if kr, err := ParseFieldKeyring("", "", ""); kr != nil || err != nil {
		t.Fatalf("no key configured = %v, %v; want off", kr, err)
	}
	vault := mustKeyring(t, "k1:"+key(1), "")
	// Reusing APP_ENCRYPTION_KEY (vault id "app") or a vault key is refused.
	if _, err := ParseFieldKeyring("", "", legacy, vault); !errors.Is(err, ErrFieldKeyReused) {
		t.Fatalf("app key reused: %v", err)
	}
	if _, err := ParseFieldKeyring("f2:"+key(1), "", key(9), vault); !errors.Is(err, ErrFieldKeyReused) {
		t.Fatalf("vault key reused: %v", err)
	}
	if _, err := ParseFieldKeyring("", "", key(9), vault); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []string{"field:" + key(2), "app:" + key(2)} {
		if _, err := ParseFieldKeyring(spec, "", key(9)); !errors.Is(err, ErrInvalidKeyring) {
			t.Fatalf("reserved id %q: %v", spec, err)
		}
	}
	if _, err := ParseFieldKeyring("", "nope", key(9)); !errors.Is(err, ErrInvalidKeyring) {
		t.Fatalf("unknown active: %v", err)
	}
}

func TestInstalledFieldCipher(t *testing.T) {
	t.Cleanup(func() { InstallFieldCipher(nil) })
	plain := []byte(`{"x":"y"}`)

	InstallFieldCipher(nil)
	if got, _ := SealField(PurposeBankRawPayload, plain); !bytes.Equal(got, plain) {
		t.Fatal("no cipher: SealField must pass through")
	}
	c := newTestFieldCipher(t, "", "")
	sealed, _ := c.Seal(PurposeBankRawPayload, plain)
	if _, err := OpenField(PurposeBankRawPayload, sealed); err == nil {
		t.Fatal("no cipher: a sealed value must not be returned as data")
	}

	InstallFieldCipher(c)
	got, err := SealField(PurposeBankRawPayload, plain)
	if err != nil || !IsSealedField(got) {
		t.Fatalf("SealField = %s, %v", got, err)
	}
	if back, err := OpenField(PurposeBankRawPayload, got); err != nil || !bytes.Equal(back, plain) {
		t.Fatalf("OpenField = %s, %v", back, err)
	}
}
