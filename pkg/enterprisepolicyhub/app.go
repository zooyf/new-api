package enterprisepolicyhub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type App struct {
	db             *gorm.DB
	newAPIDB       *gorm.DB
	newAPILog      *gorm.DB
	config         Config
	budgetLocation *time.Location
	httpClient     *http.Client
	usageSyncMu    sync.Mutex
}

type AdminIdentity struct {
	NewAPIUserID int    `json:"newapi_user_id"`
	Username     string `json:"username"`
	Role         int    `json:"role"`
	Status       int    `json:"status"`
	Group        string `json:"group"`
	HubRole      string `json:"hub_role"`
	ScopeOrgID   int    `json:"scope_org_unit_id"`
}

type apiResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type newAPIUserSelfResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Role     int    `json:"role"`
		Status   int    `json:"status"`
		Group    string `json:"group"`
	} `json:"data"`
}

func New(db *gorm.DB, newAPIDB *gorm.DB, newAPILog *gorm.DB, config Config) *App {
	if newAPIDB == nil {
		newAPIDB = db
	}
	if newAPILog == nil {
		newAPILog = newAPIDB
	}
	if config.AuthTimeout <= 0 {
		config.AuthTimeout = 10 * time.Second
	}
	budgetLocation, err := time.LoadLocation(config.BudgetTimezone)
	if err != nil {
		budgetLocation = time.UTC
	}
	return &App{
		db:             db,
		newAPIDB:       newAPIDB,
		newAPILog:      newAPILog,
		config:         config,
		budgetLocation: budgetLocation,
		httpClient: &http.Client{
			Timeout: config.AuthTimeout,
		},
	}
}

func (a *App) Router() *gin.Engine {
	router := gin.New()
	router.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		respondError(c, http.StatusInternalServerError, fmt.Sprintf("panic: %v", err))
	}))

	register := func(group *gin.RouterGroup, includeTrailingSlash bool) {
		group.GET("", a.index)
		if includeTrailingSlash {
			group.GET("/", a.index)
		}
		group.GET("/healthz", func(c *gin.Context) {
			c.String(http.StatusOK, "ok\n")
		})

		api := group.Group("/api")
		api.Use(a.csrfMiddleware(), a.authMiddleware())
		{
			api.GET("/auth/me", a.authMe)
			api.POST("/auth/logout", a.logout)
			api.GET("/reference", a.referenceData)
			api.GET("/org-units", a.listOrgUnits)
			api.POST("/org-units", a.requireOrgWriter(a.createOrgUnit))
			api.PUT("/org-units/:id", a.requireOrgWriter(a.updateOrgUnit))
			api.DELETE("/org-units/:id", a.requireOrgWriter(a.deleteOrgUnit))

			api.GET("/policies", a.listPolicies)
			api.POST("/policies", a.requirePolicyWriter(a.createPolicy))
			api.GET("/policies/:id", a.getPolicy)
			api.PUT("/policies/:id", a.requirePolicyWriter(a.updatePolicy))
			api.DELETE("/policies/:id", a.requirePolicyWriter(a.deletePolicy))
			api.POST("/policies/:id/preview-effective", a.previewEffectivePolicy)

			api.GET("/keys", a.listEnterpriseKeys)
			api.POST("/keys", a.requireKeyWriter(a.createEnterpriseKey))
			api.GET("/keys/:id", a.getEnterpriseKey)
			api.PUT("/keys/:id", a.requireKeyWriter(a.updateEnterpriseKey))
			api.DELETE("/keys/:id", a.requireKeyWriter(a.deleteEnterpriseKey))
			api.POST("/keys/:id/disable", a.requireKeyWriter(a.disableEnterpriseKey))
			api.POST("/keys/:id/enable", a.requireKeyWriter(a.enableEnterpriseKey))
			api.POST("/keys/:id/sync", a.requireKeyWriter(a.syncEnterpriseKeyHandler))
			api.POST("/keys/:id/rotate", a.requireKeyWriter(a.rotateEnterpriseKey))

			api.GET("/admin-bindings", a.listAdminBindings)
			api.POST("/admin-bindings", a.requireSuperAdmin(a.createAdminBinding))
			api.PUT("/admin-bindings/:id", a.requireSuperAdmin(a.updateAdminBinding))
			api.DELETE("/admin-bindings/:id", a.requireSuperAdmin(a.deleteAdminBinding))

			api.GET("/budgets", a.listBudgets)
			api.POST("/budgets", a.requireBudgetWriter(a.createBudget))
			api.PUT("/budgets/:id", a.requireBudgetWriter(a.updateBudget))
			api.DELETE("/budgets/:id", a.requireBudgetWriter(a.deleteBudget))
			api.POST("/budgets/:id/reset", a.requireBudgetWriter(a.resetBudget))

			api.POST("/usage/sync", a.requireBudgetWriter(a.syncUsageHandler))
			api.GET("/usage/summary", a.usageSummary)
			api.GET("/usage/details", a.usageDetails)
			api.GET("/audit-logs", a.auditLogs)
			api.GET("/sync-jobs", a.syncJobs)
			api.GET("/token-operation/status", a.tokenOperationStatus)
			api.POST("/token-operation/sync-objects", a.requireSuperAdmin(a.syncTokenOperationObjectsHandler))
			api.GET("/token-operation/usage-details", a.requireFinanceReader(a.tokenOperationUsageDetailsHandler))
		}
	}

	register(router.Group(""), false)
	if a.config.BasePath != "" && a.config.BasePath != "/" {
		register(router.Group(a.config.BasePath), true)
	}
	return router
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, apiResponse{Success: true, Message: "", Data: data})
}

func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, apiResponse{Success: false, Message: message})
	c.Abort()
}

func parseID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		respondError(c, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func currentAdmin(c *gin.Context) *AdminIdentity {
	admin, _ := c.Get("hub_admin")
	if admin == nil {
		return nil
	}
	identity, _ := admin.(*AdminIdentity)
	return identity
}

func (a *App) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		admin, err := a.authenticate(c.Request)
		if err != nil {
			respondError(c, http.StatusUnauthorized, err.Error())
			return
		}
		c.Set("hub_admin", admin)
		c.Next()
	}
}

func (a *App) csrfMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		if !sameOriginRequest(c.Request) {
			respondError(c, http.StatusForbidden, "same-origin request required")
			return
		}
		c.Next()
	}
}

func sameOriginRequest(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		origin = strings.TrimSpace(r.Header.Get("Referer"))
	}
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, r.Host)
}

func (a *App) authenticate(r *http.Request) (*AdminIdentity, error) {
	admin, err := a.authenticateWithNewAPI(r)
	if err != nil {
		return nil, err
	}
	if admin.Role >= common.RoleRootUser || a.config.BootstrapAdminIDs[admin.NewAPIUserID] || a.config.AllowAnyNewAPIAdmin {
		admin.HubRole = HubRoleSuperAdmin
		return admin, nil
	}
	var binding HubAdminBinding
	err = a.db.Where("new_api_user_id = ? AND status = ?", admin.NewAPIUserID, StatusEnabled).First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("new-api admin is not authorized in Enterprise Policy Hub")
		}
		return nil, err
	}
	admin.HubRole = binding.HubRole
	admin.ScopeOrgID = binding.ScopeOrgUnitID
	return admin, nil
}

func (a *App) authenticateWithNewAPI(r *http.Request) (*AdminIdentity, error) {
	if a.config.NewAPIBaseURL != "" {
		ctx, cancel := context.WithTimeout(r.Context(), a.config.AuthTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.config.NewAPIBaseURL+"/api/user/self", nil)
		if err != nil {
			return nil, err
		}
		for _, name := range []string{"Cookie", "Authorization", "New-Api-User"} {
			if value := r.Header.Get(name); value != "" {
				req.Header.Set(name, value)
			}
		}
		resp, err := a.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("new-api auth check failed: %w", err)
		}
		defer resp.Body.Close()
		var payload newAPIUserSelfResponse
		if err := common.DecodeJson(resp.Body, &payload); err != nil {
			return nil, fmt.Errorf("decode new-api auth response: %w", err)
		}
		if !payload.Success {
			if payload.Message == "" {
				payload.Message = "new-api auth check failed"
			}
			return nil, errors.New(payload.Message)
		}
		if payload.Data.Role < common.RoleAdminUser {
			return nil, errors.New("new-api admin role required")
		}
		if payload.Data.Status == common.UserStatusDisabled {
			return nil, errors.New("new-api user disabled")
		}
		return &AdminIdentity{
			NewAPIUserID: payload.Data.ID,
			Username:     payload.Data.Username,
			Role:         payload.Data.Role,
			Status:       payload.Data.Status,
			Group:        payload.Data.Group,
		}, nil
	}

	token := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	if token == "" {
		return nil, errors.New("Authorization required")
	}
	user, err := model.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Role < common.RoleAdminUser {
		return nil, errors.New("new-api admin access token required")
	}
	return &AdminIdentity{
		NewAPIUserID: user.Id,
		Username:     user.Username,
		Role:         user.Role,
		Status:       user.Status,
		Group:        user.Group,
	}, nil
}

func (a *App) requireOrgWriter(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin := currentAdmin(c)
		if admin == nil || !canWriteOrg(admin.HubRole) {
			respondError(c, http.StatusForbidden, "organization write permission required")
			return
		}
		next(c)
	}
}

func (a *App) requirePolicyWriter(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin := currentAdmin(c)
		if admin == nil || !canWritePolicy(admin.HubRole) {
			respondError(c, http.StatusForbidden, "policy write permission required")
			return
		}
		next(c)
	}
}

