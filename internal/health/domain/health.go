package domain

type HealthStatus string

const StatusHealthy HealthStatus = "ok"

type Health struct {
	Status HealthStatus
}

// NewHealthyStatus constructs the healthy domain status.
func NewHealthyStatus() Health {
	return Health{Status: StatusHealthy}
}
