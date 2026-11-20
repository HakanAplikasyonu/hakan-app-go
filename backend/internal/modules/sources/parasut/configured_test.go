package parasut

import "testing"

// Liste yanıtındaki `configured` (#639): uygulama anahtarı ya da kasa yoksa false.
func TestConfigured(t *testing.T) {
	if (&Service{}).Configured() {
		t.Fatal("empty app must not be configured")
	}
	app := App{ClientID: "id", ClientSecret: "secret", RedirectURL: "https://x/cb"}
	if (&Service{app: app}).Configured() {
		t.Fatal("no vault must not be configured")
	}
}