func (a *App) requireKeyWriter(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin := currentAdmin(c)
		if admin == nil || !canWriteKey(admin.HubRole) {
			respondError(c, http.StatusForbidden, "key write permission required")
			return
		}
		next(c)
	}
}

func (a *App) requireBudgetWriter(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin := currentAdmin(c)
		if admin == nil || !canWriteBudget(admin.HubRole) {
			respondError(c, http.StatusForbidden, "budget permission required")
			return
		}
		next(c)
	}
}

func (a *App) requireFinanceReader(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin := currentAdmin(c)
		switch admin.HubRole {
		case HubRoleSuperAdmin, HubRoleOrgAdmin, HubRoleFinanceAdmin, HubRoleAuditor:
			next(c)
		default:
			respondError(c, http.StatusForbidden, "finance report permission required")
		}
	}
}

func (a *App) requireSuperAdmin(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin := currentAdmin(c)
		if admin == nil || admin.HubRole != HubRoleSuperAdmin {
			respondError(c, http.StatusForbidden, "hub super admin required")
			return
		}
		next(c)
	}
}

func canWriteOrg(role string) bool {
	return role == HubRoleSuperAdmin || role == HubRoleOrgAdmin
}

func canWritePolicy(role string) bool {
	return role == HubRoleSuperAdmin || role == HubRoleOrgAdmin
}

func canWriteKey(role string) bool {
	return role == HubRoleSuperAdmin || role == HubRoleOrgAdmin || role == HubRoleKeyAdmin
}

func canWriteBudget(role string) bool {
	return role == HubRoleSuperAdmin || role == HubRoleOrgAdmin || role == HubRoleFinanceAdmin
}

func unrestrictedScope(admin *AdminIdentity) bool {
	return admin != nil && (admin.HubRole == HubRoleSuperAdmin || admin.ScopeOrgID == 0)
}

func (a *App) accessibleOrgIDs(admin *AdminIdentity) ([]int, bool, error) {
	if admin == nil {
		return nil, false, errors.New("admin identity missing")
	}
	if unrestrictedScope(admin) {
		return nil, true, nil
	}
	var closures []OrgUnitClosure
	if err := a.db.Where("ancestor_id = ?", admin.ScopeOrgID).Find(&closures).Error; err != nil {
		return nil, false, err
	}
	ids := make([]int, 0, len(closures))
	for _, closure := range closures {
		ids = append(ids, closure.DescendantID)
	}
	if len(ids) == 0 {
		ids = append(ids, admin.ScopeOrgID)
	}
	return ids, false, nil
}

func (a *App) canAccessOrg(admin *AdminIdentity, orgID int) (bool, error) {
	if admin == nil {
		return false, errors.New("admin identity missing")
	}
	if unrestrictedScope(admin) {
		return true, nil
	}
	if orgID <= 0 {
		return false, nil
	}
	var count int64
	err := a.db.Model(&OrgUnitClosure{}).
		Where("ancestor_id = ? AND descendant_id = ?", admin.ScopeOrgID, orgID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0 || orgID == admin.ScopeOrgID, nil
}

func (a *App) requireOrgScope(c *gin.Context, orgID int) bool {
	ok, err := a.canAccessOrg(currentAdmin(c), orgID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return false
	}
	if !ok {
		respondError(c, http.StatusForbidden, "organization scope denied")
		return false
	}
	return true
}

func (a *App) requireKeyScope(c *gin.Context, key EnterpriseKey) bool {
	return a.requireOrgScope(c, key.OrgUnitID)
}

func (a *App) requireBudgetScope(c *gin.Context, budget BudgetAccount) bool {
	admin := currentAdmin(c)
	if unrestrictedScope(admin) {
		return true
	}
	switch budget.ScopeType {
	case "org_unit", OrgTypeProject, OrgTypeCostCenter:
		return a.requireOrgScope(c, budget.ScopeID)
	case "enterprise_key":
		var key EnterpriseKey
		if err := a.db.First(&key, budget.ScopeID).Error; err != nil {
			respondError(c, http.StatusNotFound, err.Error())
			return false
		}
		return a.requireKeyScope(c, key)
	default:
		respondError(c, http.StatusForbidden, "budget scope denied")
		return false
	}
}

func (a *App) authMe(c *gin.Context) {
	respondOK(c, currentAdmin(c))
}

func (a *App) logout(c *gin.Context) {
	respondOK(c, gin.H{"logged_out": true})
}

type referenceUser struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        int    `json:"role"`
	Status      int    `json:"status"`
	Group       string `json:"group"`
}

type referenceChannel struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Type   int    `json:"type"`
	Status int    `json:"status"`
	Group  string `json:"group"`
	Models string `json:"models"`
}

type referenceOrgUnit struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Path      string `json:"path"`
	Status    string `json:"status"`
	ParentID  *int   `json:"parent_id,omitempty"`
	Group     string `json:"default_group" gorm:"column:default_group"`
	NewUserID int    `json:"newapi_user_id" gorm:"column:new_api_user_id"`
}

type referencePolicy struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	DefaultGroup string `json:"default_group"`
	Status       string `json:"status"`
}

type referenceEnterpriseKey struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	OrgUnitID     int    `json:"org_unit_id"`
	NewAPITokenID int    `json:"newapi_token_id"`
	Status        string `json:"status"`
}

func (a *App) referenceData(c *gin.Context) {
	var users []referenceUser
	if err := a.newAPIDB.Model(&model.User{}).
		Order("id asc").
		Limit(1000).
		Find(&users).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var channels []referenceChannel
	if err := a.newAPIDB.Model(&model.Channel{}).
		Order("id asc").
		Limit(1000).
		Find(&channels).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var abilities []model.Ability
	if err := a.newAPIDB.Model(&model.Ability{}).
		Where("enabled = ?", true).
		Limit(10000).
		Find(&abilities).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	groups := map[string]struct{}{}
	models := map[string]struct{}{}
	for group := range ratio_setting.GetGroupRatioCopy() {
		addReferenceValue(groups, group)
	}
	for _, user := range users {
		addReferenceValue(groups, user.Group)
	}
	for _, channel := range channels {
		for _, group := range splitCSV(channel.Group) {
			addReferenceValue(groups, group)
		}
		for _, modelName := range splitCSV(channel.Models) {
			addReferenceValue(models, modelName)
		}
	}
	for _, ability := range abilities {
		addReferenceValue(groups, ability.Group)
		addReferenceValue(models, ability.Model)
	}

	var orgs []referenceOrgUnit
	query := a.db.Model(&OrgUnit{}).Order("path asc, id asc")
	if ids, unrestricted, err := a.accessibleOrgIDs(currentAdmin(c)); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	} else if !unrestricted {
		query = query.Where("id IN ?", ids)
	}
	if err := query.Find(&orgs).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var policies []referencePolicy
	if err := a.db.Model(&Policy{}).Order("id desc").Find(&policies).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	var keys []referenceEnterpriseKey
	keyQuery := a.db.Model(&EnterpriseKey{}).Order("id desc")
	if ids, unrestricted, err := a.accessibleOrgIDs(currentAdmin(c)); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	} else if !unrestricted {
		keyQuery = keyQuery.Where("org_unit_id IN ?", ids)
	}
	if err := keyQuery.Find(&keys).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, gin.H{
		"users":           users,
		"groups":          sortedReferenceValues(groups),
		"channels":        channels,
		"models":          sortedReferenceValues(models),
		"org_units":       orgs,
		"policies":        policies,
		"enterprise_keys": keys,
		"budget_timezone": a.config.BudgetTimezone,
		"quota_currency":  currentQuotaCurrencyConfig(),
	})
}

func addReferenceValue(values map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		values[value] = struct{}{}
	}
}

func sortedReferenceValues(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (a *App) audit(c *gin.Context, action string, targetType string, targetID int, before any, after any) {
	admin := currentAdmin(c)
	if admin == nil {
		return
	}
	beforeJSON := ""
	afterJSON := ""
	if before != nil {
		if data, err := common.Marshal(before); err == nil {
			beforeJSON = string(data)
		}
	}
	if after != nil {
		if data, err := common.Marshal(after); err == nil {
			afterJSON = string(data)
		}
	}
	_ = a.db.Create(&AuditLog{
		AdminNewAPIUserID: admin.NewAPIUserID,
		AdminUsername:     admin.Username,
		AdminRole:         admin.Role,
		HubRole:           admin.HubRole,
		Action:            action,
		TargetType:        targetType,
		TargetID:          targetID,
		BeforeJSON:        beforeJSON,
		AfterJSON:         afterJSON,
		IP:                c.ClientIP(),
		UserAgent:         c.Request.UserAgent(),
	}).Error
}

type orgUnitRequest struct {
	ParentID        *int   `json:"parent_id"`
	Name            string `json:"name"`
	Code            string `json:"code"`
	Type            string `json:"type"`
	Status          string `json:"status"`
	OwnerAdminID    int    `json:"owner_admin_id"`
	DefaultPolicyID int    `json:"default_policy_id"`
	DefaultGroup    string `json:"default_group"`
	NewAPIUserID    int    `json:"newapi_user_id"`
}

func (a *App) listOrgUnits(c *gin.Context) {
	var rows []OrgUnit
	query := a.db.Order("path asc, id asc")
	if ids, unrestricted, err := a.accessibleOrgIDs(currentAdmin(c)); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	} else if !unrestricted {
		query = query.Where("id IN ?", ids)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, rows)
}

