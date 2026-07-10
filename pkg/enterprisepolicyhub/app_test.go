package enterprisepolicyhub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newTestApp(t *testing.T) (*App, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, Migrate(db))
	return New(db, db, db, Config{}), db
}

func TestSameOriginRequest(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://llm.example.com/enterprise/api/keys", nil)
	require.NoError(t, err)
	req.Host = "llm.example.com"
	assert.True(t, sameOriginRequest(req))

	req.Header.Set("Origin", "https://llm.example.com")
	assert.True(t, sameOriginRequest(req))

	req.Header.Set("Origin", "https://evil.example.com")
	assert.False(t, sameOriginRequest(req))
}

func TestRouterRegistersRootAndBasePath(t *testing.T) {
	app, _ := newTestApp(t)
	app.config.BasePath = "/enterprise"

	var router http.Handler
	require.NotPanics(t, func() {
		router = app.Router()
	})

	for _, path := range []string{"/healthz", "/enterprise/healthz"} {
		recorder := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, path, nil)
		require.NoError(t, err)
		router.ServeHTTP(recorder, req)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "ok\n", recorder.Body.String())
	}
}

func TestReferenceDataReturnsSelectableNewAPIObjects(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}))
	require.NoError(t, db.Create(&model.User{
		Username: "alice",
		Password: "password",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
		Group:    "vip",
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:     7,
		Name:   "seedance",
		Type:   54,
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
		Group:  "vip,default",
		Models: "doubao-seedance-2-0-filter-off",
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "vip",
		Model:     "doubao-seedance-2-0-fast-filter-off",
		ChannelId: 7,
		Enabled:   true,
	}).Error)
	policy := Policy{Name: "seedance-policy", DefaultGroup: "vip", Status: StatusEnabled}
	require.NoError(t, db.Create(&policy).Error)
	org := OrgUnit{Name: "Marketing", Type: OrgTypeDepartment, Status: StatusEnabled, DefaultPolicyID: policy.ID, DefaultGroup: "vip"}
	require.NoError(t, db.Create(&org).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: org.ID, DescendantID: org.ID, Depth: 0}).Error)
	key := EnterpriseKey{Name: "marketing-key", OrgUnitID: org.ID, NewAPITokenID: 99, Status: StatusEnabled}
	require.NoError(t, db.Create(&key).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/enterprise/api/reference", nil)
	ctx.Set("hub_admin", &AdminIdentity{NewAPIUserID: 1, Username: "root", Role: common.RoleRootUser, HubRole: HubRoleSuperAdmin})

	app.referenceData(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Users          []referenceUser          `json:"users"`
			Groups         []string                 `json:"groups"`
			Channels       []referenceChannel       `json:"channels"`
			Models         []string                 `json:"models"`
			OrgUnits       []referenceOrgUnit       `json:"org_units"`
			Policies       []referencePolicy        `json:"policies"`
			EnterpriseKeys []referenceEnterpriseKey `json:"enterprise_keys"`
			BudgetTimezone string                   `json:"budget_timezone"`
		} `json:"data"`
	}
	require.NoError(t, common.DecodeJson(recorder.Body, &payload))
	require.True(t, payload.Success)
	assert.Equal(t, []referenceUser{{ID: 1, Username: "alice", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "vip"}}, payload.Data.Users)
	assert.Contains(t, payload.Data.Groups, "vip")
	assert.Contains(t, payload.Data.Groups, "default")
	assert.Equal(t, []referenceChannel{{ID: 7, Name: "seedance", Type: 54, Status: common.ChannelStatusEnabled, Group: "vip,default", Models: "doubao-seedance-2-0-filter-off"}}, payload.Data.Channels)
	assert.Contains(t, payload.Data.Models, "doubao-seedance-2-0-filter-off")
	assert.Contains(t, payload.Data.Models, "doubao-seedance-2-0-fast-filter-off")
	require.Len(t, payload.Data.OrgUnits, 1)
	assert.Equal(t, "Marketing", payload.Data.OrgUnits[0].Name)
	require.Len(t, payload.Data.Policies, 1)
	assert.Equal(t, "seedance-policy", payload.Data.Policies[0].Name)
	require.Len(t, payload.Data.EnterpriseKeys, 1)
	assert.Equal(t, "marketing-key", payload.Data.EnterpriseKeys[0].Name)
	assert.Empty(t, payload.Data.BudgetTimezone)
}

