package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DomainFixValue is one optional fixed value or interval on a DDIC domain.
type DomainFixValue struct {
	Low  string `json:"low"`
	High string `json:"high,omitempty"`
	Text string `json:"text,omitempty"`
}

type DomainCreateOptions struct {
	Name           string
	Description    string
	PackageName    string
	Transport      string
	DataType       string
	Length         int
	Decimals       int
	OutputLength   int
	OutputStyle    string
	ConversionExit string
	SignExists     bool
	Lowercase      bool
	AmpmFormat     bool
	ValueTableRef  string
	AppendExists   bool
	FixValues      []DomainFixValue
}

type DataElementFieldLabels struct {
	Short   string `json:"short"`
	Medium  string `json:"medium"`
	Long    string `json:"long"`
	Heading string `json:"heading"`
}

type DataElementCreateOptions struct {
	Name                    string
	Description             string
	PackageName             string
	Transport               string
	Domain                  string
	DataType                string
	Length                  int
	Decimals                int
	Labels                  DataElementFieldLabels
	SearchHelp              string
	SearchHelpParameter     string
	SetGetParameter         string
	DefaultComponentName    string
	DeactivateInputHistory  bool
	ChangeDocument          bool
	LeftToRightDirection    bool
	DeactivateBIDIFiltering bool
}

type DDICCreateResult struct {
	Success    bool              `json:"success"`
	ObjectType string            `json:"objectType"`
	ObjectName string            `json:"objectName"`
	ObjectURL  string            `json:"objectUrl"`
	Activation *ActivationResult `json:"activation,omitempty"`
	Message    string            `json:"message,omitempty"`
}

