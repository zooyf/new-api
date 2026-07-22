package resellerhub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerUsageSummaryIncludesAllFilteredPages(t *testing.T) {
	db := openServiceTestDB(t)
	_, customer, mapping, _ := seedQuotaFixture(t, db, 1000, 0, common.TokenStatusEnabled)
	logs := []model.Log{
		{CreatedAt: 100, Type: model.LogTypeConsume, TokenId: mapping.NewAPITokenID, TokenName: "customer-key", ModelName: "model-a", Quota: 100},
		{CreatedAt: 200, Type: model.LogTypeConsume, TokenId: mapping.NewAPITokenID, TokenName: "customer-key", ModelName: "model-a", Quota: 200},
		{CreatedAt: 300, Type: model.LogTypeRefund, TokenId: mapping.NewAPITokenID, TokenName: "customer-key", ModelName: "model-a", Quota: 50},
	}
	require.NoError(t, db.Create(&logs).Error)

	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"quota_display_type":"USD","quota_per_unit":500000}}`))
	}))
	defer statusServer.Close()

	app := New(db, db, Config{GatewayBaseURL: statusServer.URL})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/reseller/api/customers/1/usage?page=1&page_size=1", nil)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Set("reseller_hub_identity", &Identity{NewAPIUserID: 9, HubRole: HubRoleResellerAdmin, ResellerID: customer.ResellerID})

	app.customerUsage(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Total   int64       `json:"total"`
			Items   []usageItem `json:"items"`
			Summary struct {
				StandardQuota  int    `json:"standard_quota"`
				StandardAmount string `json:"standard_amount"`
			} `json:"summary"`
		} `json:"data"`
	}
	require.NoError(t, common.DecodeJson(recorder.Body, &payload))
	assert.True(t, payload.Success)
	assert.EqualValues(t, 3, payload.Data.Total)
	assert.Len(t, payload.Data.Items, 1)
	assert.Equal(t, 250, payload.Data.Summary.StandardQuota)
	assert.Equal(t, "0.000500", payload.Data.Summary.StandardAmount)
}