func (a *App) createOrgUnit(c *gin.Context) {
	var req orgUnitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		respondError(c, http.StatusBadRequest, "name required")
		return
	}
	if req.Type == "" {
		req.Type = OrgTypeDepartment
	}
	if req.Status == "" {
		req.Status = StatusEnabled
	}
	parentID := 0
	if req.ParentID != nil {
		parentID = *req.ParentID
	}
	if !a.requireOrgScope(c, parentID) {
		return
	}
	row := OrgUnit{
		ParentID:        req.ParentID,
		Name:            req.Name,
		Code:            strings.TrimSpace(req.Code),
		Type:            req.Type,
		Status:          req.Status,
		OwnerAdminID:    req.OwnerAdminID,
		DefaultPolicyID: req.DefaultPolicyID,
		DefaultGroup:    strings.TrimSpace(req.DefaultGroup),
		NewAPIUserID:    req.NewAPIUserID,
	}
	if err := a.ensureOrgCodeAvailable(row.Code, 0); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		parentPath := "/"
		if row.ParentID != nil && *row.ParentID > 0 {
			var parent OrgUnit
			if err := tx.First(&parent, *row.ParentID).Error; err != nil {
				return err
			}
			parentPath = parent.Path
			var closures []OrgUnitClosure
			if err := tx.Where("descendant_id = ?", parent.ID).Find(&closures).Error; err != nil {
				return err
			}
			for _, closure := range closures {
				if err := tx.Create(&OrgUnitClosure{
					AncestorID:   closure.AncestorID,
					DescendantID: row.ID,
					Depth:        closure.Depth + 1,
				}).Error; err != nil {
					return err
				}
			}
		}
		row.Path = parentPath + strconv.Itoa(row.ID) + "/"
		if err := tx.Model(&row).Update("path", row.Path).Error; err != nil {
			return err
		}
		return tx.Create(&OrgUnitClosure{AncestorID: row.ID, DescendantID: row.ID, Depth: 0}).Error
	}); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.audit(c, "org_unit.create", "org_unit", row.ID, nil, row)
	if err := a.ensurePolicyBudgetsAt(time.Now().Unix(), true); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, row)
}

func (a *App) updateOrgUnit(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before OrgUnit
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !a.requireOrgScope(c, before.ID) {
		return
	}
	var req orgUnitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	updates := map[string]any{
		"name":              strings.TrimSpace(req.Name),
		"code":              strings.TrimSpace(req.Code),
		"type":              req.Type,
		"status":            req.Status,
		"owner_admin_id":    req.OwnerAdminID,
		"default_policy_id": req.DefaultPolicyID,
		"default_group":     strings.TrimSpace(req.DefaultGroup),
		"new_api_user_id":   req.NewAPIUserID,
	}
	if updates["name"] == "" {
		delete(updates, "name")
	}
	if updates["type"] == "" {
		delete(updates, "type")
	}
	if updates["status"] == "" {
		delete(updates, "status")
	}
	if code, ok := updates["code"].(string); ok {
		if err := a.ensureOrgCodeAvailable(code, id); err != nil {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := a.db.Model(&OrgUnit{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	var after OrgUnit
	_ = a.db.First(&after, id).Error
	a.audit(c, "org_unit.update", "org_unit", id, before, after)
	if err := a.markKeysPendingForOrg(id); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.ensurePolicyBudgetsAt(time.Now().Unix(), true); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.syncPendingEnterpriseKeys(1000); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, after)
}

func (a *App) ensureOrgCodeAvailable(code string, excludeID int) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	query := a.db.Model(&OrgUnit{}).Where("code = ?", code)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("org unit code %q already exists", code)
	}
	return nil
}

func (a *App) deleteOrgUnit(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before OrgUnit
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !a.requireOrgScope(c, before.ID) {
		return
	}
	var childCount int64
	if err := a.db.Model(&OrgUnit{}).Where("parent_id = ?", id).Count(&childCount).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if childCount > 0 {
		respondError(c, http.StatusConflict, "org unit has child org units; move or delete them first")
		return
	}
	var keyCount int64
	if err := a.db.Model(&EnterpriseKey{}).
		Where("org_unit_id = ? OR project_id = ? OR cost_center_id = ?", id, id, id).
		Count(&keyCount).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if keyCount > 0 {
		respondError(c, http.StatusConflict, "org unit is referenced by enterprise keys; reassign them first")
		return
	}
	var adminCount int64
	if err := a.db.Model(&HubAdminBinding{}).Where("scope_org_unit_id = ?", id).Count(&adminCount).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if adminCount > 0 {
		respondError(c, http.StatusConflict, "org unit is referenced by admin bindings; reassign them first")
		return
	}
	var budgetCount int64
	if err := a.db.Model(&BudgetAccount{}).
		Where("scope_id = ? AND scope_type IN ?", id, []string{"org_unit", OrgTypeProject, OrgTypeCostCenter}).
		Where("(source_type IS NULL OR source_type = '' OR source_type <> ?)", BudgetSourcePolicy).
		Count(&budgetCount).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if budgetCount > 0 {
		respondError(c, http.StatusConflict, "org unit is referenced by budgets; remove them first")
		return
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("ancestor_id = ? OR descendant_id = ?", id, id).Delete(&OrgUnitClosure{}).Error; err != nil {
			return err
		}
		return tx.Delete(&OrgUnit{}, id).Error
	}); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.audit(c, "org_unit.delete", "org_unit", id, before, nil)
	if err := a.ensurePolicyBudgetsAt(time.Now().Unix(), true); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, gin.H{"deleted": true})
}

type policyRequest struct {
	Name                   string           `json:"name"`
	Description            string           `json:"description"`
	DefaultGroup           string           `json:"default_group"`
	AllowedModels          []string         `json:"allowed_models"`
	DeniedModels           []string         `json:"denied_models"`
	MonthlyBudgetQuota     int              `json:"monthly_budget_quota"`
	MonthlyBudgetAmount    *decimal.Decimal `json:"monthly_budget_amount"`
	MonthlyBudgetCurrency  string           `json:"monthly_budget_currency"`
	MonthlyBudgetUnlimited bool             `json:"monthly_budget_unlimited"`
	DailyBudgetQuota       int              `json:"daily_budget_quota"`
	DailyBudgetAmount      *decimal.Decimal `json:"daily_budget_amount"`
	DailyBudgetCurrency    string           `json:"daily_budget_currency"`
	DailyBudgetUnlimited   bool             `json:"daily_budget_unlimited"`
	Currency               string           `json:"currency"`
	KeyDefaultQuota        int              `json:"key_default_quota"`
	KeyDefaultAmount       *decimal.Decimal `json:"key_default_amount"`
	KeyDefaultCurrency     string           `json:"key_default_currency"`
	KeyDefaultUnlimited    bool             `json:"key_default_unlimited"`
	InheritMode            string           `json:"inherit_mode"`
	Status                 string           `json:"status"`
}

type policyView struct {
	Policy
	AllowedModelsList []string `json:"allowed_models_list"`
	DeniedModelsList  []string `json:"denied_models_list"`
}

func policyToView(policy Policy) policyView {
	return policyView{
		Policy:            policy,
		AllowedModelsList: splitCSV(policy.AllowedModels),
		DeniedModelsList:  splitCSV(policy.DeniedModels),
	}
}

func (a *App) listPolicies(c *gin.Context) {
	var rows []Policy
	if err := a.db.Order("id desc").Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]policyView, 0, len(rows))
	for _, row := range rows {
		views = append(views, policyToView(row))
	}
	respondOK(c, views)
}

