package mailtpl

import (
	"errors"
	"flag"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// Golden files: go test ./internal/platform/mailtpl -update
var update = flag.Bool("update", false, "rewrite golden files")

const base = "https://app.example.com"

func samples() map[string]Email {
	return map[string]Email{
		"code": {
			Subject:   "Giriş kodunuz: 482913",
			Preheader: "Kod 10 dakika geçerlidir.",
			Greeting:  "Merhaba Ayşe,",
			Blocks: []Block{
				{Kind: Code, Label: "Giriş kodunuz", Text: "482913"},
				{Kind: Paragraph, Text: "Kod 10 dakika geçerlidir ve yalnızca bir kez kullanılabilir."},
				{Kind: Paragraph, Text: "Bu isteği siz yapmadıysanız bu e-postayı yok sayabilirsiniz."},
			},
		},
		"notification": {
			Subject:  "Örnek Gıda belge bekliyor",
			Heading:  "Belge bekleniyor",
			Greeting: "Merhaba,",
			Blocks: []Block{
				{Kind: Paragraph, Text: "Örnek Müşavirlik, Örnek Gıda A.Ş. için 3 banka hareketinin belgesini istiyor.\nFotoğrafını ya da PDF'ini uygulamadan yükleyebilirsiniz."},
				{Kind: Button, Label: "Belgeleri yükle", URL: base + "/m/belgeler?t=1&x=2"},
			},
			Reason:         "Bu e-postayı Örnek Gıda A.Ş. mükellef panelinin üyesi olduğunuz için aldınız.",
			PreferencesURL: base + "/profile",
		},
		"report": {
			Lang:     "tr",
			Subject:  "Örnek Gıda A.Ş. — Ekim 2026 raporu",
			Greeting: "Sayın Örnek Gıda A.Ş.,",
			Blocks: []Block{
				{Kind: Paragraph, Text: "Ekim 2026 dönemine ait raporunuz ektedir (PDF)."},
				{Kind: Facts, Label: "Özet (KDV hariç)", Facts: []Fact{
					{Label: "Ciro", Value: "1.250.000,00 ₺"},
					{Label: "Gider", Value: "830.400,50 ₺"},
					{Label: "Net", Value: "419.599,50 ₺"},
				}},
				{Kind: Note, Label: "Müşavir yorumu", Text: "Ciro geçen aya göre arttı. Ayrıntı: https://app.example.com/rapor."},
			},
			SignOff: []string{"Saygılarımızla,", "Örnek Müşavirlik"},
		},
		"english": {
			Lang:     "en",
			Subject:  "Your sign-in code: 482913",
			Greeting: "Hello Jane,",
			Blocks: []Block{
				{Kind: Code, Label: "Your sign-in code", Text: "482913"},
			},
			PreferencesURL: base + "/profile",
		},
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update): %v", path, err)
	}
	if got != string(want) {
		t.Fatalf("%s differs from golden (run with -update to accept):\n%s", name, got)
	}
}

func TestRenderGolden(t *testing.T) {
	r := New(base)
	for name, e := range samples() {
		t.Run(name, func(t *testing.T) {
			out, err := r.Render(e)
			if err != nil {
				t.Fatal(err)
			}
			golden(t, name+".html", out.HTML)
			golden(t, name+".txt", out.Text)
		})
	}
}

