package agenttasks

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"
)

func TestFinalStatus(t *testing.T) {
	cases := []struct {
		outcome string
		files   []string
		want    string
	}{
		{StatusDone, []string{FileOK, FileOK}, StatusDone},
		{StatusDone, []string{FileOK, FileFailed}, StatusPartial},
		// "Yükle"den sonra sonuç okunamadı: asla otomatik tekrar denenmez.
		{StatusDone, []string{FileOK, FileUnknown}, StatusNeedsCheck},
		{StatusDone, []string{FileOK, FilePending}, StatusNeedsCheck},
		{StatusFailed, []string{FileOK}, StatusFailed},
		{StatusNeedsCheck, nil, StatusNeedsCheck},
		{StatusDone, nil, StatusDone},
	}
	for _, c := range cases {
		if got := FinalStatus(c.outcome, c.files); got != c.want {
			t.Errorf("FinalStatus(%s, %v) = %s, want %s", c.outcome, c.files, got, c.want)
		}
	}
}

func TestTransferOutcome(t *testing.T) {
	if got := TransferOutcome([]string{FileOK, FileOK}); got != "imported" {
		t.Fatalf("all ok = %q", got)
	}
	if got := TransferOutcome([]string{FileOK, FileFailed}); got != "failed" {
		t.Fatalf("one failed = %q", got)
	}
	if got := TransferOutcome([]string{FileOK, FilePending}); got != "" {
		t.Fatalf("one pending = %q", got)
	}
	if got := TransferOutcome([]string{FileUnknown}); got != "" {
		t.Fatalf("unknown = %q", got)
	}
}

// Hatalı satır raporu: Luca'nın hata satırı doğru belgeye eşlenir.
func TestMatchEntries(t *testing.T) {
	docs := []FileDocument{
		{DocumentKey: "ettn:aaa", DocumentNo: "ABC2026000000001"},
		{DocumentKey: "bank:bbb", DocumentNo: ""},
		{DocumentKey: "z:ccc", DocumentNo: "Z-12"},
	}
	got, err := MatchEntries([]EntryResult{
		{DocumentNo: "Z-12", Status: FileFailed, Reason: "Hesap kodu bulunamadı: 320.01.E99"},
		{EntryNo: "2", Status: FileFailed, Reason: "Borç alacak eşit değil"},
		{EntryNo: "1", Status: FileOK, LucaEntryNo: "M-55"},
		{EntryNo: "9", Status: FileFailed},
	}, docs)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].DocumentKey != "z:ccc" || got[1].DocumentKey != "bank:bbb" || got[2].DocumentKey != "ettn:aaa" {
		t.Fatalf("keys: %+v", got)
	}
	if got[2].DocumentNo != "ABC2026000000001" || got[3].DocumentKey != "" {
		t.Fatalf("fill / out of range: %+v", got)
	}
	if _, err := MatchEntries([]EntryResult{{Status: "maybe"}}, docs); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad status = %v", err)
	}
	long := make([]byte, 400)
	for i := range long {
		long[i] = 'x'
	}
	got, _ = MatchEntries([]EntryResult{{Status: FileFailed, Reason: string(long)}}, docs)
	if len(got[0].Reason) != 300 {
		t.Fatalf("reason not clipped: %d", len(got[0].Reason))
	}
}

func TestNormalizeKinds(t *testing.T) {
	all, err := NormalizeKinds(nil)
	if err != nil || len(all) != len(FileKinds) {
		t.Fatalf("all = %v %v", all, err)
	}
	// Sıra her zaman yükleme sırası: banka, Z, fatura, masraf, fiş.
	got, err := NormalizeKinds([]string{FileJournals, FileBank, FileBank})
	if err != nil || len(got) != 2 || got[0] != FileBank || got[1] != FileJournals {
		t.Fatalf("order = %v %v", got, err)
	}
	if _, err := NormalizeKinds([]string{"payroll"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown = %v", err)
	}
}

// Luca sınırı aşılınca gelen zip, ayrı ayrı yüklenecek dosyalara bölünür.
func TestSplitZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range []string{"banka_2026-10_1.xlsx", "banka_2026-10_2.xlsx", "banka_2026-10_3.xlsx"} {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("data " + n))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	parts, err := split("banka.zip", "application/zip", buf.Bytes())
	if err != nil || len(parts) != 3 {
		t.Fatalf("parts %d %v", len(parts), err)
	}
	if parts[1].name != "banka_2026-10_2.xlsx" || parts[1].contentType != xlsxType || string(parts[1].data) != "data banka_2026-10_2.xlsx" {
		t.Fatalf("part %+v", parts[1])
	}
	one, err := split("z.xlsx", xlsxType, []byte("x"))
	if err != nil || len(one) != 1 || one[0].name != "z.xlsx" {
		t.Fatalf("single %+v %v", one, err)
	}
}

func TestCheckCounterparty(t *testing.T) {
	ok := CounterpartyResult{TaxID: "1234567890", MainCode: "320", Status: CariCreated, AccountCode: "320.01.E11"}
	if err := checkCounterparty(ok); err != nil {
		t.Fatal(err)
	}
	bad := []CounterpartyResult{
		{TaxID: "12", MainCode: "320", Status: CariFailed},
		{TaxID: "1234567890", MainCode: "320", Status: CariFound, AccountCode: "120.01.001"},
		{TaxID: "1234567890", MainCode: "320", Status: CariFound, AccountCode: "320 01"},
		{TaxID: "1234567890", MainCode: "320", Status: CariConflict, AccountCode: "320.01"},
	}
	for _, c := range bad {
		if err := checkCounterparty(c); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v = %v", c, err)
		}
	}
}
