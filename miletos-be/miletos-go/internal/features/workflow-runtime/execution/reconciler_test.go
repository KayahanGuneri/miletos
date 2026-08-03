package execution_test

import (
	"strings"
	"testing"
	"time"

	execution "miletos-go/internal/features/workflow-runtime/execution"
)

func TestNewReconcilerOwnsDurationInvariants(t *testing.T) {
	tests := []struct {
		name         string
		interval     time.Duration
		queued       time.Duration
		running      time.Duration
		retryPending time.Duration
		want         string
	}{
		{name: "interval", queued: time.Second, running: time.Second, retryPending: time.Second, want: "interval"},
		{name: "queued", interval: time.Second, running: time.Second, retryPending: time.Second, want: "queued-stale"},
		{name: "running", interval: time.Second, queued: time.Second, retryPending: time.Second, want: "running-stale"},
		{name: "retry pending", interval: time.Second, queued: time.Second, running: time.Second, want: "retry-pending-stale"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reconciler, err := execution.NewReconciler(
				nil, nil, nil, nil, nil, "commands", test.interval,
				test.queued, test.running, test.retryPending,
			)
			if err == nil || reconciler != nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewReconciler() = (%#v, %v), want %q", reconciler, err, test.want)
			}
		})
	}
}

func TestNewReconcilerAcceptsValidDurations(t *testing.T) {
	reconciler, err := execution.NewReconciler(
		nil, nil, nil, nil, nil, "commands",
		time.Second, 2*time.Second, 3*time.Second, 4*time.Second,
	)
	if err != nil {
		t.Fatalf("NewReconciler() error = %v", err)
	}
	if reconciler.Interval() != time.Second {
		t.Fatalf("Interval() = %v", reconciler.Interval())
	}
}
