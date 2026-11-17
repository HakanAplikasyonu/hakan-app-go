// Package mailtpl renders outgoing email: a table-based HTML layout with
// inline CSS and a plain-text version written for reading, both from the same
// structured content. Every visual value comes from theme.go.
//
// Rules the layout keeps (issue #467): 600 px column, inline styles (the
// <style> block only adds dark mode), no script, no web fonts, alt text on
// every image, readable with images off, a preheader, absolute https links,
// and HTML under Gmail's ~102 KB clipping limit. User input is escaped by
// html/template.
package mailtpl

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"strings"
	texttemplate "text/template"
	"unicode/utf8"
)

// MaxHTMLBytes keeps the HTML part under Gmail's clipping threshold
// (102 KB of the message body) with a margin for the transfer encoding.
const MaxHTMLBytes = 95 * 1024

// ErrTooLarge is returned when the HTML would be clipped by Gmail.
var ErrTooLarge = errors.New("mailtpl: html exceeds the clipping limit")

// ErrInsecureLink is returned for a button or footer link that is not an
// absolute https URL (http is allowed for localhost in development).
var ErrInsecureLink = errors.New("mailtpl: link must be an absolute https URL")

// Kind of a content block.
type Kind int

const (
	// Paragraph is running text; URLs in it become links.
	Paragraph Kind = iota
	// Code is a one-time code shown large (Label above it).
	Code
	// Button is a call to action (Label) to URL, with the address repeated
	// as text for clients that hide the button.
	Button
	// Facts is a two-column label/value table (amounts, dates).
	Facts
	// Note is a highlighted aside (Label as its title).
	Note
)

// Block is one piece of the email body, rendered in order.
type Block struct {
	Kind  Kind
	Text  string
	Label string
	URL   string
	Facts []Fact
}

// Fact is one row of a Facts block.
type Fact struct {
	Label string
	Value string
}

// Email is the content of one message.
type Email struct {
	// Lang selects the layout's own words ("tr", "en"); default Turkish.
	Lang string
	// Subject is the message subject (and the HTML title).
	Subject string
	// Preheader is the preview line inbox lists show after the subject.
	Preheader string
	// Heading is the large title at the top of the card (optional).
	Heading string
	// Greeting opens the body ("Merhaba Ayşe,").
	Greeting string
	Blocks   []Block
	// SignOff closes the body, one line each ("Saygılarımızla,", name).
	SignOff []string
	// Reason says why the recipient got this email (footer, optional).
	Reason string
	// PreferencesURL links the footer to the notification settings; set on
	// notification emails, which also carry List-Unsubscribe.
	PreferencesURL string
	// PlainText, when set, is the hand-written text body used for the text
	// part instead of rendering Greeting, Blocks and SignOff (the footer is
	// still added). Stored notification templates are written this way.
	PlainText string
}

// Rendered is the output of Render.
type Rendered struct {
	HTML string
	Text string
}

// Renderer renders emails with a theme. BaseURL (the frontend origin) makes
// the logo URL absolute.
type Renderer struct {
	Theme   Theme
	BaseURL string
}

// New returns a renderer with the default theme.
func New(baseURL string) Renderer {
	return Renderer{Theme: Default, BaseURL: baseURL}
}

//go:embed templates/*
var files embed.FS

var (
	htmlTpl = htmltemplate.Must(htmltemplate.New("layout.html.tmpl").ParseFS(files, "templates/layout.html.tmpl"))
	textTpl = texttemplate.Must(texttemplate.New("layout.txt.tmpl").ParseFS(files, "templates/layout.txt.tmpl"))
)

// Render produces the HTML and plain-text parts.
func (r Renderer) Render(e Email) (Rendered, error) {
	if err := r.checkLinks(e); err != nil {
		return Rendered{}, err
	}
	data := r.view(e)

	var h bytes.Buffer
	if err := htmlTpl.Execute(&h, data); err != nil {
		return Rendered{}, fmt.Errorf("mailtpl: html: %w", err)
	}
	if h.Len() > MaxHTMLBytes {
		return Rendered{}, fmt.Errorf("%w: %d bytes", ErrTooLarge, h.Len())
	}
	var txt bytes.Buffer
	if err := textTpl.Execute(&txt, data); err != nil {
		return Rendered{}, fmt.Errorf("mailtpl: text: %w", err)
	}
	return Rendered{HTML: h.String(), Text: tidyText(txt.String())}, nil
}

// RenderSafe is Render for senders that must not drop a message: a button
// or preferences link that is not https (e.g. a plain-http staging origin)
// is turned into plain text instead of failing. The address is still in the
// text part; the HTML just does not link it.
func (r Renderer) RenderSafe(e Email) (Rendered, error) {
	out, err := r.Render(e)
	if !errors.Is(err, ErrInsecureLink) {
		return out, err
	}
	blocks := make([]Block, 0, len(e.Blocks))
	for _, b := range e.Blocks {
		if b.Kind == Button && !SafeURL(b.URL) {
			b = Block{Kind: Paragraph, Text: b.Label + ":\n" + b.URL}
		}
		blocks = append(blocks, b)
	}
	e.Blocks = blocks
	if e.PreferencesURL != "" && !SafeURL(e.PreferencesURL) {
		e.PreferencesURL = ""
	}
	return r.Render(e)
}

