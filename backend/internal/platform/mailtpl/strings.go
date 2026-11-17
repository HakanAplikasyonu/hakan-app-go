package mailtpl

// messages are the layout's own words (not the email's content), per
// language. Keys are i18n keys; a missing language falls back to Turkish,
// the product language.
var messages = map[string]map[string]string{
	"tr": {
		"button_fallback": "Düğme çalışmıyorsa bu adresi tarayıcınıza yapıştırın:",
		"preferences":     "Bildirim tercihleri",
		"unsubscribe":     "Bu tür e-postaları almak istemiyorsanız tercihlerinizi değiştirebilirsiniz:",
	},
	"en": {
		"button_fallback": "If the button does not work, paste this address into your browser:",
		"preferences":     "Notification preferences",
		"unsubscribe":     "If you no longer want emails like this, change your preferences:",
	},
}

func lang(code string) string {
	if _, ok := messages[code]; ok {
		return code
	}
	return "tr"
}