func (a *App) getPolicy(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var row Policy
	if err := a.db.First(&row, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	respondOK(c, policyToView(row))
}

func (a *App) createPolicy(c *gin.Context) {
	var req policyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	row, err := policyFromRequest(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.db.Create(&row).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.audit(c, "policy.create", "policy", row.ID, nil, row)
	respondOK(c, policyToView(row))
}

func (a *App) updatePolicy(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before Policy
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	var req policyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	after, err := policyFromRequest(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	after.ID = id
	if err := a.db.Model(&after).Select("name", "description", "default_group", "allowed_models", "denied_models",
		"monthly_budget_quota", "monthly_budget_amount", "monthly_budget_currency", "monthly_budget_quota_per_unit", "monthly_budget_exchange_rate",
		"daily_budget_quota", "daily_budget_amount", "daily_budget_currency", "daily_budget_quota_per_unit", "daily_budget_exchange_rate",
		"currency", "key_default_quota", "key_default_amount", "key_default_currency", "key_default_quota_per_unit", "key_default_exchange_rate",
		"inherit_mode", "status").Updates(&after).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = a.db.First(&after, id).Error
	a.audit(c, "policy.update", "policy", id, before, after)
	if err := a.markKeysPendingForPolicy(id); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.ensurePolicyBudgetsAt(time.Now().Unix(), true); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.syncPendingEnterpriseKeys(1000); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, policyToView(after))
}

func (a *App) deletePolicy(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before Policy
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	var orgCount int64
	if err := a.db.Model(&OrgUnit{}).Where("default_policy_id = ?", id).Count(&orgCount).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if orgCount > 0 {
		respondError(c, http.StatusConflict, "policy is referenced by org units; reassign them first")
		return
	}
	var keyCount int64
	if err := a.db.Model(&EnterpriseKey{}).Where("policy_id = ?", id).Count(&keyCount).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if keyCount > 0 {
		respondError(c, http.StatusConflict, "policy is referenced by enterprise keys; reassign them first")
		return
	}
	if err := a.db.Delete(&Policy{}, id).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.audit(c, "policy.delete", "policy", id, before, nil)
	respondOK(c, gin.H{"deleted": true})
}

func policyFromRequest(req policyRequest) (Policy, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Policy{}, errors.New("name required")
	}
	status := req.Status
	if status == "" {
		status = StatusEnabled
	}
	monthly, err := resolveMonetaryQuota(monetaryQuotaInput{
		Amount: req.MonthlyBudgetAmount, Currency: req.MonthlyBudgetCurrency,
		Unlimited: req.MonthlyBudgetUnlimited, LegacyQuota: req.MonthlyBudgetQuota,
	})
	if err != nil {
		return Policy{}, fmt.Errorf("monthly budget: %w", err)
	}
	daily, err := resolveMonetaryQuota(monetaryQuotaInput{
		Amount: req.DailyBudgetAmount, Currency: req.DailyBudgetCurrency,
		Unlimited: req.DailyBudgetUnlimited, LegacyQuota: req.DailyBudgetQuota,
	})
	if err != nil {
		return Policy{}, fmt.Errorf("daily budget: %w", err)
	}
	keyDefault, err := resolveMonetaryQuota(monetaryQuotaInput{
		Amount: req.KeyDefaultAmount, Currency: req.KeyDefaultCurrency,
		Unlimited: req.KeyDefaultUnlimited, LegacyQuota: req.KeyDefaultQuota,
	})
	if err != nil {
		return Policy{}, fmt.Errorf("key default quota: %w", err)
	}
	currency := normalizeQuotaCurrency(req.Currency)
	if currency == "" {
		currency = monthly.Currency
	}
	if currency == "" {
		currency = quotaCurrency
	}
	inheritMode := req.InheritMode
	if inheritMode == "" {
		inheritMode = "intersect"
	}
	return Policy{
		Name:                      name,
		Description:               req.Description,
		DefaultGroup:              strings.TrimSpace(req.DefaultGroup),
		AllowedModels:             joinCSV(req.AllowedModels),
		DeniedModels:              joinCSV(req.DeniedModels),
		MonthlyBudgetQuota:        monthly.Quota,
		MonthlyBudgetAmount:       monthly.Amount,
		MonthlyBudgetCurrency:     monthly.Currency,
		MonthlyBudgetQuotaPerUnit: monthly.QuotaPerUnit,
		MonthlyBudgetExchangeRate: monthly.ExchangeRate,
		DailyBudgetQuota:          daily.Quota,
		DailyBudgetAmount:         daily.Amount,
		DailyBudgetCurrency:       daily.Currency,
		DailyBudgetQuotaPerUnit:   daily.QuotaPerUnit,
		DailyBudgetExchangeRate:   daily.ExchangeRate,
		Currency:                  currency,
		KeyDefaultQuota:           keyDefault.Quota,
		KeyDefaultAmount:          keyDefault.Amount,
		KeyDefaultCurrency:        keyDefault.Currency,
		KeyDefaultQuotaPerUnit:    keyDefault.QuotaPerUnit,
		KeyDefaultExchangeRate:    keyDefault.ExchangeRate,
		InheritMode:               inheritMode,
		Status:                    status,
	}, nil
}

type keyRequest struct {
	Name         string `json:"name"`
	OrgUnitID    int    `json:"org_unit_id"`
	ProjectID    int    `json:"project_id"`
	CostCenterID int    `json:"cost_center_id"`
	PolicyID     int    `json:"policy_id"`
	NewAPIUserID int    `json:"newapi_user_id"`
	Environment  string `json:"environment"`
	Purpose      string `json:"purpose"`
	Contact      string `json:"contact"`
	Status       string `json:"status"`
	SyncNow      *bool  `json:"sync_now"`
}

type enterpriseKeyView struct {
	EnterpriseKey
	ActiveBudgetBlocks int64  `json:"active_budget_blocks"`
	EffectiveStatus    string `json:"effective_status"`
	EffectivePolicyIDs []int  `json:"effective_policy_ids"`
}

func (a *App) listEnterpriseKeys(c *gin.Context) {
	var rows []EnterpriseKey
	query := a.db.Order("id desc")
	if ids, unrestricted, err := a.accessibleOrgIDs(currentAdmin(c)); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	} else if !unrestricted {
		query = query.Where("org_unit_id IN ?", ids)
	}
	if orgID, _ := strconv.Atoi(c.Query("org_unit_id")); orgID > 0 {
		if !a.requireOrgScope(c, orgID) {
			return
		}
		query = query.Where("org_unit_id = ?", orgID)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]enterpriseKeyView, 0, len(rows))
	for _, row := range rows {
		effectivePolicy, err := a.effectivePolicyForKey(row)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		blockCount, err := a.activeBudgetBlockCount(row.ID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		effectiveStatus := row.Status
		if blockCount > 0 {
			effectiveStatus = "budget_blocked"
		} else if row.Status == StatusEnabled && row.NewAPITokenID > 0 {
			var token model.Token
			if err := a.newAPIDB.Select("status", "remain_quota", "unlimited_quota").First(&token, row.NewAPITokenID).Error; err == nil {
				if !token.UnlimitedQuota && token.RemainQuota <= 0 {
					effectiveStatus = "quota_exhausted"
				} else {
					switch token.Status {
					case common.TokenStatusEnabled:
						effectiveStatus = StatusEnabled
					case common.TokenStatusExhausted:
						effectiveStatus = "quota_exhausted"
					case common.TokenStatusExpired:
						effectiveStatus = "expired"
					default:
						effectiveStatus = StatusDisabled
					}
				}
			}
		}
		views = append(views, enterpriseKeyView{
			EnterpriseKey:      row,
			ActiveBudgetBlocks: blockCount,
			EffectiveStatus:    effectiveStatus,
			EffectivePolicyIDs: effectivePolicy.PolicyIDs,
		})
	}
	respondOK(c, views)
}

func (a *App) getEnterpriseKey(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var row EnterpriseKey
	if err := a.db.First(&row, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !a.requireKeyScope(c, row) {
		return
	}
	effective, _ := a.effectivePolicyForKey(row)
	respondOK(c, gin.H{"key": row, "effective_policy": effective})
}

func (a *App) createEnterpriseKey(c *gin.Context) {
	var req keyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	row, err := keyFromRequest(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if !a.requireOrgScope(c, row.OrgUnitID) {
		return
	}
	if admin := currentAdmin(c); admin != nil {
		row.CreatedBy = admin.NewAPIUserID
	}
	if err := a.db.Create(&row).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.audit(c, "key.create", "enterprise_key", row.ID, nil, row)
	if err := a.ensurePolicyBudgetsAt(time.Now().Unix(), true); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	syncNow := req.SyncNow == nil || *req.SyncNow
	result := gin.H{"key": row}
	if syncNow {
		fullKey, err := a.syncEnterpriseKey(row.ID, false)
		if err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		_ = a.db.First(&row, row.ID).Error
		result["key"] = row
		if fullKey != "" {
			result["full_key"] = fullKey
		}
	}
	respondOK(c, result)
}

func (a *App) updateEnterpriseKey(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before EnterpriseKey
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !a.requireKeyScope(c, before) {
		return
	}
	var req keyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	after, err := keyFromRequest(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if !a.requireOrgScope(c, after.OrgUnitID) {
		return
	}
	after.ID = id
	after.NewAPITokenID = before.NewAPITokenID
	after.KeyFingerprint = before.KeyFingerprint
	if err := a.db.Model(&after).Select("name", "org_unit_id", "project_id", "cost_center_id", "policy_id",
		"configured_new_api_user_id", "new_api_user_mode", "new_api_token_name", "status", "sync_status", "environment", "purpose", "contact").Updates(&after).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = a.db.First(&after, id).Error
	a.audit(c, "key.update", "enterprise_key", id, before, after)
	if err := a.ensurePolicyBudgetsAt(time.Now().Unix(), true); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, after)
}

func (a *App) deleteEnterpriseKey(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before EnterpriseKey
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !a.requireKeyScope(c, before) {
		return
	}
	var budgetCount int64
	if err := a.db.Model(&BudgetAccount{}).
		Where("scope_type = ? AND scope_id = ?", "enterprise_key", id).
		Where("(source_type IS NULL OR source_type = '' OR source_type <> ?)", BudgetSourcePolicy).
		Count(&budgetCount).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if budgetCount > 0 {
		respondError(c, http.StatusConflict, "enterprise key is referenced by budgets; remove them first")
		return
	}

	var token model.Token
	if before.NewAPITokenID > 0 {
		if err := a.newAPIDB.First(&token, "id = ?", before.NewAPITokenID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if token.Id > 0 {
			if err := tx.Delete(&token).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&EnterpriseKey{}, id).Error
	}); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if token.Key != "" {
		if err := model.InvalidateTokenCache(token.Key); err != nil {
			common.SysLog("failed to invalidate deleted enterprise token cache: " + err.Error())
		}
	}
	a.audit(c, "key.delete", "enterprise_key", id, before, nil)
	if err := a.ensurePolicyBudgetsAt(time.Now().Unix(), true); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, gin.H{"deleted": true, "newapi_token_revoked": token.Id > 0})
}

func keyFromRequest(req keyRequest) (EnterpriseKey, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return EnterpriseKey{}, errors.New("name required")
	}
	status := req.Status
	if status == "" {
		status = StatusEnabled
	}
	env := req.Environment
	if env == "" {
		env = "prod"
	}
	newAPIUserMode := "inherit"
	if req.NewAPIUserID > 0 {
		newAPIUserMode = "explicit"
	}
	return EnterpriseKey{
		Name:                   name,
		OrgUnitID:              req.OrgUnitID,
		ProjectID:              req.ProjectID,
		CostCenterID:           req.CostCenterID,
		PolicyID:               req.PolicyID,
		ConfiguredNewAPIUserID: req.NewAPIUserID,
		NewAPIUserMode:         newAPIUserMode,
		NewAPITokenName:        name,
		Status:                 status,
		SyncStatus:             StatusPending,
		Environment:            env,
		Purpose:                req.Purpose,
		Contact:                req.Contact,
	}, nil
}

