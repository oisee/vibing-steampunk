package sncrfc

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ABI contracts: SAP NW RFC SDK 7.50 Doxygen, connection group:
// https://support.sap.com/content/dam/support/en_us/library/ssp/products/connectors/nwrfcsdk/sap_nwrfcsdk_750_19_documentation.zip
// SAP's public PyRFC declarations corroborate the exact members and prototypes:
// https://github.com/SAP/PyRFC/blob/main/src/pyrfc/csapnwrfc.pxd
// SAP_UC is a 16-bit UTF-16 code unit; Windows x64 unsigned/enums are 32 bits.
// These Go interface declarations are independent of SAP headers.

// RFC_CONNECTION_PARAMETER: name and value are pointers to terminated SAP_UC.
type connectionParameter struct{ name, value *uint16 }

// RFC_ERROR_INFO: code/group followed by fixed-width SAP_UC message fields.
// Messages remain private memory and are never formatted or returned.
type errorInfo struct {
	code, group                uint32
	key                        [128]uint16
	message                    [512]uint16
	msgClass                   [21]uint16
	msgType                    [2]uint16
	msgNumber                  [4]uint16
	msgV1, msgV2, msgV3, msgV4 [51]uint16
}

// RFC_ATTRIBUTES, connection group. Keep the full layout, including tail fields.
type connectionAttributes struct {
	dest                          [65]uint16
	host, partnerHost             [101]uint16
	sysNumber                     [3]uint16
	sysID                         [9]uint16
	client                        [4]uint16
	user                          [13]uint16
	language                      [3]uint16
	trace                         [2]uint16
	isoLanguage                   [3]uint16
	codepage, partnerCodepage     [5]uint16
	rfcRole, ownType, partnerType [2]uint16
	rel, partnerRel, kernelRel    [5]uint16
	cpicConvID                    [9]uint16
	progName                      [129]uint16
	partnerBytesPerChar           [2]uint16
	partnerSystemCodepage         [5]uint16
	partnerIP                     [16]uint16
	partnerIPv6                   [46]uint16
	reserved                      [17]uint16
}

type procedure interface {
	Call(...uintptr) (uintptr, uintptr, error)
}
type library interface {
	find(string) (procedure, error)
	release() error
}
type windowsLibrary struct{ dll *windows.DLL }

func (l windowsLibrary) find(s string) (procedure, error) { return l.dll.FindProc(s) }
func (l windowsLibrary) release() error                   { return l.dll.Release() }

type sdkAPI struct {
	lib                            library
	openProc, attrsProc, closeProc procedure
}

func localDLL(path, base string) bool {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) || (base != "" && !strings.EqualFold(filepath.Base(path), base)) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func loadNative(path string) (nativeAPI, error) {
	if runtime.GOARCH != "amd64" || !localDLL(path, "sapnwrfc.dll") {
		return nil, errors.New("require local x64 RFC DLL")
	}
	// Enforce the logging boundary even for direct library callers. Checking
	// before loading matters: the GUI DLL can log from its initialization code.
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.New("require guarded runtime directory")
	}
	ini, err := os.ReadFile(filepath.Join(cwd, "sapnwrfc.ini"))
	if err != nil || string(ini) != "RFC_TRACE=0\nRFC_TRACE_DIR="+cwd+"\n" {
		return nil, errors.New("require controlled SDK trace configuration")
	}
	check, writeErr := os.CreateTemp(cwd, "snc-writecheck-")
	if writeErr == nil {
		name := check.Name()
		_ = check.Close()
		_ = os.Remove(name)
		return nil, errors.New("SDK runtime directory must refuse file creation")
	}
	// The SDK loads some of its own DLLs later, by name, with the process's
	// default search, which starts in the executable's directory: with
	// vsp.exe anywhere but beside the SDK they are not found and the worker
	// dies in native code. Search, for the rest of this process, only the
	// executable's directory, System32 and the SDK's own directory; never the
	// working directory or PATH.
	if err := windows.SetDefaultDllDirectories(windows.LOAD_LIBRARY_SEARCH_DEFAULT_DIRS); err != nil {
		return nil, errors.New("restrict DLL search failed")
	}
	sdkDir, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return nil, errors.New("require local x64 RFC DLL")
	}
	if _, err := windows.AddDllDirectory(sdkDir); err != nil {
		return nil, errors.New("add SDK directory to DLL search failed")
	}
	// Search dependencies only beside the selected DLL and in Windows System32.
	h, err := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return nil, errors.New("load DLL failed")
	}
	return bindSDK(windowsLibrary{dll: &windows.DLL{Name: path, Handle: h}})
}

