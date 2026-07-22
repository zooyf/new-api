package resellerhub

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const TablePrefix = "reseller_hub_"

const (
	ResellerStatusActive    = "active"
	ResellerStatusSuspended = "suspended"
	ResellerStatusClosed    = "closed"

	MembershipRoleAdmin  = "reseller_admin"
	MembershipRoleViewer = "reseller_viewer"

	MembershipStatusActive   = "active"
	MembershipStatusDisabled = "disabled"

	CustomerStatusActive    = "active"
	CustomerStatusSuspended = "suspended"
	CustomerStatusClosed    = "closed"

	CustomerTokenStatusActive   = "active"
	CustomerTokenStatusRetiring = "retiring"
	CustomerTokenStatusRetired  = "retired"

	QuotaTargetUser  = "user_quota"
	QuotaTargetToken = "token_quota"

	QuotaOperationAdd      = "add"
	QuotaOperationSubtract = "subtract"

	QuotaLedgerStatusPrepared          = "prepared"
	QuotaLedgerStatusQuotaApplied      = "quota_applied"
	QuotaLedgerStatusReconcileRequired = "reconcile_required"
	QuotaLedgerStatusApplied           = "applied"
	QuotaLedgerStatusFailed            = "failed"
	QuotaLedgerStatusCompensated       = "compensated"
	QuotaLedgerStatusReversed          = "reversed"
)

// Reseller owns one quota-carrier user and defines the default customer discount.
// Core user references are scalar IDs by design so Sidecar migrations cannot
// create or alter constraints on new-api tables.
type Reseller struct {
	ID                 int            `json:"id" gorm:"column:id;primaryKey"`
	Code               string         `json:"code" gorm:"column:code;type:varchar(64);not null;uniqueIndex:uidx_reseller_hub_reseller_code"`
	Name               string         `json:"name" gorm:"column:name;type:varchar(128);not null"`
	Status             string         `json:"status" gorm:"column:status;type:varchar(32);not null;index:idx_reseller_hub_reseller_status"`
	DefaultDiscountBPS int            `json:"default_discount_bps" gorm:"column:default_discount_bps;not null"`
	QuotaCarrierUserID int            `json:"quota_carrier_user_id" gorm:"column:quota_carrier_user_id;not null;uniqueIndex:uidx_reseller_hub_quota_carrier_user"`
	CreatedByUserID    int            `json:"created_by_user_id" gorm:"column:created_by_user_id;not null;index:idx_reseller_hub_reseller_creator"`
	CreatedAt          int64          `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          int64          `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index:idx_reseller_hub_reseller_deleted"`
}

func (Reseller) TableName() string {
	return "reseller_hub_resellers"
}