func (r Renderer) checkLinks(e Email) error {
	for _, b := range e.Blocks {
		if b.Kind == Button && !SafeURL(b.URL) {
			return fmt.Errorf("%w: %q", ErrInsecureLink, b.URL)
		}
	}
	if e.PreferencesURL != "" && !SafeURL(e.PreferencesURL) {
		return fmt.Errorf("%w: %q", ErrInsecureLink, e.PreferencesURL)
	}
	return nil
}

// SafeURL reports whether u may be linked from an email: absolute https, or
// http to localhost / 127.0.0.1 (development, MailHog).
func SafeURL(u string) bool {
	p, err := url.Parse(strings.TrimSpace(u))
	if err != nil || p.Host == "" || p.User != nil {
		return false
	}
	switch p.Scheme {
	case "https":
		return true
	case "http":
		h := p.Hostname()
		return h == "localhost" || h == "127.0.0.1"
	}
	return false
}

// view is what the templates see.
type view struct {
	Email
	Lang    string
	S       styles
	Logo    string
	Product string
	Width   int
	Size    int
	Footer  string
	Blocks  []blockView
	T       map[string]string
	DarkCSS htmltemplate.CSS
}

type blockView struct {
	Block
	Segments []segment
	// FactLines are the Facts rows aligned for the text part.
	FactLines []string
}

// The templates switch on these.
func (b blockView) IsParagraph() bool { return b.Kind == Paragraph }
func (b blockView) IsCode() bool      { return b.Kind == Code }
func (b blockView) IsButton() bool    { return b.Kind == Button }
func (b blockView) IsFacts() bool     { return b.Kind == Facts }
func (b blockView) IsNote() bool      { return b.Kind == Note }

// segment is a run of paragraph text, linked when URL is set.
type segment struct {
	Text string
	URL  string
}

func (r Renderer) view(e Email) view {
	th := r.Theme
	l := lang(e.Lang)
	v := view{
		Email:   e,
		Lang:    l,
		S:       newStyles(th),
		Product: th.ProductName,
		Width:   th.Width,
		Size:    th.LogoSize,
		Footer:  th.Footer[l],
		T:       messages[l],
		DarkCSS: darkCSS(th),
	}
	if base := strings.TrimRight(r.BaseURL, "/"); base != "" && th.LogoPath != "" && SafeURL(base) {
		v.Logo = base + th.LogoPath
	}
	if v.Preheader == "" {
		v.Preheader = e.Subject
	}
	for _, b := range e.Blocks {
		bv := blockView{Block: b}
		switch b.Kind {
		case Paragraph, Note:
			bv.Segments = linkify(b.Text)
		case Facts:
			bv.FactLines = alignFacts(b.Facts)
		}
		v.Blocks = append(v.Blocks, bv)
	}
	return v
}

// linkify splits text into plain and URL runs. Only SafeURL links are made
// clickable; anything else stays text. Newlines are kept as "\n" segments.
func linkify(text string) []segment {
	var out []segment
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i, line := range lines {
		if i > 0 {
			out = append(out, segment{Text: "\n"})
		}
		words := strings.SplitAfter(line, " ")
		var plain strings.Builder
		for _, w := range words {
			word := strings.TrimRight(w, " ")
			trail := w[len(word):]
			core := strings.TrimRight(word, ".,;:)")
			punct := word[len(core):]
			if (strings.HasPrefix(core, "https://") || strings.HasPrefix(core, "http://")) && SafeURL(core) {
				if plain.Len() > 0 {
					out = append(out, segment{Text: plain.String()})
					plain.Reset()
				}
				out = append(out, segment{Text: core, URL: core})
				plain.WriteString(punct + trail)
				continue
			}
			plain.WriteString(w)
		}
		if plain.Len() > 0 {
			out = append(out, segment{Text: plain.String()})
		}
	}
	return out
}

// alignFacts lines the values of a Facts block up in one column.
func alignFacts(facts []Fact) []string {
	w := 0
	for _, f := range facts {
		if n := utf8.RuneCountInString(f.Label); n > w {
			w = n
		}
	}
	out := make([]string, 0, len(facts))
	for _, f := range facts {
		pad := strings.Repeat(" ", w-utf8.RuneCountInString(f.Label))
		out = append(out, "  "+f.Label+":"+pad+" "+f.Value)
	}
	return out
}

// tidyText collapses runs of blank lines the template leaves behind and
// wraps long prose lines at 76 columns (URLs are never broken).
func tidyText(s string) string {
	var out []string
	blank := 0
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
			out = append(out, "")
			continue
		}
		blank = 0
		out = append(out, wrap(line, 76)...)
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

func wrap(line string, width int) []string {
	if utf8.RuneCountInString(line) <= width {
		return []string{line}
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
	var out []string
	cur := indent
	for _, w := range strings.Fields(line) {
		switch {
		case strings.TrimSpace(cur) == "":
			cur = indent + w
		case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(w) > width:
			out = append(out, cur)
			cur = indent + w
		default:
			cur += " " + w
		}
	}
	return append(out, cur)
}
