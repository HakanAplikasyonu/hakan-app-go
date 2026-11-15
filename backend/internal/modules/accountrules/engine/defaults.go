package engine

import "github.com/shopspring/decimal"

// Platform defaults (Tek Düzen Hesap Planı). Low confidence on purpose: they
// always land in the review queue until the office writes its own rules.
var (
	defaultConfidence = decimal.RequireFromString("0.5000")
	vatConfidence     = decimal.RequireFromString("0.8000")
)

func def(ref, name string, role Role, prio int, c Conditions, code string, conf decimal.Decimal) Rule {
	return Rule{Ref: "global:" + ref, Name: name, Role: role, Priority: prio, Conditions: c, AccountCode: code,
		Confidence: conf, Origin: "global", Version: 1}
}

// Defaults are the platform fallback rules. Priority 1000 keeps them below
// every office rule (default priority 100).
var Defaults = []Rule{
	def("line_sales_return", "Satıştan iadeler", RoleLine, 1000, Conditions{Direction: "outgoing", InvoiceType: "IADE"}, "610", defaultConfidence),
	def("line_sales", "Yurt içi satışlar", RoleLine, 1000, Conditions{Direction: "outgoing"}, "600", defaultConfidence),
	def("line_purchase_return", "Alıştan iadeler (ticari mal)", RoleLine, 1000, Conditions{Direction: "incoming", InvoiceType: "IADE"}, "153", defaultConfidence),
	def("line_fixed_asset", "Demirbaş adayı", RoleLine, 1000, Conditions{Direction: "incoming", LineType: "fixed_asset_candidate"}, "255", defaultConfidence),
	def("line_service", "Hizmet alımı (genel yönetim gideri)", RoleLine, 1000, Conditions{Direction: "incoming", LineType: "service"}, "770", defaultConfidence),
	def("line_expense", "Gider", RoleLine, 1000, Conditions{Direction: "incoming", LineType: "expense"}, "770", defaultConfidence),
	def("line_goods", "Ticari mal alımı", RoleLine, 1000, Conditions{Direction: "incoming"}, "153", defaultConfidence),
	def("vat_output", "Hesaplanan KDV", RoleVAT, 1000, Conditions{Direction: "outgoing"}, "391", vatConfidence),
	def("vat_input", "İndirilecek KDV", RoleVAT, 1000, Conditions{Direction: "incoming"}, "191", vatConfidence),
	def("counterparty_customer", "Alıcılar", RoleCounterparty, 1000, Conditions{Direction: "outgoing"}, "120", defaultConfidence),
	def("counterparty_supplier", "Satıcılar", RoleCounterparty, 1000, Conditions{Direction: "incoming"}, "320", defaultConfidence),
}
