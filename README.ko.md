# WireGuide Plus

**Windows용 멀티 터널 · 자동화 중심 WireGuard 클라이언트**

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
- **로그인 시 자동 시작** — 로그인 후 WireGuide Plus를 자동으로 실행하고 규칙에 따라
  연결합니다 (「최소화 시작」과 함께 쓰면 창이 시작 직후 접힌 상태로 실행됩니다).
- **최소화 시작** — Windows에서는 시작 시 작업 표시줄로 최소화됩니다(작업 표시줄
  아이콘이 유지되므로 메인 창은 언제든 다시 열 수 있습니다). macOS/Linux에서는 시스템
  트레이로 최소화됩니다.
- **트레이 연결 상태 알림** — 시작 후 10초(권한 상승 확인 후)에 현재 연결 상태를
  알려주며, 네트워크 변화(Wi-Fi 전환, 랜 케이블 분리, 인터넷 끊김 등)로 터널 상태가
  바뀌면 10초 후 안정된 최신 상태를 표시합니다. 알림에는 작업 메뉴(메인 창 열기 /
  연결 끊기)가 있고, 수동으로 닫거나 설정한 시간(기본 10초, 설정에서 조절 가능) 후
  자동으로 닫힙니다.
- **터널 관리** — `.conf` 가져오기 / 내보내기, 연결 기록, 빠른 켜기/끄기.
- **AmneziaWG(AWG) 터널** — AmneziaWG(난독화 WireGuard) 설정 가져오기 및 연결 지원. AWG는 설정의 Jc/Jmin/Jmax/S1-S4/H1-H4 난독화 매개변수로 자동 감지되며, 해당 터널에는 「AmneziaWG」 배지가 표시됩니다. 설정 → 고급에서 지원을 끌 수 있습니다.

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
   (시작 후 10초 / 네트워크 변화 후 10초 지연 표시, 기본 10초, 조절 가능).
5. **창 제목 및 상호작용 개선** 등.
6. **AmneziaWG(AWG) 프로토콜 지원** — DPI 탐지를 회피하는 난독화 터널 AmneziaWG용 amneziawg-go 기반 프로토콜 백엔드 추가. 구성 자동 감지, UI 배지 표시, 설정 → 고급에서 지원 끄기 가능.

## 자동화 규칙

자동화는 **터널 단위**로 독립 설정됩니다 (아무 터널의 `…` 메뉴 →「자동화…」). 각 터널이 전용 규칙 집합을 갖고 있으므로, "사무실 Wi-Fi에선 회사 VPN 연결, 같은 Wi-Fi에선 개인 VPN 끊기, 집에선 개인 VPN 연결" 같은 조합이 서로 간섭하지 않고 공존합니다.

편집기 상단의 실시간 네트워크 패널은 **현재 터널**의 환경만 표시합니다. 실제 하드웨어 인터페이스(`in use` / `not in use` 표시), Wi-Fi SSID, 게이트웨이 MAC, 게이트웨이 IP, 서브넷을 항목별 한 줄로 보여 주며, 이 터널의 어떤 조건이 해당 값을 매치했는지도 표시합니다. 가상 어댑터는 제외되고 Wi-Fi 미연결 시 SSID는 "Wi-Fi 연결 안 됨"으로 표시됩니다. 패널과 규칙 안내는 접을 수 있으며 편집기 전체를 스크롤할 수 있습니다.

「인터페이스 고정」은 현재 macOS에서 `-ifscope`를 사용한 VPN 바이패스 라우트 고정만 지원합니다. Windows와 Linux에서는 아직 지원 기능으로 표시하지 않습니다. WireGuard의 물리적 출구를 지정하는 기능은 프로토콜 설정이 아니라 운영체제 라우팅 정책에 의존합니다.

### 규칙 로직

