package runner

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestDetect_Go(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/foo"), 0644)

	result := Detect(dir, slog.Default())
	if result == nil {
		t.Fatal("should detect Go project")
	}
	if result.Framework != "go" {
		t.Errorf("Framework = %q, want go", result.Framework)
	}
	if result.Command != "go" {
		t.Errorf("Command = %q, want go", result.Command)
	}
}

func TestDetect_Npm(t *testing.T) {
	dir := t.TempDir()
	pkg := `{"scripts": {"test": "jest --coverage"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)

	result := Detect(dir, slog.Default())
	if result == nil {
		t.Fatal("should detect npm project")
	}
	if result.Framework != "npm" {
		t.Errorf("Framework = %q, want npm", result.Framework)
	}
}

func TestDetect_NpmEchoSkipped(t *testing.T) {
	dir := t.TempDir()
	// The default npm init test script should not be detected
	pkg := `{"scripts": {"test": "echo \"Error: no test specified\" && exit 1"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)

	result := Detect(dir, slog.Default())
	if result != nil {
		t.Error("should not detect npm project with echo-only test script")
	}
}

func TestDetect_Pytest(t *testing.T) {
	for _, f := range []string{"pytest.ini", "setup.cfg", "pyproject.toml"} {
		t.Run(f, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, f), []byte(""), 0644)

			result := Detect(dir, slog.Default())
			if result == nil {
				t.Fatal("should detect pytest project")
			}
			if result.Framework != "pytest" {
				t.Errorf("Framework = %q, want pytest", result.Framework)
			}
		})
	}
}

func TestDetect_Maven(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0644)

	result := Detect(dir, slog.Default())
	if result == nil {
		t.Fatal("should detect Maven project")
	}
	if result.Framework != "maven" {
		t.Errorf("Framework = %q, want maven", result.Framework)
	}
}

func TestDetect_Gradle(t *testing.T) {
	for _, f := range []string{"build.gradle", "build.gradle.kts"} {
		t.Run(f, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, f), []byte(""), 0644)

			result := Detect(dir, slog.Default())
			if result == nil {
				t.Fatal("should detect Gradle project")
			}
			if result.Framework != "gradle" {
				t.Errorf("Framework = %q, want gradle", result.Framework)
			}
		})
	}
}

func TestDetect_Cargo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]"), 0644)

	result := Detect(dir, slog.Default())
	if result == nil {
		t.Fatal("should detect Cargo project")
	}
	if result.Framework != "cargo" {
		t.Errorf("Framework = %q, want cargo", result.Framework)
	}
}

func TestDetect_Makefile(t *testing.T) {
	dir := t.TempDir()
	makefile := "build:\n\tgo build\n\ntest:\n\tgo test ./...\n"
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte(makefile), 0644)

	result := Detect(dir, slog.Default())
	if result == nil {
		t.Fatal("should detect Makefile with test target")
	}
	if result.Framework != "make" {
		t.Errorf("Framework = %q, want make", result.Framework)
	}
}

func TestDetect_MakefileNoTestTarget(t *testing.T) {
	dir := t.TempDir()
	makefile := "build:\n\tgo build\n"
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte(makefile), 0644)

	result := Detect(dir, slog.Default())
	if result != nil {
		t.Error("should not detect Makefile without test target")
	}
}

func TestDetect_NoFramework(t *testing.T) {
	dir := t.TempDir()

	result := Detect(dir, slog.Default())
	if result != nil {
		t.Error("should return nil for empty directory")
	}
}

func TestDetect_Priority(t *testing.T) {
	// If both go.mod and package.json exist, Go should win
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module foo"), 0644)
	pkg := `{"scripts": {"test": "jest"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0644)

	result := Detect(dir, slog.Default())
	if result == nil {
		t.Fatal("should detect something")
	}
	if result.Framework != "go" {
		t.Errorf("Framework = %q, want go (higher priority)", result.Framework)
	}
}

func TestHasMakeTarget(t *testing.T) {
	dir := t.TempDir()
	makefile := "build:\n\tgo build\n\ntest:\n\tgo test ./...\n\nclean :\n\trm -f bin\n"
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte(makefile), 0644)

	if !hasMakeTarget(dir, "test") {
		t.Error("should find 'test' target")
	}
	if !hasMakeTarget(dir, "build") {
		t.Error("should find 'build' target")
	}
	if !hasMakeTarget(dir, "clean") {
		t.Error("should find 'clean' target (with space before colon)")
	}
	if hasMakeTarget(dir, "deploy") {
		t.Error("should not find nonexistent 'deploy' target")
	}
}
