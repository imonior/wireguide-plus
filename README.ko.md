# WireGuide Plus

**멀티 터널과 네트워크 인식 자동화를 갖춘 WireGuard 클라이언트**

WireGuide Plus는 오픈소스 프로젝트 [`korjwl1/wireguide`](https://github.com/korjwl1/wireguide)를
**깊이 있게 수정·강화한** 포크입니다. 두 가지 핵심 기능:

- **멀티 터널 동시 연결** — 여러 WireGuard 터널을 동시에 연결해 서로 간섭 없이
  독립적으로 실행할 수 있습니다.
- **조건 기반 자동 연결** — Wi-Fi SSID, 시간대, 시스템 시작 등의 조건에 따라
  알맞은 터널을 자동으로 연결합니다 (예: 사무실 Wi-Fi에서는 터널 A, 집에서는 터널 B).

[English](README.md) | [简体中文](README.zh.md) | [繁體中文](README.zh-TW.md) | **한국어** | [日本語](README.ja.md)

> **Windows 10 / 11(x64, x86 32비트 및 ARM64)과 macOS(Apple Silicon / arm64)를 완전히 지원**합니다 —
> macOS는 Apple Silicon 실기기에서 충분히 검증되었습니다. Linux(x64, arm64)는 **실험적
> 프리뷰**로 제공됩니다 — CI에서 빌드되지만 아직 실기기에서 테스트되지 않았습니다
> ([플랫폼 지원](#플랫폼-지원) 참조).
> **Android / iOS는 지원하지 않습니다.**

## 주요 기능

- **멀티 터널 동시 연결** — 업스트림의 「한 번에 한 터널만」과 달리 여러 터널을
  병렬로 실행할 수 있어, 사내망과 외부망을 동시에 접근할 수 있습니다.
- **조건 기반 자동 연결** — Wi-Fi SSID / 시간대 / 시스템 시작 등의 조건으로 터널을
  자동으로 연결·해제하며, 규칙은 우선순위와 상호 배제를 지원합니다.
- **자동 재연결** — 터널이 예기치 않게 끊기면 자동으로 복구되며, 연결 상태를
  실시간으로 확인할 수 있습니다.
- **화면 꺼짐·잠금·절전 중에도 연결 유지** — 설정(기본 켜짐, 터널별 재정의 가능)으로
  화면 꺼짐, 화면 보호기, 잠금 상태에서도 터널을 유지하고, 절전에서 깨어나면 자동으로
  다시 연결합니다.
- **로그인 시 자동 시작** — 로그인 후 WireGuide Plus를 자동으로 실행하고 규칙에 따라
  연결합니다 (「최소화 시작」과 함께 쓰면 창이 시작 직후 접힌 상태로 실행됩니다).
- **최소화 시작** — Windows에서는 시작 시 작업 표시줄로 최소화됩니다(작업 표시줄
  아이콘이 유지되므로 메인 창은 언제든 다시 열 수 있습니다). macOS/Linux에서는 시스템
  트레이로 최소화됩니다.
- **트레이 연결 상태 알림** — 시작 후 15초(권한 상승 확인 후)에 모든 터널의 개요 버블을 표시하여
  터널별 실시간 상태(녹색 체크표시 / 빨간 X / 연결 중 노란 점)와 자동/수동 모드를 알려주며, 네트워크 변화(Wi-Fi 전환, 랜 케이블 분리, 인터넷 끊김 등)로 터널 상태가
  바뀌면 10초 후 안정된 최신 상태를 표시합니다. 알림에는 작업 메뉴(메인 창 열기 /
  연결 끊기)가 있고, 수동으로 닫거나 설정한 시간(기본 10초, 설정에서 조절 가능) 후
  자동으로 닫힙니다. macOS와 Linux도 같은 풍선을 표시합니다(이전에는 Windows만).
- **터널 관리** — `.conf` 가져오기 / 내보내기, 연결 기록, 빠른 켜기/끄기.
- **AmneziaWG(AWG) 터널** — AmneziaWG(난독화 WireGuard) 설정 가져오기 및 연결 지원. AWG는 설정의 Jc/Jmin/Jmax/S1-S4/H1-H4 난독화 매개변수로 자동 감지되며, 해당 터널에는 「AmneziaWG」 배지가 표시됩니다. 설정 → 고급에서 지원을 끌 수 있습니다.
- **터널 편집기: 필드 뷰와 스크립트 훅** — conf 텍스트 외에 키별 폼(interface / peer 그룹)으로 구성을 편집하고, PreUp / PostUp / PreDown / PostDown 스크립트 훅을 관리(파일 선택, 빈 스크립트 생성, 코드 편집, 지우기)할 수 있습니다. 연결 중에도 편집할 수 있으며, 저장하면 끊고 변경을 적용한 뒤 자동으로 다시 연결합니다.
- **터널별 물리적 출구 바인딩** — 「인터페이스 고정」 활성화 시 터널마다 암호화 트래픽을 특정 물리 NIC에 고정(Windows / Linux / macOS)할 수 있으며, 바인딩된 NIC가 사라지면 대화상자에서 대기 / 자동 전환 / 수동 지정을 선택합니다.
- **설정 내보내기/가져오기** — tunnels, scripts, `config.json`(로그 제외)을 하나의 아카이브로 묶어 다른 머신으로 쉽게 이전할 수 있습니다.
- **도구 탭** — 내장 **DNS 누수 테스트**로 트래픽이 실제로 설정한 DNS 서버를 통해 나가는지 확인하고, **경로 시각화**는 현재 라우팅 테이블을 보여주며 각 경로에 VPN / Direct 배지를 달고 LAN 온링크 필터와 셀룰러 인터페이스 표기를 지원합니다.
- **터널 정책** — 터널별 DNS 확인 경로, 특정 도메인의 터널 강제 통과, 트래픽 보호, 기본 DNS, 그리고 결정적 충돌 해결.
- **지연 프로브** — 터널별 지연 프로브가 다중 대상 병렬을 지원합니다. 각 후보(공용 리졸버 8.8.8.8 / 223.5.5.5, 터널 엔드포인트, 임의의 사용자 지정 주소 또는 도메인)를 동시에 프로브하고 해석 IP·종류·왕복 시간을 표시합니다. 분할 터널에서는 사용자 지정 대상이 저장 전 터널의 AllowedIPs 커버리지로 검증됩니다. 대상은 저장 후 즉시 다시 프로브되며, 도메인 엔드포인트는 상세 페이지 상단에 현재 해석된 주소를 표시합니다.

## 업스트림 wireguide 대비 수정·개선 사항

### 수정

1. **Wi-Fi 네트워크 출구 지원(가장 중요한 수정)** — 업스트림은 Windows에서 **유선**
   인터페이스로만 트래픽을 내보내 Wi-Fi 환경에서는 출구가 사용 불가했습니다. 이
   에디션은 기본 출구 인터페이스 선택을 수정해 Wi-Fi에서도 무선 어댑터로 트래픽이
   정상적으로 나가게 했습니다.
2. **GUI 테마 표시 오류** — 다크/라이트 테마 전환 시 렌더링이 깨지던 문제를 수정했습니다.
3. **Windows 버전 리소스 표준화** — exe 속성 「세부 정보」의 버전 정보가 비어 있던
   문제를 수정했습니다 (`goversioninfo`로 생성).
4. **안정성 수정** — 업데이트 확인 스케줄 중복 제거, 더 정확한 물리 어댑터 감지 등
   (자세한 내용은 [CHANGELOG](CHANGELOG.ko.md) 참조).

### 개선

1. **SSID 드롭다운** — 자동 연결 규칙에서 시스템에 **저장된 모든 Wi-Fi SSID**를
   드롭다운으로 선택할 수 있어 오타가 없습니다.
2. **프록시 통한 업데이트 확인** — 업데이트 확인 전에 HTTP(S) 프록시를 설정할 수 있어,
   GitHub 접속 불가/제한으로 인한 업데이트 실패를 해결합니다.
3. **다국어 UI** — 简体中文 / English / 日本語 / 한국어 / 繁體中文.
4. **시스템 통합** — 로그인 시 자동 시작, 트레이 최소화 시작, 트레이 연결 상태 알림
   (시작 후 15초 전체 터널 개요 / 네트워크 변화 후 10초 지연 표시, 기본 10초, 조절 가능).
5. **창 제목 및 상호작용 개선** 등.
6. **AmneziaWG(AWG) 프로토콜 지원** — DPI 탐지를 회피하는 난독화 터널 AmneziaWG용 amneziawg-go 기반 프로토콜 백엔드 추가. 구성 자동 감지, UI 배지 표시, 설정 → 고급에서 지원 끄기 가능.

## 자동화 규칙

자동화는 **터널 단위**로 독립 설정됩니다 (아무 터널의 `…` 메뉴 →「자동화…」). 각 터널이 전용 규칙 집합을 갖고 있으므로, "사무실 Wi-Fi에선 회사 VPN 연결, 같은 Wi-Fi에선 개인 VPN 끊기, 집에선 개인 VPN 연결" 같은 조합이 서로 간섭하지 않고 공존합니다.

편집기 상단의 실시간 네트워크 패널은 **현재 터널**의 환경만 표시합니다. 실제 하드웨어 인터페이스(`in use` / `not in use` 표시), Wi-Fi SSID, 게이트웨이 MAC, 게이트웨이 IP, 서브넷을 항목별 한 줄로 보여 주며, 이 터널의 어떤 조건이 해당 값을 매치했는지도 표시합니다. 가상 어댑터는 제외되고 Wi-Fi 미연결 시 SSID는 "Wi-Fi 연결 안 됨"으로 표시됩니다. 패널과 규칙 안내는 접을 수 있으며 편집기 전체를 스크롤할 수 있습니다.

「인터페이스 고정」은 이제 모든 데스크톱 플랫폼을 지원합니다: Windows는 `IP_UNICAST_IF`로 터널의 UDP 소켓을 지정 NIC에 바인딩하고, Linux는 `ip route … dev <iface>` 바이패스 라우트를, macOS는 `-ifscope` 스코프 바이패스 라우트를 사용합니다. 물리적 출구 지정은 운영체제 라우팅 정책(WireGuard 프로토콜 설정이 아님)에 속하며, 바인딩된 NIC가 사라지면 대화상자에서 바인딩 유지 대기 / 자동 전환 / 수동 지정을 선택할 수 있습니다.

### 규칙 로직

- 한 터널의 규칙은 **하나의 정렬된 목록**이며 위에서 아래로 평가됩니다 — **규칙 위치가 곧 우선순위**입니다. 연결 규칙과 연결 해제 규칙은 목록 전체에서 자유롭게 섞일 수 있고 드래그로 재정렬할 수 있습니다.
- **첫 번째 매치가 승리 (규칙 안은 AND)**: 한 규칙 안의 모든 조건이 다 성립해야 그 규칙이 발동하지만, 목록 전체에서 **가장 먼저 매치된 규칙 딱 하나**만 실제 동작을 실행합니다. 매치된 규칙 뒤에 있는 규칙들은 "매치는 됐으나 우선순위에서 밀려 무시(deprioritized)"로 표시되며 실행되지 않으므로, 같은 SSID에서 동시에 연결 해제 / 연결이 실행되는 모순은 생기지 않습니다.
- **기본 상태 수렴**: **어떤 규칙도 일치하지 않으면** 터널은 **기본 상태**(연결 또는 연결 해제 — 자동화 에디터 상단에서 선택)로 수렴합니다. 규칙도 기본 상태도 없는 터널에는 자동화가 개입하지 않습니다. 기본 상태만 설정한 경우(규칙 0개) 터널은 항상 해당 기본 상태로 수렴합니다.
- **편집 중 라이브 매치 인디케이터**: 자동화 에디터를 열어 두면 각 조건이 현재 네트워크와 일치하는지 그 자리에서 보여주며, 실제로 유효한 첫 매치 규칙은「in use (사용 중)」으로 강조 표시됩니다. 맨 위 판정 바에는 해당 터널의 최종 결정이 표시됩니다. 편집이 일어날 때마다 인디케이터는 즉시 재평가되며 (약 250ms 디바운스로 IPC 호출), 백그라운드 helper 가 실제 규칙을 집행하는 엔진과 **정확히 같은** 엔진을 쓰므로 UI 표시와 실제 동작은 언제나 일치합니다. 같은 엔진은 커맨드라인 (`wireguideplus automation`)에서도 이용 가능해, GUI 없는 환경이나 디버깅에 편리합니다.

### 수동 오버라이드

터널 상세 보기의 **연결** / **연결 해제** 버튼은 수동 제어입니다. 수동 연결은 **연결을 강제하며 모든 자동화 규칙을 무시**하고, 수동 연결 해제는 **연결 해제를 강제하며 모든 자동화 규칙을 무시**합니다 —— 다음 자동화가 네트워크를 재평가하기 전까지. UI에는 래치 인디케이터가 있어 현재 터널 동작이 수동 오버라이드인지 자동화인지 표시하므로, 무엇이 연결을 실제로 주도하는지 항상 알 수 있습니다.

### 자동화에서 터널 제외

수동 오버라이드는 앱을 재시작할 때까지 유지됩니다. 자동화가 **절대** 터널을 동작시키지 말아야 하는 경우——전형적으로 동일한 터널을 다른 WireGuard 클라이언트도 관리할 때——터널의 자동화 편집기에서 "자동화"를 끕니다. 터널은 규칙과 기본 상태를 유지하지만, 엔진은 연결도 끊기도 하지 않습니다.

이 설정은 터널 상세 보기 상단의 원클릭 "자동화 중지" 토글에서도 할 수 있어, 터널을 보는 그 자리에서 자동화를 억제할 수 있습니다. 전역 "시스템 절전 방지" 설정(기본값 꺼짐)은 터널이 연결된 동안에만 컴퓨터를 깨운 상태로 유지하며(Windows·macOS·Linux 모두 지원), 마지막 터널이 끊기면 즉시 해제되어 유휴/절전 전원 정책이 터널을 조용히 끊지 못하게 합니다. 다른 클라이언트가 이미 터널 주소를 점유한 경우, WireGuide Plus는 충돌 소프트웨어를 명시하고 해당 터널의 자동화 상태를 표시하며, "해당 터널 자동화 중지"와 "계속 시도" 두 가지 선택지만 제공하는 상시 대화 상자를 표시합니다. 당신이 결정할 때까지 해당 터널의 자동 연결은 보류되며, 미해결 충돌은 GUI 시작 및 helper 재시작 후 다시 표시됩니다——결정은 당신이, 다른 클라이언트를 강제 중지하지 않습니다.

### 터널 하나에 클라이언트 하나

두 WireGuard 클라이언트가 같은 터널을 동시에 실행할 수 없습니다. 터널의 `Address` 는 먼저 도달한 어댑터가 선점하며, 나중에 연결하는 클라이언트는 주소 할당 단계에서 실패합니다(Windows는 `The object already exists.` 를 보고). 공식 WireGuard 클라이언트도 함께 쓰는 경우, 동일 터널의 서비스를 먼저 중지하세요(예: `Stop-Service 'WireGuardTunnel$<터널 이름>'`). 또는 해당 터널을 자동화에서 제외하세요(위 참조). WireGuide Plus는 이 충돌을 감지하고, 충돌하는 어댑터 이름을 상시 충돌 대화 상자와 helper 로그 양쪽에 남기며, 실패한 자동화 연결을 백오프로 재시도합니다(30초 → 1분 → 2분 → 5분, 매 폴링마다 어댑터를 반복해서 들이박지 않음).

### 조건 타입

| 조건 | 매치 기준 | 전형적인 사용 예 |
| --- | --- | --- |
| **SSID** | 현재 Wi-Fi SSID 이름과 **바이트 단위로 완전 일치** (대소문자 구분, 공백·특수문자 모두 비교. 802.11 정의에 따름). | "`사무실 5GHz`에 연결되면 회사 VPN 자동 연결". |
| **서브넷 (Subnet)** | 현재 물리 NIC IP 가 지정된 CIDR (예: `192.168.178.0/24`) 범위 안에 있는지. | 집/사무실 LAN 대역은 예측 가능하지만 SSID는 바뀔 수 있을 때. |
| **게이트웨이 MAC** | 현재 기본 게이트웨이(라우터)의 MAC 주소 — SSID나 서브넷이 흔한 이름이라도 특정 네트워크를 식별합니다. | "카페 라우터에서는 **절대로** 자동 연결하지 않기". |
| **게이트웨이 IP** | 현재 물리 네트워크의 기본 게이트웨이 IP. | 건물 전체가 같은 SSID지만 층마다 게이트웨이 IP가 다를 때. |
| **인터페이스 (Interface)** | 현재 상향 경로로 쓰이는 네트워크 어댑터 이름. 드롭다운에는 본체의 **모든** 네트워크 어댑터——유선·무선 구분 없이, **미연결 어댑터도**——가 모두 나오므로, 아직 꽂지 않은 독 / USB 랜 / 썬더볼트 어댑터 / Wi-Fi 카드용으로 미리 규칙을 써 둘 수 있음. | "사무실 독에 연결한 유선 어댑터를 통해 돌 때만 회사 VPN 연결". |
| **시간 대 (Time window)** | 요일 집합 + 시작/끝 시각 (로컬 시계). | "월~금 09:00–18:00 사이 회사 VPN은 항상 켜둠". |

한 규칙 안에 위 조건들을 자유롭게 조합할 수 있습니다: 예를 들어 "SSID = 사무실 AND 시간 대 = 월~금 09–18" 이면 AND 조건 2개짜리 단일 규칙입니다. 각 터널의 disconnect / connect 양쪽 그룹에, AND 조건으로 이뤄진 규칙을 원하는 개수만큼 등록 가능합니다.

## 플랫폼 지원

| 플랫폼 | 상태 |
| --- | --- |
| Windows 10 / 11(x64, x86 32비트, ARM64) | ✅ 완전 지원 (멀티 터널 동시 연결 + SSID 자동 연결, AmneziaWG 포함) |
| macOS(Apple Silicon / arm64) | ✅ 완전 지원 — Apple Silicon 실기기에서 충분히 검증됨 |
| Linux(x64, arm64) | 🚧 실험적 — CI에서 빌드되지만 아직 실기기에서 테스트되지 않음 |
| Android / iOS | ❌ **지원 안 함** (터널 동시 실행 불가, Wi-Fi SSID 자동 전환 불가) |

### 모바일 버전이 없는 이유

이 프로젝트의 핵심은 **멀티 터널 동시 연결**과 **조건 기반 자동 연결(예: Wi-Fi SSID)** 입니다.
Android / iOS에서는 시스템 커널과 권한 체계 때문에 WireGuard 구현이 **여러 터널을
동시에 실행**하거나 **Wi-Fi SSID에 따라 터널을 자동으로 전환**할 수 없습니다. 두 핵심
목표 모두 모바일에서 달성할 수 없으므로, 이 프로젝트는 **모바일을 명시적으로
지원하지 않습니다.** 모바일 단일 터널 용도는 공식 WireGuard 앱의 On-Demand 기능을
사용하세요.

## 다운로드 & 설치

각 릴리스에서는 지원하는 플랫폼별로 **설치 프로그램**(권장)과 **포터블 버전**을 배포합니다 — 사용 중인 OS를 아래에서 선택하세요. macOS는 `.dmg`/`.zip`, Linux는 `.deb`/`.tar.gz`, Windows는 아래 설치 프로그램/포터블 버전입니다. 모든 릴리스에는 Ed25519 서명된 `SHA256SUMS`(및 `SHA256SUMS.sig`)도 첨부되며, 앱 내 업데이터가 업데이트 적용 전에 검증합니다.

### Windows

**설치 프로그램(권장)**

- Windows x64 설치 프로그램: `wireguideplus-<version>-amd64-installer.exe`
- Windows x86(32비트) 설치 프로그램: `wireguideplus-<version>-x86-installer.exe`
- Windows ARM64 설치 프로그램: `wireguideplus-<version>-arm64-installer.exe`

설치 프로그램 파일 이름에는 버전과 아키텍처가 포함됩니다(`wireguideplus-<version>-<arch>-installer.exe`,
arch는 `x86` / `amd64` / `arm64`). 설치된 프로그램 파일 이름에도 아키텍처가 붙습니다
(`wireguideplus-<arch>.exe` — 파일 속성 → 자세히에서도 확인 가능). 64비트 설치
프로그램은 기본적으로 `C:\Program Files\WireGuide Plus`에, 32비트 설치 프로그램은
`C:\Program Files (x86)\WireGuide Plus`에 설치됩니다(32비트 시스템에서는
`C:\Program Files\WireGuide Plus`). 설치 중에 설치 폴더를 변경할 수 있습니다. 시작
메뉴 바로 가기(「WireGuide Plus 제거」 항목 포함, 기본 생성, 선택 해제 가능)와
바탕화면 바로 가기(항상 생성)가 등록됩니다. 설치 프로그램에는 필요한 모든 파일이
포함되어 있어 추가 다운로드가 필요 없습니다.

**포터블 버전(설치 불필요)**

- `wireguideplus-amd64.exe` **+ `wintun-amd64.dll`** (32비트 exe는 **`wintun-x86.dll`**,
  ARM64 exe는 **`wintun-arm64.dll`**) — **같은 아키텍처**의 두 파일을 함께 다운로드해
  같은 폴더에 넣은 뒤 exe를 실행하세요.

포터블 버전은 **단독으로 실행되지 않습니다**. WireGuard 터널을 만드는 데 필요한
드라이버 DLL을 exe와 같은 폴더에 두어야 합니다. 프로그램은 아키텍처에 맞는 파일을
자동으로 로드합니다(`wintun-amd64.dll` / `wintun-x86.dll` / `wintun-arm64.dll`) —
**이름을 바꿀 필요 없이** 아래 표대로 두면 됩니다:

| exe | 일치하는 드라이버 DLL |
| --- | --- |
| `wireguideplus-amd64.exe`(64비트) | `wintun-amd64.dll` |
| `wireguideplus-x86.exe`(32비트) | `wintun-x86.dll` |
| `wireguideplus-arm64.exe`(ARM64) | `wintun-arm64.dll` |

드라이버 DLL은 `wintun-0.14.1.zip`에 들어 있습니다(
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md#42-wintun-driver-dll) 참조). 릴리스에는
`wireguideplus-<version>-amd64-portable.zip` / `wireguideplus-<version>-x86-portable.zip` /
`wireguideplus-<version>-arm64-portable.zip` 포터블 zip도 제공됩니다. 각 zip에는 exe와 일치하는
드라이버 DLL이 **함께** 들어 있어 압축을 풀기만 하면 실행할 수 있습니다. 릴리스에서
더 이상 개별 DLL을 첨부하지 않습니다(포터블 zip 또는 설치 프로그램을 사용하세요).
일치하는 드라이버 DLL이 exe 옆에 없으면 터널을 만들 수 없습니다.



### macOS（Apple Silicon）

릴리스마다 두 가지 아티팩트를 제공합니다：

- `WireGuidePlus-<version>-darwin-arm64.dmg` — 응용 프로그램 폴더로 드래그하는 설치 프로그램.
- `WireGuidePlus-<version>-darwin-arm64.zip` — 포터블 `.app` 번들.

`.dmg`를 열고 **WireGuide Plus**를「응용 프로그램」으로 드래그한 뒤 Spotlight 또는 Launchpad에서 실행합니다. 포터블 `.zip`은 압축을 풀면 `wireguideplus.app`이 되며 바로 실행할 수 있습니다.

참고：

- **Apple Silicon 전용.** CI는 `arm64` 단일 아티팩트만 빌드합니다. Intel Mac은[소스에서 빌드](docs/DEVELOPMENT.md)해야 합니다.
- **Ad-hoc 서명만 해당(공증 없음).** 첫 실행 시 macOS Gatekeeper가 경고합니다. 우클릭 → **열기**를 선택하거나, 다음 명령으로 격리 속성을 한 번 제거하세요：
  ```sh
  xattr -dr com.apple.quarantine /Applications/wireguideplus.app
  ```
- arm64 빌드용 Homebrew cask도 제공합니다 — `brew install --cask wireguideplus`도 동일한 `WireGuidePlus-<version>-darwin-arm64.zip`을 받습니다.
- **위치 서비스 권한 (SSID별 자동화용).** SSID별 자동 연결은 현재 Wi-Fi SSID를 읽는데, macOS에서는 **위치 서비스** 권한이 필요합니다. **시스템 설정 → 개인정보 보호 및 보안 → 위치 서비스**에서 허용하세요(먼저 마스터 스위치를 켜고, 그다음 WireGuide Plus를 허용). 없으면 SSID 조건을 평가할 수 없어 SSID 기반 규칙이 발동하지 않습니다.

### Linux（실험적）

각 아키텍처(`amd64`, `arm64`)마다 두 가지 아티팩트를 제공합니다：

- `WireGuidePlus-<version>-linux-<arch>.deb` — Debian / Ubuntu 설치 패키지.
- `WireGuidePlus-<version>-linux-<arch>-portable.tar.gz` — 포터블 바이너리.

`.deb`는 패키지 관리자로 설치합니다(예： `sudo apt install ./WireGuidePlus-<version>-linux-amd64.deb`). 포터블 tarball은 압축을 풀고 `./wireguideplus`를 실행합니다. 포터블 버전에는 GTK3 / WebKitGTK 런타임이 필요하며(`.deb`이 자동 설치), 최소 구성 시스템에서는 먼저 설치하세요：

```sh
sudo apt-get install -y libgtk-3-0 libwebkit2gtk-4.1-0 libayatana-appindicator3-1
```

> Linux 빌드는**실험적**입니다 — CI 빌드만 되어 실기 테스트는 아직 미완료이며, 데스크톱 세션이 필요합니다(헤드리스 서버 미지원).

## 코드 서명

코드 서명은 플랫폼마다 다릅니다. **Windows**에서는 게시되는 모든 **설치 프로그램**에 Authenticode 서명( SignPath 서명이 활성화된 경우)이 적용되어 **무결성**(서명 이후 변조되지 않음)을 검증할 수 있고, 최초 실행 시 SmartScreen 경고도 줄어듭니다. **macOS**에서는 `.app`이 **Ad-hoc 서명만**(Apple 개발자 서명 없음) 되어 있어 첫 실행 시 Gatekeeper가 경고합니다(macOS 설치 안내 참조). **Linux**의 `.deb` 및 포터블 버전은 **서명되지 않았습니다**.

서명이 증명하는 것은 **누가 서명했는지**이며, 그 자체로 **설치 프로그램이 어떻게 빌드
되었는지**까지 증명하지는 않습니다. 빌드 출처, 승인 워크플로, 계정 보안, 재현성, 그리고
릴리스마다 제공되는 SHA-256 체크섬은 [SIGNING-POLICY.md](SIGNING-POLICY.md)에
정리되어 있습니다.

참고: 모든 플랫폼 중 코드 서명이 있는 것은 Windows 설치 프로그램뿐입니다. macOS의 `.app`은 Ad-hoc 서명, Linux 아티팩트는 서명되지 않았으므로, 각 릴리스에 첨부된 Ed25519 서명 `SHA256SUMS`가 범용 무결성 검증 수단입니다.

> Free code signing provided by [SignPath.io](https://signpath.io), certificate by
> [SignPath Foundation](https://signpath.org).

## 빌드 & 개발

빌드 환경 요구 사항, 개발/배포 빌드 명령(x86 + amd64 + arm64 멀티 아키텍처 빌드 포함),
NSIS 설치 프로그램 설명, 버전 리소스 및 릴리스 워크플로는 개발 문서
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)에 정리되어 있습니다. 릴리스는 버전 태그를
로컬에서 푸시하기만 하면 GitHub Actions 파이프라인이 빌드·서명·배포까지 자동으로
처리합니다 ([docs/release.md](docs/release.md) 참조).

## 데이터 & 로그

| 플랫폼 | 항목 | 위치 |
| --- | --- | --- |
| Windows | 설정 / 기록 | `%APPDATA%\wireguideplus\` (`config.json`, `history.json`) |
| Windows | 터널 설정 | `%APPDATA%\wireguideplus\tunnels\*.conf` |
| Windows | 터널 스크립트 | `%APPDATA%\wireguideplus\scripts\` |
| Windows | 로그 | `%APPDATA%\wireguideplus\logs\` |
| macOS | 설정 / 기록 | `~/Library/Application Support/wireguideplus/` |
| macOS | 터널 설정 | `~/Library/Application Support/wireguideplus/tunnels/*.conf` |
| macOS | 터널 스크립트 | `~/Library/Application Support/wireguideplus/scripts/` |
| macOS | 로그 | `~/Library/Logs/wireguideplus/` |
| Linux | 설정 / 기록 | `~/.config/wireguideplus/` (`$XDG_CONFIG_HOME/wireguideplus/`) |
| Linux | 터널 설정 | `~/.config/wireguideplus/tunnels/*.conf` |
| Linux | 터널 스크립트 | `~/.config/wireguideplus/scripts/` |
| Linux | 로그 | `~/.local/share/wireguideplus/` (`$XDG_DATA_HOME/wireguideplus/`) |

## 제거

- **Windows** — **제어판 → 프로그램 및 기능 → WireGuide Plus**에서 제거하거나, 설치 폴더의 제거 프로그램을 실행하세요.
- **macOS** — **WireGuide Plus**를「응용 프로그램」에서 휴지통으로 드래그합니다. 필요하면 `~/Library/Application Support/wireguideplus`와 `~/Library/Preferences/com.imonior.wireguide-plus.plist`도 삭제하세요.
- **Linux** — `sudo apt remove wireguideplus`(`.deb`), 또는 포터블 바이너리와 `~/.config/wireguideplus`를 삭제하세요.

## 감사의 말

- [korjwl1/wireguide](https://github.com/korjwl1/wireguide) — 업스트림 오픈소스 프로젝트
- [WireGuard](https://www.wireguard.com/) / [wireguard-go](https://git.zx2c4.com/wireguard-go)
- [Wails](https://wails.io)