func TestDeleteOrgUnitProtectsReferencesAndCleansClosures(t *testing.T) {
	t.Run("rejects child org", func(t *testing.T) {
		app, db := newTestApp(t)
		parent := OrgUnit{Name: "Parent", Type: OrgTypeDepartment, Status: StatusEnabled, Path: "/1/"}
		require.NoError(t, db.Create(&parent).Error)
		child := OrgUnit{Name: "Child", ParentID: &parent.ID, Type: OrgTypeTeam, Status: StatusEnabled, Path: "/1/2/"}
		require.NoError(t, db.Create(&child).Error)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodDelete, "/enterprise/api/org-units/"+strconv.Itoa(parent.ID), nil)
		ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(parent.ID)}}
		ctx.Set("hub_admin", &AdminIdentity{NewAPIUserID: 1, Username: "root", Role: common.RoleRootUser, HubRole: HubRoleSuperAdmin})

		app.deleteOrgUnit(ctx)

		assert.Equal(t, http.StatusConflict, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "child org units")
	})

	t.Run("rejects enterprise key", func(t *testing.T) {
		app, db := newTestApp(t)
		org := OrgUnit{Name: "Sales", Type: OrgTypeDepartment, Status: StatusEnabled, Path: "/1/"}
		require.NoError(t, db.Create(&org).Error)
		require.NoError(t, db.Create(&EnterpriseKey{Name: "sales-prod", OrgUnitID: org.ID, Status: StatusEnabled}).Error)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodDelete, "/enterprise/api/org-units/"+strconv.Itoa(org.ID), nil)
		ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(org.ID)}}
		ctx.Set("hub_admin", &AdminIdentity{NewAPIUserID: 1, Username: "root", Role: common.RoleRootUser, HubRole: HubRoleSuperAdmin})

		app.deleteOrgUnit(ctx)

		assert.Equal(t, http.StatusConflict, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "enterprise keys")
	})

	t.Run("deletes unreferenced org and closure rows", func(t *testing.T) {
		app, db := newTestApp(t)
		org := OrgUnit{Name: "Temporary", Type: OrgTypeTeam, Status: StatusEnabled, Path: "/1/"}
		require.NoError(t, db.Create(&org).Error)
		require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: org.ID, DescendantID: org.ID, Depth: 0}).Error)

		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodDelete, "/enterprise/api/org-units/"+strconv.Itoa(org.ID), nil)
		ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(org.ID)}}
		ctx.Set("hub_admin", &AdminIdentity{NewAPIUserID: 1, Username: "root", Role: common.RoleRootUser, HubRole: HubRoleSuperAdmin})

		app.deleteOrgUnit(ctx)

		assert.Equal(t, http.StatusOK, recorder.Code)
		var closureCount int64
		require.NoError(t, db.Model(&OrgUnitClosure{}).Where("ancestor_id = ? OR descendant_id = ?", org.ID, org.ID).Count(&closureCount).Error)
		assert.Zero(t, closureCount)
		assert.ErrorIs(t, db.First(&OrgUnit{}, org.ID).Error, gorm.ErrRecordNotFound)
	})
}

func TestDeletePolicyRejectsActiveReferences(t *testing.T) {
	tests := []struct {
		name       string
		createRef  func(*testing.T, *gorm.DB, int)
		wantDetail string
	}{
		{
			name: "org unit",
			createRef: func(t *testing.T, db *gorm.DB, policyID int) {
				require.NoError(t, db.Create(&OrgUnit{Name: "Sales", Type: OrgTypeDepartment, Status: StatusEnabled, DefaultPolicyID: policyID}).Error)
			},
			wantDetail: "org units",
		},
		{
			name: "enterprise key",
			createRef: func(t *testing.T, db *gorm.DB, policyID int) {
				require.NoError(t, db.Create(&EnterpriseKey{Name: "sales-prod", PolicyID: policyID, Status: StatusEnabled}).Error)
			},
			wantDetail: "enterprise keys",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, db := newTestApp(t)
			policy := Policy{Name: "protected-" + strings.ReplaceAll(tt.name, " ", "-"), Status: StatusEnabled}
			require.NoError(t, db.Create(&policy).Error)
			tt.createRef(t, db, policy.ID)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodDelete, "/enterprise/api/policies/"+strconv.Itoa(policy.ID), nil)
			ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(policy.ID)}}

			app.deletePolicy(ctx)

			assert.Equal(t, http.StatusConflict, recorder.Code)
			assert.Contains(t, recorder.Body.String(), tt.wantDetail)
		})
	}
}

func TestUpdateEnterpriseKeyMarksItPendingForResync(t *testing.T) {
	app, db := newTestApp(t)
	key := EnterpriseKey{Name: "before", Status: StatusEnabled, SyncStatus: StatusSynced, Environment: "prod"}
	require.NoError(t, db.Create(&key).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/enterprise/api/keys/"+strconv.Itoa(key.ID), strings.NewReader(`{
		"name":"after",
		"status":"enabled",
		"environment":"prod"
	}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(key.ID)}}
	ctx.Set("hub_admin", &AdminIdentity{NewAPIUserID: 1, Username: "root", Role: common.RoleRootUser, HubRole: HubRoleSuperAdmin})

	app.updateEnterpriseKey(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var refreshed EnterpriseKey
	require.NoError(t, db.First(&refreshed, key.ID).Error)
	assert.Equal(t, "after", refreshed.Name)
	assert.Equal(t, StatusPending, refreshed.SyncStatus)
}

