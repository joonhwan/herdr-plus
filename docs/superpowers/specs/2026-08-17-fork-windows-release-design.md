# fork 전용 Windows 릴리스 파이프라인 설계

- 작성일: 2026-08-17
- fork: `joonhwan/herdr-plus` / upstream: `cloudmanic/herdr-plus`
- 기준: `a9aca9d` (v0.1.20)
- 선행 문서: [2026-08-17-windows-support-audit.md](2026-08-17-windows-support-audit.md)
- 상태: 승인됨

## 목표

집과 회사 두 대의 Windows PC에서 herdr-plus를 쓸 수 있게 한다. 구체적으로는:

1. fork 저장소의 전용 브랜치에서 CI가 Windows 바이너리 릴리스를 낸다
2. `herdr plugin install`로 그 브랜치를 설치할 수 있다
3. upstream을 계속 따라갈 수 있도록 머지 충돌을 최소화한다

## 전제

- 두 PC 모두 Go 툴체인이 설치돼 있다 → 소스 빌드 경로가 동작한다
- `herdr 0.8.0-nightly` 기준으로 `herdr plugin install <owner>/<repo> --ref <REF>`를 지원한다
- upstream 태그 `v0.1.20`까지 존재한다

## 결정 사항

| 항목 | 결정 | 근거 |
|---|---|---|
| 태그 규칙 | `v<base>-nightly.<YYYYMMDD>-<HHMM>` (예: `v0.1.20-nightly.20260817-0843`) | upstream 버전을 prefix로 삼아 어느 기반인지 드러내고, 뒤쪽 nightly 타임스탬프로 빌드를 구분한다. herdr 자신의 `0.8.0-nightly.2026.08.16-0612` 표기를 따른 것이다 |
| 릴리스 트리거 | `windows-support` 브랜치 push마다 자동 | upstream의 현재 방식과 동일. 두 PC가 항상 최신을 받는다 |
| 빌드 대상 | windows/amd64 단독 | 목적이 Windows PC 두 대다. 필요해지면 늘리기 쉽다 |
| 설치 방식 | `herdr plugin install` + 소스 빌드 | Go가 있으므로 `install.ps1` 폴백이 불필요하다 |

## 1. 브랜치 / 머지 전략

`windows-support`를 장수 브랜치로 둔다. 개발과 릴리스가 모두 여기서 일어난다.

fork의 `main`은 upstream 미러 전용이며 **push하지 않는다.** upstream의
`.github/workflows/release.yml`이 `main` push에 걸려 있고, 그 안의 goreleaser가
`cloudmanic/herdr-plus`로 brew formula를 push하려 한다. fork의 `GITHUB_TOKEN`에는
그 권한이 없으므로 릴리스 잡이 실패하고 알림만 쌓인다.

`upstream` remote를 등록해 두고, 동기화는 이 경로로 한다:

```
git fetch upstream
git checkout main && git merge --ff-only upstream/main   # push 하지 않음
git checkout windows-support && git merge main
```

### 머지 위생 규칙

**fork 전용 변경은 기존 파일을 수정하지 말고 새 파일로 추가한다.**

같은 파일을 고치면 upstream이 그 파일을 건드릴 때마다 충돌한다. 새 파일은 충돌하지
않는다. 중복 몇 줄을 감수하고 무충돌 머지를 얻는다.

따라서 `.goreleaser.yml`과 `release.yml`은 그대로 두고, `.goreleaser.fork.yml`과
`release-fork.yml`을 새로 만든다.

### 커밋 분리

조사 문서의 1·2번(테스트 픽스처, CI 매트릭스)은 fork 전용이 아니라 upstream에도
유효한 수정이다. 나중에 upstream PR로 떼어낼 수 있도록 **fork 전용 커밋과 섞지 않고
독립 커밋**으로 만든다.

## 2. 릴리스 파이프라인

### `.github/workflows/release-fork.yml` (신규)

트리거: `windows-support` 브랜치 push, 그리고 `workflow_dispatch`.

동작 순서:

1. `internal/version/version.go`에서 base 버전을 읽는다. `0.1.20-nightly.…`처럼 이미
   접미사가 붙어 있으면 `0.1.20` 부분만 취한다.
2. 현재 시각으로 `<YYYYMMDD>-<HHMM>` 스탬프를 만든다. 타임존은 **Asia/Seoul**로
   고정한다 — GitHub 러너는 UTC로 도는데, 이 스탬프는 사람이 "지금 어느 빌드를 쓰고
   있나"를 읽는 용도라 쓰는 사람의 시간대가 맞다.
3. `version.go`에 `<base>-nightly.<stamp>`를 써넣고 `[skip ci]`로 커밋·push한다.
4. `v<base>-nightly.<stamp>` 태그를 만들어 push한다.
5. `goreleaser release --clean --config .goreleaser.fork.yml`을 돌린다.

base가 upstream 머지로 `0.1.21`이 되면 다음 릴리스부터 `v0.1.21-nightly.…`가 되어
저절로 따라간다. 태그를 세거나 상태 파일을 두지 않고 시각에서 바로 유도하므로 CI가
관리할 상태가 없다.

### 날짜 형식을 붙여 쓰는 이유

herdr는 `2026.08.16-0612`처럼 점으로 끊어 쓰지만, 우리는 `20260817-0843`으로 붙여
쓴다. semver 명세는 prerelease의 **순수 숫자 식별자에 leading zero를 금지**하는데,
점으로 끊으면 `08`이 여기 걸린다. 붙여 쓰면 하이픈 때문에 식별자 전체가 영숫자로
취급되어 문제가 사라진다. 태그를 파싱하는 것은 goreleaser이고, 거부당하면 릴리스가
아예 나가지 않으므로 명세를 지키는 쪽을 택했다.

