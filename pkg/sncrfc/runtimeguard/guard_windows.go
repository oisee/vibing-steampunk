// Package runtimeguard gives the SDK a private working directory that cannot
// accept log files. SAP's trace level zero still permits diagnostic log writes.
// See https://userapps.support.sap.com/sap/support/knowledge/en/2954209
package runtimeguard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Prepare creates only nonsecret SDK configuration, then removes directory write
// permissions for the current user. The caller must run its SDK worker here.
func Prepare() (string, func() error, error) {
	dir, err := os.MkdirTemp("", "vsp-snc-")
	if err != nil {
		return "", nil, errors.New("create private runtime directory failed")
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return "", nil, errors.New("resolve runtime directory failed")
	}
	original, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return "", nil, errors.New("read runtime permissions failed")
	}
	cleanup := func() error {
		// Verify the generated absolute path remains inside its original temp root.
		// Remove only our known nonsecret config and empty directory, never a tree.
		root, err := filepath.Abs(os.TempDir())
		if err != nil {
			return errors.New("resolve runtime root failed")
		}
		rel, err := filepath.Rel(root, dir)
		if err != nil || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") || filepath.Dir(rel) != "." || !strings.HasPrefix(filepath.Base(rel), "vsp-snc-") {
			return errors.New("runtime cleanup path validation failed")
		}
		if err := setAccess(dir, original); err != nil {
			return errors.New("restore runtime cleanup permissions failed")
		}
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("runtime cleanup target is not an owned directory")
		}
		if err := os.Remove(filepath.Join(dir, "sapnwrfc.ini")); err != nil && !os.IsNotExist(err) {
			return errors.New("runtime configuration cleanup failed")
		}
		if err := os.Remove(dir); err != nil {
			return errors.New("runtime cleanup failed")
		}
		return nil
	}
	// No coordinates, identities, tokens or SAP data are placed in this file.
	ini := "RFC_TRACE=0\nRFC_TRACE_DIR=" + dir + "\n"
	if err := os.WriteFile(filepath.Join(dir, "sapnwrfc.ini"), []byte(ini), 0600); err != nil {
		_ = cleanup()
		return "", nil, errors.New("prepare SDK trace configuration failed")
	}
	// Preserve inherited read/traverse rights, including restricted-token ACEs.
	// Deny Everyone creation of files/subdirectories in just this new directory.
	sddl := original.String()
	start := strings.Index(sddl, "D:")
	if start < 0 {
		_ = cleanup()
		return "", nil, errors.New("runtime directory lacks DACL")
	}
	firstACE := strings.Index(sddl[start:], "(")
	if firstACE < 0 {
		_ = cleanup()
		return "", nil, errors.New("runtime directory lacks permission entries")
	}
	position := start + firstACE
	locked, err := windows.SecurityDescriptorFromString(sddl[:position] + "(D;;0x00000006;;;WD)" + sddl[position:])
	if err != nil {
		_ = cleanup()
		return "", nil, errors.New("build runtime log-write restriction failed")
	}
	if err := setAccess(dir, locked); err != nil {
		_ = cleanup()
		return "", nil, errors.New("restrict runtime directory failed")
	}
	return dir, cleanup, nil
}

func setAccess(path string, sd *windows.SECURITY_DESCRIPTOR) error {
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