func (a *App) disableEnterpriseKey(c *gin.Context) {
	a.setEnterpriseKeyStatus(c, StatusDisabled)
}

func (a *App) enableEnterpriseKey(c *gin.Context) {
	a.setEnterpriseKeyStatus(c, StatusEnabled)
}

func (a *App) setEnterpriseKeyStatus(c *gin.Context, status string) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before EnterpriseKey
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !a.requireKeyScope(c, before) {
		return
	}
	if err := a.db.Model(&EnterpriseKey{}).Where("id = ?", id).Updates(map[string]any{
		"status":      status,
		"sync_status": StatusPending,
	}).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := a.syncEnterpriseKey(id, false); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	var after EnterpriseKey
	_ = a.db.First(&after, id).Error
	a.audit(c, "key."+status, "enterprise_key", id, before, after)
	respondOK(c, after)
}

func (a *App) syncEnterpriseKeyHandler(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before EnterpriseKey
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !a.requireKeyScope(c, before) {
		return
	}
	fullKey, err := a.syncEnterpriseKey(id, false)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	var row EnterpriseKey
	_ = a.db.First(&row, id).Error
	result := gin.H{"key": row}
	if fullKey != "" {
		result["full_key"] = fullKey
	}
	respondOK(c, result)
}

func (a *App) rotateEnterpriseKey(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before EnterpriseKey
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !a.requireKeyScope(c, before) {
		return
	}
	fullKey, err := a.syncEnterpriseKey(id, true)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	var row EnterpriseKey
	_ = a.db.First(&row, id).Error
	a.audit(c, "key.rotate", "enterprise_key", id, nil, row)
	respondOK(c, gin.H{"key": row, "full_key": fullKey})
}

type EffectivePolicy struct {
	DefaultGroup            string   `json:"default_group"`
	AllowedModels           []string `json:"allowed_models"`
	AllowedModelsRestricted bool     `json:"allowed_models_restricted"`
	DeniedModels            []string `json:"denied_models"`
	MonthlyBudgetQuota      int      `json:"monthly_budget_quota"`
	MonthlyBudgetAmount     string   `json:"monthly_budget_amount"`
	MonthlyBudgetCurrency   string   `json:"monthly_budget_currency"`
	DailyBudgetQuota        int      `json:"daily_budget_quota"`
	DailyBudgetAmount       string   `json:"daily_budget_amount"`
	DailyBudgetCurrency     string   `json:"daily_budget_currency"`
	Currency                string   `json:"currency"`
	KeyDefaultQuota         int      `json:"key_default_quota"`
	KeyDefaultAmount        string   `json:"key_default_amount"`
	KeyDefaultCurrency      string   `json:"key_default_currency"`
	PolicyIDs               []int    `json:"policy_ids"`
}

func (a *App) previewEffectivePolicy(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	orgID, _ := strconv.Atoi(c.Query("org_unit_id"))
	if orgID > 0 && !a.requireOrgScope(c, orgID) {
		return
	}
	key := EnterpriseKey{OrgUnitID: orgID, PolicyID: id}
	effective, err := a.effectivePolicyForKey(key)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, effective)
}

func (a *App) effectivePolicyForKey(key EnterpriseKey) (EffectivePolicy, error) {
	var policies []Policy
	seenPolicyIDs := map[int]bool{}
	if key.OrgUnitID > 0 {
		ancestors, err := a.orgAncestorsRootFirst(key.OrgUnitID)
		if err != nil {
			return EffectivePolicy{}, err
		}
		for _, org := range ancestors {
			if org.DefaultPolicyID <= 0 {
				continue
			}
			var policy Policy
			if err := a.db.First(&policy, org.DefaultPolicyID).Error; err == nil && policy.Status == StatusEnabled && !seenPolicyIDs[policy.ID] {
				policies = append(policies, policy)
				seenPolicyIDs[policy.ID] = true
			}
		}
	}
	if key.PolicyID > 0 && !seenPolicyIDs[key.PolicyID] {
		var policy Policy
		if err := a.db.First(&policy, key.PolicyID).Error; err != nil {
			return EffectivePolicy{}, err
		}
		if policy.Status == StatusEnabled {
			policies = append(policies, policy)
			seenPolicyIDs[policy.ID] = true
		}
	}

	effective := EffectivePolicy{Currency: "quota"}
	for _, policy := range policies {
		allowed := splitCSV(policy.AllowedModels)
		denied := splitCSV(policy.DeniedModels)
		if len(allowed) > 0 {
			effective.AllowedModelsRestricted = true
			if len(effective.AllowedModels) == 0 {
				effective.AllowedModels = allowed
			} else {
				effective.AllowedModels = intersectStrings(effective.AllowedModels, allowed)
			}
		}
		effective.DeniedModels = unionStrings(effective.DeniedModels, denied)
		if policy.DefaultGroup != "" {
			effective.DefaultGroup = policy.DefaultGroup
		}
		if policy.MonthlyBudgetQuota > 0 && (effective.MonthlyBudgetQuota == 0 || policy.MonthlyBudgetQuota < effective.MonthlyBudgetQuota) {
			effective.MonthlyBudgetQuota = policy.MonthlyBudgetQuota
			effective.MonthlyBudgetAmount = policy.MonthlyBudgetAmount
			effective.MonthlyBudgetCurrency = policy.MonthlyBudgetCurrency
		}
		if policy.DailyBudgetQuota > 0 && (effective.DailyBudgetQuota == 0 || policy.DailyBudgetQuota < effective.DailyBudgetQuota) {
			effective.DailyBudgetQuota = policy.DailyBudgetQuota
			effective.DailyBudgetAmount = policy.DailyBudgetAmount
			effective.DailyBudgetCurrency = policy.DailyBudgetCurrency
		}
		if policy.KeyDefaultQuota > 0 {
			effective.KeyDefaultQuota = policy.KeyDefaultQuota
			effective.KeyDefaultAmount = policy.KeyDefaultAmount
			effective.KeyDefaultCurrency = policy.KeyDefaultCurrency
		}
		if policy.Currency != "" {
			effective.Currency = policy.Currency
		}
		effective.PolicyIDs = append(effective.PolicyIDs, policy.ID)
	}
	if len(effective.DeniedModels) > 0 {
		denied := map[string]bool{}
		for _, modelName := range effective.DeniedModels {
			denied[modelName] = true
		}
		filtered := make([]string, 0, len(effective.AllowedModels))
		for _, modelName := range effective.AllowedModels {
			if !denied[modelName] {
				filtered = append(filtered, modelName)
			}
		}
		effective.AllowedModels = filtered
	}
	if effective.DefaultGroup == "" && key.OrgUnitID > 0 {
		var org OrgUnit
		if err := a.db.First(&org, key.OrgUnitID).Error; err == nil {
			effective.DefaultGroup = org.DefaultGroup
		}
	}
	return effective, nil
}

func (a *App) orgAncestorsRootFirst(orgID int) ([]OrgUnit, error) {
	if orgID <= 0 {
		return nil, nil
	}
	var closures []OrgUnitClosure
	if err := a.db.Where("descendant_id = ?", orgID).Order("depth desc").Find(&closures).Error; err != nil {
		return nil, err
	}
	if len(closures) == 0 {
		var org OrgUnit
		if err := a.db.First(&org, orgID).Error; err != nil {
			return nil, err
		}
		return []OrgUnit{org}, nil
	}
	orgs := make([]OrgUnit, 0, len(closures))
	for _, closure := range closures {
		var org OrgUnit
		if err := a.db.First(&org, closure.AncestorID).Error; err != nil {
			return nil, err
		}
		orgs = append(orgs, org)
	}
	return orgs, nil
}

