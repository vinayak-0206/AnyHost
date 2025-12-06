package protocol

import (
	"errors"
	"fmt"
)

// Protocol-level errors that can occur during communication.
var (
	// ErrVersionMismatch indicates incompatible protocol versions.
	ErrVersionMismatch = errors.New("protocol version mismatch")

	// ErrInvalidHandshake indicates a malformed handshake request.
	ErrInvalidHandshake = errors.New("invalid handshake request")

	// ErrUnauthorized indicates authentication failure.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrSubdomainTaken indicates the requested subdomain is in use.
	ErrSubdomainTaken = errors.New("subdomain is already taken")

	// ErrSubdomainReserved indicates the subdomain is reserved.
	ErrSubdomainReserved = errors.New("subdomain is reserved")

	// ErrSubdomainInvalid indicates the subdomain format is invalid.
	ErrSubdomainInvalid = errors.New("subdomain format is invalid")

	// ErrConnectionClosed indicates the connection was closed.
	ErrConnectionClosed = errors.New("connection closed")

	// ErrStreamClosed indicates the stream was closed.
	ErrStreamClosed = errors.New("stream closed")

	// ErrTimeout indicates an operation timed out.
	ErrTimeout = errors.New("operation timed out")

	// ErrRateLimited indicates the client has been rate limited.
	ErrRateLimited = errors.New("rate limited")

	// ErrTunnelNotFound indicates the requested tunnel does not exist.
	ErrTunnelNotFound = errors.New("tunnel not found")

	// ErrTunnelLimitReached indicates the maximum tunnel limit has been reached.
	ErrTunnelLimitReached = errors.New("tunnel limit reached")

	// ErrInvalidMessage indicates a malformed protocol message.
	ErrInvalidMessage = errors.New("invalid protocol message")
)

// ProtocolError wraps an error with additional protocol context.
type ProtocolError struct {
	Code       string
	Message    string
	Underlying error
}

// Error implements the error interface.
func (pe *ProtocolError) Error() string {
	if pe.Underlying != nil {
		return fmt.Sprintf("%s: %s (%s)", pe.Code, pe.Message, pe.Underlying.Error())
	}
	return fmt.Sprintf("%s: %s", pe.Code, pe.Message)
}

// Unwrap returns the underlying error for errors.Is/As support.
func (pe *ProtocolError) Unwrap() error {
	return pe.Underlying
}

// NewProtocolError creates a new ProtocolError with the given details.
func NewProtocolError(code, message string, underlying error) *ProtocolError {
	return &ProtocolError{
		Code:       code,
		Message:    message,
		Underlying: underlying,
	}
}

// ErrorToCode converts a known error to its corresponding error code.
func ErrorToCode(err error) string {
	switch {
	case errors.Is(err, ErrUnauthorized):
		return ErrorCodeUnauthorized
	case errors.Is(err, ErrSubdomainTaken):
		return ErrorCodeSubdomainTaken
	case errors.Is(err, ErrSubdomainReserved):
		return ErrorCodeSubdomainReserved
	case errors.Is(err, ErrSubdomainInvalid):
		return ErrorCodeSubdomainInvalid
	case errors.Is(err, ErrRateLimited):
		return ErrorCodeRateLimited
	case errors.Is(err, ErrTunnelLimitReached):
		return ErrorCodeTunnelLimitReached
	default:
		return ErrorCodeInternalError
	}
}

// CodeToError converts an error code to its corresponding error.
func CodeToError(code string) error {
	switch code {
	case ErrorCodeUnauthorized:
		return ErrUnauthorized
	case ErrorCodeSubdomainTaken:
		return ErrSubdomainTaken
	case ErrorCodeSubdomainReserved:
		return ErrSubdomainReserved
	case ErrorCodeSubdomainInvalid:
		return ErrSubdomainInvalid
	case ErrorCodeRateLimited:
		return ErrRateLimited
	case ErrorCodeTunnelLimitReached:
		return ErrTunnelLimitReached
	case ErrorCodeConnectionLimit:
		return ErrTunnelLimitReached
	default:
		return fmt.Errorf("unknown error: %s", code)
	}
}
