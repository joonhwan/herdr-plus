# herdr-plus Windows 지원 현황 조사

- 작성일: 2026-08-17
- 기준 커밋: `a9aca9d` (v0.1.20, main)
- 조사 환경: Windows 11 Pro 26200, Go 1.26.6 windows/amd64
- 상태: 조사 완료 / 범위 결정 대기

## 요약

herdr-plus의 Windows 대응은 **런타임 코드는 거의 끝나 있고, 빌드·CI·배포 쪽이 비어 있는** 상태다.
실제로 이 저장소를 Windows에서 빌드해보면 `go build ./...`와 `go vet ./...`는 통과하지만
`go test ./...`는 2개가 실패한다. 그리고 그 실패를 여태 아무도 몰랐던 이유는 CI 매트릭스에
Windows가 없어서다 — Windows 전용 코드가 CI에서 컴파일조차 되지 않는다.

## 이미 되어 있는 것

| 영역 | 구현 위치 | 내용 |
|---|---|---|
| IPC | `herdrdial_windows.go` | 유닉스 소켓 대신 named pipe(`\\.\pipe\<path>`). `ERROR_PIPE_BUSY`(231) 재시도 포함 |
| 셸 실행 | `shell.go` | `platform` 구조체에 PowerShell 프리픽스, 인용 규칙, pane 접미사, opener를 모아둠 |
| 매니페스트 | `herdr-plugin.toml` | 모든 action/pane/event에 `-windows` 쌍둥이 항목. 상대경로 exe 문제를 `powershell -Command "& .\bin\herdr-plus.exe"`로 우회 |
| 매니페스트 검증 | `manifest_test.go` | 유닉스 항목마다 windows 짝이 있는지, 그 짝이 `.exe`를 호출하는지 검사 |
| 경로 처리 | `project.go` `expandedWorkingDir` | `filepath.Clean`으로 구분자 정규화, `os.UserHomeDir`(=`USERPROFILE`) 사용 |
| 인용 라운드트립 | `action_run_integration_test.go` | 실제 PowerShell을 띄워 악성 값(`a b 'c" $x`)이 그대로 돌아오는지 검증 |
| 바이너리 배포 | `.goreleaser.yml` | windows/amd64 바이너리를 zip으로 릴리스에 첨부 |

`shell.go`의 `platform` 구조체는 OS 분기를 한 곳에 모아둔 좋은 설계다. OS를 추가할 때
네 군데를 고치는 게 아니라 `case` 하나만 늘리면 된다.

## 남은 변경사항

### 1. Windows에서 테스트 2개 실패 (실측)

```
--- FAIL: TestHerdrManagedConfigDir   open_test.go:67
--- FAIL: TestEnsureManagedConfigDir  open_test.go:87
```

`open_test.go`의 `writeFakeHerdr` 헬퍼가 가짜 herdr 바이너리를 `#!/bin/sh` 스크립트로 쓴다.
Windows에는 셔뱅이 없어 `exec.Command`가 실행하지 못하고, `herdrManagedConfigDir()`이
빈 문자열을 돌려주면서 두 테스트가 함께 무너진다.

프로덕션 코드는 크로스플랫폼으로 잘 짜여 있는데 **테스트 픽스처만 POSIX를 가정**한
전형적인 사각지대다.

**할 일:** `writeFakeHerdr`를 Windows에서 `.cmd` 배치 파일(또는 작은 Go 헬퍼 바이너리)로
쓰도록 분기.

### 2. CI 매트릭스에 Windows 없음 — 가장 큰 구멍

`.github/workflows/test.yml`의 매트릭스가 `[ubuntu-latest, macos-latest]`뿐이다.
따라서 다음이 CI에서 **컴파일조차 되지 않는다**:

- `herdrdial_windows.go` (`//go:build windows`)
- `powershellQuote` / `paneEntrypoint`의 windows 분기
- PowerShell 인용 라운드트립 통합 테스트

1번 실패가 지금까지 안 잡힌 직접적인 원인이다.

**할 일:** 매트릭스에 `windows-latest` 추가.

**주의:** Windows에서 `-race`는 cgo(gcc)를 요구한다. `windows-latest` 러너에는 mingw가
들어 있어 대체로 동작하지만, 불안정하면 Windows 잡만 `-race` 없이 돌리는 선택지가 있다.

### 3. Windows 설치에만 Go 툴체인이 필수

유닉스는 `scripts/build.sh`가 "Go 있으면 소스 빌드, 없으면 릴리스 바이너리 다운로드"라는
폴백을 가진다. Windows용 `[[build]]` 스텝은 `go build`를 바로 호출하므로 **Go가 PATH에
없으면 `herdr plugin install`이 그냥 실패**한다. README에도 이 제약이 명시돼 있다.

**할 일:**
- `scripts/build.ps1` — `build.sh`와 같은 구조(Go 우선, 없으면 다운로드)
- `install.ps1` — `install.sh`의 PowerShell 판. 릴리스 zip을 받아 압축 해제 후 배치
- `herdr-plugin.toml`의 windows `[[build]]`를 `powershell -File scripts/build.ps1`로 교체

### 4. Windows 배포 채널이 없음

macOS/Linux에는 Homebrew 탭과 `install.sh`가 있지만 Windows는 릴리스 zip 수동 다운로드뿐이다.

**할 일:**
- goreleaser에 **Scoop 버킷** 또는 **winget 매니페스트** 추가 (goreleaser가 둘 다 지원)
- `.goreleaser.yml`이 현재 windows/arm64를 `ignore` 중 — 되살릴지 결정 필요

### 5. 개발자 경험 / 문서

- `Makefile`이 `mkdir -p`, `rm -rf`, `trap` 등 POSIX 전용이다. Windows 개발자는 Git Bash
  없이는 `make build`도 못 쓴다. 대안: `make.ps1`, Taskfile, 또는 "Git Bash를 쓰라"고
  문서에 명시
- `expandedWorkingDir`가 `$VAR` / `${VAR}`만 확장하고 Windows 표기 `%VAR%`는 지원하지
  않는다. 코드 주석에는 "문서화하라"고 적혀 있지만 실제 문서에는 없다. 지원할지 문서로만
  막을지 결정 필요
- README와 `www/` 문서의 Windows 안내가 "preview, Go 필요" 상태 — 3번이 끝나면 갱신

## 추천 우선순위

**1단계 — 당장, 리스크 제거**
2번(CI 매트릭스) + 1번(테스트 픽스처). 둘은 묶여야 의미가 있다. CI가 있어야 앞으로
안 깨지고, 픽스처를 고쳐야 CI가 초록불이 된다. 범위가 작아 스펙 문서 없이 바로 구현 가능.

**2단계 — 사용자 체감**
3번(`build.ps1` + `install.ps1`). "Windows는 Go 필수"라는 제약이 사라진다.

**3단계 — 선택**
4번(Scoop/winget), 5번(Makefile 대안, `%VAR%` 정책, 문서 갱신).

## 미결 사항

- 어느 단계까지 진행할지
- Windows CI에서 `-race`를 켤지
- windows/arm64 바이너리를 배포할지
- `%VAR%` 확장을 지원할지, 문서로만 안내할지
- Windows 배포 채널로 Scoop과 winget 중 무엇을 (또는 둘 다) 쓸지
