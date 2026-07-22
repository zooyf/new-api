package resellerhub

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaFromDisplayCurrencyAppliesDiscountWithoutFloatDrift(t *testing.T) {
	config := CurrencyConfig{
		DisplayType:      "USD",
		Symbol:           "$",
		QuotaPerUnit:     decimal.NewFromInt(500000),
		USDToDisplayRate: decimal.NewFromInt(1),
	}

	quota, customerAmount, err := quotaFromInput("display_currency", "0.85", config, 8500)
	require.NoError(t, err)
	assert.Equal(t, 500000, quota)
	assert.True(t, customerAmount.Equal(decimal.RequireFromString("0.85")))

	standard, discounted := quotaAmounts(quota, config, 8500)
	assert.True(t, standard.Equal(decimal.NewFromInt(1)))
	assert.True(t, discounted.Equal(decimal.RequireFromString("0.85")))
}

func TestQuotaFromCNYAndRawQuota(t *testing.T) {
	config := CurrencyConfig{
		DisplayType:      "CNY",
		Symbol:           "¥",
		QuotaPerUnit:     decimal.NewFromInt(500000),
		USDToDisplayRate: decimal.RequireFromString("7.3"),
	}

	quota, amount, err := quotaFromInput("display_currency", "6.205", config, 8500)
	require.NoError(t, err)
	assert.Equal(t, 500000, quota)
	assert.True(t, amount.Equal(decimal.RequireFromString("6.205")))

	quota, amount, err = quotaFromInput("quota", "500000", config, 8500)
	require.NoError(t, err)
	assert.Equal(t, 500000, quota)
	assert.True(t, amount.Equal(decimal.RequireFromString("6.205")))
}

func TestQuotaInputRejectsUnsupportedOrUnsafeValues(t *testing.T) {
	tokens := CurrencyConfig{
		DisplayType:      "TOKENS",
		QuotaPerUnit:     decimal.NewFromInt(500000),
		USDToDisplayRate: decimal.NewFromInt(1),
	}
	_, _, err := quotaFromInput("display_currency", "1", tokens, 10000)
	require.Error(t, err)

	_, _, err = quotaFromInput("quota", "1.5", tokens, 10000)
	require.Error(t, err)

	_, _, err = quotaFromInput("quota", decimal.NewFromInt(common.MaxQuota).Add(decimal.NewFromInt(1)).String(), tokens, 10000)
	require.Error(t, err)
}

func TestDiscountAtPrefersCustomerVersionAndHonorsTimeWindow(t *testing.T) {
	ended := int64(200)
	versions := []DiscountVersion{
		{CustomerID: 0, DiscountBPS: 9000, EffectiveAt: 10},
		{CustomerID: 7, DiscountBPS: 8000, EffectiveAt: 100, EndedAt: &ended},
		{CustomerID: 7, DiscountBPS: 7500, EffectiveAt: 200},
	}

	assert.Equal(t, 9000, discountAt(versions, 7, 50, 10000))
	assert.Equal(t, 8000, discountAt(versions, 7, 150, 10000))
	assert.Equal(t, 7500, discountAt(versions, 7, 250, 10000))
}
