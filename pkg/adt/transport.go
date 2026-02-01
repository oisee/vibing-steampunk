// ABOUTME: Transport management operations for SAP CTS.
// ABOUTME: Create, list, release, and delete transport requests.

package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// --- Transport Operations ---

// GetUserTransports retrieves all transport requests for a user.
// Returns both workbench and customizing requests grouped by target system.
func (c *Client) GetUserTransports(ctx context.Context, userName string) (*UserTransports, error) {
	if err := c.config.Safety.CheckTransport("", "GetUserTransports", false); err != nil {
		return nil, err
	}

	userName = strings.ToUpper(userName)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/cts/transportrequests", &RequestOptions{
		Method: http.MethodGet,
		Query:  map[string][]string{"user": {userName}, "targets": {"true"}},
	})
	if err != nil {
		return nil, fmt.Errorf("get user transports failed: %w", err)
	}

	return parseUserTransports(resp.Body)
}

func parseUserTransports(data []byte) (*UserTransports, error) {
	xmlStr := StripXMLNamespaces(string(data), "tm:", "atom:")

	type transportObject struct {
		PgmID   string `xml:"pgmid,attr"`
		Type    string `xml:"type,attr"`
		Name    string `xml:"name,attr"`
		ObjInfo string `xml:"obj_info,attr"`
	}
	type task struct {
		Number  string            `xml:"number,attr"`
		Owner   string            `xml:"owner,attr"`
		Desc    string            `xml:"desc,attr"`
		Status  string            `xml:"status,attr"`
		Objects []transportObject `xml:"abap_object"`
	}
	type request struct {
		Number string `xml:"number,attr"`
		Owner  string `xml:"owner,attr"`
		Desc   string `xml:"desc,attr"`
		Status string `xml:"status,attr"`
		Tasks  []task `xml:"task"`
	}
	type target struct {
		Name       string `xml:"name,attr"`
		Modifiable struct {
			Requests []request `xml:"request"`
		} `xml:"modifiable"`
		Released struct {
			Requests []request `xml:"request"`
		} `xml:"released"`
	}
	type root struct {
		Workbench struct {
			Targets []target `xml:"target"`
		} `xml:"workbench"`
		Customizing struct {
			Targets []target `xml:"target"`
		} `xml:"customizing"`
	}

	var resp root
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing transport list: %w", err)
	}

	convertRequests := func(reqs []request, targetName, transportType string) []TransportSummary {
		var result []TransportSummary
		for _, r := range reqs {
			tr := TransportSummary{
				Number:      r.Number,
				Owner:       r.Owner,
				Description: r.Desc,
				Status:      r.Status,
				Target:      targetName,
				Type:        transportType,
			}
			for _, t := range r.Tasks {
				task := TransportTask{
					Number:      t.Number,
					Owner:       t.Owner,
					Description: t.Desc,
					Status:      t.Status,
				}
				for _, o := range t.Objects {
					task.Objects = append(task.Objects, TransportObject{
						PgmID: o.PgmID,
						Type:  o.Type,
						Name:  o.Name,
						Info:  o.ObjInfo,
					})
				}
				tr.Tasks = append(tr.Tasks, task)
			}
			result = append(result, tr)
		}
		return result
	}

	result := &UserTransports{}

	for _, t := range resp.Workbench.Targets {
		result.Workbench = append(result.Workbench, convertRequests(t.Modifiable.Requests, t.Name, "workbench")...)
		result.Workbench = append(result.Workbench, convertRequests(t.Released.Requests, t.Name, "workbench")...)
	}

	for _, t := range resp.Customizing.Targets {
		result.Customizing = append(result.Customizing, convertRequests(t.Modifiable.Requests, t.Name, "customizing")...)
	}

	return result, nil
}

