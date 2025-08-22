package core

import "context"

// HealthCheckable represents something that can report its health
type HealthCheckable interface {
	// HealthCheck returns health information
	HealthCheck(context.Context) (interface{}, error)
}

// HealthStatus represents health status
type HealthStatus int

const (
	HealthUnknown HealthStatus = iota
	HealthHealthy
	HealthUnhealthy
)

// String returns the string representation
func (h HealthStatus) String() string {
	switch h {
	case HealthHealthy:
		return "healthy"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}