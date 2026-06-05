package soroush

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/logger"

	"github.com/hashicorp/yamux"
)

// PoolStats contains telemetry for the multiplexer pool.
type PoolStats struct {
	TotalWorkers   int   `json:"total_workers"`
	HealthyWorkers int   `json:"healthy_workers"`
	TotalStreams    int64 `json:"total_streams"`
	AvgLatencyMs   int64 `json:"avg_latency_ms"`
}

// WorkerChannel represents a single worker's tunnel connection.
type WorkerChannel struct {
	AccountID    string
	Transport    *LiveKitTransport
	YamuxSession *yamux.Session
	LatencyMs    int64
	LastPingAt   time.Time
	Healthy      bool
}

// MultiplexerPool manages a set of worker channels and provides
// load-balanced yamux stream allocation for SOCKS5 connections.
type MultiplexerPool struct {
	mu      sync.RWMutex
	workers []*WorkerChannel
	algo    string // "round-robin" or "least-latency"
	rrIndex atomic.Uint64
}

// NewMultiplexerPool creates a new pool with the given load balancing algorithm.
func NewMultiplexerPool(algo string) *MultiplexerPool {
	if algo == "" {
		algo = "least-latency"
	}
	return &MultiplexerPool{
		algo: algo,
	}
}

// NextStream opens a yamux stream on the best available worker.
// Returns a net.Conn that the SOCKS5 handler can use for bidirectional I/O.
func (p *MultiplexerPool) NextStream() (net.Conn, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	healthy := p.healthyWorkersLocked()
	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy workers available")
	}

	var wc *WorkerChannel

	switch p.algo {
	case "round-robin":
		idx := p.rrIndex.Add(1) - 1
		wc = healthy[idx%uint64(len(healthy))]

	case "least-latency":
		wc = healthy[0]
		for _, w := range healthy[1:] {
			if w.LatencyMs < wc.LatencyMs {
				wc = w
			}
		}

	default:
		// Fallback to first healthy worker
		wc = healthy[0]
	}

	stream, err := wc.YamuxSession.Open()
	if err != nil {
		return nil, fmt.Errorf("yamux open stream on worker %s: %w", wc.AccountID, err)
	}

	totalStreams.Add(1)
	return stream, nil
}

// Inject adds a healthy worker to the pool.
func (p *MultiplexerPool) Inject(wc *WorkerChannel) {
	p.mu.Lock()
	defer p.mu.Unlock()

	wc.Healthy = true
	wc.LastPingAt = time.Now()
	p.workers = append(p.workers, wc)

	logger.Info(component, "Worker injected into pool",
		"account_id", wc.AccountID,
		"total_workers", len(p.workers),
	)
}

// Purge removes a worker from the pool by account ID.
func (p *MultiplexerPool) Purge(accountID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, w := range p.workers {
		if w.AccountID == accountID {
			p.workers = append(p.workers[:i], p.workers[i+1:]...)
			logger.Info(component, "Worker purged from pool",
				"account_id", accountID,
				"remaining_workers", len(p.workers),
			)
			return
		}
	}
}

// HealthCheck runs periodic yamux ping checks on all workers.
// Marks unhealthy workers and removes dead ones.
func (p *MultiplexerPool) HealthCheck(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.mu.Lock()
			for _, w := range p.workers {
				if w.YamuxSession.IsClosed() {
					w.Healthy = false
					continue
				}

				start := time.Now()
				_, err := w.YamuxSession.Ping()
				if err != nil {
					w.Healthy = false
					logger.Warn(component, "Worker health check failed",
						"account_id", w.AccountID, "error", err,
					)
				} else {
					w.LatencyMs = time.Since(start).Milliseconds()
					w.LastPingAt = time.Now()
					w.Healthy = true
				}
			}

			// Remove dead workers
			alive := p.workers[:0]
			for _, w := range p.workers {
				if w.Healthy || !w.YamuxSession.IsClosed() {
					alive = append(alive, w)
				}
			}
			p.workers = alive
			p.mu.Unlock()
		}
	}
}

// Stats returns current pool telemetry.
func (p *MultiplexerPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	healthy := p.healthyWorkersLocked()
	var totalLatency int64
	for _, w := range healthy {
		totalLatency += w.LatencyMs
	}

	var avgLatency int64
	if len(healthy) > 0 {
		avgLatency = totalLatency / int64(len(healthy))
	}

	return PoolStats{
		TotalWorkers:   len(p.workers),
		HealthyWorkers: len(healthy),
		TotalStreams:    totalStreams.Load(),
		AvgLatencyMs:   avgLatency,
	}
}

// healthyWorkersLocked returns healthy workers (caller must hold at least RLock).
func (p *MultiplexerPool) healthyWorkersLocked() []*WorkerChannel {
	var healthy []*WorkerChannel
	for _, w := range p.workers {
		if w.Healthy {
			healthy = append(healthy, w)
		}
	}
	return healthy
}
