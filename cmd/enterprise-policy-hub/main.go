package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/enterprisepolicyhub"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		common.FatalLog(err.Error())
	}
}

func run() error {
	_ = godotenv.Load(".env")
	common.InitEnv()
	common.IsMasterNode = false
	logger.SetupLogger()

	if err := model.InitDB(); err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	if err := model.InitLogDB(); err != nil {
		return fmt.Errorf("initialize log database: %w", err)
	}
	if err := common.InitRedisClient(); err != nil {
		return fmt.Errorf("initialize Redis: %w", err)
	}
	defer func() {
		if err := model.CloseDB(); err != nil {
			common.SysError(fmt.Sprintf("failed to close database: %v", err))
		}
	}()

	if err := enterprisepolicyhub.Migrate(model.DB); err != nil {
		return fmt.Errorf("migrate Enterprise Policy Hub tables: %w", err)
	}

	config := enterprisepolicyhub.LoadConfig()
	if _, err := strconv.Atoi(config.Port); err != nil {
		return fmt.Errorf("invalid EPH_PORT: %w", err)
	}
	if _, err := time.LoadLocation(config.BudgetTimezone); err != nil {
		return fmt.Errorf("invalid EPH_BUDGET_TIMEZONE: %w", err)
	}
	app := enterprisepolicyhub.New(model.DB, model.DB, model.LOG_DB, config)
	server := &http.Server{
		Addr:              ":" + config.Port,
		Handler:           app.Router(),
		ReadHeaderTimeout: 30 * time.Second,
	}

	ctx, stopSync := context.WithCancel(context.Background())
	defer stopSync()
	if config.EnableBackgroundSync {
		go runBackgroundUsageSync(ctx, app, config.LogSyncInterval)
	}

	go func() {
		common.SysLog("enterprise policy hub started on port " + config.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			common.FatalLog("failed to start enterprise policy hub: " + err.Error())
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	common.SysLog(fmt.Sprintf("received signal: %v, shutting down enterprise policy hub...", sig))
	stopSync()

	shutdownTimeout := time.Duration(common.GetEnvOrDefault("SHUTDOWN_TIMEOUT_SECONDS", 120)) * time.Second
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown enterprise policy hub: %w", err)
	}
	common.SysLog("enterprise policy hub exited")
	return nil
}

func runBackgroundUsageSync(ctx context.Context, app *enterprisepolicyhub.App, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if _, err := app.SyncUsage(1000); err != nil {
		common.SysError("enterprise policy hub initial usage sync failed: " + err.Error())
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := app.SyncUsage(1000)
			if err != nil {
				common.SysError("enterprise policy hub usage sync failed: " + err.Error())
				continue
			}
			if result.ScannedLogs > 0 || result.ImportedLedgers > 0 || result.DisabledKeyCount > 0 {
				common.SysLog(fmt.Sprintf("enterprise policy hub usage sync: scanned=%d imported=%d skipped=%d disabled=%d last_log_id=%d",
					result.ScannedLogs,
					result.ImportedLedgers,
					result.SkippedLogs,
					result.DisabledKeyCount,
					result.LastLogID,
				))
			}
		}
	}
}
