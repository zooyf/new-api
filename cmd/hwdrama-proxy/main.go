package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/hwdramaproxy"
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
	common.RedisEnabled = false
	logger.SetupLogger()

	if err := model.InitDB(); err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	model.LOG_DB = model.DB
	defer func() {
		if err := model.CloseDB(); err != nil {
			common.SysError(fmt.Sprintf("failed to close database: %v", err))
		}
	}()

	port := strings.TrimSpace(os.Getenv("HWD_PROXY_PORT"))
	if port == "" {
		port = "3001"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("invalid HWD_PROXY_PORT: %w", err)
	}

	timeoutSeconds := common.GetEnvOrDefault("HWD_PROXY_REQUEST_TIMEOUT_SECONDS", 600)
	proxy, err := hwdramaproxy.New(hwdramaproxy.Config{
		UpstreamBaseURL: strings.TrimSpace(os.Getenv("HWD_PROXY_UPSTREAM_BASE_URL")),
		UpstreamAPIKey:  strings.TrimSpace(os.Getenv("HWD_PROXY_UPSTREAM_API_KEY")),
		Timeout:         time.Duration(timeoutSeconds) * time.Second,
	})
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("/", proxy)

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() {
		common.SysLog("hwdrama proxy started on port " + port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			common.FatalLog("failed to start hwdrama proxy: " + err.Error())
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	common.SysLog(fmt.Sprintf("received signal: %v, shutting down hwdrama proxy...", sig))

	shutdownTimeout := time.Duration(common.GetEnvOrDefault("SHUTDOWN_TIMEOUT_SECONDS", 120)) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown hwdrama proxy: %w", err)
	}
	common.SysLog("hwdrama proxy exited")
	return nil
}