func TestDeleteEnterpriseKeyRevokesTokenAndPreservesUsage(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	token := model.Token{UserId: 42, Name: "sales-prod", Key: "test-token", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(&token).Error)
	key := EnterpriseKey{Name: "sales-prod", NewAPIUserID: 42, NewAPITokenID: token.Id, Status: StatusDisabled}
	require.NoError(t, db.Create(&key).Error)
	ledger := OrganizationUsageLedger{NewAPILogID: 1, NewAPITokenID: token.Id, EnterpriseKeyID: key.ID, Quota: 100}
	require.NoError(t, db.Create(&ledger).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/enterprise/api/keys/"+strconv.Itoa(key.ID), nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(key.ID)}}
	ctx.Set("hub_admin", &AdminIdentity{NewAPIUserID: 1, Username: "root", Role: common.RoleRootUser, HubRole: HubRoleSuperAdmin})

	app.deleteEnterpriseKey(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.ErrorIs(t, db.First(&EnterpriseKey{}, key.ID).Error, gorm.ErrRecordNotFound)
	assert.ErrorIs(t, db.First(&model.Token{}, token.Id).Error, gorm.ErrRecordNotFound)
	var preserved OrganizationUsageLedger
	require.NoError(t, db.First(&preserved, ledger.ID).Error)
	assert.Equal(t, key.ID, preserved.EnterpriseKeyID)
}

func TestDeleteEnterpriseKeyRejectsBudgetReference(t *testing.T) {
	app, db := newTestApp(t)
	key := EnterpriseKey{Name: "budgeted", Status: StatusDisabled}
	require.NoError(t, db.Create(&key).Error)
	require.NoError(t, db.Create(&BudgetAccount{ScopeType: "enterprise_key", ScopeID: key.ID, BudgetQuota: 1000, Status: StatusEnabled}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/enterprise/api/keys/"+strconv.Itoa(key.ID), nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(key.ID)}}
	ctx.Set("hub_admin", &AdminIdentity{NewAPIUserID: 1, Username: "root", Role: common.RoleRootUser, HubRole: HubRoleSuperAdmin})

	app.deleteEnterpriseKey(ctx)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "referenced by budgets")
	var existing EnterpriseKey
	require.NoError(t, db.First(&existing, key.ID).Error)
}

func TestEffectivePolicyForKeyMergesAncestorPolicies(t *testing.T) {
	app, db := newTestApp(t)

	rootPolicy := Policy{
		Name:               "root",
		DefaultGroup:       "root-group",
		AllowedModels:      joinCSV([]string{"gpt-4o-mini", "claude-haiku"}),
		MonthlyBudgetQuota: 1000,
		KeyDefaultQuota:    500,
		Status:             StatusEnabled,
	}
	childPolicy := Policy{
		Name:               "child",
		DefaultGroup:       "child-group",
		AllowedModels:      joinCSV([]string{"gpt-4o-mini", "doubao-lite"}),
		DeniedModels:       joinCSV([]string{"claude-haiku"}),
		MonthlyBudgetQuota: 300,
		KeyDefaultQuota:    200,
		Status:             StatusEnabled,
	}
	require.NoError(t, db.Create(&rootPolicy).Error)
	require.NoError(t, db.Create(&childPolicy).Error)

	root := OrgUnit{Name: "Company", Type: OrgTypeCompany, DefaultPolicyID: rootPolicy.ID, DefaultGroup: "org-root", Status: StatusEnabled}
	child := OrgUnit{Name: "Sales", Type: OrgTypeDepartment, DefaultPolicyID: childPolicy.ID, DefaultGroup: "org-child", Status: StatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&child).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: root.ID, DescendantID: root.ID, Depth: 0}).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: root.ID, DescendantID: child.ID, Depth: 1}).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: child.ID, DescendantID: child.ID, Depth: 0}).Error)

	effective, err := app.effectivePolicyForKey(EnterpriseKey{OrgUnitID: child.ID})
	require.NoError(t, err)
	assert.Equal(t, "child-group", effective.DefaultGroup)
	assert.Equal(t, []string{"gpt-4o-mini"}, effective.AllowedModels)
	assert.Equal(t, []string{"claude-haiku"}, effective.DeniedModels)
	assert.Equal(t, 300, effective.MonthlyBudgetQuota)
	assert.Equal(t, 200, effective.KeyDefaultQuota)
	assert.Equal(t, []int{rootPolicy.ID, childPolicy.ID}, effective.PolicyIDs)
}

func TestEffectivePolicyForKeyDeduplicatesSameAncestorPolicy(t *testing.T) {
	app, db := newTestApp(t)
	policy := Policy{Name: "shared", DefaultGroup: "shared-group", Status: StatusEnabled}
	require.NoError(t, db.Create(&policy).Error)

	root := OrgUnit{Name: "Company", Type: OrgTypeCompany, DefaultPolicyID: policy.ID, Status: StatusEnabled}
	child := OrgUnit{Name: "Engineering", Type: OrgTypeDepartment, DefaultPolicyID: policy.ID, Status: StatusEnabled}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&child).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: root.ID, DescendantID: root.ID, Depth: 0}).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: root.ID, DescendantID: child.ID, Depth: 1}).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: child.ID, DescendantID: child.ID, Depth: 0}).Error)

	effective, err := app.effectivePolicyForKey(EnterpriseKey{OrgUnitID: child.ID, PolicyID: policy.ID})
	require.NoError(t, err)
	assert.Equal(t, []int{policy.ID}, effective.PolicyIDs)
}

