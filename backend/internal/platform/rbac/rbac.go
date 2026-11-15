package rbac

// Role slugs (seeded).
const (
	RoleSuperAdmin = "super_admin"
	RoleCMSUser    = "cms_user"
)

// Permission slugs (seeded).
const (
	PermPlatformUsersRead                 = "platform.users.read"
	PermPlatformUsersWrite                = "platform.users.write"
	PermPlatformUsersExport               = "platform.users.export"
	PermPlatformUsersImport               = "platform.users.import"
	PermPlatformUsersBulkDisable          = "platform.users.bulk.disable"
	PermPlatformUsersBulkEnable           = "platform.users.bulk.enable"
	PermPlatformUsersImpersonate          = "platform.users.impersonate"
	PermPlatformRolesRead                 = "platform.roles.read"
	PermPlatformRolesWrite                = "platform.roles.write"
	PermPlatformRolesExport               = "platform.roles.export"
	PermPlatformRolesImport               = "platform.roles.import"
	PermPlatformRolesBulkDelete           = "platform.roles.bulk.delete"
	PermPlatformOrganizationsRead         = "platform.organizations.read"
	PermPlatformOrganizationsWrite        = "platform.organizations.write"
	PermPlatformTaxpayersRead             = "platform.taxpayers.read"
	PermPlatformTaxpayersWrite            = "platform.taxpayers.write"
	PermPlatformBulkRead                  = "platform.bulk.read"
	PermPlatformNotificationsRead         = "platform.notifications.read"
	PermPlatformNotificationsReadAll      = "platform.notifications.read_all"
	PermPlatformNotificationsExport       = "platform.notifications.export"
	PermPlatformSettingsRead              = "platform.settings.read"
	PermPlatformSettingsWrite             = "platform.settings.write"
	PermPlatformActivityRead              = "platform.activity.read"
	PermPlatformImportsRead               = "platform.imports.read"
	PermPlatformExportsRead               = "platform.exports.read"
	PermPlatformStorageRead               = "platform.storage.read"
	PermPlatformStorageWrite              = "platform.storage.write"
	PermPlatformLogsRead                  = "platform.logs.read"
	PermPlatformLogsWrite                 = "platform.logs.write"
	PermPlatformAccessRead                = "platform.access.read"
	PermPlatformAccessWrite               = "platform.access.write"
	PermPlatformIntegrationsGitHubRead    = "platform.integrations.github.read"
	PermPlatformIntegrationsGitHubWrite   = "platform.integrations.github.write"
	PermPlatformIntegrationsGoogleRead    = "platform.integrations.google.read"
	PermPlatformIntegrationsGoogleWrite   = "platform.integrations.google.write"
	PermPlatformIntegrationsFacebookRead  = "platform.integrations.facebook.read"
	PermPlatformIntegrationsFacebookWrite = "platform.integrations.facebook.write"
	PermPlatformIntegrationsAppleRead     = "platform.integrations.apple.read"
	PermPlatformIntegrationsAppleWrite    = "platform.integrations.apple.write"
	PermPlatformAuthSettingsRead          = "platform.auth.settings.read"
	PermPlatformAuthSettingsWrite         = "platform.auth.settings.write"
	PermAuthSession                       = "auth.session"
	PermNotificationsRead                 = "notifications.read"
	PermNotificationsManage               = "notifications.manage"
	PermTenantSettingsRead                = "tenant.settings.read"
	PermTenantSettingsWrite               = "tenant.settings.write"
	PermTenantMembersRead                 = "tenant.members.read"
	PermTenantMembersWrite                = "tenant.members.write"
	PermTenantRolesRead                   = "tenant.roles.read"
	PermTenantRolesWrite                  = "tenant.roles.write"
	PermTenantEngagementsRead             = "tenant.engagements.read"
	PermTenantEngagementsWrite            = "tenant.engagements.write"
	// PermTenantEngagementsReadAll reaches every engagement of the office
	// without an assignment (issue #2).
	PermTenantEngagementsReadAll = "tenant.engagements.read_all"
	PermTenantAIUsageRead        = "tenant.ai.usage.read"
	PermTenantWarningsRead       = "tenant.warnings.read"
	PermTenantWarningsWrite      = "tenant.warnings.write"
	PermTenantWarningRulesWrite  = "tenant.warning_rules.write"
	// Fatura okuma ve UBL yükleme (issue #11).
	PermTenantInvoicesRead        = "tenant.invoices.read"
	PermTenantInvoicesWrite       = "tenant.invoices.write"
	PermTenantReviewRead          = "tenant.review.read"
	PermTenantReviewDecide        = "tenant.review.decide"
	PermTenantReviewSettingsWrite = "tenant.review.settings.write"
	// Excel içe aktarma (issue #22).
	PermTenantExcelImportsRead   = "tenant.excel_imports.read"
	PermTenantExcelImportsWrite  = "tenant.excel_imports.write"
	PermTenantExcelImportsCommit = "tenant.excel_imports.commit"
	// Fiş yakalama (issue #23).
	PermTenantReceiptsRead    = "tenant.receipts.read"
	PermTenantReceiptsWrite   = "tenant.receipts.write"
	PermTenantReceiptsApprove = "tenant.receipts.approve"
	// AI dönem özeti (issue #35).
	PermTenantSummariesRead    = "tenant.summaries.read"
	PermTenantSummariesWrite   = "tenant.summaries.write"
	PermTenantSummariesApprove = "tenant.summaries.approve"
	// Bank statements and movements (#19, #20).
	PermTenantBankRead  = "tenant.bank.read"
	PermTenantBankWrite = "tenant.bank.write"
	// Hedef hesap planı (issue #30).
	PermTenantAccountPlanRead  = "tenant.account_plan.read"
	PermTenantAccountPlanWrite = "tenant.account_plan.write"
	// Dönem yönetimi (issue #32): hazırlık durumu ve kilit, değişiklik talebi kararı.
	PermTenantPeriodsRead          = "tenant.periods.read"
	PermTenantPeriodsLock          = "tenant.periods.lock"
	PermTenantChangeRequestsDecide = "tenant.change_requests.decide"
	// Eksik belge görevleri (issue #36).
	PermTenantDocTasksRead  = "tenant.doc_tasks.read"
	PermTenantDocTasksWrite = "tenant.doc_tasks.write"
	// Hesap kodu kuralları (issue #25).
	PermTenantAccountRulesRead  = "tenant.account_rules.read"
	PermTenantAccountRulesWrite = "tenant.account_rules.write"
)

// IsSystemRole reports whether slug is a protected system role.
func IsSystemRole(slug string) bool {
	return slug == RoleSuperAdmin
}
