package queue

import (
	"container/heap"

	"github.com/luxfi/ids"
)

// Job represents a bootstrap job
type Job interface {
	ID() ids.ID
	Priority() uint64
	Execute() error
}

// Queue is a priority queue for jobs
type Queue interface {
	// Push adds a job
	Push(Job)

	// Pop removes highest priority job
	Pop() Job

	// Len returns queue length
	Len() int

	// Has checks if job exists
	Has(ids.ID) bool
}

// priorityQueue implements heap.Interface
type priorityQueue []Job

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	return pq[i].Priority() > pq[j].Priority()
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *priorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(Job))
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[0 : n-1]
	return item
}

// queue implementation
type queue struct {
	pq   priorityQueue
	jobs map[ids.ID]Job
}

// NewQueue creates a new queue
func NewQueue() Queue {
	q := &queue{
		jobs: make(map[ids.ID]Job),
	}
	heap.Init(&q.pq)
	return q
}

// Push adds a job
func (q *queue) Push(job Job) {
	if _, exists := q.jobs[job.ID()]; !exists {
		heap.Push(&q.pq, job)
		q.jobs[job.ID()] = job
	}
}

// Pop removes highest priority job
func (q *queue) Pop() Job {
	if q.pq.Len() == 0 {
		return nil
	}
	job := heap.Pop(&q.pq).(Job)
	delete(q.jobs, job.ID())
	return job
}

// Len returns queue length
func (q *queue) Len() int {
	return q.pq.Len()
}

// Has checks if job exists
func (q *queue) Has(id ids.ID) bool {
	_, exists := q.jobs[id]
	return exists
}