func bindSDK(lib library) (nativeAPI, error) {
	a := &sdkAPI{lib: lib}
	var err error
	// RfcOpenConnection(RFC_CONNECTION_PARAMETER*, unsigned, RFC_ERROR_INFO*) -> handle.
	a.openProc, err = lib.find("RfcOpenConnection")
	if err != nil {
		_ = lib.release()
		return nil, errors.New("missing open API")
	}
	// RfcGetConnectionAttributes(handle, RFC_ATTRIBUTES*, RFC_ERROR_INFO*) -> RFC_RC.
	a.attrsProc, err = lib.find("RfcGetConnectionAttributes")
	if err != nil {
		_ = lib.release()
		return nil, errors.New("missing identity API")
	}
	// RfcCloseConnection(handle, RFC_ERROR_INFO*) -> RFC_RC.
	a.closeProc, err = lib.find("RfcCloseConnection")
	if err != nil {
		_ = lib.release()
		return nil, errors.New("missing close API")
	}
	return a, nil
}

func (a *sdkAPI) open(p map[string]string) (uintptr, error) {
	if !localDLL(p["snc_lib"], "") {
		return 0, errors.New("require local SNC library")
	}
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	params := make([]connectionParameter, len(keys))
	buffers := make([][]uint16, 0, len(keys)*2)
	// The interface call obscures the compiler's uintptr escape annotation.
	// Explicitly pin all native-call memory so Go stack growth cannot move it.
	var pins runtime.Pinner
	defer pins.Unpin()
	for i, k := range keys {
		n, e := windows.UTF16FromString(k)
		if e != nil {
			return 0, errors.New("invalid parameter")
		}
		v, e := windows.UTF16FromString(p[k])
		if e != nil {
			return 0, errors.New("invalid parameter")
		}
		buffers = append(buffers, n, v)
		pins.Pin(&n[0])
		pins.Pin(&v[0])
		params[i] = connectionParameter{&n[0], &v[0]}
	}
	var info errorInfo
	pins.Pin(&params[0])
	pins.Pin(&info)
	h, _, _ := a.openProc.Call(uintptr(unsafe.Pointer(&params[0])), uintptr(len(params)), uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(params)
	runtime.KeepAlive(buffers)
	runtime.KeepAlive(&info)
	if h == 0 {
		return 0, &NativeError{info.code, info.group}
	}
	return h, nil
}

func (a *sdkAPI) attributes(h uintptr) (Identity, error) {
	var attrs connectionAttributes
	var info errorInfo
	var pins runtime.Pinner
	pins.Pin(&attrs)
	pins.Pin(&info)
	defer pins.Unpin()
	rc, _, _ := a.attrsProc.Call(h, uintptr(unsafe.Pointer(&attrs)), uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(&attrs)
	runtime.KeepAlive(&info)
	if rc != 0 {
		return Identity{}, &NativeError{uint32(rc), info.group}
	}
	return Identity{System: decodeUC(attrs.sysID[:]), Client: decodeUC(attrs.client[:]), User: decodeUC(attrs.user[:])}, nil
}

func (a *sdkAPI) close(h uintptr) error {
	var info errorInfo
	var pins runtime.Pinner
	pins.Pin(&info)
	defer pins.Unpin()
	rc, _, _ := a.closeProc.Call(h, uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(&info)
	if rc != 0 {
		return &NativeError{uint32(rc), info.group}
	}
	return nil
}
func (a *sdkAPI) release() error { return a.lib.release() }
func decodeUC(v []uint16) string {
	for i, c := range v {
		if c == 0 {
			v = v[:i]
			break
		}
	}
	return strings.TrimRight(string(utf16.Decode(v)), " ")
}
