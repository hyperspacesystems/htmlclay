package dataguard

import (
	"log"
	"sync"
)

// Queue judges jobs for one document in the order they were submitted. Two saves
// in a row are judged in the order they happened, because a key's jobs run on a
// single goroutine, and a slow judge for one document never blocks another.
type Queue struct {
	mu   sync.Mutex
	runs map[string]*run
	wg   sync.WaitGroup
}

// run is one key's FIFO and whether a worker is already draining it.
type run struct {
	jobs    []func()
	running bool
}

// Do appends job to its key's FIFO and starts a worker when none is running.
func (q *Queue) Do(key string, job func()) {
	q.mu.Lock()
	if q.runs == nil {
		q.runs = make(map[string]*run)
	}
	r := q.runs[key]
	if r == nil {
		r = &run{}
		q.runs[key] = r
	}
	r.jobs = append(r.jobs, job)
	start := !r.running
	r.running = true
	q.wg.Add(1)
	q.mu.Unlock()

	if start {
		go q.work(key, r)
	}
}

// work drains one key's FIFO and drops the key's map entry when it is empty.
func (q *Queue) work(key string, r *run) {
	for {
		q.mu.Lock()
		if len(r.jobs) == 0 {
			r.running = false
			delete(q.runs, key)
			q.mu.Unlock()
			return
		}
		job := r.jobs[0]
		r.jobs[0] = nil
		r.jobs = r.jobs[1:]
		q.mu.Unlock()

		q.judge(job)
		q.wg.Done()
	}
}

// judge runs one job and swallows its panic: a guard failure must never bring
// the server down or stop the jobs queued behind it.
func (q *Queue) judge(job func()) {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("dataguard: recovered panic in queued job: %v", v)
		}
	}()
	job()
}

// Settle waits for every job submitted so far to finish. Tests use it.
func (q *Queue) Settle() {
	q.wg.Wait()
}
