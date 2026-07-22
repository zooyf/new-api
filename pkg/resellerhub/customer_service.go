package resellerhub

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type customerRequest struct {
	ResellerID  int    `json:"reseller_id"`
	DisplayName string `json:"display_name"`
	ExternalRef string `json:"external_ref"`
	DiscountBPS *int   `json:"discount_bps"`
	Status      string `json:"status"`
}

type discountRequest struct {
	DiscountBPS *int   `json:"discount_bps"`
	Reason      string `json:"reason"`
}

func validCustomerStatus(status string) bool {
	return status == "active" || status == "suspended" || status == "closed"
}

func (a *App) listCustomers(c *gin.Context) {
	resellerID, ok := a.scopedResellerID(c)
	if !ok {
		return
	}
	query := a.db.Model(&Customer{})
	if resellerID > 0 {
		query = query.Where("reseller_id = ?", resellerID)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	search := strings.TrimSpace(c.Query("search"))
	if search == "" {
		search = strings.TrimSpace(c.Query("q"))
	}
	if search != "" {
		query = query.Where("display_name LIKE ? OR external_ref = ?", "%"+search+"%", search)
	}
	page := queryPositiveInt(c, "page", 1, 1000000)
	pageSize := queryPositiveInt(c, "page_size", 50, 200)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	var rows []Customer
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]gin.H, 0, len(rows))
	for i := range rows {
		row := rows[i]
		discount, source, _ := a.effectiveDiscount(&row)
		item := gin.H{
			"id": row.ID, "reseller_id": row.ResellerID, "active_token_mapping_id": row.ActiveTokenMappingID,
			"display_name": row.DisplayName, "external_ref": row.ExternalRef, "discount_bps": row.DiscountBPS,
			"effective_discount_bps": discount, "discount_source": source, "status": row.Status,
			"created_at": row.CreatedAt, "updated_at": row.UpdatedAt,
		}
		var reseller Reseller
		if err := a.db.Select("id", "name", "code").First(&reseller, row.ResellerID).Error; err == nil {
			item["reseller_name"] = reseller.Name
			item["reseller_code"] = reseller.Code
		}
		if row.ActiveTokenMappingID != nil {
			var mapping CustomerToken
			if err := a.db.First(&mapping, *row.ActiveTokenMappingID).Error; err == nil {
				if token, err := a.managedToken(&mapping); err == nil {
					item["token_id"] = token.Id
					item["token_name"] = token.Name
					item["token_status"] = tokenStatusName(token.Status)
					item["token_remain_quota"] = token.RemainQuota
					item["remain_quota"] = token.RemainQuota
					item["negative_key_count"] = boolInt(token.RemainQuota < 0)
					item["active_key_count"] = boolInt(mapping.Status == "active")
				}
			}
		}
		items = append(items, item)
	}
	respondOK(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (a *App) createCustomer(c *gin.Context) {
	identity := currentIdentity(c)
	var input customerRequest
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	resellerID := identity.ResellerID
	if identity.HubRole == HubRoleSuperAdmin {
		resellerID = input.ResellerID
	}
	if resellerID <= 0 {
		respondError(c, http.StatusBadRequest, "reseller_id is required")
		return
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.ExternalRef = strings.TrimSpace(input.ExternalRef)
	if input.Status == "" {
		input.Status = "active"
	}
	if input.DisplayName == "" || len(input.DisplayName) > 128 || len(input.ExternalRef) > 128 || !validCustomerStatus(input.Status) {
		respondError(c, http.StatusBadRequest, "invalid customer fields")
		return
	}
	if input.DiscountBPS != nil {
		if err := a.validateDiscount(*input.DiscountBPS); err != nil {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	var reseller Reseller
	if err := a.db.Where("id = ? AND status = ?", resellerID, "active").First(&reseller).Error; err != nil {
		respondError(c, http.StatusBadRequest, "reseller not found or inactive")
		return
	}
	now := time.Now().Unix()
	row := Customer{
		ResellerID:      resellerID,
		DisplayName:     input.DisplayName,
		ExternalRef:     input.ExternalRef,
		DiscountBPS:     input.DiscountBPS,
		Status:          input.Status,
		CreatedByUserID: identity.NewAPIUserID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if input.ExternalRef != "" {
			var count int64
			if err := tx.Model(&Customer{}).Where("reseller_id = ? AND external_ref = ?", resellerID, input.ExternalRef).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("external_ref already exists")
			}
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if row.DiscountBPS != nil {
			version := DiscountVersion{ResellerID: resellerID, CustomerID: row.ID, DiscountBPS: *row.DiscountBPS, EffectiveAt: now, Reason: "initial customer discount", CreatedByUserID: identity.NewAPIUserID, CreatedAt: now}
			if err := tx.Create(&version).Error; err != nil {
				return err
			}
		}
		return appendAudit(tx, c, &resellerID, "customer.create", "customer", strconv.Itoa(row.ID), gin.H{"display_name": row.DisplayName, "external_ref": row.ExternalRef})
	})
	if err != nil {
		respondError(c, http.StatusConflict, err.Error())
		return
	}
	respondCreated(c, row)
}

func (a *App) getCustomer(c *gin.Context) {
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	row, ok := a.customerForRequest(c, id)
	if !ok {
		return
	}
	discount, source, err := a.effectiveDiscount(row)
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	var tokens []CustomerToken
	_ = a.db.Where("customer_id = ? AND reseller_id = ?", row.ID, row.ResellerID).Order("id DESC").Find(&tokens).Error
	var versions []DiscountVersion
	_ = a.db.Where("reseller_id = ? AND customer_id IN ?", row.ResellerID, []int{0, row.ID}).Order("effective_at DESC").Find(&versions).Error
	respondOK(c, gin.H{"customer": row, "effective_discount_bps": discount, "discount_source": source, "tokens": tokens, "discount_history": versions})
}

func (a *App) updateCustomer(c *gin.Context) {
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	row, ok := a.customerForRequest(c, id)
	if !ok {
		return
	}
	var input customerRequest
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if row.Status == "closed" && input.Status != "" && input.Status != "closed" {
		respondError(c, http.StatusConflict, "closed customer cannot be reopened")
		return
	}
	updates := map[string]any{"updated_at": time.Now().Unix()}
	if name := strings.TrimSpace(input.DisplayName); name != "" {
		updates["display_name"] = name
	}
	if input.ExternalRef != "" {
		updates["external_ref"] = strings.TrimSpace(input.ExternalRef)
	}
	if input.Status != "" {
		if !validCustomerStatus(input.Status) {
			respondError(c, http.StatusBadRequest, "invalid status")
			return
		}
		updates["status"] = input.Status
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Customer{}).Where("id = ? AND reseller_id = ?", row.ID, row.ResellerID).Updates(updates).Error; err != nil {
			return err
		}
		if status, _ := updates["status"].(string); (status == "suspended" || status == "closed") && row.ActiveTokenMappingID != nil {
			var mapping CustomerToken
			if err := tx.First(&mapping, *row.ActiveTokenMappingID).Error; err == nil {
				if err = tx.Model(&model.Token{}).Where("id = ?", mapping.NewAPITokenID).Update("status", common.TokenStatusDisabled).Error; err != nil {
					return err
				}
			}
		}
		return appendAudit(tx, c, &row.ResellerID, "customer.update", "customer", strconv.Itoa(row.ID), updates)
	})
	if err != nil {
		respondError(c, http.StatusConflict, err.Error())
		return
	}
	if input.Status == "suspended" || input.Status == "closed" {
		a.invalidateCustomerToken(row)
	}
	_ = a.db.First(row, row.ID).Error
	respondOK(c, row)
}

func (a *App) createDiscount(c *gin.Context) {
	id, ok := parsePositiveID(c, "id")
	if !ok {
		return
	}
	customer, ok := a.customerForRequest(c, id)
	if !ok {
		return
	}
	var input discountRequest
	if err := common.DecodeJson(c.Request.Body, &input); err != nil || strings.TrimSpace(input.Reason) == "" {
		respondError(c, http.StatusBadRequest, "discount and reason are required")
		return
	}
	if input.DiscountBPS != nil {
		if err := a.validateDiscount(*input.DiscountBPS); err != nil {
			respondError(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	identity := currentIdentity(c)
	now := time.Now().Unix()
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&DiscountVersion{}).Where("customer_id = ? AND ended_at IS NULL", customer.ID).Update("ended_at", now).Error; err != nil {
			return err
		}
		if input.DiscountBPS != nil {
			version := DiscountVersion{ResellerID: customer.ResellerID, CustomerID: customer.ID, DiscountBPS: *input.DiscountBPS, EffectiveAt: now, Reason: strings.TrimSpace(input.Reason), CreatedByUserID: identity.NewAPIUserID, CreatedAt: now}
			if err := tx.Create(&version).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&Customer{}).Where("id = ?", customer.ID).Updates(map[string]any{"discount_bps": input.DiscountBPS, "updated_at": now}).Error; err != nil {
			return err
		}
		return appendAudit(tx, c, &customer.ResellerID, "discount.change", "customer", strconv.Itoa(customer.ID), gin.H{"discount_bps": input.DiscountBPS, "reason": input.Reason})
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	customer.DiscountBPS = input.DiscountBPS
	discount, source, _ := a.effectiveDiscount(customer)
	respondOK(c, gin.H{"discount_bps": discount, "source": source, "effective_at": now})
}

func (a *App) effectiveDiscount(customer *Customer) (int, string, error) {
	if customer.DiscountBPS != nil {
		return *customer.DiscountBPS, "customer", nil
	}
	var reseller Reseller
	if err := a.db.First(&reseller, customer.ResellerID).Error; err != nil {
		return 0, "", err
	}
	return reseller.DefaultDiscountBPS, "reseller", nil
}

func (a *App) invalidateCustomerToken(customer *Customer) {
	if customer.ActiveTokenMappingID == nil {
		return
	}
	var mapping CustomerToken
	if err := a.db.First(&mapping, *customer.ActiveTokenMappingID).Error; err != nil {
		return
	}
	if token, err := a.managedToken(&mapping); err == nil {
		_ = model.InvalidateTokenCache(token.Key)
	}
}