func (c *Client) CreateDomain(ctx context.Context, opts DomainCreateOptions) (*DDICCreateResult, error) {
	if err := validateDomainCreateOptions(&opts); err != nil {
		return nil, err
	}
	result := &DDICCreateResult{ObjectType: string(ObjectTypeDomain), ObjectName: strings.ToUpper(opts.Name), ObjectURL: GetObjectURL(ObjectTypeDomain, opts.Name, "")}
	if err := c.createDDICPrimitive(ctx, CreateObjectOptions{
		ObjectType: ObjectTypeDomain, Name: opts.Name, Description: opts.Description,
		PackageName: opts.PackageName, Transport: opts.Transport,
	}, result, func(lockHandle string) error {
		return c.setDomainProperties(ctx, result.ObjectURL, opts, lockHandle)
	}, func() error {
		return c.verifyDomainProperties(ctx, result.ObjectURL, opts)
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) CreateDataElement(ctx context.Context, opts DataElementCreateOptions) (*DDICCreateResult, error) {
	if err := validateDataElementCreateOptions(&opts); err != nil {
		return nil, err
	}
	result := &DDICCreateResult{ObjectType: string(ObjectTypeDataElement), ObjectName: strings.ToUpper(opts.Name), ObjectURL: GetObjectURL(ObjectTypeDataElement, opts.Name, "")}
	if opts.Domain != "" {
		matches, err := c.SearchObjectByType(ctx, strings.ToUpper(opts.Domain), string(ObjectTypeDomain), 10)
		if err != nil {
			return nil, fmt.Errorf("checking referenced domain %s: %w", opts.Domain, err)
		}
		found := false
		for _, match := range matches {
			if strings.EqualFold(match.Name, opts.Domain) && strings.EqualFold(match.Type, string(ObjectTypeDomain)) {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("referenced domain %s was not found", opts.Domain)
		}
	}
	if err := c.createDDICPrimitive(ctx, CreateObjectOptions{
		ObjectType: ObjectTypeDataElement, Name: opts.Name, Description: opts.Description,
		PackageName: opts.PackageName, Transport: opts.Transport,
	}, result, func(lockHandle string) error {
		return c.setDataElementProperties(ctx, result.ObjectURL, opts, lockHandle)
	}, func() error {
		return c.verifyDataElementProperties(ctx, result.ObjectURL, opts)
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) createDDICPrimitive(ctx context.Context, createOpts CreateObjectOptions, result *DDICCreateResult, write func(string) error, verify func() error) error {
	if err := c.checkMutation(ctx, MutationContext{Op: OpWorkflow, OpName: "Create" + strings.TrimSuffix(strings.TrimPrefix(string(createOpts.ObjectType), "DDIC "), "/DD"), Package: createOpts.PackageName, Transport: createOpts.Transport}); err != nil {
		return err
	}
	if err := c.CreateObject(ctx, createOpts); err != nil {
		return err
	}
	lock, err := c.LockObject(ctx, result.ObjectURL, "MODIFY")
	if err != nil {
		return fmt.Errorf("locking %s %s after creation: %w", result.ObjectType, result.ObjectName, err)
	}
	if err := write(lock.LockHandle); err != nil {
		_ = c.UnlockObject(ctx, result.ObjectURL, lock.LockHandle)
		return err
	}
	if err := c.UnlockObject(ctx, result.ObjectURL, lock.LockHandle); err != nil {
		return fmt.Errorf("unlocking %s %s: %w", result.ObjectType, result.ObjectName, err)
	}
	activation, err := c.Activate(ctx, result.ObjectURL, result.ObjectName)
	if err != nil {
		return fmt.Errorf("activating %s %s: %w", result.ObjectType, result.ObjectName, err)
	}
	result.Activation = activation
	if !activation.Success {
		result.Message = "Activation failed - check activation messages"
		return fmt.Errorf("activating %s %s failed", result.ObjectType, result.ObjectName)
	}
	if err := verify(); err != nil {
		return err
	}
	result.Success = true
	result.Message = fmt.Sprintf("%s %s created, activated, and verified successfully", result.ObjectType, result.ObjectName)
	return nil
}

func (c *Client) setDomainProperties(ctx context.Context, objectURL string, opts DomainCreateOptions, lockHandle string) error {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<doma:domain xmlns:doma="http://www.sap.com/dictionary/domain" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="%s" adtcore:type="DOMA/DD" adtcore:version="new">
  <doma:content>
    <doma:typeInformation><doma:datatype>%s</doma:datatype><doma:length>%d</doma:length><doma:decimals>%d</doma:decimals></doma:typeInformation>
    <doma:outputInformation><doma:length>%d</doma:length><doma:style>%s</doma:style><doma:conversionExit>%s</doma:conversionExit><doma:signExists>%t</doma:signExists><doma:lowercase>%t</doma:lowercase><doma:ampmFormat>%t</doma:ampmFormat></doma:outputInformation>%s
  </doma:content>
</doma:domain>`, strings.ToUpper(opts.Name), escapeXML(opts.DataType), opts.Length, opts.Decimals, opts.OutputLength, escapeXML(opts.OutputStyle), escapeXML(opts.ConversionExit), opts.SignExists, opts.Lowercase, opts.AmpmFormat, domainValueXML(opts))
	return c.putDDICProperties(ctx, objectURL, body, lockHandle, opts.Transport)
}

func domainValueXML(opts DomainCreateOptions) string {
	if opts.ValueTableRef == "" && !opts.AppendExists && len(opts.FixValues) == 0 {
		return ""
	}
	var fixed strings.Builder
	for _, value := range opts.FixValues {
		fixed.WriteString("<doma:fixValue><doma:low>")
		fixed.WriteString(escapeXML(value.Low))
		fixed.WriteString("</doma:low>")
		if value.High != "" {
			fixed.WriteString("<doma:high>" + escapeXML(value.High) + "</doma:high>")
		}
		if value.Text != "" {
			fixed.WriteString("<doma:text>" + escapeXML(value.Text) + "</doma:text>")
		}
		fixed.WriteString("</doma:fixValue>")
	}
	if fixed.Len() > 0 {
		fixed.WriteString("")
	}
	return fmt.Sprintf(`<doma:valueInformation><doma:valueTableRef adtcore:name="%s"/><doma:appendExists>%t</doma:appendExists>%s</doma:valueInformation>`, escapeXML(strings.ToUpper(opts.ValueTableRef)), opts.AppendExists, fixedValuesXML(fixed.String()))
}

func fixedValuesXML(values string) string {
	if values == "" {
		return ""
	}
	return "<doma:fixValues>" + values + "</doma:fixValues>"
}

func (c *Client) setDataElementProperties(ctx context.Context, objectURL string, opts DataElementCreateOptions, lockHandle string) error {
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<blue:wbobj xmlns:blue="http://www.sap.com/wbobj/dictionary/dtel" xmlns:dtel="http://www.sap.com/adt/dictionary/dataelements" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="%s" adtcore:type="DTEL/DE" adtcore:version="new">
  <dtel:dataElement><dtel:typeKind>%s</dtel:typeKind><dtel:typeName>%s</dtel:typeName><dtel:dataType>%s</dtel:dataType><dtel:dataTypeLength>%d</dtel:dataTypeLength><dtel:dataTypeDecimals>%d</dtel:dataTypeDecimals>
    <dtel:shortFieldLabel>%s</dtel:shortFieldLabel><dtel:shortFieldLength>10</dtel:shortFieldLength><dtel:mediumFieldLabel>%s</dtel:mediumFieldLabel><dtel:mediumFieldLength>20</dtel:mediumFieldLength><dtel:longFieldLabel>%s</dtel:longFieldLabel><dtel:longFieldLength>40</dtel:longFieldLength><dtel:headingFieldLabel>%s</dtel:headingFieldLabel><dtel:headingFieldLength>55</dtel:headingFieldLength>
    <dtel:searchHelp>%s</dtel:searchHelp><dtel:searchHelpParameter>%s</dtel:searchHelpParameter><dtel:setGetParameter>%s</dtel:setGetParameter><dtel:defaultComponentName>%s</dtel:defaultComponentName><dtel:deactivateInputHistory>%t</dtel:deactivateInputHistory><dtel:changeDocument>%t</dtel:changeDocument><dtel:leftToRightDirection>%t</dtel:leftToRightDirection><dtel:deactivateBIDIFiltering>%t</dtel:deactivateBIDIFiltering>
  </dtel:dataElement>
</blue:wbobj>`, strings.ToUpper(opts.Name), map[bool]string{true: "domain", false: "predefinedAbapType"}[opts.Domain != ""], escapeXML(strings.ToUpper(opts.Domain)), escapeXML(opts.DataType), opts.Length, opts.Decimals, escapeXML(opts.Labels.Short), escapeXML(opts.Labels.Medium), escapeXML(opts.Labels.Long), escapeXML(opts.Labels.Heading), escapeXML(opts.SearchHelp), escapeXML(opts.SearchHelpParameter), escapeXML(opts.SetGetParameter), escapeXML(opts.DefaultComponentName), opts.DeactivateInputHistory, opts.ChangeDocument, opts.LeftToRightDirection, opts.DeactivateBIDIFiltering)
	return c.putDDICProperties(ctx, objectURL, body, lockHandle, opts.Transport)
}

func (c *Client) putDDICProperties(ctx context.Context, objectURL, body, lockHandle, transport string) error {
	query := url.Values{"lockHandle": []string{lockHandle}}
	if transport != "" {
		query.Set("corrNr", transport)
	}
	if _, err := c.transport.Request(ctx, objectURL, &RequestOptions{Method: http.MethodPut, Query: query, Body: []byte(body), ContentType: "application/*", Stateful: true}); err != nil {
		return fmt.Errorf("writing DDIC properties: %w", err)
	}
	return nil
}

func (c *Client) verifyDomainProperties(ctx context.Context, objectURL string, opts DomainCreateOptions) error {
	resp, err := c.transport.Request(ctx, objectURL, &RequestOptions{Method: http.MethodGet, Accept: "application/xml"})
	if err != nil {
		return fmt.Errorf("verifying domain properties: %w", err)
	}
	body := string(resp.Body)
	for _, expected := range []string{opts.DataType, strconv.Itoa(opts.Length), strconv.Itoa(opts.Decimals), strconv.Itoa(opts.OutputLength)} {
		if !strings.Contains(body, escapeXML(expected)) {
			return fmt.Errorf("verified domain properties do not contain %q", expected)
		}
	}
	return nil
}

func (c *Client) verifyDataElementProperties(ctx context.Context, objectURL string, opts DataElementCreateOptions) error {
	resp, err := c.transport.Request(ctx, objectURL, &RequestOptions{Method: http.MethodGet, Accept: "application/xml"})
	if err != nil {
		return fmt.Errorf("verifying data element properties: %w", err)
	}
	body := string(resp.Body)
	for _, expected := range []string{opts.Labels.Short, opts.Labels.Medium, opts.Labels.Long, opts.Labels.Heading, strconv.Itoa(opts.Length)} {
		if !strings.Contains(body, escapeXML(expected)) {
			return fmt.Errorf("verified data element properties do not contain %q", expected)
		}
	}
	return nil
}

func validateDomainCreateOptions(opts *DomainCreateOptions) error {
	if strings.TrimSpace(opts.Name) == "" || strings.TrimSpace(opts.Description) == "" || strings.TrimSpace(opts.PackageName) == "" || strings.TrimSpace(opts.DataType) == "" {
		return fmt.Errorf("name, description, packageName, and dataType are required")
	}
	if opts.Length < 1 || opts.Length > 5000 || opts.OutputLength < 1 || opts.OutputLength > 5000 {
		return fmt.Errorf("length and outputLength must be between 1 and 5000")
	}
	if opts.Decimals < 0 || opts.Decimals > 31 {
		return fmt.Errorf("decimals must be between 0 and 31")
	}
	return nil
}

func validateDataElementCreateOptions(opts *DataElementCreateOptions) error {
	if strings.TrimSpace(opts.Name) == "" || strings.TrimSpace(opts.Description) == "" || strings.TrimSpace(opts.PackageName) == "" || strings.TrimSpace(opts.DataType) == "" {
		return fmt.Errorf("name, description, packageName, and dataType are required")
	}
	if opts.Length < 1 || opts.Length > 5000 || opts.Decimals < 0 || opts.Decimals > 31 {
		return fmt.Errorf("length must be 1..5000 and decimals must be 0..31")
	}
	if opts.Labels.Short == "" || opts.Labels.Medium == "" || opts.Labels.Long == "" || opts.Labels.Heading == "" {
		return fmt.Errorf("short, medium, long, and heading labels are required")
	}
	return nil
}
