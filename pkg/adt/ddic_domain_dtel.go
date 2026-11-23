package adt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Domains and data elements are form objects to ADT, not source: one XML
// document carries the whole definition, and a POST of the complete document
// creates the object with it (inactive). CreateDomain and CreateDataElement
// build that document, activate, and check the inactive list afterwards.

// The form documents' media types, newest first. SAP_BASIS 7.50 only knows v1
// and refuses v2 with 406 or 415 before creating anything; the document is the
// same in both versions, so createDDICForm sends it again as v1.
var (
	domainMediaTypes      = []string{"application/vnd.sap.adt.domains.v2+xml", "application/vnd.sap.adt.domains.v1+xml"}
	dataElementMediaTypes = []string{"application/vnd.sap.adt.dataelements.v2+xml", "application/vnd.sap.adt.dataelements.v1+xml"}
)

// dataElementsAccept is what the data element readers (GetTypeInfo,
// GetDataElementLabels) send: v2 where the system has it, else v1, which 7.50
// answers with the same document. A v2-only Accept gets 406 there.
const dataElementsAccept = "application/vnd.sap.adt.dataelements.v2+xml, application/vnd.sap.adt.dataelements.v1+xml;q=0.9"

// FixedValue is one fixed value of a domain.
type FixedValue struct {
	Low  string `json:"low"`
	High string `json:"high,omitempty"`
	Text string `json:"text,omitempty"`
}

// DomainOptions describes a domain (SE11).
type DomainOptions struct {
	Name           string
	Description    string
	Package        string // defaults to $TMP
	Transport      string
	DataType       string // CHAR, NUMC, DEC, INT4, DATS, ...
	Length         int
	Decimals       int
	OutputLength   int // defaults to Length
	Lowercase      bool
	Signed         bool
	ConversionExit string
	ValueTable     string
	FixedValues    []FixedValue
	MasterLanguage string // ISO code; the session language when empty
}

// DataElementOptions describes a data element (SE11). Either Domain or
// DataType (a predefined type with Length/Decimals) gives its type.
type DataElementOptions struct {
	Name           string
	Description    string
	Package        string
	Transport      string
	Domain         string
	DataType       string
	Length         int
	Decimals       int
	ShortLabel     string // 10
	MediumLabel    string // 20
	LongLabel      string // 40
	HeadingLabel   string // 55
	SearchHelp     string
	ParameterID    string // SET/GET parameter
	ChangeDocument bool
	MasterLanguage string
}

// DDICObjectResult is what a DDIC create did.
type DDICObjectResult struct {
	Name       string            `json:"name"`
	ObjectURL  string            `json:"objectUrl"`
	Active     bool              `json:"active"`
	Activation *ActivationResult `json:"activation,omitempty"`
}