// GetTransportInfo retrieves transport information for an object.
// Returns available transports and whether the object is locked.
func (c *Client) GetTransportInfo(ctx context.Context, objectURL string, devClass string) (*TransportInfo, error) {
	if err := c.config.Safety.CheckTransport("", "GetTransportInfo", false); err != nil {
		return nil, err
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <DEVCLASS>%s</DEVCLASS>
      <OPERATION>I</OPERATION>
      <URI>%s</URI>
    </DATA>
  </asx:values>
</asx:abap>`, devClass, objectURL)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/cts/transportchecks", &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/vnd.sap.as+xml; charset=UTF-8; dataname=com.sap.adt.transport.service.checkData",
		Accept:      "application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.transport.service.checkData",
	})
	if err != nil {
		return nil, fmt.Errorf("get transport info failed: %w", err)
	}

	return parseTransportInfo(resp.Body)
}

func parseTransportInfo(data []byte) (*TransportInfo, error) {
	type dataType struct {
		PgmID      string `xml:"PGMID"`
		Object     string `xml:"OBJECT"`
		ObjectName string `xml:"OBJECTNAME"`
		Operation  string `xml:"OPERATION"`
		DevClass   string `xml:"DEVCLASS"`
		Recording  string `xml:"RECORDING"`
	}
	type values struct {
		Data dataType `xml:"DATA"`
	}
	type abap struct {
		Values values `xml:"values"`
	}

	xmlStr := StripXMLNamespaces(string(data), "asx:")

	var resp abap
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing transport info: %w", err)
	}

	return &TransportInfo{
		PgmID:      resp.Values.Data.PgmID,
		Object:     resp.Values.Data.Object,
		ObjectName: resp.Values.Data.ObjectName,
		Operation:  resp.Values.Data.Operation,
		DevClass:   resp.Values.Data.DevClass,
		Recording:  resp.Values.Data.Recording,
	}, nil
}

// ListTransports returns transport requests for a user.
func (c *Client) ListTransports(ctx context.Context, user string) ([]TransportSummary, error) {
	if err := c.config.Safety.CheckTransport("", "ListTransports", false); err != nil {
		return nil, err
	}

	if user == "" {
		user = c.config.Username
	}

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/cts/transportrequests", &RequestOptions{
		Method: http.MethodGet,
		Query:  map[string][]string{"user": {strings.ToUpper(user)}},
		Accept: "application/vnd.sap.adt.transportorganizertree.v1+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("listing transports: %w", err)
	}

	return parseTransportList(resp.Body)
}

func parseTransportList(data []byte) ([]TransportSummary, error) {
	xmlStr := StripXMLNamespaces(string(data), "tm:")

	type request struct {
		Number      string `xml:"number,attr"`
		Owner       string `xml:"owner,attr"`
		Desc        string `xml:"desc,attr"`
		Type        string `xml:"type,attr"`
		Status      string `xml:"status,attr"`
		StatusText  string `xml:"status_text,attr"`
		Target      string `xml:"target,attr"`
		TargetDesc  string `xml:"target_desc,attr"`
		LastChanged string `xml:"lastchanged_timestamp,attr"`
		Client      string `xml:"source_client,attr"`
	}
	type root struct {
		Requests []request `xml:"request"`
	}

	var resp root
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing transport list: %w", err)
	}

	var transports []TransportSummary
	for _, req := range resp.Requests {
		transports = append(transports, TransportSummary{
			Number:      req.Number,
			Owner:       req.Owner,
			Description: req.Desc,
			Type:        req.Type,
			Status:      req.Status,
			StatusText:  req.StatusText,
			Target:      req.Target,
			TargetDesc:  req.TargetDesc,
			ChangedAt:   req.LastChanged,
			Client:      req.Client,
		})
	}

	return transports, nil
}

// GetTransport returns detailed transport information.
func (c *Client) GetTransport(ctx context.Context, number string) (*TransportDetails, error) {
	if err := c.config.Safety.CheckTransport(number, "GetTransport", false); err != nil {
		return nil, err
	}

	if number == "" {
		return nil, fmt.Errorf("transport number is required")
	}

	path := fmt.Sprintf("/sap/bc/adt/cts/transportrequests/%s", strings.ToUpper(number))

	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.transportorganizer.v1+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting transport %s: %w", number, err)
	}

	return parseTransportDetail(resp.Body)
}

// parsePosition parses a position string to int, returning 0 on failure.
func parsePosition(s string) int {
	if s == "" {
		return 0
	}
	var pos int
	_, _ = fmt.Sscanf(s, "%d", &pos)
	return pos
}

func parseTransportDetail(data []byte) (*TransportDetails, error) {
	xmlStr := StripXMLNamespaces(string(data), "tm:")

	type abapObject struct {
		PgmID    string `xml:"pgmid,attr"`
		Type     string `xml:"type,attr"`
		Name     string `xml:"name,attr"`
		WBType   string `xml:"wbtype,attr"`
		ObjInfo  string `xml:"obj_info,attr"`
		Position string `xml:"position,attr"`
	}
	type task struct {
		Number      string       `xml:"number,attr"`
		Parent      string       `xml:"parent,attr"`
		Owner       string       `xml:"owner,attr"`
		Desc        string       `xml:"desc,attr"`
		Type        string       `xml:"type,attr"`
		Status      string       `xml:"status,attr"`
		StatusText  string       `xml:"status_text,attr"`
		LastChanged string       `xml:"lastchanged_timestamp,attr"`
		Objects     []abapObject `xml:"abap_object"`
	}
	type request struct {
		Number      string       `xml:"number,attr"`
		Owner       string       `xml:"owner,attr"`
		Desc        string       `xml:"desc,attr"`
		Type        string       `xml:"type,attr"`
		Status      string       `xml:"status,attr"`
		StatusText  string       `xml:"status_text,attr"`
		Target      string       `xml:"target,attr"`
		TargetDesc  string       `xml:"target_desc,attr"`
		Client      string       `xml:"source_client,attr"`
		LastChanged string       `xml:"lastchanged_timestamp,attr"`
		Objects     []abapObject `xml:"abap_object"`
		AllObjects  struct {
			Objects []abapObject `xml:"abap_object"`
		} `xml:"all_objects"`
		Tasks []task `xml:"task"`
	}
	type root struct {
		Request *request `xml:"request"`
	}

	var resp root
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing transport: %w", err)
	}

	if resp.Request == nil {
		return nil, fmt.Errorf("transport not found in response")
	}

	req := resp.Request
	t := &TransportDetails{
		TransportSummary: TransportSummary{
			Number:      req.Number,
			Owner:       req.Owner,
			Description: req.Desc,
			Type:        req.Type,
			Status:      req.Status,
			StatusText:  req.StatusText,
			Target:      req.Target,
			TargetDesc:  req.TargetDesc,
			ChangedAt:   req.LastChanged,
			Client:      req.Client,
		},
	}

	// Convert objects - prefer AllObjects if available
	objects := req.Objects
	if len(req.AllObjects.Objects) > 0 {
		objects = req.AllObjects.Objects
	}
	for _, obj := range objects {
		t.Objects = append(t.Objects, TransportObject{
			PgmID:    obj.PgmID,
			Type:     obj.Type,
			Name:     obj.Name,
			WBType:   obj.WBType,
			Info:     obj.ObjInfo,
			Position: parsePosition(obj.Position),
		})
	}

	// Convert tasks
	for _, task := range req.Tasks {
		tt := TransportTask{
			Number:      task.Number,
			Parent:      task.Parent,
			Owner:       task.Owner,
			Description: task.Desc,
			Type:        task.Type,
			Status:      task.Status,
			StatusText:  task.StatusText,
		}
		for _, obj := range task.Objects {
			tt.Objects = append(tt.Objects, TransportObject{
				PgmID:    obj.PgmID,
				Type:     obj.Type,
				Name:     obj.Name,
				WBType:   obj.WBType,
				Info:     obj.ObjInfo,
				Position: parsePosition(obj.Position),
			})
		}
		t.Tasks = append(t.Tasks, tt)
	}

	return t, nil
}

// CreateTransport creates a new transport request.
func (c *Client) CreateTransport(ctx context.Context, opts CreateTransportOptions) (string, error) {
	if err := c.config.Safety.CheckTransport("", "CreateTransport", true); err != nil {
		return "", err
	}

	if opts.Description == "" {
		return "", fmt.Errorf("description is required")
	}
	if opts.Package == "" {
		return "", fmt.Errorf("package is required")
	}

	// Default to workbench request
	reqType := "K"
	if strings.ToLower(opts.Type) == "customizing" {
		reqType = "W"
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<tm:root xmlns:tm="http://www.sap.com/cts/adt/tm">
  <tm:request tm:desc="%s" tm:type="%s" tm:target="" tm:cts_project="">
    <tm:abap_object tm:pgmid="R3TR" tm:type="DEVC" tm:name="%s"/>
  </tm:request>
</tm:root>`,
		escapeXMLAttr(opts.Description),
		reqType,
		strings.ToUpper(opts.Package))

	query := make(map[string][]string)
	if opts.TransportLayer != "" {
		query["transportLayer"] = []string{opts.TransportLayer}
	}

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/cts/transports", &RequestOptions{
		Method:      http.MethodPost,
		Query:       query,
		Body:        []byte(body),
		ContentType: "application/vnd.sap.as+xml",
		Accept:      "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("creating transport: %w", err)
	}

	return strings.TrimSpace(string(resp.Body)), nil
}