func TestSyncUsageImportsNewAPILogsByEnterpriseKey(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	key := EnterpriseKey{
		Name:          "dept-key",
		OrgUnitID:     0,
		ProjectID:     20,
		CostCenterID:  30,
		NewAPITokenID: 99,
		Status:        StatusEnabled,
		SyncStatus:    StatusSynced,
	}
	require.NoError(t, db.Create(&key).Error)
	budget := BudgetAccount{
		ScopeType:   "enterprise_key",
		ScopeID:     key.ID,
		BudgetQuota: 1000,
		Status:      StatusEnabled,
	}
	require.NoError(t, db.Create(&budget).Error)
	require.NoError(t, db.Create(&model.Log{
		Id:        1,
		CreatedAt: 123,
		Type:      model.LogTypeConsume,
		ModelName: "gpt-4o-mini",
		Quota:     200,
		ChannelId: 5,
		TokenId:   key.NewAPITokenID,
		Group:     "dept-sales",
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		Id:        2,
		CreatedAt: 124,
		Type:      model.LogTypeRefund,
		ModelName: "gpt-4o-mini",
		Quota:     50,
		ChannelId: 5,
		TokenId:   key.NewAPITokenID,
		Group:     "dept-sales",
	}).Error)

	result, err := app.SyncUsage(100)
	require.NoError(t, err)
	assert.Equal(t, 2, result.ScannedLogs)
	assert.Equal(t, 2, result.ImportedLedgers)
	assert.Equal(t, 2, result.LastLogID)

	var ledger OrganizationUsageLedger
	require.NoError(t, db.First(&ledger, "new_api_log_id = ?", 1).Error)
	assert.Equal(t, key.ID, ledger.EnterpriseKeyID)
	assert.Equal(t, 200, ledger.Quota)
	assert.Equal(t, "gpt-4o-mini", ledger.ModelName)

	var refundLedger OrganizationUsageLedger
	require.NoError(t, db.First(&refundLedger, "new_api_log_id = ?", 2).Error)
	assert.Equal(t, key.ID, refundLedger.EnterpriseKeyID)
	assert.Equal(t, -50, refundLedger.Quota)

	var refreshed BudgetAccount
	require.NoError(t, db.First(&refreshed, budget.ID).Error)
	assert.Equal(t, 150, refreshed.UsedQuota)

	var transactions []BudgetTransaction
	require.NoError(t, db.Order("new_api_log_id asc").Find(&transactions).Error)
	require.Len(t, transactions, 2)
	assert.Equal(t, "consume", transactions[0].Direction)
	assert.Equal(t, 200, transactions[0].Quota)
	assert.Equal(t, "refund", transactions[1].Direction)
	assert.Equal(t, -50, transactions[1].Quota)
}

func TestSyncUsageRepairsMissingBudgetTransactionForExistingLedger(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	key := EnterpriseKey{Name: "retry-key", NewAPITokenID: 101, Status: StatusEnabled, SyncStatus: StatusSynced}
	require.NoError(t, db.Create(&key).Error)
	budget := BudgetAccount{ScopeType: "enterprise_key", ScopeID: key.ID, BudgetQuota: 1000, Status: StatusEnabled}
	require.NoError(t, db.Create(&budget).Error)
	logRow := model.Log{Id: 10, CreatedAt: 123, Type: model.LogTypeConsume, Quota: 250, TokenId: key.NewAPITokenID}
	require.NoError(t, db.Create(&logRow).Error)
	ledger := OrganizationUsageLedger{
		NewAPILogID:     logRow.Id,
		NewAPITokenID:   key.NewAPITokenID,
		EnterpriseKeyID: key.ID,
		Quota:           logRow.Quota,
		CreatedAt:       logRow.CreatedAt,
	}
	require.NoError(t, db.Create(&ledger).Error)

	result, err := app.SyncUsage(100)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ImportedLedgers)
	assert.Equal(t, 1, result.SkippedLogs)

	var refreshed BudgetAccount
	require.NoError(t, db.First(&refreshed, budget.ID).Error)
	assert.Equal(t, 250, refreshed.UsedQuota)
	var transaction BudgetTransaction
	require.NoError(t, db.First(&transaction, "budget_account_id = ? AND new_api_log_id = ?", budget.ID, logRow.Id).Error)
	assert.Equal(t, 250, transaction.Quota)
}

func TestAsyncTaskPrechargeDoesNotBlockBudgetBeforeSettlement(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Task{}))

	key := EnterpriseKey{Name: "async-key", NewAPITokenID: 201, Status: StatusEnabled, SyncStatus: StatusSynced}
	require.NoError(t, db.Create(&key).Error)
	budget := BudgetAccount{ScopeType: "enterprise_key", ScopeID: key.ID, BudgetQuota: 200, Status: StatusEnabled}
	require.NoError(t, db.Create(&budget).Error)
	taskID := "task_pending_budget"
	require.NoError(t, db.Create(&model.Task{TaskID: taskID, Status: model.TaskStatusInProgress, UpdatedAt: time.Now().Unix()}).Error)
	require.NoError(t, db.Create(&model.Log{
		Id:        20,
		CreatedAt: time.Now().Unix(),
		Type:      model.LogTypeConsume,
		Quota:     700,
		TokenId:   key.NewAPITokenID,
		Other:     common.MapToJsonStr(map[string]interface{}{"is_task": true, "task_id": taskID}),
	}).Error)

	_, err := app.SyncUsage(100)
	require.NoError(t, err)
	var pending BudgetAccount
	require.NoError(t, db.First(&pending, budget.ID).Error)
	assert.Equal(t, 700, pending.UsedQuota)
	assert.Equal(t, 700, pending.PendingQuota)
	assert.False(t, budgetShouldBlock(pending, time.Now().Unix()))
	var blockCount int64
	require.NoError(t, db.Model(&BudgetKeyBlock{}).Where("status = ?", BudgetBlockActive).Count(&blockCount).Error)
	assert.Zero(t, blockCount)

	require.NoError(t, db.Create(&model.Log{
		Id:        21,
		CreatedAt: time.Now().Unix() + 1,
		Type:      model.LogTypeRefund,
		Quota:     558,
		TokenId:   key.NewAPITokenID,
		Other:     common.MapToJsonStr(map[string]interface{}{"task_id": taskID, "pre_consumed_quota": 700, "actual_quota": 142}),
	}).Error)
	_, err = app.SyncUsage(100)
	require.NoError(t, err)
	require.NoError(t, db.First(&pending, budget.ID).Error)
	assert.Equal(t, 142, pending.UsedQuota)
	assert.Zero(t, pending.PendingQuota)
	assert.False(t, budgetShouldBlock(pending, time.Now().Unix()))

	var transactions []BudgetTransaction
	require.NoError(t, db.Where("budget_account_id = ?", budget.ID).Order("new_api_log_id asc").Find(&transactions).Error)
	require.Len(t, transactions, 2)
	assert.False(t, transactions[0].Pending)
	assert.Equal(t, taskID, transactions[0].TaskID)
	assert.Equal(t, -558, transactions[1].Quota)
}

