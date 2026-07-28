package config

import (
	"fmt"
	"time"
)

const (
	environmentRetryMaxAttempts                = environmentPrefix + "RETRY_MAX_ATTEMPTS"
	environmentRetryInitialBackoff             = environmentPrefix + "RETRY_INITIAL_BACKOFF"
	environmentRetryMaxBackoff                 = environmentPrefix + "RETRY_MAX_BACKOFF"
	environmentInterruptedAttemptAuditInterval = environmentPrefix +
		"INTERRUPTED_ATTEMPT_AUDIT_INTERVAL"

	defaultRetryMaxAttempts                int16 = 3
	defaultRetryInitialBackoff                   = 250 * time.Millisecond
	defaultRetryMaxBackoff                       = 30 * time.Second
	defaultInterruptedAttemptAuditInterval       = 30 * time.Second
)

type FaultToleranceConfig struct {
	RetryMaxAttempts                int16         `env:"MILETOS_RUNTIME_RETRY_MAX_ATTEMPTS"`
	RetryInitialBackoff             time.Duration `env:"MILETOS_RUNTIME_RETRY_INITIAL_BACKOFF"`
	RetryMaxBackoff                 time.Duration `env:"MILETOS_RUNTIME_RETRY_MAX_BACKOFF"`
	InterruptedAttemptAuditInterval time.Duration `env:"MILETOS_RUNTIME_INTERRUPTED_ATTEMPT_AUDIT_INTERVAL"`
}

func (configuration FaultToleranceConfig) IsValid() bool {
	return validateFaultToleranceConfig(configuration) == nil
}

func faultToleranceConfigDefaults() FaultToleranceConfig {
	return FaultToleranceConfig{
		RetryMaxAttempts:                defaultRetryMaxAttempts,
		RetryInitialBackoff:             defaultRetryInitialBackoff,
		RetryMaxBackoff:                 defaultRetryMaxBackoff,
		InterruptedAttemptAuditInterval: defaultInterruptedAttemptAuditInterval,
	}
}

func validateFaultToleranceConfig(configuration FaultToleranceConfig) error {
	if configuration.RetryMaxAttempts < 1 {
		return fmt.Errorf(
			"%s must be greater than or equal to 1",
			environmentRetryMaxAttempts,
		)
	}
	if configuration.RetryInitialBackoff <= 0 {
		return fmt.Errorf(
			"%s must be greater than zero",
			environmentRetryInitialBackoff,
		)
	}
	if configuration.RetryMaxBackoff < configuration.RetryInitialBackoff {
		return fmt.Errorf(
			"%s must be greater than or equal to %s",
			environmentRetryMaxBackoff,
			environmentRetryInitialBackoff,
		)
	}
	if configuration.InterruptedAttemptAuditInterval <= 0 {
		return fmt.Errorf(
			"%s must be greater than zero",
			environmentInterruptedAttemptAuditInterval,
		)
	}
	return nil
}
