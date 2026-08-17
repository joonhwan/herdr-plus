# fork 전용 Windows 릴리스 파이프라인 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `joonhwan/herdr-plus` fork의 `windows-support` 브랜치에서 Windows 바이너리를 자동 릴리스하고, 두 대의 Windows PC에서 `herdr plugin install`로 설치해 문제없이 쓸 수 있게 한다.

**Architecture:** Windows CI를 먼저 세워 검증 기반을 만든 뒤, 릴리스 버전 계산을 셸 스크립트가 아닌 **Go 도구**(`internal/relver` 라이브러리 + 얇은 CLI)로 구현한다. fork 전용 파일(`.goreleaser.fork.yml`, `release-fork.yml`)은 upstream 파일을 수정하지 않고 새로 추가해 머지 충돌을 없앤다.

**Tech Stack:** Go 1.26, GitHub Actions, GoReleaser v2, herdr 0.8.0-nightly 플러그인 시스템

**Spec:** `docs/superpowers/specs/2026-08-17-fork-windows-release-design.md`
(선행 조사: `docs/superpowers/specs/2026-08-17-windows-support-audit.md`)

## Global Constraints

- 작업 브랜치는 `windows-support`. fork의 `main`에는 **절대 push하지 않는다** (upstream `release.yml`이 걸려 있어 실패한다).
- **upstream 파일을 수정하지 말 것.** fork 전용 변경은 새 파일로 추가한다. 예외: `test.yml`과 `open_test.go`는 upstream에도 유효한 수정이므로 고치되, **fork 전용 커밋과 섞지 않고 독립 커밋**으로 만든다 (나중에 upstream PR로 떼어내기 위함).
- 태그 형식: `v<base>-nightly.<YYYYMMDD>-<HHMM>` (예: `v0.1.20-nightly.20260817-0843`). 날짜는 점 없이 붙여 쓴다 — semver는 순수 숫자 prerelease 식별자의 leading zero를 금지하므로 `2026.08.17`의 `08`이 위반이다.
- 타임스탬프 타임존은 **Asia/Seoul** 고정.
- 릴리스 빌드 대상은 **windows/amd64 단독**.
- 셸 스크립트보다 **Go 크로스플랫폼 도구**를 우선한다. GitHub Actions의 `run:` 스텝처럼 불가피한 곳만 셸을 쓰고, 로직은 Go 쪽에 둔다.
- 기존 코드 주석 스타일을 따른다: 파일 상단 `Date/Author/Copyright` 헤더, 모든 exported/비자명 심볼에 **무엇을 하는지가 아니라 왜 그런지**를 설명하는 주석.
- 커밋은 각 Task 끝에서 한 번. 커밋 메시지는 영어.

---

### Task 1: Windows에서 실패하는 테스트 픽스처 수정

`open_test.go`의 `writeFakeHerdr`가 가짜 herdr을 `#!/bin/sh` 스크립트로 쓴다. Windows에는 셔뱅이 없어 `exec.Command`가 실행하지 못하고 `TestHerdrManagedConfigDir` / `TestEnsureManagedConfigDir`이 실패한다.

해결책은 **테스트 바이너리가 자기 자신을 가짜 herdr로 재실행**하는 Go 표준 패턴이다 (`os/exec` 패키지 테스트가 쓰는 방식). 셸도, `.cmd` 심도, Git for Windows 의존도 없다.

**Files:**
- Modify: `open_test.go:60-108` (두 테스트의 헬퍼 호출부 + `writeFakeHerdr` 교체)
- Create: `main_test.go` (`TestMain` — 패키지에 아직 없다)

**Interfaces:**
- Consumes: 없음
- Produces: `installFakeHerdr(t *testing.T, output string, exitCode int) string` — 가짜 herdr로 쓸 실행 파일 경로를 돌려주고, 그 동작을 환경변수로 심는다. 상수 `fakeHerdrOutputEnv`, `fakeHerdrExitEnv`.

- [ ] **Step 1: `TestMain`을 담을 새 파일을 만든다**

`main_test.go`를 새로 만든다:

```go
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
```

- [ ] **Step 2: 헬퍼를 교체한다**

`open_test.go` 끝의 `writeFakeHerdr`(96-108행)를 지우고 다음으로 바꾼다:

