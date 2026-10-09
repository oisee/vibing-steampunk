//go:build windows

package config

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// sncLib64FromRegistry reads SNC_LIB_64 where SAP GUI sets it: the user's
// environment, then the machine's. The process environment is not read: vsp
// loads ./.env into it, and a project must not choose a DLL vsp loads.
func sncLib64FromRegistry() string {
	for _, k := range []struct {
		root registry.Key
		path string
	}{
		{registry.CURRENT_USER, `Environment`},
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
	} {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, typ, err := key.GetStringValue("SNC_LIB_64")
		_ = key.Close()
		if err != nil || v == "" {
			continue
		}
		// %VAR% would expand from the process environment, which is the
		// thing not to trust; such a value needs snc_lib written out. The
		// machine's value is not tried instead: the user's overrides it, as
		// in Windows itself, and may name a different library.
		if typ == registry.EXPAND_SZ && strings.Contains(v, "%") {
			return ""
		}
		return v
	}
	return ""
}
