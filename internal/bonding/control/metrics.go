// Package control implements the adaptive controller for DMB multipath bonding.
// It provides per-path performance estimation, scheduling algorithms, and the
// path state machine that governs promotion/demotion/quarantine decisions.
package control

import (
	"math"
	"sync"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// Per-Path EWMA Metrics Estimator
// ──────────────────────────────────────────────────────────────────────────────

// PathMetrics tracks real-time performance measurements for a single artery path.
// All timing values are in milliseconds. Thread-safe.
type PathMetrics struct {
	mu sync.RWMutex

	PathID string // "artery-0", "artery-1", ...

	// EWMA-smoothed round-trip time (RFC 6298 algorithm)
	SRTT   float64 // smoothed RTT (ms)
	RTTVar float64 // RTT variance (ms)
	MinRTT float64 // minimum RTT observed in the sliding window

	// Loss estimation
	LossPct float64 // loss percentage (0-100)
	totalSent   uint64
	totalLost   uint64

	// Throughput estimation
	DelivRate float64 // delivery rate (bytes/sec), EWMA smoothed
	CWnd      float64 // congestion window (frames)
	InFlight  float64 // currently in-flight frames

	// Win-rate tracking (for duplication mode)
	WinRate    float64 // percentage of races won (0-100)
	totalRaces uint64
	totalWins  uint64

	// Liveness
	LastPingSentAt time.Time
	LastPongRecvAt time.Time
	Alive          bool

	// Sliding window for minRTT calculation
	rttWindow     []float64
	rttWindowSize int
	rttWindowIdx  int

	// Timestamps
	CreatedAt   time.Time
	LastUpdated time.Time
}

const (
	// EWMA smoothing factors (RFC 6298)
	alphaRTT = 0.125 // 1/8
	betaRTT  = 0.25  // 1/4

	// Default sliding window for minRTT
	defaultRTTWindowSize = 100 // samples

	// EWMA smoothing for delivery rate
	alphaDelivRate = 0.2

	// EWMA smoothing for loss
	alphaLoss = 0.1

	// Initial congestion window
	initialCWnd = 16
)

// NewPathMetrics creates a new metrics tracker for a path.
func NewPathMetrics(pathID string) *PathMetrics {
	return &PathMetrics{
		PathID:        pathID,
		SRTT:          0,
		RTTVar:        0,
		MinRTT:        math.MaxFloat64,
		CWnd:          float64(initialCWnd),
		Alive:         true,
		rttWindow:     make([]float64, defaultRTTWindowSize),
		rttWindowSize: defaultRTTWindowSize,
		rttWindowIdx:  0,
		CreatedAt:     time.Now(),
		LastUpdated:   time.Now(),
	}
}

// UpdateRTT updates the smoothed RTT and variance using the RFC 6298 algorithm.
// rttMs is the measured round-trip time in milliseconds.
func (m *PathMetrics) UpdateRTT(rttMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.LastUpdated = time.Now()

	if m.SRTT == 0 {
		// First measurement
		m.SRTT = rttMs
		m.RTTVar = rttMs / 2
	} else {
		// RFC 6298: RTTVAR = (1 - beta) * RTTVAR + beta * |SRTT - R'|
		//           SRTT   = (1 - alpha) * SRTT + alpha * R'
		m.RTTVar = (1-betaRTT)*m.RTTVar + betaRTT*math.Abs(m.SRTT-rttMs)
		m.SRTT = (1-alphaRTT)*m.SRTT + alphaRTT*rttMs
	}

	// Update sliding window for minRTT
	m.rttWindow[m.rttWindowIdx] = rttMs
	m.rttWindowIdx = (m.rttWindowIdx + 1) % m.rttWindowSize

	// Recalculate minRTT from window
	minRTT := math.MaxFloat64
	for _, v := range m.rttWindow {
		if v > 0 && v < minRTT {
			minRTT = v
		}
	}
	m.MinRTT = minRTT
}

// RecordSend records a frame being sent on this path.
func (m *PathMetrics) RecordSend() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalSent++
	m.InFlight += 1
	m.LastUpdated = time.Now()
}