func (a *App) syncEnterpriseKey(id int, rotate bool) (string, error) {
	job := NewAPISyncJob{
		EntityType: "enterprise_key",
		EntityID:   id,
		Operation:  "sync_token",
		Status:     SyncStatusRunning,
	}
	if rotate {
		job.Operation = "rotate_token"
	}
	_ = a.db.Create(&job).Error

	var key EnterpriseKey
	if err := a.db.First(&key, id).Error; err != nil {
		a.failSyncJob(job.ID, err)
		return "", err
	}
	effective, err := a.effectivePolicyForKey(key)
	if err != nil {
		a.failSyncJob(job.ID, err)
		return "", err
	}
	orgNewAPIUserID := 0
	if key.OrgUnitID > 0 {
		var org OrgUnit
		if err := a.db.First(&org, key.OrgUnitID).Error; err == nil {
			orgNewAPIUserID = org.NewAPIUserID
		}
	}
	newAPIUserMode := key.NewAPIUserMode
	configuredNewAPIUserID := key.ConfiguredNewAPIUserID
	if newAPIUserMode == "" {
		newAPIUserMode = "inherit"
		if configuredNewAPIUserID > 0 || key.NewAPIUserID > 0 && key.NewAPIUserID != orgNewAPIUserID {
			newAPIUserMode = "explicit"
			if configuredNewAPIUserID == 0 {
				configuredNewAPIUserID = key.NewAPIUserID
			}
		}
	}
	if newAPIUserMode == "inherit" {
		configuredNewAPIUserID = 0
	}
	newAPIUserID := configuredNewAPIUserID
	if newAPIUserMode == "inherit" {
		newAPIUserID = orgNewAPIUserID
	}
	if newAPIUserID == 0 {
		err := errors.New("newapi_user_id required on enterprise key or org unit")
		a.failSyncJob(job.ID, err)
		return "", err
	}
	if effective.DefaultGroup == "" {
		err := errors.New("effective default_group required")
		a.failSyncJob(job.ID, err)
		return "", err
	}

	modelLimitValues, restrictAllModels, err := a.resolvePolicyModelLimits(effective)
	if err != nil {
		a.failSyncJob(job.ID, err)
		return "", err
	}
	activeBudgetBlocks, err := a.activeBudgetBlockCount(key.ID)
	if err != nil {
		a.failSyncJob(job.ID, err)
		return "", err
	}
	status := common.TokenStatusEnabled
	if key.Status != StatusEnabled || activeBudgetBlocks > 0 || restrictAllModels {
		status = common.TokenStatusDisabled
	}
	modelLimits := joinCSV(modelLimitValues)
	unlimited := effective.KeyDefaultQuota <= 0
	remainQuota := effective.KeyDefaultQuota

	fullKey := ""
	if key.NewAPITokenID == 0 {
		tokenKey, err := common.GenerateKey()
		if err != nil {
			a.failSyncJob(job.ID, err)
			return "", err
		}
		token := model.Token{
			UserId:             newAPIUserID,
			Name:               key.Name,
			Key:                tokenKey,
			CreatedTime:        common.GetTimestamp(),
			AccessedTime:       common.GetTimestamp(),
			ExpiredTime:        -1,
			RemainQuota:        remainQuota,
			UnlimitedQuota:     unlimited,
			ModelLimitsEnabled: modelLimits != "",
			ModelLimits:        modelLimits,
			Group:              effective.DefaultGroup,
			Status:             status,
		}
		if err := a.newAPIDB.Create(&token).Error; err != nil {
			a.failSyncJob(job.ID, err)
			return "", err
		}
		fullKey = "sk-" + tokenKey
		key.NewAPITokenID = token.Id
		key.KeyFingerprint = fingerprintToken(tokenKey)
		key.AppliedKeyQuota = effective.KeyDefaultQuota
	} else {
		var token model.Token
		if err := a.newAPIDB.First(&token, "id = ?", key.NewAPITokenID).Error; err != nil {
			a.failSyncJob(job.ID, err)
			return "", err
		}
		oldTokenKey := token.Key
		appliedQuota := key.AppliedKeyQuota
		if appliedQuota <= 0 && !token.UnlimitedQuota {
			appliedQuota = token.RemainQuota + token.UsedQuota
		}
		if unlimited {
			remainQuota = token.RemainQuota
		} else if token.UnlimitedQuota || appliedQuota <= 0 {
			remainQuota = effective.KeyDefaultQuota
		} else {
			remainQuota = token.RemainQuota + effective.KeyDefaultQuota - appliedQuota
			if remainQuota < 0 {
				remainQuota = 0
			}
		}
		remainQuotaDelta := remainQuota - token.RemainQuota
		replaceCachedRemainQuota := token.UnlimitedQuota && !unlimited
		updates := map[string]any{
			"user_id":              newAPIUserID,
			"name":                 key.Name,
			"status":               status,
			"expired_time":         -1,
			"remain_quota":         remainQuota,
			"unlimited_quota":      unlimited,
			"model_limits_enabled": modelLimits != "",
			"model_limits":         modelLimits,
			"group":                effective.DefaultGroup,
		}
		if rotate {
			tokenKey, err := common.GenerateKey()
			if err != nil {
				a.failSyncJob(job.ID, err)
				return "", err
			}
			updates["key"] = tokenKey
			token.Key = tokenKey
			fullKey = "sk-" + tokenKey
			key.KeyFingerprint = fingerprintToken(tokenKey)
		}
		if err := a.newAPIDB.Model(&model.Token{}).Where("id = ?", token.Id).Updates(updates).Error; err != nil {
			a.failSyncJob(job.ID, err)
			return "", err
		}
		token.UserId = newAPIUserID
		token.Name = key.Name
		token.Status = status
		token.ExpiredTime = -1
		token.RemainQuota = remainQuota
		token.UnlimitedQuota = unlimited
		token.ModelLimitsEnabled = modelLimits != ""
		token.ModelLimits = modelLimits
		token.Group = effective.DefaultGroup
		cacheQuotaValue := remainQuotaDelta
		if replaceCachedRemainQuota {
			cacheQuotaValue = remainQuota
		}
		if err := model.UpdateTokenCacheAfterExternalWrite(oldTokenKey, token, cacheQuotaValue, replaceCachedRemainQuota); err != nil {
			a.failSyncJob(job.ID, err)
			return "", err
		}
		key.AppliedKeyQuota = effective.KeyDefaultQuota
	}
	key.NewAPIUserID = newAPIUserID
	key.NewAPITokenName = key.Name
	key.SyncStatus = StatusSynced
	if err := a.db.Model(&EnterpriseKey{}).Where("id = ?", key.ID).Updates(map[string]any{
		"new_api_user_id":            key.NewAPIUserID,
		"configured_new_api_user_id": configuredNewAPIUserID,
		"new_api_user_mode":          newAPIUserMode,
		"new_api_token_id":           key.NewAPITokenID,
		"new_api_token_name":         key.NewAPITokenName,
		"key_fingerprint":            key.KeyFingerprint,
		"applied_key_quota":          key.AppliedKeyQuota,
		"sync_status":                key.SyncStatus,
	}).Error; err != nil {
		a.failSyncJob(job.ID, err)
		return fullKey, err
	}
	_ = a.db.Model(&NewAPISyncJob{}).Where("id = ?", job.ID).Updates(map[string]any{"status": SyncStatusDone}).Error
	return fullKey, nil
}

func (a *App) failSyncJob(id int, err error) {
	if id == 0 || err == nil {
		return
	}
	_ = a.db.Model(&NewAPISyncJob{}).Where("id = ?", id).Updates(map[string]any{
		"status":        SyncStatusFailed,
		"error_message": err.Error(),
	}).Error
}

type adminBindingRequest struct {
	NewAPIUserID   int    `json:"newapi_user_id"`
	NewAPIUsername string `json:"newapi_username"`
	HubRole        string `json:"hub_role"`
	ScopeOrgUnitID int    `json:"scope_org_unit_id"`
	Status         string `json:"status"`
}

func (a *App) listAdminBindings(c *gin.Context) {
	var rows []HubAdminBinding
	if err := a.db.Order("id desc").Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, rows)
}

func (a *App) createAdminBinding(c *gin.Context) {
	var req adminBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	row, err := adminBindingFromRequest(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.db.Create(&row).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.audit(c, "admin_binding.create", "admin_binding", row.ID, nil, row)
	respondOK(c, row)
}

func (a *App) updateAdminBinding(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before HubAdminBinding
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	var req adminBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	after, err := adminBindingFromRequest(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	after.ID = id
	if err := a.db.Model(&after).Select("new_api_user_id", "new_api_username", "hub_role", "scope_org_unit_id", "status").Updates(&after).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = a.db.First(&after, id).Error
	a.audit(c, "admin_binding.update", "admin_binding", id, before, after)
	respondOK(c, after)
}

func (a *App) deleteAdminBinding(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before HubAdminBinding
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if err := a.db.Delete(&HubAdminBinding{}, id).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.audit(c, "admin_binding.delete", "admin_binding", id, before, nil)
	respondOK(c, gin.H{"deleted": true})
}

func adminBindingFromRequest(req adminBindingRequest) (HubAdminBinding, error) {
	if req.NewAPIUserID <= 0 {
		return HubAdminBinding{}, errors.New("newapi_user_id required")
	}
	role := req.HubRole
	if role == "" {
		role = HubRoleOrgAdmin
	}
	status := req.Status
	if status == "" {
		status = StatusEnabled
	}
	return HubAdminBinding{
		NewAPIUserID:   req.NewAPIUserID,
		NewAPIUsername: strings.TrimSpace(req.NewAPIUsername),
		HubRole:        role,
		ScopeOrgUnitID: req.ScopeOrgUnitID,
		Status:         status,
	}, nil
}

type budgetRequest struct {
	ScopeType      string           `json:"scope_type"`
	ScopeID        int              `json:"scope_id"`
	PeriodStart    int64            `json:"period_start"`
	PeriodEnd      int64            `json:"period_end"`
	BudgetQuota    int              `json:"budget_quota"`
	BudgetAmount   *decimal.Decimal `json:"budget_amount"`
	BudgetCurrency string           `json:"budget_currency"`
	Currency       string           `json:"currency"`
	Status         string           `json:"status"`
}

type budgetView struct {
	BudgetAccount
	ActiveBlockCount   int64 `json:"active_block_count"`
	ConfirmedUsedQuota int   `json:"confirmed_used_quota"`
}

func (a *App) listBudgets(c *gin.Context) {
	var rows []BudgetAccount
	query := a.db.Order("id desc")
	admin := currentAdmin(c)
	if ids, unrestricted, err := a.accessibleOrgIDs(admin); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	} else if !unrestricted {
		var keyIDs []int
		if err := a.db.Model(&EnterpriseKey{}).Where("org_unit_id IN ?", ids).Pluck("id", &keyIDs).Error; err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		query = query.Where(
			"(scope_type = ? AND scope_id IN ?) OR (scope_type IN ? AND scope_id IN ?) OR (scope_type = ? AND scope_id IN ?)",
			"org_unit", ids,
			[]string{OrgTypeProject, OrgTypeCostCenter}, ids,
			"enterprise_key", keyIDs,
		)
	}
	if scopeType := strings.TrimSpace(c.Query("scope_type")); scopeType != "" {
		query = query.Where("scope_type = ?", scopeType)
	}
	if scopeID, _ := strconv.Atoi(c.Query("scope_id")); scopeID > 0 {
		if !a.requireBudgetScope(c, BudgetAccount{ScopeType: strings.TrimSpace(c.Query("scope_type")), ScopeID: scopeID}) {
			return
		}
		query = query.Where("scope_id = ?", scopeID)
	}
	if err := query.Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]budgetView, 0, len(rows))
	for _, row := range rows {
		var blockCount int64
		if err := a.db.Model(&BudgetKeyBlock{}).
			Where("budget_account_id = ? AND status = ?", row.ID, BudgetBlockActive).
			Count(&blockCount).Error; err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
		if row.SourceType == "" {
			row.SourceType = BudgetSourceManual
		}
		if row.PeriodKind == "" {
			row.PeriodKind = BudgetPeriodCustom
		}
		views = append(views, budgetView{
			BudgetAccount:      row,
			ActiveBlockCount:   blockCount,
			ConfirmedUsedQuota: row.UsedQuota - row.PendingQuota,
		})
	}
	respondOK(c, views)
}