- 한 터널 안의 규칙은 두 그룹으로 나뉘어 위에서 아래로 평가됩니다: **disconnect (연결 해제) 그룹 먼저, connect (연결) 그룹 나중**. 그룹 내 순서는 에디터에서 드래그로 정렬한 우선순위 그 자체입니다.
- **규칙 안은 AND, 규칙 사이는 OR, 첫 번째 매치가 승리**: 한 규칙 안의 모든 조건이 다 성립해야 그 규칙이 발동하지만, 두 그룹 통틀어 **가장 먼저 매치된 규칙 딱 하나**만 실제 동작을 실행합니다. disconnect 그룹을 먼저 평가하므로, 매치된 disconnect 규칙은 그 뒤에 매치되는 connect 규칙을 항상 이깁니다. 뒤쪽 connect 규칙은 "매치는 됐으나 우선순위에서 밀려 무시(deprioritized)"로 표시되며 실행되지 않으므로, 같은 SSID에서 동시에 disconnect / connect 둘 다 실행되는 모순은 생기지 않습니다.
- **그 외 / none-match 규칙** (각 액션 그룹 맨 뒤의 카드): 같은 액션 그룹에서 그보다 위에 있는 규칙이 **하나도 매치되지 않았을 때**만 발동합니다.
- **편집 중 라이브 매치 인디케이터**: 자동화 에디터를 열어 두면 각 조건이 현재 네트워크와 일치하는지 그 자리에서 보여주며, 실제로 유효한 첫 매치 규칙은「in use (사용 중)」으로 강조 표시됩니다. 맨 위 판정 바에는 해당 터널의 최종 결정이 표시됩니다. 편집이 일어날 때마다 인디케이터는 즉시 재평가되며 (약 250ms 디바운스로 IPC 호출), 백그라운드 helper 가 실제 규칙을 집행하는 엔진과 **정확히 같은** 엔진을 쓰므로 UI 표시와 실제 동작은 언제나 일치합니다. 같은 엔진은 커맨드라인 (`wireguideplus automation`)에서도 이용 가능해, GUI 없는 환경이나 디버깅에 편리합니다.

### 조건 타입

| 조건 | 매치 기준 | 전형적인 사용 예 |
| --- | --- | --- |
| **SSID** | 현재 Wi-Fi SSID 이름과 **바이트 단위로 완전 일치** (대소문자 구분, 공백·특수문자 모두 비교. 802.11 정의에 따름). | "`사무실 5GHz`에 연결되면 회사 VPN 자동 연결". |
| **서브넷 (Subnet)** | 현재 물리 NIC IP 가 지정된 CIDR (예: `192.168.178.0/24`) 범위 안에 있는지. | 집/사무실 LAN 대역은 예측 가능하지만 SSID는 바뀔 수 있을 때. |
| **네트워크 / BSSID** | 기본 게이트웨이 MAC 주소 (BSSID). 단순 SSID가 아니라 물리적인 AP 본체를 특정하고 싶을 때. | "카페 공유기에서는 **절대로** 자동 연결하지 않기". |
| **게이트웨이 IP** | 현재 물리 네트워크의 기본 게이트웨이 IP. | 건물 전체가 같은 SSID지만 층마다 게이트웨이 IP가 다를 때. |
| **인터페이스 (Interface)** | 현재 상향 경로로 쓰이는 물리 NIC 이름. 드롭다운에는 **미연결된 물리 어댑터도 모두** 나오므로, 아직 꽂지 않은 독 / USB 랜 / 썬더볼트 어댑터용으로 미리 규칙을 써 둘 수 있음. | "사무실 독에 연결한 유선 어댑터를 통해 돌 때만 회사 VPN 연결". |
| **유선 네트워크 (Ethernet)** | 상향 경로가 non-Wi-Fi **유선** 어댑터를 통과할 때 매치. SSID 필요 없이 유선 / 무선 이분 판정. | "데스크에서 유선 LAN 쓸 때는 VPN 필수, Wi-Fi로 바뀌면 연결 안 함". |
| **시간 대 (Time window)** | 요일 집합 + 시작/끝 시각 (로컬 시계). | "월~금 09:00–18:00 사이 회사 VPN은 항상 켜둠". |

한 규칙 안에 위 조건들을 자유롭게 조합할 수 있습니다: 예를 들어 "SSID = 사무실 AND 시간 대 = 월~금 09–18" 이면 AND 조건 2개짜리 단일 규칙입니다. 각 터널의 disconnect / connect 양쪽 그룹에, AND 조건으로 이뤄진 규칙을 원하는 개수만큼 등록 가능합니다.

## 플랫폼 지원

