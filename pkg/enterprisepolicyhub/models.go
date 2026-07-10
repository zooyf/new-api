package enterprisepolicyhub

import "gorm.io/gorm"

const (
	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
	StatusPending  = "pending"
	StatusSynced   = "synced"
	StatusFailed   = "failed"

	BudgetSourceManual  = "manual"
	BudgetSourcePolicy  = "policy"
	BudgetPeriodCustom  = "custom"
	BudgetPeriodDaily   = "daily"
	BudgetPeriodMonthly = "monthly"
	BudgetBlockActive   = "active"
	BudgetBlockReleased = "released"

	HubRoleSuperAdmin   = "hub_super_admin"
	HubRoleOrgAdmin     = "hub_org_admin"
	HubRoleKeyAdmin     = "hub_key_admin"
	HubRoleFinanceAdmin = "hub_finance_admin"
	HubRoleAuditor      = "hub_auditor"

	SyncStatusPending = "pending"
	SyncStatusRunning = "running"
	SyncStatusDone    = "succeeded"
	SyncStatusFailed  = "failed"

	OrgTypeCompany      = "company"
	OrgTypeBusinessUnit = "business_unit"
	OrgTypeDepartment   = "department"
	OrgTypeTeam         = "team"
	OrgTypeProject      = "project"
	OrgTypeCostCenter   = "cost_center"
)

