//
// Date: 2026-06-15
// Author: Spicer Matthews (spicer@cloudmanic.com)
// Copyright: 2026 Cloudmanic Labs, LLC. All rights reserved.
//

// Tests for the tiny version package. The Version constant is the single source
// of truth this fork's release pipeline bumps, so we fail loudly if its shape
// ever drifts away from what that pipeline produces.

package version

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// nightlyPreRe matches the only prerelease identifier this fork's pipeline
// stamps: internal/relver composes `<base>-nightly.YYYYMMDD-HHMM`. Pinning the
// exact shape rather than allowing any prerelease means a hand-edit or a typo in
// the constant still fails here instead of shipping.
var nightlyPreRe = regexp.MustCompile(`^nightly\.[0-9]{8}-[0-9]{4}$`)

// splitVersion separates Version into its X.Y.Z base and its prerelease, which
// is empty for a plain upstream version.
func splitVersion(v string) (base, pre string) {
	base, pre, _ = strings.Cut(v, "-")
	return base, pre
}

// TestVersionNotEmpty makes sure the constant has a value at all.
func TestVersionNotEmpty(t *testing.T) {
	if Version == "" {
		t.Fatal("Version is empty")
	}
}

// TestVersionBaseIsSemver verifies the constant's base is in major.minor.patch
// form so relver.BaseVersion's auto-bump logic stays correct.
func TestVersionBaseIsSemver(t *testing.T) {
	base, _ := splitVersion(Version)

	parts := strings.Split(base, ".")
	if len(parts) != 3 {
		t.Fatalf("Version %q base %q is not in x.y.z form (got %d parts)", Version, base, len(parts))
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("Version %q base part %d (%q) is not an integer: %v", Version, i, p, err)
		}
		if n < 0 {
			t.Fatalf("Version %q base part %d (%q) is negative", Version, i, p)
		}
	}
}

// TestVersionPrereleaseShape allows the constant to be either a plain upstream
// version or one this fork's pipeline stamped, and nothing else. A release build
// carries the nightly prerelease; a freshly merged upstream base carries none.
func TestVersionPrereleaseShape(t *testing.T) {
	_, pre := splitVersion(Version)
	if pre == "" {
		return
	}
	if !nightlyPreRe.MatchString(pre) {
		t.Fatalf("Version %q prerelease %q is not the pipeline's nightly.YYYYMMDD-HHMM form", Version, pre)
	}
}

// TestVersionPreOneZero pins the major version at 0 while we are still pre-1.0.
// Bumping past 0 is a deliberate event, so update this test on purpose rather
// than letting the constant drift.
func TestVersionPreOneZero(t *testing.T) {
	base, _ := splitVersion(Version)

	major, err := strconv.Atoi(strings.Split(base, ".")[0])
	if err != nil {
		t.Fatalf("Version %q has non-numeric major: %v", Version, err)
	}
	if major != 0 {
		t.Fatalf("Version %q major is %d, expected 0 (pre-1.0). Update this test deliberately when shipping 1.0.", Version, major)
	}
}

// TestSplitVersionCases locks in how the helper above splits the two shapes the
// constant is ever allowed to take, so the tests that depend on it cannot pass
// for the wrong reason.
func TestSplitVersionCases(t *testing.T) {
	cases := []struct {
		in       string
		wantBase string
		wantPre  string
	}{
		{"0.1.20", "0.1.20", ""},
		{"0.1.20-nightly.20260817-0946", "0.1.20", "nightly.20260817-0946"},
	}
	for _, c := range cases {
		base, pre := splitVersion(c.in)
		if base != c.wantBase || pre != c.wantPre {
			t.Errorf("splitVersion(%q) = (%q, %q), want (%q, %q)", c.in, base, pre, c.wantBase, c.wantPre)
		}
	}
}

// TestNightlyPreReRejectsDrift makes sure the prerelease pattern is tight enough
// to catch the ways a stamp realistically goes wrong.
func TestNightlyPreReRejectsDrift(t *testing.T) {
	bad := []string{
		"nightly",                     // no stamp
		"nightly.20260817",            // date only, no time
		"nightly.2026081-0946",        // short date
		"nightly.20260817-946",        // short time
		"nightly.20260817-0946.extra", // nested stamp
		"beta.20260817-0946",          // wrong tag
		"nightly.20260817_0946",       // wrong separator
	}
	for _, s := range bad {
		if nightlyPreRe.MatchString(s) {
			t.Errorf("nightlyPreRe accepted %q, expected it to be rejected", s)
		}
	}

	if !nightlyPreRe.MatchString("nightly.20260817-0946") {
		t.Error("nightlyPreRe rejected a well-formed stamp")
	}
}
