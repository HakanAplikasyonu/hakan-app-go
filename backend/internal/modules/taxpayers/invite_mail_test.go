package taxpayers

import "testing"

func TestMailUnavailableReason(t *testing.T) {
	cases := []struct {
		env, host, want string
	}{
		{"production", "", EmailErrorNotConfigured},
		{"production", "127.0.0.1", EmailErrorNotConfigured},
		{"production", "localhost", EmailErrorNotConfigured},
		{"production", "[::1]", EmailErrorNotConfigured},
		{"production", "smtp.example.com", ""},
		{"development", "127.0.0.1", ""},
		{"test", "", ""},
	}
	for _, c := range cases {
		if got := MailUnavailableReason(c.env, c.host); got != c.want {
			t.Errorf("MailUnavailableReason(%q, %q) = %q, want %q", c.env, c.host, got, c.want)
		}
	}
}

func TestEmailDomain(t *testing.T) {
	if got := emailDomain("sahip@example.test"); got != "example.test" {
		t.Fatalf("emailDomain = %q", got)
	}
	if got := emailDomain("bozuk"); got != "" {
		t.Fatalf("emailDomain = %q", got)
	}
}
