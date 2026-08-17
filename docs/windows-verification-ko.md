# Windows 동작 검증 가이드

이 포크(`windows-support` 브랜치)를 Windows에서 직접 빌드해 herdr에 붙이고,
기능이 실제로 도는지 확인하는 절차입니다.

여기 적힌 절차는 전부 아래 환경에서 한 번씩 실제로 돌려 본 것입니다.
돌려 보지 않은 항목은 [미검증 항목](#미검증-항목)에 따로 모아 두었습니다.

| 항목 | 값 |
| --- | --- |
| 검증일 | 2026-08-17 |
| OS | Windows 11 Pro (10.0.26200) |
| herdr | 0.8.0-nightly.2026.08.16-0612 |
| herdr-plus | 0.1.20-nightly.20260817-0946 |
| Go | go1.26.6 windows/amd64 |

---

## 1. 사전 준비

- **Go 툴체인이 PATH에 있어야 합니다.** Windows에는 POSIX `sh`가 없어서
  `scripts/build.sh` 경로를 못 씁니다. 매니페스트의 Windows용 `[[build]]` 단계가
  `go build`를 직접 부르기 때문에, GitHub에서 설치하든 로컬에서 링크하든 Go가 필요합니다.
- **herdr는 Windows 베타에서 돌아갑니다.** 매니페스트의 `platforms`에 `windows`가
  들어 있는 것도 그 베타를 전제로 한 것입니다.
- `make`는 필요 없습니다. 오히려 이 리포의 `Makefile`은 `mkdir -p`를 써서
  Windows에서 그대로 돌지 않으니, `make plugin-link` 대신 아래 절차를 쓰세요.

---

## 2. 빌드하고 개발용으로 붙이기

리포 루트에서 실행합니다.

```powershell
go test ./...
go build -o bin/herdr-plus.exe .
```

바이너리 이름이 `herdr-plus.exe`인 건 필수입니다. Windows는 확장자 없는 PE 파일을
경로만으로 실행하지 못합니다(`CreateProcessW`가 `.exe`를 붙여서 찾습니다).

이미 GitHub 설치본이 등록돼 있다면 먼저 떼어 내야 합니다. 같은 플러그인 ID를
두 번 등록할 수 없습니다.

```powershell
herdr plugin unlink cloudmanic.herdr-plus
herdr plugin link .
```

붙었는지 확인:

```powershell
herdr plugin list
# - cloudmanic.herdr-plus (Herdr Plus) enabled [local:\\?\D:\...\herdr-plus]
```

소스는 `local:`로 바뀌고, 경로 앞에 `\\?\`(Windows 확장 길이 경로 접두사)가 붙습니다.
이건 정상이며 동작에 영향이 없습니다 — 액션에 넘어오는 `{{.WorkDir}}`는
접두사 없는 평범한 경로로 들어옵니다(확인함).

코드를 고친 뒤에는 **`go build`를 다시 돌려야** 반영됩니다. herdr는 링크된
디렉터리의 바이너리를 그대로 실행할 뿐 다시 빌드해 주지 않습니다.

---

## 3. 설정 디렉터리 — Windows에서 어디를 보는가

herdr가 관리하는 플러그인별 설정 디렉터리를 씁니다. 항상 herdr에게 물어보세요.

```powershell
herdr plugin config-dir cloudmanic.herdr-plus
# → C:\Users\<이름>\AppData\Roaming\herdr\plugins\config\cloudmanic.herdr-plus
```

그 안의 구조:

```
<config>\
  config.toml        선택 사항 (전역 설정)
  projects\          프로젝트 템플릿
  quick-actions\     퀵 액션 (처음 실행할 때 예제가 자동으로 깔림)
  worktrees\         worktree 자동 레이아웃
```

`~/.config/herdr-plus`를 찾지 마세요. 그건 herdr **밖에서** 바이너리를 직접
실행할 때만 쓰는 폴백 경로이고, herdr 안에서는 만들어지지도 않습니다.
herdr가 `HERDR_PLUGIN_CONFIG_DIR` 환경 변수로 위 경로를 넘겨주고
`configBaseDir()`이 그걸 우선합니다.

> 이 동작 때문에 걸리는 함정이 하나 있습니다. **herdr pane 안에서 `go test ./...`를
> 돌리면** 그 변수가 이미 설정돼 있어서, 테스트가 임시 디렉터리 대신 여러분의
> 진짜 설정을 읽을 수 있습니다. 이번에 `TestLoadWorktreeLayouts`가 실제로 그렇게
> 깨졌고, 해당 테스트가 변수를 비우도록 고쳤습니다. 새 테스트를 쓸 때는
> `t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "")`를 잊지 마세요.

---

## 4. 기능별 검증 절차

### 4.1 ping — 플러그인 연결 확인

가장 먼저 이걸 돌려 보세요. 매니페스트의 Windows용 entrypoint가 실제로
뜨는지 확인하는 가장 싼 방법입니다.

```powershell
herdr plugin action invoke ping-windows
herdr plugin log list --plugin cloudmanic.herdr-plus --limit 1
```

기대 출력:

```
herdr-plus ping ok — plugin "cloudmanic.herdr-plus"
  pane=wJ:p1 tab=wJ:t1 workspace=wJ ("herdr-plus")
  cwd=D:\workspace\prj\work\herdr-plus
