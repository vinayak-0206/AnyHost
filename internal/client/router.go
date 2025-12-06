package client

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/anyhost/gotunnel/internal/common"
	"github.com/anyhost/gotunnel/internal/protocol"
)

// Router routes incoming streams to local services.
type Router struct {
	config *common.ClientConfig
	logger *slog.Logger
	pools  map[int]*ConnectionPool

	mu sync.RWMutex
}

// NewRouter creates a new router with connection pooling.
func NewRouter(cfg *common.ClientConfig, logger *slog.Logger) *Router {
	r := &Router{
		config: cfg,
		logger: logger.With(slog.String("component", "router")),
		pools:  make(map[int]*ConnectionPool),
	}

	// Initialize connection pools for each tunnel
	for _, tunnel := range cfg.Tunnels {
		addr := fmt.Sprintf("%s:%d", tunnel.LocalHost, tunnel.LocalPort)
		if tunnel.LocalHost == "" {
			addr = fmt.Sprintf("127.0.0.1:%d", tunnel.LocalPort)
		}
		r.pools[tunnel.LocalPort] = NewConnectionPool(addr, &PoolConfig{
			MaxIdleConns:    10,
			MaxOpenConns:    100,
			ConnMaxLifetime: 5 * time.Minute,
			ConnMaxIdleTime: 1 * time.Minute,
			DialTimeout:     5 * time.Second,
		})
	}

	return r
}

// Forward forwards a stream to the appropriate local service.
func (r *Router) Forward(stream net.Conn, header *protocol.StreamHeader) error {
	r.mu.RLock()
	pool, exists := r.pools[header.LocalPort]
	r.mu.RUnlock()

	var localConn net.Conn
	var err error

	if exists {
		// Use pooled connection
		localConn, err = pool.Get()
		if err != nil {
			return fmt.Errorf("failed to get connection from pool: %w", err)
		}
		defer pool.Put(localConn)
	} else {
		// Direct connection (fallback)
		addr := header.GetLocalAddr()
		localConn, err = net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			return fmt.Errorf("failed to connect to %s: %w", addr, err)
		}
		defer localConn.Close()
	}

	// Bidirectional copy
	return r.pipe(stream, localConn)
}

// pipe copies data bidirectionally between two connections.
func (r *Router) pipe(server, local net.Conn) error {
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	// Server -> Local
	go func() {
		defer wg.Done()
		_, err := io.Copy(local, server)
		if err != nil && !isClosedError(err) {
			errCh <- fmt.Errorf("server->local: %w", err)
		}
		// Signal EOF to local
		if tc, ok := local.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	// Local -> Server
	go func() {
		defer wg.Done()
		_, err := io.Copy(server, local)
		if err != nil && !isClosedError(err) {
			errCh <- fmt.Errorf("local->server: %w", err)
		}
		// Signal EOF to server
		if tc, ok := server.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()

	wg.Wait()
	close(errCh)

	// Return first error if any
	for err := range errCh {
		return err
	}

	return nil
}

// Close closes all connection pools.
func (r *Router) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, pool := range r.pools {
		pool.Close()
	}
	r.pools = make(map[int]*ConnectionPool)
}

// AddPool adds a connection pool for a new tunnel.
func (r *Router) AddPool(port int, host string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", host, port)
	if host == "" {
		addr = fmt.Sprintf("127.0.0.1:%d", port)
	}

	r.pools[port] = NewConnectionPool(addr, &PoolConfig{
		MaxIdleConns:    10,
		MaxOpenConns:    100,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
		DialTimeout:     5 * time.Second,
	})
}

// RemovePool removes a connection pool.
func (r *Router) RemovePool(port int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if pool, exists := r.pools[port]; exists {
		pool.Close()
		delete(r.pools, port)
	}
}

// GetPoolStats returns statistics for all connection pools.
func (r *Router) GetPoolStats() map[int]PoolStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[int]PoolStats)
	for port, pool := range r.pools {
		stats[port] = pool.Stats()
	}
	return stats
}

// isClosedError checks if the error is due to a closed connection.
func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	// Check for common closed connection errors
	if netErr, ok := err.(*net.OpError); ok {
		if netErr.Err.Error() == "use of closed network connection" {
			return true
		}
	}
	return false
}
