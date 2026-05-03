package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/time/rate"

	apiv2 "github.com/yclenove/telegram-relay/internal/api/v2"
	"github.com/yclenove/telegram-relay/internal/config"
	relayhttp "github.com/yclenove/telegram-relay/internal/http"
	"github.com/yclenove/telegram-relay/internal/relay"
	"github.com/yclenove/telegram-relay/internal/repository/postgres"
	"github.com/yclenove/telegram-relay/internal/security"
	"github.com/yclenove/telegram-relay/internal/service"
	"github.com/yclenove/telegram-relay/internal/telegram"
)

// main 负责完成服务启动的整体编排：
// 1) 读取配置
// 2) 组装依赖
// 3) 启动 HTTP 服务
// 4) 处理优雅停机
// adminStaticNoStoreIndex 为入口 HTML 关闭强缓存，便于更新子路径 base 后客户端立即拿到新 index。
func adminStaticNoStoreIndex(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		if p == "/" || path.Base(p) == "index.html" {
			w.Header().Set("Cache-Control", "no-store, must-revalidate")
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	// 使用 JSON 日志，便于后续接入 Loki/ELK 或通过 grep 分析字段。
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// 本地开发常在仓库根放 .env（已在 .gitignore）；存在则注入进程环境变量，便于直接执行 go run。
	// 生产环境仍推荐由编排或系统注入环境变量，不依赖磁盘上的 .env 文件。
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(".env"); err != nil {
			logger.Error("load .env failed", "error", err)
			os.Exit(1)
		}
	}

	// 配置优先级：默认值 < 配置文件 < 环境变量（含 .env 注入项）。
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}

	telegramClient, err := telegram.NewClient(cfg.Telegram)
	if err != nil {
		logger.Error("init telegram client failed", "error", err)
		os.Exit(1)
	}
	relayService := relay.NewService(telegramClient, cfg.Retry)
	store, err := postgres.NewStore(context.Background(), cfg.Database)
	if err != nil {
		logger.Error("init postgres failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	if err := store.ApplyMigrations(context.Background(), filepath.Join(".", "migrations")); err != nil {
		logger.Error("apply migrations failed", "error", err)
		os.Exit(1)
	}
	bootstrapHash, err := service.HashPassword(cfg.Auth.BootstrapPassword)
	if err != nil {
		logger.Error("hash bootstrap password failed", "error", err)
		os.Exit(1)
	}
	if err := store.EnsureBootstrapData(context.Background(), cfg.Auth.BootstrapUsername, bootstrapHash, cfg.Auth.BootstrapPasswordSync); err != nil {
		logger.Error("bootstrap admin failed", "error", err)
		os.Exit(1)
	}
	if cfg.Auth.BootstrapPasswordSync {
		logger.Info("bootstrap password sync enabled: admin password hash updated from current BOOTSTRAP_PASSWORD")
	}
	notifySvc := service.NewNotifyService(store, cfg.Retry.MaxAttempts)
	authSvc := service.NewAuthService(store, cfg.Auth)
	worker := service.NewDispatchWorker(logger, store, cfg.Retry, cfg.Worker, cfg.Telegram)
	verifier := security.NewCompositeVerifier(cfg.Security, store)
	// 全局限流器：用于保护中转服务，防止上游突发请求把实例打挂。
	limiter := rate.NewLimiter(rate.Limit(cfg.Security.RateLimitPerSecond), cfg.Security.RateLimitBurst)

	mux := http.NewServeMux()
	relayHandler := relayhttp.NewHandler(logger, verifier, relayService, notifySvc, limiter)
	relayHandler.Register(mux)
	v2Handler := apiv2.NewHandler(logger, store, authSvc, notifySvc, verifier, limiter, apiv2.IntegrationHints{
		PublicBaseURL: cfg.Web.PublicBaseURL,
		SecurityLevel: cfg.Security.Level,
	})
	v2Handler.Register(mux)
	// 仅当配置了静态目录且路径存在时挂载管理台，避免前端独立仓库后镜像内无文件导致异常。
	if cfg.Web.AdminStaticDir != "" {
		if _, err := os.Stat(cfg.Web.AdminStaticDir); err == nil {
			// 避免同域部署升级后浏览器长期 304 缓存旧 index.html（仍引用根路径 /assets 导致白屏）。
			mux.Handle("/", adminStaticNoStoreIndex(http.FileServer(http.Dir(cfg.Web.AdminStaticDir))))
			logger.Info("admin static mounted", "dir", cfg.Web.AdminStaticDir)
		} else {
			logger.Warn("admin static dir missing, skip file server", "dir", cfg.Web.AdminStaticDir, "error", err)
		}
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go worker.Start(workerCtx)

	// 统一包一层访问日志：默认仅启动/停机有几条日志，中间请求不可见会让排障困难。
	httpHandler := relayhttp.AccessLog(logger, mux)

	server := &http.Server{
		Addr:         cfg.Server.ListenAddr,
		Handler:      httpHandler,
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