```go
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
```

import 블록도 함께 고친다. `filepath.Join`은 이 파일에서 103행 한 곳에서만 쓰였고 그 줄이 사라지므로 `"path/filepath"`를 **지우고**, `strconv.Itoa` 때문에 `"strconv"`를 **넣는다**. `"os"`와 `"strings"`는 다른 곳에서 계속 쓰이니 그대로 둔다. 결과:

```go
import (
	"os"
	"strconv"
	"strings"
	"testing"
)
```

- [ ] **Step 3: 세 곳의 호출부를 고친다**

`open_test.go:65`:
```go
	t.Setenv("HERDR_BIN_PATH", installFakeHerdr(t, "  /managed/dir  \n", 0))
```

`open_test.go:71`:
```go
	t.Setenv("HERDR_BIN_PATH", installFakeHerdr(t, "", 1))
```

`open_test.go:81`:
```go
	t.Setenv("HERDR_BIN_PATH", installFakeHerdr(t, "/queried/dir\n", 0))
```

- [ ] **Step 4: 헬퍼 주석의 낡은 설명을 고친다**

`open_test.go:96-98`의 "writes an executable shell script" 문구는 Step 2에서 이미 교체된다. `TestHerdrManagedConfigDir`(59-62행) 독스트링의 "using a fake herdr located via HERDR_BIN_PATH so the test never needs a real one" 부분은 그대로 유효하니 남긴다.

- [ ] **Step 5: 테스트를 돌려 통과를 확인한다**

Run: `go test -run 'TestHerdrManagedConfigDir|TestEnsureManagedConfigDir' -v ./...`
Expected: 두 테스트 모두 PASS. (수정 전에는 `herdrManagedConfigDir = "", want "/managed/dir"`로 실패했다.)

- [ ] **Step 6: 전체 스위트를 돌린다**

Run: `go test ./...`
Expected: 전부 PASS. `TestMain`이 정상 경로에서 `m.Run()`을 그대로 부르는지 여기서 검증된다 — 하나라도 실행되지 않으면 `TestMain` 분기가 잘못된 것이다.

- [ ] **Step 7: 커밋**

upstream PR로 떼어낼 수 있게 이 커밋에는 fork 전용 변경을 섞지 않는다.

```bash
git add open_test.go main_test.go
git commit -F - <<'EOF'
Make the fake herdr fixture work on Windows

writeFakeHerdr wrote its stand-in as a #!/bin/sh script, which Windows
cannot launch at all — there is no shebang, so CreateProcessW refuses the
file and TestHerdrManagedConfigDir/TestEnsureManagedConfigDir both fail
on a Windows checkout.

The test binary now re-executes itself as the stand-in, the pattern
os/exec's own tests use. Two environment variables carry the canned
output and exit code, and TestMain intercepts them before the framework
starts. No shell, no .cmd shim, no Git for Windows — one code path on
every OS we build for.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

### Task 2: CI 매트릭스에 Windows 추가

Windows 전용 코드(`herdrdial_windows.go`, `powershellQuote`, `paneEntrypoint`의 windows 분기, PowerShell 라운드트립 통합 테스트)가 지금 CI에서 **컴파일조차 되지 않는다.** Task 1의 실패가 여태 안 잡힌 직접적인 원인이다.

**Files:**
- Modify: `.github/workflows/test.yml:33-37` (매트릭스), `:47-56` (스텝)

**Interfaces:**
- Consumes: Task 1의 픽스처 수정 (이게 없으면 Windows 잡이 빨간불로 시작한다)
- Produces: 없음

- [ ] **Step 1: 매트릭스에 windows-latest를 넣는다**

`.github/workflows/test.yml`의 매트릭스를 이렇게 바꾼다:

```yaml
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
```

- [ ] **Step 2: tidy 검사 스텝을 Windows에서 건너뛴다**

"Verify go.mod is tidy" 스텝은 `git diff`의 종료 코드에 기대는 bash 스크립트다. Windows 러너의 기본 셸은 PowerShell이라 그대로 두면 깨진다. 어차피 tidy 여부는 OS와 무관하니 한 곳에서만 검사하면 충분하다. 해당 스텝에 조건을 단다:

```yaml
      # go.mod tidiness does not vary by OS, so one runner checking it is
      # enough. It is skipped elsewhere because this step is a bash script and
      # the Windows runner defaults to PowerShell.
      - name: Verify go.mod is tidy
        if: matrix.os == 'ubuntu-latest'
        run: |
