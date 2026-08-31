# Changelog

WireGuide Plus의 모든 주요 변경 사항은 이 파일에 기록됩니다.

> 简体中文: [CHANGELOG.md](CHANGELOG.md) · English: [CHANGELOG.en.md](CHANGELOG.en.md) · 繁體中文: [CHANGELOG.zh-TW.md](CHANGELOG.zh-TW.md) · 日本語: [CHANGELOG.ja.md](CHANGELOG.ja.md)

## [1.1.10] - 2026-08-31

이번 버전은 1.1.9에서 보고된 세 가지 화면 문제를 수정하고 설정 인터랙션을 개선합니다: DNS 누출 테스트 화면의 너비 제한을 없애고 본체 DNS를 표시하며, 로그 레벨 필터를 정확 일치로 변경하고, 알림 시간 및 프록시 선택의 저장·표시를 수정했습니다. 커스텀 미러 / 로컬 프록시 입력란은 마지막으로 저장한 주소를 기억합니다.

### 🐛 수정

- **DNS 누출 테스트 너비** — 640px 최대 너비 제한을 제거하여 '기록', '경로' 화면과 동일하게 창 크기에 맞춰 확장됩니다.
- **본체 DNS 표시** — 테스트 대상은 모두 시스템 DNS 설정(수동 또는 DHCP)에서 가져오므로 각 행에 '시스템' 태그를 표시하여 VPN 제공 DNS와 구분하기 쉽게 했습니다.
- **로그 레벨 필터** — DEBUG / INFO / WARN / ERROR 버튼을 클릭하면 해당 레벨의 기록만 표시됩니다(기존에는 '이상' 필터라 해당 레벨 기록이 없으면 필터가 안 되는 것처럼 보였습니다).
- **알림 시간 설정** — 드롭다운을 '로그 보존 / 기록 보존 / 언어'와 동일한 동적 옵션 방식으로 변경하여 변경 사항이 올바르게 저장되고 다시 표시됩니다.
- **프록시 모드 표시** — 설정 화면을 다시 열면 '직접'으로 보이던 문제를 수정했습니다(Svelte는 함수 본문이 읽는 필드를 추적할 수 없어 `value={함수()}`가 첫 평가 후 갱신되지 않았습니다). 이제 반응형 재계산으로 저장한 미러 / 수동 모드가 표시됩니다.
- **프록시 주소 기억** — '커스텀 미러'나 로컬 프록시로 다시 전환하면 마지막으로 저장한 주소가 입력란에 자동 복원됩니다(예: 이전에 저장한 미러 접두사). 기록이 없으면 빈칸과 힌트가 표시됩니다.

## [1.1.9] - 2026-08-31

이번 버전은 앱 내 업데이트가 '다운로드 성공 후 설치 불가'가 되는 문제를 수정합니다: 업데이트 프로세스가 설치 프로그램 실행 전에 임시 다운로드 파일을 삭제하여 Windows에서 설치 프로그램 실행 시 '파일을 찾을 수 없음' 오류가 나고 릴리스 페이지로 폴백했습니다.

### 🐛 수정

- **앱 내 업데이트 설치 불가** — `runUpdateNative`가 `Install` 호출 전에 `os.Remove(path)`로 임시 다운로드 설치 프로그램을 삭제했지만, Windows 설치 경로는 해당 파일을 직접 실행하므로(`fork/exec …wireguide-update-*.exe: The system cannot find the file specified`) 다운로드 100% 후 항상 실행에 실패했습니다. 이제 설치 프로그램 실행이 시작된 후에 임시 파일을 해제하도록 수정했습니다. Windows에서는 설치 프로그램 실행 중 파일이 잠겨 삭제가 실패할 수 있지만, OS가 %TEMP%를 자동으로 정리하므로 무해합니다.
- **수동 업그레이드 1회 필요** — 1.1.7 / 1.1.8의 업데이트 프로세스에도 같은 문제가 있어 해당 버전에서 앱 내 업데이트를 하면 여전히 실패합니다. 1.1.9를 한 번 수동으로 설치해 주세요(설정 → 업데이트 → 릴리스 페이지 열기). 이후에는 앱 내 업데이트가 정상 동작합니다.

## [1.1.8] - 2026-08-31

이번 버전은 자동화 규칙의 판정 의미와 에디터 안내를 정렬하고, 구형 형식 규칙 처리를 더욱 견고하게 합니다: 규칙은 위에서 아래로 평가되어 먼저 맞는 조건이 적용되고, 같은 동작의 조건은 OR 관계이며, 그 외(otherwise)는 마지막에 두는 폴백으로 보통 위 규칙과 반대 동작을 수행합니다. 조건 유형이 없는 구형 규칙이 불필요한 리로드를 유발하지 않습니다.

### ✨ 개선

- **자동화 규칙 의미 안내 정렬** — 에디터 힌트와 '그 외' 행 설명을 업데이트: '그 외'는 위 규칙이 모두 안 맞을 때 발동하고, 마지막에 두어 폴백으로 쓰며, 동작은 보통 위 규칙과 반대로 설정(5개 언어 모두 동기화). 판정 로직 자체는 변경 없음: 순차 평가·첫 일치, `none_match`는 무조건 매치 — 기대하신 동작과 동일합니다.

### 🐛 수정

- **구형 형식 규칙에서 불필요한 리로드가 발생하지 않도록 수정** — 디스크와 로컬 규칙을 비교할 때 로드와 동일한 유형 추론을 사용(`type`이 없는 구형 '그 외' 규칙을 network로 회귀시키지 않음). 설정 변경마다 외부 편집으로 오판하여 추가 리로드가 발생하던 문제를 수정.

### 🛠 내부

- bindings를 재생성하고 Go API와 완전히 일치함을 확인(차이 없음).
- 버전을 **1.1.8**로 업데이트: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, NSIS, MSIX, Linux nfpm 모두 동기화.

## [1.1.7] - 2026-08-31

이번 버전은 1.1.6에서 보고된 문제를 수정합니다: 자동화 규칙이 사라지지 않도록 수정, DNS 누출 감지에 상태와 암호화 방식을 표시, 라우트 테이블에서 VPN/직접 구분, 로그 필터 수정, 알림 시간 및 프록시 표시 문제 해결. 연결 기록 보관 기간 설정과 설치 후 '실행' 옵션도 추가했습니다.

### 🐛 수정

- **자동화 규칙이 사라지지 않음(otherwise 포함)** — 조건 유형이 없는 규칙을 불완전한 것으로 오판하여 삭제하지 않도록 수정. 폼으로 표현할 수 없는 디스크의 규칙도 그대로 보존되어 설정을 열기만 해도 규칙이 사라지지 않습니다.
- **DNS 누출 감지 결과 보완** — 각 DNS 서버의 프로브 상태(VPN / 누출 / 정상 / 응답 없음)와 지연 시간을 정확히 표시하고, 실제 사용 중인 출구 DNS를 '사용 중' 표시로 구분.
- **DNS 암호화 방식 감지** — 각 리졸버의 전송 방식을 감지: 평문 UDP/53, DoT(TCP/853 TLS), DoH(TCP/443 후보). 테스트 후 결과 해석과 누출 방지 방법(VPN DNS, 암호화 DNS, 풀 터널 모드 등)을 표시.
- **라우트 테이블에서 VPN/직접 구분** — 백엔드가 활성 터널 인터페이스를 대조해 `is_vpn`을 판정하여 이름 추측이 아닌 정확한 VPN/Direct 배지를 표시.
- **로그 필터 수정** — 로그 이벤트에 `category` 필드를 추가하여 카테고리 필터가 실제로 동작하도록 수정. 레벨/카테고리 버튼에 각 개수를 표시하여 분포를 한눈에 파악 가능.
- **알림 시간 설정** — 일부 Svelte 버전에서 드롭다운이 비어 보이고 선택 시간이 표시되지 않는 문제 수정.
- **프록시 표시 일관성** — direct 모드에서 프록시 주소가 남지 않도록 하고, CLI로 변경한 프록시 모드도 설정 UI에 실시간 반영.

