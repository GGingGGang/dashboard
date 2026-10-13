# 기능 명세와 구현 현황

기준일: 2026-10-13. 원문: `private/DESIGN_BRIEF.ko.md`(로컬 비공개 문서).
분류 기준 커밋: `9aadaf1`. 아래 상태는 구현 진행에 따라 갱신한다.

이 문서는 브리프의 제안과 실제 동작을 구별한다. **완료**는 명시한 범위의 코드와 테스트가 있다는 뜻이며 실서비스 검증을 뜻하지 않는다. **부분**은 계약 일부만 충족한다. **미구현**은 실행 경로가 없다. **조건부 보류**는 이번 범위에서 제외한 선택 사항이다.

이번 범위는 기존 코드 유지, 핵심 계약 보완, Loki 로그와 Tempo 트레이스의 읽기 전용 조회다. CLI/MCP 소비자와 외부 프로세스 플러그인은 사용자 선택에 따라 보류한다. 브리프의 장기 제안 전체를 이번 릴리스의 완료 조건으로 간주하지 않는다.

## 기능별 계약

| ID / 기능 | 명세와 책임 | 현재 상태 / 근거 | 수용 조건과 남은 작업 |
|---|---|---|---|
| F01 계층 분리 | Desktop은 UI 입출력, Application은 사용 사례·수명주기, Platform은 계약, Provider는 외부 API 변환, Storage는 보존 담당 | 완료: [Application](internal/application/service.go), [ports](internal/application/ports.go), [Desktop](app.go) | Application이 Wails·구체 저장소·벤더를 import하지 않는다. 기존 기능을 같은 포트로 호출한다. |
| F02 Provider 등록 | 부작용 없는 설명과 로컬 검증 뒤 생성한다. 선언 capability와 실제 Source 구현이 일치해야 한다 | 부분: [Registry](internal/platform/registry.go), [계약](internal/platform/model.go) | 복합 Provider에 capability별 query 언어·한도·모드를 제공한다. 선언과 인가·현재 가용성을 혼동하지 않는다. |
| F03 설정·자격증명 | 연결별 endpoint, tenant, 사용자, secret binding을 분리한다. 비밀은 OS 저장소에만 둔다 | 부분: Application과 [HTTP client](internal/providers/internal/httpapi/client.go) | tenant 지원, 다른 tenant/address에 기존 비밀 자동 재사용 금지. 로컬 검증은 네트워크 호출을 하지 않는다. |
| F04 인스턴스 수명주기 | revision마다 취소 범위와 소유권을 갖고, 종료 시 요청을 배수한 뒤 Close한다 | 완료 범위: 협력적 취소, 늦은 성공·실패 무효화, 종료 대기 | [수명주기 테스트](internal/application/service_test.go). 비협력적 in-process Provider의 강제 중단은 보장하지 않는다. |
| F05 수집·예산 | capability와 자동 수집을 분리한다. 대기·실행·응답 크기·건수에 유한 한도를 둔다 | 부분: on-demand, 전역 동시 2, HTTP 8초/8MiB, poll 60초 | capability별 시간 예산, 대상별 부분 결과와 공정성 보완. 무한 재시도·무한 페이지 수집 금지. |
| F06 결과 소유권·캐시 | 반환된 slice/map은 소유 경계를 넘을 때 복사한다. 캐시는 연결 revision과 질의 범위에 귀속한다 | 완료 범위: snapshot/query/descriptor 복사, UI 세대 검사 | [ownership](internal/platform/ownership.go), [UI](frontend/src/main.ts). 새 로그·트레이스 결과에도 적용한다. |
| F07 관측 결과 | 요청 ID, 연결 revision, 관측 시각, 요청 범위, outcome, completeness, 진단을 데이터와 함께 반환한다 | 부분: Snapshot의 module별 시각·error만 존재 | 빈 결과·실패·부분 결과를 구분한다. HTTP 성공이 서비스 정상 또는 데이터 완전성을 뜻하지 않는다. |
| F08 빌드 이력 | 실행 식별자와 표시 번호를 분리하고 재시도·번호 재사용을 보존한다 | 부분: Jenkins 숫자 번호와 시작시각 조합, 늦은 RUNNING 회귀 방지 | 실제 두 번째 CI 사례로 opaque execution ID 계약을 검증한 뒤 DB migration을 결정한다. 현재 SQL 의미를 조용히 바꾸지 않는다. |
| F09 페이지·커서 | cursor는 Provider 소유이며 연결/revision/query/window에 귀속한다. 유효기간·반복 페이지를 다룬다 | 부분: Jenkins offset만 지원 | Loki 시각에 1ns를 더해 완전한 다음 페이지라고 주장하지 않는다. 지원하지 않는 연속 조회는 표시하고 제한된 검색으로 제공한다. |
| F10 배포 | sync/health와 rollout 상태를 분리하고 원본 revision·누락 의미를 보존한다 | 부분: [Argo CD](internal/providers/argocd/argocd.go) 독립 Source | desired/observed revision의 범용 계약과 다중 배포 도구 검증은 남음. sync가 health를 대체하지 않는다. |
| F11 지표 | instant/range, labels, sample time, 비유한 값, 손실 경고를 보존한다 | 부분: [Prometheus](internal/providers/prometheus/prometheus.go) | 규칙 실패가 마지막 성공을 갱신하지 않는다. stale/빈값은 정상으로 판정하지 않는다. histogram의 미표시를 알린다. |
| F12 로그 | 독립 LogSource, LogQL 범위 검색, ns 문자열, labels와 structured metadata, 중복·동일 시각 보존 | 미구현 | Loki GET query_range → Application → Desktop → UI. 최대 24시간/500행, 제한 도달 시 완전성 불명 표시. tail은 별도 선택 기능이다. |
| F13 트레이스 | 독립 TraceSource, TraceQL 검색과 ID 상세 조회. trace/span/parent ID와 자원 속성·시간 단위를 보존한다 | 미구현 | Tempo GET search/trace → UI. 검색 한도와 상세 부분 결과 표시. 없는 parent를 root로 만들지 않는다. |
| F14 상관관계 | Source 밖에서 명시적 ID·commit·artifact·service mapping을 증거로 연결한다 | 부분: 대상 service/environment 매핑만 있음 | 시간상 근접만으로 인과를 확정하지 않는다. 자동 로그→트레이스/배포→빌드 그래프는 추가 도메인 설계 필요. |
| F15 설정 권위 | SQLite 설정, OS secret, archive, cache, UI preference의 권위를 구분한다 | 부분: [Storage](internal/storage/store.go), DB backup | 알려지지 않은 설정 필드와 없는 Provider를 보존한다. 파일 import/export와 file-authoritative 모드는 별도 기능이다. |
| F16 호환성·이전 | 미래 DB 버전을 거절하고 실패 시 원본을 보존한다. 백업은 이후 관측의 복구를 보장하지 않는다 | 부분: schema v1, 미래 version 거절, VACUUM INTO backup | 실제 schema 변경 때 원자적 migration/재시도/실패 fixture 추가. 앱·설정·DB·프로토콜 버전을 하나로 취급하지 않는다. |
| F17 전송 정밀도 | JS 안전 정수를 넘는 ID와 ns 시각은 문자열로 전송한다. 단위를 명시한다 | 부분: 기존 build number는 숫자 | Loki/Tempo의 ns·ID는 처음부터 문자열. 기존 빌드 ID 변경은 F08과 함께 결정한다. |
| F18 소비자 | Application 계약을 여러 소비자가 공유하고 UI가 정책을 복제하지 않는다 | 부분: Desktop 사용, 분리된 Application | CLI/MCP 구현은 **조건부 보류**. 공유 계약과 headless 테스트는 유지한다. |
| F19 런타임 확장 | 컴파일 등록과 외부 프로세스 프로토콜을 구분한다 | 정적 등록 완료, 외부 프로세스 **조건부 보류** | 필요 시 Windows subprocess/handshake/version/cancel/신뢰 설치 정책을 별도 ADR로 결정한다. 프로세스 분리가 sandbox는 아니다. |
| F20 검증·기록 | 계약/HTTP fixture/통합/UI/DB 검증과 선택적 live 검증을 분리한다 | 부분: [VERIFICATION](VERIFICATION.md), Go·Playwright | 신규 Source의 변환·한도·정밀도·취소·UI 늦은 결과를 검증하고 실제 실행 결과만 기록한다. |