// TestHTMLRules checks the deliverability rules on every sample's HTML by
// walking the parsed document.
func TestHTMLRules(t *testing.T) {
	r := New(base)
	for name, e := range samples() {
		t.Run(name, func(t *testing.T) {
			out, err := r.Render(e)
			if err != nil {
				t.Fatal(err)
			}
			if len(out.HTML) > MaxHTMLBytes {
				t.Fatalf("html is %d bytes", len(out.HTML))
			}
			doc, err := html.Parse(strings.NewReader(out.HTML))
			if err != nil {
				t.Fatal(err)
			}
			var metas []string
			var walk func(n *html.Node)
			walk = func(n *html.Node) {
				if n.Type == html.ElementNode {
					attr := map[string]string{}
					for _, a := range n.Attr {
						attr[a.Key] = a.Val
						if strings.HasPrefix(a.Key, "on") {
							t.Errorf("event handler attribute %s", a.Key)
						}
					}
					switch n.Data {
					case "script", "link", "iframe", "form", "object", "embed", "video", "audio":
						t.Errorf("<%s> is not allowed in email", n.Data)
					case "img":
						if strings.TrimSpace(attr["alt"]) == "" {
							t.Errorf("img without alt: %v", attr)
						}
						if !strings.HasPrefix(attr["src"], "https://") {
							t.Errorf("img src must be absolute https: %q", attr["src"])
						}
						if attr["width"] == "" || attr["height"] == "" {
							t.Errorf("img without width/height (layout shifts with images off)")
						}
					case "a":
						if !SafeURL(attr["href"]) {
							t.Errorf("unsafe link %q", attr["href"])
						}
					case "table":
						if w, err := strconv.Atoi(attr["width"]); err == nil && w > 600 {
							t.Errorf("table width %d > 600", w)
						}
						if attr["role"] != "presentation" {
							t.Errorf("layout table without role=presentation")
						}
					case "style":
						css := n.FirstChild.Data
						if strings.Contains(css, "@import") || strings.Contains(css, "@font-face") || strings.Contains(css, "url(") {
							t.Errorf("style block loads external resources")
						}
					case "meta":
						metas = append(metas, attr["name"]+"="+attr["content"])
					case "div", "p", "td", "h1", "span":
						// Everything visible is styled inline.
						if attr["style"] == "" && n.Parent != nil && n.Parent.Data != "td" {
							t.Errorf("<%s> without inline style", n.Data)
						}
					}
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
			}
			walk(doc)
			joined := strings.Join(metas, " ")
			for _, want := range []string{"color-scheme=light dark", "supported-color-schemes=light dark"} {
				if !strings.Contains(joined, want) {
					t.Errorf("missing meta %s", want)
				}
			}
			if !strings.Contains(out.HTML, "display:none") {
				t.Error("missing preheader")
			}
		})
	}
}

func TestTextIsReadable(t *testing.T) {
	r := New(base)
	for name, e := range samples() {
		out, err := r.Render(e)
		if err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(out.Text, "<>") && !strings.Contains(name, "html") {
			t.Errorf("%s: markup in the text part:\n%s", name, out.Text)
		}
		for _, line := range strings.Split(out.Text, "\n") {
			if n := len([]rune(line)); n > 76 && !strings.Contains(line, "://") {
				t.Errorf("%s: line of %d runes: %q", name, n, line)
			}
		}
		for _, b := range e.Blocks {
			if b.Kind == Button && !strings.Contains(out.Text, b.URL) {
				t.Errorf("%s: text part lacks the button URL", name)
			}
			if b.Kind == Code && !strings.Contains(out.Text, b.Text) {
				t.Errorf("%s: text part lacks the code", name)
			}
		}
	}
}

func TestRenderEscapesInput(t *testing.T) {
	evil := `<script>alert(1)</script><img src=x onerror=alert(2)>"'&`
	e := Email{
		Subject:   evil,
		Preheader: evil,
		Heading:   evil,
		Greeting:  "Merhaba " + evil,
		Blocks: []Block{
			{Kind: Paragraph, Text: evil + " https://app.example.com/a?b=<c>&d=\"e\""},
			{Kind: Code, Label: evil, Text: evil},
			{Kind: Button, Label: evil, URL: "https://app.example.com/x?q=\"><script>"},
			{Kind: Facts, Label: evil, Facts: []Fact{{Label: evil, Value: evil}}},
			{Kind: Note, Label: evil, Text: evil},
		},
		SignOff: []string{evil},
		Reason:  evil,
	}
	out, err := New(base).Render(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"<script", "<img src=x"} {
		if strings.Contains(out.HTML, bad) {
			t.Fatalf("unescaped %q in HTML", bad)
		}
	}
	doc, err := html.Parse(strings.NewReader(out.HTML))
	if err != nil {
		t.Fatal(err)
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "script" {
				t.Fatal("script element in output")
			}
			for _, a := range n.Attr {
				if strings.HasPrefix(a.Key, "on") {
					t.Fatalf("event handler %s in output", a.Key)
				}
				if a.Key == "href" && !SafeURL(a.Val) {
					t.Fatalf("unsafe href %q", a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if !strings.Contains(out.HTML, "&lt;script&gt;") {
		t.Fatal("expected escaped script tag")
	}
	// The text part is plain text: it carries the input verbatim.
	if !strings.Contains(out.Text, evil) {
		t.Fatal("text part altered user input")
	}
}

func TestRenderRejectsUnsafeLinks(t *testing.T) {
	for _, u := range []string{"javascript:alert(1)", "/relative", "http://app.example.com/x", "ftp://a.example", "https://user:pw@a.example"} {
		_, err := New(base).Render(Email{Blocks: []Block{{Kind: Button, Label: "x", URL: u}}})
		if !errors.Is(err, ErrInsecureLink) {
			t.Errorf("%q: err = %v", u, err)
		}
		_, err = New(base).Render(Email{PreferencesURL: u})
		if !errors.Is(err, ErrInsecureLink) {
			t.Errorf("preferences %q: err = %v", u, err)
		}
	}
	// Development: http to localhost is allowed (MailHog).
	if _, err := New("http://localhost:3000").Render(Email{Blocks: []Block{{Kind: Button, Label: "x", URL: "http://localhost:3000/a"}}}); err != nil {
		t.Fatal(err)
	}
	// Non-https URLs in running text stay text.
	out, err := New(base).Render(Email{Blocks: []Block{{Kind: Paragraph, Text: "bkz. javascript:alert(1) ve http://evil.example/x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.HTML, `href="javascript`) || strings.Contains(out.HTML, `href="http://evil`) {
		t.Fatal("unsafe URL linked")
	}
}

func TestRenderSizeLimit(t *testing.T) {
	long := strings.Repeat("Uzun bir paragraf. ", 6000)
	_, err := New(base).Render(Email{Blocks: []Block{{Kind: Paragraph, Text: long}}})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestLinkifyKeepsPunctuation(t *testing.T) {
	got := linkify("Yüklemek için: https://app.example.com/x. Teşekkürler")
	if len(got) != 3 || got[1].URL != "https://app.example.com/x" || got[2].Text != ". Teşekkürler" {
		t.Fatalf("segments = %+v", got)
	}
}

// TestThemeContrast keeps text readable in both schemes (WCAG AA, 4.5:1).
func TestThemeContrast(t *testing.T) {
	for name, p := range map[string]Palette{"light": Default.Light, "dark": Default.Dark} {
		for _, pair := range [][3]string{
			{"text", p.Text, p.Surface},
			{"muted", p.TextMuted, p.Surface},
			{"muted on page", p.TextMuted, p.Background},
			{"link", p.Link, p.Surface},
			{"button", p.OnBrand, p.Brand},
			{"code", p.Text, p.Tint},
		} {
			if r := contrast(pair[1], pair[2]); r < 4.5 {
				t.Errorf("%s %s: contrast %.2f < 4.5 (%s on %s)", name, pair[0], r, pair[1], pair[2])
			}
		}
	}
}

// TestColoursOnlyInTheme: a theme change must touch theme.go only.
func TestColoursOnlyInTheme(t *testing.T) {
	hex := regexp.MustCompile(`(?:^|[^&\w])#[0-9A-Fa-f]{6}\b|rgba?\(`)
	paths, _ := filepath.Glob("*.go")
	tpls, _ := filepath.Glob("templates/*")
	for _, p := range append(paths, tpls...) {
		if p == "theme.go" || strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if loc := hex.Find(b); loc != nil {
			t.Errorf("%s has a colour literal %q; colours live in theme.go", p, loc)
		}
	}
}

func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	v, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	ch := func(c uint64) float64 {
		s := float64(c) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(v>>16&0xff) + 0.7152*ch(v>>8&0xff) + 0.0722*ch(v&0xff)
}
