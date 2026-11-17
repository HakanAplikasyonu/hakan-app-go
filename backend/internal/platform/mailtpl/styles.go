package mailtpl

import (
	"fmt"
	htmltemplate "html/template"
)

// styles are the inline style attributes of the layout, computed from the
// theme. They are trusted constants (the theme is code, never input), hence
// htmltemplate.CSS: html/template would otherwise reject the quoted font
// names.
type styles struct {
	Body, Outer, Card, Header, Wordmark, Logo, Content    htmltemplate.CSS
	Heading, P, Link, CodeLabel, Code, ButtonCell, Button htmltemplate.CSS
	Fallback, FactsTable, FactLabel, FactValue, Note      htmltemplate.CSS
	NoteTitle, SignOff, Footer, FooterLink, Preheader     htmltemplate.CSS
	BgColor, CardColor, BrandColor, TintColor             string
}

func newStyles(th Theme) styles {
	c := th.Light
	font := "font-family:" + th.FontStack + ";"
	css := func(format string, a ...any) htmltemplate.CSS {
		return htmltemplate.CSS(fmt.Sprintf(format, a...)) //nolint:gosec // theme constants, not input
	}
	return styles{
		Body:       css("margin:0;padding:0;width:100%%;background-color:%s;%s", c.Background, font),
		Outer:      css("width:100%%;background-color:%s;", c.Background),
		Card:       css("width:100%%;max-width:%dpx;background-color:%s;border:1px solid %s;border-radius:%dpx;", th.Width, c.Surface, c.Border, th.Radius),
		Header:     css("padding:24px 32px 8px 32px;border-top:4px solid %s;border-radius:%dpx %dpx 0 0;", c.Brand, th.Radius, th.Radius),
		Wordmark:   css("%sfont-size:15px;font-weight:700;letter-spacing:0.08em;color:%s;vertical-align:middle;padding-left:12px;", font, c.Text),
		Logo:       css("display:block;border:0;outline:none;text-decoration:none;border-radius:%dpx;", th.Radius),
		Content:    css("padding:16px 32px 32px 32px;%sfont-size:16px;line-height:24px;color:%s;", font, c.Text),
		Heading:    css("margin:0 0 16px 0;%sfont-size:22px;line-height:30px;font-weight:700;color:%s;", font, c.Text),
		P:          css("margin:0 0 16px 0;%sfont-size:16px;line-height:24px;color:%s;", font, c.Text),
		Link:       css("color:%s;text-decoration:underline;word-break:break-all;", c.Link),
		CodeLabel:  css("margin:0 0 8px 0;%sfont-size:14px;line-height:20px;color:%s;", font, c.TextMuted),
		Code:       css("padding:16px 24px;background-color:%s;border:1px solid %s;border-radius:%dpx;font-family:%s;font-size:30px;line-height:36px;font-weight:700;letter-spacing:0.3em;color:%s;", c.Tint, c.Border, th.Radius, th.MonoStack, c.Text),
		ButtonCell: css("border-radius:%dpx;background-color:%s;", th.Radius, c.Brand),
		Button:     css("display:inline-block;padding:14px 28px;%sfont-size:16px;line-height:20px;font-weight:700;color:%s;text-decoration:none;border-radius:%dpx;", font, c.OnBrand, th.Radius),
		Fallback:   css("margin:12px 0 16px 0;%sfont-size:13px;line-height:20px;color:%s;", font, c.TextMuted),
		FactsTable: css("width:100%%;margin:0 0 16px 0;border-collapse:collapse;"),
		FactLabel:  css("padding:8px 12px 8px 0;border-bottom:1px solid %s;%sfont-size:15px;line-height:22px;color:%s;", c.Border, font, c.TextMuted),
		FactValue:  css("padding:8px 0;border-bottom:1px solid %s;font-family:%s;font-size:15px;line-height:22px;color:%s;text-align:right;white-space:nowrap;", c.Border, th.MonoStack, c.Text),
		Note:       css("padding:16px;background-color:%s;border-left:4px solid %s;border-radius:4px;%sfont-size:15px;line-height:22px;color:%s;", c.Tint, c.Brand, font, c.Text),
		NoteTitle:  css("margin:0 0 4px 0;font-weight:700;"),
		SignOff:    css("margin:24px 0 0 0;%sfont-size:16px;line-height:24px;color:%s;", font, c.Text),
		Footer:     css("padding:16px 32px 32px 32px;%sfont-size:12px;line-height:18px;color:%s;", font, c.TextMuted),
		FooterLink: css("color:%s;text-decoration:underline;", c.TextMuted),
		Preheader:  css("display:none;font-size:1px;line-height:1px;max-height:0;max-width:0;opacity:0;overflow:hidden;mso-hide:all;"),
		BgColor:    c.Background,
		CardColor:  c.Surface,
		BrandColor: c.Brand,
		TintColor:  c.Tint,
	}
}

// darkCSS is the only <style> content: dark-mode overrides for clients that
// support prefers-color-scheme (Apple Mail, iOS, Outlook.com via
// [data-ogsc]). Clients that strip it keep the inline light theme, which is
// complete on its own.
func darkCSS(th Theme) htmltemplate.CSS {
	d := th.Dark
	rules := fmt.Sprintf(`
.m-bg{background-color:%[1]s !important}
.m-card{background-color:%[2]s !important;border-color:%[3]s !important}
.m-text{color:%[4]s !important}
.m-muted{color:%[5]s !important}
.m-link{color:%[6]s !important}
.m-tint{background-color:%[7]s !important;border-color:%[3]s !important}
.m-fact{border-color:%[3]s !important}
`, d.Background, d.Surface, d.Border, d.Text, d.TextMuted, d.Link, d.Tint)
	return htmltemplate.CSS("@media (prefers-color-scheme: dark) {" + rules + "}\n" + //nolint:gosec // theme constants, not input
		prefixed("[data-ogsc] ", rules))
}

// prefixed repeats rules under an attribute selector (Outlook.com dark mode).
func prefixed(prefix, rules string) string {
	var out []byte
	for _, line := range splitLines(rules) {
		if line == "" {
			continue
		}
		out = append(out, prefix+line+"\n"...)
	}
	return string(out)
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
