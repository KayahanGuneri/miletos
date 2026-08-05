package crontrigger

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type Scheduler struct {
	service  *Service
	interval time.Duration
	batch    int
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewScheduler(service *Service, interval time.Duration, batch int) (*Scheduler, error) {
	if interval <= 0 || batch <= 0 || batch > 1000 {
		return nil, ErrSchedulerUnavailable
	}
	return &Scheduler{service: service, interval: interval, batch: batch}, nil
}

func (scheduler *Scheduler) Start(ctx context.Context) {
	if scheduler.cancel != nil {
		return
	}
	workerContext, cancel := context.WithCancel(ctx)
	scheduler.cancel = cancel
	scheduler.done = make(chan struct{})
	go func() {
		defer close(scheduler.done)
		scheduler.poll(workerContext)
		ticker := time.NewTicker(scheduler.interval)
		defer ticker.Stop()
		for {
			select {
			case <-workerContext.Done():
				return
			case <-ticker.C:
				scheduler.poll(workerContext)
			}
		}
	}()
}

func (scheduler *Scheduler) Stop() {
	if scheduler.cancel == nil {
		return
	}
	scheduler.cancel()
	<-scheduler.done
	scheduler.cancel = nil
}

func (scheduler *Scheduler) poll(ctx context.Context) {
	now := time.Now().UTC()
	if _, err := scheduler.service.triggers.ClaimDue(ctx, now, scheduler.batch); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("claim due cron triggers", "error", err)
		return
	}
	occurrences, err := scheduler.service.triggers.ClaimOccurrences(ctx, now, scheduler.batch)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Error("claim cron occurrences", "error", err)
		}
		return
	}
	for _, occurrence := range occurrences {
		if ctx.Err() != nil {
			return
		}
		if err := scheduler.fire(ctx, occurrence); err != nil {
			slog.Error("fire cron occurrence", "trigger_id", occurrence.TriggerID, "occurrence_id", occurrence.ID, "error", err)
		}
	}
}

func (scheduler *Scheduler) fire(ctx context.Context, occurrence Occurrence) error {
	binding, err := scheduler.service.triggers.FindByID(ctx, occurrence.CompanyID, occurrence.TriggerID)
	if err != nil {
		return scheduler.fail(ctx, occurrence, "BINDING_UNAVAILABLE")
	}
	if binding.Status != StatusActive {
		return scheduler.service.triggers.MarkOccurrenceSucceeded(ctx, occurrence.ID, "")
	}
	outcome, err := scheduler.service.Fire(ctx, DueOccurrence{
		Binding: binding, OccurrenceID: occurrence.ID, ScheduledAt: occurrence.ScheduledAt, FiredAt: time.Now().UTC(),
	})
	if err != nil {
		return scheduler.fail(ctx, occurrence, "EXECUTION_UNAVAILABLE")
	}
	return scheduler.service.triggers.MarkOccurrenceSucceeded(ctx, occurrence.ID, outcome.Execution.ID)
}

func (scheduler *Scheduler) fail(ctx context.Context, occurrence Occurrence, code string) error {
	return scheduler.service.triggers.MarkOccurrenceFailed(ctx, occurrence.ID, occurrence.AttemptCount, code, "cron execution could not be created")
}
