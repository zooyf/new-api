package resellerhub

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerScopeReturnsNotFoundAcrossResellers(t *testing.T) {
	db := openServiceTestDB(t)
	first := Customer{ResellerID: 1, DisplayName: "A", Status: CustomerStatusActive, CreatedByUserID: 1}
	second := Customer{ResellerID: 2, DisplayName: "B", Status: CustomerStatusActive, CreatedByUserID: 2}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&second).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/reseller/api/customers/"+strconv.Itoa(second.ID), nil)
	c.Set("reseller_hub_identity", &Identity{NewAPIUserID: 1, HubRole: HubRoleResellerAdmin, ResellerID: 1})
	app := New(db, db, Config{})

	_, ok := app.customerForRequest(c, second.ID)
	assert.False(t, ok)
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestStaticInterfaceDoesNotLoadExternalAssets(t *testing.T) {
	html := EmbeddedIndexHTML()
	assert.NotContains(t, html, "https://")
	assert.NotContains(t, html, "http://")
	assert.Contains(t, html, "var API = '/reseller/api'")
	assert.Contains(t, html, "API+'/customers'")
	assert.Contains(t, html, "finite_key_explanation")
	assert.Contains(t, html, "unlimited_quota=false")
	assert.Contains(t, html, "MaxUserTokens")
	assert.Contains(t, html, "window.localStorage.getItem('uid')")
	assert.Contains(t, html, "headers['New-Api-User']=uid")
	assert.Contains(t, html, "membership_required")
	assert.Contains(t, html, "每个客户最多 1 个 Active 或 Retiring Key")
	assert.Contains(t, html, "系统创建时自动生成")
	assert.Contains(t, html, "外部客户编号（选填）")
	assert.Contains(t, html, "首次配置流程")
	assert.Contains(t, html, "计费比例（%）")
	assert.Contains(t, html, "minDiscountPercent()")
	assert.Contains(t, html, "maxDiscountPercent()")
	assert.NotContains(t, html, "折扣基点")
	assert.NotContains(t, html, "10000 = 原价")
	assert.NotContains(t, html, "虚拟客户")
	assert.NotContains(t, html, "Virtual customer")
}

func TestWriteLeaderMiddlewareVerifiesDatabaseLease(t *testing.T) {
	db := openServiceTestDB(t)
	app := New(db, db, Config{InstanceID: "instance-a"})
	app.isLeader.Store(true)
	require.NoError(t, db.Create(&Lease{
		Name: writerLeaseName, HolderID: "instance-b", ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}).Error)

	router := gin.New()
	router.POST("/write", app.requireWriteLeader(func(c *gin.Context) { c.Status(http.StatusNoContent) }))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/write", nil))

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.False(t, app.isLeader.Load())
}
