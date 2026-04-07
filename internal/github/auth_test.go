package github

import (
	"os"
	"strings"
	"testing"
)

func TestGhEnv_DoesNotMutateGlobalEnv(t *testing.T) {
	patMu.Lock()
	storedPAT = "test-token-12345"
	patMu.Unlock()
	t.Cleanup(func() {
		patMu.Lock()
		storedPAT = ""
		patMu.Unlock()
	})

	before := os.Getenv("GITHUB_TOKEN")
	_ = ghEnv()
	after := os.Getenv("GITHUB_TOKEN")

	if before != after {
		t.Errorf("ghEnv() mutated global GITHUB_TOKEN: before=%q after=%q", before, after)
	}
}

func TestGhEnv_InjectsToken(t *testing.T) {
	const token = "ghp_testtoken"

	patMu.Lock()
	storedPAT = token
	patMu.Unlock()
	t.Cleanup(func() {
		patMu.Lock()
		storedPAT = ""
		patMu.Unlock()
	})

	env := ghEnv()

	var found string
	for _, e := range env {
		if strings.HasPrefix(e, "GITHUB_TOKEN=") {
			found = strings.TrimPrefix(e, "GITHUB_TOKEN=")
		}
	}
	if found != token {
		t.Errorf("ghEnv() GITHUB_TOKEN=%q, want %q", found, token)
	}
}

func TestGhEnv_NoTokenWhenEmpty(t *testing.T) {
	patMu.Lock()
	storedPAT = ""
	patMu.Unlock()

	// #174: use t.Setenv for parallel safety instead of os.Setenv/Unsetenv
	t.Setenv("GITHUB_TOKEN", "")
	os.Unsetenv("GITHUB_TOKEN")

	env := ghEnv()
	for _, e := range env {
		if strings.HasPrefix(e, "GITHUB_TOKEN=") {
			t.Errorf("ghEnv() unexpectedly set GITHUB_TOKEN when no PAT stored: %q", e)
		}
	}
}
