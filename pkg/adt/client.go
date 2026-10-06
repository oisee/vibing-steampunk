package adt

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Client is the main ADT API client.
type Client struct {
	transport *Transport
	config    *Config

	// Keep-alive goroutine management
	keepAliveCancel context.CancelFunc
	keepAliveDone   chan struct{}
	keepAliveMu     sync.Mutex

	// Lock handles believed outstanding, so the keep-alive ping does not
	// retire the session one of them is bound to. See lock_window.go.
	locks lockWindow

	// rfcFetcherFactory, when non-nil, overrides the default WebSocket-backed
	// RFC source fetcher used by GetEnhancement's fallback path. Production
	// callers leave this nil; tests inject a stub to avoid opening a real
	// WebSocket. See enhancements.go.
	rfcFetcherFactory func(ctx context.Context) (rfcSourceFetcher, error)
}

// NewClient creates a new ADT client with the given configuration.
func NewClient(baseURL, username, password string, opts ...Option) *Client {
	cfg := NewConfig(baseURL, username, password, opts...)
	return newClient(cfg, NewTransport(cfg))
}

// newClient wires a client to its transport, including the lock window the
// transport consults before reloading a cookie file and while routing
// stateless requests.
func newClient(cfg *Config, transport *Transport) *Client {
	c := &Client{
		transport: transport,
		config:    cfg,
	}
	if transport != nil {
		transport.lockOutstanding = c.lockOutstanding
		// The transport keeps stateless requests out of the context a lock
		// handle is bound to while one is outstanding (see Transport.do).
		transport.locks = &c.locks
		// The pin's preflight can fall back to SQL and the transport
		// organizer, which only the client can ask.
		if transport.identity != nil {
			transport.identity.probe = c.probeIdentity
		}
	}
	return c
}

// NewClientWithTransport creates a new client with a custom transport.
// This is useful for testing.
func NewClientWithTransport(cfg *Config, transport *Transport) *Client {
	return newClient(cfg, transport)
}

// StartKeepAlive starts a background goroutine that periodically pings the SAP server
// to keep the session alive. This is especially useful for cookie/browser-auth sessions
// which can time out during idle periods. The interval should be shorter than the SAP
// server's session timeout. A reasonable default is 5 minutes.
// Calling StartKeepAlive again stops any existing keep-alive before starting a new one.
func (c *Client) StartKeepAlive(interval time.Duration, verbose bool) {
	c.keepAliveMu.Lock()
	defer c.keepAliveMu.Unlock()

	// Stop existing keep-alive if running
	c.stopKeepAliveLocked()

	ctx, cancel := context.WithCancel(context.Background())
	c.keepAliveCancel = cancel
	c.keepAliveDone = make(chan struct{})

	go func() {
		defer close(c.keepAliveDone)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		if verbose {
			fmt.Fprintf(LogOutput, "[KEEPALIVE] Started (interval: %s)\n", interval)
		}

		for {
			select {
			case <-ctx.Done():
				if verbose {
					fmt.Fprintf(LogOutput, "[KEEPALIVE] Stopped\n")
				}
				return
			case <-ticker.C:
				// A ping is an ordinary request, and an ordinary request is
				// stamped stateless — which retires the session a lock handle
				// lives in. Skipping a tick costs nothing; sending it during a
				// write costs the write (#168).
				if c.lockOutstanding() {
					if verbose {
						fmt.Fprintf(LogOutput, "[KEEPALIVE] Skipped: a lock is outstanding\n")
					}
					continue
				}
				if err := c.transport.Ping(ctx); err != nil {
					if ctx.Err() != nil {
						return // context cancelled, expected
					}
					if verbose {
						fmt.Fprintf(LogOutput, "[KEEPALIVE] Ping failed: %v\n", err)
					}
				} else if verbose {
					fmt.Fprintf(LogOutput, "[KEEPALIVE] Ping OK\n")
				}
			}
		}
	}()
}

// StopKeepAlive stops the background keep-alive goroutine if running.
func (c *Client) StopKeepAlive() {
	c.keepAliveMu.Lock()
	defer c.keepAliveMu.Unlock()
	c.stopKeepAliveLocked()
}

// stopKeepAliveLocked stops the keep-alive goroutine. Must be called with keepAliveMu held.
func (c *Client) stopKeepAliveLocked() {
	if c.keepAliveCancel != nil {
		c.keepAliveCancel()
		<-c.keepAliveDone
		c.keepAliveCancel = nil
		c.keepAliveDone = nil
	}
}

// Safety returns the safety configuration for checking transport operations.
func (c *Client) Safety() *SafetyConfig {
	return &c.config.Safety
}

// Language is the logon language the client was configured with, as an ISO
// code ("EN") or a SAP key; empty when none was set.
func (c *Client) Language() string {
	if c.config == nil {
		return ""
	}
	return c.config.Language
}

// CloseTransport ends the helper process of a transport command, if this
// client has one (see Config.TransportCmd): its stdin is closed, and it is
// killed if it has not exited three seconds later. Every later request fails.
// Without a transport command it does nothing.
func (c *Client) CloseTransport() error {
	if c == nil || c.transport == nil {
		return nil
	}
	return c.transport.CloseTransport()
}
