package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/yclenove/telegram-relay/internal/config"
	"github.com/yclenove/telegram-relay/internal/model"
	"github.com/yclenove/telegram-relay/internal/repository/postgres"
	relaylegacy "github.com/yclenove/telegram-relay/internal/relay"
	"github.com/yclenove/telegram-relay/internal/telegram"
)

// DispatchWorker 周期扫描待发送任务并投递到 Telegram。
type DispatchWorker struct {
	logger       *slog.Logger
	store        *postgres.Store
	retryCfg     config.RetryConfig
	workerCfg    config.WorkerConfig
	telegramBase config.TelegramConfig
}

// NewDispatchWorker 创建 worker；telegramBase 提供与主进程一致的 Bot API 基址与超时，避免与全局配置分叉。
func NewDispatchWorker(logger *slog.Logger, store *postgres.Store, retryCfg config.RetryConfig, workerCfg config.WorkerConfig, telegramBase config.TelegramConfig) *DispatchWorker {
	return &DispatchWorker{
		logger:       logger,
		store:        store,
		retryCfg:     retryCfg,
		workerCfg:    workerCfg,
		telegramBase: telegramBase,
	}
}

func (w *DispatchWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(w.workerCfg.PollIntervalMS) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("dispatch worker stopped")
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *DispatchWorker) runOnce(ctx context.Context) {
	jobs, err := w.store.NextPendingJobs(ctx, w.workerCfg.BatchSize)
	if err != nil {
		w.logger.Error("load pending jobs failed", "error", err)
		return
	}
	for _, job := range jobs {
		w.handleJob(ctx, job.ID)
	}
}

func (w *DispatchWorker) handleJob(ctx context.Context, jobID int64) {
	job, event, destination, bot, err := w.store.LoadDispatchContext(ctx, jobID)
	if err != nil {
		w.logger.Error("load dispatch context failed", "job_id", jobID, "error", err)
		return
	}
	client := telegram.NewClient(config.TelegramConfig{
		BotToken:   postgres.DecryptSecret(bot.BotTokenEnc),
		ChatID:     destination.ChatID,
		ParseMode:  destination.ParseMode,
		APIBaseURL: w.telegramBase.APIBaseURL,
		TimeoutSec: w.telegramBase.TimeoutSec,
	})
	relaySvc := relaylegacy.NewService(client, w.retryCfg)
	err = relaySvc.Send(ctx, model.NotifyRequest{
		Title:   event.Title,
		Message: event.Message,
		Level:   event.Level,
		Source:  event.Source,
		EventID: event.EventID,
	})
	if err == nil {
		if err = w.store.MarkJobSuccess(ctx, job.ID, event.ID); err != nil {
			w.logger.Error("mark job success failed", "job_id", job.ID, "error", err)
		}
		return
	}
	backoff := time.Duration(w.retryCfg.InitialBackoffMS) * time.Millisecond
	if err2 := w.store.MarkJobFailedOrRetry(ctx, job, event.ID, err.Error(), backoff); err2 != nil {
		w.logger.Error("mark job failed/retry failed", "job_id", job.ID, "error", err2)
	}
	w.logger.Error("dispatch job failed", "job_id", job.ID, "event_id", event.EventID, "error", fmt.Sprintf("%v", err))
}
