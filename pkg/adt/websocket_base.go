package adt

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// BaseWebSocketClient provides common WebSocket functionality for ZADT_VSP connections.
// Embed this in domain-specific clients (Debug, AMDP, etc.).
type BaseWebSocketClient struct {
	baseURL    string
	client     string
	user       string
	password   string
	cookies    map[string]string
	insecure   bool
	clientCert *tls.Certificate // when set, mTLS client-cert auth (no basic auth)
	// clientCertProvider resolves the cert lazily per handshake (wins over clientCert)
	clientCertProvider func() (*tls.Certificate, error)

	conn      *websocket.Conn
	sessionID string
	mu        sync.RWMutex

	// Request/response handling
	msgID     atomic.Int64
	pending   map[string]chan *WSResponse
	pendingMu sync.Mutex

	// Welcome signal
	welcomeCh chan struct{}

	// Connection state
	connected bool

	// Optional callback when connection is lost
	onDisconnect func()
}

// SetCookies makes the client authenticate with a browser session rather than a
// password.
//
// The upgrade that opens a WebSocket is an ordinary HTTP request, so it carries
// a cookie like any other — which is the only thing that lets these features
// reach a system where single sign-on is the way in and no password exists to
// send. Without it the client had exactly one credential it could offer, and
// systems that cannot supply that one were shut out of every WebSocket feature
// for a reason that had nothing to do with the protocol.
func (c *BaseWebSocketClient) SetCookies(cookies map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cookies = cookies
}

// hasCookieAuth reports whether a session was supplied.
func (c *BaseWebSocketClient) hasCookieAuth() bool { return len(c.cookies) > 0 }

