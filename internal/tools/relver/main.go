//
// Date: 2026-08-17
// Author: Joonhwan Lee (joonhwan.lee@mirero.co.kr)
// Copyright: 2026 Cloudmanic Labs, LLC. All rights reserved.
//

// Command relver computes this fork's next nightly version and, with -write,
// stamps it into the files that carry it. It is invoked by
// .github/workflows/release-fork.yml:
//
//	go run ./internal/tools/relver -write -manifest
//
// The full version goes to stdout on its own line so the workflow can capture it
// with a command substitution; everything else this tool says goes to stderr.
//
// All the interesting logic lives in internal/relver, which is unit-tested. This
// file is only argument parsing and file I/O.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/cloudmanic/herdr-plus/internal/relver"
)

// The two files that carry the release version. Paths are relative to the repo
// root, which is where the workflow runs this tool.
const (
	versionGoPath = "internal/version/version.go"
	manifestPath  = "herdr-plugin.toml"
)

func main() {
	write := flag.Bool("write", false, "stamp the computed version into version.go")
	manifest := flag.Bool("manifest", false, "also stamp herdr-plugin.toml (requires -write)")
	flag.Parse()

	versionGoSrc, err := os.ReadFile(versionGoPath)
	if err != nil {
		fatal("read %s: %v", versionGoPath, err)
	}

	base, err := relver.BaseVersion(string(versionGoSrc))
	if err != nil {
		fatal("%s: %v", versionGoPath, err)
	}

	full := relver.Compose(base, relver.Stamp(time.Now()))

	if *write {
		stamped, err := relver.RewriteVersionGo(string(versionGoSrc), full)
		if err != nil {
			fatal("%s: %v", versionGoPath, err)
		}
		// 0o644 matches what the file already carries; os.WriteFile only applies
		// the mode when creating, and this file always exists.
		if err := os.WriteFile(versionGoPath, []byte(stamped), 0o644); err != nil {
			fatal("write %s: %v", versionGoPath, err)
		}
		fmt.Fprintf(os.Stderr, "relver: %s -> %s\n", versionGoPath, full)

		if *manifest {
			src, err := os.ReadFile(manifestPath)
			if err != nil {
				fatal("read %s: %v", manifestPath, err)
			}
			stamped, err := relver.RewriteManifest(string(src), full)
			if err != nil {
				fatal("%s: %v", manifestPath, err)
			}
			if err := os.WriteFile(manifestPath, []byte(stamped), 0o644); err != nil {
				fatal("write %s: %v", manifestPath, err)
			}
			fmt.Fprintf(os.Stderr, "relver: %s -> %s\n", manifestPath, full)
		}
	}

	// stdout carries exactly one thing: the version. Keep it that way.
	fmt.Println(full)
}

// fatal reports a problem and exits non-zero, so a broken release stops the
// workflow instead of tagging something wrong.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "relver: "+format+"\n", args...)
	os.Exit(1)
}
