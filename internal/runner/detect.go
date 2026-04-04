package runner

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// DetectResult holds the detected test framework info.
type DetectResult struct {
	Framework string
	Command   string
	Args      []string
}

// Detect auto-detects the test framework for a project.
func Detect(workDir string, log *slog.Logger) *DetectResult {
	// Go
	if fileExists(workDir, "go.mod") {
		log.Info("detected test framework", "framework", "go")
		return &DetectResult{Framework: "go", Command: "go", Args: []string{"test", "./..."}}
	}

	// Node.js — check for test script in package.json
	if fileExists(workDir, "package.json") && hasNpmTestScript(workDir) {
		log.Info("detected test framework", "framework", "npm")
		return &DetectResult{Framework: "npm", Command: "npm", Args: []string{"test"}}
	}

	// Python pytest
	for _, f := range []string{"pytest.ini", "setup.cfg", "pyproject.toml"} {
		if fileExists(workDir, f) {
			log.Info("detected test framework", "framework", "pytest")
			return &DetectResult{Framework: "pytest", Command: "pytest", Args: nil}
		}
	}

	// Java Maven
	if fileExists(workDir, "pom.xml") {
		log.Info("detected test framework", "framework", "maven")
		return &DetectResult{Framework: "maven", Command: "mvn", Args: []string{"test"}}
	}

	// Java Gradle
	if fileExists(workDir, "build.gradle") || fileExists(workDir, "build.gradle.kts") {
		log.Info("detected test framework", "framework", "gradle")
		return &DetectResult{Framework: "gradle", Command: "gradle", Args: []string{"test"}}
	}

	// Rust
	if fileExists(workDir, "Cargo.toml") {
		log.Info("detected test framework", "framework", "cargo")
		return &DetectResult{Framework: "cargo", Command: "cargo", Args: []string{"test"}}
	}

	// Makefile with test target
	if fileExists(workDir, "Makefile") && hasMakeTarget(workDir, "test") {
		log.Info("detected test framework", "framework", "make")
		return &DetectResult{Framework: "make", Command: "make", Args: []string{"test"}}
	}

	log.Warn("no test framework detected")
	return nil
}

func fileExists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func hasNpmTestScript(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}
	testCmd, ok := pkg.Scripts["test"]
	if !ok {
		return false
	}
	// Exclude npm's default placeholder
	return !strings.HasPrefix(testCmd, "echo ")
}

func hasMakeTarget(dir, target string) bool {
	f, err := os.Open(filepath.Join(dir, "Makefile"))
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, target+":") || strings.HasPrefix(line, target+" :") {
			return true
		}
	}
	return false
}
