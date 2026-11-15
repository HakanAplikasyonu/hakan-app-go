package secrets

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func key(b byte) string { return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32)) }

const legacy = "test-legacy-key-32-bytes-long!!!" // 32 raw bytes, test only

func mustKeyring(t *testing.T, spec, active string) *Keyring {
	t.Helper()
	kr, err := ParseKeyring(spec, active, legacy)
	if err != nil {
		t.Fatal(err)
	}
	return kr
}

func TestParseKeyring(t *testing.T) {
	kr := mustKeyring(t, "", "")
	if kr.ActiveID() != LegacyKEKID {
		t.Fatalf("active = %q", kr.ActiveID())
	}
	kr = mustKeyring(t, "k1:"+key(1)+", k2:"+key(2), "")
	if kr.ActiveID() != "k2" || len(kr.IDs()) != 3 {
		t.Fatalf("active=%q ids=%v", kr.ActiveID(), kr.IDs())
	}
	kr = mustKeyring(t, "k1:"+key(1)+",k2:"+key(2), "k1")
	if kr.ActiveID() != "k1" {
		t.Fatal(kr.ActiveID())
	}
	bad := []struct{ spec, active string }{
		{"k1:" + key(1) + ",k1:" + key(2), ""},
		{"K1:" + key(1), ""},
		{"k1:short", ""},
		{"app:" + key(1), ""},
		{"k1:" + key(1), "nope"},
		{"noseparator", ""},
	}
	for _, c := range bad {
		if _, err := ParseKeyring(c.spec, c.active, legacy); !errors.Is(err, ErrInvalidKeyring) {
			t.Errorf("%q/%q: err = %v", c.spec, c.active, err)
		}
	}
	if _, err := ParseKeyring("", "", ""); !errors.Is(err, ErrInvalidKeyring) {
		t.Errorf("empty keyring accepted: %v", err)
	}
}

type creds struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func TestPutGetDoesNotStorePlaintext(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	v := New(mustKeyring(t, "k1:"+key(1), ""), store)
	plain := creds{Username: "entegrator-kullanici", Password: "cok-gizli-parola-123"}
	ref, err := PutJSON(ctx, v, "source_connection", plain)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(ref), RefPrefix) {
		t.Fatalf("ref = %q", ref)
	}
	for _, e := range store.raw() {
		blob, _ := json.Marshal(e)
		for _, needle := range []string{plain.Username, plain.Password} {
			if bytes.Contains(e.Ciphertext, []byte(needle)) || bytes.Contains(e.WrappedDEK, []byte(needle)) || bytes.Contains(blob, []byte(needle)) {
				t.Fatalf("stored row contains plaintext %q", needle)
			}
		}
		if e.KEKID != "k1" {
			t.Fatalf("kek = %q", e.KEKID)
		}
	}
	var got creds
	if err := GetJSON(ctx, v, "source_connection", ref, &got); err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("got %+v", got)
	}
	if _, err := v.Get(ctx, "bank_connection", ref); !errors.Is(err, ErrPurpose) {
		t.Fatalf("wrong purpose: %v", err)
	}
}