```

(`run:` 블록 본문은 그대로 둔다.)

- [ ] **Step 3: Windows에서 -race를 분리한다**

Windows의 race detector는 cgo(gcc)를 요구한다. `windows-latest` 러너에는 mingw가 있어 대체로 동작하지만, 여기에 걸려 릴리스가 막히면 곤란하다. Test 스텝을 둘로 나눈다:

```yaml
      # The race detector needs cgo (gcc) on Windows. The runner ships mingw so
      # it usually works, but a toolchain hiccup there would block a platform
      # whose real risk is the OS-specific code paths, not data races — so
      # Windows runs the suite without -race.
      - name: Test
        if: matrix.os != 'windows-latest'
        run: go test -race ./...

      - name: Test (Windows, no race detector)
        if: matrix.os == 'windows-latest'
        run: go test ./...
```

- [ ] **Step 4: 워크플로 상단 주석을 갱신한다**

파일 헤더의 "on Linux + macOS"를 고친다:

```
# Test pipeline for herdr-plus. Runs the build, vet, and `go test ./...` with the
# race detector on every push and pull request, on Linux + macOS + Windows, so we
# catch platform-specific issues before they ship. Windows matters most here: it
# is the only runner that compiles the //go:build windows files at all.
```

- [ ] **Step 5: 변경 범위를 확인한다**

Run: `git diff --stat`
Expected: `.github/workflows/test.yml` 한 파일만 변경. YAML 문법을 검증할 로컬 린터가 없으므로, 실제 확인은 Step 7의 push로 한다.

- [ ] **Step 6: 커밋**

```bash
git add .github/workflows/test.yml
git commit -F - <<'EOF'
Run CI on Windows

The matrix covered Linux and macOS only, so every //go:build windows file
— the named-pipe dialer, the PowerShell quoting, the pane-entrypoint
suffix — was never compiled by CI, let alone tested. That is why the
fixture bug in the previous commit went unnoticed.

The tidy check stays on Ubuntu alone (it is a bash step, and tidiness
does not vary by OS), and Windows runs without -race because the
detector needs cgo there.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

- [ ] **Step 7: push하고 CI를 확인한다**

```bash
git push -u origin windows-support
```

Expected: Test 워크플로가 3개 OS에서 모두 초록불. Windows 잡이 빨간불이면 Task 3으로 넘어가지 말고 여기서 고친다 — 이 파이프라인이 이후 모든 작업의 안전망이다.

주의: Release 워크플로는 `main` push에만 걸려 있으므로 이 push로 트리거되지 않는다.

---

### Task 3: 릴리스 버전 계산 라이브러리

버전 계산을 워크플로 YAML 안의 `sed`/`bash`가 아니라 Go 라이브러리로 만든다. 순수 문자열 변환이라 단위 테스트가 쉽고, 러너 OS와 무관하게 같은 결과를 낸다.

**Files:**
- Create: `internal/relver/relver.go`
- Test: `internal/relver/relver_test.go`

**Interfaces:**
- Consumes: 없음
- Produces:
  - `func BaseVersion(versionGoSrc string) (string, error)` — `version.go` 소스에서 prerelease를 뗀 `X.Y.Z`를 뽑는다
  - `func Stamp(t time.Time) string` — `20260817-0843`
  - `func Compose(base, stamp string) string` — `0.1.20-nightly.20260817-0843`
  - `func RewriteVersionGo(src, full string) (string, error)` — `const Version = "..."`를 교체한 소스를 돌려준다
  - `func RewriteManifest(src, full string) (string, error)` — `herdr-plugin.toml`의 `version = "..."`를 교체한 소스를 돌려준다
  - `const SeoulZone = "Asia/Seoul"`

- [ ] **Step 1: 실패하는 테스트를 쓴다**

`internal/relver/relver_test.go`:

```go
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
```

- [ ] **Step 2: 테스트가 실패하는지 확인한다**

