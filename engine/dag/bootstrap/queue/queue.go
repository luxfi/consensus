// Package queue provides a simple blocking queue for DAG bootstrap
package queue

import "sync"

// Queue is a simple blocking queue for DAG vertices during bootstrap
type Queue struct {
	mu    sync.Mutex
	items []interface{}
}

// NewQueue creates a new empty queue
func NewQueue() *Queue {
	return &Queue{
		items: make([]interface{}, 0),
	}
}

// Push adds an item to the queue
func (q *Queue) Push(item interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, item)
}

// Pop removes and returns the first item from the queue
// Returns nil if the queue is empty
func (q *Queue) Pop() interface{} {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item
}

// Len returns the number of items in the queue
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// IsEmpty returns true if the queue is empty
func (q *Queue) IsEmpty() bool {
	return q.Len() == 0
}
