package landscape

import (
	"strings"
	"testing"
)

const directFixture = `<Landscape><Services><Service type="SAPGUI" uuid="s" name="Test" systemid="TST" mode="1" server="fixture.invalid:3230" sncop="9" sncname="p:fixture"/></Services></Landscape>`

func TestExactDirectEntry(t *testing.T) {
	l, err := Parse(strings.NewReader(directFixture))
	if err != nil {
		t.Fatal(err)
	}
	p, err := l.Resolve("TST", "123", "Test", "fixture.dll")
	if err != nil {
		t.Fatal(err)
	}
	if p["ashost"] != "fixture.invalid" || p["sysnr"] != "30" || p["client"] != "123" || p["snc_qop"] != "9" || p["snc_mode"] != "1" || p["snc_sso"] != "1" || p["trace"] != "0" {
		t.Fatal("incorrect RFC parameters")
	}
	if _, err := l.Resolve("TST", "123", "Tes", "fixture.dll"); err == nil {
		t.Fatal("fuzzy selection")
	}
}

func TestRejectAmbiguityDisabledSNCAndWrongSID(t *testing.T) {
	for _, data := range []string{
		strings.Replace(directFixture, `</Services>`, strings.TrimSuffix(strings.TrimPrefix(directFixture, `<Landscape><Services>`), `</Services></Landscape>`)+`</Services>`, 1),
		strings.Replace(directFixture, `sncop="9"`, `sncop="0"`, 1),
		strings.Replace(directFixture, `sncop="9"`, `sncop="9" sncnosso="1"`, 1),
		strings.Replace(directFixture, `systemid="TST"`, `systemid="BAD"`, 1),
		strings.Replace(directFixture, `sncname="p:fixture"`, ``, 1),
	} {
		l, err := Parse(strings.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := l.Resolve("TST", "123", "Test", "fixture.dll"); err == nil {
			t.Fatal("unsafe landscape accepted")
		}
	}
}

func TestMessageServerUsesExactPortAndGroup(t *testing.T) {
	l, err := Parse(strings.NewReader(`<Landscape><Messageservers><Messageserver uuid="m" name="TST" host="fixture.invalid" port="3630"/></Messageservers><Services><Service type="SAPGUI" uuid="s" name="Test" systemid="TST" msid="m" server="GROUP" sncop="3" sncname="p:fixture"/></Services></Landscape>`))
	if err != nil {
		t.Fatal(err)
	}
	p, err := l.Resolve("TST", "123", "Test", "fixture.dll")
	if err != nil {
		t.Fatal(err)
	}
	if p["mshost"] != "fixture.invalid" || p["msserv"] != "3630" || p["group"] != "GROUP" || p["r3name"] != "TST" || p["snc_qop"] != "3" {
		t.Fatal("guessed connection coordinates")
	}
}

func TestMalformedXMLDoesNotLeak(t *testing.T) {
	_, err := Parse(strings.NewReader(`<Landscape secret-host`))
	if err == nil || strings.Contains(err.Error(), "secret-host") {
		t.Fatal("XML error leaked")
	}
}

func TestOptionalSIDRequiresUniqueMatchingMessageServer(t *testing.T) {
	fixture := `<Landscape><Messageservers><Messageserver uuid="m" name="TST" host="fixture.invalid" port="3630"/></Messageservers><Services><Service type="SAPGUI" name="Test" msid="m" server="GROUP" sncop="9" sncname="p:fixture"/></Services></Landscape>`
	l, _ := Parse(strings.NewReader(fixture))
	if _, err := l.Resolve("TST", "123", "Test", "fixture.dll"); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		strings.Replace(fixture, `name="TST"`, `name="BAD"`, 1),
		strings.Replace(fixture, `type="SAPGUI"`, `type="SAPGUI" systemid="BAD"`, 1),
		strings.Replace(fixture, `</Messageservers>`, `<Messageserver uuid="m" name="TST" host="other.invalid" port="3630"/></Messageservers>`, 1),
		strings.Replace(directFixture, `systemid="TST"`, ``, 1),
	} {
		l, _ := Parse(strings.NewReader(data))
		if _, err := l.Resolve("TST", "123", "Test", "fixture.dll"); err == nil {
			t.Fatal("unverified target accepted")
		}
	}
}
