//go:build !windows

package sncrfc

import "errors"

func loadNative(string) (nativeAPI, error) {
	return nil, errors.New("SNC SSO requires a Windows process")
}
