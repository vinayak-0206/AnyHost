package client

import (
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/anyhost/gotunnel/internal/common"
)

// Reconnector handles reconnection with exponential backoff.
type Reconnector struct {
	config *common.ReconnectConfig
	logger *slog.Logger

	mu           sync.Mutex
	attempts     int
	currentDelay time.Duration
	lastAttempt  time.Time
}

// NewReconnector creates a new reconnector.
func NewReconnector(cfg *common.ReconnectConfig, logger *slog.Logger) *Reconnector {
	return &Reconnector{
		config:       cfg,
		logger:       logger.With(slog.String("component", "reconnector")),
		currentDelay: cfg.InitialDelay,
	}
}

// NextDelay calculates the next delay before attempting to reconnect.
// Uses exponential backoff with jitter.
func (r *Reconnector) NextDelay() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.attempts++

	// Check if we've exceeded max attempts
	if r.config.MaxAttempts > 0 && r.attempts > r.config.MaxAttempts {
		r.logger.Warn("max reconnection attempts exceeded",
			slog.Int("attempts", r.attempts),
			slog.Int("max_attempts", r.config.MaxAttempts))
		return -1 // Signal to stop reconnecting
	}

	// Calculate base delay with exponential backoff
	baseDelay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(r.attempts-1))

	// Cap at max delay
	if baseDelay > float64(r.config.MaxDelay) {
		baseDelay = float64(r.config.MaxDelay)
	}

	// Add jitter (up to 25% of base delay)
	jitter := baseDelay * 0.25 * rand.Float64()
	delay := time.Duration(baseDelay + jitter)

	r.currentDelay = delay
	r.lastAttempt = time.Now()

	r.logger.Debug("calculated reconnect delay",
		slog.Int("attempt", r.attempts),
		slog.Duration("delay", delay))

	return delay
}

// Reset resets the reconnector state after a successful connection.
func (r *Reconnector) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.attempts = 0
	r.currentDelay = r.config.InitialDelay

	r.logger.Debug("reconnector reset")
}

// Attempts returns the number of reconnection attempts made.
func (r *Reconnector) Attempts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts
}

// CurrentDelay returns the current delay.
func (r *Reconnector) CurrentDelay() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentDelay
}

// ShouldRetry returns true if more reconnection attempts should be made.
func (r *Reconnector) ShouldRetry() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.config.MaxAttempts == 0 {
		return true // Unlimited retries
	}

	return r.attempts < r.config.MaxAttempts
}

// BackoffStrategy defines the interface for different backoff strategies.
type BackoffStrategy interface {
	NextDelay(attempt int) time.Duration
	Reset()
}

// ExponentialBackoff implements exponential backoff with jitter.
type ExponentialBackoff struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	MaxAttempts  int

	mu       sync.Mutex
	attempts int
}

// NextDelay returns the next delay for exponential backoff.
func (e *ExponentialBackoff) NextDelay(attempt int) time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.MaxAttempts > 0 && attempt > e.MaxAttempts {
		return -1
	}

	baseDelay := float64(e.InitialDelay) * math.Pow(e.Multiplier, float64(attempt-1))
	if baseDelay > float64(e.MaxDelay) {
		baseDelay = float64(e.MaxDelay)
	}

	// Add jitter
	jitter := baseDelay * 0.25 * rand.Float64()
	return time.Duration(baseDelay + jitter)
}

// Reset resets the backoff state.
func (e *ExponentialBackoff) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.attempts = 0
}

// LinearBackoff implements linear backoff.
type LinearBackoff struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Increment    time.Duration
	MaxAttempts  int

	mu       sync.Mutex
	attempts int
}

// NextDelay returns the next delay for linear backoff.
func (l *LinearBackoff) NextDelay(attempt int) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.MaxAttempts > 0 && attempt > l.MaxAttempts {
		return -1
	}

	delay := l.InitialDelay + time.Duration(attempt-1)*l.Increment
	if delay > l.MaxDelay {
		delay = l.MaxDelay
	}

	// Add small jitter
	jitter := time.Duration(float64(delay) * 0.1 * rand.Float64())
	return delay + jitter
}

// Reset resets the backoff state.
func (l *LinearBackoff) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.attempts = 0
}

// ConstantBackoff implements constant delay backoff.
type ConstantBackoff struct {
	Delay       time.Duration
	MaxAttempts int
}

// NextDelay returns the constant delay.
func (c *ConstantBackoff) NextDelay(attempt int) time.Duration {
	if c.MaxAttempts > 0 && attempt > c.MaxAttempts {
		return -1
	}
	return c.Delay
}

// Reset does nothing for constant backoff.
func (c *ConstantBackoff) Reset() {}