| 플랫폼 | 상태 |
| --- | --- |
| Windows 10 / 11(x64, x86 32비트, ARM64) | ✅ 완전 지원 (멀티 터널 동시 연결 + SSID 자동 연결, AmneziaWG 포함) |
| macOS(Apple Silicon / arm64) | ✅ 완전 지원 — Apple Silicon 실기기에서 충분히 검증됨; 다른 WireGuard 앱 [WireTunnels](https://github.com/FMDigitech/WireTunnels)도 사용해 볼 수 있습니다 |
| Linux(x64, arm64) | 🚧 실험적 — CI에서 빌드되지만 아직 실기기에서 테스트되지 않음 |
| Android / iOS | ❌ **지원 안 함** (터널 동시 실행 불가, Wi-Fi SSID 자동 전환 불가) |

> **macOS 대안: [WireTunnels](https://github.com/FMDigitech/WireTunnels)** — 멀티
> 터널, 모니터링, 제어를 지원하는 네이티브 macOS 메뉴바 WireGuard 클라이언트로,
> 업스트림 `wireguide`를 보완합니다.

### 모바일 버전이 없는 이유

이 프로젝트의 핵심은 **멀티 터널 동시 연결**과 **조건 기반 자동 연결(예: Wi-Fi SSID)** 입니다.
Android / iOS에서는 시스템 커널과 권한 체계 때문에 WireGuard 구현이 **여러 터널을
동시에 실행**하거나 **Wi-Fi SSID에 따라 터널을 자동으로 전환**할 수 없습니다. 두 핵심
목표 모두 모바일에서 달성할 수 없으므로, 이 프로젝트는 **모바일을 명시적으로
지원하지 않습니다.** 모바일 단일 터널 용도는 공식 WireGuard 앱의 On-Demand 기능을
사용하세요.

## 로드맵

- **v2.0(계획)**: **Windows 시스템 서비스**로 실행 — 사용자 로그인 없이 자동 연결,
  더 안정적인 네트워크 스택과 권한 제어.

## 다운로드 & 설치

각 릴리스에서는 Windows 빌드를 **설치 프로그램**과 **포터블 버전** 두 종류로 나누어
배포합니다.

**설치 프로그램(권장)**

- Windows x64 설치 프로그램: `wireguideplus-amd64-installer.exe`
- Windows x86(32비트) 설치 프로그램: `wireguideplus-x86-installer.exe`
- Windows ARM64 설치 프로그램: `wireguideplus-arm64-installer.exe`

설치 프로그램 파일 이름에는 아키텍처가 포함됩니다(`wireguideplus-<arch>-installer.exe`,
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
`wireguideplus-amd64-portable.zip` / `wireguideplus-x86-portable.zip` /
`wireguideplus-arm64-portable.zip` 포터블 zip도 제공됩니다. 각 zip에는 exe와 일치하는
드라이버 DLL이 **함께** 들어 있어 압축을 풀기만 하면 실행할 수 있습니다. 릴리스에서
더 이상 개별 DLL을 첨부하지 않습니다(포터블 zip 또는 설치 프로그램을 사용하세요).
일치하는 드라이버 DLL이 exe 옆에 없으면 터널을 만들 수 없습니다.

## 코드 서명

게시되는 모든 Windows **설치 프로그램**은 Authenticode 서명이 적용되어 **무결성**(전송
중이거나 디스크에서 변조되지 않음)과 **출처**(이 프로젝트에서 빌드·배포되었음)를 동시에
검증할 수 있습니다. 서명된 바이너리는 최초 실행 시 Windows SmartScreen 경고도 덜
표시됩니다.

참고: **설치 프로그램만** 서명됩니다. 포터블 zip 안의 exe는 서명되지 않은 빌드 산출물입니다.
전체 서명 정책(범위, 승인 워크플로, 계정 보안, 재현성)은
[SIGNING-POLICY.md](SIGNING-POLICY.md)를 참조하세요.

> Free code signing provided by [SignPath.io](https://signpath.io), certificate by
> [SignPath Foundation](https://signpath.org).

## 빌드 & 개발

빌드 환경 요구 사항, 개발/배포 빌드 명령(x86 + amd64 + arm64 멀티 아키텍처 빌드 포함),
NSIS 설치 프로그램 설명, 버전 리소스 및 릴리스 워크플로는 개발 문서
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)에 정리되어 있습니다. 릴리스는 버전 태그를
로컬에서 푸시하기만 하면 GitHub Actions 파이프라인이 빌드·서명·배포까지 자동으로
처리합니다 ([docs/release.md](docs/release.md) 참조).

## 데이터 & 로그

| 항목 | 위치 |
| --- | --- |
| 설정 / 기록 | `%APPDATA%\wireguideplus\` (`config.json`, `history.json`) |
| 터널 설정 | `%APPDATA%\wireguideplus\tunnels\*.conf` |
| 로그 | `%APPDATA%\wireguideplus\logs\` |

## 제거

**제어판 → 프로그램 및 기능 → WireGuide Plus**에서 제거하거나, 설치 폴더의 제거
프로그램을 실행하세요.

## 감사의 말

- [korjwl1/wireguide](https://github.com/korjwl1/wireguide) — 업스트림 오픈소스 프로젝트
- [WireGuard](https://www.wireguard.com/) / [wireguard-go](https://git.zx2c4.com/wireguard-go)
- [Wails](https://wails.io)
