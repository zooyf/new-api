package resellerhub

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResellerDashboardSummaryUsesAllManagedCustomers(t *testing.T) {
	db := openServiceTestDB(t)
	reseller, _, _, _ := seedQuotaFixture(t, db, -10, 25, common.TokenStatusExhausted)
	second := Customer{ResellerID: reseller.ID, DisplayName: "No token customer", Status: CustomerStatusActive, CreatedByUserID: 1}
	require.NoError(t, db.Create(&second).Error)
	app := New(db, db, Config{})

	summary, alerts, err := app.summarizeReseller(reseller)
	require.NoError(t, err)
	assert.EqualValues(t, 2, summary.CustomerCount)
	assert.EqualValues(t, 1, summary.ActiveKeyCount)
	assert.EqualValues(t, 1, summary.NegativeKeyCount)
	assert.EqualValues(t, -10, summary.ManagedBalance)
	assert.EqualValues(t, 2000000, summary.CarrierBalance)
	require.NotEmpty(t, alerts)
	assert.Equal(t, "negative_balance", alerts[0].Code)
}
