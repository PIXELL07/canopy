package notify

import (
	"context"
	"sync"

	"github.com/pixell07/canopy/internal/models"
	"go.uber.org/zap"
)

const defaultWorkers = 10

// single delivery task queued to the pool.
type job struct {
	ctx   context.Context
	event models.WebhookEvent
	data  map[string]interface{}
}

// Pool is a bounded worker pool that fans out webhook deliveries.
// At most `workers` goroutines run concurrently, preventing runaway
// goroutine creation when many servers go offline simultaneously.
type Pool struct {
	notifier *WebhookNotifier
	jobs     chan job
	wg       sync.WaitGroup
	log      *zap.Logger
}

// NewPool creates a pool with `workers` concurrent delivery goroutines
// and a buffered job queue of size `queueSize`.
func NewPool(n *WebhookNotifier, workers, queueSize int, log *zap.Logger) *Pool {
	if workers <= 0 {
		workers = defaultWorkers
	}
	if queueSize <= 0 {
		queueSize = 100
	}
	p := &Pool{
		notifier: n,
		jobs:     make(chan job, queueSize),
		log:      log,
	}
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
	return p
}

// Enqueue adds a webhook delivery task to the pool.
// If the queue is full it drops the job and logs a warning rather than
// blocking the caller (e.g. the watcher goroutine).
func (p *Pool) Enqueue(ctx context.Context, event models.WebhookEvent, data map[string]interface{}) {
	select {
	case p.jobs <- job{ctx: ctx, event: event, data: data}:
	default:
		p.log.Warn("webhook pool queue full — delivery dropped",
			zap.String("event", string(event)),
		)
	}
}

// Stop drains the queue and waits for all in-flight deliveries to finish.
// Call this during graceful shutdown.
func (p *Pool) Stop() {
	close(p.jobs)
	p.wg.Wait()
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for j := range p.jobs {
		p.notifier.Send(j.ctx, j.event, j.data)
	}
}
