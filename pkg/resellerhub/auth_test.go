package resellerhub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticateUsesGatewayIdentityAndSidecarMembership(t *testing.T) {
	db := openServiceTestDB(t)
	reseller := Reseller{Code: "auth-r", Name: "Auth reseller", Status: ResellerStatusActive, DefaultDiscountBPS: 9000, QuotaCarrierUserID: 999, CreatedByUserID: 100}
	require.NoError(t, db.Create(&reseller).Error)
	require.NoError(t, db.Create(&Membership{ResellerID: reseller.ID, NewAPIUserID: 7, Role: HubRoleResellerAdmin, Status: MembershipStatusActive}).Error)

	currentUserID := 7
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]any{
			"success": true,
			"data": map[string]any{
				"id": currentUserID, "username": "operator", "role": common.RoleCommonUser,
				"status": common.UserStatusEnabled, "group": "default",
			},
		}
		if currentUserID == 100 {
			payload["data"].(map[string]any)["role"] = common.RoleRootUser
		}
		encoded, err := common.Marshal(payload)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encoded)
	}))
	defer server.Close()

	app := New(db, db, Config{GatewayBaseURL: server.URL})
	req := httptest.NewRequest(http.MethodGet, "/reseller/api/me", nil)
	identity, err := app.authenticate(req)
	require.NoError(t, err)
	assert.Equal(t, HubRoleResellerAdmin, identity.HubRole)
	assert.Equal(t, reseller.ID, identity.ResellerID)

	currentUserID = 8
	_, err = app.authenticate(req)
	require.Error(t, err)
	assert.ErrorIs(t, err, errMembershipRequired)

	router := http.NewServeMux()
	ginRouter := gin.New()
	ginRouter.GET("/reseller/api/me", app.authMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.Handle("/", ginRouter)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/reseller/api/me", nil))
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), errMembershipRequired.Error())

	currentUserID = 100
	identity, err = app.authenticate(req)
	require.NoError(t, err)
	assert.Equal(t, HubRoleSuperAdmin, identity.HubRole)
	assert.Zero(t, identity.ResellerID)
}