Run: `go test ./internal/relver/`
Expected: 컴파일 실패 — `undefined: BaseVersion` 등.

- [ ] **Step 3: 라이브러리를 구현한다**

`internal/relver/relver.go`:

```go
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
```

- [ ] **Step 4: 테스트를 돌려 통과를 확인한다**

Run: `go test ./internal/relver/ -v`
Expected: 9개 테스트 전부 PASS.

- [ ] **Step 5: 전체 스위트와 vet을 돌린다**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: 전부 통과.

- [ ] **Step 6: 커밋**

```bash
git add internal/relver
git commit -F - <<'EOF'
Add relver, the fork release version calculator

The fork's release pipeline needs to turn the upstream version in
version.go into a stamped nightly version, and rewrite the two files that
carry it. Doing that with sed inside a workflow step would be both
untestable and runner-dependent, so it lives here as pure string
transforms with unit tests.

Two rules are worth calling out. BaseVersion strips any prerelease before
composing a new one, which keeps the workflow idempotent — stamps never
nest, and an upstream merge that raises the base is picked up for free.
And Stamp writes the date unseparated because semver forbids leading
zeroes in a purely numeric prerelease identifier, which a dotted
2026.08.17 would trip on.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

### Task 4: relver CLI

라이브러리를 워크플로가 부를 수 있는 실행 파일로 감싼다. I/O만 담당하는 얇은 층이다.

**Files:**
- Create: `internal/tools/relver/main.go`

**Interfaces:**
- Consumes: Task 3의 `relver.BaseVersion`, `relver.Stamp`, `relver.Compose`, `relver.RewriteVersionGo`, `relver.RewriteManifest`
- Produces: `go run ./internal/tools/relver [-write] [-manifest]` — 계산한 전체 버전을 stdout에 한 줄로 출력한다

- [ ] **Step 1: CLI를 구현한다**

`internal/tools/relver/main.go`:

```go
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
```

- [ ] **Step 2: 읽기 전용 모드를 실측한다**

Run: `go run ./internal/tools/relver`
Expected: `0.1.20-nightly.<오늘날짜>-<현재시각>` 한 줄이 출력되고, `git status`는 깨끗하다 (`-write` 없이는 파일을 건드리지 않는다).

- [ ] **Step 3: 쓰기 모드를 실측하고 되돌린다**

```bash
go run ./internal/tools/relver -write -manifest
git diff --stat
```
Expected: `internal/version/version.go`와 `herdr-plugin.toml` 두 파일만 변경. 각각 한 줄씩.

되돌린다:
```bash
git checkout -- internal/version/version.go herdr-plugin.toml
```

- [ ] **Step 4: 빌드와 vet을 돌린다**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: 전부 통과. (`go build ./...`가 새 main 패키지도 빌드한다.)

- [ ] **Step 5: 커밋**

```bash
git add internal/tools/relver
git commit -F - <<'EOF'
Add the relver command the fork release workflow calls

A thin CLI over internal/relver: it reads version.go, computes the
nightly version, and with -write stamps it back into version.go and
optionally herdr-plugin.toml. The version goes to stdout alone so the
workflow can capture it directly; diagnostics go to stderr.

Whether -manifest is safe to pass depends on herdr accepting a semver
prerelease in the manifest, which the next task settles.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

### Task 5: 매니페스트가 prerelease 버전을 받아주는지 실측

설계 문서 3절의 미확정 지점을 여기서 닫는다. `herdr-plugin.toml`의 `version`에 `0.1.20-nightly.20260817-0843`을 넣었을 때 herdr가 받아주는지 확인하고, 결과에 따라 Task 6의 워크플로가 `-manifest` 플래그를 쓸지 결정한다.

**이 Task는 코드를 남기지 않는다.** 산출물은 설계 문서에 기록되는 결론이다.

**Files:**
- Modify: `docs/superpowers/specs/2026-08-17-fork-windows-release-design.md:110-126` (3절을 확정된 결론으로 교체)

**Interfaces:**
- Consumes: Task 4의 `go run ./internal/tools/relver -write -manifest`
- Produces: Task 6이 워크플로에 `-manifest`를 넣을지 말지의 결정

