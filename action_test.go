//
// Date: 2026-06-15
// Author: Spicer Matthews (spicer@cloudmanic.com)
// Copyright: 2026 Cloudmanic Labs, LLC. All rights reserved.
//

package main

import (
	"strings"
	"testing"
)

// TestActionRender exercises command template rendering: explicit {{.Value}}
// placement, context variables, the urlquery helper, and the auto-append of a
// value when the template does not reference it.
func TestActionRender(t *testing.T) {
	cases := []struct {
		name    string
		action  Action
		ctx     RunContext
		want    string
		wantSub []string // substrings that must appear (when an exact match is brittle)
	}{
		{
			name:   "plain command unchanged",
			action: Action{Name: "GitHub", Command: "open https://github.com"},
			ctx:    RunContext{},
			want:   "open https://github.com",
		},
		{
			name:   "value substituted into template",
			action: Action{Name: "Repo", Type: TypeSelect, Command: "open https://github.com/cloudmanic/{{.Value}}"},
			ctx:    RunContext{Value: "herdr-plus"},
			want:   "open https://github.com/cloudmanic/herdr-plus",
		},
		{
			// The value is appended and quoted for the current platform's shell;
			// build the expected string with shellQuote so it holds on Windows too.
			name:   "value appended when template omits it",
			action: Action{Name: "Say", Type: TypeForm, Command: "say"},
			ctx:    RunContext{Value: "hi there"},
			want:   "say " + shellQuote("hi there"),
		},
		{
			name:   "value with single quote is shell-safe when appended",
			action: Action{Name: "Say", Type: TypeForm, Command: "say"},
			ctx:    RunContext{Value: "it's me"},
			want:   "say " + shellQuote("it's me"),
		},
		{
			name:   "workdir variable",
			action: Action{Name: "Reveal", Command: "open {{.WorkDir}}"},
			ctx:    RunContext{WorkDir: "/tmp/project"},
			want:   "open /tmp/project",
		},
		{
			name:   "session title method",
			action: Action{Name: "Echo", Command: "echo {{.SessionTitle}}"},
			ctx:    RunContext{WorkspaceLabel: "herdr-plus"},
			want:   "echo herdr-plus",
		},
		{
			name:    "urlquery escapes spaces",
			action:  Action{Name: "Search", Type: TypeForm, Command: "open 'https://g.co/s?q={{.Value | urlquery}}'"},
			ctx:     RunContext{Value: "hello world"},
			wantSub: []string{"hello", "world"},
		},
		{
			// {{opener}} renders to the host's open command (open/xdg-open/
			// Start-Process); assert against opener() so it holds on every OS.
			name:   "opener helper renders host open command",
			action: Action{Name: "Open", Command: "{{opener}} https://github.com"},
			ctx:    RunContext{},
			want:   opener() + " https://github.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.action.render(tc.ctx)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if tc.want != "" && got != tc.want {
				t.Fatalf("render = %q, want %q", got, tc.want)
			}
			for _, sub := range tc.wantSub {
				if !strings.Contains(got, sub) {
					t.Fatalf("render = %q, want substring %q", got, sub)
				}
			}
			if tc.name == "urlquery escapes spaces" && strings.Contains(got, "hello world") {
				t.Fatalf("render = %q, space was not escaped", got)
			}
		})
	}
}

