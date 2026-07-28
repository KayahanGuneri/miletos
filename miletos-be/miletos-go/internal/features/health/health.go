package healthfeature

import "context"

const (
	StatusReady         = "READY"
	StatusNotReady      = "NOT_READY"
	CheckStatusUp       = "UP"
	CheckStatusDown     = "DOWN"
	CheckStatusDisabled = "DISABLED"
	CheckPostgreSQL     = "postgresql"
	CheckPlugins        = "plugins"
	CheckKafka          = "kafka"
)

type ReadinessProbe interface {
	Ping(context.Context) error
}

type PluginCatalog interface {
	Len() int
}

type ReadinessCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type ReadinessReport struct {
	Status string           `json:"status"`
	Checks []ReadinessCheck `json:"checks"`
}

type ReadinessService struct {
	postgresProbe ReadinessProbe
	pluginCatalog PluginCatalog
	kafkaProbe    ReadinessProbe
	asyncRequired bool
}

func NewReadinessService(
	postgresProbe ReadinessProbe,
	pluginCatalog PluginCatalog,
	kafkaProbe ReadinessProbe,
	asyncRequired bool,
) ReadinessService {
	return ReadinessService{
		postgresProbe: postgresProbe,
		pluginCatalog: pluginCatalog,
		kafkaProbe:    kafkaProbe,
		asyncRequired: asyncRequired,
	}
}

func (service ReadinessService) Check(
	ctx context.Context,
) ReadinessReport {
	if ctx == nil {
		ctx = context.Background()
	}

	checks := make([]ReadinessCheck, 0, 3)
	ready := true

	postgresStatus := CheckStatusDown

	if service.postgresProbe != nil &&
		service.postgresProbe.Ping(ctx) == nil {
		postgresStatus = CheckStatusUp
	} else {
		ready = false
	}

	checks = append(
		checks,
		ReadinessCheck{
			Name:   CheckPostgreSQL,
			Status: postgresStatus,
		},
	)

	pluginStatus := CheckStatusDown

	if service.pluginCatalog != nil &&
		service.pluginCatalog.Len() > 0 {
		pluginStatus = CheckStatusUp
	} else {
		ready = false
	}

	checks = append(
		checks,
		ReadinessCheck{
			Name:   CheckPlugins,
			Status: pluginStatus,
		},
	)

	kafkaStatus := CheckStatusDisabled

	if service.asyncRequired {
		kafkaStatus = CheckStatusDown

		if service.kafkaProbe != nil &&
			service.kafkaProbe.Ping(ctx) == nil {
			kafkaStatus = CheckStatusUp
		} else {
			ready = false
		}
	}

	checks = append(
		checks,
		ReadinessCheck{
			Name:   CheckKafka,
			Status: kafkaStatus,
		},
	)

	status := StatusNotReady

	if ready {
		status = StatusReady
	}

	return ReadinessReport{
		Status: status,
		Checks: checks,
	}
}
