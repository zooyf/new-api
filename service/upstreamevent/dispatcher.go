package upstreamevent

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func runDispatcher(cfg Config) {
	interval := cfg.DispatcherInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		dispatchOnce(cfg)
		<-ticker.C
	}
}

func dispatchOnce(cfg Config) {
	if cfg.WebhookURL == "" {
		return
	}
	now := time.Now().Unix()
	events, err := model.LeaseUpstreamEventOutbox(cfg.DispatcherBatchSize, now)
	if err != nil {
		common.SysLog("failed to lease upstream events: " + err.Error())
		return
	}
	if len(events) == 0 {
		return
	}

	payloadEvents := make([]ProviderEvent, 0, len(events))
	rowsByPayloadIndex := make([]model.UpstreamEventOutbox, 0, len(events))
	for _, row := range events {
		var event ProviderEvent
		if err := common.Unmarshal([]byte(row.Payload), &event); err != nil {
			_ = model.MarkUpstreamEventOutboxDead(row.ID, "invalid payload: "+err.Error())
			continue
		}
		payloadEvents = append(payloadEvents, event)
		rowsByPayloadIndex = append(rowsByPayloadIndex, row)
	}
	if len(payloadEvents) == 0 {
		return
	}

	body, err := common.Marshal(bulkRequest{Events: payloadEvents})
	if err != nil {
		markDispatchFailure(rowsByPayloadIndex, cfg, "marshal dispatch payload: "+err.Error())
		return
	}
	statusCode, responsePreview, err := postEvents(cfg, body)
	if err != nil {
		markDispatchFailure(rowsByPayloadIndex, cfg, err.Error())
		return
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		markDispatchFailure(rowsByPayloadIndex, cfg, fmt.Sprintf("webhook status %d: %s", statusCode, responsePreview))
		return
	}
	deliveredAt := time.Now().Unix()
	for _, row := range rowsByPayloadIndex {
		if err := model.MarkUpstreamEventOutboxDelivered(row.ID, deliveredAt); err != nil {
			common.SysLog("failed to mark upstream event delivered: " + err.Error())
		}
	}
}

func postEvents(cfg Config, body []byte) (int, string, error) {
	timeout := cfg.DispatcherRequestTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	req, err := http.NewRequest(http.MethodPost, cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.WebhookSecret != "" {
		timestamp := fmt.Sprint(time.Now().Unix())
		mac := hmac.New(sha256.New, []byte(cfg.WebhookSecret))
		mac.Write([]byte(timestamp))
		mac.Write([]byte("."))
		mac.Write(body)
		req.Header.Set("X-New-Api-Event-Timestamp", timestamp)
		req.Header.Set("X-New-Api-Event-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(respBody), nil
}

func markDispatchFailure(rows []model.UpstreamEventOutbox, cfg Config, message string) {
	for _, row := range rows {
		if row.DeliveryAttempts >= cfg.DispatcherMaxRetry {
			if err := model.MarkUpstreamEventOutboxDead(row.ID, localPreview(message, 2048)); err != nil {
				common.SysLog("failed to mark upstream event dead: " + err.Error())
			}
			continue
		}
		nextRetryAt := time.Now().Add(dispatchBackoff(row.DeliveryAttempts)).Unix()
		if err := model.MarkUpstreamEventOutboxRetrying(row.ID, nextRetryAt, localPreview(message, 2048)); err != nil {
			common.SysLog("failed to mark upstream event retrying: " + err.Error())
		}
	}
}

func dispatchBackoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	seconds := 1 << min(attempts-1, 10)
	return time.Duration(seconds) * time.Second
}
