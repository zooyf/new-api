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
	"github.com/QuantumNous/new-api/pkg/resellerhub"
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
		return fmt.Errorf("initialize main database: %w", err)
	}
	logDatabaseInitialized := false
	defer func() {
		if logDatabaseInitialized {
			if err := model.CloseDB(); err != nil {
				common.SysError("close Reseller Hub databases: " + err.Error())
			}
			return
		}
		sqlDB, err := model.DB.DB()
		if err != nil {
			common.SysError("close Reseller Hub database: " + err.Error())
			return
		}
		if err := sqlDB.Close(); err != nil {
			common.SysError("close Reseller Hub database: " + err.Error())
		}
	}()
	config := resellerhub.LoadConfig()
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "migrate":
		migrationTimeout := time.Duration(common.GetEnvOrDefault("RESELLER_HUB_MIGRATION_TIMEOUT_SECONDS", 60)) * time.Second
		migrationLockTimeout := time.Duration(common.GetEnvOrDefault("RESELLER_HUB_MIGRATION_LOCK_TIMEOUT_SECONDS", 10)) * time.Second
		migrationCtx, cancel := context.WithTimeout(context.Background(), migrationTimeout)
		defer cancel()
		if err := resellerhub.MigrateWithLockTimeout(model.DB.WithContext(migrationCtx), migrationLockTimeout); err != nil {
			return fmt.Errorf("migrate Reseller Hub tables: %w", err)
		}
		common.SysLog("Reseller Hub migration completed")
		return nil
	case "serve":
		if config.AutoMigrate {
			if err := resellerhub.Migrate(model.DB); err != nil {
				return fmt.Errorf("auto-migrate Reseller Hub tables: %w", err)
			}
		} else if err := resellerhub.VerifySchema(model.DB); err != nil {
			return fmt.Errorf("verify Reseller Hub schema: %w; run /reseller-hub migrate first", err)
		}
	default:
		return fmt.Errorf("unknown command %q; expected serve or migrate", command)
	}

	model.InitOptionMap()
	if err := model.InitLogDB(); err != nil {
		return fmt.Errorf("initialize log database: %w", err)
	}
	logDatabaseInitialized = true
	if err := common.InitRedisClient(); err != nil {
		return fmt.Errorf("initialize Redis: %w", err)
	}

	if _, err := strconv.Atoi(config.Port); err != nil {
		return fmt.Errorf("invalid RESELLER_HUB_PORT: %w", err)
	}
	if config.GatewayBaseURL == "" {
		return errors.New("RESELLER_HUB_GATEWAY_BASE_URL is required")
	}

	app := resellerhub.New(model.DB, model.LOG_DB, config)
	ctx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	app.StartBackground(ctx)
	go app.RunReconciler(ctx)

	server := &http.Server{
		Addr:              ":" + config.Port,
		Handler:           app.Router(),
		ReadHeaderTimeout: 30 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		common.SysLog("Reseller Hub started on port " + config.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-quit:
		common.SysLog(fmt.Sprintf("received signal %v, shutting down Reseller Hub", sig))
	case err := <-serverErrors:
		return fmt.Errorf("serve Reseller Hub: %w", err)
	}
	stopWorkers()
	shutdownTimeout := time.Duration(common.GetEnvOrDefault("SHUTDOWN_TIMEOUT_SECONDS", 120)) * time.Second
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown Reseller Hub: %w", err)
	}
	return nil
}