### ✨ 개선

- **연결 기록 보관 기간** — 설정 → 고급에 '기록 보관 기간'(기본 7일, 비활성화 가능)을 추가. 초과분은 자동으로 정리됩니다(200개 상한은 계속 적용).
- **설치 후 실행** — Windows 설치 프로그램 완료 페이지에 'WireGuide Plus 실행' 옵션(기본 선택)을 추가.

### 🛠 내부

- 버전을 **1.1.7**로 업데이트: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, NSIS, MSIX, Linux nfpm 모두 동기화.

## [1.1.6] - 2026-08-30

이번 버전은 업데이트 메커니즘을 개선합니다. Windows / Linux에서 앱 내에서 직접 업데이트를 다운로드하여 설치할 수 있고(GitHub 페이지로만 이동하던 방식에서 개선), 업데이트 알림에 '지금 업데이트'와 '릴리스 페이지 열기' 두 버튼과 실시간 다운로드 진행률을 제공합니다. 미러 모드에서는 자산 다운로드도 가속 미러를 통과합니다.

### ✨ 새로운 기능

- **앱 내 직접 업데이트(Windows / Linux)** — 업데이트 알림에 '지금 업데이트' 버튼 추가. 다운로드 완료 후 SHA256(릴리스 버전에는 Ed25519 서명 포함)을 검증하고, 통과하면 설치 프로그램을 실행한 뒤 앱을 종료합니다. macOS의 Homebrew 설치는 기존대로 `brew upgrade`를 사용합니다.
- **'릴리스 페이지 열기' 대체 버튼** — 다운로드 실패, 검증 실패, 또는 릴리스 노트를 확인하고 싶을 때 해당 버전의 GitHub Release 페이지를 브라우저에서 한 번에 엽니다.
- **실시간 다운로드 진행률** — 업데이트 중 다운로드된 / 전체 크기와 진행률(%)을 표시합니다(GitHub API가 보고하는 자산 크기를 기준으로 하므로 청크 전송에서도 정확합니다).
- **미러 모드가 자산 다운로드에도 적용** — GitHub 가속 미러(mirror)를 설정하면 자산과 체크섬 파일 다운로드도 미러 접두사로 다시 작성됩니다(기존에는 API 확인만 미러를 사용하고 바이너리는 GitHub에 직접 연결).

### 🛠 내부

- 다운로드·설치 실패 시 조용히 넘어가지 않고 로그를 기록한 뒤 릴리스 페이지 열기로 대체합니다. 새 버전으로 가는 경로가 항상 보장됩니다.
- 다운로드 진행률 콜백, 미러 다운로드 재작성, `RunUpdate` 방어 분기의 단위 테스트 추가.
- 버전을 **1.1.6**으로 업데이트: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, `windows/wails.exe.manifest`, NSIS, MSIX, Linux nfpm, macOS `Info.plist` 모두 동기화.

## [1.1.5] - 2026-08-30

이번 버전은 로그 시스템을 대폭 강화하고(업데이트 확인, 설정 감사, 분류/등급, 보존 기간 정리), 일부 설정 문제를 수정하며, 기본적으로 꺼져 있는 WireGuard 스크립트 지원을 다시 제공합니다.

### ✨ 새로운 기능

- **업데이트 확인 전체 로그** — 수동·자동 확인 모두 실제 요청한 endpoint, 로컬 버전, 최신 버전, `not_modified`, 오류/재시도 정보를 기록합니다. 실패(403, 타임아웃 등)는 `category=update`로 표시되어 Log 화면에서 확인·필터링할 수 있습니다.
- **설정 변경 감사 로그** — 저장할 때마다 변경된 설정(프록시 모드, kill switch 등)과 주요 값을 기록합니다. 프록시 자격 증명은 마스킹됩니다(`http://***@host`).
- **로그 분류 및 필터링** — `ipc.LogEntry`에 `category` 필드 추가(app / update / settings / tunnel / network / system). Log 화면에 분류 필터 행 추가(All이 맨 앞, 기본 선택). 각 줄에 분류를 표시하고 복사 시에도 포함됩니다.
- **로그 보존 기간(기본 7일)** — 일별 순환 저장(`wireguideplus-YYYY-MM-DD.log`) 후, 설정 가능한 보존 기간이 지난 파일을 자동 삭제합니다.
- **WireGuard 스크립트 지원(PreUp / PostUp / PreDown / PostDown, 기본 꺼짐)** — wg-quick과 동일한 동작(Unix는 `sh -c`, Windows는 `cmd.exe /C`). helper 내에서 30초 타임아웃으로 실행하고 출력은 1000자로 제한합니다. 기본 꺼짐(설정 → 고급). 전체 시스템 권한으로 실행되므로 활성화 시 눈에 띄는 보안 경고를 표시합니다. PostUp 실패로 연결이 중단되지는 않습니다.
- **DNS leak test 강화** — 각 DNS 서버에 프로브 상태(vpn / ok / leak / timeout)와 지연 시간을 표시합니다. Windows DNS 수집 시 IPv4와 IPv6를 모두 다룹니다.
- **폴더 열기 바로가기** — 설정에 터널 설정 폴더와 로그 저장 폴더를 여는 클릭 가능한 링크를 추가(크로스 플랫폼).

### 🐛 버그 수정

- **알림 표시 시간 설정을 저장할 수 없던 문제** — 설정 화면을 나갔다가 다시 열어도 값이 리셋되지 않습니다.
- **설정의 로그 등급에 All이 없던 문제** — 드롭다운에 `All` 추가(Log 화면 기본값과 일치). 싱크 단계에서 레코드가 필터링되지 않습니다.

### 🛠 내부

- **로그 등급 All 전 구간 활성화** — helper/GUI 로그 핸들러가 `all`(`slog.Level(-8)`)을 해석하여 레코드를 놓치지 않습니다.
- 버전을 **1.1.5**로 업데이트: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, `windows/wails.exe.manifest`, NSIS, MSIX, Linux nfpm, macOS `Info.plist` 모두 동기화.

## [1.1.3] - 2026-08-30

이번 버전은 Windows 자동 업데이트가 동작하지 않던 문제를 수정합니다. v1.1.0 자산 이름 변경 이후 Windows 릴리스 자산(`wireguideplus-<arch>-installer.exe` / `wireguideplus-<arch>-portable.zip`)은 OS 토큰을 포함하지 않는데, 업데이트 체커는 자산 이름에 OS 토큰과 아키텍처가 모두 필요했습니다. 따라서 Windows는 자신의 자산과 전혀 매칭되지 않아 '업데이트가 있지만 일치하는 자산 없음' 상태로 자동 업데이트를 할 수 없었습니다.

### 🐛 버그 수정

- **Windows 자동 업데이트 자산 매칭 수정** — `matchAsset`(`internal/update/checker.go`)은 이제 Windows에서 아키텍처 고정 + Windows 전용 확장자(`.exe` / `.msi` / `.zip`)를 가진 자산 이름을 OS 토큰 없이도 허용합니다. macOS / Linux 자산은 여전히 각 OS 토큰(`darwin` / `linux`)이 필요하므로 OS 토큰이 없는 Windows 자산 이름에 잘못 매칭되지 않습니다. 회귀 테스트는 세 가지 Windows 아키텍처의 정상 매칭과, Linux / macOS가 OS 토큰 없는 Windows 자산 이름을 거부해야 한다는 역방향 단언을 다룹니다.

### 🛠 내부