```

액션 ID에 `-windows`가 붙는 이유: 매니페스트는 entrypoint를 OS별로 두 벌
선언합니다. Windows 쪽은 `powershell -NoProfile -Command "& .\bin\herdr-plus.exe ..."`
형태로 감쌌는데, 이건 취향이 아니라 필요해서입니다. herdr가 자식 프로세스의
작업 디렉터리를 플러그인 루트로 지정해도, Windows는 **상대 경로 실행 파일을
herdr 자신의 작업 디렉터리 기준으로** 찾기 때문에 `.\bin\herdr-plus.exe`를
직접 띄우면 os error 3이 납니다. PATH에 있는 powershell을 한 단계 거치면
powershell의 작업 디렉터리가 플러그인 루트라서 `&` 연산자가 제대로 찾아 줍니다.

### 4.2 Quick Actions

```powershell
herdr plugin action invoke quick-actions-windows
```

오버레이 pane이 뜨고 액션 목록이 그려지면 성공입니다. 처음 실행하면
`quick-actions\` 디렉터리가 만들어지고 예제 파일이 자동으로 깔립니다.
리포가 자체 액션을 들고 있으면(`<repo>\.herdr-plus\quick-actions\`)
**Project** 헤딩 아래에, 전역 액션은 **Global** 아래에 나뉘어 나옵니다.

세 가지 타입을 각각 확인했습니다.

| 타입 | 확인한 것 |
| --- | --- |
| `command` | 템플릿 치환(`{{.WorkDir}}`, `{{.WorkspaceLabel}}`, `{{.PaneId}}`)과 PowerShell 실행 |
| `select` | 2단계 옵션 목록, 고른 항목의 `label`이 아닌 `value`가 치환됨 |
| `form` | 텍스트 입력 필드, 입력값이 `{{.Value}}`로 전달됨 |

**Windows에서 액션은 PowerShell로 실행됩니다** (macOS/Linux는 `sh -c`).
이건 그냥 "동작이 조금 다른" 수준이 아닙니다. PowerShell은 `<`를 예약어로
막아 두었고 `||`도 구문 구분자로 받지 않아서, POSIX용으로 쓴 명령은
**파싱 단계에서 죽고 아무것도 실행되지 않습니다.**

그래서 액션에 `[windows]` 블록으로 Windows 전용 명령을 따로 줄 수 있습니다.
기본 `command`는 그대로 필수라, 기존 액션 파일은 손대지 않아도 계속 돕니다.

```toml
name = "make test"
command = 'make test; read -t 30 _ </dev/tty || true'

[windows]
command = 'go test ./...; Write-Host "`n— done —"; Start-Sleep -Seconds 15'
```

명령이 끝난 뒤 출력을 읽을 시간을 벌려면 마지막에 기다리는 동작을 넣어야 합니다.
액션의 stdin은 `/dev/null`이라, unix 예제가 굳이 `</dev/tty`를 쓰는 것도 그 때문이고,
Windows에서 `Read-Host`를 쓰면 EOF를 바로 받고 끝나 버립니다. `Start-Sleep`을 쓰세요.

OS 기본 프로그램으로 열기만 하면 되는 경우엔 `{{opener}}`를 쓰면 오버라이드가
아예 필요 없습니다. macOS는 `open`, Linux는 `xdg-open`, Windows는 `Start-Process`로
알아서 바뀝니다.

**액션이 실패하면** herdr-plus가 에러를 찍고 `— press Enter to close —`를 띄운 채
pane을 붙잡아 둡니다. Enter를 누르면 닫힙니다.

### 4.3 Projects

설정의 `projects\`에 `*.toml`을 하나씩 넣습니다(파일 이름은 상관없음).

```toml
name = "ZZ Verify"
description = "Windows verification project"
group = "Verification"          # 선택: 목록에서 헤딩으로 묶임
working_dir = "D:/workspace/prj/work/herdr-plus"

