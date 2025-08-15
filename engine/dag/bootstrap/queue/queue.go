package queue

import (
	"github.com/luxfi/database"
)

// Jobs represents a queue of jobs to process
type Jobs interface {
	// Push adds a job to the queue
	Push([]byte) error
	
	// Pop removes a job from the queue
	Pop() ([]byte, error)
	
	// Has checks if a job exists
	Has([]byte) (bool, error)
	
	// Clear removes all jobs
	Clear() error
	
	// Size returns the number of jobs
	Size() (int, error)
}

// NewWithMissing creates a new job queue with missing functionality
func NewWithMissing(db database.Database, namespace string, metrics interface{}) (Jobs, error) {
	return &noOpJobs{}, nil
}

// New creates a new job queue
func New(db database.Database, namespace string, metrics interface{}) (Jobs, error) {
	return &noOpJobs{}, nil
}

type noOpJobs struct{}

func (n *noOpJobs) Push([]byte) error {
	return nil
}

func (n *noOpJobs) Pop() ([]byte, error) {
	return nil, database.ErrNotFound
}

func (n *noOpJobs) Has([]byte) (bool, error) {
	return false, nil
}

func (n *noOpJobs) Clear() error {
	return nil
}

func (n *noOpJobs) Size() (int, error) {
	return 0, nil
}