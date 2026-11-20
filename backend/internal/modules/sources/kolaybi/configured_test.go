package kolaybi

import "testing"

// Liste yanıtındaki `configured` (#639): kanal ya da kasa yoksa false.
func TestConfigured(t *testing.T) {
	if (&Service{}).Configured() {
		t.Fatal("empty app must not be configured")
	}
	if (&Service{app: App{Channel: "hakan"}}).Configured() {
		t.Fatal("no vault must not be configured")
	}
}
