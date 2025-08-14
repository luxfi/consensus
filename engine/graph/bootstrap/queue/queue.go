package queue

// Job represents a bootstrap job
type Job interface {
    Execute() error
    Priority() int
}

// Queue manages bootstrap jobs
type Queue struct {
    jobs []Job
}

// New creates a new queue
func New() *Queue {
    return &Queue{
        jobs: make([]Job, 0),
    }
}

// Push adds a job to the queue
func (q *Queue) Push(job Job) {
    q.jobs = append(q.jobs, job)
}

// Pop removes and returns the next job
func (q *Queue) Pop() Job {
    if len(q.jobs) == 0 {
        return nil
    }
    job := q.jobs[0]
    q.jobs = q.jobs[1:]
    return job
}