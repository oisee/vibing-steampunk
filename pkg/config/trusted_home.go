package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// The home directory a transport_cmd may come from, as the process
// environment had it when vsp started.
//
// os.UserHomeDir reads $HOME (USERPROFILE on Windows) at the time of the call,
// and the vsp command loads ./.env into the environment for any variable that
// is not already set. A project's .env with HOME=. would then make the
// project's own .vsp.json look like the user's ~/.vsp.json, and its
// transport_cmd would run. Package variables are initialised before any init()
// of the importing program, so this snapshot is taken before .env is read.
var (
	trustedHomeMu sync.RWMutex
	trustedHome   = homeFromEnv(os.LookupEnv)
)

// homeFromEnv picks the home directory the way os.UserHomeDir does, from the
// given environment lookup. Empty when unset.
func homeFromEnv(lookup func(string) (string, bool)) string {
	key := "HOME"
	switch runtime.GOOS {
	case "windows":
		key = "USERPROFILE"
	case "plan9":
		key = "home"
	}
	v, _ := lookup(key)
	return v
}

// SetTrustedHome replaces the home-directory snapshot and returns a function
// that restores the previous one. It exists for tests and for programs that
// embed this package and know their user's home better than the environment
// did at start-up.
func SetTrustedHome(dir string) (restore func()) {
	trustedHomeMu.Lock()
	prev := trustedHome
	trustedHome = dir
	trustedHomeMu.Unlock()
	return func() {
		trustedHomeMu.Lock()
		trustedHome = prev
		trustedHomeMu.Unlock()
	}
}

func currentTrustedHome() string {
	trustedHomeMu.RLock()
	defer trustedHomeMu.RUnlock()
	return trustedHome
}

// IsHomeConfigPath reports whether path is one of the user's own systems
// files, ~/.vsp.json or ~/.vsp/systems.json, judged by the home directory the
// process started with (see checkTrustedHomeConfig for the full rule).
func IsHomeConfigPath(path string) bool {
	return checkTrustedHomeConfig(path) == nil
}

// checkTrustedHomeConfig says why path is not a systems file the user owns in
// their home directory, or nil when it is:
//   - the home directory is the start-up snapshot, and must be absolute;
//   - path, after resolving symlinks, is ~/.vsp.json or ~/.vsp/systems.json,
//     after resolving symlinks too;
//   - outside Windows, the file is writable by its owner only: a file that
//     group or world can write is a file someone else can make run a program.
func checkTrustedHomeConfig(path string) error {
	if path == "" {
		return errors.New("not read from a file")
	}
	home := currentTrustedHome()
	if home == "" {
		return errors.New("the home directory was not set when vsp started")
	}
	if !filepath.IsAbs(home) {
		return fmt.Errorf("the home directory %q is not an absolute path", home)
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolving the config path: %w", err)
	}
	real, err = filepath.Abs(real)
	if err != nil {
		return err
	}
	match := false
	for _, p := range []string{filepath.Join(home, ".vsp.json"), filepath.Join(home, ".vsp", "systems.json")} {
		hp, err := filepath.EvalSymlinks(p)
		if err != nil {
			continue
		}
		if hp, err = filepath.Abs(hp); err == nil && hp == real {
			match = true
			break
		}
	}
	if !match {
		return errors.New("not the systems file in the home directory")
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(real)
		if err != nil {
			return err
		}
		if fi.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("the file is writable by group or others (mode %v)", fi.Mode().Perm())
		}
	}
	return nil
}