// cookieHeader renders the session as a Cookie header value.
func (c *BaseWebSocketClient) cookieHeader() string {
	parts := make([]string, 0, len(c.cookies))
	for name, value := range c.cookies {
		parts = append(parts, name+"="+value)
	}
	// A stable order keeps the header reproducible, which matters when someone
	// is comparing two captures to work out why a logon behaved differently.
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

// applyAuth puts this client's credential on an outgoing request's headers.
func (c *BaseWebSocketClient) applyAuth(header http.Header) {
	if c.hasCookieAuth() {
		header.Set("Cookie", c.cookieHeader())
		return
	}
	header.Set("Authorization", basicAuth(c.user, c.password))
}

// NewBaseWebSocketClient creates a new base WebSocket client.
func NewBaseWebSocketClient(baseURL, client, user, password string, insecure bool) *BaseWebSocketClient {
	return &BaseWebSocketClient{
		baseURL:   baseURL,
		client:    client,
		user:      user,
		password:  password,
		insecure:  insecure,
		pending:   make(map[string]chan *WSResponse),
		welcomeCh: make(chan struct{}, 1),
	}
}

// SetClientCert enables TLS client-certificate (mTLS) auth for the WebSocket
// bridge. When set, no basic-auth header is sent and TLS is capped at 1.2.
func (c *BaseWebSocketClient) SetClientCert(cert *tls.Certificate) {
	c.clientCert = cert
}

// SetClientCertProvider is SetClientCert with lazy per-handshake resolution
// (takes precedence over SetClientCert).
func (c *BaseWebSocketClient) SetClientCertProvider(p func() (*tls.Certificate, error)) {
	c.clientCertProvider = p
}

// certMode reports whether cert auth is configured (static or lazy).
func (c *BaseWebSocketClient) certMode() bool {
	return c.clientCert != nil || c.clientCertProvider != nil
}

// Connect establishes WebSocket connection to ZADT_VSP.
func (c *BaseWebSocketClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.conn != nil {
		c.mu.Unlock()
		return fmt.Errorf("already connected")
	}

	// Build WebSocket URL
	u, err := url.Parse(c.baseURL)
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("invalid base URL: %w", err)
	}

	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/sap/bc/apc/sap/zadt_vsp?sap-client=%s", scheme, u.Host, c.client)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.insecure,
	}
	if c.clientCertProvider != nil {
		// mTLS, lazy: resolve at handshake time; 7.50 ICM drops TLS 1.3 client certs
		provider := c.clientCertProvider
		tlsConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return provider()
		}
		tlsConfig.MaxVersion = tls.VersionTLS12
	} else if c.clientCert != nil {
		// mTLS: present the client cert; 7.50 ICM drops TLS 1.3 client-cert handshakes
		tlsConfig.Certificates = []tls.Certificate{*c.clientCert}
		tlsConfig.MaxVersion = tls.VersionTLS12
	}

	header := http.Header{}
	if !c.certMode() {
		// no basic auth in cert mode — the certificate authenticates the user
		c.applyAuth(header)
	}

	// Try 1: Direct Basic Auth (works on most SAP systems)
	dialer := newWebSocketDialer(tlsConfig)

	conn, resp, err := dialer.DialContext(ctx, wsURL, header)

	// Try 2: If 401, pre-authenticate to get session cookies first.
	// Some SAP systems reject standalone Basic Auth on WebSocket upgrade
	// but accept it on regular HTTP to issue session cookies.
	if err != nil && resp != nil && resp.StatusCode == http.StatusUnauthorized && !c.hasCookieAuth() {
		jar, _ := cookiejar.New(nil)
		preAuthClient := &http.Client{
			Jar: jar,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
				Proxy:           http.ProxyFromEnvironment,
			},
			Timeout: 30 * time.Second,
		}

		authURL := fmt.Sprintf("%s/sap/bc/adt/core/discovery?sap-client=%s", c.baseURL, c.client)
		authReq, authErr := http.NewRequestWithContext(ctx, http.MethodGet, authURL, nil)
		if authErr != nil {
			c.mu.Unlock()
			return fmt.Errorf("WebSocket connection failed (HTTP 401), pre-auth setup error: %w", authErr)
		}
		if !c.certMode() {
			// cert mode: the TLS client cert on preAuthClient authenticates the
			// user; a basic-auth header with the empty password would 401.
			authReq.SetBasicAuth(c.user, c.password)
		}

		authResp, authErr := preAuthClient.Do(authReq)
		if authErr != nil {
			c.mu.Unlock()
			return fmt.Errorf("WebSocket connection failed (HTTP 401), pre-auth failed: %w", authErr)
		}
		io.Copy(io.Discard, authResp.Body)
		authResp.Body.Close()

		if authResp.StatusCode == http.StatusUnauthorized {
			c.mu.Unlock()
			return fmt.Errorf("WebSocket authentication failed: 401 Unauthorized (check credentials)")
		}

		// Retry with session cookies (no Basic Auth header — use cookies only)
		dialer.Jar = jar
		conn, resp, err = dialer.DialContext(ctx, wsURL, nil)
	}

	if err != nil {
		c.mu.Unlock()
		if resp != nil {
			return fmt.Errorf("WebSocket connection failed (HTTP %d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("WebSocket connection failed: %w", err)
	}

	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	// Start message reader goroutine
	go c.readMessages()

	// Wait for welcome message
	select {
	case <-c.welcomeCh:
		return nil
	case <-time.After(5 * time.Second):
		c.Close()
		return fmt.Errorf("timeout waiting for welcome message")
	case <-ctx.Done():
		c.Close()
		return ctx.Err()
	}
}

// Close closes the WebSocket connection.
func (c *BaseWebSocketClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.connected = false
		return err
	}
	return nil
}