부수 효과로 사전순 정렬이 시간순과 일치한다.

`concurrency: group: release-fork`로 직렬화한다. 같은 분에 두 번 push하면 스탬프가
겹치므로, 태그가 이미 있으면 잡을 실패시키지 말고 건너뛴다.

### `.goreleaser.fork.yml` (신규)

upstream 설정과 다른 점:

- `goos: [windows]`, `goarch: [amd64]`
- `brews:` 블록 **제거** — fork 토큰으로 `cloudmanic/herdr-plus`에 push할 수 없어,
  남겨두면 릴리스가 반드시 실패한다
- 아카이브 포맷은 zip
- `-nightly.<stamp>`가 semver prerelease이므로 goreleaser가 GitHub Release를 prerelease로
  표시한다

## 3. 미확정 지점: 매니페스트의 prerelease 버전

`herdr-plugin.toml`의 `version` 필드가 `0.1.20-nightly.20260817-0843` 같은 prerelease
표기를 받아주는지 확인되지 않았다. semver로 파싱한다면 유효하지만 검증 전에는 가정하지
않는다.

구현 3단계에서 `herdr plugin link`로 실측한 뒤 분기한다:

- **수용** → 매니페스트에도 전체 버전을 써서 `herdr plugin list`에 정확히 표시되게 한다.
  `release-fork.yml`이 `version.go`와 매니페스트를 함께 갱신한다.
- **거부** → 매니페스트는 base 버전(`0.1.20`)을 유지한다. 구분은
  `herdr-plus version` 출력과 git 태그로 한다.

어느 쪽이든 `internal/version/version.go`에는 전체 버전
(`0.1.20-nightly.20260817-0843`)을 써넣는다.
두 PC를 오갈 때 지금 어느 빌드가 깔려 있는지 아는 것이 이 작업의 실질적 목적 중
하나이기 때문이다.

## 4. 설치 경로

```
herdr plugin install joonhwan/herdr-plus --ref windows-support -y
```

herdr가 fork를 클론하고 매니페스트의 windows `[[build]]` 스텝
(`go build -o bin/herdr-plus.exe .`)을 실행한다. Go가 있으므로 이대로 동작한다.

CI 릴리스는 두 가지 역할을 한다: 파이프라인이 실제로 도는지 검증하고, Go 없는 환경이
생겼을 때 쓸 아티팩트를 남긴다.

## 5. Windows CI 정상화 (조사 문서 1·2번)

Windows CI 없이 Windows 릴리스를 내는 것은 검증 없이 배포하는 것과 같다. 릴리스
파이프라인보다 먼저 넣는다.

### 테스트 픽스처

`open_test.go`의 `writeFakeHerdr`가 가짜 herdr을 `#!/bin/sh` 스크립트로 쓴다.
Windows에는 셔뱅이 없어 실행되지 않고, `TestHerdrManagedConfigDir`과
`TestEnsureManagedConfigDir`이 현재 실패한다.

Windows에서는 `.cmd` 배치 파일을 쓰도록 분기한다. 헬퍼 시그니처는 sh 스크립트 본문을
받는 형태이므로, 호출부가 "출력할 문자열"과 "종료 코드"를 넘기고 헬퍼가 OS에 맞는
스크립트를 생성하는 형태로 바꾸는 것이 깔끔하다.

### CI 매트릭스

`.github/workflows/test.yml`의 매트릭스에 `windows-latest`를 추가한다. 현재
`[ubuntu-latest, macos-latest]`뿐이라 `herdrdial_windows.go`, `powershellQuote`,
PowerShell 라운드트립 통합 테스트가 CI에서 컴파일조차 되지 않는다.

Windows에서 `-race`는 cgo(gcc)를 요구한다. `windows-latest` 러너에는 mingw가 있어
대체로 동작한다. 실패하면 Windows 잡만 `-race` 없이 돌린다.

## 6. 작업 순서

| 단계 | 내용 | 완료 판정 |
|---|---|---|
| 0 | `upstream` remote 추가, `windows-support` 브랜치 생성, 문서 커밋 | 브랜치에 문서 2개 |
| 1 | `open_test.go` 픽스처 Windows 분기 | 로컬 `go test ./...` 전체 통과 |
| 2 | `test.yml`에 `windows-latest` 추가 | CI 3개 OS 모두 통과 |
| 3 | 매니페스트 prerelease 수용 여부 실측 | 3절의 분기 확정 |
| 4 | `.goreleaser.fork.yml` + `release-fork.yml` | push로 `v0.1.20-nightly.<stamp>` 릴리스 생성 |
| 5 | 설치 실측 | `herdr plugin install ... --ref windows-support` 성공 |
| 6 | README에 fork 설치 안내 | fork 전용 섹션 추가 |

## 범위 밖

다음은 이번 작업에 포함하지 않는다. 필요해지면 별건으로 다룬다.

- `install.ps1` / `scripts/build.ps1` 폴백 — 두 PC 모두 Go가 있어 불필요
- `install.sh`의 `REPO` 하드코딩 오버라이드 — 위 항목과 함께 묶인다
- Scoop / winget 배포 채널
- windows/arm64 빌드
- `%VAR%` 경로 확장 지원
- `Makefile`의 POSIX 의존성 해소
