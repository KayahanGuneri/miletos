package dataarrival

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

const pollInterval = 15 * time.Second

type Scheduler struct {
	service *Service
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewScheduler(service *Service) *Scheduler {
	return &Scheduler{service: service}
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
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-workerContext.Done():
				return
			case <-scheduler.service.wake:
				scheduler.poll(workerContext)
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
	if err := scheduler.service.Poll(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("poll data-arrival sources", "error", err)
	}
}
