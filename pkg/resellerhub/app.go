package resellerhub

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type App struct {
	db         *gorm.DB
	logDB      *gorm.DB
	config     Config
	httpClient *http.Client
	isLeader   atomic.Bool
}

type apiResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func New(db *gorm.DB, logDB *gorm.DB, config Config) *App {
	if logDB == nil {
		logDB = db
	}
	if config.AuthTimeout <= 0 {
		config.AuthTimeout = 10 * time.Second
	}
	if config.MinDiscountBPS <= 0 {
		config.MinDiscountBPS = 5000
	}
	if config.MaxDiscountBPS <= 0 {
		config.MaxDiscountBPS = 10000
	}
	if config.LeaderLeaseDuration <= 0 {
		config.LeaderLeaseDuration = 30 * time.Second
	}
	if config.RedisEventMarkerTTL <= 0 {
		config.RedisEventMarkerTTL = 24 * time.Hour
	}
	return &App{
		db:     db,
		logDB:  logDB,
		config: config,
		httpClient: &http.Client{
			Timeout: config.AuthTimeout,
		},
	}
}

func (a *App) Router() *gin.Engine {
	router := gin.New()
	router.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("internal error: %v", recovered))
	}))

	register := func(group *gin.RouterGroup, trailingSlash bool) {
		group.GET("", a.index)
		if trailingSlash {
			group.GET("/", a.index)
		}
		group.GET("/healthz", a.healthz)

		api := group.Group("/api")
		api.Use(a.csrfMiddleware(), a.authMiddleware())
		api.GET("/me", a.me)
		api.GET("/auth/me", a.me)
		api.GET("/reference", a.referenceData)
		api.GET("/quota-conversion-config", a.quotaConversionConfig)
		api.GET("/dashboard-summary", a.dashboardSummary)

		api.GET("/resellers", requireRoot(a.listResellers))
		api.POST("/resellers", requireRoot(a.requireWriteLeader(a.requireIdempotency(a.createReseller))))
		api.GET("/resellers/:id", requireRoot(a.getReseller))
		api.PATCH("/resellers/:id", requireRoot(a.requireWriteLeader(a.requireIdempotency(a.updateReseller))))
		api.POST("/resellers/:id/members", requireRoot(a.requireWriteLeader(a.requireIdempotency(a.createMembership))))
		api.DELETE("/resellers/:id/members/:member_id", requireRoot(a.requireWriteLeader(a.requireIdempotency(a.deleteMembership))))
		api.POST("/resellers/:id/funding-adjustments", requireRoot(a.requireWriteLeader(a.adjustFunding)))

		api.GET("/customers", a.listCustomers)
		api.POST("/customers", requireResellerWriter(a.requireWriteLeader(a.requireIdempotency(a.createCustomer))))
		api.GET("/customers/:id", a.getCustomer)
		api.PATCH("/customers/:id", requireResellerWriter(a.requireWriteLeader(a.requireIdempotency(a.updateCustomer))))
		api.POST("/customers/:id/discounts", requireResellerWriter(a.requireWriteLeader(a.requireIdempotency(a.createDiscount))))
		api.POST("/customers/:id/quota-adjustments", requireResellerWriter(a.requireWriteLeader(a.adjustCustomerQuota)))
		api.GET("/customers/:id/quota-ledger", a.customerQuotaLedger)
		api.GET("/customers/:id/usage", a.customerUsage)
		api.GET("/customers/:id/tokens", a.listCustomerTokens)
		api.POST("/customers/:id/tokens", requireResellerWriter(a.requireWriteLeader(a.requireIdempotency(a.createCustomerToken))))
		api.PATCH("/customers/:id/tokens/:token_id", requireResellerWriter(a.requireWriteLeader(a.requireIdempotency(a.updateCustomerToken))))
		api.POST("/customers/:id/tokens/:token_id/retire", requireResellerWriter(a.requireWriteLeader(a.requireIdempotency(a.retireCustomerToken))))
		api.GET("/quota-adjustments/:event_id", a.getQuotaAdjustment)
		api.GET("/audit-logs", a.listAuditLogs)
	}

	register(router.Group(""), false)
	if a.config.BasePath != "" && a.config.BasePath != "/" {
		register(router.Group(a.config.BasePath), true)
	}
	return router
}

func (a *App) index(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.String(http.StatusOK, EmbeddedIndexHTML())
}

func (a *App) healthz(c *gin.Context) {
	sqlDB, err := a.db.DB()
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err = sqlDB.PingContext(ctx); err != nil {
		respondError(c, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (a *App) me(c *gin.Context) {
	identity := currentIdentity(c)
	data := gin.H{
		"new_api_user_id": identity.NewAPIUserID, "username": identity.Username, "role": identity.Role,
		"status": identity.Status, "group": identity.Group, "hub_role": identity.HubRole,
		"reseller_id": identity.ResellerID, "write_leader": a.isLeader.Load(),
	}
	if identity.ResellerID > 0 {
		var reseller Reseller
		if err := a.db.First(&reseller, identity.ResellerID).Error; err == nil {
			var carrier model.User
			if err = a.db.Select("id", "username", "quota", "status").First(&carrier, reseller.QuotaCarrierUserID).Error; err == nil {
				data["reseller_name"] = reseller.Name
				data["quota_carrier_user_id"] = carrier.Id
				data["quota_carrier_username"] = carrier.Username
				data["carrier_quota"] = carrier.Quota
			}
		}
	}
	respondOK(c, data)
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, apiResponse{Success: true, Data: data})
}

func respondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, apiResponse{Success: true, Data: data})
}

