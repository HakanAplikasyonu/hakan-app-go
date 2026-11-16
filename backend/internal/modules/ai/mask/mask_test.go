package mask

import (
	"strings"
	"sync"
	"testing"
)

func TestMaskVKNAndTCKN(t *testing.T) {
	m := New()
	in := "Satıcı VKN:1234567890, alıcı TCKN 12345678901, fatura no 987654321098765."
	out := m.Mask(in)
	if strings.Contains(out, "1234567890,") || strings.Contains(out, "12345678901") {
		t.Fatalf("identifiers leaked: %q", out)
	}
	if !strings.Contains(out, "VKN:[VKN_1]") || !strings.Contains(out, "TCKN [TCKN_1]") {
		t.Fatalf("unexpected tokens: %q", out)
	}
	// 15-digit invoice number is not an identifier.
	if !strings.Contains(out, "987654321098765") {
		t.Fatalf("long number should stay: %q", out)
	}
	if got := m.Unmask(out); got != in {
		t.Fatalf("round trip:\n got %q\nwant %q", got, in)
	}
}

func TestMaskElevenDigitsStartingWithZeroIsNotTCKN(t *testing.T) {
	m := New()
	// A number starting with 0 is a phone number, not a TCKN (#169).
	if out := m.Mask("Tel 05321234567"); out != "Tel [TEL_1]" {
		t.Fatalf("got %q", out)
	}
	if m.Counts()[KindTCKN] != 0 {
		t.Fatalf("counts = %v", m.Counts())
	}
}

func TestMaskIBANGroupedAndCompact(t *testing.T) {
	m := New()
	grouped := "TR33 0006 1005 1978 6457 8413 26"
	compact := "TR330006100519786457841326"
	out := m.Mask("Ödeme " + grouped + " ve " + compact)
	if strings.Contains(out, "0006") || strings.Contains(out, compact) {
		t.Fatalf("IBAN leaked: %q", out)
	}
	// Same IBAN in two spellings -> one token.
	if strings.Count(out, "[IBAN_1]") != 2 || m.Counts()[KindIBAN] != 1 {
		t.Fatalf("want one IBAN token used twice: %q counts=%v", out, m.Counts())
	}
	// IBAN digits are not re-masked as VKN/TCKN.
	if m.Counts()[KindVKN] != 0 || m.Counts()[KindTCKN] != 0 {
		t.Fatalf("IBAN digits double-masked: %v", m.Counts())
	}
	if got := m.Unmask("[IBAN_1]"); got != grouped {
		t.Fatalf("unmask = %q, want first spelling %q", got, grouped)
	}
}

func TestMaskForeignIBANNeedsValidChecksum(t *testing.T) {
	m := New()
	valid := "DE89370400440532013000"
	invalid := "DE00370400440532013000"
	out := m.Mask(valid + " " + invalid)
	if strings.Contains(out, valid) {
		t.Fatalf("valid IBAN leaked: %q", out)
	}
	if !strings.Contains(out, invalid) {
		t.Fatalf("checksum-invalid foreign string should stay: %q", out)
	}
}

func TestMaskNames(t *testing.T) {
	m := New("Ayşe Yılmaz", "Yılmaz Gıda Ltd. Şti.", "Al")
	in := "AYŞE  YILMAZ, Yılmaz Gıda Ltd. Şti. firmasına ödeme yaptı. Alış faturası."
	out := m.Mask(in)
	if strings.Contains(strings.ToLower(out), "ayşe") || strings.Contains(out, "Gıda") {
		t.Fatalf("name leaked: %q", out)
	}
	// "Al" is too short to register; "Alış" must stay.
	if !strings.Contains(out, "Alış") {
		t.Fatalf("short name masked a word: %q", out)
	}
	if m.Counts()[KindName] != 2 {
		t.Fatalf("counts = %v", m.Counts())
	}
	if got := m.Unmask(out); got != in {
		t.Fatalf("round trip:\n got %q\nwant %q", got, in)
	}
}

func TestMaskNameTurkishCaseSharesToken(t *testing.T) {
	m := New("Işık İnşaat")
	out := m.Mask("IŞIK İNŞAAT / ışık inşaat / Işık İnşaat")
	if out != "[AD_1] / [AD_1] / [AD_1]" {
		t.Fatalf("got %q", out)
	}
}

func TestMaskNameNotInsideWord(t *testing.T) {
	m := New("Ali")
	out := m.Mask("Alim Ali'ye yazdı")
	if out != "Alim [AD_1]'ye yazdı" {
		t.Fatalf("got %q", out)
	}
}

func TestMaskStableTokensAcrossCalls(t *testing.T) {
	m := New()
	a := m.Mask("1111111111")
	b := m.Mask("x 1111111111 y 2222222222")
	if a != "[VKN_1]" || b != "x [VKN_1] y [VKN_2]" {
		t.Fatalf("a=%q b=%q", a, b)
	}
	if m.Total() != 2 {
		t.Fatalf("total = %d", m.Total())
	}
}

func TestUnmaskLeavesUnknownTokens(t *testing.T) {
	m := New()
	if got := m.Unmask("[VKN_9] bilinmiyor"); got != "[VKN_9] bilinmiyor" {
		t.Fatalf("got %q", got)
	}
}

func TestMaskDoesNotTouchTokenDigits(t *testing.T) {
	m := New()
	once := m.Mask("1234567890")
	if twice := m.Mask(once); twice != once {
		t.Fatalf("re-masking changed tokens: %q -> %q", once, twice)
	}
}

func TestMaskConcurrent(t *testing.T) {
	m := New("Mehmet Demir")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Mask("Mehmet Demir 1234567890 TR330006100519786457841326")
		}()
	}
	wg.Wait()
	c := m.Counts()
	if c[KindName] != 1 || c[KindVKN] != 1 || c[KindIBAN] != 1 {
		t.Fatalf("counts = %v", c)
	}
}
