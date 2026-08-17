//
// Date: 2026-08-17
// Author: Joonhwan Lee (joonhwan.lee@mirero.co.kr)
// Copyright: 2026 Cloudmanic Labs, LLC. All rights reserved.
//

package main

import (
	"fmt"
	"os"
	"strconv"
	"testing"
)

// The tests need a stand-in for the herdr binary that prints a canned answer and
// exits with a chosen code. Writing that stand-in as a shell script does not work
// on Windows: there is no shebang, so CreateProcessW cannot launch a .sh file at
// all. Instead the test binary re-executes *itself* as the fake herdr — the
// pattern os/exec's own tests use. It needs no shell, no .cmd shim, and no Git
// for Windows, so the same code path runs on every OS we build for.
//
// These two variables are the switch. TestMain reads them before running any
// test: when the output variable is present, this process is the re-executed
// stand-in rather than a real test run.
const (
	fakeHerdrOutputEnv = "HERDR_PLUS_TEST_FAKE_HERDR_OUTPUT"
	fakeHerdrExitEnv   = "HERDR_PLUS_TEST_FAKE_HERDR_EXIT"
)

// TestMain intercepts the re-executed stand-in before the test framework starts.
// Exiting here — rather than letting m.Run() proceed — matters for two reasons:
// the child would otherwise try to parse herdr's arguments as test flags, and it
// would run the whole suite recursively.
func TestMain(m *testing.M) {
	if out, ok := os.LookupEnv(fakeHerdrOutputEnv); ok {
		fmt.Print(out)
		// A malformed or absent exit code means success; the tests that care set
		// it explicitly.
		code, _ := strconv.Atoi(os.Getenv(fakeHerdrExitEnv))
		os.Exit(code)
	}
	os.Exit(m.Run())
}