// ReleaseTransport releases a transport request.
func (c *Client) ReleaseTransport(ctx context.Context, number string, opts ReleaseTransportOptions) error {
	if err := c.config.Safety.CheckTransport(number, "ReleaseTransport", true); err != nil {
		return err
	}

	if number == "" {
		return fmt.Errorf("transport number is required")
	}

	action := opts.GetAction()
	path := fmt.Sprintf("/sap/bc/adt/cts/transportrequests/%s/%s", strings.ToUpper(number), action)

	_, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodPost,
		Accept: "application/vnd.sap.adt.transportorganizer.v1+xml",
	})
	if err != nil {
		return fmt.Errorf("releasing transport %s: %w", number, err)
	}

	return nil
}

// DeleteTransport deletes a transport request.
func (c *Client) DeleteTransport(ctx context.Context, number string) error {
	if err := c.config.Safety.CheckTransport(number, "DeleteTransport", true); err != nil {
		return err
	}

	if number == "" {
		return fmt.Errorf("transport number is required")
	}

	path := fmt.Sprintf("/sap/bc/adt/cts/transportrequests/%s", strings.ToUpper(number))

	_, err := c.transport.Request(ctx, path, &RequestOptions{
		Method: http.MethodDelete,
		Accept: "application/vnd.sap.adt.transportorganizer.v1+xml",
	})
	if err != nil {
		return fmt.Errorf("deleting transport %s: %w", number, err)
	}

	return nil
}

// escapeXMLAttr is defined in ui5.go