- [ ] **Step 1: 매니페스트에 prerelease 버전을 심는다**

```bash
go run ./internal/tools/relver -write -manifest
grep '^version' herdr-plugin.toml
```
Expected: `version = "0.1.20-nightly.<stamp>"`

- [ ] **Step 2: 빌드하고 herdr에 링크한다**

```bash
go build -o bin/herdr-plus.exe .
herdr plugin link .
```
Expected: 링크 성공, 또는 버전 파싱 에러. **에러 메시지 전문을 기록해 둔다.**

- [ ] **Step 3: herdr가 무슨 버전으로 인식하는지 본다**

```bash
herdr plugin list
```
Expected: 성공했다면 `cloudmanic.herdr-plus`가 stamped 버전으로 표시된다.

- [ ] **Step 4: 정리한다**

```bash
herdr plugin unlink cloudmanic.herdr-plus
git checkout -- internal/version/version.go herdr-plugin.toml
rm -rf bin
```

- [ ] **Step 5: 설계 문서의 3절을 결론으로 바꾼다**

`2026-08-17-fork-windows-release-design.md`의 "## 3. 미확정 지점: 매니페스트의 prerelease 버전" 절 전체를 실측 결과로 교체한다. 제목에서 "미확정 지점:"을 뗀다. 두 갈래 서술("수용 → …, 거부 → …")을 지우고 확정된 한 갈래만 남기되, **실측한 herdr 버전을 명시**한다 — 나중에 herdr가 바뀌면 재확인이 필요하다는 신호가 되기 때문이다.

수용된 경우의 본문 예시:

```markdown
## 3. 매니페스트의 prerelease 버전

`herdr-plugin.toml`의 `version`에 `0.1.20-nightly.20260817-0843` 같은 semver
prerelease를 넣어도 herdr 0.8.0-nightly가 받아준다 (2026-08-17 `herdr plugin link`로
실측). 따라서 `release-fork.yml`은 `relver -write -manifest`를 호출해 `version.go`와
매니페스트를 함께 갱신하고, `herdr plugin list`에 정확한 빌드가 표시된다.

herdr를 크게 올린 뒤에는 이 가정을 다시 확인할 것.
```

거부된 경우에는 매니페스트를 base 버전으로 두는 쪽으로 서술하고, Task 6에서 `-manifest`를 빼면 된다.

- [ ] **Step 6: 작업 트리가 깨끗한지 확인한다**

Run: `git status --short`
Expected: 설계 문서 한 개만 수정된 상태. `version.go`, `herdr-plugin.toml`, `bin/`이 남아 있으면 안 된다.

- [ ] **Step 7: 커밋**

```bash
git add docs/superpowers/specs/2026-08-17-fork-windows-release-design.md
git commit -F - <<'EOF'
Settle whether the manifest accepts a prerelease version

Measured against herdr 0.8.0-nightly with `herdr plugin link`, so the
release workflow no longer has to hedge on it. The spec now records the
herdr version the result was measured on — a future major bump is reason
to re-check rather than assume.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

---

### Task 6: fork 전용 릴리스 파이프라인

`windows-support` push마다 Windows 바이너리 릴리스를 낸다. **upstream 파일을 고치지 않고 새 파일 두 개만 추가한다.**

**Files:**
- Create: `.goreleaser.fork.yml`
- Create: `.github/workflows/release-fork.yml`

**Interfaces:**
- Consumes: Task 4의 `go run ./internal/tools/relver -write [-manifest]`, Task 5의 `-manifest` 사용 여부 결정
- Produces: `v<base>-nightly.<stamp>` 태그와 그에 붙은 GitHub prerelease (`herdr-plus_<version>_windows_amd64.zip`)

- [ ] **Step 1: goreleaser 설정을 만든다**

`.goreleaser.fork.yml`:

```yaml
#
# Date: 2026-08-17
# Author: Joonhwan Lee (joonhwan.lee@mirero.co.kr)
# Copyright: 2026 Cloudmanic Labs, LLC. All rights reserved.
#
# GoReleaser config for this fork's nightly Windows releases, driven by
# .github/workflows/release-fork.yml on every push to windows-support.
#
# It is a separate file from .goreleaser.yml rather than an edit to it, and that
# is the whole point: upstream owns .goreleaser.yml and changes it, so editing it
# here would make every upstream merge a conflict. A new file never conflicts.
#
# Two differences from upstream's config matter:
#
#   1. No `brews:` block. Upstream pushes a Homebrew formula into
#      cloudmanic/herdr-plus; this fork's GITHUB_TOKEN cannot write there, so
#      keeping the block would fail every release. The fork ships no formula.
#   2. Windows amd64 only — the two PCs this fork exists for.