## 불변식 대응

| 브리프 ID | 기능 | 판정과 예외 |
|---|---|---|
| INV-01 의존 방향 | F01 | 완료: Application 포트 분리 |
| INV-02 Provider 책임 | F01/F10–13 | 부분: 기존 어댑터 독립, 신규 계약 진행 예정 |
| INV-03 capability/인가/탐색 | F02 | 부분: 발견 권한 실패와 직접 조회 분리, feature 계약 보완 필요 |
| INV-04 설명·검증 무부작용 | F02/F03 | 완료 범위: registry metadata/local validation |
| INV-05 범위·revision | F03/F04 | 부분: revision 있음, tenant 추가 필요 |
| INV-06 수명주기 | F04 | 협력적 취소 범위 완료 |
| INV-07 수집 정책 분리 | F05 | 완료 범위: OnDemand |
| INV-08 유한 예산 | F05 | 부분: 글로벌/HTTP 한도, 세부 공정성 남음 |
| INV-09 부분 실패 | F07 | 부분: module 격리, target별 관측 부족 |
| INV-10 freshness/health | F07/F11 | 부분: rule 실패 갱신 수정 필요 |
| INV-11 사실 보존 | F08/F10–13 | 부분: 각 Source 의미 검토 필요 |
| INV-12 실행 정체성 | F08 | 부분: Jenkins에 한정 |
| INV-13 cursor 소유 | F09 | 미구현: 범용 cursor를 지원한다고 선언하지 않음 |
| INV-14 wire 정밀도 | F17 | 부분: 기존 숫자 ID 제약 |
| INV-15 불변 결과 | F06 | 기존 경로 완료, 신규 결과도 동일 적용 |
| INV-16 소비자 공통 계약 | F18 | Application 완료, 추가 소비자 보류 |
| INV-17 비밀·endpoint | F03 | 부분: tenant binding 추가 필요 |
| INV-18 읽기 전용 | F01/F03 | 완료 범위: 관리 대상 변경 API 없음, Argo 로그인은 명시적 인증 |
| INV-19 설정·migration | F15/F16 | 부분: unknown 필드 보존과 migration 검증 남음 |
| INV-20 호환 거절 | F16 | 부분: 미래 DB 거절, 설정 버전 정책 남음 |
| INV-21 증거 기반 연계 | F14 | 자동 상관관계 미구현 |
| INV-22 구현 교체 불변성 | F19/F20 | 정적 테스트 일부, 외부 실행은 보류 |

