//
// Date: 2026-07-23
// Author: Spicer Matthews (spicer@cloudmanic.com)
// Copyright: 2026 Cloudmanic Labs, LLC. All rights reserved.
//

package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestFindProject covers findProject's three resolution paths: an exact
// case-sensitive match, a case-insensitive fallback, and a miss that reports the
// available project names. A separate case checks the empty-set error.
func TestFindProject(t *testing.T) {
	projects := []Project{
		{Name: "app.harbor.my"},
		{Name: "harbor-sysadmin"},
	}

	// Exact match wins.
	got, err := findProject(projects, "harbor-sysadmin")
	if err != nil {
		t.Fatalf("exact match: unexpected error: %v", err)
	}
	if got.Name != "harbor-sysadmin" {
		t.Fatalf("exact match: got %q, want %q", got.Name, "harbor-sysadmin")
	}

	// Case-insensitive fallback resolves when the exact case differs.
	got, err = findProject(projects, "Harbor-Sysadmin")
	if err != nil {
		t.Fatalf("case-insensitive match: unexpected error: %v", err)
	}
	if got.Name != "harbor-sysadmin" {
		t.Fatalf("case-insensitive match: got %q, want %q", got.Name, "harbor-sysadmin")
	}

	// A miss names the bad input and lists every available project.
	_, err = findProject(projects, "nope")
	if err == nil {
		t.Fatal("no match: expected an error, got nil")
	}
	for _, want := range []string{"nope", "app.harbor.my", "harbor-sysadmin"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("no match: error %q is missing %q", err.Error(), want)
		}
	}

	// An empty project set returns a distinct, clear error.
	if _, err := findProject(nil, "anything"); err == nil {
		t.Fatal("empty set: expected an error, got nil")
	}
}

// TestHerdrManagedConfigDir confirms herdrManagedConfigDir returns herdr's
// (whitespace-trimmed) output on success and "" when the herdr binary fails,
// using a fake herdr located via HERDR_BIN_PATH so the test never needs a real one.
func TestHerdrManagedConfigDir(t *testing.T) {
	// Success: fake herdr prints a path with surrounding whitespace.
	t.Setenv("HERDR_BIN_PATH", installFakeHerdr(t, "  /managed/dir  \n", 0))
	if got := herdrManagedConfigDir(); got != "/managed/dir" {
		t.Fatalf("herdrManagedConfigDir = %q, want %q", got, "/managed/dir")
	}

	// Failure: fake herdr exits non-zero, so we get "".
	t.Setenv("HERDR_BIN_PATH", installFakeHerdr(t, "", 1))
	if got := herdrManagedConfigDir(); got != "" {
		t.Fatalf("herdrManagedConfigDir on failure = %q, want empty", got)
	}
}

// TestEnsureManagedConfigDir confirms ensureManagedConfigDir exports the queried
// directory when HERDR_PLUGIN_CONFIG_DIR is unset, and leaves an already-set value
// untouched (never consulting herdr in that case).
func TestEnsureManagedConfigDir(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", installFakeHerdr(t, "/queried/dir\n", 0))

	// Unset → the queried directory is exported.
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "")
	ensureManagedConfigDir()
	if got := os.Getenv("HERDR_PLUGIN_CONFIG_DIR"); got != "/queried/dir" {
		t.Fatalf("unset case: HERDR_PLUGIN_CONFIG_DIR = %q, want %q", got, "/queried/dir")
	}

	// Already set → left exactly as-is.
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "/preset/dir")
	ensureManagedConfigDir()
	if got := os.Getenv("HERDR_PLUGIN_CONFIG_DIR"); got != "/preset/dir" {
		t.Fatalf("preset case: HERDR_PLUGIN_CONFIG_DIR = %q, want %q", got, "/preset/dir")
	}
}

// installFakeHerdr points the code under test at a stand-in herdr that prints
// output and exits with exitCode, returning the path to use as HERDR_BIN_PATH.
// The stand-in is this very test binary, re-executed with the fake-herdr
// variables set (see TestMain); the child ignores its arguments, so it serves any
// `herdr ...` invocation the code under test makes. t.Setenv unsets both
// variables when the test ends.
func installFakeHerdr(t *testing.T, output string, exitCode int) string {
	t.Helper()

	t.Setenv(fakeHerdrOutputEnv, output)
	t.Setenv(fakeHerdrExitEnv, strconv.Itoa(exitCode))

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	return exe
}
