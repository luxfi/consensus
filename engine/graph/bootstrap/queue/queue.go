package queue

import (
	"github.com/luxfi/database"
	"github.com/luxfi/metric"
)

// Jobs represents a job queue
type Jobs interface {
	Push(job interface{}) error
	Pop() (interface{}, error)
	Has(jobID interface{}) (bool, error)
	Clear() error
}

// NewWithMissing creates a new job queue with missing job tracking
func NewWithMissing(db database.Database, namespace string, metrics metric.Metrics) (Jobs, error) {
	// Simple implementation - returns a no-op queue for now
	return &noOpJobs{}, nil
}

// New creates a new job queue
func New(db database.Database, namespace string, metrics metric.Metrics) (Jobs, error) {
	// Simple implementation - returns a no-op queue for now
	return &noOpJobs{}, nil
}

type noOpJobs struct{}

func (n *noOpJobs) Push(job interface{}) error {
	return nil
}

func (n *noOpJobs) Pop() (interface{}, error) {
	return nil, database.ErrNotFound
}

func (n *noOpJobs) Has(jobID interface{}) (bool, error) {
	return false, nil
}

func (n *noOpJobs) Clear() error {
	return nil
}