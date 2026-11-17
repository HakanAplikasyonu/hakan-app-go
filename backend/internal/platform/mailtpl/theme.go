package mailtpl

// Theme is every visual value of an email: colours, type, logo and footer.
// The layouts read nothing else, so a theme change edits this file only.
//
// The values follow the N3 ("yeşil") palette of docs/tasarim/tokens/tokens.css
// (n3-light, and n3-dark for clients that honour prefers-color-scheme). The
// brand green #01FF11 is a fill colour: text on it is near-black (15:1), and
// green text on white uses the darker brand-text #067A12 (5.9:1, WCAG AA).
//
// Email clients do not load web fonts reliably, so type is the system stack.
type Theme struct {
	// ProductName appears in the header (logo alt text, wordmark) and footer.
	ProductName string
	// Width is the content column in px (600 is the de-facto email width).
	Width int

	FontStack string
	MonoStack string

	Light Palette
	Dark  Palette

	// Radius of cards, buttons and the code box (px).
	Radius int

	// LogoPath is joined to the frontend URL: an absolute https image, never
	// inlined base64. LogoSize is its rendered width and height (px); the
	// file is twice that for high-density screens.
	LogoPath string
	LogoSize int

	// Footer is the closing line under every email, per language.
	Footer map[string]string
}

// Palette is one colour scheme.
type Palette struct {
	Background string // page behind the card
	Surface    string // the card
	Border     string
	Text       string
	TextMuted  string
	Brand      string // button fill, accent bar
	OnBrand    string // text on Brand
	Link       string // link colour on Surface
	Tint       string // code box and note background
}

// Default is the theme every email uses.
var Default = Theme{
	ProductName: "HAKAN",
	Width:       600,
	FontStack:   `-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif`,
	MonoStack:   `ui-monospace, SFMono-Regular, Menlo, Consolas, 'Liberation Mono', monospace`,
	Light: Palette{
		Background: "#F3F3F5",
		Surface:    "#FFFFFF",
		Border:     "#E3E3E9",
		Text:       "#0A0A0A",
		TextMuted:  "#4A4A55",
		Brand:      "#01FF11",
		OnBrand:    "#0A0A0A",
		Link:       "#067A12",
		Tint:       "#E6FFE8",
	},
	Dark: Palette{
		Background: "#0A0A0A",
		Surface:    "#131316",
		Border:     "#26262C",
		Text:       "#F3F3F5",
		TextMuted:  "#B4B4BE",
		Brand:      "#01FF11",
		OnBrand:    "#0A0A0A",
		Link:       "#01FF11",
		Tint:       "#0D2410",
	},
	Radius:   10,
	LogoPath: "/email/logo-96.png",
	LogoSize: 48,
	Footer: map[string]string{
		"tr": "Bu e-posta HAKAN üzerinden gönderildi.",
		"en": "This email was sent through HAKAN.",
	},
}
