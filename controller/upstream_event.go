package controller

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetUpstreamEventStats(c *gin.Context) {
	stats, err := model.GetUpstreamEventOutboxStats()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

func ListUpstreamEvents(c *gin.Context) {
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	status := c.Query("status")
	eventType := c.Query("event_type")

	events, total, err := model.ListUpstreamEventOutbox(status, eventType, offset, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":  events,
		"total":  total,
		"offset": offset,
		"limit":  limit,
	})
}

func GetUpstreamEvent(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid upstream event id: %w", err))
		return
	}
	event, err := model.GetUpstreamEventOutbox(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, event)
}

func RetryUpstreamEvent(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid upstream event id: %w", err))
		return
	}
	if err := model.RetryUpstreamEventOutbox(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
