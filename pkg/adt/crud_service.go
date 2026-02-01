// ABOUTME: Service binding publish/unpublish operations.
// ABOUTME: Provides PublishServiceBinding and UnpublishServiceBinding for OData services.

package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
)

// PublishResult represents the result of a publish/unpublish operation.
type PublishResult struct {
	Severity  string `json:"severity"`
	ShortText string `json:"shortText"`
	LongText  string `json:"longText"`
}

// PublishServiceBinding publishes a service binding to make it available as OData service.
// serviceName is the service binding name (e.g., "ZTRAVEL_SB")
// serviceVersion is typically "0001"
func (c *Client) PublishServiceBinding(ctx context.Context, serviceName string, serviceVersion string) (*PublishResult, error) {
	return c.publishUnpublishServiceBinding(ctx, "publishjobs", serviceName, serviceVersion)
}

// UnpublishServiceBinding unpublishes a service binding.
func (c *Client) UnpublishServiceBinding(ctx context.Context, serviceName string, serviceVersion string) (*PublishResult, error) {
	return c.publishUnpublishServiceBinding(ctx, "unpublishjobs", serviceName, serviceVersion)
}

func (c *Client) publishUnpublishServiceBinding(ctx context.Context, action, serviceName, serviceVersion string) (*PublishResult, error) {
	if serviceVersion == "" {
		serviceVersion = "0001"
	}

	params := url.Values{}
	params.Set("servicename", serviceName)
	params.Set("serviceversion", serviceVersion)

	body := fmt.Sprintf(`<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:name="%s"/>
</adtcore:objectReferences>`, serviceName)

	path := fmt.Sprintf("/sap/bc/adt/businessservices/odatav2/%s", action)

	resp, err := c.transport.Request(ctx, path, &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Body:        []byte(body),
		ContentType: "application/*",
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("%s service binding: %w", action, err)
	}

	return parsePublishResult(resp.Body)
}

func parsePublishResult(data []byte) (*PublishResult, error) {
	type publishData struct {
		Severity  string `xml:"SEVERITY"`
		ShortText string `xml:"SHORT_TEXT"`
		LongText  string `xml:"LONG_TEXT"`
	}
	type values struct {
		Data publishData `xml:"DATA"`
	}
	type abapResponse struct {
		Values values `xml:"values"`
	}

	var resp abapResponse
	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing publish response: %w", err)
	}

	return &PublishResult{
		Severity:  resp.Values.Data.Severity,
		ShortText: resp.Values.Data.ShortText,
		LongText:  resp.Values.Data.LongText,
	}, nil
}
