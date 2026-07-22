package resellerhub

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultDiscountRangeAllowsDiscountsAndMarkups(t *testing.T) {
	t.Setenv("RESELLER_HUB_MIN_DISCOUNT_BPS", "")
	t.Setenv("RESELLER_HUB_MAX_DISCOUNT_BPS", "")

	config := LoadConfig()

	assert.Equal(t, 100, config.MinDiscountBPS)
	assert.Equal(t, 50000, config.MaxDiscountBPS)
}

func TestValidateDiscountUsesConfiguredBoundaries(t *testing.T) {
	app := New(nil, nil, Config{MinDiscountBPS: 100, MaxDiscountBPS: 50000})

	require.NoError(t, app.validateDiscount(100))
	require.NoError(t, app.validateDiscount(50000))
	assert.Error(t, app.validateDiscount(99))
	assert.Error(t, app.validateDiscount(50001))
}
