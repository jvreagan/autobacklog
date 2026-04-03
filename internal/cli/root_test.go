package cli

import "testing"

func TestNewRootCmd_HasSubcommands(t *testing.T) {
	cmd := NewRootCmd()

	expected := []string{"run", "daemon", "status", "init", "version"}
	commands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		commands[sub.Name()] = true
	}

	for _, name := range expected {
		if !commands[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}