func (a *App) createBudget(c *gin.Context) {
	var req budgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	row, err := budgetFromRequest(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if row.BudgetQuota <= 0 {
		respondError(c, http.StatusBadRequest, "budget_quota must be greater than zero")
		return
	}
	if row.PeriodStart > 0 && row.PeriodEnd > 0 && row.PeriodEnd <= row.PeriodStart {
		respondError(c, http.StatusBadRequest, "period_end must be later than period_start")
		return
	}
	if !a.requireBudgetScope(c, row) {
		return
	}
	if err := a.db.Create(&row).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.audit(c, "budget.create", "budget", row.ID, nil, row)
	respondOK(c, row)
}

func (a *App) updateBudget(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before BudgetAccount
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !a.requireBudgetScope(c, before) {
		return
	}
	var req budgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	after, err := budgetFromRequest(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if before.SourceType == BudgetSourcePolicy {
		respondError(c, http.StatusConflict, "policy-managed budgets must be changed through Policy")
		return
	}
	if after.BudgetQuota <= 0 {
		respondError(c, http.StatusBadRequest, "budget_quota must be greater than zero")
		return
	}
	if after.PeriodStart > 0 && after.PeriodEnd > 0 && after.PeriodEnd <= after.PeriodStart {
		respondError(c, http.StatusBadRequest, "period_end must be later than period_start")
		return
	}
	if !a.requireBudgetScope(c, after) {
		return
	}
	after.ID = id
	if err := a.db.Model(&after).Select("scope_type", "scope_id", "period_start", "period_end", "budget_quota",
		"budget_amount", "budget_currency", "budget_quota_per_unit", "budget_exchange_rate", "currency", "status").Updates(&after).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = a.db.First(&after, id).Error
	a.audit(c, "budget.update", "budget", id, before, after)
	if _, err := a.releaseBudgetBlocks(id); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.reconcileBudgetEnforcement(time.Now().Unix()); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, after)
}

func (a *App) resetBudget(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before BudgetAccount
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !a.requireBudgetScope(c, before) {
		return
	}
	if before.SourceType == BudgetSourcePolicy {
		respondError(c, http.StatusConflict, "policy-managed budgets reset automatically with their natural period")
		return
	}
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&BudgetAccount{}).Where("id = ?", id).Updates(map[string]any{
			"used_quota":    0,
			"pending_quota": 0,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&BudgetTransaction{}).Where("budget_account_id = ? AND pending = ?", id, true).Update("pending", false).Error
	}); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := a.releaseBudgetBlocks(id); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	var after BudgetAccount
	_ = a.db.First(&after, id).Error
	a.audit(c, "budget.reset", "budget", id, before, after)
	respondOK(c, after)
}

func (a *App) deleteBudget(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var before BudgetAccount
	if err := a.db.First(&before, id).Error; err != nil {
		respondError(c, http.StatusNotFound, err.Error())
		return
	}
	if !a.requireBudgetScope(c, before) {
		return
	}
	if before.SourceType == BudgetSourcePolicy {
		respondError(c, http.StatusConflict, "policy-managed budgets must be changed through Policy")
		return
	}
	if _, err := a.releaseBudgetBlocks(id); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.db.Delete(&BudgetAccount{}, id).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.audit(c, "budget.delete", "budget", id, before, nil)
	respondOK(c, gin.H{"deleted": true})
}

func budgetFromRequest(req budgetRequest) (BudgetAccount, error) {
	status := req.Status
	if status == "" {
		status = StatusEnabled
	}
	monetary, err := resolveMonetaryQuota(monetaryQuotaInput{
		Amount: req.BudgetAmount, Currency: req.BudgetCurrency,
		LegacyQuota: req.BudgetQuota, RequirePositive: true,
	})
	if err != nil {
		return BudgetAccount{}, fmt.Errorf("budget: %w", err)
	}
	currency := monetary.Currency
	if currency == "" {
		currency = normalizeQuotaCurrency(req.Currency)
	}
	if currency == "" {
		currency = quotaCurrency
	}
	return BudgetAccount{
		ScopeType:          req.ScopeType,
		ScopeID:            req.ScopeID,
		PeriodStart:        req.PeriodStart,
		PeriodEnd:          req.PeriodEnd,
		BudgetQuota:        monetary.Quota,
		BudgetAmount:       monetary.Amount,
		BudgetCurrency:     monetary.Currency,
		BudgetQuotaPerUnit: monetary.QuotaPerUnit,
		BudgetExchangeRate: monetary.ExchangeRate,
		Currency:           currency,
		Status:             status,
		SourceType:         BudgetSourceManual,
		PeriodKind:         BudgetPeriodCustom,
	}, nil
}

func (a *App) syncUsageHandler(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "1000"))
	if limit <= 0 || limit > 10000 {
		limit = 1000
	}
	result, err := a.SyncUsage(limit)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.audit(c, "usage.sync", "usage", 0, nil, result)
	respondOK(c, result)
}

type UsageSyncResult struct {
	LastLogID        int `json:"last_log_id"`
	ScannedLogs      int `json:"scanned_logs"`
	ImportedLedgers  int `json:"imported_ledgers"`
	SkippedLogs      int `json:"skipped_logs"`
	DisabledKeyCount int `json:"disabled_key_count"`
}

type taskUsageLogMetadata struct {
	IsTask bool   `json:"is_task"`
	TaskID string `json:"task_id"`
}

func taskUsageMetadata(logRow model.Log) taskUsageLogMetadata {
	var metadata taskUsageLogMetadata
	if logRow.Other != "" {
		_ = common.UnmarshalJsonStr(logRow.Other, &metadata)
	}
	metadata.TaskID = strings.TrimSpace(metadata.TaskID)
	return metadata
}

func (a *App) SyncUsage(limit int) (UsageSyncResult, error) {
	a.usageSyncMu.Lock()
	defer a.usageSyncMu.Unlock()
	now := time.Now().Unix()
	if err := a.ensurePolicyBudgetsAt(now, true); err != nil {
		return UsageSyncResult{}, err
	}
	lastID := a.getIntSetting("last_newapi_log_id")
	var logs []model.Log
	if err := a.newAPILog.Where("id > ? AND token_id > 0 AND quota <> 0", lastID).Order("id asc").Limit(limit).Find(&logs).Error; err != nil {
		return UsageSyncResult{}, err
	}
	result := UsageSyncResult{LastLogID: lastID, ScannedLogs: len(logs)}
	ensuredPeriods := make(map[int64]struct{})
	for _, logRow := range logs {
		if logRow.Id > result.LastLogID {
			result.LastLogID = logRow.Id
		}
		var key EnterpriseKey
		if err := a.db.Where("new_api_token_id = ?", logRow.TokenId).First(&key).Error; err != nil {
			result.SkippedLogs++
			continue
		}
		periodStart, _ := a.policyBudgetPeriod(BudgetPeriodDaily, logRow.CreatedAt)
		if _, ok := ensuredPeriods[periodStart]; !ok {
			if err := a.ensurePolicyBudgetsAt(logRow.CreatedAt, false); err != nil {
				return result, err
			}
			ensuredPeriods[periodStart] = struct{}{}
		}
		quota := logRow.Quota
		if logRow.Type == model.LogTypeRefund && quota > 0 {
			quota = -quota
		}
		metadata := taskUsageMetadata(logRow)
		usageState := UsageStateSettled
		if metadata.IsTask && metadata.TaskID != "" {
			usageState = UsageStatePending
		}
		var ledger OrganizationUsageLedger
		err := a.db.Where("new_api_log_id = ?", logRow.Id).First(&ledger).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ledger = OrganizationUsageLedger{
				NewAPILogID:     logRow.Id,
				NewAPITokenID:   logRow.TokenId,
				EnterpriseKeyID: key.ID,
				OrgUnitID:       key.OrgUnitID,
				ProjectID:       key.ProjectID,
				CostCenterID:    key.CostCenterID,
				ModelName:       logRow.ModelName,
				ChannelID:       logRow.ChannelId,
				UseGroup:        logRow.Group,
				Quota:           quota,
				Amount:          float64(quota) / float64(common.QuotaPerUnit),
				Currency:        operation_setting.QuotaDisplayTypeUSD,
				TaskID:          metadata.TaskID,
				UsageState:      usageState,
				CreatedAt:       logRow.CreatedAt,
			}
			if err := a.db.Create(&ledger).Error; err != nil {
				return result, err
			}
			result.ImportedLedgers++
		} else if err != nil {
			return result, err
		} else {
			result.SkippedLogs++
		}
		disabled, err := a.applyBudgetTransactions(ledger, key)
		if err != nil {
			return result, err
		}
		result.DisabledKeyCount += disabled
		if metadata.TaskID != "" && !metadata.IsTask {
			settled, err := a.settlePendingTaskBudgetTransactions(metadata.TaskID, logRow.CreatedAt)
			if err != nil {
				return result, err
			}
			result.DisabledKeyCount += settled
		}
	}
	settled, err := a.reconcileCompletedTaskUsage(now)
	if err != nil {
		return result, err
	}
	result.DisabledKeyCount += settled
	if err := a.setSetting("last_newapi_log_id", strconv.Itoa(result.LastLogID)); err != nil {
		return result, err
	}
	if err := a.reconcileBudgetEnforcement(now); err != nil {
		return result, err
	}
	if err := a.syncPendingEnterpriseKeys(1000); err != nil {
		return result, err
	}
	return result, nil
}

