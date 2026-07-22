package resellerhub

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type schemaRequirement struct {
	model   any
	table   string
	columns []string
	indexes []string
}

var sidecarSchema = []schemaRequirement{
	{
		model: &Reseller{},
		table: "reseller_hub_resellers",
		columns: []string{
			"id", "code", "name", "status", "default_discount_bps",
			"quota_carrier_user_id", "created_by_user_id", "created_at",
			"updated_at", "deleted_at",
		},
		indexes: []string{"uidx_reseller_hub_reseller_code", "uidx_reseller_hub_quota_carrier_user"},
	},
	{
		model: &Membership{},
		table: "reseller_hub_memberships",
		columns: []string{
			"id", "reseller_id", "new_api_user_id", "role", "status", "created_at", "updated_at",
		},
		indexes: []string{"uidx_reseller_hub_membership_user"},
	},
	{
		model: &Customer{},
		table: "reseller_hub_customers",
		columns: []string{
			"id", "reseller_id", "active_token_mapping_id", "display_name", "external_ref",
			"discount_bps", "status", "created_by_user_id", "created_at", "updated_at", "deleted_at",
		},
		indexes: []string{"uidx_reseller_hub_customer_active_token", "uidx_reseller_hub_customer_external_ref"},
	},
	{
		model: &CustomerToken{},
		table: "reseller_hub_customer_tokens",
		columns: []string{
			"id", "reseller_id", "customer_id", "new_api_token_id", "quota_carrier_user_id",
			"status", "effective_at", "ended_at", "created_by_user_id", "created_at",
		},
		indexes: []string{"uidx_reseller_hub_token_id"},
	},
	{
		model: &DiscountVersion{},
		table: "reseller_hub_discount_versions",
		columns: []string{
			"id", "reseller_id", "customer_id", "discount_bps", "effective_at",
			"ended_at", "reason", "created_by_user_id", "created_at",
		},
	},
	{
		model: &QuotaLedger{},
		table: "reseller_hub_quota_ledger",
		columns: []string{
			"id", "event_id", "idempotency_key", "reseller_id", "customer_id", "target_type",
			"new_api_user_id", "new_api_token_id", "operation", "reverses_event_id",
			"requested_quota", "quota_delta", "quota_before", "quota_after", "used_quota_before",
			"used_quota_after", "input_unit", "input_amount_decimal", "currency_type_snapshot",
			"currency_symbol_snapshot", "quota_per_unit_snapshot", "usd_to_currency_rate_snapshot",
			"discount_bps_snapshot", "status", "reason", "actor_user_id", "request_id",
			"error_message", "created_at", "applied_at",
		},
		indexes: []string{"uidx_reseller_hub_quota_event", "uidx_reseller_hub_quota_idempotency", "uidx_reseller_hub_quota_reversal"},
	},
	{
		model: &AuditLog{},
		table: "reseller_hub_audit_logs",
		columns: []string{
			"id", "event_id", "reseller_id", "actor_user_id", "action", "object_type", "object_id",
			"request_id", "source_ip", "user_agent", "before_json", "after_json", "detail_json", "created_at",
		},
		indexes: []string{"uidx_reseller_hub_audit_event", "uidx_reseller_hub_audit_idempotency"},
	},
	{
		model: &Lease{},
		table: "reseller_hub_leases",
		columns: []string{
			"name", "holder_id", "fencing_token", "expires_at", "created_at", "updated_at",
		},
	},
}

// Migrate only creates or upgrades tables declared by SidecarModels. No core
// new-api model is present in the migration graph.
func Migrate(db *gorm.DB) error {
	if db == nil {
		return errors.New("reseller hub migrate: database is nil")
	}

	models := SidecarModels()
	for _, model := range models {
		statement := &gorm.Statement{DB: db}
		if err := statement.Parse(model); err != nil {
			return fmt.Errorf("reseller hub migrate: parse model: %w", err)
		}
		if statement.Schema == nil || !strings.HasPrefix(statement.Schema.Table, TablePrefix) {
			table := ""
			if statement.Schema != nil {
				table = statement.Schema.Table
			}
			return fmt.Errorf("reseller hub migrate: table %q is outside %q allowlist", table, TablePrefix)
		}
	}

	if err := db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("reseller hub migrate: %w", err)
	}
	if err := VerifySchema(db); err != nil {
		return fmt.Errorf("reseller hub migrate verification: %w", err)
	}
	return nil
}

// MigrateWithLockTimeout pins one database connection, applies the dialect's
// session-level DDL lock wait, and runs the allowlisted Sidecar migration.
func MigrateWithLockTimeout(db *gorm.DB, timeout time.Duration) error {
	if db == nil {
		return errors.New("reseller hub migrate: database is nil")
	}
	if timeout <= 0 {
		return Migrate(db)
	}
	milliseconds := timeout.Milliseconds()
	seconds := int64(timeout / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return db.Connection(func(conn *gorm.DB) error {
		switch conn.Dialector.Name() {
		case "postgres":
			if err := conn.Exec(fmt.Sprintf("SET lock_timeout = '%dms'", milliseconds)).Error; err != nil {
				return fmt.Errorf("reseller hub migrate: set PostgreSQL lock timeout: %w", err)
			}
		case "mysql":
			if err := conn.Exec("SET SESSION lock_wait_timeout = ?", seconds).Error; err != nil {
				return fmt.Errorf("reseller hub migrate: set MySQL metadata lock timeout: %w", err)
			}
			if err := conn.Exec("SET SESSION innodb_lock_wait_timeout = ?", seconds).Error; err != nil {
				return fmt.Errorf("reseller hub migrate: set MySQL row lock timeout: %w", err)
			}
		case "sqlite":
			if err := conn.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", milliseconds)).Error; err != nil {
				return fmt.Errorf("reseller hub migrate: set SQLite busy timeout: %w", err)
			}
		}
		return Migrate(conn)
	})
}

// VerifySchema is read-only and is intended for normal serve startup. It
// verifies the Sidecar tables, columns, and invariants needed for isolation and
// idempotency without issuing DDL.
func VerifySchema(db *gorm.DB) error {
	if db == nil {
		return errors.New("reseller hub schema verification: database is nil")
	}

	migrator := db.Migrator()
	for _, requirement := range sidecarSchema {
		if !strings.HasPrefix(requirement.table, TablePrefix) {
			return fmt.Errorf("reseller hub schema verification: table %q is outside %q allowlist", requirement.table, TablePrefix)
		}
		if !migrator.HasTable(requirement.model) {
			return fmt.Errorf("reseller hub schema verification: missing table %s", requirement.table)
		}
		for _, column := range requirement.columns {
			if !migrator.HasColumn(requirement.model, column) {
				return fmt.Errorf("reseller hub schema verification: missing column %s.%s", requirement.table, column)
			}
		}
		for _, index := range requirement.indexes {
			if !migrator.HasIndex(requirement.model, index) {
				return fmt.Errorf("reseller hub schema verification: missing index %s on %s", index, requirement.table)
			}
		}
	}
	return nil
}