func TestAsyncTaskExactPrechargeSettlesAfterTerminalState(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Task{}))

	key := EnterpriseKey{Name: "exact-key", NewAPITokenID: 202, Status: StatusEnabled, SyncStatus: StatusSynced}
	require.NoError(t, db.Create(&key).Error)
	budget := BudgetAccount{ScopeType: "enterprise_key", ScopeID: key.ID, BudgetQuota: 1000, Status: StatusEnabled}
	require.NoError(t, db.Create(&budget).Error)
	task := model.Task{TaskID: "task_exact_budget", Status: model.TaskStatusInProgress, UpdatedAt: time.Now().Unix()}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&model.Log{
		Id:        30,
		CreatedAt: time.Now().Unix(),
		Type:      model.LogTypeConsume,
		Quota:     700,
		TokenId:   key.NewAPITokenID,
		Other:     common.MapToJsonStr(map[string]interface{}{"is_task": true, "task_id": task.TaskID}),
	}).Error)

	_, err := app.SyncUsage(100)
	require.NoError(t, err)
	var account BudgetAccount
	require.NoError(t, db.First(&account, budget.ID).Error)
	assert.Equal(t, 700, account.PendingQuota)

	require.NoError(t, db.Model(&model.Task{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":      model.TaskStatusSuccess,
		"finish_time": time.Now().Add(-time.Minute).Unix(),
		"updated_at":  time.Now().Add(-time.Minute).Unix(),
	}).Error)
	_, err = app.SyncUsage(100)
	require.NoError(t, err)
	require.NoError(t, db.First(&account, budget.ID).Error)
	assert.Equal(t, 700, account.UsedQuota)
	assert.Zero(t, account.PendingQuota)
}

func TestBudgetShouldBlockUsesConfirmedQuotaOnly(t *testing.T) {
	now := time.Now().Unix()
	account := BudgetAccount{Status: StatusEnabled, BudgetQuota: 200, UsedQuota: 700, PendingQuota: 700}
	assert.False(t, budgetShouldBlock(account, now))
	account.PendingQuota = 400
	assert.True(t, budgetShouldBlock(account, now))
}

func TestSyncEnterpriseKeyUpdatesExistingTokenUserID(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}))

	org := OrgUnit{
		Name:         "Sales",
		Type:         OrgTypeDepartment,
		Status:       StatusEnabled,
		DefaultGroup: "dept-sales",
		NewAPIUserID: 2,
	}
	require.NoError(t, db.Create(&org).Error)
	token := model.Token{
		UserId:         1,
		Name:           "old-name",
		Key:            "old-key",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
		Group:          "old-group",
	}
	require.NoError(t, db.Create(&token).Error)
	key := EnterpriseKey{
		Name:          "sales-prod",
		OrgUnitID:     org.ID,
		NewAPITokenID: token.Id,
		Status:        StatusEnabled,
	}
	require.NoError(t, db.Create(&key).Error)

	fullKey, err := app.syncEnterpriseKey(key.ID, false)
	require.NoError(t, err)
	assert.Empty(t, fullKey)

	var refreshed model.Token
	require.NoError(t, db.First(&refreshed, token.Id).Error)
	assert.Equal(t, 2, refreshed.UserId)
	assert.Equal(t, "sales-prod", refreshed.Name)
	assert.Equal(t, "dept-sales", refreshed.Group)
	assert.Equal(t, common.TokenStatusEnabled, refreshed.Status)
}

func TestInheritedServiceUserFollowsOrganizationChanges(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	org := OrgUnit{
		Name:         "Sales",
		Type:         OrgTypeDepartment,
		Status:       StatusEnabled,
		DefaultGroup: "default",
		NewAPIUserID: 2,
	}
	require.NoError(t, db.Create(&org).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: org.ID, DescendantID: org.ID, Depth: 0}).Error)
	key := EnterpriseKey{Name: "inherited", OrgUnitID: org.ID, Status: StatusEnabled, SyncStatus: StatusPending}
	require.NoError(t, db.Create(&key).Error)

	_, err := app.syncEnterpriseKey(key.ID, false)
	require.NoError(t, err)
	require.NoError(t, db.First(&key, key.ID).Error)
	assert.Zero(t, key.ConfiguredNewAPIUserID)
	assert.Equal(t, 2, key.NewAPIUserID)

	require.NoError(t, db.Model(&OrgUnit{}).Where("id = ?", org.ID).Update("new_api_user_id", 3).Error)
	require.NoError(t, app.markKeysPendingForOrg(org.ID))
	require.NoError(t, app.syncPendingEnterpriseKeys(100))
	require.NoError(t, db.First(&key, key.ID).Error)
	assert.Zero(t, key.ConfiguredNewAPIUserID)
	assert.Equal(t, 3, key.NewAPIUserID)
	var token model.Token
	require.NoError(t, db.First(&token, key.NewAPITokenID).Error)
	assert.Equal(t, 3, token.UserId)
}

