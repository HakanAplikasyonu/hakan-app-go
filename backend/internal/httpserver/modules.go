package httpserver

import (
	accessmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/access"
	accesshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/access/handler"
	accountplanmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan"
	accountplanhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/accountplan/handler"
	activitymodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/activity"
	activityhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/activity/handler"
	aimodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai"
	aihandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/ai/handler"
	excelimportmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport"
	excelimporthandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/excelimport/handler"
	logsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/logs"
	logshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/logs/handler"
	receiptsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts"
	receiptshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/receipts/handler"
	reviewmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review"
	reviewhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/review/handler"
	searchmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/search"
	searchhandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/search/handler"
	warningsmodule "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings"
	warningshandler "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/modules/warnings/handler"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/engagementctx"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/module"
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
	// engagements resolves {engagementUuid} for engagement-scoped module routes.
	engagements engagementctx.Resolver
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
	}
}