func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, apiResponse{Success: false, Message: message})
	c.Abort()
}

func parsePositiveID(c *gin.Context, name string) (int, bool) {
	id, err := strconv.Atoi(c.Param(name))
	if err != nil || id <= 0 {
		respondError(c, http.StatusBadRequest, "invalid "+name)
		return 0, false
	}
	return id, true
}

func queryPositiveInt(c *gin.Context, name string, fallback, maximum int) int {
	value, err := strconv.Atoi(strings.TrimSpace(c.Query(name)))
	if err != nil || value <= 0 {
		return fallback
	}
	if maximum > 0 && value > maximum {
		return maximum
	}
	return value
}

func (a *App) requireIdempotency(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		if key == "" {
			respondError(c, http.StatusBadRequest, "Idempotency-Key header is required")
			return
		}
		if len(key) > 128 {
			respondError(c, http.StatusBadRequest, "Idempotency-Key is too long")
			return
		}
		identity := currentIdentity(c)
		if identity == nil {
			respondError(c, http.StatusUnauthorized, "identity missing")
			return
		}
		var count int64
		if err := a.db.Model(&AuditLog{}).Where("actor_user_id = ? AND request_id = ?", identity.NewAPIUserID, key).Count(&count).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "failed to verify idempotency key")
			return
		}
		if count > 0 {
			respondError(c, http.StatusConflict, "Idempotency-Key was already applied")
			return
		}
		c.Set("reseller_hub_idempotency_key", key)
		next(c)
	}
}

func (a *App) requireWriteLeader(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.isLeader.Load() {
			respondError(c, http.StatusServiceUnavailable, "this Reseller Hub instance is read-only; write leader unavailable")
			return
		}
		if !a.config.DisableBackgroundWorkers {
			var count int64
			now := time.Now().Unix()
			err := a.db.Model(&Lease{}).
				Where("name = ? AND holder_id = ? AND expires_at >= ?", writerLeaseName, a.config.InstanceID, now).
				Count(&count).Error
			if err != nil || count != 1 {
				a.isLeader.Store(false)
				respondError(c, http.StatusServiceUnavailable, "this Reseller Hub instance no longer owns the write lease")
				return
			}
		}
		next(c)
	}
}

func idempotencyKey(c *gin.Context) string {
	if value, ok := c.Get("reseller_hub_idempotency_key"); ok {
		key, _ := value.(string)
		return key
	}
	return strings.TrimSpace(c.GetHeader("Idempotency-Key"))
}

func (a *App) scopedResellerID(c *gin.Context) (int, bool) {
	identity := currentIdentity(c)
	if identity == nil {
		respondError(c, http.StatusUnauthorized, "identity missing")
		return 0, false
	}
	if identity.HubRole != HubRoleSuperAdmin {
		return identity.ResellerID, true
	}
	value := strings.TrimSpace(c.Query("reseller_id"))
	if value == "" {
		return 0, true
	}
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		respondError(c, http.StatusBadRequest, "invalid reseller_id")
		return 0, false
	}
	return id, true
}

func (a *App) customerForRequest(c *gin.Context, customerID int) (*Customer, bool) {
	identity := currentIdentity(c)
	query := a.db.Where("id = ?", customerID)
	if identity == nil {
		respondError(c, http.StatusUnauthorized, "identity missing")
		return nil, false
	}
	if identity.HubRole != HubRoleSuperAdmin {
		query = query.Where("reseller_id = ?", identity.ResellerID)
	}
	var customer Customer
	if err := query.First(&customer).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "customer not found")
		} else {
			respondError(c, http.StatusInternalServerError, err.Error())
		}
		return nil, false
	}
	return &customer, true
}

func (a *App) tokenMappingForRequest(c *gin.Context, customer *Customer, tokenID int) (*CustomerToken, bool) {
	var mapping CustomerToken
	err := a.db.Where("reseller_id = ? AND customer_id = ? AND new_api_token_id = ?", customer.ResellerID, customer.ID, tokenID).First(&mapping).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			respondError(c, http.StatusNotFound, "token not found")
		} else {
			respondError(c, http.StatusInternalServerError, err.Error())
		}
		return nil, false
	}
	return &mapping, true
}

func (a *App) managedToken(mapping *CustomerToken) (*model.Token, error) {
	var token model.Token
	err := a.db.Unscoped().Where("id = ? AND user_id = ?", mapping.NewAPITokenID, mapping.QuotaCarrierUserID).First(&token).Error
	return &token, err
}