func TestSyncEnterpriseKeyPreservesConsumedQuotaAcrossResync(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	policy := Policy{
		Name:            "bounded-key",
		DefaultGroup:    "default",
		KeyDefaultQuota: 1000,
		Status:          StatusEnabled,
	}
	require.NoError(t, db.Create(&policy).Error)
	key := EnterpriseKey{
		Name:                   "bounded",
		PolicyID:               policy.ID,
		ConfiguredNewAPIUserID: 1,
		Status:                 StatusEnabled,
		SyncStatus:             StatusPending,
	}
	require.NoError(t, db.Create(&key).Error)

	_, err := app.syncEnterpriseKey(key.ID, false)
	require.NoError(t, err)
	require.NoError(t, db.First(&key, key.ID).Error)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", key.NewAPITokenID).Updates(map[string]any{
		"remain_quota": 600,
		"used_quota":   400,
	}).Error)

	_, err = app.syncEnterpriseKey(key.ID, false)
	require.NoError(t, err)
	var token model.Token
	require.NoError(t, db.First(&token, key.NewAPITokenID).Error)
	assert.Equal(t, 600, token.RemainQuota)

	require.NoError(t, db.Model(&Policy{}).Where("id = ?", policy.ID).Update("key_default_quota", 1500).Error)
	_, err = app.syncEnterpriseKey(key.ID, false)
	require.NoError(t, err)
	require.NoError(t, db.First(&token, key.NewAPITokenID).Error)
	assert.Equal(t, 1100, token.RemainQuota)

	require.NoError(t, db.Model(&Policy{}).Where("id = ?", policy.ID).Update("key_default_quota", 500).Error)
	_, err = app.syncEnterpriseKey(key.ID, false)
	require.NoError(t, err)
	require.NoError(t, db.First(&token, key.NewAPITokenID).Error)
	assert.Equal(t, 100, token.RemainQuota)
	require.NoError(t, db.First(&key, key.ID).Error)
	assert.Equal(t, 500, key.AppliedKeyQuota)
}

func TestPolicyDailyBudgetBlocksAndReleasesAtNextPeriod(t *testing.T) {
	app, db := newTestApp(t)
	app.budgetLocation = time.UTC
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	policy := Policy{
		Name:             "daily-limit",
		DefaultGroup:     "default",
		DailyBudgetQuota: 100,
		Status:           StatusEnabled,
	}
	require.NoError(t, db.Create(&policy).Error)
	org := OrgUnit{
		Name:            "Sales",
		Type:            OrgTypeDepartment,
		Status:          StatusEnabled,
		DefaultPolicyID: policy.ID,
		NewAPIUserID:    1,
	}
	require.NoError(t, db.Create(&org).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: org.ID, DescendantID: org.ID, Depth: 0}).Error)
	key := EnterpriseKey{Name: "sales", OrgUnitID: org.ID, Status: StatusEnabled, SyncStatus: StatusPending}
	require.NoError(t, db.Create(&key).Error)
	_, err := app.syncEnterpriseKey(key.ID, false)
	require.NoError(t, err)
	require.NoError(t, db.First(&key, key.ID).Error)

	firstDay := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC).Unix()
	require.NoError(t, app.ensurePolicyBudgetsAt(firstDay, true))
	var firstBudget BudgetAccount
	require.NoError(t, db.Where("source_type = ? AND period_kind = ?", BudgetSourcePolicy, BudgetPeriodDaily).First(&firstBudget).Error)
	require.NoError(t, db.Model(&BudgetAccount{}).Where("id = ?", firstBudget.ID).Update("used_quota", 100).Error)
	require.NoError(t, app.reconcileBudgetEnforcement(firstDay))

	var block BudgetKeyBlock
	require.NoError(t, db.Where("enterprise_key_id = ? AND status = ?", key.ID, BudgetBlockActive).First(&block).Error)
	var token model.Token
	require.NoError(t, db.First(&token, key.NewAPITokenID).Error)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
	require.NoError(t, db.First(&key, key.ID).Error)
	assert.Equal(t, StatusEnabled, key.Status)

	secondDay := time.Date(2026, time.July, 11, 0, 0, 1, 0, time.UTC).Unix()
	require.NoError(t, app.ensurePolicyBudgetsAt(secondDay, true))
	require.NoError(t, db.First(&token, key.NewAPITokenID).Error)
	assert.Equal(t, common.TokenStatusEnabled, token.Status)
	require.NoError(t, db.First(&block, block.ID).Error)
	assert.Equal(t, BudgetBlockReleased, block.Status)
	var dailyBudgetCount int64
	require.NoError(t, db.Model(&BudgetAccount{}).Where("source_type = ? AND period_kind = ?", BudgetSourcePolicy, BudgetPeriodDaily).Count(&dailyBudgetCount).Error)
	assert.EqualValues(t, 2, dailyBudgetCount)
}

