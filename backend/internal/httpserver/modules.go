package httpserver

import (
	communicationmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/communication"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/officesetup"
	supportmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/support"
	"net/http"

	accessmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/access"
	accesshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/access/handler"
	accountplanmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan"
	accountplanhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/handler"
	planexporthandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/planexport"
	accountrulesmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules"
	accountruleshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/handler"
	activitymodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/activity"
	activityhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/activity/handler"
	agentsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/agents"
	agentshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/agents/handler"
	aimodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai"
	aihandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/handler"
	copilotmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/copilot"
	doctasksmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks"
	excelimportmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport"
	excelimporthandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/handler"
	legalmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/legal"
	legalhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/legal/handler"
	logsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/logs"
	logshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/logs/handler"
	officedashmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/officedash"
	periodsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods"
	periodshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/handler"
	receiptsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts"
	receiptshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/handler"
	relatedmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/related"
	reportsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/reports"
	reviewmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review"
	reviewhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/handler"
	searchmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/search"
	searchhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/search/handler"
	summariesmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/summaries"
	summarieshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/summaries/handler"
	vatforecastmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast"
	vatforecasthandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/vatforecast/handler"
	warningsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings"
	warningshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/handler"
	whatsappmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/whatsapp"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/taxpayerctx"
)

// moduleHandlers carries the handlers of self-registering modules.
type moduleHandlers struct {
	access   *accesshandler.Handler
	search   *searchhandler.Handler
	activity *activityhandler.Handler
	logs     *logshandler.Handler
	ai       *aihandler.Handler
	warnings *warningshandler.Handler
	review   *reviewhandler.Handler
	excel    *excelimporthandler.Handler
	receipts *receiptshandler.Handler
	// accountplan: hedef hesap planı (issue #30).
	accountplan *accountplanhandler.Handler
	// planexport: Luca hesap planı CSV ve cari alt hesaplar (#464).
	planexport *planexporthandler.Handler
	periods    *periodshandler.Handler
	summaries  *summarieshandler.Handler
	// reports: mükellef raporu (issue #386).
	reports *reportsmodule.Handler
	// copilot: sıradaki iş akışı ve komut çubuğu (issue #392).
	copilot  *copilotmodule.Handler
	agents   *agentshandler.Handler
	legal    *legalhandler.Handler
	doctasks *doctasksmodule.Handler
	// officedash: ofis ana panosu (issue #237).
	officedash    *officedashmodule.Handler
	officeClients *officedashmodule.ClientsHandler
	officeReview  *officedashmodule.ReviewHandler
	// accountrules: hesap kodu kural motoru (issue #25).
	accountrules *accountruleshandler.Handler
	vat          *vatforecasthandler.Handler
	setup        *officesetup.Handler
	// support: ClickFix destek widget'ı ayarı (issue #387).
	support *supportmodule.Handler
	// communication: İletişim merkezi, Ayarlar > SMS (#780 A).
	communication *communicationmodule.Handler
	// whatsapp: WhatsApp Web uzantısı (issue #388).
	whatsapp *whatsappmodule.Handler
	// related: kayıtlar arası ilişkili kayıtlar (issue #469).
	related *relatedmodule.Handler
	// engagements resolves {engagementUuid} for engagement-scoped module routes.
	engagements engagementctx.Resolver
	// rlsScope is rls.Middleware for modules whose tables are enforced.
	rlsScope func(http.Handler) http.Handler
	// taxpayers resolves X-Taxpayer for /v1/taxpayer/* module routes.
	taxpayers taxpayerctx.Resolver
}

// selfRegisteredModules is the single list of modules mounted through
// module.Router (docs/design/module-registry.md). server.go mounts it with
// real handlers; modules_test.go mounts it record-only to check OpenAPI.
func selfRegisteredModules(h moduleHandlers) []module.Module {
	return []module.Module{
		accessmodule.Module{H: h.access},
		searchmodule.Module{H: h.search},
		activitymodule.Module{H: h.activity},
		logsmodule.Module{H: h.logs},
		aimodule.Module{H: h.ai},
		warningsmodule.Module{H: h.warnings, Engagements: h.engagements},
		reviewmodule.Module{H: h.review, Engagements: h.engagements},
		excelimportmodule.Module{H: h.excel, Engagements: h.engagements},
		receiptsmodule.Module{H: h.receipts, Engagements: h.engagements},
		accountplanmodule.Module{H: h.accountplan, Engagements: h.engagements, Plan: h.planexport},
		periodsmodule.Module{H: h.periods, Engagements: h.engagements},
		summariesmodule.Module{H: h.summaries, Engagements: h.engagements},
		reportsmodule.Module{H: h.reports, Engagements: h.engagements, Taxpayers: h.taxpayers},
		copilotmodule.Module{H: h.copilot, Engagements: h.engagements},
		agentsmodule.Module{H: h.agents},
		legalmodule.Module{H: h.legal},
		doctasksmodule.Module{H: h.doctasks, Engagements: h.engagements, Taxpayers: h.taxpayers},
		accountrulesmodule.Module{H: h.accountrules, Engagements: h.engagements},
		vatforecastmodule.Module{H: h.vat, Engagements: h.engagements, Scope: h.rlsScope},
		officesetup.Module{H: h.setup},
		supportmodule.Module{H: h.support, Taxpayers: h.taxpayers},
		communicationmodule.Module{H: h.communication, Engagements: h.engagements},
		officedashmodule.Module{H: h.officedash, Clients: h.officeClients, Review: h.officeReview, Engagements: h.engagements},
		whatsappmodule.Module{H: h.whatsapp, Engagements: h.engagements, Scope: h.rlsScope},
		relatedmodule.Module{H: h.related, Engagements: h.engagements, Scope: h.rlsScope},
	}
}
