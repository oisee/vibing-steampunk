package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// abs makes a slash path absolute on the platform the test runs on.
func abs(p string) string {
	if runtime.GOOS == "windows" {
		return `C:` + filepath.FromSlash(p)
	}
	return p
}

// sncSystems is a systems file with one snc system.
func sncSystems(t *testing.T, mutate func(sys map[string]any)) []byte {
	t.Helper()
	sys := map[string]any{
		"snc": map[string]any{
			"dll":        abs("/opt/sap/nwrfcsdk/lib/sapnwrfc.dll"),
			"snc_lib":    abs("/opt/sap/secure/gx64krb5.dll"),
			"connection": "DEV - Development",
			"system":     "DEV",
			"client":     "100",
			"user":       "testuser",
		},
	}
	if mutate != nil {
		mutate(sys)
	}
	b, err := json.Marshal(map[string]any{"default": "dev", "systems": map[string]any{"dev": sys}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// asWindows makes applySNC believe it runs on Windows, as vsp.exe at exe.
func asWindows(t *testing.T, exe string) {
	t.Helper()
	goos, executable := sncGOOS, sncExecutable
	sncGOOS = "windows"
	sncExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { sncGOOS, sncExecutable = goos, executable })
}

func writeSystems(t *testing.T, dir string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".vsp.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func loadDev(t *testing.T) (*SystemConfig, error) {
	t.Helper()
	cfg, _, err := LoadSystems()
	if err != nil || cfg == nil {
		t.Fatalf("LoadSystems: %v", err)
	}
	return cfg.GetSystem("dev")
}

func TestSNC_BecomesTransportCmdFromHome(t *testing.T) {
	home, _ := isolateHome(t)
	asWindows(t, abs("/opt/vsp/vsp.exe"))
	writeSystems(t, home, sncSystems(t, func(sys map[string]any) {
		snc := sys["snc"].(map[string]any)
		snc["landscape"] = abs("/opt/sap/SAPUILandscape.xml")
		snc["request_timeout"] = "90s"
		snc["allow_data_preview"] = true
	}))
	t.Setenv("VSP_DEV_PASSWORD", "secret") // must not be picked up
	sys, err := loadDev(t)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{abs("/opt/vsp/vsp.exe"), "snc-serve",
		"-system", "DEV", "-client", "100", "-user", "testuser",
		"-connection", "DEV - Development", "-dll", abs("/opt/sap/nwrfcsdk/lib/sapnwrfc.dll"), "-snc-lib", abs("/opt/sap/secure/gx64krb5.dll"),
		"-landscape", abs("/opt/sap/SAPUILandscape.xml"), "-request-timeout", "1m30s", "-allow-data-preview"}
	if strings.Join(sys.TransportCmd, "|") != strings.Join(want, "|") {
		t.Errorf("TransportCmd = %q\nwant %q", sys.TransportCmd, want)
	}
	if sys.Client != "100" || sys.URL != SNCPlaceholderURL || sys.Expect != "DEV.100/TESTUSER" || !sys.ReadOnly {
		t.Errorf("client %q url %q expect %q read-only %v", sys.Client, sys.URL, sys.Expect, sys.ReadOnly)
	}
	if sys.Password != "" {
		t.Error("a password from the environment was attached to an snc system")
	}
}

func TestSNC_RefusedFromWorkingDirectory(t *testing.T) {
	_, work := isolateHome(t)
	asWindows(t, abs("/opt/vsp/vsp.exe"))
	writeSystems(t, work, sncSystems(t, nil))
	_, err := loadDev(t)
	if err == nil || !strings.Contains(err.Error(), "must not be able to make vsp load a DLL") {
		t.Fatalf("snc from a working-directory file: %v", err)
	}
}

func TestSNC_RefusedOutsideWindows(t *testing.T) {
	home, _ := isolateHome(t)
	asWindows(t, "/opt/vsp/vsp")
	sncGOOS = "linux"
	writeSystems(t, home, sncSystems(t, nil))
	_, err := loadDev(t)
	if err == nil || !strings.Contains(err.Error(), "only in vsp.exe on Windows") || !strings.Contains(err.Error(), "transport_cmd") {
		t.Fatalf("snc outside Windows: %v", err)
	}
}

func TestSNC_RefusesConflictsAndBadFields(t *testing.T) {
	cases := map[string]func(sys map[string]any){
		"transport_cmd": func(sys map[string]any) { sys["transport_cmd"] = []string{"helper"} },
		"user":          func(sys map[string]any) { sys["user"] = "TESTUSER" },
		"password":      func(sys map[string]any) { sys["password"] = "x" },
		"cookies":       func(sys map[string]any) { sys["cookie_string"] = "a=b" },
		"sso":           func(sys map[string]any) { sys["auth"] = "sso" },
		"client":        func(sys map[string]any) { sys["client"] = "200" },
		"system":        func(sys map[string]any) { sys["snc"].(map[string]any)["system"] = "dev" },
		"snc client":    func(sys map[string]any) { sys["snc"].(map[string]any)["client"] = "1" },
		"snc user":      func(sys map[string]any) { delete(sys["snc"].(map[string]any), "user") },
		"connection":    func(sys map[string]any) { sys["snc"].(map[string]any)["connection"] = " " },
		"dll relative":  func(sys map[string]any) { sys["snc"].(map[string]any)["dll"] = "sapnwrfc.dll" },
		"dll name":      func(sys map[string]any) { sys["snc"].(map[string]any)["dll"] = abs("/opt/sap/other.dll") },
		"snc_lib":       func(sys map[string]any) { sys["snc"].(map[string]any)["snc_lib"] = "gx64krb5.dll" },
		"landscape":     func(sys map[string]any) { sys["snc"].(map[string]any)["landscape"] = "SAPUILandscape.xml" },
		"logon_timeout": func(sys map[string]any) { sys["snc"].(map[string]any)["logon_timeout"] = "5m" },
		"request_timeout": func(sys map[string]any) {
			sys["snc"].(map[string]any)["request_timeout"] = "soon"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			home, _ := isolateHome(t)
			asWindows(t, abs("/opt/vsp/vsp.exe"))
			writeSystems(t, home, sncSystems(t, mutate))
			if sys, err := loadDev(t); err == nil {
				t.Fatalf("accepted; TransportCmd %q", sys.TransportCmd)
			}
		})
	}
}

func TestSNC_DoesNotAliasLoadedConfig(t *testing.T) {
	home, _ := isolateHome(t)
	asWindows(t, abs("/opt/vsp/vsp.exe"))
	writeSystems(t, home, sncSystems(t, nil))
	cfg, _, err := LoadSystems()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.GetSystem("dev"); err != nil {
		t.Fatal(err)
	}
	if again, err := cfg.GetSystem("dev"); err != nil || len(cfg.Systems["dev"].TransportCmd) != 0 || again.SNC == cfg.Systems["dev"].SNC {
		t.Fatalf("GetSystem changed the loaded config or shares its snc block: %v", err)
	}
}

// Without dll and snc_lib the block falls back to sapnwrfc.dll beside vsp.exe
// and to SNC_LIB_64 as SAP GUI sets it in the registry. Neither the process
// environment (./.env) nor SNC_LIB, the 32-bit one, is read.
func TestSNC_DefaultsFromExeDirAndSNCLIB64(t *testing.T) {
	home, _ := isolateHome(t)
	asWindows(t, abs("/opt/vsp/vsp.exe"))
	writeSystems(t, home, sncSystems(t, func(sys map[string]any) {
		snc := sys["snc"].(map[string]any)
		delete(snc, "dll")
		delete(snc, "snc_lib")
	}))
	old := sncLib64
	t.Cleanup(func() { sncLib64 = old })
	registryValue := abs("/home/u/lib/gx64krb5.dll")
	sncLib64 = func() string { return registryValue }
	// The process environment, which ./.env feeds, must not be read.
	t.Setenv("SNC_LIB_64", abs("/project/evil.dll"))
	t.Setenv("SNC_LIB", abs("/x86/sapsncencryption.dll"))

	sys, err := loadDev(t)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(sys.TransportCmd, "|")
	if !strings.Contains(joined, "-dll|"+abs("/opt/vsp/sapnwrfc.dll")) || !strings.Contains(joined, "-snc-lib|"+abs("/home/u/lib/gx64krb5.dll")) {
		t.Errorf("TransportCmd = %q", sys.TransportCmd)
	}

	registryValue = ""
	if _, err := loadDev(t); err == nil || !strings.Contains(err.Error(), "SNC_LIB_64") {
		t.Errorf("err = %v, want a refusal naming SNC_LIB_64 (SNC_LIB must not be used)", err)
	}
}

// A production snc system forwards GETs only; any other forwards bounded data
// preview SELECTs as well, unless nothing says otherwise.
func TestSNC_ProductionDecidesDataPreview(t *testing.T) {
	for _, c := range []struct {
		production, allow, want bool
	}{
		{false, false, true},
		{true, false, false},
		{true, true, true},
	} {
		home, _ := isolateHome(t)
		asWindows(t, abs("/opt/vsp/vsp.exe"))
		writeSystems(t, home, sncSystems(t, func(sys map[string]any) {
			snc := sys["snc"].(map[string]any)
			delete(snc, "allow_data_preview")
			if c.production {
				snc["production"] = true
			}
			if c.allow {
				snc["allow_data_preview"] = true
			}
		}))
		sys, err := loadDev(t)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(strings.Join(sys.TransportCmd, " "), "-allow-data-preview"); got != c.want {
			t.Errorf("production=%v allow=%v: data preview %v, want %v", c.production, c.allow, got, c.want)
		}
		if !sys.ReadOnly {
			t.Error("an snc system must stay read-only")
		}
	}
}
