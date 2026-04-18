package ports

type HealthService interface {
	Liveness() error
	SetShuttingDown(bool)
}