type Membership struct {
	ID           int    `json:"id" gorm:"column:id;primaryKey"`
	ResellerID   int    `json:"reseller_id" gorm:"column:reseller_id;not null;index:idx_reseller_hub_membership_reseller"`
	NewAPIUserID int    `json:"new_api_user_id" gorm:"column:new_api_user_id;not null;uniqueIndex:uidx_reseller_hub_membership_user"`
	Role         string `json:"role" gorm:"column:role;type:varchar(32);not null;index:idx_reseller_hub_membership_role"`
	Status       string `json:"status" gorm:"column:status;type:varchar(32);not null;index:idx_reseller_hub_membership_status"`
	CreatedAt    int64  `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    int64  `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (Membership) TableName() string {
	return "reseller_hub_memberships"
}

type Customer struct {
	ID                   int            `json:"id" gorm:"column:id;primaryKey"`
	ResellerID           int            `json:"reseller_id" gorm:"column:reseller_id;not null;index:idx_reseller_hub_customer_reseller;uniqueIndex:uidx_reseller_hub_customer_external_ref,priority:1"`
	ActiveTokenMappingID *int           `json:"active_token_mapping_id,omitempty" gorm:"column:active_token_mapping_id;uniqueIndex:uidx_reseller_hub_customer_active_token"`
	DisplayName          string         `json:"display_name" gorm:"column:display_name;type:varchar(128);not null;index:idx_reseller_hub_customer_name"`
	ExternalRef          string         `json:"external_ref" gorm:"column:external_ref;type:varchar(128);uniqueIndex:uidx_reseller_hub_customer_external_ref,priority:2"`
	DiscountBPS          *int           `json:"discount_bps,omitempty" gorm:"column:discount_bps"`
	Status               string         `json:"status" gorm:"column:status;type:varchar(32);not null;index:idx_reseller_hub_customer_status"`
	CreatedByUserID      int            `json:"created_by_user_id" gorm:"column:created_by_user_id;not null;index:idx_reseller_hub_customer_creator"`
	CreatedAt            int64          `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt            int64          `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt            gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index:idx_reseller_hub_customer_deleted"`
}

func (Customer) TableName() string {
	return "reseller_hub_customers"
}

// BeforeCreate keeps an omitted external reference as SQL NULL. All supported
// databases permit multiple NULL values in the composite unique index, while a
// non-empty reference remains unique inside one reseller.
func (customer *Customer) BeforeCreate(tx *gorm.DB) error {
	if customer.ExternalRef == "" {
		tx.Statement.Omit("external_ref")
	}
	return nil
}

type CustomerToken struct {
	ID                 int    `json:"id" gorm:"column:id;primaryKey"`
	ResellerID         int    `json:"reseller_id" gorm:"column:reseller_id;not null;index:idx_reseller_hub_customer_token_reseller"`
	CustomerID         int    `json:"customer_id" gorm:"column:customer_id;not null;index:idx_reseller_hub_customer_token_customer"`
	NewAPITokenID      int    `json:"new_api_token_id" gorm:"column:new_api_token_id;not null;uniqueIndex:uidx_reseller_hub_token_id"`
	QuotaCarrierUserID int    `json:"quota_carrier_user_id" gorm:"column:quota_carrier_user_id;not null;index:idx_reseller_hub_customer_token_carrier"`
	Status             string `json:"status" gorm:"column:status;type:varchar(32);not null;index:idx_reseller_hub_customer_token_status"`
	EffectiveAt        int64  `json:"effective_at" gorm:"column:effective_at;not null;index:idx_reseller_hub_customer_token_effective"`
	EndedAt            *int64 `json:"ended_at,omitempty" gorm:"column:ended_at;index:idx_reseller_hub_customer_token_ended"`
	CreatedByUserID    int    `json:"created_by_user_id" gorm:"column:created_by_user_id;not null;index:idx_reseller_hub_customer_token_creator"`
	CreatedAt          int64  `json:"created_at" gorm:"column:created_at;autoCreateTime"`
}

func (CustomerToken) TableName() string {
	return "reseller_hub_customer_tokens"
}

type DiscountVersion struct {
	ID              int    `json:"id" gorm:"column:id;primaryKey"`
	ResellerID      int    `json:"reseller_id" gorm:"column:reseller_id;not null;index:idx_reseller_hub_discount_reseller"`
	CustomerID      int    `json:"customer_id" gorm:"column:customer_id;not null;index:idx_reseller_hub_discount_customer"`
	DiscountBPS     int    `json:"discount_bps" gorm:"column:discount_bps;not null"`
	EffectiveAt     int64  `json:"effective_at" gorm:"column:effective_at;not null;index:idx_reseller_hub_discount_effective"`
	EndedAt         *int64 `json:"ended_at,omitempty" gorm:"column:ended_at;index:idx_reseller_hub_discount_ended"`
	Reason          string `json:"reason" gorm:"column:reason;type:text;not null"`
	CreatedByUserID int    `json:"created_by_user_id" gorm:"column:created_by_user_id;not null;index:idx_reseller_hub_discount_creator"`
	CreatedAt       int64  `json:"created_at" gorm:"column:created_at;autoCreateTime"`
}

func (DiscountVersion) TableName() string {
	return "reseller_hub_discount_versions"
}

type QuotaLedger struct {
	ID                        int     `json:"id" gorm:"column:id;primaryKey"`
	EventID                   string  `json:"event_id" gorm:"column:event_id;type:varchar(64);not null;uniqueIndex:uidx_reseller_hub_quota_event"`
	IdempotencyKey            string  `json:"idempotency_key" gorm:"column:idempotency_key;type:varchar(128);not null;uniqueIndex:uidx_reseller_hub_quota_idempotency,priority:2"`
	ResellerID                int     `json:"reseller_id" gorm:"column:reseller_id;not null;index:idx_reseller_hub_quota_reseller;uniqueIndex:uidx_reseller_hub_quota_idempotency,priority:1"`
	CustomerID                *int    `json:"customer_id,omitempty" gorm:"column:customer_id;index:idx_reseller_hub_quota_customer"`
	TargetType                string  `json:"target_type" gorm:"column:target_type;type:varchar(32);not null;index:idx_reseller_hub_quota_target"`
	NewAPIUserID              int     `json:"new_api_user_id" gorm:"column:new_api_user_id;not null;index:idx_reseller_hub_quota_user"`
	NewAPITokenID             *int    `json:"new_api_token_id,omitempty" gorm:"column:new_api_token_id;index:idx_reseller_hub_quota_token"`
	Operation                 string  `json:"operation" gorm:"column:operation;type:varchar(32);not null;index:idx_reseller_hub_quota_operation"`
	ReversesEventID           *string `json:"reverses_event_id,omitempty" gorm:"column:reverses_event_id;type:varchar(64);uniqueIndex:uidx_reseller_hub_quota_reversal"`
	RequestedQuota            int     `json:"requested_quota" gorm:"column:requested_quota;not null"`
	QuotaDelta                int     `json:"quota_delta" gorm:"column:quota_delta;not null"`
	QuotaBefore               int     `json:"quota_before" gorm:"column:quota_before;not null"`
	QuotaAfter                int     `json:"quota_after" gorm:"column:quota_after;not null"`
	UsedQuotaBefore           int     `json:"used_quota_before" gorm:"column:used_quota_before;not null"`
	UsedQuotaAfter            int     `json:"used_quota_after" gorm:"column:used_quota_after;not null"`
	InputUnit                 string  `json:"input_unit" gorm:"column:input_unit;type:varchar(32);not null"`
	InputAmountDecimal        string  `json:"input_amount_decimal" gorm:"column:input_amount_decimal;type:varchar(128);not null"`
	CurrencyTypeSnapshot      string  `json:"currency_type_snapshot" gorm:"column:currency_type_snapshot;type:varchar(16);not null"`
	CurrencySymbolSnapshot    string  `json:"currency_symbol_snapshot" gorm:"column:currency_symbol_snapshot;type:varchar(32);not null"`
	QuotaPerUnitSnapshot      string  `json:"quota_per_unit_snapshot" gorm:"column:quota_per_unit_snapshot;type:varchar(64);not null"`
	USDToCurrencyRateSnapshot string  `json:"usd_to_currency_rate_snapshot" gorm:"column:usd_to_currency_rate_snapshot;type:varchar(64);not null"`
	DiscountBPSSnapshot       int     `json:"discount_bps_snapshot" gorm:"column:discount_bps_snapshot;not null"`
	Status                    string  `json:"status" gorm:"column:status;type:varchar(32);not null;index:idx_reseller_hub_quota_status"`
	Reason                    string  `json:"reason" gorm:"column:reason;type:text;not null"`
	ActorUserID               int     `json:"actor_user_id" gorm:"column:actor_user_id;not null;index:idx_reseller_hub_quota_actor"`
	RequestID                 string  `json:"request_id" gorm:"column:request_id;type:varchar(191);not null;index:idx_reseller_hub_quota_request"`
	ErrorMessage              string  `json:"error_message" gorm:"column:error_message;type:text;not null"`
	CreatedAt                 int64   `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	AppliedAt                 *int64  `json:"applied_at,omitempty" gorm:"column:applied_at;index:idx_reseller_hub_quota_applied"`
}

func (QuotaLedger) TableName() string {
	return "reseller_hub_quota_ledger"
}

type AuditLog struct {
	ID          int    `json:"id" gorm:"column:id;primaryKey"`
	EventID     string `json:"event_id" gorm:"column:event_id;type:varchar(64);not null;uniqueIndex:uidx_reseller_hub_audit_event"`
	ResellerID  *int   `json:"reseller_id,omitempty" gorm:"column:reseller_id;index:idx_reseller_hub_audit_reseller"`
	ActorUserID int    `json:"actor_user_id" gorm:"column:actor_user_id;not null;index:idx_reseller_hub_audit_actor;uniqueIndex:uidx_reseller_hub_audit_idempotency,priority:1"`
	Action      string `json:"action" gorm:"column:action;type:varchar(128);not null;index:idx_reseller_hub_audit_action"`
	ObjectType  string `json:"object_type" gorm:"column:object_type;type:varchar(32);not null;index:idx_reseller_hub_audit_object,priority:1"`
	ObjectID    string `json:"object_id" gorm:"column:object_id;type:varchar(128);not null;index:idx_reseller_hub_audit_object,priority:2"`
	RequestID   string `json:"request_id" gorm:"column:request_id;type:varchar(191);not null;index:idx_reseller_hub_audit_request;uniqueIndex:uidx_reseller_hub_audit_idempotency,priority:2"`
	SourceIP    string `json:"source_ip" gorm:"column:source_ip;type:varchar(64);not null"`
	UserAgent   string `json:"user_agent" gorm:"column:user_agent;type:text;not null"`
	BeforeJSON  string `json:"before_json" gorm:"column:before_json;type:text;not null"`
	AfterJSON   string `json:"after_json" gorm:"column:after_json;type:text;not null"`
	DetailJSON  string `json:"detail_json" gorm:"column:detail_json;type:text;not null"`
	CreatedAt   int64  `json:"created_at" gorm:"column:created_at;autoCreateTime;index:idx_reseller_hub_audit_created"`
}

func (AuditLog) TableName() string {
	return "reseller_hub_audit_logs"
}

func (audit *AuditLog) BeforeCreate(_ *gorm.DB) error {
	if audit.EventID != "" {
		return nil
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	audit.EventID = eventID.String()
	return nil
}

// Lease coordinates the single Sidecar write leader. Every write verifies the
// holder and expiry in the database; FencingToken is reserved for a future
// multi-writer protocol that needs to fence work outside the database.
type Lease struct {
	Name         string `json:"name" gorm:"column:name;type:varchar(128);primaryKey"`
	HolderID     string `json:"holder_id" gorm:"column:holder_id;type:varchar(191);not null;index:idx_reseller_hub_lease_holder"`
	FencingToken int64  `json:"fencing_token" gorm:"column:fencing_token;not null"`
	ExpiresAt    int64  `json:"expires_at" gorm:"column:expires_at;not null;index:idx_reseller_hub_lease_expiry"`
	CreatedAt    int64  `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    int64  `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

func (Lease) TableName() string {
	return "reseller_hub_leases"
}

// SidecarModels is the complete migration allowlist. Keep core users, tokens,
// logs, and task models out of this list.
func SidecarModels() []any {
	return []any{
		&Reseller{},
		&Membership{},
		&Customer{},
		&CustomerToken{},
		&DiscountVersion{},
		&QuotaLedger{},
		&AuditLog{},
		&Lease{},
	}
}