func TestManualKeyDisableSurvivesBudgetRelease(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	policy := Policy{Name: "manual-disable", DefaultGroup: "default", Status: StatusEnabled}
	require.NoError(t, db.Create(&policy).Error)
	key := EnterpriseKey{
		Name:                   "disabled",
		PolicyID:               policy.ID,
		ConfiguredNewAPIUserID: 1,
		Status:                 StatusDisabled,
		SyncStatus:             StatusPending,
	}
	require.NoError(t, db.Create(&key).Error)
	_, err := app.syncEnterpriseKey(key.ID, false)
	require.NoError(t, err)
	require.NoError(t, db.First(&key, key.ID).Error)
	budget := BudgetAccount{ScopeType: "enterprise_key", ScopeID: key.ID, BudgetQuota: 10, UsedQuota: 10, Status: StatusEnabled}
	require.NoError(t, db.Create(&budget).Error)
	_, err = app.ensureBudgetBlocks(budget, key)
	require.NoError(t, err)
	_, err = app.releaseBudgetBlocks(budget.ID)
	require.NoError(t, err)

	var token model.Token
	require.NoError(t, db.First(&token, key.NewAPITokenID).Error)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
}

func TestDeniedOnlyPolicyBuildsConcreteModelAllowlist(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Ability{}, &model.Channel{}))
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "allowed-model", ChannelId: 1, Enabled: true}).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "denied-model", ChannelId: 1, Enabled: true}).Error)
	policy := Policy{
		Name:         "deny-one",
		DefaultGroup: "default",
		DeniedModels: "denied-model",
		Status:       StatusEnabled,
	}
	require.NoError(t, db.Create(&policy).Error)
	key := EnterpriseKey{
		Name:                   "deny-one",
		PolicyID:               policy.ID,
		ConfiguredNewAPIUserID: 1,
		Status:                 StatusEnabled,
		SyncStatus:             StatusPending,
	}
	require.NoError(t, db.Create(&key).Error)

	_, err := app.syncEnterpriseKey(key.ID, false)
	require.NoError(t, err)
	require.NoError(t, db.First(&key, key.ID).Error)
	var token model.Token
	require.NoError(t, db.First(&token, key.NewAPITokenID).Error)
	assert.True(t, token.ModelLimitsEnabled)
	assert.Equal(t, "allowed-model", token.ModelLimits)
}

func TestDisjointInheritedModelAllowListsDisableToken(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	rootPolicy := Policy{Name: "root-models", DefaultGroup: "default", AllowedModels: "model-a", Status: StatusEnabled}
	childPolicy := Policy{Name: "child-models", AllowedModels: "model-b", Status: StatusEnabled}
	require.NoError(t, db.Create(&rootPolicy).Error)
	require.NoError(t, db.Create(&childPolicy).Error)
	root := OrgUnit{Name: "Company", Type: OrgTypeCompany, Status: StatusEnabled, DefaultPolicyID: rootPolicy.ID, NewAPIUserID: 1}
	require.NoError(t, db.Create(&root).Error)
	child := OrgUnit{Name: "Team", ParentID: &root.ID, Type: OrgTypeTeam, Status: StatusEnabled, DefaultPolicyID: childPolicy.ID, NewAPIUserID: 1}
	require.NoError(t, db.Create(&child).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: root.ID, DescendantID: root.ID, Depth: 0}).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: root.ID, DescendantID: child.ID, Depth: 1}).Error)
	require.NoError(t, db.Create(&OrgUnitClosure{AncestorID: child.ID, DescendantID: child.ID, Depth: 0}).Error)
	key := EnterpriseKey{Name: "no-models", OrgUnitID: child.ID, Status: StatusEnabled, SyncStatus: StatusPending}
	require.NoError(t, db.Create(&key).Error)

	_, err := app.syncEnterpriseKey(key.ID, false)
	require.NoError(t, err)
	require.NoError(t, db.First(&key, key.ID).Error)
	var token model.Token
	require.NoError(t, db.First(&token, key.NewAPITokenID).Error)
	assert.Equal(t, common.TokenStatusDisabled, token.Status)
}

