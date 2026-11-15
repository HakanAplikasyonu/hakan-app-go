package httpserver

import (
	accessmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/access"
	accesshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/access/handler"
	accountplanmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan"
	accountplanhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/handler"
	accountrulesmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules"
	accountruleshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountrules/handler"
	activitymodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/activity"
	activityhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/activity/handler"
	agentsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/agents"
	agentshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/agents/handler"
	aimodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai"
	aihandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/handler"
	doctasksmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/doctasks"
	excelimportmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport"
	excelimporthandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/handler"
	logsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/logs"
	logshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/logs/handler"
	periodsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods"
	periodshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/periods/handler"
	receiptsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts"
	receiptshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/handler"
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
	periods     *periodshandler.Handler
	summaries   *summarieshandler.Handler
	agents      *agentshandler.Handler
	doctasks    *doctasksmodule.Handler
	// accountrules: hesap kodu kural motoru (issue #25).
	accountrules *accountruleshandler.Handler
	vat          *vatforecasthandler.Handler
	// engagements resolves {engagementUuid} for engagement-scoped module routes.
	engagements engagementctx.Resolver
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
		accountplanmodule.Module{H: h.accountplan, Engagements: h.engagements},
		periodsmodule.Module{H: h.periods, Engagements: h.engagements},
		summariesmodule.Module{H: h.summaries, Engagements: h.engagements},
		agentsmodule.Module{H: h.agents},
		doctasksmodule.Module{H: h.doctasks, Engagements: h.engagements, Taxpayers: h.taxpayers},
		accountrulesmodule.Module{H: h.accountrules, Engagements: h.engagements},
		vatforecastmodule.Module{H: h.vat, Engagements: h.engagements},
	}
}