## 설계 결정과 보류 이유

| ADR | 이번 결정 | 재검토 조건 |
|---|---|---|
| 01 typed/hybrid 계약 | typed Source/DTO 유지, 필요한 metadata만 확장 | 실제 동적 설정 화면 필요 시 schema subset 결정 |
| 02 실행 방식 | 컴파일 등록 유지 | 사용자 선택으로 subprocess 보류 |
| 03 capability·feature | 로그/트레이스 독립, capability별 질의 설명 추가 | tail·TraceQL metrics는 실제 지원 시 별도 선언 |
| 04 설정 권위 | SQLite 유지 | 파일 기반 협업 설정 도입 시 import/export 설계 |
| 05 실행 식별자 | 기존 archive 키 보존 | 실제 두 번째 CI 사례+이전 fixture 확보 |
| 06 cursor | 제한된 검색만 제공, 완전성 한계 명시 | upstream의 안정적인 continuation 의미 확보 |
| 07 스케줄링 | Application이 admission/cancel, Collector가 bounded poll | 대상 규모·대기시간 측정 후 fairness 강화 |
| 08 결과 계약 | 새 탐색 경로에 typed envelope 도입 | 기존 Source 전체 이전은 호환성 검증과 함께 |
| 09 지원 범위 | 현재 DB v1/정적 Provider, 미래 DB 거절 | 첫 schema migration에서 지원 창 명시 |
| 10 정밀도·증거 | ns와 opaque ID 문자열, 연계는 명시적 증거만 | 자동 상관관계 도입 시 provenance/보존 정책 |