version: 2

project_name: herdr-plus

# No `before: hooks: [go mod tidy]`. Upstream runs it here; the Test workflow
# already verifies tidiness on every push, and a tidy run that dirties the tree
# mid-release would break the tag goreleaser just checked out.

builds:
  - id: herdr-plus
    binary: herdr-plus
    main: .
    env:
      - CGO_ENABLED=0
    goos: [windows]
    goarch: [amd64]
    ldflags:
      - -s -w

archives:
  - id: default
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    # zip is the platform-native format, and it matches the archive name the
    # upstream config produces for windows — so a future install script can treat
    # both origins identically.
    formats: [zip]
    files:
      - LICENSE*
      - README*

checksum:
  name_template: "checksums.txt"

# Nightly builds cut on every push; a generated changelog per build would be
# noise, and the commit list is a click away on the release page.
changelog:
  disable: true

release:
  # Every version this pipeline produces carries a -nightly.… prerelease
  # identifier, so `auto` always marks these as prereleases. That keeps them out
  # of the "latest release" slot, which matters because upstream's real releases
  # share this repo's fork network.
  prerelease: auto
```

- [ ] **Step 2: 릴리스 워크플로를 만든다**

`.github/workflows/release-fork.yml`. Task 5에서 매니페스트 stamping이 거부된 경우 `-manifest`를 지운다:

```yaml
#
# Date: 2026-08-17
# Author: Joonhwan Lee (joonhwan.lee@mirero.co.kr)
# Copyright: 2026 Cloudmanic Labs, LLC. All rights reserved.
#
# Nightly release pipeline for this fork. Triggers on every push to
# windows-support:
#
#   1. internal/tools/relver reads the upstream base version out of version.go,
#      composes <base>-nightly.<YYYYMMDD>-<HHMM> in Seoul time, and stamps it
#      into the files that carry it.
#   2. The stamp is committed with [skip ci] and tagged v<version>.
#   3. GoReleaser builds windows/amd64 and attaches the zip to a GitHub
#      prerelease.
#
# This is a NEW file, not an edit to release.yml. Upstream owns that one and it
# only fires on main; leaving it untouched keeps upstream merges conflict-free.
# Never push this fork's main — upstream's release.yml would fire there and fail
# trying to push a Homebrew formula into cloudmanic/herdr-plus.

name: Release (fork)

on:
  push:
    branches: [windows-support]
    # Docs and website changes should not cut a binary. paths-ignore skips the
    # run only when every changed file matches, so a mixed commit still releases.
    paths-ignore:
      - "docs/**"
      - "www/**"
      - "README.md"
  workflow_dispatch:

# Pushing the stamp commit and the tag needs write access.
permissions:
  contents: write

# Serialise releases so two pushes cannot race for the same stamp.
concurrency:
  group: release-fork
  cancel-in-progress: false

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          # goreleaser needs prior tags, and the stamp commit needs a real branch
          # to push back to.
          fetch-depth: 0
          token: ${{ secrets.GITHUB_TOKEN }}

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: stable

      - name: Configure git identity
        run: |
          git config user.name  "github-actions[bot]"
          git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

      # relver owns every rule about the version string; this step only moves its
      # output into the workflow. Keeping the logic in Go is what makes it
      # unit-testable and identical on any runner.
      - name: Stamp version
        id: version
        run: |
          set -euo pipefail
          VERSION="$(go run ./internal/tools/relver -write -manifest)"
          echo "version=$VERSION" >> "$GITHUB_OUTPUT"
          echo "Stamped $VERSION"

      # The stamp is minute-resolution, so two pushes inside one minute would
      # collide. That is a duplicate build, not a failure — skip it rather than
      # turning the run red.
      - name: Check tag is free
        id: tag
        run: |
          set -euo pipefail
          TAG="v${{ steps.version.outputs.version }}"
          if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
            echo "$TAG already exists — skipping this release."
            echo "skip=true" >> "$GITHUB_OUTPUT"
          else
            echo "skip=false" >> "$GITHUB_OUTPUT"
          fi

      - name: Commit and tag
        if: steps.tag.outputs.skip == 'false'
        run: |
          set -euo pipefail
          TAG="v${{ steps.version.outputs.version }}"
          git add internal/version/version.go herdr-plugin.toml
          git commit -m "Release $TAG [skip ci]"
          git push origin HEAD:windows-support
          git tag -a "$TAG" -m "Release $TAG"
          git push origin "$TAG"

      - name: Run GoReleaser
        if: steps.tag.outputs.skip == 'false'
        uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean --config .goreleaser.fork.yml
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 3: 로컬에서 goreleaser 설정을 검증한다**

