//
// Date: 2026-08-17
// Author: Joonhwan Lee (joonhwan.lee@mirero.co.kr)
// Copyright: 2026 Cloudmanic Labs, LLC. All rights reserved.
//

package relver

import (
	"strings"
	"testing"
	"time"
)

// TestBaseVersion covers the two shapes version.go can be in when the release
// workflow reads it: a plain upstream version, and one this pipeline already
// stamped. Stripping the prerelease is what makes the workflow idempotent — the
// stamp is always derived from the upstream base, never from the previous stamp.
func TestBaseVersion(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "plain upstream version",
			src:  "package version\n\nconst Version = \"0.1.20\"\n",
			want: "0.1.20",
		},
		{
			name: "already stamped by this pipeline",
			src:  "package version\n\nconst Version = \"0.1.20-nightly.20260817-0843\"\n",
			want: "0.1.20",
		},
		{
			name: "surrounding comments and blank lines are ignored",
			src:  "// Version is the release version.\nconst Version = \"1.2.3\" // trailing\n",
			want: "1.2.3",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BaseVersion(tc.src)
			if err != nil {
				t.Fatalf("BaseVersion: %v", err)
			}
			if got != tc.want {
				t.Fatalf("BaseVersion = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBaseVersionMissing confirms a source with no parsable version fails loudly
// rather than yielding an empty base that would produce the nonsense tag
// "v-nightly.…".
func TestBaseVersionMissing(t *testing.T) {
	if _, err := BaseVersion("package version\n"); err == nil {
		t.Fatal("expected an error for a source with no const Version")
	}
}

// TestStamp pins the exact stamp layout. The date is unseparated on purpose:
// semver forbids leading zeroes in a purely numeric prerelease identifier, so a
// dotted 2026.08.17 would make "08" invalid and goreleaser could reject the tag.
func TestStamp(t *testing.T) {
	seoul, err := time.LoadLocation(SeoulZone)
	if err != nil {
		t.Fatalf("load %s: %v", SeoulZone, err)
	}

	got := Stamp(time.Date(2026, 8, 17, 8, 43, 12, 0, seoul))
	if want := "20260817-0843"; got != want {
		t.Fatalf("Stamp = %q, want %q", got, want)
	}
}

// TestStampConvertsToSeoul proves the stamp reflects Seoul wall-clock time even
// when handed a UTC instant. CI runners are UTC, but the stamp exists for a human
// deciding which build their PC is running, so it must read in their timezone.
func TestStampConvertsToSeoul(t *testing.T) {
	// 2026-08-16 23:30 UTC is 2026-08-17 08:30 in Seoul — a different day.
	got := Stamp(time.Date(2026, 8, 16, 23, 30, 0, 0, time.UTC))
	if want := "20260817-0830"; got != want {
		t.Fatalf("Stamp = %q, want %q", got, want)
	}
}

// TestCompose pins the full version string the tag is built from.
func TestCompose(t *testing.T) {
	got := Compose("0.1.20", "20260817-0843")
	if want := "0.1.20-nightly.20260817-0843"; got != want {
		t.Fatalf("Compose = %q, want %q", got, want)
	}
}

// TestRewriteVersionGo checks the rewrite replaces the version and leaves every
// other byte alone — the file is the single source of truth a reviewer reads, so
// the diff must stay a one-liner.
func TestRewriteVersionGo(t *testing.T) {
	src := "// comment\npackage version\n\nconst Version = \"0.1.20\"\n"

	got, err := RewriteVersionGo(src, "0.1.20-nightly.20260817-0843")
	if err != nil {
		t.Fatalf("RewriteVersionGo: %v", err)
	}

	want := "// comment\npackage version\n\nconst Version = \"0.1.20-nightly.20260817-0843\"\n"
	if got != want {
		t.Fatalf("RewriteVersionGo =\n%q\nwant\n%q", got, want)
	}
}

// TestRewriteVersionGoIdempotent confirms rewriting an already-stamped source
// replaces the stamp rather than appending to it.
func TestRewriteVersionGoIdempotent(t *testing.T) {
	src := "const Version = \"0.1.20-nightly.20260817-0843\"\n"

	got, err := RewriteVersionGo(src, "0.1.20-nightly.20260818-0910")
	if err != nil {
		t.Fatalf("RewriteVersionGo: %v", err)
	}
	if want := "const Version = \"0.1.20-nightly.20260818-0910\"\n"; got != want {
		t.Fatalf("RewriteVersionGo = %q, want %q", got, want)
	}
}

// TestRewriteManifest checks only the top-level version key is touched. The
// manifest also contains min_herdr_version and per-entry keys; rewriting the
// wrong one would silently change what herdr requires of the host.
func TestRewriteManifest(t *testing.T) {
	src := `id = "cloudmanic.herdr-plus"
version = "0.1.20"
min_herdr_version = "0.7.0"
`

	got, err := RewriteManifest(src, "0.1.20-nightly.20260817-0843")
	if err != nil {
		t.Fatalf("RewriteManifest: %v", err)
	}

	if !strings.Contains(got, `version = "0.1.20-nightly.20260817-0843"`) {
		t.Fatalf("manifest version was not rewritten:\n%s", got)
	}
	if !strings.Contains(got, `min_herdr_version = "0.7.0"`) {
		t.Fatalf("min_herdr_version must not change:\n%s", got)
	}
}

// TestRewriteManifestMissing confirms a manifest with no version key is an error
// rather than a silent no-op that would ship an unstamped plugin.
func TestRewriteManifestMissing(t *testing.T) {
	if _, err := RewriteManifest("id = \"x\"\n", "1.0.0"); err == nil {
		t.Fatal("expected an error for a manifest with no version key")
	}
}
