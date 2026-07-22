package resellerhub

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func requestID(c *gin.Context) string {
	value := idempotencyKey(c)
	if value == "" {
		value = strings.TrimSpace(c.GetHeader("X-Request-Id"))
	}
	if value == "" {
		value = common.NewRequestId()
	}
	return value
}

func appendAudit(tx *gorm.DB, c *gin.Context, resellerID *int, action, objectType, objectID string, detail any) error {
	detailJSON := "{}"
	if detail != nil {
		encoded, err := common.Marshal(detail)
		if err != nil {
			return err
		}
		detailJSON = string(encoded)
	}
	actorID := 0
	if identity := currentIdentity(c); identity != nil {
		actorID = identity.NewAPIUserID
	}
	return tx.Create(&AuditLog{
		EventID:     newEventID(),
		ResellerID:  resellerID,
		ActorUserID: actorID,
		Action:      action,
		ObjectType:  objectType,
		ObjectID:    objectID,
		RequestID:   requestID(c),
		SourceIP:    c.ClientIP(),
		UserAgent:   c.Request.UserAgent(),
		BeforeJSON:  "{}",
		AfterJSON:   "{}",
		DetailJSON:  detailJSON,
		CreatedAt:   time.Now().Unix(),
	}).Error
}

func (a *App) listAuditLogs(c *gin.Context) {
	identity := currentIdentity(c)
	query := a.db.Model(&AuditLog{})
	if identity.HubRole != HubRoleSuperAdmin {
		query = query.Where("reseller_id = ?", identity.ResellerID)
	} else if resellerID, ok := a.scopedResellerID(c); !ok {
		return
	} else if resellerID > 0 {
		query = query.Where("reseller_id = ?", resellerID)
	}
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		query = query.Where("action = ?", action)
	}
	if objectType := strings.TrimSpace(c.Query("object_type")); objectType != "" {
		query = query.Where("object_type = ?", objectType)
	}
	if actorID, err := strconv.Atoi(c.Query("actor_user_id")); err == nil && actorID > 0 {
		query = query.Where("actor_user_id = ?", actorID)
	}
	page := queryPositiveInt(c, "page", 1, 1000000)
	pageSize := queryPositiveInt(c, "page_size", 50, 200)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	var rows []AuditLog
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(c, gin.H{"items": rows, "total": total, "page": page, "page_size": pageSize})
}