[[tabs]]
name = "one"
command = "echo TAB_ONE_MARKER"

[[tabs]]
name = "split"

[[tabs.panes]]
label = "Left"
command = "echo PANE_LEFT_MARKER"

[[tabs.panes]]
label = "Right"
command = "echo PANE_RIGHT_MARKER"
split = "right"

[[tabs]]
name = "plain"                  # command 없음 — 빈 셸
```

여는 방법은 두 가지입니다.

```powershell
# 대화형 — 퍼지 목록에서 고르기
herdr plugin action invoke projects-windows

# 헤드리스 — 이름으로 바로 열기 (herdr 안에서 실행해야 함)
.\bin\herdr-plus.exe open "ZZ Verify"
```

> 헤드리스 서브커맨드는 `open <이름>`입니다. `projects <이름>`이 아닙니다.
> 인자 없는 `projects`는 대화형 picker를 띄우는 액션 진입점입니다.

확인한 것:

- 탭이 선언 순서대로 만들어지고 `name`이 탭 라벨로 붙음
- `[[tabs.panes]]`가 pane으로 분할되고 `label`과 `split` 방향이 그대로 반영됨
- 각 탭/pane의 `command`가 실제로 실행됨
- `working_dir`의 `~`가 `C:\Users\<이름>`으로 확장됨
- picker에서 `group` 값이 헤딩으로 묶여 나옴

레이아웃 검증은 눈으로 보는 것보다 herdr에게 물어보는 쪽이 확실합니다.

```powershell
herdr tab list --workspace <workspace_id>
herdr pane list --workspace <workspace_id>
herdr pane read <pane_id> --source recent-unwrapped --lines 20
```

### 4.4 Worktree 자동 레이아웃

설정의 `worktrees\`에 레이아웃을 넣어 두면, herdr가 worktree를 만들거나
열 때 그 워크스페이스에 탭을 자동으로 깔아 줍니다.

```toml
# zz-specific.toml — 특정 리포에만 적용
repo = "myrepo"

[[tabs]]
name = "wt-one"
command = "echo WT_SPECIFIC_MARKER"
```

```toml
# zz-wildcard.toml — 매칭되는 게 없을 때의 폴백
repo = "*"

