//go:build !windows

package config

// sncLib64FromRegistry has no source outside Windows, where snc is refused.
func sncLib64FromRegistry() string { return "" }
