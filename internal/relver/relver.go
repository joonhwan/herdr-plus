//
// Date: 2026-08-17
// Author: Joonhwan Lee (joonhwan.lee@mirero.co.kr)
// Copyright: 2026 Cloudmanic Labs, LLC. All rights reserved.
//

// Package relver computes the version string this fork's release pipeline stamps
// into a build, and rewrites the two files that carry it.
//
// It exists as a Go package rather than sed in a workflow step for two reasons:
// the string rules below (base extraction, the semver-safe stamp layout) are
// worth unit tests, and a Go tool behaves identically on every runner OS while a
// shell step does not.
//
// Every function here is a pure string transform. Reading and writing the files
// is the CLI's job (internal/tools/relver), which keeps this package trivially
// testable.
package relver

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	// The stamp is rendered in Asia/Seoul, and a CI runner may carry no system
	// timezone database. Embedding Go's copy makes LoadLocation work anywhere.
	_ "time/tzdata"
)

// SeoulZone is the timezone the release stamp is rendered in. CI runners are
// UTC, but the stamp is read by a person deciding which build their PC is
// running, so it uses their wall clock rather than the runner's.
const SeoulZone = "Asia/Seoul"

// prereleaseTag is the prerelease identifier every version this pipeline stamps
// carries, mirroring herdr's own 0.8.0-nightly.… scheme.
const prereleaseTag = "nightly"

// versionGoRe matches the version.go line this pipeline owns. It captures the
// quoted value loosely (anything but a quote) so it matches both a plain
// upstream version and one this pipeline already stamped.
var versionGoRe = regexp.MustCompile(`(const Version = ")([^"]*)(")`)

// manifestVersionRe matches herdr-plugin.toml's top-level version key. It is
// anchored to the start of a line so it cannot match min_herdr_version or a key
// nested under a [[section]] — rewriting either of those would change what the
// plugin demands of herdr.
var manifestVersionRe = regexp.MustCompile(`(?m)^(version = ")([^"]*)(")`)

// BaseVersion extracts the upstream X.Y.Z from a version.go source, discarding
// any prerelease this pipeline previously stamped.
//
// Dropping the old prerelease is what makes the workflow idempotent: every run
// derives its stamp from the upstream base, so stamps never nest and a merge that
// raises the base is picked up automatically.
func BaseVersion(versionGoSrc string) (string, error) {
	m := versionGoRe.FindStringSubmatch(versionGoSrc)
	if m == nil {
		return "", fmt.Errorf("no `const Version = \"...\"` found in version.go source")
	}

	base, _, _ := strings.Cut(m[2], "-")
	if base == "" {
		return "", fmt.Errorf("version %q has no base X.Y.Z component", m[2])
	}
	return base, nil
}

// Stamp renders t as the YYYYMMDD-HHMM identifier that distinguishes one nightly
// build from the next, converting to Seoul time first.
//
// The date is unseparated on purpose. Semver forbids leading zeroes in a purely
// numeric prerelease identifier, so a dotted 2026.08.17 would make "08" invalid;
// joined, the hyphen makes the whole identifier alphanumeric and the constraint
// disappears. A side benefit is that lexical order matches chronological order.
func Stamp(t time.Time) string {
	loc, err := time.LoadLocation(SeoulZone)
	if err != nil {
		// Unreachable with time/tzdata embedded, but falling back to the given
		// time's own zone still produces a usable stamp rather than a panic.
		return t.Format("20060102-1504")
	}
	return t.In(loc).Format("20060102-1504")
}

// Compose joins an upstream base version and a stamp into the full version the
// release is tagged and built with.
func Compose(base, stamp string) string {
	return fmt.Sprintf("%s-%s.%s", base, prereleaseTag, stamp)
}

// RewriteVersionGo returns src with the version constant replaced by full,
// leaving every other byte untouched so the commit stays a reviewable one-liner.
func RewriteVersionGo(src, full string) (string, error) {
	if !versionGoRe.MatchString(src) {
		return "", fmt.Errorf("no `const Version = \"...\"` found in version.go source")
	}
	return versionGoRe.ReplaceAllString(src, "${1}"+full+"${3}"), nil
}

// RewriteManifest returns src with herdr-plugin.toml's top-level version key
// replaced by full. Only that key is touched — see manifestVersionRe.
func RewriteManifest(src, full string) (string, error) {
	if !manifestVersionRe.MatchString(src) {
		return "", fmt.Errorf("no top-level `version = \"...\"` key found in manifest source")
	}
	return manifestVersionRe.ReplaceAllString(src, "${1}"+full+"${3}"), nil
}
