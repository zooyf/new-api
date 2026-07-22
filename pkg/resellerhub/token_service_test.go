package resellerhub

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTokenTestContext(t *testing.T, customerID int, name string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	body, err := common.Marshal(gin.H{"name": name, "group": "default", "models": []string{"model-a"}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/reseller/api/customers/1/tokens", bytes.NewReader(body))
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(customerID)}}
	c.Set("reseller_hub_identity", &Identity{NewAPIUserID: 9, HubRole: HubRoleResellerAdmin, ResellerID: 1})
	return c, recorder
}

func TestCustomerTokenIsFiniteAndLimitedToOneActiveMapping(t *testing.T) {
	db := openServiceTestDB(t)
	user := model.User{Username: "carrier", Password: "not-used-password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 2000000, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	reseller := Reseller{Code: "token-r", Name: "Token reseller", Status: ResellerStatusActive, DefaultDiscountBPS: 9000, QuotaCarrierUserID: user.Id, CreatedByUserID: 100}
	require.NoError(t, db.Create(&reseller).Error)
	customer := Customer{ResellerID: reseller.ID, DisplayName: "Customer", Status: CustomerStatusActive, CreatedByUserID: 9}
	require.NoError(t, db.Create(&customer).Error)
	app := New(db, db, Config{})

	firstContext, firstRecorder := createTokenTestContext(t, customer.ID, "first-key")
	firstContext.Set("reseller_hub_identity", &Identity{NewAPIUserID: 9, HubRole: HubRoleResellerAdmin, ResellerID: reseller.ID})
	app.createCustomerToken(firstContext)
	require.Equal(t, http.StatusCreated, firstRecorder.Code)

	var tokens []model.Token
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&tokens).Error)
	require.Len(t, tokens, 1)
	assert.False(t, tokens[0].UnlimitedQuota)
	assert.Zero(t, tokens[0].RemainQuota)
	var storedCustomer Customer
	require.NoError(t, db.First(&storedCustomer, customer.ID).Error)
	require.NotNil(t, storedCustomer.ActiveTokenMappingID)

	secondContext, secondRecorder := createTokenTestContext(t, customer.ID, "second-key")
	secondContext.Set("reseller_hub_identity", &Identity{NewAPIUserID: 9, HubRole: HubRoleResellerAdmin, ResellerID: reseller.ID})
	app.createCustomerToken(secondContext)
	assert.Equal(t, http.StatusConflict, secondRecorder.Code)
	tokens = nil
	require.NoError(t, db.Where("user_id = ?", user.Id).Find(&tokens).Error)
	assert.Len(t, tokens, 1)
}