goreleaser가 설치돼 있으면:
```bash
goreleaser check --config .goreleaser.fork.yml
```
Expected: `1 configuration file(s) validated`

설치돼 있지 않으면 이 스텝은 건너뛰고 Step 5의 실제 실행으로 검증한다.

- [ ] **Step 4: 커밋**

```bash
git add .goreleaser.fork.yml .github/workflows/release-fork.yml
git commit -F - <<'EOF'
Add the fork's nightly Windows release pipeline

Two new files rather than edits to .goreleaser.yml and release.yml:
upstream owns those and changes them, so editing them would turn every
upstream merge into a conflict, while a new file never conflicts.

The config drops upstream's brews block — this fork's token cannot push a
formula into cloudmanic/herdr-plus, so keeping it would fail every
release — and builds windows/amd64 alone. The workflow delegates all
version rules to internal/tools/relver and skips rather than fails when a
minute-resolution stamp collides.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
```

- [ ] **Step 5: push하고 릴리스가 실제로 나오는지 확인한다**

```bash
git push origin windows-support
```

Expected, 순서대로:
1. Test 워크플로가 3개 OS에서 통과
2. Release (fork) 워크플로가 통과
3. `git fetch --tags && git tag -l 'v*nightly*'`에 `v0.1.20-nightly.<stamp>`가 보인다
4. GitHub Releases 페이지에 prerelease가 있고, `herdr-plus_0.1.20-nightly.<stamp>_windows_amd64.zip`과 `checksums.txt`가 붙어 있다

실패하면 흔한 원인 두 가지: **fork에서 Actions가 비활성화**돼 있거나 (Settings → Actions에서 켠다), **워크플로 쓰기 권한 부족** (Settings → Actions → General → Workflow permissions를 "Read and write"로).

- [ ] **Step 6: stamp 커밋을 로컬로 당겨온다**

CI가 `version.go`(와 매니페스트)를 커밋해 push했으므로 로컬이 뒤처져 있다.

```bash
git pull --ff-only origin windows-support
grep 'const Version' internal/version/version.go
```
Expected: stamped 버전. 이걸 안 당겨오면 다음 push가 non-fast-forward로 거부된다.

---

### Task 7: 설치 검증과 README 안내

**Files:**
- Modify: `README.md` (Install 절 뒤에 fork 전용 절 추가)

**Interfaces:**
- Consumes: Task 6이 만든 릴리스와 브랜치
- Produces: 없음 (최종 산출물)

- [ ] **Step 1: 실제로 설치해 본다**

기존에 링크/설치된 게 있으면 먼저 정리한다:
```bash
herdr plugin list
herdr plugin uninstall cloudmanic.herdr-plus   # 설치돼 있을 때만
```

설치:
```bash
herdr plugin install joonhwan/herdr-plus --ref windows-support -y
```
Expected: herdr가 fork를 클론하고 매니페스트의 windows `[[build]]`(`go build -o bin/herdr-plus.exe .`)를 돌려 등록에 성공한다.

- [ ] **Step 2: 등록된 내용을 확인한다**

```bash
herdr plugin list
herdr plugin action list --plugin cloudmanic.herdr-plus
```
Expected: 버전이 stamped 값으로 보이고(Task 5에서 매니페스트 stamping을 채택한 경우), `projects-windows` / `quick-actions-windows` / `ping-windows` 액션이 등록돼 있다.