- 버전을 **1.1.3**로 업데이트: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, `windows/wails.exe.manifest`, NSIS, MSIX, Linux nfpm, macOS `Info.plist` 모두 동기화.

## [1.1.2] - 2026-08-30

이번 버전은 Windows 파일 버전 불일치를 수정합니다. 배포된 1.1.1 설치 프로그램에서 실행 파일(`wireguideplus-<arch>.exe`)의 "파일 버전"이 **1.1.0.1**로 표시되었습니다(올바른 값은 **1.1.1.0**).

### 🐛 버그 수정

- **Windows 실행 파일 버전 불일치 수정** — 근본 원인: `goversioninfo v1.7`은 `FixedFileInfo` 구조체를 `Major/Minor/Patch/Build` 순서로 선언합니다(Windows 표준 레이아웃과 Build/Patch가 바뀜). JSON에 숫자 버전을 명시적으로 쓰면 뒤바뀐 바이너리 버전이 생성되었습니다(`1.1.1.0`이 `1.1.0.1`로 표시). 이제 `build/windows/versioninfo.json`의 `FixedFileInfo` 숫자는 0으로 고정하고, 유일한 입력으로 `StringFileInfo` 4자리 버전 문자열만 사용하며, goversioninfo가 그로부터 바이너리 버전을 도출합니다(레이아웃 독립, 항상 일치). `tools/genverinfo`는 문자열 버전만 렌더링하고, `tools/bumpversion`은 숫자 필드를 건드리지 않습니다. 검증 완료: `1.1.2.0` 문자열을 전달하면 goversioninfo가 `FixedFileInfo.FileVersion (1.1.2.0)`을 출력하며, 설치 후 속성 페이지와 `FileVersionInfo` 모두 올바르게 표시됩니다.

### 🛠 내부

- 버전을 **1.1.2**로 업데이트: `VERSION`, `build/config.yml`, `windows/info.json`, `windows/versioninfo.json`, `windows/wails.exe.manifest`, NSIS(`wails_tools.nsh` + `project.nsi`), MSIX, Linux nfpm, macOS `Info.plist` 모두 동기화.
- NSIS 설치/제거 프로그램 설명(`project.nsi`)을 수정하여 설치 프로그램과 제거 프로그램의 버전 정보가 배포된 실행 파일과 일치하도록 했습니다.

## [1.1.1] - 2026-08-30

이번 버전은 Windows 트레이 알림 풍선의 '메인 창 열기' 버튼이 시스템 고부하 시 간헐적으로 GUI를 멈추게 하던 문제를 수정합니다.

### 🐛 버그 수정

- **알림 풍선의 '메인 창 열기'가 간헐적으로 GUI를 멈추지 않게 수정** — 시스템 CPU 경쟁이 심할 때(예: Windows 유지 관리 프로세스가 코어를 점유) 또는 WebView2 응답이 지연될 때, 트레이 알림 풍선의 'Open Window' 버튼을 클릭하면 UI 스레드를 동기적으로 차단·대기하여 GUI 전체가 멈춘 것처럼 보였습니다(VPN 터널은 영향 없음). `showDock`(`internal/gui/dock_other.go`)은 `application.InvokeAsync`를 통해 Wails UI 스레드에서 비동기 실행되도록 변경했습니다. 호출자는 즉시 반환되고, 창 표시/포커스는 모두 UI 스레드에서 인라인으로 처리되어 스레드 간 대기가 없습니다. 또한 recover 가드를 추가하여 예상치 못한 panic이 메인 스레드 콜백 체인을 중단하지 않습니다.

### 🛠 내부

- 버전을 **1.1.1**로 업데이트: `internal/update/checker.go` 메인 버전, `build/config.yml`, `windows/info.json`(`1.1.1.0`), `windows/wails.exe.manifest`, NSIS(`wails_tools.nsh`), MSIX, Linux nfpm, `tools/genverinfo` 모두 동기화.

## [1.1.0] - 2026-08-28

이번 버전은 식별성, 프록시 견고성, 시작 시 자동화 규칙에 중점을 둡니다. 트레이 상태는 식별하기 쉬운 글리프로 변경하고, 프록시는 세 가지 모드의 의미를 명확히 하고 연결 테스트를 추가했으며, 잘못된 프록시 URL이 업데이트 확인을 깨뜨리지 않게 했고, 시작 시 자동화 규칙을 먼저 판단한 후 연결합니다.

### ✨ 새로운 기능

- **트레이 상태 글리프 식별성 개선 (Tray state glyphs)** — Windows 트레이 메뉴의 연결 상태를 순수 텍스트 글리프로 구분하도록 변경했습니다. `●` 채움=연결됨, `○` 비움=연결 안 됨(Windows 트레이 팝업은 GDI로 그려져 컬러 이모지를 렌더링할 수 없으며, `🟢`는 회색 외곽선으로 퇴화하여 새/구 상태를 구분하기 어렵습니다). macOS 메뉴 바(AppKit 네이티브 렌더링)는 계속 컬러 이모지를 사용합니다. 시작 중/전환 상태는 별도 표시가 있습니다.
- **프록시 3-모드 의미 명확화 + 연결 테스트 (Proxy modes & test)** — 설정 → 프록시의 옵션을 세 가지로 통일하고 의미가 더 이상 혼동되지 않게 했습니다. **직접 연결**(시스템/환경 프록시를 완전히 무시), **GitHub 미러**(`mirror`, 예: `https://ghfast.top` 가속 접두사), **수동 프록시**(`manual`, http/https/socks5 전체 URL). **"연결 테스트"** 버튼을 추가했습니다. 저장 전에 GitHub Releases API에 왕복 요청을 보내 성공 여부와 지연 시간을 보고합니다.
- **프록시 설정 즉시 적용 (Proxy applies immediately)** — 프록시 구성을 저장하면 다음 예약 업데이트 확인(및 수동 "지금 확인")이 재시작 없이 바로 적용됩니다. GUI 시작 시에도 저장된 프록시를 바로 적용하여 "시작 직후 잘못된 구성으로 검사가 실행되는" 상황을 방지합니다.

### 🐛 버그 수정

- **잘못된 프록시 URL이 업데이트 확인을 깨뜨리지 않도록 수정** — `config.json`에 불완전한 수동 프록시(예: `proxy_url = "https://"`)가 있으면 기존에는 `http.ProxyURL`이 그대로 사용되어 매번 업데이트 확인 시 `proxyconnect tcp: tls: either ServerName or InsecureSkipVerify must be specified in the tls.Config` 오류가 발생했습니다. 이제 시작 시와 사용할 때마다 URL을 검증하고(`internal/update/proxy.go`), 잘못된 값은 `WARN update: ignoring invalid manual proxy URL`을 기록하고 직접 연결로 폴백하므로 검사가 더 이상 실패하지 않습니다.
- **"먼저 연결 후 규칙에 따라 해제"되는 시작 인상을 수정** — 시작 규칙 평가를 helper 시작 직후로 앞당겨 실행하고(로그 `startup rule re-evaluation`), 각 터널의 목표 상태가 규칙에 의해 먼저 결정되도록 했습니다. 또한 `scheduleRuleCheck` 폴백을 추가했습니다. 시작 후 60초 창 내의 모든 RPC 수동 연결(예: 마지막 세션 복원)은 3초 후 규칙에 따라 다시 평가되어 수정되며, 30초 폴링을 기다리지 않습니다. 로그에 트리거 출처를 기록하여 문제 해결에 도움이 됩니다.
- **잘못된 미러 접두사가 조용히 검사를 깨뜨리지 않도록 수정** — `mirror` 모드의 가속 접두사도 scheme/host 검증을 수행하며, 잘못된 값은 공식 API 엔드포인트로 폴백합니다.

### 🛠 내부

