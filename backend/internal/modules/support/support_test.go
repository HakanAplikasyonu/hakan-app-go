package support

import "testing"

func TestNormalizeWidgetID(t *testing.T) {
	for in, want := range map[string]string{"": "", "  ab_C-1 ": "ab_C-1"} {
		got, err := NormalizeWidgetID(in)
		if err != nil || got != want {
			t.Fatalf("NormalizeWidgetID(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"<script>", "a b", string(make([]byte, 129))} {
		if _, err := NormalizeWidgetID(bad); err == nil {
			t.Fatalf("NormalizeWidgetID(%q) accepted", bad)
		}
	}
}

// ClickFix docs: hex(HMAC-SHA256(secret, user_id + "\n" + company_id)).
func TestUserHash(t *testing.T) {
	// printf 'u-1\nc-9' | openssl dgst -sha256 -hmac s3cret
	if got := UserHash("s3cret", "u-1", "c-9"); got != "98f87e3d671933ee39c0e39b1cf52f3ebbc9ab66b4995eb4e15ca13682a6807e" {
		t.Fatalf("UserHash = %s", got)
	}
	if UserHash("s3cret", "u-1", "") == UserHash("s3cret", "u-1", "c-9") {
		t.Fatal("company id must change the signature")
	}
	if len(UserHash("s", "u", "c")) != 64 {
		t.Fatal("want 64 hex chars")
	}
}
