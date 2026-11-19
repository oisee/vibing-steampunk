// Command vsp is the release binary: vsp with no extensions.
package main

import "github.com/oisee/vibing-steampunk/pkg/vsp"

// Version information, set by build flags (-X main.Version=...).
var (
	Version     = "dev"
	Commit      = "unknown"
	BuildDate   = "unknown"
	ReleaseRepo = ""
)

func main() {
	vsp.Version, vsp.Commit, vsp.BuildDate, vsp.ReleaseRepo = Version, Commit, BuildDate, ReleaseRepo
	vsp.Run()
}
