package sncrfc

import (
	"errors"
	"os"
	"testing"
	"unsafe"

	"github.com/oisee/vibing-steampunk/pkg/sncrfc/runtimeguard"
)

type fakeLibrary struct {
	missing  string
	lookups  []string
	releases int
}
type fakeProcedure struct{}

func (fakeProcedure) Call(...uintptr) (uintptr, uintptr, error) { return 0, 0, nil }
func (f *fakeLibrary) find(s string) (procedure, error) {
	f.lookups = append(f.lookups, s)
	if s == f.missing {
		return nil, errors.New("secret SDK details")
	}
	return fakeProcedure{}, nil
}
func (f *fakeLibrary) release() error { f.releases++; return nil }

func TestRequiredSymbolsResolvedBeforeLogon(t *testing.T) {
	for _, name := range []string{"RfcOpenConnection", "RfcGetConnectionAttributes", "RfcCloseConnection"} {
		f := &fakeLibrary{missing: name}
		if a, err := bindSDK(f); a != nil || err == nil {
			t.Fatal("accepted incomplete SDK")
		}
		if f.releases != 1 {
			t.Fatal("DLL leaked after symbol failure")
		}
	}
}

func TestWindowsX64ABILayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("x64 contract")
	}
	if unsafe.Sizeof(connectionParameter{}) != 16 || unsafe.Sizeof(errorInfo{}) != 1752 || unsafe.Sizeof(connectionAttributes{}) != 1118 {
		t.Fatal("SDK ABI layout mismatch")
	}
	var a connectionAttributes
	if unsafe.Offsetof(a.sysID) != 540 || unsafe.Offsetof(a.client) != 558 || unsafe.Offsetof(a.user) != 566 {
		t.Fatal("identity field offsets mismatch")
	}
}

func TestInstalledSDKExportsOffline(t *testing.T) {
	path := os.Getenv("VSP_SNC_TEST_DLL")
	if path == "" {
		t.Skip("explicit DLL required for offline integration check")
	}
	dir, cleanup, err := runtimeguard.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(original)
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	})
	a, err := loadNative(path)
	if err != nil {
		t.Fatal(safeError("load installed SDK", err))
	}
	// NULL is an invalid handle, so this exercises the actual call ABI without
	// opening a connection, selecting a target or using the network.
	_, err = a.attributes(0)
	var native *NativeError
	if !errors.As(err, &native) || native.Code != 13 {
		t.Fatal("invalid-handle ABI check did not return RFC_INVALID_HANDLE")
	}
	if err := a.release(); err != nil {
		t.Fatal("release installed SDK failed")
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal("inspect guarded runtime directory failed")
	}
	if len(files) != 1 || files[0].Name() != "sapnwrfc.ini" {
		t.Fatal("installed SDK wrote a native artifact")
	}
}
