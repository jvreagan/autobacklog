//go:build !windows

package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// StubBinDir creates a temporary directory and prepends it to PATH so that
// stub scripts placed there are found before real binaries.
func StubBinDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "stub-bin")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("creating stub bin dir: %v", err)
	}
	PrependPath(t, dir)
	return dir
}

// WriteStubScript writes an executable shell script into dir with the given
// name and body. The script is prefixed with #!/bin/sh.
func WriteStubScript(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	content := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("writing stub script %s: %v", name, err)
	}
}

// PrependPath adds dir to the front of PATH for the duration of the test.
func PrependPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}
