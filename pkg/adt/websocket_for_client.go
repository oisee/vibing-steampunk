package adt

// The ZADT_VSP WebSocket clients, built from the ADT client they work beside.
//
// Every caller used to assemble one by hand from whatever configuration it had
// to hand -- a URL, a user, a password -- and then, sometimes, a cookie map.
// Which cookie map differed from caller to caller: the global CLI config (empty
// for every profile that signs in with a cookie_file, a cookie_string or single
// sign-on), the server config captured at startup (dead once the session is
// renewed), or nothing at all. Each of those reached ZADT_VSP with a password
// the profile did not have and failed with 401 while the ADT calls beside it
// worked.
//
// Building the WebSocket from the ADT client removes the choice: it carries
// the same system, client and TLS setting, and the session that client is
// using at this moment -- including the last refresh one of its HTTP calls
// made. Building a WebSocket does not re-authenticate; a session that lapsed
// since the last HTTP call is still lapsed. With a session, the password is
// left out, so the upgrade carries cookies or basic auth, never both.

// wsCredentials is how a WebSocket built for c authenticates: the session c
// holds now, or, without one, c's user and password.
func (c *Client) wsCredentials() (user, password string, cookies map[string]string) {
	cookies = c.CurrentCookies()
	if len(cookies) > 0 {
		return c.config.Username, "", cookies
	}
	return c.config.Username, c.config.Password, nil
}

// NewDebugWebSocketClient returns an unconnected ZADT_VSP WebSocket client for
// the system c talks to, authenticated as c is.
func (c *Client) NewDebugWebSocketClient() *DebugWebSocketClient {
	user, password, cookies := c.wsCredentials()
	ws := NewDebugWebSocketClient(c.config.BaseURL, c.config.Client, user, password, c.config.InsecureSkipVerify)
	if len(cookies) > 0 {
		ws.SetCookies(cookies)
	}
	ws.verify = c.VerifyIdentity
	return ws
}

// NewAMDPWebSocketClient returns an unconnected ZADT_VSP AMDP WebSocket client
// for the system c talks to, authenticated as c is.
func (c *Client) NewAMDPWebSocketClient() *AMDPWebSocketClient {
	user, password, cookies := c.wsCredentials()
	ws := NewAMDPWebSocketClient(c.config.BaseURL, c.config.Client, user, password, c.config.InsecureSkipVerify)
	if len(cookies) > 0 {
		ws.SetCookies(cookies)
	}
	ws.verify = c.VerifyIdentity
	return ws
}
