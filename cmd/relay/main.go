package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/time/rate"

	"telegram-notification/internal/config"
	relayhttp "telegram-notification/internal/http"
	"telegram-notification/internal/relay"
	"telegram-notification/internal/security"
	"telegram-notification/internal/telegram"
)

// main 负责完成服务启动的整体编排：
// 1) 读取配置
// 2) 组装依赖
// 3) 启动 HTTP 服务
// 4) 处理优雅停机
func main() {
	// 使用 JSON 日志，便于后续接入 Loki/ELK 或通过 grep 分析字段。
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// 配置优先级：默认值 < 配置文件 < 环境变量。
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}

	telegramClient := telegram.NewClient(cfg.Telegram)
	relayService := relay.NewService(telegramClient, cfg.Retry)
	verifier := security.NewVerifier(cfg.Security)
	// 全局限流器：用于保护中转服务，防止上游突发请求把实例打挂。
	limiter := rate.NewLimiter(rate.Limit(cfg.Security.RateLimitPerSecond), cfg.Security.RateLimitBurst)

	mux := http.NewServeMux()
	handler := relayhttp.NewHandler(logger, verifier, relayService, limiter)
	handler.Register(mux)

	server := &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 监听放到 goroutine，主协程用于等待退出信号并执行优雅停机。
	go func() {
		logger.Info("relay server started", "listen_addr", cfg.Server.ListenAddr, "security_level", cfg.Security.Level)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	waitForShutdown(logger, server)
}

// waitForShutdown 等待系统信号并触发优雅停机，
// 避免请求处理中途被强制中断。
func waitForShutdown(logger *slog.Logger, server *http.Server) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logger.Info("shutdown signal received")
	if err := server.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}
