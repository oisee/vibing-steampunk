//go:build !windows

package runtimeguard

import "errors"

func Prepare() (string, func() error, error) {
	return "", nil, errors.New("SNC worker requires Windows")
}
