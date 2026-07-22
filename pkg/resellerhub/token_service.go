package resellerhub

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type tokenRequest struct {
	Name        string   `json:"name"`
	Group       string   `json:"group"`
	Models      []string `json:"models"`
	ExpiredTime *int64   `json:"expired_time"`
	ExpiresAt   string   `json:"expires_at"`
	Status      string   `json:"status"`
}

type tokenView struct {
	MappingID      int      `json:"mapping_id"`
	TokenID        int      `json:"token_id"`
	Name           string   `json:"name"`
	KeyPrefix      string   `json:"key_prefix"`
	Fingerprint    string   `json:"fingerprint"`
	Status         string   `json:"status"`
	StatusCode     int      `json:"status_code"`
	MappingStatus  string   `json:"mapping_status"`
	RemainQuota    int      `json:"remain_quota"`
	UsedQuota      int      `json:"used_quota"`
	UnlimitedQuota bool     `json:"unlimited_quota"`
	Group          string   `json:"group"`
	Models         []string `json:"models"`
	ExpiredTime    int64    `json:"expired_time"`
	EffectiveAt    int64    `json:"effective_at"`
	EndedAt        *int64   `json:"ended_at,omitempty"`
}

func buildTokenView(mapping CustomerToken, token model.Token) tokenView {
	digest := sha256.Sum256([]byte(token.Key))
	prefix := token.Key
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	models := make([]string, 0)
	if token.ModelLimitsEnabled {
		for _, name := range strings.Split(token.ModelLimits, ",") {
			if name = strings.TrimSpace(name); name != "" {
				models = append(models, name)
			}
		}
	}
	return tokenView{
		MappingID:      mapping.ID,
		TokenID:        token.Id,
		Name:           token.Name,
		KeyPrefix:      "sk-" + prefix,
		Fingerprint:    hex.EncodeToString(digest[:8]),
		Status:         tokenStatusName(token.Status),
		StatusCode:     token.Status,
		MappingStatus:  mapping.Status,
		RemainQuota:    token.RemainQuota,
		UsedQuota:      token.UsedQuota,
		UnlimitedQuota: token.UnlimitedQuota,
		Group:          token.Group,
		Models:         models,
		ExpiredTime:    token.ExpiredTime,
		EffectiveAt:    mapping.EffectiveAt,
		EndedAt:        mapping.EndedAt,
	}
}

func tokenStatusName(status int) string {
	switch status {
	case common.TokenStatusEnabled:
		return "active"
	case common.TokenStatusDisabled:
		return "disabled"
	case common.TokenStatusExpired:
		return "expired"
	case common.TokenStatusExhausted:
		return "exhausted"
	default:
		return "unknown"
	}
}

