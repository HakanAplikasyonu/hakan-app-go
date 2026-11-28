package entegrator

import (
	// EDM Bilişim EFaturaEDM SOAP client (in-house, ./edm); registers "edm".
	// Login → SESSION_ID, GetInvoice IN/OUT (header-only listing paged by
	// OFFSET, UBL by UUID), Logout. The web-service user and password come
	// from the vault like the other integrators.
	_ "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/entegrator/edm"
)

func init() {
	// CR_START_DATE / CR_END_DATE go out as RFC 3339 without a fraction.
	RegisterListProfile("edm", ListProfile{SecondBounds: true})
}
