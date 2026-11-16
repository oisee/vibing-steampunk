// Package landscape resolves only exact user-approved SAP Logon entries.
// Coordinates are transient memory and must never be logged or serialized.
package landscape

import (
	"encoding/xml"
	"errors"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// XML contracts: SAP UI Landscape Configuration Guide, sections 3.2.4.2.1,
// 3.2.6, 3.2.7 and 3.2.10:
// https://help.sap.com/doc/df5f752eb4004b2c9ecab769c9f71208/770.01/en-US/sap_ui_landscape_conf_guide.pdf
type service struct {
	UUID     string `xml:"uuid,attr"`
	Name     string `xml:"name,attr"`
	Type     string `xml:"type,attr"`
	System   string `xml:"systemid,attr"`
	Mode     string `xml:"mode,attr"`
	Server   string `xml:"server,attr"`
	MSID     string `xml:"msid,attr"`
	RouterID string `xml:"routerid,attr"`
	QOP      string `xml:"sncop,attr"`
	SNCName  string `xml:"sncname,attr"`
	NoSSO    string `xml:"sncnosso,attr"`
	Client   string `xml:"client,attr"`
}
type messageServer struct {
	UUID     string `xml:"uuid,attr"`
	Name     string `xml:"name,attr"`
	Host     string `xml:"host,attr"`
	Port     string `xml:"port,attr"`
	RouterID string `xml:"routerid,attr"`
}
type router struct {
	UUID  string `xml:"uuid,attr"`
	Route string `xml:"router,attr"`
}
type include struct {
	URL   string `xml:"url,attr"`
	Index int    `xml:"index,attr"`
}
type Landscape struct {
	XMLName  xml.Name        `xml:"Landscape"`
	Services []service       `xml:"Services>Service"`
	Servers  []messageServer `xml:"Messageservers>Messageserver"`
	Routers  []router        `xml:"Routers>Router"`
	Includes []include       `xml:"Includes>Include"`
}

const maxXML = 4 << 20

func Parse(r io.Reader) (*Landscape, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxXML+1))
	if err != nil || len(data) > maxXML {
		return nil, errors.New("landscape read failed or exceeded size cap")
	}
	var l Landscape
	if err := xml.Unmarshal(data, &l); err != nil {
		return nil, errors.New("invalid landscape XML")
	}
	return &l, nil
}

var sid = regexp.MustCompile(`^[A-Z0-9]{3}$`)
var client = regexp.MustCompile(`^[0-9]{3}$`)

func (l *Landscape) Resolve(system, clientID, name, sncLib string) (map[string]string, error) {
	if !sid.MatchString(system) || !client.MatchString(clientID) || strings.TrimSpace(name) == "" || sncLib == "" {
		return nil, errors.New("require exact target, connection and SNC library")
	}
	var matches []service
	for _, s := range l.Services {
		if s.Type == "SAPGUI" && s.Name == name {
			matches = append(matches, s)
		}
	}
	if len(matches) != 1 {
		return nil, errors.New("selected SAP Logon entry missing or ambiguous")
	}
	s := matches[0]
	if s.System != "" && s.System != system {
		return nil, errors.New("SAP Logon entry system does not match target")
	}
	if s.NoSSO != "" && s.NoSSO != "0" && !strings.EqualFold(s.NoSSO, "false") {
		return nil, errors.New("selected entry disables SSO")
	}
	if strings.TrimSpace(s.SNCName) == "" {
		return nil, errors.New("selected entry has no SNC partner configured")
	}
	switch s.QOP {
	case "1", "2", "3", "9":
	default:
		return nil, errors.New("selected entry lacks explicit enabled SNC quality of protection")
	}
	if s.Client != "" && s.Client != clientID {
		return nil, errors.New("SAP Logon entry client conflicts with target")
	}
	p := map[string]string{"client": clientID, "snc_mode": "1", "snc_sso": "1", "snc_qop": s.QOP, "snc_partnername": s.SNCName, "snc_lib": sncLib, "trace": "0"}
	routerID := s.RouterID
	switch s.Mode {
	case "1":
		if s.System != system {
			return nil, errors.New("direct entry requires matching system ID")
		}
		if s.MSID != "" {
			return nil, errors.New("conflicting application/message-server entry")
		}
		host, port, err := net.SplitHostPort(s.Server)
		if err != nil || host == "" || len(port) != 4 || !strings.HasPrefix(port, "32") {
			return nil, errors.New("unsupported direct connection; require exact host and SAP GUI port 32NN")
		}
		if _, err := strconv.Atoi(port); err != nil {
			return nil, errors.New("invalid SAP GUI port")
		}
		p["ashost"], p["sysnr"] = host, port[2:]
	case "", "0":
		var servers []messageServer
		for _, m := range l.Servers {
			if s.MSID != "" && m.UUID == s.MSID {
				servers = append(servers, m)
			}
		}
		if len(servers) != 1 || s.Server == "" {
			return nil, errors.New("message-server reference missing or ambiguous")
		}
		m := servers[0]
		port, err := strconv.Atoi(m.Port)
		if err != nil || port < 1 || port > 65535 || m.Host == "" || m.Name != system {
			return nil, errors.New("invalid or mismatching message-server definition")
		}
		if routerID != "" && m.RouterID != "" && routerID != m.RouterID {
			return nil, errors.New("conflicting SAProuter definitions")
		}
		if routerID == "" {
			routerID = m.RouterID
		}
		p["mshost"], p["msserv"], p["r3name"], p["group"] = m.Host, m.Port, system, s.Server
	default:
		return nil, errors.New("unsupported SAP Logon connection mode")
	}
	if routerID != "" {
		var routes []router
		for _, r := range l.Routers {
			if r.UUID == routerID {
				routes = append(routes, r)
			}
		}
		if len(routes) != 1 || !strings.HasPrefix(routes[0].Route, "/H/") {
			return nil, errors.New("SAProuter reference missing or ambiguous")
		}
		// Password-bearing router strings cannot be used by this SSO-only client.
		if strings.Contains(strings.ToUpper(routes[0].Route), "/P/") || strings.Contains(strings.ToUpper(routes[0].Route), "/W/") {
			return nil, errors.New("password-bearing router routes are unsupported")
		}
		p["saprouter"] = routes[0].Route
	}
	return p, nil
}
