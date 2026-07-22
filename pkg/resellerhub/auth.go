package resellerhub

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	HubRoleSuperAdmin     = "hub_super_admin"
	HubRoleResellerAdmin  = "reseller_admin"
	HubRoleResellerViewer = "reseller_viewer"

	membershipStatusActive   = "active"
	membershipStatusDisabled = "disabled"
)

var (
	errMembershipRequired = errors.New("active Reseller Hub membership required")
	errResellerInactive   = errors.New("reseller is not active")
)

type Identity struct {
	NewAPIUserID int    `json:"new_api_user_id"`
	Username     string `json:"username"`
	Role         int    `json:"role"`
	Status       int    `json:"status"`
	Group        string `json:"group"`
	HubRole      string `json:"hub_role"`
	ResellerID   int    `json:"reseller_id,omitempty"`
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

func (a *App) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, err := a.authenticate(c.Request)
		if err != nil {
			status := http.StatusUnauthorized
			if errors.Is(err, errMembershipRequired) || errors.Is(err, errResellerInactive) {
				status = http.StatusForbidden
			}
			respondError(c, status, err.Error())
			return
		}
		c.Set("reseller_hub_identity", identity)
		c.Next()
	}
}

func (a *App) authenticate(r *http.Request) (*Identity, error) {
	identity, err := a.authenticateWithGateway(r)
	if err != nil {
		return nil, err
	}
	if identity.Status != common.UserStatusEnabled {
		return nil, errors.New("account is disabled")
	}
	if identity.Role >= common.RoleRootUser {
		identity.HubRole = HubRoleSuperAdmin
		return identity, nil
	}

	var membership Membership
	err = a.db.Where("new_api_user_id = ? AND status = ?", identity.NewAPIUserID, membershipStatusActive).First(&membership).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errMembershipRequired
		}
		return nil, err
	}
	var reseller Reseller
	if err = a.db.Where("id = ? AND status = ?", membership.ResellerID, "active").First(&reseller).Error; err != nil {
		return nil, errResellerInactive
	}
	identity.HubRole = membership.Role
	identity.ResellerID = membership.ResellerID
	return identity, nil
}

func (a *App) authenticateWithGateway(r *http.Request) (*Identity, error) {
	if a.config.GatewayBaseURL != "" {
		ctx, cancel := context.WithTimeout(r.Context(), a.config.AuthTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.config.GatewayBaseURL+"/api/user/self", nil)
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
			return nil, fmt.Errorf("gateway identity check failed: %w", err)
		}
		defer resp.Body.Close()
		var payload newAPIUserSelfResponse
		if err = common.DecodeJson(resp.Body, &payload); err != nil {
			return nil, fmt.Errorf("decode gateway identity: %w", err)
		}
		if !payload.Success {
			if payload.Message == "" {
				payload.Message = "gateway identity check failed"
			}
			return nil, errors.New(payload.Message)
		}
		return &Identity{
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
	return &Identity{
		NewAPIUserID: user.Id,
		Username:     user.Username,
		Role:         user.Role,
		Status:       user.Status,
		Group:        user.Group,
	}, nil
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
	return err == nil && parsed.Host != "" && strings.EqualFold(parsed.Host, r.Host)
}

func currentIdentity(c *gin.Context) *Identity {
	value, exists := c.Get("reseller_hub_identity")
	if !exists {
		return nil
	}
	identity, _ := value.(*Identity)
	return identity
}

func requireRoot(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := currentIdentity(c)
		if identity == nil || identity.HubRole != HubRoleSuperAdmin {
			respondError(c, http.StatusForbidden, "hub super admin required")
			return
		}
		next(c)
	}
}

func requireResellerWriter(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity := currentIdentity(c)
		if identity == nil || (identity.HubRole != HubRoleSuperAdmin && identity.HubRole != HubRoleResellerAdmin) {
			respondError(c, http.StatusForbidden, "reseller write permission required")
			return
		}
		next(c)
	}
}
