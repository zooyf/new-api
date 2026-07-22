package resellerhub

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func sqliteCoreSchemaSummary(t *testing.T, db *gorm.DB, table string) []string {
	t.Helper()

	type schemaObject struct {
		Type string
		Name string
		SQL  string
	}
	var objects []schemaObject
	err := db.Raw(
		"SELECT type, name, COALESCE(sql, '') AS sql FROM sqlite_master WHERE tbl_name = ? ORDER BY type, name",
		table,
	).Scan(&objects).Error
	require.NoError(t, err)

	type columnInfo struct {
		CID        int     `gorm:"column:cid"`
		Name       string  `gorm:"column:name"`
		Type       string  `gorm:"column:type"`
		NotNull    int     `gorm:"column:notnull"`
		Default    *string `gorm:"column:dflt_value"`
		PrimaryKey int     `gorm:"column:pk"`
	}
	var columns []columnInfo
	require.NoError(t, db.Raw("PRAGMA table_info("+table+")").Scan(&columns).Error)

	summary := make([]string, 0, len(objects)+len(columns))
	for _, object := range objects {
		summary = append(summary, fmt.Sprintf("object:%s:%s:%s", object.Type, object.Name, object.SQL))
	}
	for _, column := range columns {
		defaultValue := "<nil>"
		if column.Default != nil {
			defaultValue = *column.Default
		}
		summary = append(summary, fmt.Sprintf(
			"column:%d:%s:%s:%d:%s:%d",
			column.CID,
			column.Name,
			column.Type,
			column.NotNull,
			defaultValue,
			column.PrimaryKey,
		))
	}
	return summary
}

func sqliteUserTables(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var tables []string
	err := db.Raw(
		"SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name",
	).Scan(&tables).Error
	require.NoError(t, err)
	return tables
}

func TestMigrateOnlyAddsSidecarTablesAndPreservesCoreSchema(t *testing.T) {
	db := openMigrationTestDB(t)
	require.NoError(t, db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			username TEXT NOT NULL,
			quota INTEGER NOT NULL DEFAULT 0
		)
	`).Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_users_username ON users(username)").Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE tokens (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			key TEXT NOT NULL,
			remain_quota INTEGER NOT NULL DEFAULT 0,
			used_quota INTEGER NOT NULL DEFAULT 0
		)
	`).Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX idx_tokens_key ON tokens(key)").Error)

	usersBefore := sqliteCoreSchemaSummary(t, db, "users")
	tokensBefore := sqliteCoreSchemaSummary(t, db, "tokens")
	tablesBefore := sqliteUserTables(t, db)

	require.NoError(t, Migrate(db))
	require.NoError(t, VerifySchema(db))

	assert.Equal(t, usersBefore, sqliteCoreSchemaSummary(t, db, "users"))
	assert.Equal(t, tokensBefore, sqliteCoreSchemaSummary(t, db, "tokens"))

	tablesAfter := sqliteUserTables(t, db)
	beforeSet := make(map[string]struct{}, len(tablesBefore))
	for _, table := range tablesBefore {
		beforeSet[table] = struct{}{}
	}
	var added []string
	for _, table := range tablesAfter {
		if _, existed := beforeSet[table]; !existed {
			added = append(added, table)
			assert.Truef(t, strings.HasPrefix(table, TablePrefix), "migration added non-Sidecar table %q", table)
		}
	}

	expected := make([]string, 0, len(sidecarSchema))
	for _, requirement := range sidecarSchema {
		expected = append(expected, requirement.table)
	}
	sort.Strings(expected)
	sort.Strings(added)
	assert.Equal(t, expected, added)
}

