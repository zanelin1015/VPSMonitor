//go:build !windows

package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadOpenWrtSystemVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "openwrt_release")
	if err := os.WriteFile(path, []byte("DISTRIB_ID='iStoreOS'\nDISTRIB_DESCRIPTION='iStoreOS 24.10.5'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readOpenWrtSystemVersion(path); got != "iStoreOS 24.10.5" {
		t.Fatalf("readOpenWrtSystemVersion() = %q", got)
	}
}
