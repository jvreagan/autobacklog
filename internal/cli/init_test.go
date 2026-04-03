package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunInit_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	cmd := newInitCmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "autobacklog.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("config file should be created")
	}

	content, _ := os.ReadFile(path)
	if len(content) == 0 {
		t.Error("config file should not be empty")
	}
}

func TestRunInit_FileAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	// Create the file first
	os.WriteFile("autobacklog.yaml", []byte("existing"), 0644)

	cmd := newInitCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Error("should return error when file already exists")
	}
}