func xmlTrueFalse(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (c *Client) ddicLanguage(lang string) string {
	lang = strings.ToUpper(strings.TrimSpace(lang))
	if lang == "" {
		lang = strings.ToUpper(c.config.Language)
	}
	if lang == "" {
		lang = "EN"
	}
	return lang
}

func validateDDICName(what, name string) error {
	if name == "" {
		return fmt.Errorf("a %s name is required", what)
	}
	if len(name) > 30 {
		return fmt.Errorf("%s name %s is longer than 30 characters", what, name)
	}
	return nil
}

// domainBody is the complete domain document.
func domainBody(o DomainOptions, lang string) string {
	var fv strings.Builder
	for i, v := range o.FixedValues {
		fmt.Fprintf(&fv, `<doma:fixValue><doma:position>%04d</doma:position><doma:low>%s</doma:low><doma:high>%s</doma:high><doma:text>%s</doma:text></doma:fixValue>`,
			i+1, escapeXML(v.Low), escapeXML(v.High), escapeXML(v.Text))
	}
	valueTable := `<doma:valueTableRef/>`
	if o.ValueTable != "" {
		valueTable = fmt.Sprintf(`<doma:valueTableRef adtcore:name="%s" adtcore:type="TABL/DT"/>`, escapeXML(strings.ToUpper(o.ValueTable)))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<doma:domain xmlns:doma="http://www.sap.com/dictionary/domain" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="%s" adtcore:type="DOMA/DD" adtcore:description="%s" adtcore:language="%s" adtcore:masterLanguage="%s">
  <adtcore:packageRef adtcore:name="%s"/>
  <doma:content>
    <doma:typeInformation><doma:datatype>%s</doma:datatype><doma:length>%06d</doma:length><doma:decimals>%06d</doma:decimals></doma:typeInformation>
    <doma:outputInformation><doma:length>%06d</doma:length><doma:style>00</doma:style><doma:conversionExit>%s</doma:conversionExit><doma:signExists>%s</doma:signExists><doma:lowercase>%s</doma:lowercase><doma:ampmFormat>false</doma:ampmFormat></doma:outputInformation>
    <doma:valueInformation>%s<doma:appendExists>false</doma:appendExists><doma:fixValues>%s</doma:fixValues></doma:valueInformation>
  </doma:content>
</doma:domain>`,
		escapeXML(o.Name), escapeXML(o.Description), escapeXML(lang), escapeXML(lang), escapeXML(o.Package),
		escapeXML(strings.ToUpper(o.DataType)), o.Length, o.Decimals,
		o.OutputLength, escapeXML(strings.ToUpper(o.ConversionExit)), xmlTrueFalse(o.Signed), xmlTrueFalse(o.Lowercase),
		valueTable, fv.String())
}

// dataElementBody is the complete data element document. ADT wants every
// element in this order, the *MaxLength ones included.
func dataElementBody(o DataElementOptions, lang string) string {
	typeKind, typeName := "predefinedAbapType", ""
	if o.Domain != "" {
		typeKind, typeName = "domain", strings.ToUpper(o.Domain)
	}
	var labels strings.Builder
	for _, l := range []struct {
		kind, text string
	}{{"short", o.ShortLabel}, {"medium", o.MediumLabel}, {"long", o.LongLabel}, {"heading", o.HeadingLabel}} {
		m := maxLen(l.kind)
		fmt.Fprintf(&labels, `<dtel:%[1]sFieldLabel>%[2]s</dtel:%[1]sFieldLabel><dtel:%[1]sFieldLength>%[3]d</dtel:%[1]sFieldLength><dtel:%[1]sFieldMaxLength>%[3]d</dtel:%[1]sFieldMaxLength>`,
			l.kind, escapeXML(l.text), m)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<blue:wbobj xmlns:blue="http://www.sap.com/wbobj/dictionary/dtel" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="%s" adtcore:type="DTEL/DE" adtcore:description="%s" adtcore:language="%s" adtcore:masterLanguage="%s">
  <adtcore:packageRef adtcore:name="%s"/>
  <dtel:dataElement xmlns:dtel="http://www.sap.com/adt/dictionary/dataelements"><dtel:typeKind>%s</dtel:typeKind><dtel:typeName>%s</dtel:typeName><dtel:dataType>%s</dtel:dataType><dtel:dataTypeLength>%06d</dtel:dataTypeLength><dtel:dataTypeDecimals>%06d</dtel:dataTypeDecimals>%s<dtel:searchHelp>%s</dtel:searchHelp><dtel:searchHelpParameter/><dtel:setGetParameter>%s</dtel:setGetParameter><dtel:defaultComponentName/><dtel:deactivateInputHistory>false</dtel:deactivateInputHistory><dtel:changeDocument>%s</dtel:changeDocument><dtel:leftToRightDirection>false</dtel:leftToRightDirection><dtel:deactivateBIDIFiltering>false</dtel:deactivateBIDIFiltering></dtel:dataElement>
</blue:wbobj>`,
		escapeXML(o.Name), escapeXML(o.Description), escapeXML(lang), escapeXML(lang), escapeXML(o.Package),
		typeKind, escapeXML(typeName), escapeXML(strings.ToUpper(o.DataType)), o.Length, o.Decimals,
		labels.String(), escapeXML(strings.ToUpper(o.SearchHelp)), escapeXML(strings.ToUpper(o.ParameterID)), xmlTrueFalse(o.ChangeDocument))
}

// maxLen is the standard maximum length of a field label kind.
func maxLen(kind string) int {
	switch kind {
	case "short":
		return 10
	case "medium":
		return 20
	case "long":
		return 40
	}
	return 55
}

// CreateDomain creates and activates a domain.
func (c *Client) CreateDomain(ctx context.Context, o DomainOptions) (*DDICObjectResult, error) {
	o.Name = strings.ToUpper(strings.TrimSpace(o.Name))
	if err := validateDDICName("domain", o.Name); err != nil {
		return nil, err
	}
	if o.DataType == "" || o.Length <= 0 && !fixedLengthType(o.DataType) {
		return nil, fmt.Errorf("a data type and a length are required (e.g. CHAR 10)")
	}
	if o.OutputLength == 0 {
		o.OutputLength = o.Length
	}
	for _, v := range o.FixedValues {
		if len([]rune(v.Text)) > 60 {
			return nil, fmt.Errorf("fixed value %s: the text is %d characters long, at most 60 fit", v.Low, len([]rune(v.Text)))
		}
	}
	o.Package = defaultPackage(o.Package)
	return c.createDDICForm(ctx, "domain", o.Name, o.Package, o.Transport,
		"/sap/bc/adt/ddic/domains", domainMediaTypes,
		domainBody(o, c.ddicLanguage(o.MasterLanguage)))
}

// CreateDataElement creates and activates a data element.
func (c *Client) CreateDataElement(ctx context.Context, o DataElementOptions) (*DDICObjectResult, error) {
	o.Name = strings.ToUpper(strings.TrimSpace(o.Name))
	if err := validateDDICName("data element", o.Name); err != nil {
		return nil, err
	}
	if (o.Domain == "") == (o.DataType == "") {
		return nil, fmt.Errorf("name either a domain or a predefined data type (with length), not both")
	}
	for kind, text := range map[string]string{"short": o.ShortLabel, "medium": o.MediumLabel, "long": o.LongLabel, "heading": o.HeadingLabel} {
		if len([]rune(text)) > maxLen(kind) {
			return nil, fmt.Errorf("the %s label holds %d characters, got %d", kind, maxLen(kind), len([]rune(text)))
		}
	}
	o.Package = defaultPackage(o.Package)
	return c.createDDICForm(ctx, "data element", o.Name, o.Package, o.Transport,
		"/sap/bc/adt/ddic/dataelements", dataElementMediaTypes,
		dataElementBody(o, c.ddicLanguage(o.MasterLanguage)))
}

func defaultPackage(p string) string {
	if strings.TrimSpace(p) == "" {
		return "$TMP"
	}
	return strings.ToUpper(strings.TrimSpace(p))
}

// fixedLengthType is a predefined type whose length SAP fixes itself.
func fixedLengthType(t string) bool {
	switch strings.ToUpper(t) {
	case "DATS", "TIMS", "INT1", "INT2", "INT4", "INT8", "FLTP", "STRING", "RAWSTRING", "CLNT", "LANG", "UTCLONG", "D16N", "D34N":
		return true
	}
	return false
}

// createDDICForm POSTs a complete form document, as the first of contentTypes
// the system accepts, activates the object and checks the inactive list.
func (c *Client) createDDICForm(ctx context.Context, what, name, pkg, transport, collection string, contentTypes []string, body string) (*DDICObjectResult, error) {
	if err := c.checkMutation(ctx, MutationContext{
		Op:        OpCreate,
		OpName:    "Create " + what,
		Package:   pkg,
		Transport: transport,
	}); err != nil {
		return nil, err
	}
	params := url.Values{}
	if transport != "" {
		params.Set("corrNr", transport)
	}
	var err error
	for _, contentType := range contentTypes {
		_, err = c.transport.Request(ctx, collection, &RequestOptions{
			Method:      http.MethodPost,
			Query:       params,
			Body:        []byte(body),
			ContentType: contentType,
			Accept:      contentType,
		})
		if !mediaTypeRefused(err) {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("creating %s %s: %w", what, name, err)
	}
	objectURL := collection + "/" + url.PathEscape(strings.ToLower(name))
	res := &DDICObjectResult{Name: name, ObjectURL: objectURL}
	activation, err := c.Activate(ctx, objectURL, name)
	if err != nil {
		return res, fmt.Errorf("activating %s %s: %w", what, name, err)
	}
	res.Activation = activation
	if !activation.Success {
		return res, fmt.Errorf("%s %s was created but did not activate: %s", what, name, strings.Join(activation.ProblemLines(), "; "))
	}
	// Trust the inactive list, not the answer to the activation (see
	// CreateStructure). Without the list there is nothing to confirm it with.
	records, err := c.GetInactiveObjects(ctx)
	if err != nil {
		return res, fmt.Errorf("%s %s: SAP reported the activation as successful, but the inactive list could not be read to confirm it: %w", what, name, err)
	}
	if objectInactive(objectURL, records) {
		return res, fmt.Errorf("%s %s: SAP reported the activation as successful, but it is still inactive", what, name)
	}
	res.Active = true
	return res, nil
}

// mediaTypeRefused is a 406 or a 415: the system does not know this version of
// the document and refused it before doing anything, so another may be tried.
func mediaTypeRefused(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) &&
		(apiErr.StatusCode == http.StatusNotAcceptable || apiErr.StatusCode == http.StatusUnsupportedMediaType)
}