- [ ] **Step 3: 플러그인 루프를 실제로 태운다**

```bash
herdr plugin action invoke cloudmanic.herdr-plus.ping-windows
herdr plugin log list --plugin cloudmanic.herdr-plus
```
Expected: ping이 herdr에 named pipe로 붙어 포커스된 pane/workspace를 로그에 남긴다. 이게 통과하면 Windows IPC 경로가 실제로 살아 있다는 뜻이다.

- [ ] **Step 4: Projects UI를 띄워 본다**

```bash
herdr plugin action invoke cloudmanic.herdr-plus.projects-windows
```
Expected: zoomed pane에 fuzzy picker TUI가 뜨고 키 입력을 받는다 (pane 항목이 `-NonInteractive`를 뺀 이유가 이것이다). ESC로 닫는다.

- [ ] **Step 5: README에 fork 절을 추가한다**

`README.md`의 "### Just the binary" 절 **뒤**에 넣는다. 이 절은 fork 전용이라 upstream 머지 시 충돌 후보이므로, 그 사실을 주석 없이도 알 수 있게 절 제목에 fork임을 드러낸다:

```markdown
### This fork (joonhwan/herdr-plus)

This fork tracks upstream and adds Windows-focused CI and a nightly release
pipeline. Install it from the `windows-support` branch:

```bash
herdr plugin install joonhwan/herdr-plus --ref windows-support -y
```

herdr clones the branch and builds from source, so **Go must be on your `PATH`**.
The `windows-support` branch also publishes nightly Windows binaries as GitHub
prereleases, tagged `v<upstream-version>-nightly.<YYYYMMDD>-<HHMM>` — the
upstream version stays in front so it is always clear what the build is based on.

Check what you are running with:

```bash
herdr-plus version
```

To follow upstream, merge it in rather than pushing this fork's `main`:

```bash
git fetch upstream
git checkout main && git merge --ff-only upstream/main
git checkout windows-support && git merge main
```
```

- [ ] **Step 6: 커밋하고 push한다**

```bash
git add README.md
git commit -F - <<'EOF'
Document installing this fork from the windows-support branch

Covers the install command, the nightly tag format and what the upstream
prefix in it means, and the upstream-merge routine — including why this
fork's main is never pushed.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
git push origin windows-support
```

Expected: README만 바뀌었으므로 `paths-ignore`가 걸려 Release (fork) 워크플로는 **돌지 않는다.** Test 워크플로는 돈다. 이 동작 자체가 `paths-ignore` 설정의 검증이다.

- [ ] **Step 7: 다른 PC에서 설치해 본다**

집/회사 중 아직 안 해본 쪽에서:
```bash
herdr plugin install joonhwan/herdr-plus --ref windows-support -y
herdr plugin action invoke cloudmanic.herdr-plus.ping-windows
```
Expected: 성공. 이걸로 이 작업의 목적이 달성된다.

---

## 완료 판정

- [ ] `go test ./...`가 Windows에서 전부 통과
- [ ] Test 워크플로가 ubuntu / macos / windows 3개 모두 초록불
- [ ] `windows-support` push마다 `v<base>-nightly.<stamp>` prerelease가 생성됨
- [ ] 두 대의 Windows PC에서 `herdr plugin install joonhwan/herdr-plus --ref windows-support`가 성공
- [ ] `ping-windows`와 `projects-windows`가 실제로 동작
- [ ] upstream 파일 중 수정된 것은 `test.yml`, `open_test.go`, `README.md` 셋뿐이고, 앞의 둘은 upstream PR로 떼어낼 수 있는 독립 커밋에 들어 있음

## 범위 밖

설계 문서와 동일하다. 필요해지면 별건으로 다룬다.

- `install.ps1` / `scripts/build.ps1` 폴백 — 두 PC 모두 Go가 있어 불필요
- `install.sh`의 `REPO` 하드코딩 오버라이드
- Scoop / winget 배포 채널
- windows/arm64 빌드
- `%VAR%` 경로 확장 지원
- `Makefile`의 POSIX 의존성 해소 — Go 기반 태스크 러너로 바꾸는 건 별도 작업이다