// TestActionValidate confirms validation rejects incomplete or inconsistent
// action definitions and accepts well-formed ones.
func TestActionValidate(t *testing.T) {
	cases := []struct {
		name    string
		action  Action
		wantErr bool
	}{
		{"valid command", Action{Name: "A", Command: "open x"}, false},
		{"valid select", Action{Name: "A", Type: TypeSelect, Command: "open {{.Value}}", Options: []Option{{Label: "x"}}}, false},
		{"valid form", Action{Name: "A", Type: TypeForm, Command: "open {{.Value}}"}, false},
		{"missing name", Action{Command: "open x"}, true},
		{"missing command", Action{Name: "A"}, true},
		{"select without options", Action{Name: "A", Type: TypeSelect, Command: "x"}, true},
		{"unknown type", Action{Name: "A", Type: "wat", Command: "x"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.action.validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestOptionResolvedValue checks that an option falls back to its label when no
// explicit value is given.
func TestOptionResolvedValue(t *testing.T) {
	if got := (Option{Label: "Herdr Plus", Value: "herdr-plus"}).resolvedValue(); got != "herdr-plus" {
		t.Fatalf("resolvedValue = %q, want %q", got, "herdr-plus")
	}
	if got := (Option{Label: "herdr-plus"}).resolvedValue(); got != "herdr-plus" {
		t.Fatalf("resolvedValue = %q, want label fallback %q", got, "herdr-plus")
	}
}

// Shell quoting is OS-specific and lives in shell_test.go (posixQuote /
// powershellQuote are tested directly there, on every platform).

// TestActionCommandFor covers the optional per-OS command override. An action
// carries one portable `command` plus an optional [windows] block; a shell
// command written for sh does not parse under PowerShell, so a Windows build
// must be able to run a different string without forking the whole action file.
func TestActionCommandFor(t *testing.T) {
	posix := `make test; read -t 30 _ </dev/tty || true`
	win := `make test; Read-Host "press Enter"`

	cases := []struct {
		name   string
		action Action
		goos   string
		want   string
	}{
		{
			name:   "windows build uses the override when present",
			action: Action{Command: posix, Windows: OSCommand{Command: win}},
			goos:   "windows",
			want:   win,
		},
		{
			name:   "non-windows build ignores the override",
			action: Action{Command: posix, Windows: OSCommand{Command: win}},
			goos:   "linux",
			want:   posix,
		},
		{
			name:   "windows build falls back when no override is given",
			action: Action{Command: posix},
			goos:   "windows",
			want:   posix,
		},
		{
			name:   "a blank override does not shadow the base command",
			action: Action{Command: posix, Windows: OSCommand{Command: "   "}},
			goos:   "windows",
			want:   posix,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.action.commandFor(c.goos); got != c.want {
				t.Errorf("commandFor(%q) = %q, want %q", c.goos, got, c.want)
			}
		})
	}
}

// TestActionRenderUsesOSCommand makes sure the override is what actually gets
// templated and run, not just what commandFor reports. Rendering the base
// command on Windows is the exact bug this feature exists to prevent.
func TestActionRenderUsesOSCommand(t *testing.T) {
	a := Action{
		Name:    "demo",
		Command: "echo posix {{.Value}}",
		Windows: OSCommand{Command: "Write-Output windows {{.Value}}"},
	}

	got, err := a.renderFor(RunContext{Value: "v"}, "windows")
	if err != nil {
		t.Fatalf("renderFor: %v", err)
	}
	if want := "Write-Output windows v"; got != want {
		t.Errorf("renderFor(windows) = %q, want %q", got, want)
	}
}

// TestActionRenderAutoAppendUsesOSCommand pins a subtle case: the auto-append of
// an unreferenced value inspects the command string, and it must inspect the one
// actually being run. An override that omits {{.Value}} should still get the
// value appended even when the base command references it.
func TestActionRenderAutoAppendUsesOSCommand(t *testing.T) {
	a := Action{
		Name:    "demo",
		Command: "echo {{.Value}}",
		Windows: OSCommand{Command: "Write-Output"},
	}

	got, err := a.renderFor(RunContext{Value: "hello there"}, "windows")
	if err != nil {
		t.Fatalf("renderFor: %v", err)
	}
	if !strings.Contains(got, "Write-Output ") {
		t.Fatalf("renderFor(windows) = %q, want the windows command", got)
	}
	if !strings.Contains(got, "hello there") {
		t.Errorf("renderFor(windows) = %q, want the value appended", got)
	}
}