type OrgUnit struct {
	ID              int            `json:"id" gorm:"primaryKey"`
	ParentID        *int           `json:"parent_id" gorm:"index"`
	Path            string         `json:"path" gorm:"type:varchar(512);index"`
	Name            string         `json:"name" gorm:"type:varchar(128);not null"`
	Code            string         `json:"code" gorm:"type:varchar(128);index"`
	Type            string         `json:"type" gorm:"type:varchar(32);index"`
	Status          string         `json:"status" gorm:"type:varchar(32);index;default:'enabled'"`
	OwnerAdminID    int            `json:"owner_admin_id" gorm:"index;default:0"`
	DefaultPolicyID int            `json:"default_policy_id" gorm:"index;default:0"`
	DefaultGroup    string         `json:"default_group" gorm:"type:varchar(64);default:''"`
	NewAPIUserID    int            `json:"newapi_user_id" gorm:"column:new_api_user_id;index;default:0"`
	CreatedAt       int64          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

func (OrgUnit) TableName() string {
	return "eph_org_units"
}

type OrgUnitClosure struct {
	AncestorID   int   `json:"ancestor_id" gorm:"primaryKey;autoIncrement:false"`
	DescendantID int   `json:"descendant_id" gorm:"primaryKey;autoIncrement:false"`
	Depth        int   `json:"depth"`
	CreatedAt    int64 `json:"created_at" gorm:"autoCreateTime"`
}

func (OrgUnitClosure) TableName() string {
	return "eph_org_unit_closure"
}

type HubAdminBinding struct {
	ID             int            `json:"id" gorm:"primaryKey"`
	NewAPIUserID   int            `json:"newapi_user_id" gorm:"column:new_api_user_id;uniqueIndex;not null"`
	NewAPIUsername string         `json:"newapi_username" gorm:"column:new_api_username;type:varchar(64);index"`
	HubRole        string         `json:"hub_role" gorm:"type:varchar(32);index"`
	ScopeOrgUnitID int            `json:"scope_org_unit_id" gorm:"index;default:0"`
	Status         string         `json:"status" gorm:"type:varchar(32);index;default:'enabled'"`
	CreatedAt      int64          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

func (HubAdminBinding) TableName() string {
	return "eph_hub_admin_bindings"
}

type Policy struct {
	ID                 int            `json:"id" gorm:"primaryKey"`
	Name               string         `json:"name" gorm:"type:varchar(128);uniqueIndex;not null"`
	Description        string         `json:"description" gorm:"type:text"`
	DefaultGroup       string         `json:"default_group" gorm:"type:varchar(64);default:''"`
	AllowedModels      string         `json:"allowed_models" gorm:"type:text"`
	DeniedModels       string         `json:"denied_models" gorm:"type:text"`
	MonthlyBudgetQuota int            `json:"monthly_budget_quota" gorm:"default:0"`
	DailyBudgetQuota   int            `json:"daily_budget_quota" gorm:"default:0"`
	Currency           string         `json:"currency" gorm:"type:varchar(16);default:'quota'"`
	KeyDefaultQuota    int            `json:"key_default_quota" gorm:"default:0"`
	InheritMode        string         `json:"inherit_mode" gorm:"type:varchar(32);default:'intersect'"`
	Status             string         `json:"status" gorm:"type:varchar(32);index;default:'enabled'"`
	CreatedAt          int64          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Policy) TableName() string {
	return "eph_policies"
}

type EnterpriseKey struct {
	ID                     int            `json:"id" gorm:"primaryKey"`
	Name                   string         `json:"name" gorm:"type:varchar(128);index;not null"`
	OrgUnitID              int            `json:"org_unit_id" gorm:"index;default:0"`
	ProjectID              int            `json:"project_id" gorm:"index;default:0"`
	CostCenterID           int            `json:"cost_center_id" gorm:"index;default:0"`
	PolicyID               int            `json:"policy_id" gorm:"index;default:0"`
	NewAPIUserID           int            `json:"newapi_user_id" gorm:"column:new_api_user_id;index;default:0"`
	ConfiguredNewAPIUserID int            `json:"configured_newapi_user_id" gorm:"column:configured_new_api_user_id;index;default:0"`
	NewAPIUserMode         string         `json:"newapi_user_mode" gorm:"column:new_api_user_mode;type:varchar(16);index"`
	NewAPITokenID          int            `json:"newapi_token_id" gorm:"column:new_api_token_id;uniqueIndex;default:0"`
	NewAPITokenName        string         `json:"newapi_token_name" gorm:"column:new_api_token_name;type:varchar(128);index"`
	KeyFingerprint         string         `json:"key_fingerprint" gorm:"type:varchar(64);index"`
	AppliedKeyQuota        int            `json:"applied_key_quota" gorm:"default:0"`
	Status                 string         `json:"status" gorm:"type:varchar(32);index;default:'enabled'"`
	SyncStatus             string         `json:"sync_status" gorm:"type:varchar(32);index;default:'pending'"`
	Environment            string         `json:"environment" gorm:"type:varchar(32);index;default:'prod'"`
	Purpose                string         `json:"purpose" gorm:"type:text"`
	Contact                string         `json:"contact" gorm:"type:varchar(128)"`
	CreatedBy              int            `json:"created_by" gorm:"index;default:0"`
	CreatedAt              int64          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt              int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt              gorm.DeletedAt `json:"-" gorm:"index"`
}

func (EnterpriseKey) TableName() string {
	return "eph_enterprise_keys"
}

type BudgetAccount struct {
	ID          int            `json:"id" gorm:"primaryKey"`
	ScopeType   string         `json:"scope_type" gorm:"type:varchar(32);index:idx_eph_budget_scope,priority:1"`
	ScopeID     int            `json:"scope_id" gorm:"index:idx_eph_budget_scope,priority:2"`
	PeriodStart int64          `json:"period_start" gorm:"index"`
	PeriodEnd   int64          `json:"period_end" gorm:"index"`
	BudgetQuota int            `json:"budget_quota" gorm:"default:0"`
	UsedQuota   int            `json:"used_quota" gorm:"default:0"`
	Currency    string         `json:"currency" gorm:"type:varchar(16);default:'quota'"`
	Status      string         `json:"status" gorm:"type:varchar(32);index;default:'enabled'"`
	SourceType  string         `json:"source_type" gorm:"type:varchar(32);index"`
	SourceID    int            `json:"source_id" gorm:"index;default:0"`
	PeriodKind  string         `json:"period_kind" gorm:"type:varchar(32);index"`
	ManagedKey  *string        `json:"managed_key,omitempty" gorm:"type:varchar(255);uniqueIndex"`
	CreatedAt   int64          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (BudgetAccount) TableName() string {
	return "eph_budget_accounts"
}

type BudgetTransaction struct {
	ID              int    `json:"id" gorm:"primaryKey"`
	BudgetAccountID int    `json:"budget_account_id" gorm:"index;uniqueIndex:idx_eph_budget_log,priority:1"`
	EnterpriseKeyID int    `json:"enterprise_key_id" gorm:"index"`
	NewAPILogID     int    `json:"newapi_log_id" gorm:"column:new_api_log_id;index;uniqueIndex:idx_eph_budget_log,priority:2"`
	SourceType      string `json:"source_type" gorm:"type:varchar(32);index"`
	SourceID        int    `json:"source_id" gorm:"index"`
	Quota           int    `json:"quota"`
	Direction       string `json:"direction" gorm:"type:varchar(32);index"`
	CreatedAt       int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (BudgetTransaction) TableName() string {
	return "eph_budget_transactions"
}

type BudgetKeyBlock struct {
	ID              int    `json:"id" gorm:"primaryKey"`
	BudgetAccountID int    `json:"budget_account_id" gorm:"uniqueIndex:idx_eph_budget_key_block,priority:1;index"`
	EnterpriseKeyID int    `json:"enterprise_key_id" gorm:"uniqueIndex:idx_eph_budget_key_block,priority:2;index"`
	Status          string `json:"status" gorm:"type:varchar(32);index"`
	CreatedAt       int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       int64  `json:"updated_at" gorm:"autoUpdateTime"`
	ReleasedAt      int64  `json:"released_at" gorm:"default:0"`
}

func (BudgetKeyBlock) TableName() string {
	return "eph_budget_key_blocks"
}

type OrganizationUsageLedger struct {
	ID              int     `json:"id" gorm:"primaryKey"`
	NewAPILogID     int     `json:"newapi_log_id" gorm:"column:new_api_log_id;uniqueIndex;not null"`
	NewAPITokenID   int     `json:"newapi_token_id" gorm:"column:new_api_token_id;index"`
	EnterpriseKeyID int     `json:"enterprise_key_id" gorm:"index"`
	OrgUnitID       int     `json:"org_unit_id" gorm:"index"`
	ProjectID       int     `json:"project_id" gorm:"index"`
	CostCenterID    int     `json:"cost_center_id" gorm:"index"`
	ModelName       string  `json:"model_name" gorm:"type:varchar(191);index"`
	ChannelID       int     `json:"channel_id" gorm:"index"`
	UseGroup        string  `json:"use_group" gorm:"type:varchar(64);index"`
	Quota           int     `json:"quota"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency" gorm:"type:varchar(16);default:'quota'"`
	CreatedAt       int64   `json:"created_at" gorm:"index"`
	ImportedAt      int64   `json:"imported_at" gorm:"autoCreateTime"`
}

func (OrganizationUsageLedger) TableName() string {
	return "eph_organization_usage_ledger"
}

type NewAPISyncJob struct {
	ID           int            `json:"id" gorm:"primaryKey"`
	EntityType   string         `json:"entity_type" gorm:"type:varchar(64);index"`
	EntityID     int            `json:"entity_id" gorm:"index"`
	Operation    string         `json:"operation" gorm:"type:varchar(64);index"`
	Status       string         `json:"status" gorm:"type:varchar(32);index"`
	ErrorMessage string         `json:"error_message" gorm:"type:text"`
	RetryCount   int            `json:"retry_count" gorm:"default:0"`
	CreatedAt    int64          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

func (NewAPISyncJob) TableName() string {
	return "eph_newapi_sync_jobs"
}

type AuditLog struct {
	ID                int    `json:"id" gorm:"primaryKey"`
	AdminNewAPIUserID int    `json:"admin_newapi_user_id" gorm:"column:admin_new_api_user_id;index"`
	AdminUsername     string `json:"admin_username" gorm:"type:varchar(128);index"`
	AdminRole         int    `json:"admin_role" gorm:"index"`
	HubRole           string `json:"hub_role" gorm:"type:varchar(32);index"`
	Action            string `json:"action" gorm:"type:varchar(128);index"`
	TargetType        string `json:"target_type" gorm:"type:varchar(64);index"`
	TargetID          int    `json:"target_id" gorm:"index"`
	BeforeJSON        string `json:"before_json" gorm:"type:text"`
	AfterJSON         string `json:"after_json" gorm:"type:text"`
	IP                string `json:"ip" gorm:"type:varchar(64);index"`
	UserAgent         string `json:"user_agent" gorm:"type:text"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AuditLog) TableName() string {
	return "eph_audit_logs"
}

type Setting struct {
	Key       string `json:"key" gorm:"primaryKey;type:varchar(128)"`
	Value     string `json:"value" gorm:"type:text"`
	UpdatedAt int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (Setting) TableName() string {
	return "eph_settings"
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&OrgUnit{},
		&OrgUnitClosure{},
		&HubAdminBinding{},
		&Policy{},
		&EnterpriseKey{},
		&BudgetAccount{},
		&BudgetTransaction{},
		&BudgetKeyBlock{},
		&OrganizationUsageLedger{},
		&NewAPISyncJob{},
		&AuditLog{},
		&Setting{},
	)
}
