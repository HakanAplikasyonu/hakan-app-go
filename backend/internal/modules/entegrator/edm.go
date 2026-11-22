package entegrator

import (
	// EDM Bilişim EFaturaEDM SOAP client (in-house, ./edm); registers "edm".
	// Login → SESSION_ID, GetInvoice IN/OUT (header-only listing paged by
	// OFFSET, UBL by UUID), Logout. The web-service user and password come
	// from the vault like the other integrators.
	_ "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/entegrator/edm"
)
