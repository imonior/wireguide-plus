# WireGuide Plus

**Windows용 멀티 터널 · 자동화 중심 WireGuard 클라이언트**

WireGuide Plus는 오픈소스 프로젝트 [`korjwl1/wireguide`](https://github.com/korjwl1/wireguide)를
**깊이 있게 수정·강화한** 포크입니다. 두 가지 핵심 기능:

- **멀티 터널 동시 연결** — 여러 WireGuard 터널을 동시에 연결해 서로 간섭 없이
  독립적으로 실행할 수 있습니다.
- **조건 기반 자동 연결** — Wi-Fi SSID, 시간대, 시스템 시작 등의 조건에 따라
  알맞은 터널을 자동으로 연결합니다 (예: 사무실 Wi-Fi에서는 터널 A, 집에서는 터널 B).

[English](README.md) | [简体中文](README.zh.md) | [繁體中文](README.zh-TW.md) | **한국어** | [日本語](README.ja.md)

> **Windows 10 / 11(x64, x86 32비트 및 ARM64)을 완전히 지원**합니다. macOS(Apple
> Silicon)와 Linux(x64, arm64)는 **실험적 프리뷰**로 제공됩니다 — CI에서 빌드되지만
> 아직 실기기에서 테스트되지 않았습니다 ([플랫폼 지원](#플랫폼-지원) 참조).
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
   (자세한 내용은 [CHANGELOG](CHANGELOG.en.md) 참조).

### 개선

1. **SSID 드롭다운** — 자동 연결 규칙에서 시스템에 **저장된 모든 Wi-Fi SSID**를
   드롭다운으로 선택할 수 있어 오타가 없습니다.
2. **프록시 통한 업데이트 확인** — 업데이트 확인 전에 HTTP(S) 프록시를 설정할 수 있어,
   GitHub 접속 불가/제한으로 인한 업데이트 실패를 해결합니다.
3. **다국어 UI** — 简体中文 / English / 日本語 / 한국어 / 繁體中文.
4. **시스템 통합** — 로그인 시 자동 시작, 트레이 최소화 시작, 트레이 연결 상태 알림
   (시작 후 10초 / 네트워크 변화 후 10초 지연 표시, 기본 10초, 조절 가능).
5. **창 제목 및 상호작용 개선** 등.

## 플랫폼 지원

| 플랫폼 | 상태 |
| --- | --- |
| Windows 10 / 11(x64, x86 32비트, ARM64) | ✅ 완전 지원 (멀티 터널 동시 연결 + SSID 자동 연결) |
| macOS(Apple Silicon / arm64) | 🚧 실험적 — CI에서 빌드되지만 아직 실기기에서 테스트되지 않음; 다른 WireGuard 앱 [WireTunnels](https://github.com/FMDigitech/WireTunnels)도 사용해 볼 수 있습니다 |
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