func TestRotationKeepsRecordsReadable(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	old := New(mustKeyring(t, "", ""), store) // legacy key only
	refs := map[Ref]string{}
	for i := range 5 {
		pt := fmt.Sprintf("secret-%d", i)
		ref, err := old.Put(ctx, "source_connection", NewSecret([]byte(pt)))
		if err != nil {
			t.Fatal(err)
		}
		refs[ref] = pt
	}
	before := map[Ref][]byte{}
	for _, e := range store.raw() {
		before[refFor(e.ID)] = e.Ciphertext
	}

	// Operator adds k2 and makes it active; the legacy key stays for reading.
	next := New(mustKeyring(t, "k2:"+key(2), "k2"), store)
	rep, err := next.Rotate(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Rewrapped != 5 || rep.Failed != 0 || rep.Remaining["k2"] != 5 || rep.Remaining[LegacyKEKID] != 0 {
		t.Fatalf("report = %+v", rep)
	}
	for _, e := range store.raw() {
		if !bytes.Equal(before[refFor(e.ID)], e.Ciphertext) {
			t.Fatal("rotation must not touch ciphertext")
		}
	}

	// After rotation the legacy key can be dropped from the keyring entirely.
	onlyK2, err := ParseKeyring("k2:"+key(2), "k2", "")
	if err != nil {
		t.Fatal(err)
	}
	v := New(onlyK2, store)
	for ref, want := range refs {
		s, err := v.Get(ctx, "source_connection", ref)
		if err != nil || string(s.Reveal()) != want {
			t.Fatalf("%s: %v %q", ref, err, s.Reveal())
		}
	}
	// Idempotent.
	rep, err = next.Rotate(ctx, 2)
	if err != nil || rep.Rewrapped != 0 {
		t.Fatalf("second run: %+v %v", rep, err)
	}
}

func TestRotationSkipsUnknownKEK(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	a := New(mustKeyring(t, "k1:"+key(1), "k1"), store)
	if _, err := a.Put(ctx, "source_connection", NewSecret([]byte("x"))); err != nil {
		t.Fatal(err)
	}
	// k1 missing from this keyring: the record is reported, not destroyed.
	kr, _ := ParseKeyring("k2:"+key(2), "k2", "")
	rep, err := New(kr, store).Rotate(ctx, 10)
	if err != nil || rep.Failed != 1 || rep.Rewrapped != 0 {
		t.Fatalf("%+v %v", rep, err)
	}
	if _, err := a.Get(ctx, "source_connection", refFor(store.raw()[0].ID)); err != nil {
		t.Fatal(err)
	}
}

func TestTamperingFails(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	v := New(mustKeyring(t, "k1:"+key(1), ""), store)
	r1, _ := v.Put(ctx, "source_connection", NewSecret([]byte("one")))
	r2, _ := v.Put(ctx, "source_connection", NewSecret([]byte("two")))
	id1, _ := refID(r1)
	id2, _ := refID(r2)
	e1, _ := store.Get(ctx, id1)
	e2, _ := store.Get(ctx, id2)
	// Copy row 1's sealed data onto row 2: AAD binds it to id1, so it must fail.
	e2.WrappedDEK, e2.Ciphertext = e1.WrappedDEK, e1.Ciphertext
	store.rows[id2] = e2
	if _, err := v.Get(ctx, "source_connection", r2); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("swapped row decrypted: %v", err)
	}
	e1.Ciphertext[len(e1.Ciphertext)-1] ^= 0xff
	store.rows[id1] = e1
	if _, err := v.Get(ctx, "source_connection", r1); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("flipped bit decrypted: %v", err)
	}
}

func TestReplaceAndDelete(t *testing.T) {
	ctx := context.Background()
	v := New(mustKeyring(t, "", ""), NewMemoryStore())
	ref, _ := v.Put(ctx, "source_connection", NewSecret([]byte("a")))
	if err := v.Replace(ctx, "source_connection", ref, NewSecret([]byte("b"))); err != nil {
		t.Fatal(err)
	}
	s, _ := v.Get(ctx, "source_connection", ref)
	if string(s.Reveal()) != "b" {
		t.Fatal(string(s.Reveal()))
	}
	if err := v.Replace(ctx, "bank_connection", ref, NewSecret([]byte("c"))); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replace across purpose: %v", err)
	}
	if err := v.Delete(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get(ctx, "source_connection", ref); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := v.Put(ctx, "source_connection", Secret{}); !errors.Is(err, ErrEmpty) {
		t.Fatal(err)
	}
	if _, err := v.Put(ctx, "Bad Purpose", NewSecret([]byte("x"))); !errors.Is(err, ErrPurpose) {
		t.Fatal(err)
	}
	if _, err := ParseRef("vault:not-a-uuid"); !errors.Is(err, ErrInvalidRef) {
		t.Fatal(err)
	}
}

func TestSecretNeverPrints(t *testing.T) {
	s := NewSecret([]byte("hunter2"))
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	log.Info("x", "cred", s, "wrapped", struct{ S Secret }{s})
	j, _ := json.Marshal(map[string]any{"s": s})
	outs := []string{
		fmt.Sprintf("%v %+v %#v %s %q %x", s, s, s, s, s, s),
		fmt.Sprint(struct{ S Secret }{s}),
		buf.String(), string(j),
	}
	for _, o := range outs {
		if strings.Contains(o, "hunter2") || strings.Contains(o, "68756e74657232") {
			t.Fatalf("plaintext leaked: %s", o)
		}
	}
}