- 버전을 **1.1.0**으로 업데이트: `internal/update/checker.go` 메인 버전, `build/config.yml`, `windows/info.json`(`1.1.0.0`), `windows/wails.exe.manifest`, NSIS, MSIX, Linux nfpm 모두 동기화.
- **Windows 버전 리소스 표준화** — `wails3 generate syso`로 생성된 버전 리소스는 언어가 `0x0000`이고 `VS_FIXEDFILEINFO.ProductVersion`이 0이라서 Windows 탐색기 / `FileVersionInfo`가 읽을 수 없었습니다(속성 페이지 버전 필드가 비어 있음). `goversioninfo`(구성: `build/windows/versioninfo.json`)로 전환하여 표준 `0409/04B0` 리소스를 생성하도록 하고 `generate:syso` 작업도 함께 업데이트했습니다. 이제 exe와 설치 패키지의 속성 페이지에 `1.1.0`이 올바르게 표시됩니다.
- **Windows x86(32비트) 빌드 추가** — `task windows:build ARCH=386`로 32비트 실행 프로그램과 `wireguide-x86-installer.exe` 설치 패키지를 생성합니다(NSIS 스크립트가 x86 아키텍처를 지원하고, Program Files에 설치하며, x86용 `wintun.dll`을 패키징).
- **플랫폼 경계 명확화** — iOS 빌드 작업과 구성 주석을 제거했습니다. 이 프로젝트는 Android / iOS를 지원하지 않습니다(멀티 채널 동시 연결 불가, SSID 기반 자동 연결 불가). README에 그 내용을 반영했으며, macOS / Linux 강화판은 추후 개발 예정입니다.
- **시스템 통합 강화** — **"최소화 시작"** 설정 추가(시작 시 메인 창을 표시하지 않고 시스템 트레이로 바로 최소화, 설정 → 시작). **연결 상태 트레이 알림** 추가: 시작 후 10초 지연 후 현재 연결 상태를 표시하고, 네트워크 변경(Wi-Fi 전환, 랜 케이블 분리/연결, 네트워크 끊김 등)으로 터널 연결 상태가 바뀌면 안정화된 최신 상태를 10초 지연 후 표시합니다. 알림 풍선에는 작업 메뉴(메인 창 열기 / 연결 끊기)가 있으며, 수동으로 닫거나 설정에 따라 자동으로 닫을 수 있습니다(기본 10초 유지, 설정 → 시작 → 알림 유지 시간에서 조정, `internal/gui/notify_windows.go`).
- **듀얼 아키텍처 릴리스** — 매 빌드마다 32비트(x86)와 64비트(amd64) 프로그램과 해당 설치 패키지를 함께 생성합니다(`task windows:build:all`, wintun.dll 아키텍처 자동 갱신 포함). 소프트웨어/설치 패키지 설명을 "다중 터널 + 자동화" 중심으로 통일하고, 크로스 플랫폼(cross-platform) 표현을 제거했습니다.
- **설치 경험** — 설치 패키지는 기본적으로 Program Files에 설치되며(32비트 설치 패키지는 Program Files (x86)을 자동 선택), 설치 중에 디렉터리를 직접 지정할 수 있습니다. 시작 메뉴 바로 가기("WireGuide Plus 제거" 항목 포함, 제거 항목 아이콘은 실행 프로그램과 동일)가 기본 생성되며 "바로 가기 옵션" 페이지에서 체크를 해제할 수 있습니다. 바탕 화면 바로 가기는 항상 생성됩니다(`build/windows/nsis/project.nsi`).
- **개발 및 릴리스 문서** — 빌드/패키징 설명을 README에서 별도 개발 문서 `docs/DEVELOPMENT.md`로 옮겼습니다. GitHub Release 워크플로우에 32비트 Windows 산출물과 CI 툴체인(goversioninfo)을 추가했으며, 로컬에서 `v*` 태그를 푸시하면 자동으로 빌드(Windows x86+amd64, macOS arm64, Linux amd64+arm64), 서명 및 배포됩니다(`docs/release.md`).
- Windows 네트워크 어댑터 이름 매칭 로직 조정(`internal/wifi/known_windows.go`, `detect_windows.go`), 물리 네트워크 카드 인식이 더 정확해졌습니다.
- 창 제목을 **WireGuide Plus**로 통일.
- 업데이트 확인이 스케줄러 내에서 중복 제거되어 한 라운드에 여러 번 트리거되지 않습니다(실패는 한 번만 기록하고 재시도 간격을 제공).

## [1.0.0] - 2026-08-28

이정표(milestone) 버전: A11y 접근성 의미론 리팩터링, Windows 네트워크 출구 라우팅 로직 조정, Wails3 빌드/아이콘/권한 정리, 그리고 중국어 간체 UI와 트레이 토글이 추가되었습니다.

### ✨ 새로운 기능

- **중국어 간체 UI (Chinese UI)** — 전체 인터페이스에 중국어 간체 번역을 추가했습니다. 터널 목록, 기록, 도구(DNS 누출 테스트/라우팅 테이블), 로그, 설정, 업데이트, 자동화 편집기 등 총 199개 문자열을 모두 커버합니다. 첫 실행 시 시스템 언어를 자동으로 따르며(`zh-*` 로케일 자동 감지), 설정 → 일반 → 언어에서 수동으로 전환하고 영구 저장할 수도 있습니다.
- **트레이 메뉴 토글 (Tray toggles)** — 시스템 트레이의 각 터널이 독립적으로 클릭 가능한 토글이 되었습니다. 체크하면 연결, 체크 해제하면 연결 끊김. 연결 상태 이모지(🟢 연결됨/🟡 연결 중/○ 연결 안 됨)는 라벨 옆에 유지됩니다. 수동으로 끈 터널은 다시 연결하거나 WireGuide를 재시작할 때까지 자동화 규칙 면제(manual-off)가 유지됩니다.

#### 프런트엔드 A11y 접근성 리팩터링

> 영향 범위: 전 플랫폼(Windows/macOS/Linux) Svelte 프런트엔드, Windows에 한정되지 않음.

- 모든 모달 오버레이에서 스크림의 `role="button"`과 `tabindex="0"`을 제거했습니다. 스크림이 순수한 마스크 의미로 돌아가, 화면 읽기 프로그램이 전체 화면 배경을 상호작용 가능한 버튼으로 인식하지 않게 됩니다.
- 모든 dialog에 `tabindex="-1"`을 통일하고 표준 `role="dialog" aria-modal="true"`를 유지하여 WCAG 팝업 의미론 규범을 따릅니다.
- ESC 닫기 통합 처리: 누락되어 있던 팝업(가져오기 결과, 기록, 업데이트 알림, 자동화 편집기)에 **컴포넌트 최상위**에서 `<svelte:window on:keydown>`을 연결했습니다(handler 내에서 팝업 상태를 조건으로 판단, Svelte는 `{#if}` 내부에 마운트할 수 없음). 나머지 팝업은 App.svelte의 전역 capture 핸들러를 재사용하여, 다중 팝업 ESC 충돌을 피하면서 CodeMirror의 키 캡처를 깨뜨리지 않습니다.
- `Settings.svelte`: `<nav role="tablist">`를 일반 `<div>`로 변경하여 탭 의미론 불일치 경고를 제거했습니다. 분할 막대 `pane-resizer`는 `role="separator"`를 유지하면서 `tabindex="0"`과 실제 키보드 조작(방향 키로 너비 조정, Enter/Space로 초기화)을 추가했습니다.
- `frontend/vite.config.js`의 svelte 플러그인 `onwarn`이 정적 오탐을 필터링합니다(`a11y_click_events_have_key_events`, `a11y_no_static_element_interactions`, `a11y_no_noninteractive_tabindex`, `a11y_no_noninteractive_element_interactions`). 프로덕션 빌드 경고가 0이 되며 비즈니스 로직 변경은 없습니다.
- 관련 파일: `src/App.svelte`, `src/lib/History.svelte`, `src/lib/ConflictWarning.svelte`, `src/lib/TunnelDetail.svelte`, `src/lib/UpdateNotice.svelte`, `src/lib/Settings.svelte`, `src/lib/AutomationEditor.svelte`

