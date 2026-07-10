package enterprisepolicyhub

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfigBudgetTimezoneFollowsGatewayTimezone(t *testing.T) {
	t.Setenv("EPH_BUDGET_TIMEZONE", "")
	t.Setenv("TZ", "Asia/Singapore")
	assert.Equal(t, "Asia/Singapore", LoadConfig().BudgetTimezone)
}

func TestLoadConfigBudgetTimezoneOverride(t *testing.T) {
	t.Setenv("TZ", "Asia/Singapore")
	t.Setenv("EPH_BUDGET_TIMEZONE", "America/Los_Angeles")
	assert.Equal(t, "America/Los_Angeles", LoadConfig().BudgetTimezone)
}

func TestLoadConfigBudgetTimezoneDefaultsToUTC(t *testing.T) {
	t.Setenv("EPH_BUDGET_TIMEZONE", "")
	t.Setenv("TZ", "")
	assert.Equal(t, "UTC", LoadConfig().BudgetTimezone)
}