// IsConnected returns whether the client is connected.
func (c *BaseWebSocketClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetUser returns the username.
func (c *BaseWebSocketClient) GetUser() string {
	return c.user
}

// readMessages reads messages from WebSocket and routes them.
func (c *BaseWebSocketClient) readMessages() {
	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			return
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			c.conn = nil
			c.connected = false
			onDisconnect := c.onDisconnect
			c.mu.Unlock()

			// Call disconnect callback if set
			if onDisconnect != nil {
				onDisconnect()
			}
			return
		}

		var resp WSResponse
		if err := json.Unmarshal(message, &resp); err != nil {
			continue
		}

		// Check if this is a response to a pending request
		c.pendingMu.Lock()
		if ch, ok := c.pending[resp.ID]; ok {
			ch <- &resp
			delete(c.pending, resp.ID)
			c.pendingMu.Unlock()
			continue
		}
		c.pendingMu.Unlock()

		// Handle welcome message
		if resp.ID == "welcome" {
			var welcomeData struct {
				Session string   `json:"session"`
				Version string   `json:"version"`
				Domains []string `json:"domains"`
			}
			if err := json.Unmarshal(resp.Data, &welcomeData); err == nil {
				c.mu.Lock()
				c.sessionID = welcomeData.Session
				c.mu.Unlock()
			}
			select {
			case c.welcomeCh <- struct{}{}:
			default:
			}
		}
	}
}

// SendDomainRequest sends a request to any domain and waits for response.
func (c *BaseWebSocketClient) SendDomainRequest(ctx context.Context, domain, action string, params map[string]any, timeout time.Duration) (*WSResponse, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	id := fmt.Sprintf("%s_%d", domain, c.msgID.Add(1))

	msg := WSMessage{
		ID:      id,
		Domain:  domain,
		Action:  action,
		Params:  params,
		Timeout: int(timeout.Milliseconds()),
	}

	respCh := make(chan *WSResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	c.mu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, data)
	c.mu.Unlock()
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-time.After(timeout):
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("request timeout")
	}
}

// SendRawRequest sends a raw message (for domains that use different format).
func (c *BaseWebSocketClient) SendRawRequest(ctx context.Context, id string, rawMsg map[string]any, timeout time.Duration) (*WSResponse, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	respCh := make(chan *WSResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	data, err := json.Marshal(rawMsg)
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	c.mu.Lock()
	err = c.conn.WriteMessage(websocket.TextMessage, data)
	c.mu.Unlock()
	if err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-time.After(timeout):
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("request timeout")
	}
}

// GenerateID generates a unique message ID for a domain.
func (c *BaseWebSocketClient) GenerateID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, c.msgID.Add(1))
}

// RegisterPending registers a channel for a pending request.
func (c *BaseWebSocketClient) RegisterPending(id string, ch chan *WSResponse) {
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()
}

// UnregisterPending removes a pending request channel.
func (c *BaseWebSocketClient) UnregisterPending(id string) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

// WriteMessage writes a message to the WebSocket connection.
func (c *BaseWebSocketClient) WriteMessage(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// basicAuth creates basic auth header value.
func basicAuth(user, password string) string {
	auth := user + ":" + password
	return "Basic " + base64Encode([]byte(auth))
}

// base64Encode encodes bytes to base64 string.
func base64Encode(data []byte) string {
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result []byte
	for i := 0; i < len(data); i += 3 {
		var b uint32
		remaining := len(data) - i
		if remaining >= 3 {
			b = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
			result = append(result, base64Chars[b>>18&0x3F], base64Chars[b>>12&0x3F], base64Chars[b>>6&0x3F], base64Chars[b&0x3F])
		} else if remaining == 2 {
			b = uint32(data[i])<<16 | uint32(data[i+1])<<8
			result = append(result, base64Chars[b>>18&0x3F], base64Chars[b>>12&0x3F], base64Chars[b>>6&0x3F], '=')
		} else {
			b = uint32(data[i]) << 16
			result = append(result, base64Chars[b>>18&0x3F], base64Chars[b>>12&0x3F], '=', '=')
		}
	}
	return string(result)
}

// newWebSocketDialer builds the dialer used for the ZADT_VSP upgrade. It honours
// HTTP_PROXY/HTTPS_PROXY/NO_PROXY exactly as the ADT HTTP client does — behind a
// corporate proxy the upgrade used to fail with no hint why, while every plain
// HTTP call went through.
func newWebSocketDialer(tlsConfig *tls.Config) websocket.Dialer {
	return websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
		TLSClientConfig:  tlsConfig,
		Proxy:            http.ProxyFromEnvironment,
	}
}
