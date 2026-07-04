package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("reverse-newapi-volcengine 配置错误: %v", err)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           newServer(cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("reverse-newapi-volcengine started on :%s, upstream=%s", cfg.port, cfg.upstreamBaseURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("reverse-newapi-volcengine 启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("received signal: %v, shutting down...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server forced to shutdown: %v", err)
	}
}

type config struct {
	upstreamBaseURL string
	port            string
	timeout         time.Duration
}

func loadConfig() (config, error) {
	baseURL := strings.TrimSpace(os.Getenv("NEW_API_BASE_URL"))
	if baseURL == "" {
		return config{}, fmt.Errorf("NEW_API_BASE_URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return config{}, fmt.Errorf("NEW_API_BASE_URL is invalid: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return config{}, fmt.Errorf("NEW_API_BASE_URL must include scheme and host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "3001"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return config{}, fmt.Errorf("PORT must be a number: %w", err)
	}

	timeoutSeconds := 120
	if raw := strings.TrimSpace(os.Getenv("NEW_API_TIMEOUT_SECONDS")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			return config{}, fmt.Errorf("NEW_API_TIMEOUT_SECONDS must be a positive number")
		}
		timeoutSeconds = v
	}

	return config{
		upstreamBaseURL: parsed.String(),
		port:            port,
		timeout:         time.Duration(timeoutSeconds) * time.Second,
	}, nil
}