func TestBuildTokenOperationObjectSyncRequestUsesNewAPIStableIDs(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}))

	require.NoError(t, db.Create(&model.User{Id: 42, Username: "dept-sales", Role: 1, Status: 1}).Error)
	org := OrgUnit{Name: "Sales", Code: "sales", Type: OrgTypeDepartment, Status: StatusEnabled, NewAPIUserID: 42}
	require.NoError(t, db.Create(&org).Error)
	key := EnterpriseKey{
		Name:           "sales-prod",
		OrgUnitID:      org.ID,
		NewAPIUserID:   42,
		NewAPITokenID:  7,
		KeyFingerprint: "fp_test",
		Status:         StatusEnabled,
		Environment:    "prod",
	}
	require.NoError(t, db.Create(&key).Error)
	require.NoError(t, db.Create(&model.Channel{
		Id:     3,
		Type:   constant.ChannelTypeDoubaoVideo,
		Name:   "hwdrama85oversea",
		Status: common.ChannelStatusEnabled,
		Models: "doubao-seedance-2-0-filter-off",
		Group:  "AlbertG",
		Key:    "masked",
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "AlbertG",
		Model:     "doubao-seedance-2-0-filter-off",
		ChannelId: 3,
		Enabled:   true,
	}).Error)

	payload, counts, err := app.buildTokenOperationObjectSyncRequest()
	require.NoError(t, err)
	assert.Equal(t, 1, counts["customers"])
	assert.Equal(t, 1, counts["users"])
	assert.Equal(t, 1, counts["api_keys"])
	assert.Equal(t, 1, counts["channels"])
	assert.Equal(t, 1, counts["models"])

	require.Len(t, payload.Objects.APIKeys, 1)
	assert.Equal(t, "7", payload.Objects.APIKeys[0]["token_id"])
	assert.Equal(t, "42", payload.Objects.APIKeys[0]["gateway_user_id"])
	assert.Equal(t, "42", payload.Objects.APIKeys[0]["gateway_customer_id"])
	assert.Equal(t, "org_unit_id="+strconv.Itoa(org.ID), payload.Objects.APIKeys[0]["org_unit_id"])
	assert.Equal(t, "active", payload.Objects.APIKeys[0]["object_status"])
	assert.NotContains(t, payload.Objects.APIKeys[0], "status")

	require.Len(t, payload.Objects.Channels, 1)
	assert.Equal(t, "3", payload.Objects.Channels[0]["channel_id"])
	assert.Equal(t, "active", payload.Objects.Channels[0]["object_status"])
	assert.NotContains(t, payload.Objects.Channels[0], "status")

	require.Len(t, payload.Objects.Models, 1)
	assert.Equal(t, "doubao-seedance-2-0-filter-off", payload.Objects.Models[0]["model_name"])
	assert.Equal(t, "3", payload.Objects.Models[0]["channel_id"])
	assert.Equal(t, "video_generation", payload.Objects.Models[0]["call_type"])
	assert.Equal(t, "active", payload.Objects.Models[0]["object_status"])

	require.Len(t, payload.Objects.Projects, 1)
	assert.Equal(t, "active", payload.Objects.Projects[0]["object_status"])
	assert.NotContains(t, payload.Objects.Projects[0], "status")
}

func TestSyncTokenOperationObjectsPostsGatewayContract(t *testing.T) {
	app, db := newTestApp(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}))
	require.NoError(t, db.Create(&model.User{Id: 42, Username: "dept-sales", Role: 1, Status: 1}).Error)
	require.NoError(t, db.Create(&EnterpriseKey{Name: "sales-prod", NewAPIUserID: 42, NewAPITokenID: 7, Status: StatusEnabled}).Error)

	seen := make(chan map[string]string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload tokenOperationObjectSyncRequest
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		require.Len(t, payload.Objects.APIKeys, 1)
		seen <- map[string]string{
			"path":            r.URL.Path,
			"gateway_key":     r.Header.Get("x-gateway-key"),
			"schema_version":  r.Header.Get("x-schema-version"),
			"idempotency_key": r.Header.Get("idempotency-key"),
			"token_id":        payload.Objects.APIKeys[0]["token_id"].(string),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true,"data":{"status":"accepted","syncBatchId":"test_batch"}}`))
	}))
	defer server.Close()

	app.config.TokenOperation = TokenOperationConfig{
		Enabled:           true,
		BaseURL:           server.URL,
		GatewayKey:        "gw_test",
		ObjectSyncEnabled: true,
		Timeout:           time.Second,
	}

	result, err := app.SyncTokenOperationObjects(context.Background())
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, result.StatusCode)
	assert.Equal(t, 1, result.ObjectCounts["api_keys"])

	headers := <-seen
	assert.Equal(t, tokenOperationObjectSyncPath, headers["path"])
	assert.Equal(t, "gw_test", headers["gateway_key"])
	assert.Equal(t, tokenOperationObjectSyncSchemaVersion, headers["schema_version"])
	assert.NotEmpty(t, headers["idempotency_key"])
	assert.Equal(t, "7", headers["token_id"])
}

func TestGetTokenOperationUsageDetailsUsesGatewayReadContract(t *testing.T) {
	app, _ := newTestApp(t)

	seen := make(chan map[string]string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- map[string]string{
			"path":           r.URL.Path,
			"gateway_key":    r.Header.Get("x-gateway-key"),
			"schema_version": r.Header.Get("x-schema-version"),
			"limit":          r.URL.Query().Get("limit"),
			"ignored":        r.URL.Query().Get("ignored"),
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{"contractVersion":"gateway-usage-details-v1","count":0,"items":[]}}`))
	}))
	defer server.Close()

	app.config.TokenOperation = TokenOperationConfig{
		Enabled:    true,
		BaseURL:    server.URL,
		GatewayKey: "gw_test",
		Timeout:    time.Second,
	}
	query := url.Values{"limit": []string{"25"}, "ignored": []string{"drop-me"}}
	result, err := app.GetTokenOperationUsageDetails(context.Background(), query)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, result.StatusCode)

	headers := <-seen
	assert.Equal(t, tokenOperationUsageDetailsPath, headers["path"])
	assert.Equal(t, "gw_test", headers["gateway_key"])
	assert.Equal(t, tokenOperationUsageDetailsVersion, headers["schema_version"])
	assert.Equal(t, "25", headers["limit"])
	assert.Empty(t, headers["ignored"])
}