## 필수 시나리오 검증 목록

| 브리프 시나리오 | 현재 검증 / 추가할 검증 |
|---|---|
| T01 다중 연결·tenant·비밀 | 기존 연결 격리 테스트, tenant HTTP/binding 추가 예정 |
| T02 선언과 구현 불일치 | Registry 거절 테스트 있음, logs/traces 추가 예정 |
| T03 발견 403·직접 읽기 성공 | Application/UI 회귀 테스트 있음 |
| T04 단일 capability/target timeout | module 실패 fixture 있음, deadline 격리 보완 필요 |
| T05 HTTP 200 stale/empty | Prometheus nonfinite fixture, rule 실패/빈 결과 보완 필요 |
| T06 cursor 만료·반복·scope | cursor 미지원, 미검증 |
| T07 새 항목 삽입·동일 시각 | Loki 동일 ns·중복 보존 테스트 추가 예정 |
| T08 opaque ID·attempt·재사용 | 번호 재사용 archive 테스트 있음, 두 번째 CI 없음 |
| T09 늦은 RUNNING | Storage terminal 보존 테스트 있음 |
| T10 편집·삭제·교체 후 늦은 응답 | Application과 UI 테스트 있음 |
| T11 slice/map 재사용 | Collector/Registry 복사 테스트 있음 |
| T12 429·크기·동시성 | HTTP 제한과 admission 테스트 있음, 신규 어댑터 한도 추가 예정 |
| T13 비밀과 endpoint | TLS/redirect/auth fixture 있음, tenant binding 추가 예정 |
| T14 없는 Provider·신규 schema | 읽기는 가능, unknown 필드 보존 보완 필요 |
| T15 migration 실패·재시도·backup | backup/미래 DB 거절 있음, migration 아직 없음 |
| T16 ns·큰 ID wire | Loki/Tempo DTO JSON round trip 추가 예정 |
| T17 metadata 무부작용 | Registry metadata 테스트 있음 |
| T18 종료·배수 | Application 테스트 있음, CLI/MCP 보류 |

## 원문 구조와 문서 위치

브리프 §1–3(목적·비교·현황)은 이 문서의 범위와 기준 커밋, §4–7(개념·계층·descriptor·수명주기)은 F01–06, §8(관측 모델)은 F07–13, §9(식별자·페이지·연계)는 F08/F09/F14, §10–12(설정·보안·호환성)는 F03/F15–17, §13(외부 확장)은 F19, §14–16(불변식·기록·ADR)은 위 대응표, §17–20(변경면·테스트·실행 절차)은 F20과 아래 이력에 대응한다. §21 참고자료는 원문에 유지한다.

기존 [ARCHITECTURE.md](ARCHITECTURE.md)는 구조 설명, [CONTRIBUTING.md](CONTRIBUTING.md)는 구현 방법, [VERIFICATION.md](VERIFICATION.md)는 실행 증거를 담당한다. 비공개 `private/provider-architecture-review/` 문서는 과거 검토 기록이며 현재 구현의 증거로 간주하지 않는다.

## 구현 순서와 커밋 이력

1. `9aadaf1` — Application·포트 분리, lifecycle/소유권/발견 권한 테스트. Go race/vet, UI 20개, Windows 빌드 검증 완료.
2. 이 문서 — 기능별 계약·현황·수용 조건 기록.
3. 구현 — capability별 query metadata, tenant binding, 결과 envelope와 LogSource/TraceSource, Loki/Tempo HTTP 어댑터와 Desktop 메서드. Go fixture로 로그 중복/ns, Tempo base64/hex ID·typed 속성, 부분 결과·revision 취소를 검증했다. UI 연결 전까지 F12/F13은 부분 구현이다.
4. 예정 — Loki/Tempo UI, fixture와 회귀 테스트.
5. 예정 — 최종 검증 결과와 남은 항목 갱신.

현재 live Loki/Tempo 서버와 실제 Windows 자격증명에 대한 검증은 수행하지 않았다. fixture 통과를 제품 버전 전체의 호환성 보장으로 해석하지 않는다.
