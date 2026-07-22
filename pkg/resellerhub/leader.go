package resellerhub

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const writerLeaseName = "quota-writer"

func (a *App) StartBackground(ctx context.Context) {
	if a.config.DisableBackgroundWorkers {
		a.isLeader.Store(true)
		return
	}
	go a.runLeader(ctx)
}

func (a *App) runLeader(ctx context.Context) {
	interval := a.config.LeaderLeaseDuration / 3
	if interval < time.Second {
		interval = time.Second
	}
	a.refreshLeadership(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			a.isLeader.Store(false)
			return
		case <-ticker.C:
			a.refreshLeadership(ctx)
		}
	}
}

func (a *App) refreshLeadership(ctx context.Context) {
	now := time.Now().Unix()
	expiresAt := time.Now().Add(a.config.LeaderLeaseDuration).Unix()
	acquired := false
	err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lease := Lease{Name: writerLeaseName, HolderID: a.config.InstanceID, ExpiresAt: expiresAt, UpdatedAt: now}
		if err := tx.Where("name = ?", writerLeaseName).FirstOrCreate(&lease).Error; err != nil {
			return err
		}
		result := tx.Model(&Lease{}).
			Where("name = ? AND (holder_id = ? OR expires_at < ?)", writerLeaseName, a.config.InstanceID, now).
			Updates(map[string]any{"holder_id": a.config.InstanceID, "expires_at": expiresAt, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		acquired = result.RowsAffected == 1
		return nil
	})
	if err != nil {
		common.SysError("reseller hub leader lease failed: " + err.Error())
		acquired = false
	}
	wasLeader := a.isLeader.Swap(acquired)
	if acquired && !wasLeader {
		common.SysLog("reseller hub instance acquired write leadership")
		go a.reconcileOnce(context.Background())
	}
}

func (a *App) RunReconciler(ctx context.Context) {
	if a.config.DisableBackgroundWorkers {
		return
	}
	interval := a.config.ReconcileInterval
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.isLeader.Load() {
				a.reconcileOnce(ctx)
			}
		}
	}
}
