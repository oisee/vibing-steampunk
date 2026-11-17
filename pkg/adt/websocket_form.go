package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// FormRequest sends one action to ZADT_VSP's form service (domain "form") and
// returns its data. A system whose ZADT_VSP predates the service answers
// UNKNOWN_DOMAIN, which comes back as an error saying so.
func (c *DebugWebSocketClient) FormRequest(ctx context.Context, action string, params map[string]any) (json.RawMessage, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}
	id := c.GenerateID("form")
	// Replacing an Adobe form deletes, recreates and saves it; a large form
	// takes its time.
	rawMsg := map[string]any{
		"id":      id,
		"domain":  "form",
		"action":  action,
		"params":  params,
		"timeout": 300000,
	}
	resp, err := c.SendRawRequest(ctx, id, rawMsg, 305*time.Second)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		if resp.Error == nil {
			return nil, fmt.Errorf("form service: %s failed", action)
		}
		if strings.EqualFold(resp.Error.Code, "UNKNOWN_DOMAIN") {
			return nil, fmt.Errorf("the ZADT_VSP installed in this system has no form service (version 2.4.0 or later is needed): %s", resp.Error.Message)
		}
		return nil, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Data, nil
}
