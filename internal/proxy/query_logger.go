package proxy

import (
	"encoding/binary"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LoggedQuery represents an intercepted SQL query captured from the PostgreSQL wire protocol.
type LoggedQuery struct {
	ID         int64     `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Branch     string    `json:"branch"`
	Database   string    `json:"database"`
	ClientAddr string    `json:"client_addr"`
	QueryType  string    `json:"query_type"` // SELECT, INSERT, UPDATE, DELETE, DDL, OTHER
	SQL        string    `json:"sql"`
	DurationMs float64   `json:"duration_ms"`
}

// QueryRingBuffer is a thread-safe circular ring buffer for query observability.
type QueryRingBuffer struct {
	mu       sync.RWMutex
	capacity int
	entries  []LoggedQuery
	start    int
	count    int
	seq      atomic.Int64
}

// NewQueryRingBuffer creates a circular buffer with the given capacity.
func NewQueryRingBuffer(capacity int) *QueryRingBuffer {
	if capacity <= 0 {
		capacity = 1000
	}
	return &QueryRingBuffer{
		capacity: capacity,
		entries:  make([]LoggedQuery, capacity),
	}
}

// Record captures a new query event.
func (rb *QueryRingBuffer) Record(branch, database, clientAddr, sql string, durationMs float64) LoggedQuery {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return LoggedQuery{}
	}

	qType := ClassifyQuery(sql)
	entry := LoggedQuery{
		ID:         rb.seq.Add(1),
		Timestamp:  time.Now(),
		Branch:     branch,
		Database:   database,
		ClientAddr: clientAddr,
		QueryType:  qType,
		SQL:        sql,
		DurationMs: durationMs,
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	idx := (rb.start + rb.count) % rb.capacity
	if rb.count < rb.capacity {
		rb.entries[idx] = entry
		rb.count++
	} else {
		// Buffer is full: overwrite oldest entry and advance start pointer
		rb.entries[rb.start] = entry
		rb.start = (rb.start + 1) % rb.capacity
	}

	return entry
}

// List returns recorded queries with optional filtering, ordered newest first.
func (rb *QueryRingBuffer) List(branch, search, queryType string, limit int) []LoggedQuery {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if limit <= 0 || limit > rb.count {
		limit = rb.count
	}

	search = strings.ToLower(strings.TrimSpace(search))
	queryType = strings.ToUpper(strings.TrimSpace(queryType))

	var results []LoggedQuery
	for i := rb.count - 1; i >= 0; i-- {
		idx := (rb.start + i) % rb.capacity
		item := rb.entries[idx]

		if branch != "" && item.Branch != branch {
			continue
		}
		if queryType != "" && item.QueryType != queryType {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(item.SQL), search) {
			continue
		}

		results = append(results, item)
		if len(results) >= limit {
			break
		}
	}

	return results
}

// Clear wipes all recorded queries.
func (rb *QueryRingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.start = 0
	rb.count = 0
}

// Count returns the current number of logged entries in the buffer.
func (rb *QueryRingBuffer) Count() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

// ClassifyQuery determines whether a query is SELECT, INSERT, UPDATE, DELETE, DDL, or OTHER.
func ClassifyQuery(sql string) string {
	clean := strings.ToUpper(strings.TrimSpace(sql))
	// Strip leading SQL comments if present
	for strings.HasPrefix(clean, "/*") {
		idx := strings.Index(clean, "*/")
		if idx == -1 {
			break
		}
		clean = strings.TrimSpace(clean[idx+2:])
	}
	for strings.HasPrefix(clean, "--") {
		idx := strings.Index(clean, "\n")
		if idx == -1 {
			break
		}
		clean = strings.TrimSpace(clean[idx+1:])
	}

	switch {
	case strings.HasPrefix(clean, "SELECT ") || strings.HasPrefix(clean, "WITH "):
		return "SELECT"
	case strings.HasPrefix(clean, "INSERT "):
		return "INSERT"
	case strings.HasPrefix(clean, "UPDATE "):
		return "UPDATE"
	case strings.HasPrefix(clean, "DELETE "):
		return "DELETE"
	case strings.HasPrefix(clean, "CREATE ") || strings.HasPrefix(clean, "ALTER ") || strings.HasPrefix(clean, "DROP ") || strings.HasPrefix(clean, "TRUNCATE "):
		return "DDL"
	case strings.HasPrefix(clean, "BEGIN") || strings.HasPrefix(clean, "COMMIT") || strings.HasPrefix(clean, "ROLLBACK"):
		return "TX"
	default:
		return "OTHER"
	}
}

// ExtractSQLFromPacket parses the SQL statement text from 'Q' (Simple Query) or 'P' (Parse) packet.
func ExtractSQLFromPacket(payload []byte) (string, bool) {
	if len(payload) < 6 {
		return "", false
	}

	msgType := payload[0]
	pktLen := int(binary.BigEndian.Uint32(payload[1:5]))
	if len(payload) < 1+pktLen {
		return "", false
	}

	data := payload[5 : 1+pktLen]

	if msgType == 'Q' {
		// 'Q' format: null-terminated query string
		for i, b := range data {
			if b == 0 {
				return string(data[:i]), true
			}
		}
		return string(data), true
	}

	if msgType == 'P' {
		// 'P' format:
		// 1. null-terminated prepared statement name
		firstNull := -1
		for i, b := range data {
			if b == 0 {
				firstNull = i
				break
			}
		}
		if firstNull == -1 || firstNull+1 >= len(data) {
			return "", false
		}

		// 2. null-terminated query string
		queryPart := data[firstNull+1:]
		for i, b := range queryPart {
			if b == 0 {
				return string(queryPart[:i]), true
			}
		}
		return string(queryPart), true
	}

	return "", false
}
