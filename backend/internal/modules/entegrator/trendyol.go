package entegrator

import (
	// Trendyol e-Faturam REST client (#59); registers "trendyol". It lists
	// only the e-Fatura inbox (incoming-einvoice/search); outgoing e-Fatura
	// and e-Arşiv can be downloaded but not listed, so those boxes are
	// recorded as not_listable on the connection. Reply, send and cancel are
	// not reachable through Client (30 §5). Credentials: Username / Password
	// (sign-in e-mail and password), Options user_id and company_id.
	_ "github.com/furkanmeclis/go-ubltr/provider/trendyol"
)
