package client

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// PoolConfig holds configuration for a connection pool.
type PoolConfig struct {
	// MaxIdleConns is the maximum number of idle connections to keep.
	MaxIdleConns int

	// MaxOpenConns is the maximum number of open connections.
	MaxOpenConns int

	// ConnMaxLifetime is the maximum lifetime of a connection.
	ConnMaxLifetime time.Duration

	// ConnMaxIdleTime is the maximum time a connection can be idle.
	ConnMaxIdleTime time.Duration

	// DialTimeout is the timeout for establishing new connections.
	DialTimeout time.Duration
}

// DefaultPoolConfig returns sensible default pool configuration.
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxIdleConns:    10,
		MaxOpenConns:    100,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
		DialTimeout:     5 * time.Second,
	}
}

// PoolStats holds statistics for a connection pool.
type PoolStats struct {
	Address     string
	IdleConns   int
	OpenConns   int
	WaitCount   int64
	TotalConns  int64
	TotalReused int64
}

// pooledConn wraps a connection with metadata.
type pooledConn struct {
	conn      net.Conn
	createdAt time.Time
	lastUsed  time.Time
}

// ConnectionPool manages a pool of reusable connections.
type ConnectionPool struct {
	addr   string
	config *PoolConfig

	mu       sync.Mutex
	idle     []*pooledConn
	numOpen  int
	closed   bool
	cleanerC chan struct{}

	// Stats
	waitCount   atomic.Int64
	totalConns  atomic.Int64
	totalReused atomic.Int64
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(addr string, cfg *PoolConfig) *ConnectionPool {
	if cfg == nil {
		cfg = DefaultPoolConfig()
	}

	pool := &ConnectionPool{
		addr:     addr,
		config:   cfg,
		idle:     make([]*pooledConn, 0, cfg.MaxIdleConns),
		cleanerC: make(chan struct{}),
	}

	// Start cleaner goroutine
	go pool.cleaner()

	return pool
}

// Get retrieves a connection from the pool or creates a new one.
func (p *ConnectionPool) Get() (net.Conn, error) {
	p.mu.Lock()

	if p.closed {
		p.mu.Unlock()
		return nil, errors.New("pool is closed")
	}

	// Try to get an idle connection
	for len(p.idle) > 0 {
		// Pop from the end (most recently used)
		pc := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]

		// Check if connection is still valid
		if p.isValidConnection(pc) {
			pc.lastUsed = time.Now()
			p.mu.Unlock()
			p.totalReused.Add(1)
			return pc.conn, nil
		}

		// Connection is stale, close it
		pc.conn.Close()
		p.numOpen--
	}

	// Check if we can create a new connection
	if p.config.MaxOpenConns > 0 && p.numOpen >= p.config.MaxOpenConns {
		p.waitCount.Add(1)
		p.mu.Unlock()
		return nil, errors.New("connection pool exhausted")
	}

	// Create new connection
	p.numOpen++
	p.mu.Unlock()

	conn, err := net.DialTimeout("tcp", p.addr, p.config.DialTimeout)
	if err != nil {
		p.mu.Lock()
		p.numOpen--
		p.mu.Unlock()
		return nil, err
	}

	p.totalConns.Add(1)
	return conn, nil
}

// Put returns a connection to the pool.
func (p *ConnectionPool) Put(conn net.Conn) {
	if conn == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		conn.Close()
		p.numOpen--
		return
	}

	// Check if we have room in the idle pool
	if len(p.idle) >= p.config.MaxIdleConns {
		conn.Close()
		p.numOpen--
		return
	}

	pc := &pooledConn{
		conn:      conn,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}

	p.idle = append(p.idle, pc)
}

// Close closes the pool and all connections.
func (p *ConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	p.closed = true
	close(p.cleanerC)

	// Close all idle connections
	for _, pc := range p.idle {
		pc.conn.Close()
	}
	p.idle = nil
	p.numOpen = 0
}

// Stats returns pool statistics.
func (p *ConnectionPool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	return PoolStats{
		Address:     p.addr,
		IdleConns:   len(p.idle),
		OpenConns:   p.numOpen,
		WaitCount:   p.waitCount.Load(),
		TotalConns:  p.totalConns.Load(),
		TotalReused: p.totalReused.Load(),
	}
}

// isValidConnection checks if a pooled connection is still valid.
func (p *ConnectionPool) isValidConnection(pc *pooledConn) bool {
	now := time.Now()

	// Check max lifetime
	if p.config.ConnMaxLifetime > 0 && now.Sub(pc.createdAt) > p.config.ConnMaxLifetime {
		return false
	}

	// Check max idle time
	if p.config.ConnMaxIdleTime > 0 && now.Sub(pc.lastUsed) > p.config.ConnMaxIdleTime {
		return false
	}

	// Quick connectivity check (set short deadline)
	pc.conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	buf := make([]byte, 1)
	_, err := pc.conn.Read(buf)
	pc.conn.SetReadDeadline(time.Time{})

	// If we get EOF or a real error (not timeout), connection is dead
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// Timeout is expected for healthy connections
			return true
		}
		return false
	}

	return true
}

// cleaner periodically cleans up stale connections.
func (p *ConnectionPool) cleaner() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.cleanerC:
			return
		case <-ticker.C:
			p.cleanIdleConnections()
		}
	}
}

// cleanIdleConnections removes stale connections from the idle pool.
func (p *ConnectionPool) cleanIdleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	now := time.Now()
	newIdle := make([]*pooledConn, 0, len(p.idle))

	for _, pc := range p.idle {
		// Check if connection has exceeded idle time
		if p.config.ConnMaxIdleTime > 0 && now.Sub(pc.lastUsed) > p.config.ConnMaxIdleTime {
			pc.conn.Close()
			p.numOpen--
			continue
		}

		// Check if connection has exceeded max lifetime
		if p.config.ConnMaxLifetime > 0 && now.Sub(pc.createdAt) > p.config.ConnMaxLifetime {
			pc.conn.Close()
			p.numOpen--
			continue
		}

		newIdle = append(newIdle, pc)
	}

	p.idle = newIdle
}