func TestMigrateUniqueConstraints(t *testing.T) {
	db := openMigrationTestDB(t)
	require.NoError(t, Migrate(db))

	resellerA := Reseller{
		Code: "reseller-a", Name: "Reseller A", Status: ResellerStatusActive,
		DefaultDiscountBPS: 8500, QuotaCarrierUserID: 101, CreatedByUserID: 1,
	}
	resellerB := Reseller{
		Code: "reseller-b", Name: "Reseller B", Status: ResellerStatusActive,
		DefaultDiscountBPS: 9000, QuotaCarrierUserID: 102, CreatedByUserID: 1,
	}
	require.NoError(t, db.Create(&resellerA).Error)
	require.NoError(t, db.Create(&resellerB).Error)
	require.NoError(t, db.Create(&Customer{
		ResellerID: resellerA.ID, DisplayName: "No external ref 1",
		Status: CustomerStatusActive, CreatedByUserID: 1,
	}).Error)
	require.NoError(t, db.Create(&Customer{
		ResellerID: resellerA.ID, DisplayName: "No external ref 2",
		Status: CustomerStatusActive, CreatedByUserID: 1,
	}).Error)
	require.NoError(t, db.Create(&Customer{
		ResellerID: resellerA.ID, DisplayName: "External ref owner", ExternalRef: "customer-001",
		Status: CustomerStatusActive, CreatedByUserID: 1,
	}).Error)
	assert.Error(t, db.Create(&Customer{
		ResellerID: resellerA.ID, DisplayName: "Duplicate external ref", ExternalRef: "customer-001",
		Status: CustomerStatusActive, CreatedByUserID: 1,
	}).Error)
	require.NoError(t, db.Create(&Customer{
		ResellerID: resellerB.ID, DisplayName: "Same ref in another reseller", ExternalRef: "customer-001",
		Status: CustomerStatusActive, CreatedByUserID: 1,
	}).Error)

	duplicateCode := Reseller{
		Code: "reseller-a", Name: "Duplicate", Status: ResellerStatusActive,
		DefaultDiscountBPS: 8000, QuotaCarrierUserID: 103, CreatedByUserID: 1,
	}
	assert.Error(t, db.Create(&duplicateCode).Error)
	duplicateCarrier := Reseller{
		Code: "reseller-c", Name: "Duplicate carrier", Status: ResellerStatusActive,
		DefaultDiscountBPS: 8000, QuotaCarrierUserID: 101, CreatedByUserID: 1,
	}
	assert.Error(t, db.Create(&duplicateCarrier).Error)

	require.NoError(t, db.Create(&Membership{
		ResellerID: resellerA.ID, NewAPIUserID: 201,
		Role: MembershipRoleAdmin, Status: MembershipStatusActive,
	}).Error)
	assert.Error(t, db.Create(&Membership{
		ResellerID: resellerB.ID, NewAPIUserID: 201,
		Role: MembershipRoleViewer, Status: MembershipStatusActive,
	}).Error)

	require.NoError(t, db.Create(&CustomerToken{
		ResellerID: resellerA.ID, CustomerID: 301, NewAPITokenID: 401,
		QuotaCarrierUserID: 101, Status: CustomerTokenStatusActive,
		EffectiveAt: 1, CreatedByUserID: 1,
	}).Error)
	assert.Error(t, db.Create(&CustomerToken{
		ResellerID: resellerB.ID, CustomerID: 302, NewAPITokenID: 401,
		QuotaCarrierUserID: 102, Status: CustomerTokenStatusActive,
		EffectiveAt: 1, CreatedByUserID: 1,
	}).Error)

	ledger := QuotaLedger{
		EventID: "0190-event-1", IdempotencyKey: "quota-adjustment-1", ResellerID: resellerA.ID,
		TargetType: QuotaTargetToken, NewAPIUserID: 101, Operation: QuotaOperationAdd,
		RequestedQuota: 500000, QuotaDelta: 500000, QuotaBefore: 0, QuotaAfter: 500000,
		InputUnit: "quota", InputAmountDecimal: "500000", CurrencyTypeSnapshot: "USD",
		CurrencySymbolSnapshot: "$", QuotaPerUnitSnapshot: "500000",
		USDToCurrencyRateSnapshot: "1", DiscountBPSSnapshot: 8500,
		Status: QuotaLedgerStatusApplied, Reason: "initial allocation", ActorUserID: 1,
		RequestID: "request-1", ErrorMessage: "",
	}
	require.NoError(t, db.Create(&ledger).Error)

	duplicateIdempotency := ledger
	duplicateIdempotency.ID = 0
	duplicateIdempotency.EventID = "0190-event-2"
	assert.Error(t, db.Create(&duplicateIdempotency).Error)

	sameKeyDifferentReseller := ledger
	sameKeyDifferentReseller.ID = 0
	sameKeyDifferentReseller.EventID = "0190-event-3"
	sameKeyDifferentReseller.ResellerID = resellerB.ID
	require.NoError(t, db.Create(&sameKeyDifferentReseller).Error)

	duplicateEvent := ledger
	duplicateEvent.ID = 0
	duplicateEvent.IdempotencyKey = "quota-adjustment-2"
	duplicateEvent.ResellerID = resellerB.ID
	assert.Error(t, db.Create(&duplicateEvent).Error)

	auditA := AuditLog{ActorUserID: 1, Action: "customer.create", ObjectType: "customer", ObjectID: "1", RequestID: "request-a", DetailJSON: "{}"}
	auditB := AuditLog{ActorUserID: 1, Action: "customer.create", ObjectType: "customer", ObjectID: "2", RequestID: "request-b", DetailJSON: "{}"}
	require.NoError(t, db.Create(&auditA).Error)
	require.NoError(t, db.Create(&auditB).Error)
	assert.NotEmpty(t, auditA.EventID)
	assert.NotEmpty(t, auditB.EventID)
	assert.NotEqual(t, auditA.EventID, auditB.EventID)
	duplicateAuditRequest := AuditLog{ActorUserID: 1, Action: "customer.create", ObjectType: "customer", ObjectID: "3", RequestID: "request-a", DetailJSON: "{}"}
	assert.Error(t, db.Create(&duplicateAuditRequest).Error)
	sameRequestOtherActor := AuditLog{ActorUserID: 2, Action: "customer.create", ObjectType: "customer", ObjectID: "4", RequestID: "request-a", DetailJSON: "{}"}
	require.NoError(t, db.Create(&sameRequestOtherActor).Error)
}

func TestVerifySchemaDetectsMissingSchema(t *testing.T) {
	db := openMigrationTestDB(t)
	err := VerifySchema(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing table reseller_hub_resellers")

	require.NoError(t, Migrate(db))
	require.NoError(t, db.Migrator().DropTable(&QuotaLedger{}))
	require.NoError(t, db.Exec("CREATE TABLE reseller_hub_quota_ledger (id INTEGER PRIMARY KEY)").Error)

	err = VerifySchema(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing column reseller_hub_quota_ledger.event_id")
}

func TestMigrateWithLockTimeoutSupportsSQLite(t *testing.T) {
	db := openMigrationTestDB(t)
	require.NoError(t, MigrateWithLockTimeout(db, 2*time.Second))
	require.NoError(t, VerifySchema(db))
}