#### Windows 백그라운드 helper: 네트워크 출구 라우팅 로직 조정

> 영향 범위: Windows 플랫폼 Go helper 코드만, 다른 플랫폼은 변경 없음.

- helper 시작 단계에서 주 업스트림 물리 네트워크 카드의 LUID를 수집하여 시스템 초기 기본 출구 물리 인터페이스를 기록합니다. 이 LUID는 시작 시점의 스냅샷으로, 런타임 중 네트워크 전환 시 캐시가 자동으로 갱신되지 않습니다.
- 네트워크 인터페이스 필터링 로직 수정: TUN/터널/루프백 가상 네트워크 카드를 필터링하고 물리 네트워크 카드만 업스트림 후보로 선택합니다. TUN 가상 네트워크 카드 자체에는 물리 네트워크 카드 바인딩 잠금을 적용하지 않습니다.
- WireGuard UDP 패킷의 출발은 전적으로 Windows 라우팅 테이블 + 네트워크 카드 InterfaceMetric 홉 수로 선택됩니다. 소프트웨어가 더 이상 특정 물리 네트워크 카드에 강제로 바인딩하지 않습니다.
- 분할 터널 모드(`full_tunnel=false`) 논리 제약 추가: Peer Endpoint IP를 `AllowedIPs`에 명시적으로 추가해야 합니다. 핸드셰이크 UDP 패킷이 라우팅에서 버려져 `no-handshake`가 발생하는 것을 방지합니다.
- 로그 강화: `network primary upstream interface initial luid`가 주 물리 네트워크 카드 LUID를 출력하여 문제 해결에 사용합니다. 로그의 `tunnel connected`는 TUN 어댑터가 준비되었음을 의미할 뿐, 원격 Peer 핸드셰이크 성공과 동일하지 않다는 점을 명확히 했습니다.
- 문제 해결 도구 팁: Windows에서는 `Find-NetRoute -RemoteIPAddress <peer-ip>`를 우선 사용하여 대상 IP의 실제 출구 네트워크 카드를 확인하세요. PowerShell `Get-NetAdapter.Luid`는 구조체이므로 Go가 출력하는 uint64 값과 직접 등가 비교할 수 없습니다.

### 🛠 빌드 및 엔지니어링

주로 Windows 빌드 동작이며, 크로스 플랫폼 부분은 별도로 표기했습니다.

1. **Wails3 Windows 아이콘 빌드 동작**(Windows 전용) — `task build` 전체 빌드는 자동으로 `wails3 generate icons`를 실행하여 `build/appicon.png`를 읽고 `windows/icon.ico`를 덮어씁니다. 수동으로 수정한 `windows/icon.ico`는 전체 빌드 시 덮어써집니다. `windows/icon.ico`가 최종적으로 exe에 임베드되는 아이콘이며, `build/appicon.png`는 소스 자산일 뿐입니다. `task windows:build` 디버그 빌드는 아이콘 생성을 건너뛰고 기존 `windows/icon.ico`를 유지합니다. exe / 창 제목 표시줄 / 작업 표시줄 아이콘은 exe 내부의 ico 리소스를 재사용하며, 시스템 트레이 아이콘은 Go `embed` 독립 리소스가 필요합니다.
2. **Windows 버전 정보 관리**(Windows 전용) — exe 파일 상세 정보는 `windows/info.json`이 제어하며, `FileVersion`은 4자리 숫자 형식 `major.minor.patch.build`이어야 합니다. UI에 표시되는 버전은 Go 상수로 유지되며(`internal/update/checker.go`), `info.json`과 수동으로 동기화해야 합니다. 향후 ldflags 컴파일 주입을 통해 단일 버전 소스로 통합할 수 있습니다.
3. **Windows UAC / 관리자 권한 정리**(Windows 전용) — 현재 아키텍처는 GUI 프로세스가 helper 하위 프로세스를 실행합니다. helper가 TUN 네트워크 카드를 조작하고 라우팅을 수정하려면 관리자 권한이 필요하며, 하위 프로세스 권한 상승은 UAC 팝업을 트리거합니다. Windows 보안 메커니즘을 완전히 조용히 우회할 수는 없습니다. 단기 방안: `windows/wails.exe.manifest`에 `requireAdministrator`를 추가하여 UAC 팝업을 exe 더블 클릭 시작 시점으로 옮깁니다(여전히 사용자 확인 필요). 장기 권장: helper를 Windows 시스템 서비스로 재구성(LocalSystem 권한으로 백그라운드 실행)하고, GUI는 일반 사용자 권한으로 IPC를 통해 통신하여 UAC 팝업을 완전히 제거합니다.

### 🐛 문제 조사 기록

조사 기록이며 코드 변경이 없습니다. 개발 참고용입니다.

- 증상: helper 로그에 `tunnel connected`가 출력되지만 GUI에 `no handshake`가 표시됩니다.
  - 근본 원인 구분: TUN 장치 생성 완료 ≠ WireGuard가 원격 Peer와 암호화 핸드셰이크를 완료함. wg 커널의 `latest handshake` 상태를 읽어 실제 연결성을 판단해야 합니다.
  - 분할 터널 모드에서 자주 겪는 함정: Peer IP가 `AllowedIPs`에 없어 핸드셰이크 UDP 패킷이 라우팅에서 버려집니다.
  - 기타 가능성: Windows 아웃바운드 방화벽이 WireGuard UDP를 차단, endpoint 도메인 DNS 해석 오류.
- 로컬 프록시가 `0.0.0.0`을 수신: 프록시 프로세스 트래픽은 독립적이며 WireGuard 터널로 자동 유입되지 않습니다. 트래픽 경로는 Windows 라우팅 테이블과 터널의 `AllowedIPs`가 함께 결정합니다.

### 📝 참고

1. **변경 영향 범위 구분**
   - Svelte 프런트엔드 A11y 코드: **전 플랫폼 적용(Windows / Linux / macOS)**. 팝업 ESC, 접근성 의미론 변경은 모든 데스크톱 플랫폼에 적용됩니다.
   - helper 네트워크 출구 라우팅 로직: **Windows 플랫폼 Go 코드만 수정**, 다른 OS는 영향 없음.
   - 빌드, manifest, ico, info.json, UAC 관련: **Windows 플랫폼만 해당**.
2. 프런트엔드 A11y 수정과 helper 백그라운드 네트워크 로직은 완전히 분리되어 있으며, 터널 생성, 라우팅, 자동화 Wi-Fi 규칙 실행에 영향을 주지 않습니다.
3. helper가 기록하는 업스트림 LUID는 시작 순간의 스냅샷일 뿐입니다. Wi-Fi/유선 네트워크 전환 시 이 값이 자동으로 갱신되지 않습니다.

## [0.5.1] - 2026-08-11

Patch release: the in-app "Update Now" button is now trustworthy on macOS. If you are on 0.5.0 via Homebrew, this is also the first update the button itself should complete cleanly end-to-end.