// RecordAck records a successful acknowledgment (frame arrived).
func (m *PathMetrics) RecordAck(deliveredBytes int, rttMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.InFlight > 0 {
		m.InFlight -= 1
	}
	m.LastUpdated = time.Now()

	// Update delivery rate (EWMA)
	if rttMs > 0 {
		instantRate := float64(deliveredBytes) / (rttMs / 1000.0) // bytes/sec
		if m.DelivRate == 0 {
			m.DelivRate = instantRate
		} else {
			m.DelivRate = (1-alphaDelivRate)*m.DelivRate + alphaDelivRate*instantRate
		}
	}

	// Update win-rate
	m.totalRaces++
	m.totalWins++
	if m.totalRaces > 0 {
		m.WinRate = float64(m.totalWins) / float64(m.totalRaces) * 100.0
	}
}

// RecordLoss records a lost frame on this path.
func (m *PathMetrics) RecordLoss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalLost++

	// Update loss percentage (EWMA)
	if m.totalSent > 0 {
		instantLoss := float64(m.totalLost) / float64(m.totalSent) * 100.0
		m.LossPct = (1-alphaLoss)*m.LossPct + alphaLoss*instantLoss
	}

	m.LastUpdated = time.Now()
}

// RecordRaceLoss records that another path won a race (this path lost).
func (m *PathMetrics) RecordRaceLoss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalRaces++
	if m.totalRaces > 0 {
		m.WinRate = float64(m.totalWins) / float64(m.totalRaces) * 100.0
	}
}

// RecordPingSent records the time a PING was sent on this path.
func (m *PathMetrics) RecordPingSent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastPingSentAt = time.Now()
}

// RecordPongReceived records the time a PONG was received on this path.
func (m *PathMetrics) RecordPongReceived() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastPongRecvAt = time.Now()
	m.Alive = true
}

// SetAlive explicitly sets the liveness flag.
func (m *PathMetrics) SetAlive(alive bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Alive = alive
}

// SetCWnd sets the congestion window (used by the adaptive controller).
func (m *PathMetrics) SetCWnd(cwnd float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CWnd = cwnd
}

// Snapshot returns a read-only copy of the current metrics.
type MetricsSnapshot struct {
	PathID     string  `json:"path_id"`
	SRTT       float64 `json:"srtt_ms"`
	RTTVar     float64 `json:"rtt_var_ms"`
	MinRTT     float64 `json:"min_rtt_ms"`
	LossPct    float64 `json:"loss_pct"`
	DelivRate  float64 `json:"deliv_rate_bps"`
	CWnd       float64 `json:"cwnd"`
	InFlight   float64 `json:"in_flight"`
	WinRate    float64 `json:"win_rate"`
	Alive      bool    `json:"alive"`
	TotalSent  uint64  `json:"total_sent"`
	TotalLost  uint64  `json:"total_lost"`
}

func (m *PathMetrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	minRTT := m.MinRTT
	if minRTT == math.MaxFloat64 {
		minRTT = 0
	}

	return MetricsSnapshot{
		PathID:    m.PathID,
		SRTT:      m.SRTT,
		RTTVar:    m.RTTVar,
		MinRTT:    minRTT,
		LossPct:   m.LossPct,
		DelivRate: m.DelivRate,
		CWnd:      m.CWnd,
		InFlight:  m.InFlight,
		WinRate:   m.WinRate,
		Alive:     m.Alive,
		TotalSent: m.totalSent,
		TotalLost: m.totalLost,
	}
}

// RTO returns the retransmission timeout using RFC 6298: RTO = SRTT + max(G, 4*RTTVAR)
// with a minimum of 200ms and maximum of 60s. G (clock granularity) = 1ms.
func (m *PathMetrics) RTO() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.SRTT == 0 {
		return 1 * time.Second // initial RTO
	}

	rtoMs := m.SRTT + math.Max(1.0, 4*m.RTTVar)
	if rtoMs < 200 {
		rtoMs = 200
	}
	if rtoMs > 60000 {
		rtoMs = 60000
	}
	return time.Duration(rtoMs) * time.Millisecond
}

// HasCapacity returns true if the path has room in its congestion window.
func (m *PathMetrics) HasCapacity() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.InFlight < m.CWnd && m.Alive
}