[[tabs]]
name = "fallback"
command = "echo WILDCARD_LAYOUT_MARKER"
```

검증 방법 — 임시 리포를 하나 만들어서 돌리면 실제 작업물을 안 건드립니다.

```powershell
herdr worktree create --cwd <임시-리포-경로> --branch zz-verify --no-focus
herdr plugin log list --plugin cloudmanic.herdr-plus --limit 1
```

기대 로그:

```
herdr-plus: applied worktree layout "zz-specific.toml" to repo "myrepo" (branch "zz-verify"): 2 tab(s).
```

확인한 것:

- `worktree.created` 이벤트에서 레이아웃이 적용됨
- `worktree.opened`(기존 worktree를 다시 열 때)에서도 적용됨
- `repo = "*"` 와일드카드가 폴백으로 동작함
- 리포를 지정한 레이아웃이 와일드카드를 이김 (우선순위: repo+branch > repo > `*`+branch > `*`)

이건 매니페스트의 `[[events]]` 진입점이라, 여기까지 돌면 action / pane / event
**세 종류의 진입점이 모두 Windows에서 뜬다**는 게 확인된 셈입니다.

---

## 5. 검증 결과 요약

| 항목 | 결과 |
| --- | --- |
| `go test ./...` (평범한 셸) | 검증됨 |
| `go test ./...` (herdr pane 안) | 검증됨 (설정 디렉터리 누수 수정 후) |
| `go build` + `herdr plugin link .` | 검증됨 |
| `ping` — action 진입점 | 검증됨 |
| Quick Actions — picker 렌더링, 필터, Project/Global 그룹 | 검증됨 |
| Quick Actions — `command` / `select` / `form` | 검증됨 |
| Quick Actions — `[windows]` 명령 오버라이드 | 검증됨 |
| Quick Actions — 실패 시 pane 유지 | 검증됨 |
| Quick Actions — 예제 자동 시딩 | 검증됨 |
| Projects — picker (pane 진입점), 그룹 헤딩 | 검증됨 |
| Projects — 탭 / 분할 pane / 시작 명령 | 검증됨 |
| Projects — 헤드리스 `open <이름>` | 검증됨 |
| Projects — `working_dir`의 `~` 확장 | 검증됨 |
| Worktree — `worktree.created` / `worktree.opened` (event 진입점) | 검증됨 |
| Worktree — 와일드카드 및 우선순위 | 검증됨 |
| 설정 디렉터리 해석 | 검증됨 |

---

## 6. 알려진 문제

**`{{.Value}}`를 직접 쓰면 인용이 깨질 수 있습니다.**
템플릿이 `.Value`를 참조하지 **않으면** 값이 셸 인용을 거쳐 마지막 인자로 붙지만,
`{{.Value}}`를 명시적으로 쓰면 날것 그대로 치환됩니다. 그래서 입력값에 따옴표가
들어 있으면 명령이 깨집니다. 셸 인용용 템플릿 함수는 아직 없습니다.
URL이라면 `{{.Value | urlquery}}`로 피할 수 있습니다.
Windows 한정이 아니라 POSIX에서도 같습니다.

**`make` 관련 타깃은 Windows에서 안 돕니다.**
`Makefile`이 `mkdir -p` 같은 POSIX 명령을 씁니다. `make plugin-link` 대신
[2절](#2-빌드하고-개발용으로-붙이기)의 `go build` + `herdr plugin link .`를 쓰세요.

**`gofmt -l`이 거의 모든 파일을 뱉습니다.**
git이 Windows에서 CRLF로 체크아웃하기 때문입니다(36개 중 33개). 실제 포맷
문제가 아니고, 커밋할 때 git이 LF로 정규화하므로 무시해도 됩니다. 다만 이것 때문에
`gofmt -l`을 포맷 검사로 쓰기는 어렵습니다.

---

## 7. 미검증 항목

이번에 확인하지 **않은** 것들입니다. "안 된다"가 아니라 "안 돌려 봤다"는 뜻입니다.

- **`herdr plugin install joonhwan/herdr-plus@windows-support`로 GitHub에서 설치하는 경로.**
  이번엔 로컬 `plugin link`만 썼습니다. 매니페스트의 Windows용 `[[build]]` 단계가
  설치 시점에 실제로 도는지는 확인하지 않았습니다.
- **`{{opener}}` → `Start-Process` 실제 실행.** 브라우저를 띄우는 번들 예제
  (`google.toml`, `github.toml`, `open-repo.toml`, `google-search.toml`,
  `open-working-dir.toml`)는 실행하지 않았습니다. 템플릿·실행 경로 자체는
  파일을 쓰는 임시 액션으로 확인했습니다.
- **projects picker의 `ctrl+g` worktree 브랜치 생성**과 `config.toml`의
  `[worktree] branch_prefix` 설정.
- **picker에서 마우스 클릭/휠 조작.** 키보드 조작만 확인했습니다.
- **`select` 액션의 구분선/헤딩 옵션**(`label` 없는 항목).
- **herdr 액션 메뉴가 OS별 진입점을 걸러 주는지.** `herdr plugin action list`는
  unix용과 windows용을 둘 다 보여 줍니다. TUI 메뉴에서도 그대로 둘 다 보이는지,
  아니면 herdr가 platform으로 걸러 주는지는 확인하지 않았습니다.
- **이번 변경의 macOS/Linux 동작.** 단위 테스트로 `renderFor(ctx, "linux")`가
  기본 명령을 쓰는 것까지만 확인했고, 실제 unix 환경에서 돌려 보지는 않았습니다.

---

## 8. 원복 절차

개발 링크를 떼고 GitHub 설치본으로 돌아가려면:

```powershell
herdr plugin unlink cloudmanic.herdr-plus
herdr plugin install joonhwan/herdr-plus@windows-support
herdr plugin list
```

검증하면서 만든 것들을 지우려면:

```powershell
# 검증용으로 만든 워크스페이스 (직접 만든 것만 닫을 것)
herdr workspace close <workspace_id>

# worktree까지 만들었다면
herdr worktree remove --workspace <workspace_id> --force
```

설정 디렉터리에 넣은 검증용 `projects\*.toml`, `worktrees\*.toml`,
`quick-actions\*.toml`도 잊지 말고 지우세요. `worktrees\` 디렉터리 자체가
없으면 worktree 자동 레이아웃은 그냥 아무 일도 하지 않습니다.

로컬 빌드 산출물은 `bin\herdr-plus.exe` 하나뿐이고, 지워도 됩니다.