func normalizeModels(models []string) []string {
	set := make(map[string]struct{}, len(models))
	for _, name := range models {
		name = strings.TrimSpace(name)
		if name != "" {
			set[name] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for name := range set {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func normalizeTokenExpiry(input *tokenRequest) error {
	if input.ExpiredTime == nil && strings.TrimSpace(input.ExpiresAt) != "" {
		parsed, err := time.ParseInLocation("2006-01-02T15:04", input.ExpiresAt, time.Local)
		if err != nil {
			return errors.New("expires_at is invalid")
		}
		value := parsed.Unix()
		input.ExpiredTime = &value
	}
	if input.ExpiredTime != nil && *input.ExpiredTime != -1 && *input.ExpiredTime <= time.Now().Unix() {
		return errors.New("expired_time must be in the future or -1")
	}
	return nil
}

func (a *App) listCustomerTokens(c *gin.Context) {
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	customer, ok := a.customerForRequest(c, id)
	if !ok {
		return
	}
	var mappings []CustomerToken
	if err := a.db.Where("customer_id = ? AND reseller_id = ?", customer.ID, customer.ResellerID).Order("id DESC").Find(&mappings).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]tokenView, 0, len(mappings))
	for _, mapping := range mappings {
		token, err := a.managedToken(&mapping)
		if err == nil {
			views = append(views, buildTokenView(mapping, *token))
		}
	}
	limits := gin.H{
		"max_user_tokens":                operation_setting.GetMaxUserTokens(),
		"max_active_tokens_per_customer": 1,
		"unlimited_quota_allowed":        false,
	}
	var reseller Reseller
	if err := a.db.First(&reseller, customer.ResellerID).Error; err == nil {
		var tokenCount int64
		if err = a.db.Model(&model.Token{}).Where("user_id = ?", reseller.QuotaCarrierUserID).Count(&tokenCount).Error; err == nil {
			limits["carrier_token_count"] = tokenCount
			if maxTokens := operation_setting.GetMaxUserTokens(); maxTokens > 0 && tokenCount >= int64(maxTokens) {
				limits["allocation_block_reason"] = "quota carrier token limit reached"
			} else if maxTokens > 0 && tokenCount*100 >= int64(maxTokens)*80 {
				limits["allocation_warning"] = "quota carrier token usage reached 80% of MaxUserTokens"
			}
		}
		if err = a.verifyCarrierInventory(reseller); err != nil {
			limits["allocation_block_reason"] = err.Error()
		}
	}
	respondOK(c, gin.H{"items": views, "limits": limits})
}

func (a *App) createCustomerToken(c *gin.Context) {
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	customer, ok := a.customerForRequest(c, id)
	if !ok {
		return
	}
	if customer.Status != "active" {
		respondError(c, http.StatusConflict, "customer is not active")
		return
	}
	var reseller Reseller
	if err := a.db.Where("id = ? AND status = ?", customer.ResellerID, "active").First(&reseller).Error; err != nil {
		respondError(c, http.StatusConflict, "reseller is not active")
		return
	}
	var input tokenRequest
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Group = strings.TrimSpace(input.Group)
	input.Models = normalizeModels(input.Models)
	if err := normalizeTokenExpiry(&input); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if input.Name == "" || len(input.Name) > 64 || input.Group == "" {
		respondError(c, http.StatusBadRequest, "name and group are required")
		return
	}
	if err := a.verifyCarrierInventory(reseller); err != nil {
		respondError(c, http.StatusConflict, err.Error())
		return
	}
	identity := currentIdentity(c)
	now := time.Now().Unix()
	expiredTime := int64(-1)
	if input.ExpiredTime != nil {
		expiredTime = *input.ExpiredTime
	}
	key, err := common.GenerateKey()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to generate API key")
		return
	}
	token := model.Token{
		UserId:             reseller.QuotaCarrierUserID,
		Key:                key,
		Status:             common.TokenStatusEnabled,
		Name:               input.Name,
		CreatedTime:        now,
		AccessedTime:       0,
		ExpiredTime:        expiredTime,
		RemainQuota:        0,
		UnlimitedQuota:     false,
		ModelLimitsEnabled: len(input.Models) > 0,
		ModelLimits:        strings.Join(input.Models, ","),
		Group:              input.Group,
		CrossGroupRetry:    false,
	}
	mapping := CustomerToken{
		ResellerID:         customer.ResellerID,
		CustomerID:         customer.ID,
		QuotaCarrierUserID: reseller.QuotaCarrierUserID,
		Status:             "active",
		EffectiveAt:        now,
		CreatedByUserID:    identity.NewAPIUserID,
		CreatedAt:          now,
	}
	err = a.db.Transaction(func(tx *gorm.DB) error {
		var current Customer
		if err := tx.Where("id = ? AND reseller_id = ?", customer.ID, customer.ResellerID).First(&current).Error; err != nil {
			return err
		}
		if current.ActiveTokenMappingID != nil {
			return errors.New("customer already has an active or retiring token")
		}
		var tokenCount int64
		if err := tx.Model(&model.Token{}).Where("user_id = ?", reseller.QuotaCarrierUserID).Count(&tokenCount).Error; err != nil {
			return err
		}
		if maxTokens := operation_setting.GetMaxUserTokens(); maxTokens > 0 && tokenCount >= int64(maxTokens) {
			return errors.New("quota carrier token limit reached")
		}
		if err := tx.Create(&token).Error; err != nil {
			return err
		}
		mapping.NewAPITokenID = token.Id
		if err := tx.Create(&mapping).Error; err != nil {
			return err
		}
		result := tx.Model(&Customer{}).Where("id = ? AND reseller_id = ? AND active_token_mapping_id IS NULL", customer.ID, customer.ResellerID).Update("active_token_mapping_id", mapping.ID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("customer token was created concurrently")
		}
		return appendAudit(tx, c, &customer.ResellerID, "token.create", "token", strconv.Itoa(token.Id), gin.H{"customer_id": customer.ID, "name": token.Name, "group": token.Group, "models": input.Models})
	})
	if err != nil {
		respondError(c, http.StatusConflict, err.Error())
		return
	}
	respondCreated(c, gin.H{"token": buildTokenView(mapping, token), "api_key": "sk-" + key, "shown_once": true})
}

func (a *App) verifyCarrierInventory(reseller Reseller) error {
	var tokens []model.Token
	if err := a.db.Where("user_id = ?", reseller.QuotaCarrierUserID).Find(&tokens).Error; err != nil {
		return err
	}
	var mappings []CustomerToken
	if err := a.db.Where("reseller_id = ?", reseller.ID).Find(&mappings).Error; err != nil {
		return err
	}
	managed := make(map[int]struct{}, len(mappings))
	for _, mapping := range mappings {
		managed[mapping.NewAPITokenID] = struct{}{}
	}
	for _, token := range tokens {
		if token.UnlimitedQuota {
			return errors.New("quota carrier has an unlimited token; new allocation is blocked")
		}
		if _, ok := managed[token.Id]; !ok {
			return errors.New("quota carrier has an unmanaged token; new allocation is blocked")
		}
	}
	return nil
}

func (a *App) updateCustomerToken(c *gin.Context) {
	customerID, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	tokenID, ok := parsePositiveID(c, "token_id")
	if !ok {
		return
	}
	customer, ok := a.customerForRequest(c, customerID)
	if !ok {
		return
	}
	mapping, ok := a.tokenMappingForRequest(c, customer, tokenID)
	if !ok {
		return
	}
	if mapping.Status != "active" {
		respondError(c, http.StatusConflict, "only active token can be edited")
		return
	}
	token, err := a.managedToken(mapping)
	if err != nil {
		respondError(c, http.StatusNotFound, "token not found")
		return
	}
	var input tokenRequest
	if err = common.DecodeJson(c.Request.Body, &input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if err = normalizeTokenExpiry(&input); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	updates := map[string]any{}
	if name := strings.TrimSpace(input.Name); name != "" {
		updates["name"] = name
		token.Name = name
	}
	if group := strings.TrimSpace(input.Group); group != "" {
		updates["group"] = group
		token.Group = group
	}
	if input.Models != nil {
		input.Models = normalizeModels(input.Models)
		updates["model_limits_enabled"] = len(input.Models) > 0
		updates["model_limits"] = strings.Join(input.Models, ",")
		token.ModelLimitsEnabled = len(input.Models) > 0
		token.ModelLimits = strings.Join(input.Models, ",")
	}
	if input.ExpiredTime != nil {
		updates["expired_time"] = *input.ExpiredTime
		token.ExpiredTime = *input.ExpiredTime
	}
	if strings.TrimSpace(input.Status) != "" {
		status := 0
		switch strings.ToLower(strings.TrimSpace(input.Status)) {
		case "active", "enabled", "1":
			status = common.TokenStatusEnabled
		case "disabled", "2":
			status = common.TokenStatusDisabled
		}
		if status == 0 {
			respondError(c, http.StatusBadRequest, "status must be enabled or disabled")
			return
		}
		if status == common.TokenStatusEnabled {
			if customer.Status != "active" || token.RemainQuota <= 0 {
				respondError(c, http.StatusConflict, "active customer and positive token quota are required")
				return
			}
		}
		updates["status"] = status
		token.Status = status
	}
	if len(updates) == 0 {
		respondOK(c, buildTokenView(*mapping, *token))
		return
	}
	err = a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Token{}).Where("id = ? AND user_id = ?", token.Id, token.UserId).Updates(updates).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, &customer.ResellerID, "token.update", "token", strconv.Itoa(token.Id), updates)
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	if err = model.UpdateTokenCacheAfterExternalWrite(token.Key, *token, 0, false); err != nil {
		_ = model.InvalidateTokenCache(token.Key)
	}
	respondOK(c, buildTokenView(*mapping, *token))
}

func (a *App) retireCustomerToken(c *gin.Context) {
	customerID, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	tokenID, ok := parsePositiveID(c, "token_id")
	if !ok {
		return
	}
	customer, ok := a.customerForRequest(c, customerID)
	if !ok {
		return
	}
	mapping, ok := a.tokenMappingForRequest(c, customer, tokenID)
	if !ok {
		return
	}
	if mapping.Status == "retired" {
		respondOK(c, mapping)
		return
	}
	token, err := a.managedToken(mapping)
	if err != nil {
		respondError(c, http.StatusNotFound, "token not found")
		return
	}
	err = a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&CustomerToken{}).Where("id = ? AND status = ?", mapping.ID, "active").Update("status", "retiring").Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Token{}).Where("id = ?", token.Id).Update("status", common.TokenStatusDisabled).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, &customer.ResellerID, "token.retire", "token", strconv.Itoa(token.Id), gin.H{"customer_id": customer.ID})
	})
	if err != nil {
		respondError(c, http.StatusConflict, err.Error())
		return
	}
	mapping.Status = "retiring"
	token.Status = common.TokenStatusDisabled
	_ = model.InvalidateTokenCache(token.Key)
	respondOK(c, buildTokenView(*mapping, *token))
}