func (a *App) applyBudgetTransactions(ledger OrganizationUsageLedger, key EnterpriseKey) (int, error) {
	if ledger.UsageState == "" {
		ledger.UsageState = UsageStateSettled
	}
	if ledger.UsageState == UsageStatePending {
		return 0, nil
	}
	scopePairs := []struct {
		scopeType string
		scopeID   int
	}{
		{"enterprise_key", ledger.EnterpriseKeyID},
		{"project", ledger.ProjectID},
		{"cost_center", ledger.CostCenterID},
	}
	if ledger.OrgUnitID > 0 {
		ancestors, err := a.orgAncestorsRootFirst(ledger.OrgUnitID)
		if err != nil {
			return 0, err
		}
		for _, org := range ancestors {
			scopePairs = append(scopePairs, struct {
				scopeType string
				scopeID   int
			}{"org_unit", org.ID})
		}
	}
	disabled := 0
	for _, pair := range scopePairs {
		if pair.scopeID <= 0 {
			continue
		}
		var accounts []BudgetAccount
		query := a.db.Where("scope_type = ? AND scope_id = ? AND status = ?", pair.scopeType, pair.scopeID, StatusEnabled)
		if ledger.CreatedAt > 0 {
			query = query.Where("(period_start = 0 OR period_start <= ?) AND (period_end = 0 OR period_end > ?)", ledger.CreatedAt, ledger.CreatedAt)
		}
		if err := query.Find(&accounts).Error; err != nil {
			return disabled, err
		}
		for _, account := range accounts {
			var transactionCount int64
			if err := a.db.Model(&BudgetTransaction{}).
				Where("budget_account_id = ? AND new_api_log_id = ?", account.ID, ledger.NewAPILogID).
				Count(&transactionCount).Error; err != nil {
				return disabled, err
			}
			if transactionCount > 0 {
				continue
			}
			direction := "consume"
			if ledger.Quota < 0 {
				direction = "refund"
			}
			if err := a.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Create(&BudgetTransaction{
					BudgetAccountID: account.ID,
					EnterpriseKeyID: ledger.EnterpriseKeyID,
					NewAPILogID:     ledger.NewAPILogID,
					SourceType:      "newapi_log",
					SourceID:        ledger.NewAPILogID,
					Quota:           ledger.Quota,
					Direction:       direction,
					TaskID:          ledger.TaskID,
					Pending:         false,
				}).Error; err != nil {
					return err
				}
				updates := map[string]any{"used_quota": gorm.Expr("used_quota + ?", ledger.Quota)}
				return tx.Model(&BudgetAccount{}).Where("id = ?", account.ID).Updates(updates).Error
			}); err != nil {
				return disabled, err
			}
			var refreshed BudgetAccount
			if err := a.db.First(&refreshed, account.ID).Error; err != nil {
				return disabled, err
			}
			if budgetShouldBlock(refreshed, time.Now().Unix()) {
				count, err := a.ensureBudgetBlocks(refreshed, key)
				if err != nil {
					return disabled, err
				}
				disabled += count
			} else {
				if _, err := a.releaseBudgetBlocks(refreshed.ID); err != nil {
					return disabled, err
				}
			}
		}
	}
	return disabled, nil
}

func (a *App) usageSummary(c *gin.Context) {
	groupBy := c.DefaultQuery("group_by", "org_unit")
	switch groupBy {
	case "org_unit", "key", "model", "channel", "project", "cost_center":
	default:
		groupBy = "org_unit"
	}
	var ledgers []OrganizationUsageLedger
	query := a.db.Model(&OrganizationUsageLedger{})
	if ids, unrestricted, err := a.accessibleOrgIDs(currentAdmin(c)); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	} else if !unrestricted {
		query = query.Where("org_unit_id IN ?", ids)
	}
	var ok bool
	query, ok = a.applyUsageFilters(c, query)
	if !ok {
		return
	}
	if err := query.Find(&ledgers).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	type summaryRow struct {
		Key             string  `json:"key"`
		Quota           int     `json:"quota"`
		PendingQuota    int     `json:"pending_quota"`
		ConfirmedQuota  int     `json:"confirmed_quota"`
		Amount          float64 `json:"amount"`
		PendingAmount   float64 `json:"pending_amount"`
		ConfirmedAmount float64 `json:"confirmed_amount"`
		Count           int64   `json:"count"`
	}
	rowsByKey := map[string]*summaryRow{}
	for _, ledger := range ledgers {
		key := strconv.Itoa(ledger.OrgUnitID)
		switch groupBy {
		case "key":
			key = strconv.Itoa(ledger.EnterpriseKeyID)
		case "model":
			key = ledger.ModelName
		case "channel":
			key = strconv.Itoa(ledger.ChannelID)
		case "project":
			key = strconv.Itoa(ledger.ProjectID)
		case "cost_center":
			key = strconv.Itoa(ledger.CostCenterID)
		}
		if key == "" {
			key = "(empty)"
		}
		row := rowsByKey[key]
		if row == nil {
			row = &summaryRow{Key: key}
			rowsByKey[key] = row
		}
		row.Quota += ledger.Quota
		row.Amount += ledger.Amount
		if ledger.UsageState != UsageStateSettled {
			row.PendingQuota += ledger.Quota
			row.PendingAmount += ledger.Amount
		} else {
			row.ConfirmedQuota += ledger.Quota
			row.ConfirmedAmount += ledger.Amount
		}
		row.Count++
	}
	rows := make([]summaryRow, 0, len(rowsByKey))
	for _, row := range rowsByKey {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Quota > rows[j].Quota
	})
	respondOK(c, rows)
}

func (a *App) usageDetails(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var rows []OrganizationUsageLedger
	query := a.db.Order("id desc").Limit(limit)
	if ids, unrestricted, err := a.accessibleOrgIDs(currentAdmin(c)); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	} else if !unrestricted {
		query = query.Where("org_unit_id IN ?", ids)
	}
	var ok bool
	query, ok = a.applyUsageFilters(c, query)
	if !ok {
		return
	}
	if err := query.Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, rows)
}

func (a *App) applyUsageFilters(c *gin.Context, query *gorm.DB) (*gorm.DB, bool) {
	if keyID, _ := strconv.Atoi(c.Query("enterprise_key_id")); keyID > 0 {
		query = query.Where("enterprise_key_id = ?", keyID)
	}
	if orgID, _ := strconv.Atoi(c.Query("org_unit_id")); orgID > 0 {
		if !a.requireOrgScope(c, orgID) {
			return query, false
		}
		query = query.Where("org_unit_id = ?", orgID)
	}
	if modelName := strings.TrimSpace(c.Query("model_name")); modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	if channelID, _ := strconv.Atoi(c.Query("channel_id")); channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	if projectID, _ := strconv.Atoi(c.Query("project_id")); projectID > 0 {
		query = query.Where("project_id = ?", projectID)
	}
	if costCenterID, _ := strconv.Atoi(c.Query("cost_center_id")); costCenterID > 0 {
		query = query.Where("cost_center_id = ?", costCenterID)
	}
	if start, _ := strconv.ParseInt(c.Query("created_at_start"), 10, 64); start > 0 {
		query = query.Where("created_at >= ?", start)
	}
	if end, _ := strconv.ParseInt(c.Query("created_at_end"), 10, 64); end > 0 {
		query = query.Where("created_at < ?", end)
	}
	return query, true
}

func (a *App) auditLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var rows []AuditLog
	if err := a.db.Order("id desc").Limit(limit).Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, rows)
}

func (a *App) syncJobs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var rows []NewAPISyncJob
	if err := a.db.Order("id desc").Limit(limit).Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, rows)
}

func (a *App) getIntSetting(key string) int {
	var setting Setting
	if err := a.db.First(&setting, "key = ?", key).Error; err != nil {
		return 0
	}
	value, _ := strconv.Atoi(setting.Value)
	return value
}

func (a *App) setSetting(key string, value string) error {
	var setting Setting
	err := a.db.First(&setting, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return a.db.Create(&Setting{Key: key, Value: value}).Error
	}
	if err != nil {
		return err
	}
	return a.db.Model(&Setting{}).Where("key = ?", key).Update("value", value).Error
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		result = append(result, part)
	}
	sort.Strings(result)
	return result
}

func joinCSV(values []string) string {
	return strings.Join(splitCSV(strings.Join(values, ",")), ",")
}

func unionStrings(a []string, b []string) []string {
	seen := map[string]bool{}
	for _, value := range a {
		if value != "" {
			seen[value] = true
		}
	}
	for _, value := range b {
		if value != "" {
			seen[value] = true
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func intersectStrings(a []string, b []string) []string {
	allowed := map[string]bool{}
	for _, value := range b {
		allowed[value] = true
	}
	result := make([]string, 0)
	for _, value := range a {
		if allowed[value] {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func fingerprintToken(key string) string {
	key = strings.TrimPrefix(key, "sk-")
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return "sk-" + key[:4] + "..." + key[len(key)-4:]
}