### Fixed
- **macOS "Update Now" (issue #38)** — the in-app update can no longer report success without actually installing: after `brew upgrade` exits, the installed bundle's version is verified against the release it claimed to install, progress phases ("refreshing" / "installing") are shown in the banner and About panel, and failures surface inline instead of vanishing behind a relaunch. Also survives Homebrew 6's tap-trust gate (`untrusted tap` errors trigger a `brew trust` + one retry) and skips the redundant `brew update` (`HOMEBREW_NO_AUTO_UPDATE=1` — the checker already knows the target version).
- The Homebrew cask itself dropped `auto_updates` (korjwl1/homebrew-tap), so bulk `brew upgrade` no longer skips WireGuide — the root cause of months of silent non-updates.

## [0.5.0] - 2026-08-10

Linux graduates to a supported platform, the CLI learns to start and stop the app, and the Windows helper's IPC surface is locked down to the launching user. Verified on all three OSes before release: a full runtime pass on Windows 11 against a real tunnel (helper IPC, multi-tunnel, kill-switch cycles, CLI lifecycle, tray), the Linux plan in `docs/linux-test-plan.md` on Debian 13 / Raspberry Pi OS ARM64, and the macOS DNS/lifecycle fixes below.

### Added
- **Linux support** — tested and hardened end-to-end on Debian 13 / Raspberry Pi OS ARM64 (Wayland and X11): window decorations restored after tray-restore, gateway/physical-interface detection fingerprints the right network (issue #22), routine RTNETLINK traffic no longer registers as a primary-network change (reconnect decisions compare real default-route snapshots), nftables kill-switch fixes, DEB packaging via nfpm.
- **`wireguide ctl start` / `ctl stop`** — explicit app lifecycle from the CLI. `start` launches the app detached and waits for the helper (long deadline: the macOS admin prompt has no timeout of its own; on macOS it launches its *own* bundle rather than whatever LaunchServices resolves); `stop` quits GUI and helper together and confirms they actually went away. Deliberately the only commands that start anything — `connect`/`status` still refuse rather than boot a VPN stack behind your back.
- **`--json`** on `ctl status` and `ctl list` for scripts and coding agents.
- **CI: 3-OS test matrix** (Linux/macOS/Windows) on every PR; release workflow untouched.

### Security
- **Windows helper pipe scoped to the spawning user (issue #20)** — the named pipe's ACL now grants access to the launching user's SID instead of every interactive user, and each connection's peer SID is verified against it (SYSTEM and a helper spawned without the SID keep working). Verified live on Windows 11 by reading back the pipe's security descriptor.

### Fixed
- **Windows multi-tunnel** — connecting a second tunnel no longer fails on the Wintun adapter name collision; each tunnel gets its own `WireGuide-<id>` adapter, and multi-tunnel status reports per-tunnel interface/duration/traffic instead of zeroed copies.
- **Helper lifetime** — the helper never runs at boot and its lifetime is tied to the GUI: a 60 s startup grace covers a helper whose GUI never attaches (login-autostart with an unanswered UAC prompt no longer leaves an invisible elevated process), and a teardown that leaves no tunnels and no GUI re-arms the shutdown grace window — closing the orphan-helper hole that transient CLI connections opened (a GUI-less `ctl disconnect` of the last tunnel previously left the elevated helper alive until reboot). CLI clients are excluded from connection-lifecycle tracking by design.
- **Kill switch** — rebuilt atomically around every connect/disconnect from actual manager state; a failed connect restores the blockade instead of leaving it half-applied.
- **macOS DNS teardown (issue #34)** — search domains, services added mid-session, and the failed-verify / ForceShutdown paths now all restore DNS.
- **macOS updates (issue #38)** — "Update Now" runs `brew upgrade --greedy` so cask-held updates can't silently no-op.
- **Diagnostics (issue #32)** — ping parsing is locale-agnostic (Korean Windows included), and unreachable hosts report as unreachable instead of a fabricated wall-clock-derived latency.
- **Automation** — rules are validated on save: a malformed CIDR or MAC is rejected with a clear error instead of being written and silently never matching.
- **Idle efficiency** — Wi-Fi polling backs off to 60 s while native change notifications are attached; config-file watching drops from 1 s to 3 s; endpoint-latency logging demoted to debug.

### Removed
- Key generator, CIDR calculator, speed test, mini mode, and the split-tunnel UI stub — dead or abandoned surfaces found in the audit sweep (#35); their bindings and i18n strings went with them.

## [0.4.2] - 2026-07-27

**Urgent fix release for Windows users.** 0.4.1 and earlier shipped with a tray that could permanently lose the main window and an installer that cannot upgrade in place while the app is running. Windows users should update; to get past the installer bug one last time, run `taskkill /F /IM wireguide.exe` from an elevated terminal before launching the 0.4.2 installer. macOS and Linux are unaffected by the tray-window bug (Linux picks up the same Show Window fix), and nothing else changed.

### Fixed
- **Windows tray, issue #30** — left-clicking the tray icon now shows the main window (the platform convention; previously a no-op), and the "Show Window" menu item actually works: it was wired to a macOS-only implementation, so on Windows **a window closed to the tray could never be reopened** — the only recovery was killing the process. The tray menu also showed stale connection state (○ while connected) because menu refills never reached the Win32 popup; the menu now rebuilds through `SetMenu` on every change. macOS behavior is unchanged; Linux gains the same Show Window fix.
- **Windows installer, issue #29** — upgrading by running the installer while WireGuide was running failed with "Error opening file for writing: wireguide.exe" (the GUI and the elevated helper are the same executable, and Windows locks running images; the helper deliberately outlives the GUI, so quitting the tray app wasn't enough). The installer and uninstaller now terminate running instances before touching files. **This fix takes effect when the 0.4.2 installer runs — upgrading *to* 0.4.2 still hits the old installer's bug**, hence the elevated `taskkill` workaround above.

## [0.4.1] - 2026-07-27

### Fixed
- **Automation (GUI), issue #27** — creating or editing rules in the Automation editor was effectively impossible in 0.4.0: the editor's own autosave re-fired the config watcher, and the resulting reload wiped the just-added row before it could be filled in (and could transiently delete a rule being edited). The editor now ignores its own writes (reloading only when the file genuinely changed externally), a blank draft row is no longer autosaved, and a rule that is momentarily incomplete mid-edit keeps its last saved value on disk instead of being deleted. External edits (`wireguide ctl`, another window) still appear live.
- **Automation (GUI)** — per-tunnel rule saves now go through the cross-process-locked settings update instead of a whole-settings overwrite, so a GUI rule edit can no longer clobber a concurrent `wireguide ctl` change to any other setting (and vice versa); condition labels survive the GUI round-trip; a dash- or bare-hex-formatted gateway MAC written by the CLI is no longer treated as a foreign change.
- **Windows (dev):** `go test ./internal/ipc` no longer fails/panics when run unelevated — the tests accept the test binary's own pipe (test builds only; the production SY/BA pipe-owner check is unchanged) (#24).

## [0.4.0] - 2026-07-15

### Added
- **Automation** (issue #12) — per-tunnel `condition → action` rules that connect or disconnect a tunnel based on the network you're on. Conditions: Wi-Fi SSID, subnet (CIDR), or the default-gateway MAC (a precise, medium-agnostic network fingerprint that tells apart networks sharing a subnet); action: connect/disconnect. Rules are ordered by priority (drag-to-reorder, first match wins) and evaluated entirely in the helper via a hybrid trigger (macOS route-monitor subscription; 30 s poll on Windows/Linux). Replaces the legacy per-tunnel Wi-Fi auto-connect / trusted-SSID UI (migrated automatically). Editable in the GUI or via the CLI.
- **Command-line interface** `wireguide ctl` (issue #10) — a third IPC client alongside the GUI (Tailscale-style): `status`, `list`, `connect`, `disconnect`, `import`, `rename`, `delete`, and `automation add/rm/rules` + a read-only decision preview. No per-command sudo, cross-platform, shares the GUI's tunnel store.
- Tunnel-list **sorting** (name / last used / date added, active-on-top) and **compact mode** (issue #16, #17); **drag-resizable** tunnel-list column.

### Fixed
- **update:** the Ed25519 signature is now bound to the hash actually installed (a repo-write attacker could previously pass both checks by swapping SHA256SUMS between check and download); `Install` also enforces `SignatureVerified` in signed-update builds.
- **Windows:** `findInterfaceMTU` buffer overflow + wrong `NlMtu` offset (undefined behaviour on every no-MTU connect; auto-MTU always fell back).
- **Linux:** split-tunnel routes were deleted from the wrong table on the default `Table=auto` path (route leak); DNS search-domain injection; nft kill-switch endpoint-port validation and `oifname` consistency.
- **macOS:** `route -n monitor` subprocess is now supervised (was a silent zombie + stuck monitor on unexpected exit); the tray menu-bar icon uses native click-to-open (fixed the "does nothing on macOS 26" report, issue #18) and follows the menu bar's actual appearance; the connect/Disconnect-race no longer holds `Manager.mu` across slow teardown.
- **storage:** reject case-collisions and Windows reserved names; fsync the parent directory after atomic writes; latency-probe target validation; meta-sidecar lost-update race.
- **Automation (code review, issue #12):** `else`/none_match now matches at its own position so drag-to-reorder priority is uniform (was always held to the end); malformed conditions and unknown actions now fail closed (rule skipped) instead of an unknown action defaulting to connect; a rule-driven connect now runs the same DNS-protection + kill-switch folding as a manual connect (headless automation could previously connect with no protection, or fail entirely under an already-on kill switch), and a rule-driven disconnect strips the tunnel from the kill-switch filter set; macOS no longer overwrites the GUI-reported SSID with an empty root-helper poll (which silently broke SSID rules); Windows gateway-MAC resolves the physical underlay gateway (excluding the WireGuard adapter) so a full tunnel no longer blanks the fingerprint and flaps `mac:` rules; tunnel rename/delete now carry/drop the tunnel's automation rules instead of orphaning them; the rule editor no longer races a debounced save against a tunnel switch. *(Windows gateway change compiles but is unverified on a Windows build.)*
- **config.json:** cross-process read-modify-write is now atomic (file lock) so a `wireguide ctl` edit and a GUI edit can't clobber each other.
- **CLI (issue #10):** `import`/automation edits work on a fresh install (dirs created); `set` exits nonzero when the helper is running but the live apply fails; `delete` refuses to remove a still-connected tunnel whose disconnect failed; `install-skills` writes agent files atomically. The NSIS installer PATH edit no longer interpolates the install path into a PowerShell command (injection), and the macOS cask + Windows installer put `wireguide` on `PATH`.
- **list:** date-added sort now uses a stamped creation time (survives edits) instead of the `.conf` mtime (issue #17).

### Changed
- Latency probe no longer fabricates a `x.x.x.1` gateway target (issue #15); per-tunnel latency target added.

## [0.3.1] - 2026-05-26

### Added
- **Full-tunnel routing-loop protection (Windows + macOS)** — multi-layer defense against the encrypted-UDP-loops-through-tunnel-adapter class of bug (issue #14).
  - Windows: WFP block at `ALE_AUTH_CONNECT_V{4,6}` + `OUTBOUND_TRANSPORT_V{4,6}` layers, iphlpapi-based `/32` bypass host route with `InitializeIpForwardEntry`, `IP_UNICAST_IF` UDP socket binding with `NotifyRouteChange2`-pushed re-pin monitor, runaway-TX watchdog with sustained-asymmetry trip.
  - macOS: `/32` bypass installed before `/1` split routes with fail-fast preflight on missing default gateway, 5 s underlay-detection retry, blackhole fallback on gateway loss inside `reapply` to keep the loop class contained when the upstream gateway briefly disappears, runaway-TX watchdog via `netstat -ibnI`.
- **SignPath Foundation code signing** — CI hooks for SignPath OSS signing of the Windows installer; gated on the foundation's onboarding approval. Releases ship unsigned until then.

### Fixed
- Helper now exits within ~20 s of the GUI dying (was ~70 s) — IPC read deadline trimmed to 10 s now that the GUI's 5 s health-monitor ping cadence is the canonical liveness signal.
- macOS: `RestoreDNS` no longer fires a noisy `netsh`-equivalent against an adapter that's already been detached from the IP stack during disconnect.
- macOS: `getDefaultInterface()` now parses the `netstat -nr` header dynamically; previously the "first lowercase field" heuristic could misidentify `awdl0` (AirDrop) as the default interface on some machines.
- Windows: UAPI listener "may not work" warning downgraded to DEBUG on Windows — the named-pipe bind is expected to fail because the helper runs as an elevated user rather than as `LocalSystem`; status queries route through the in-process `Engine.IpcGet` regardless.

### Changed
- CI release notes generated by `git-cliff` (fuller diffs than the previous auto-generated body).
- CI: explicit NSIS install on Windows runners (the default Windows-latest image no longer carries `makensis` on PATH).
- CI: `Get-FileHash` / `Expand-Archive` in the wintun vendoring step replaced with direct .NET APIs to avoid PowerShell version skew on the runner.
- README: `Install` section moved above `Features`, code-signing dev-process notes trimmed to user-facing status only.

## [0.3.0] - 2026-05-25

### Added
- **Windows kill switch via WFP** — Windows Filtering Platform-based kill switch that survives helper restarts; complements the existing macOS `pf` and Linux `nftables` implementations.
- **Periodic auto-update scheduler** — background check for new releases on a configurable cadence (default 24 h with focus-opportunistic refresh), separate from the existing manual "Check for updates" path.
- **CI release pipeline** — automated darwin (arm64) + Windows (amd64/arm64) builds on tag push, with SHA256SUMS, Ed25519 signature, and `homebrew-tap` cask auto-bump.

### Fixed
- macOS kill switch: `pf` anchor renamed from `com.apple.wireguide` (dot) to `com.apple/wireguide` (slash) so it actually matches the `anchor "com.apple/*"` wildcard in the system `/etc/pf.conf` — previously the rules loaded without ever being evaluated.
- macOS kill switch can now be toggled on without an active tunnel (base block-all set installs cleanly; per-tunnel permits are folded in on subsequent connects).
- Windows disconnect: lingering wintun adapter "defanged" (DNS cleared, metric bumped) before `engine.Close`, so the brief window where Windows still treats the dying adapter as a viable metric-1 path doesn't dump every DNS query onto its dead `8.8.8.8` binding.
- Windows disconnect: dead 12 s DNS-restore call removed; `netsh` output now decoded as the OEM code page so Korean / non-English Windows installs no longer mis-parse error messages.
- Windows: UAPI bypass (status queries served by in-process `Engine.IpcGet` rather than the named pipe that the elevated helper can't bind under the kernel's owner-SID requirement).
- Windows: suicide-reconnect / orphaned `conhost` / dangling route fixes from the WFP kill-switch rework.
- DNS protection regression introduced during the periodic-update-scheduler refactor.
- Numerous race conditions, leak fixes, and audit findings from the cross-platform hardening pass.

### Changed
- Tray and taskbar icons: rounded silhouette via custom genicon (matches the macOS dock icon's visual weight).
- Sidebar dividers, tool pages, and drop affordance polished.
- Settings: maintainer credit added in footer; helper SIGTRAP fix.
- Rebrand: WireGuide red accent + Material-style flat buttons.

## [0.2.0] - 2026-05-05

### Added
- **Wi-Fi auto-connect rules** — per-tunnel SSID-based auto-connect/disconnect; rules fire in the helper so they work even when the GUI is quit
- **Trusted SSID support** — designated SSIDs auto-disconnect all VPN tunnels (home/office network detection)
- **macOS 14+ Location Services integration** — CoreWLAN CGo replaces `networksetup` for SSID detection; app now appears in System Settings → Location Services
- **GUI→Helper SSID forwarding** — on macOS 14+ the helper (root LaunchDaemon) cannot read SSID itself; the GUI polls via CoreWLAN and forwards changes over IPC so auto-connect rules fire correctly
- **Ed25519 signature verification** — auto-update downloads verified against a Ed25519 signature over SHA256SUMS; embedded public key prevents tampered binaries from being installed

### Fixed
- Wi-Fi auto-connect status not updating in GUI/tray after rule fires (`ActiveTunnels` now populated in all status broadcasts)
- `autoConnectedBy` accessed under wrong mutex in `handleRename` (race condition; changed to `wifiMu`)
- Lock ordering violation between `handleRename` and `handleSSIDChange` that could cause deadlock
- Kill switch and DNS protection handlers using `Status().State` instead of `IsConnected()` (broke in multi-tunnel setups where the primary was not the connected tunnel)
- `handleReportSSID` panic on nil `wifiMon` (non-darwin builds and pre-init race)
- `sleep_darwin.go` unsafe.Pointer misuse flagged by `go vet`; replaced with `runtime/cgo.Handle`
- Duplicate SSID appearing in Wi-Fi rules dropdown when current SSID matched a saved rule

### Changed
- Auto-connect logic moved to helper process (was frontend-side) so rules fire independently of GUI lifecycle
- `postConnectRefresh` refactored: `refreshTunnels`+`refreshStatus` kept for manual connect UX; auto-connect path calls only `applyFirewallSettings` (event stream handles status update)
- Dead backward-compat fallback in `subscribeToEvents` removed (active_tunnels now always populated)

## [0.1.9] - 2026-05-05

### Changed
- Removed Wi-Fi rules master toggle; trusted SSIDs are always active when configured

### Fixed
- Various regressions, lifecycle, and performance issues from audit rounds (Round 2, Round 3)
- 30+ fixes from full-codebase review (null guards, lock safety, error propagation)

## [0.1.8] - 2026-04-13

### Changed
- Sidebar navigation: removed Tools tab bar, DNS Leak Test and Route Table are now direct sidebar sub-items
- Settings modal: fixed size regardless of active tab (no more resize when switching to Advanced)
- Settings sidebar active state: tint highlight instead of solid blue (macOS HIG)
- Dropdown controls: custom styled per macOS HIG (28px height, 6px radius, theme-aware chevron)

### Improved
- Route table: sticky column header, legend pinned to bottom, table fills remaining space with scroll
- DNS Leak Test and Route Table now call real backend (previously stub implementations)
- macOS HIG design tokens: added `--border-strong` for input control borders

### Removed
- Network Diagnostics (Ping) tool — not meaningfully useful as a standalone feature
- Unused i18n keys for removed Diagnostics feature

## [0.1.7] - 2026-04-09

### Added
- Multiple simultaneous tunnel support
- Per-tunnel NetworkManager (independent routes, DNS, route monitor per tunnel)
- Per-tunnel health check and reconnection
- Full-tunnel conflict detection (reject two 0.0.0.0/0 configs)
- DNS union across all active tunnels
- No-handshake warning: orange dot in tunnel list, ◐ in tray menu
- Tray menu shows per-tunnel connection + handshake status
- Architecture & design documentation (docs/DESIGN.md)

### Fixed
- Disconnect one tunnel no longer breaks other active tunnels
- Conflict detection: macOS netstat abbreviated CIDRs now parsed correctly
- GUI not reflecting connection state when tunnel connected via system tray
- Bypass route race conditions (lock safety, error propagation)
- Tray icon padding: trimmed transparent pixels for tighter menu bar fit
- Tunnel list unnecessary re-renders on every status tick
- README streamlined: removed defensive tone, screenshots moved to top

### Changed
- Pin Interface toggle added (Settings > Advanced) for dual-network stability
- Bypass routes pinned to upstream interface with -ifscope when enabled

## [0.1.6] - 2026-04-08

### Added
- Settings redesign: split layout with sidebar (General / Advanced / About)
- About tab: app icon, version, GitHub/Issues/License links, update status
- Update popup: modal with release notes ("What's New") and "Skip This Version"
- Helper auto-upgrade: detects version mismatch and reinstalls on app update
- Helper install retry dialog with Quit/Retry options on cancel
- OpenURL Wails binding (restricted to github.com)
- Tests for IsBrewInstall and OpenURL validation (7 new tests)

### Fixed
- Brew install detection: check Caskroom receipt instead of binary path
- Non-brew update: opens GitHub Releases page instead of broken auto-download
- Brew update: runs `brew update` before `brew upgrade` for third-party taps
- Helper Ping response: separate AppVersion field (fixes IPC protocol validation)
- Update popup double-click guard
- localStorage exception handling for skip version
- Detailed admin prompt explaining why password is needed

### Changed
- README/About description: "native macOS" → "cross-platform"

## [0.1.5] - 2026-04-07

### Added
- Health Check toggle in Settings (default: off, recommended with PersistentKeepalive)

### Changed
- Health Check default changed from on to off (consistent with other WG clients)
- README rewritten: removed aggressive tone, verified claims, acknowledged official app works for many users

## [0.1.4] - 2026-04-07

### Security
- Remove script execution (PreUp/PostUp/PreDown/PostDown) — eliminates local privilege escalation via ApproveScripts RPC
- Fix Windows IPC ACL: allow non-admin GUI to connect to helper pipe
- Harden update integrity: asset size validation + Content-Length check

### Fixed
- Kill switch pf rules: use anchor-only approach instead of modifying main ruleset (fixes Tahoe compatibility)
- Kill switch + DNS protection now toggleable while VPN is connected
- Kill switch reconnect deadlock: suspend/resume firewall rules during reconnect
- Log viewer scroll not working
- Tunnel list scroll overflow

### Added
- Handshake-based health check: detects dead tunnels and triggers reconnect after 180s
- Instant sleep/wake detection via NSWorkspace notification (polling fallback kept)
- Typed tunnel error enums (ErrAlreadyConnected, ErrNetwork, etc.)
- DNS post-write verification
- Crash recovery journal with pre-modification DNS snapshot
- Comprehensive unit tests (102 tests, race-clean)
- CHANGELOG.md
- Info-level logs for kill switch and DNS protection events

## [0.1.3] - 2026-04-07

### Fixed
- "Show Window" not working after closing the window (RegisterHook instead of OnWindowEvent)
- Dock icon hide/show when window is closed/reopened
- App icon showing Wails default (white W) instead of WireGuide red icon
- About/Settings dialog showing wrong version — now fetched dynamically from Go

### Added
- GitHub issue templates (bug report, feature request)
- CONTRIBUTING.md and PR template

## [0.1.2] - 2026-04-07

### Fixed
- Dock icon not hiding when window is closed
- Tunnel list not updating after rename

## [0.1.1] - 2026-04-06

### Fixed
- Daemon socket directory permissions (0700 → 0755)
- LaunchDaemon install flow rewrite (app first-launch, not cask postflight)

### Added
- Version display in Settings

## [0.1.0] - 2026-04-05

### Added
- Initial release
- WireGuard tunnel management (import, create, edit, export .conf files)
- Config editor with CodeMirror 6 syntax highlighting and autocompletion
- System tray with connection status badge
- Kill switch via macOS pf
- DNS protection (force DNS through VPN tunnel only)
- Auto-reconnect with exponential backoff
- Sleep/wake recovery
- Route monitor for gateway changes
- Conflict detection (Tailscale, other WG interfaces)
- Network diagnostics (ping, DNS leak test, route table)
- Auto-update (GitHub Releases + Homebrew)
- Real-time RX/TX speed graph
- i18n (English, Korean, Japanese)
- Dark / Light / System theme
