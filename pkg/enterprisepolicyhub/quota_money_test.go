package enterprisepolicyhub

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveMonetaryQuotaConvertsSupportedCurrencies(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	originalUSDExchangeRate := operation_setting.USDExchangeRate
	originalCustomRate := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.USDExchangeRate = originalUSDExchangeRate
		operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate = originalCustomRate
	})
	common.QuotaPerUnit = 500000
	operation_setting.USDExchangeRate = 7.3
	operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate = 2

	tests := []struct {
		name     string
		amount   string
		currency string
		quota    int
		rate     float64
	}{
		{name: "USD", amount: "200", currency: "USD", quota: 100000000, rate: 1},
		{name: "CNY", amount: "730", currency: "CNY", quota: 50000000, rate: 7.3},
		{name: "custom", amount: "20", currency: "CUSTOM", quota: 5000000, rate: 2},
		{name: "raw quota", amount: "1234", currency: "QUOTA", quota: 1234, rate: 500000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			amount := decimal.RequireFromString(test.amount)
			result, err := resolveMonetaryQuota(monetaryQuotaInput{Amount: &amount, Currency: test.currency})
			require.NoError(t, err)
			assert.Equal(t, test.quota, result.Quota)
			assert.Equal(t, test.amount, result.Amount)
			assert.Equal(t, test.currency, result.Currency)
			assert.Equal(t, test.rate, result.ExchangeRate)
			assert.Equal(t, float64(500000), result.QuotaPerUnit)
		})
	}
}

func TestResolveMonetaryQuotaSupportsLegacyAndUnlimitedRequests(t *testing.T) {
	legacy, err := resolveMonetaryQuota(monetaryQuotaInput{LegacyQuota: 7654321})
	require.NoError(t, err)
	assert.Equal(t, 7654321, legacy.Quota)
	assert.Empty(t, legacy.Amount)

	unlimited, err := resolveMonetaryQuota(monetaryQuotaInput{Unlimited: true, Currency: "USD", LegacyQuota: 7654321})
	require.NoError(t, err)
	assert.Zero(t, unlimited.Quota)
	assert.Equal(t, "0", unlimited.Amount)
	assert.Equal(t, "USD", unlimited.Currency)

	_, err = resolveMonetaryQuota(monetaryQuotaInput{Currency: "EUR", Unlimited: true})
	assert.ErrorContains(t, err, "unsupported currency")
}

func TestPolicyFromRequestPersistsMonetarySnapshots(t *testing.T) {
	monthly := decimal.RequireFromString("200")
	daily := decimal.RequireFromString("10.5")
	keyDefault := decimal.RequireFromString("75")
	policy, err := policyFromRequest(policyRequest{
		Name:                "engineering",
		MonthlyBudgetAmount: &monthly, MonthlyBudgetCurrency: "USD",
		DailyBudgetAmount: &daily, DailyBudgetCurrency: "USD",
		KeyDefaultAmount: &keyDefault, KeyDefaultCurrency: "USD",
	})
	require.NoError(t, err)
	assert.Equal(t, 100000000, policy.MonthlyBudgetQuota)
	assert.Equal(t, "200", policy.MonthlyBudgetAmount)
	assert.Equal(t, 5250000, policy.DailyBudgetQuota)
	assert.Equal(t, "10.5", policy.DailyBudgetAmount)
	assert.Equal(t, 37500000, policy.KeyDefaultQuota)
	assert.Equal(t, "75", policy.KeyDefaultAmount)
	assert.Equal(t, float64(500000), policy.MonthlyBudgetQuotaPerUnit)
	assert.Equal(t, float64(1), policy.MonthlyBudgetExchangeRate)
}

func TestBudgetFromRequestSupportsMoneyAndLegacyQuota(t *testing.T) {
	amount := decimal.RequireFromString("40")
	monetary, err := budgetFromRequest(budgetRequest{
		ScopeType: "org_unit", ScopeID: 7, BudgetAmount: &amount, BudgetCurrency: "USD",
	})
	require.NoError(t, err)
	assert.Equal(t, 20000000, monetary.BudgetQuota)
	assert.Equal(t, "40", monetary.BudgetAmount)
	assert.Equal(t, "USD", monetary.BudgetCurrency)

	legacy, err := budgetFromRequest(budgetRequest{ScopeType: "org_unit", ScopeID: 7, BudgetQuota: 12345})
	require.NoError(t, err)
	assert.Equal(t, 12345, legacy.BudgetQuota)
	assert.Empty(t, legacy.BudgetAmount)
	assert.Equal(t, quotaCurrency, legacy.Currency)

	_, err = budgetFromRequest(budgetRequest{ScopeType: "org_unit", ScopeID: 7})
	assert.ErrorContains(t, err, "greater than zero")
}

func TestPolicyManagedBudgetCopiesMonetarySnapshot(t *testing.T) {
	app, db := newTestApp(t)
	policy := Policy{
		Name: "sales", Status: StatusEnabled,
		MonthlyBudgetQuota: 100000000, MonthlyBudgetAmount: "200", MonthlyBudgetCurrency: "USD",
		MonthlyBudgetQuotaPerUnit: 500000, MonthlyBudgetExchangeRate: 1,
	}
	require.NoError(t, db.Create(&policy).Error)
	org := OrgUnit{Name: "Sales", Code: "sales", Type: OrgTypeDepartment, Status: StatusEnabled, DefaultPolicyID: policy.ID}
	require.NoError(t, db.Create(&org).Error)

	require.NoError(t, app.ensurePolicyBudgetsAt(1721000000, false))
	var budget BudgetAccount
	require.NoError(t, db.Where("source_type = ? AND source_id = ?", BudgetSourcePolicy, policy.ID).First(&budget).Error)
	assert.Equal(t, policy.MonthlyBudgetQuota, budget.BudgetQuota)
	assert.Equal(t, policy.MonthlyBudgetAmount, budget.BudgetAmount)
	assert.Equal(t, policy.MonthlyBudgetCurrency, budget.BudgetCurrency)
	assert.Equal(t, policy.MonthlyBudgetQuotaPerUnit, budget.BudgetQuotaPerUnit)
	assert.Equal(t, policy.MonthlyBudgetExchangeRate, budget.BudgetExchangeRate)
}
