package proxy

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics holds runtime statistics about client connections and query volumes.
type Metrics struct {
	StartTime            time.Time            `json:"start_time"`
	ActiveConnections    int64                `json:"active_connections"`
	TotalConnections     int64                `json:"total_connections"`
	TotalQueries         int64                `json:"total_queries"`
	BranchRoutingCounts  map[string]int64     `json:"branch_routing_counts"`
	BranchActiveConns    map[string]int64     `json:"branch_active_conns"`
	BytesReceived        int64                `json:"bytes_received"`
	BytesSent            int64                `json:"bytes_sent"`
	mu                   sync.RWMutex
}

// NewMetrics initializes an empty metrics tracker.
func NewMetrics() *Metrics {
	return &Metrics{
		StartTime:           time.Now(),
		BranchRoutingCounts: make(map[string]int64),
		BranchActiveConns:   make(map[string]int64),
	}
}

// ConnOpened increments active and total connection counters for a branch.
func (m *Metrics) ConnOpened(branchDB string) {
	atomic.AddInt64(&m.ActiveConnections, 1)
	atomic.AddInt64(&m.TotalConnections, 1)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.BranchRoutingCounts[branchDB]++
	m.BranchActiveConns[branchDB]++
}

// ConnClosed decrements active connection counters.
func (m *Metrics) ConnClosed(branchDB string) {
	atomic.AddInt64(&m.ActiveConnections, -1)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.BranchActiveConns[branchDB] > 0 {
		m.BranchActiveConns[branchDB]--
	}
}

// QueryExecuted increments the query counter.
func (m *Metrics) QueryExecuted() {
	atomic.AddInt64(&m.TotalQueries, 1)
}

// AddBytes tracks network throughput.
func (m *Metrics) AddBytes(rx, tx int64) {
	atomic.AddInt64(&m.BytesReceived, rx)
	atomic.AddInt64(&m.BytesSent, tx)
}

// Snapshot returns a copy of current metrics for API consumption.
func (m *Metrics) Snapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	routingCopy := make(map[string]int64, len(m.BranchRoutingCounts))
	for k, v := range m.BranchRoutingCounts {
		routingCopy[k] = v
	}

	activeCopy := make(map[string]int64, len(m.BranchActiveConns))
	for k, v := range m.BranchActiveConns {
		activeCopy[k] = v
	}

	return map[string]interface{}{
		"uptime_seconds":        int64(time.Since(m.StartTime).Seconds()),
		"active_connections":    atomic.LoadInt64(&m.ActiveConnections),
		"total_connections":     atomic.LoadInt64(&m.TotalConnections),
		"total_queries":         atomic.LoadInt64(&m.TotalQueries),
		"bytes_received":        atomic.LoadInt64(&m.BytesReceived),
		"bytes_sent":            atomic.LoadInt64(&m.BytesSent),
		"branch_routing_counts": routingCopy,
		"branch_active_conns":   activeCopy,
	}
}
