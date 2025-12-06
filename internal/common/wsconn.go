package common

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSConn wraps a websocket.Conn to implement net.Conn interface.
// This allows yamux to work over WebSocket connections.
type WSConn struct {
	ws     *websocket.Conn
	reader io.Reader
	mu     sync.Mutex
}

// NewWSConn creates a new WSConn wrapper.
func NewWSConn(ws *websocket.Conn) *WSConn {
	return &WSConn{
		ws: ws,
	}
}

// Read reads data from the WebSocket connection.
func (c *WSConn) Read(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.reader == nil {
		_, reader, err := c.ws.NextReader()
		if err != nil {
			return 0, err
		}
		c.reader = reader
	}

	n, err := c.reader.Read(b)
	if err == io.EOF {
		c.reader = nil
		return n, nil
	}
	return n, err
}

// Write writes data to the WebSocket connection.
func (c *WSConn) Write(b []byte) (int, error) {
	err := c.ws.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// Close closes the WebSocket connection.
func (c *WSConn) Close() error {
	return c.ws.Close()
}

// LocalAddr returns the local network address.
func (c *WSConn) LocalAddr() net.Addr {
	return c.ws.LocalAddr()
}

// RemoteAddr returns the remote network address.
func (c *WSConn) RemoteAddr() net.Addr {
	return c.ws.RemoteAddr()
}

// SetDeadline sets the read and write deadlines.
func (c *WSConn) SetDeadline(t time.Time) error {
	if err := c.ws.SetReadDeadline(t); err != nil {
		return err
	}
	return c.ws.SetWriteDeadline(t)
}

// SetReadDeadline sets the read deadline.
func (c *WSConn) SetReadDeadline(t time.Time) error {
	return c.ws.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline.
func (c *WSConn) SetWriteDeadline(t time.Time) error {
	return c.ws.SetWriteDeadline(t)
}
