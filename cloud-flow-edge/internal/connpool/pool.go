// Package connpool provides a gRPC connection pool manager for tracking and
// limiting Agent connections by IP and probe ID, with periodic stale cleanup.
package connpool

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrPoolFull is returned when the connection pool has reached its capacity.
	ErrPoolFull = errors.New("connection pool is full")

	// ErrDuplicateConnection is returned when a connection with the same probe ID already exists.
	ErrDuplicateConnection = errors.New("duplicate connection for probe ID")

	// ErrNotFound is returned when a connection is not found in the pool.
	ErrNotFound = errors.New("connection not found")
)

// ConnectionInfo holds metadata about a single Agent gRPC connection.
type ConnectionInfo struct {
	ProbeID      string
	IP           string
	ConnectedAt  time.Time
	LastActive   time.Time
}

// ConnectionStats holds aggregate statistics about the connection pool.
type ConnectionStats struct {
	TotalConnections int
	MaxConnections   int
	PerIPCounts      map[string]int
}

// OnConnectFunc is called when a new connection is established.
type OnConnectFunc func(info ConnectionInfo)

// OnDisconnectFunc is called when a connection is removed.
type OnDisconnectFunc func(info ConnectionInfo)

// Pool manages gRPC Agent connections with per-IP tracking and stale cleanup.
type Pool struct {
	maxConns    int
	staleTimeout time.Duration
	cleanupInterval time.Duration

	mu       sync.RWMutex
	conns    map[string]*ConnectionInfo // probeID -> ConnectionInfo
	ipCounts map[string]int             // IP -> connection count

	onConnect    OnConnectFunc
	onDisconnect OnDisconnectFunc

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// Option configures a Pool.
type Option func(*Pool)

// WithMaxConnections sets the maximum number of concurrent connections (default 10000).
func WithMaxConnections(n int) Option {
	return func(p *Pool) {
		if n > 0 {
			p.maxConns = n
		}
	}
}

// WithStaleTimeout sets the duration after which a connection is considered stale (default 5 minutes).
func WithStaleTimeout(d time.Duration) Option {
	return func(p *Pool) {
		if d > 0 {
			p.staleTimeout = d
		}
	}
}

// WithCleanupInterval sets the interval between periodic stale connection cleanups (default 1 minute).
func WithCleanupInterval(d time.Duration) Option {
	return func(p *Pool) {
		if d > 0 {
			p.cleanupInterval = d
		}
	}
}

// WithOnConnect sets the callback invoked when a new connection is added.
func WithOnConnect(fn OnConnectFunc) Option {
	return func(p *Pool) {
		p.onConnect = fn
	}
}

// WithOnDisconnect sets the callback invoked when a connection is removed.
func WithOnDisconnect(fn OnDisconnectFunc) Option {
	return func(p *Pool) {
		p.onDisconnect = fn
	}
}

// New creates a new connection pool and starts the periodic cleanup goroutine.
func New(opts ...Option) *Pool {
	p := &Pool{
		maxConns:       10000,
		staleTimeout:   5 * time.Minute,
		cleanupInterval: 1 * time.Minute,
		conns:          make(map[string]*ConnectionInfo),
		ipCounts:       make(map[string]int),
		stopCh:         make(chan struct{}),
	}
	for _, o := range opts {
		o(p)
	}

	p.wg.Add(1)
	go p.cleanupLoop()

	return p
}

// cleanupLoop periodically removes stale connections.
func (p *Pool) cleanupLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.RemoveStale()
		case <-p.stopCh:
			return
		}
	}
}

// Add registers a new connection. Returns an error if the pool is full or
// a connection with the same probe ID already exists.
func (p *Pool) Add(info ConnectionInfo) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.conns) >= p.maxConns {
		return ErrPoolFull
	}

	if _, exists := p.conns[info.ProbeID]; exists {
		return ErrDuplicateConnection
	}

	p.conns[info.ProbeID] = &info
	p.ipCounts[info.IP]++

	if p.onConnect != nil {
		p.onConnect(info)
	}

	return nil
}

// Remove unregisters a connection by probe ID.
func (p *Pool) Remove(probeID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	info, exists := p.conns[probeID]
	if !exists {
		return ErrNotFound
	}

	delete(p.conns, probeID)
	p.ipCounts[info.IP]--
	if p.ipCounts[info.IP] <= 0 {
		delete(p.ipCounts, info.IP)
	}

	if p.onDisconnect != nil {
		p.onDisconnect(*info)
	}

	return nil
}

// Touch updates the LastActive timestamp for a connection, indicating it is still alive.
func (p *Pool) Touch(probeID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	info, exists := p.conns[probeID]
	if !exists {
		return ErrNotFound
	}

	info.LastActive = time.Now()
	return nil
}

// Get retrieves connection info by probe ID.
func (p *Pool) Get(probeID string) (ConnectionInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	info, exists := p.conns[probeID]
	if !exists {
		return ConnectionInfo{}, ErrNotFound
	}

	return *info, nil
}

// GetByIP returns all connection infos for a given IP address.
func (p *Pool) GetByIP(ip string) []ConnectionInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []ConnectionInfo
	for _, info := range p.conns {
		if info.IP == ip {
			result = append(result, *info)
		}
	}
	return result
}

// IPCount returns the number of connections for a given IP.
func (p *Pool) IPCount(ip string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ipCounts[ip]
}

// RemoveStale removes all connections that have not been active within the stale timeout.
// Returns the number of connections removed.
func (p *Pool) RemoveStale() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	threshold := now.Add(-p.staleTimeout)

	var removed []ConnectionInfo
	for probeID, info := range p.conns {
		if info.LastActive.Before(threshold) {
			removed = append(removed, *info)
			delete(p.conns, probeID)
			p.ipCounts[info.IP]--
			if p.ipCounts[info.IP] <= 0 {
				delete(p.ipCounts, info.IP)
			}
		}
	}

	if p.onDisconnect != nil {
		for _, info := range removed {
			p.onDisconnect(info)
		}
	}

	return len(removed)
}

// GetStats returns a snapshot of the current pool statistics.
func (p *Pool) GetStats() ConnectionStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	perIP := make(map[string]int, len(p.ipCounts))
	for ip, count := range p.ipCounts {
		perIP[ip] = count
	}

	return ConnectionStats{
		TotalConnections: len(p.conns),
		MaxConnections:   p.maxConns,
		PerIPCounts:      perIP,
	}
}

// Count returns the current total number of connections.
func (p *Pool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.conns)
}

// Capacity returns the maximum number of connections allowed.
func (p *Pool) Capacity() int {
	return p.maxConns
}

// Stop gracefully shuts down the pool, stopping the cleanup goroutine.
// It does not close existing connections — callers are responsible for that.
func (p *Pool) Stop() {
	close(p.stopCh)
	p.wg.Wait()
}

// String returns a human-readable summary of the pool state.
func (p *Pool) String() string {
	stats := p.GetStats()
	return fmt.Sprintf(
		"connpool{total=%d, max=%d, ips=%d}",
		stats.TotalConnections,
		stats.MaxConnections,
		len(stats.PerIPCounts),
	)
}
