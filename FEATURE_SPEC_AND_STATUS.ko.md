# 기능 명세와 구현 현황

기준일: 2026-10-28. 원문: `private/DESIGN_BRIEF.ko.md`(로컬 비공개 문서).
최초 분류 기준: `9aadaf1`. 현재 상태는 아래 커밋 이력의 구현과 검증 결과를 반영한다.

이 문서는 브리프의 제안과 실제 동작을 구별한다. **완료**는 명시한 범위의 코드와 테스트가 있다는 뜻이며 실서비스 검증을 뜻하지 않는다. **부분**은 계약 일부만 충족한다. **미구현**은 실행 경로가 없다. **조건부 보류**는 이번 범위에서 제외한 선택 사항이다.

이번 범위는 기존 코드 유지, 핵심 계약 보완, Loki 로그와 Tempo 트레이스의 읽기 전용 조회, 개인별 대시보드 배치, 인증 없는 HTTP JSON의 jq 조회다. CLI/MCP 소비자, Command 실행 및 범용 API 인증은 보류한다. 플러그인 확장 1–4단계는 구현했고 외부 프로세스·ZIP 설치는 5단계, 나머지 provider 전환은 6단계다. 브리프의 장기 제안 전체를 이번 릴리스의 완료 조건으로 간주하지 않는다.

## 기능별 계약

| ID / 기능 | 명세와 책임 | 현재 상태 / 근거 | 수용 조건과 남은 작업 |
|---|---|---|---|
| F01 계층 분리 | Desktop은 UI 입출력, Application은 사용 사례·수명주기, Platform은 계약, Provider는 외부 API 변환, Storage는 보존 담당 | 완료: [Application](internal/application/service.go), [ports](internal/application/ports.go), [Desktop](app.go) | Application이 Wails·구체 저장소·벤더를 import하지 않는다. 기존 기능을 같은 포트로 호출한다. |
| F02 Provider 등록 | 부작용 없는 설명과 로컬 검증 뒤 생성한다. 선언 capability와 실제 Source 구현이 일치해야 한다 | 완료 범위: [Registry](internal/platform/registry.go), capability별 `Queries`, 기존 `Query` 호환 별칭 | 기존 6개 Source와 범용 DataSource의 선언/구현을 검사한다. 선언형 플러그인 화면과 flat JSON Schema 폼을 제공한다. 권한별 feature 협상과 중첩 스키마는 미구현이다. |
| F03 설정·자격증명 | 연결별 endpoint, tenant, 사용자, secret binding을 분리한다. 비밀은 OS 저장소에만 둔다 | 완료 범위: tenant 검증, HTTP 헤더, 기존 연결의 tenant 변경 거절, 다른 tenant에 저장 secret 사용 거절 | [binding 테스트](internal/application/observations_test.go). 로컬 검증은 네트워크 호출을 하지 않는다. OS 비밀 삭제 실패 후 잔여 항목 청소는 남음. |
| F04 인스턴스 수명주기 | revision마다 취소 범위와 소유권을 갖고, 종료 시 요청을 배수한 뒤 Close한다 | 완료 범위: 협력적 취소, 늦은 성공·실패 무효화, 종료 대기 | [수명주기 테스트](internal/application/service_test.go). 비협력적 in-process Provider의 강제 중단은 보장하지 않는다. |
| F05 수집·예산 | capability와 자동 수집을 분리한다. 대기·실행·응답 크기·건수에 유한 한도를 둔다 | 완료 범위: on-demand, 전역 2/연결별 1, HTTP 8초/8MiB, 대기 포함 poll 60초, module별 12초 | 수동 조회 8초/연결검사 45초에 대기 포함. target별 freshness와 부하 기반 공정성 측정은 남음. 취소는 협력적이다. |
| F06 결과 소유권·캐시 | 반환된 slice/map은 소유 경계를 넘을 때 복사한다. 캐시는 연결 revision과 질의 범위에 귀속한다 | 완료 범위: snapshot/query/descriptor 복사, UI 세대 검사 | [ownership](internal/platform/ownership.go), [UI](frontend/src/main.ts). 새 로그·트레이스 결과에도 적용한다. |
| F07 관측 결과 | 요청 ID, 연결 revision, 관측 시각, 요청 범위, outcome, completeness, 진단을 데이터와 함께 반환한다 | 부분: 새 로그·트레이스 경로는 [Observation](internal/platform/observations.go) envelope 구현, 기존 Snapshot은 module 상태 | Wails에서도 부분 데이터와 진단을 함께 받는다. 구 revision의 취소는 호출 거절로 처리한다. 기존 빌드·배포·지표의 공통 envelope 이전은 남음. |
| F08 빌드 이력 | 실행 식별자와 표시 번호를 분리하고 재시도·번호 재사용을 보존한다 | 부분: Jenkins 숫자 번호와 시작시각 조합, 늦은 RUNNING 회귀 방지 | 실제 두 번째 CI 사례로 opaque execution ID 계약을 검증한 뒤 DB migration을 결정한다. 현재 SQL 의미를 조용히 바꾸지 않는다. |
| F09 페이지·커서 | cursor는 Provider 소유이며 연결/revision/query/window에 귀속한다. 유효기간·반복 페이지를 다룬다 | 부분: Jenkins offset만 지원 | Loki 시각에 1ns를 더해 완전한 다음 페이지라고 주장하지 않는다. 지원하지 않는 연속 조회는 표시하고 제한된 검색으로 제공한다. |
| F10 배포 | sync/health와 rollout 상태를 분리하고 원본 revision·누락 의미를 보존한다 | 부분: [Argo CD](internal/providers/argocd/argocd.go) 독립 Source | desired/observed revision의 범용 계약과 다중 배포 도구 검증은 남음. sync가 health를 대체하지 않는다. |
| F11 지표 | instant/range, labels, sample time, 비유한 값, 손실 경고를 보존한다 | 완료 범위: 기존 조회와 [규칙 실패 회귀 수정](internal/collector/metrics_test.go) | 오류 시 이전 값 유지, 성공 시각 미갱신. 빈값·stale·미래 시각·truncation·경고는 판정 불가. native histogram은 경고 후 미표시이며 typed histogram 구현은 남음. |
| F12 로그 | 독립 LogSource, LogQL 범위 검색, ns 문자열, labels와 structured metadata, 중복·동일 시각 보존 | 완료: [Loki](internal/providers/loki/loki.go), [UI](frontend/src/components/explorer.ts), HTTP/Application/브라우저 검증 | 최대 24시간/500행. 제한 도달 시 전체 여부 불명, 동시각 중복을 제거하지 않음. tail·안정적 cursor·로그 archive는 미구현이다. |
| F13 트레이스 | 독립 TraceSource, TraceQL 검색과 ID 상세 조회. trace/span/parent ID와 자원 속성·시간 단위를 보존한다 | 완료: [Tempo](internal/providers/tempo/tempo.go), 검색/ID 상세 UI, HTTP/브라우저 검증 | 검색 100건·상세 2000 span. base64/hex ID 지원, OTLP typed 속성·events·links 원문 보존. 샘플링/누락은 불명, 부모 누락을 root로 추정하지 않는다. |
| F14 상관관계 | Source 밖에서 명시적 ID·commit·artifact·service mapping을 증거로 연결한다 | 부분: 대상 service/environment 매핑만 있음 | 시간상 근접만으로 인과를 확정하지 않는다. 자동 로그→트레이스/배포→빌드 그래프는 추가 도메인 설계 필요. |
| F15 설정 권위 | SQLite 설정, OS secret, archive, cache, UI preference의 권위를 구분한다 | 완료 범위: [Storage](internal/storage/sqlite.go), 알 수 없는 최상위 설정·target/rule 필드 보존, DB backup | target/rule ID로 기존 unknown 필드를 보존하고 삭제한 항목은 복원하지 않는다. unknown 필드를 UI DTO에 노출하지 않는다. 설정 파일 import/export는 미구현이다. |
| F16 호환성·이전 | 미래 DB 버전을 거절하고 실패 시 원본을 보존한다. 백업은 이후 관측의 복구를 보장하지 않는다 | 부분: DB v1 유지, 설정 schemaVersion 0/1 지원·저장 시 1, 미래 설정 덮어쓰기 거절, backup | 설정 편집은 transaction으로 처리한다. 실제 DB schema 변경은 없으므로 versioned migration/재시도 검증은 남음. |
| F17 전송 정밀도 | JS 안전 정수를 넘는 ID와 ns 시각은 문자열로 전송한다. 단위를 명시한다 | 부분: Loki/Tempo ns·ID·OTLP int64 문자열 보존 검증 완료 | 기존 build number는 숫자이므로 F08과 함께 변경해야 한다. 표시용 날짜 변환과 원본 ns 표시를 구분한다. |
| F18 소비자 | Application 계약을 여러 소비자가 공유하고 UI가 정책을 복제하지 않는다 | 부분: Desktop 사용, 분리된 Application | CLI/MCP 구현은 **조건부 보류**. 공유 계약과 headless 테스트는 유지한다. |
| F19 런타임 확장 | 컴파일 등록과 외부 프로세스 프로토콜을 구분한다 | 정적 등록 완료, 외부 프로세스 **조건부 보류** | 필요 시 Windows subprocess/handshake/version/cancel/신뢰 설치 정책을 별도 ADR로 결정한다. 프로세스 분리가 sandbox는 아니다. |
| F20 검증·기록 | 계약/HTTP fixture/통합/UI/DB 검증과 선택적 live 검증을 분리한다 | 실행 근거와 최종 테스트 수·빌드 결과는 [VERIFICATION](VERIFICATION.md)에 기록한다 | 이번 UI의 네이티브 직접 조작·실서비스·대규모 부하 검증은 미수행이다. 과거 네이티브 검증과 구분한다. |
| F21 개인 대시보드 | 개발자가 계속 열어 두는 화면에서 카드의 위치·크기·내용을 직접 구성한다. 저장된 배치가 유효하지 않으면 원본을 보존한다 | 완료 범위: [layout](frontend/src/components/dashboard-layout.ts), [controller](frontend/src/components/dashboard.ts), [data](frontend/src/components/dashboard-data.ts). `dashboard.layout` v2는 v1 원본을 별도 백업하고 SQLite preference에 저장하며 12열·최대 24개·비중첩을 검증한다 | 빌드/대기열/배포는 수집 snapshot을 표시한다. 지표/로그/트레이스는 대시보드를 여는 시점·수동 새로고침·선택한 주기에만 직접 조회한다. 로그 100행, 트레이스 30건, 24시간 범위를 넘지 않는다. 빈 배치는 저장한다. 손상되었거나 미래 버전인 배치는 덮어쓰지 않고 편집을 막는다. 배치·조회 수명주기·저장 실패는 브라우저 fixture로 검증했다. |

## 새 조회 기능의 입출력과 실패 의미

`SearchLogs(connectionID, SearchRequest)`와 `SearchTraces(connectionID, SearchRequest)`의 입력은 `expression`, epoch 초 단위 `start/end`, 양수 `limit`이다. 식은 16KiB 이하, 범위는 24시간 이하이며 미래 종료 시각은 60초까지의 오차만 허용한다. 연결의 저장 설정과 credential binding으로 Provider를 생성한다. 등록하거나 새로고침하는 것만으로 로그·트레이스를 조회하지 않는다.

`Trace(connectionID, traceID)`는 hex ID를 검증하고 상세 데이터를 읽는다. 검색 시간 범위를 상세 조회에 자동 적용하지 않는다. 결과는 로그 행, 트레이스 요약, span 배열 중 하나이며 공통 `observation`에 `connectionId/revision/requestId/observedAt/windowStart/windowEnd/outcome/completeness/diagnostics`를 담는다. 로그의 `timestampNs`, 트레이스의 `startNs/durationNs`, 모든 trace/span ID는 문자열이다. 검색 요약의 `durationMs`는 밀리초 숫자다.

`outcome`은 `success/partial/error`, `completeness`는 `complete/unknown/truncated`다. 행이 있는 오류는 데이터와 진단을 함께 전달한다. 설정 교체·삭제·종료로 취소된 응답은 결과를 버리고 호출을 거절한다. UI는 페이지·요청 번호·연결 revision을 다시 확인해 늦은 성공과 실패를 모두 버린다. 빈 검색은 정상 서비스의 증거가 아니며, HTTP 성공도 sampling·누락 없음의 증거가 아니다.

Loki는 [공식 range API](https://grafana.com/docs/loki/latest/reference/loki-http-api/)의 시작 포함·종료 제외 범위를 사용한다. Tempo 검색과 상세는 [공식 HTTP API](https://grafana.com/docs/tempo/latest/api_docs/)를 사용하며, tenant 헤더는 [공식 tenant 명세](https://grafana.com/docs/tempo/latest/configuration/tenant-ids/)를 따른다. 구현 fixture는 이 응답 형태를 검증하며 제품 버전 전체를 인증하지 않는다.

## 이번 범위에서 남긴 항목

CLI/MCP는 보류다. 외부 프로세스 플러그인·ZIP 설치는 다음 5단계다. 그 밖에 실제 두 번째 CI와 archive migration, provider-owned cursor, target별 상세 freshness, 기존 Source 전체의 envelope 전환, 자동 상관관계, 중첩·고급 설정 schema, 파일 import/export는 아직 구현되지 않았다. 각각 F08/F09/F05/F07/F14/F02/F15의 수용 조건을 충족한 뒤 도입한다. Loki tail과 LogQL/TraceQL metrics도 지원한다고 선언하지 않는다.

## 불변식 대응

| 브리프 ID | 기능 | 판정과 예외 |
|---|---|---|
| INV-01 의존 방향 | F01 | 완료: Application 포트 분리 |
| INV-02 Provider 책임 | F01/F10–13 | 완료 범위: 5개 어댑터 독립, 별도 로그·트레이스 계약 |
| INV-03 capability/인가/탐색 | F02 | 부분: 발견 권한 실패와 직접 조회 분리, feature 계약 보완 필요 |
| INV-04 설명·검증 무부작용 | F02/F03 | 완료 범위: registry metadata/local validation |
| INV-05 범위·revision | F03/F04 | 완료 범위: tenant binding·runtime revision·늦은 응답 차단 |
| INV-06 수명주기 | F04 | 협력적 취소 범위 완료 |
| INV-07 수집 정책 분리 | F05 | 완료 범위: OnDemand |
| INV-08 유한 예산 | F05 | 기본 한도 완료, 부하 공정성 측정 남음 |
| INV-09 부분 실패 | F07 | 부분: module 격리, target별 관측 부족 |
| INV-10 freshness/health | F07/F11 | 규칙 실패/불완전 결과 처리 완료, target별 freshness 남음 |
| INV-11 사실 보존 | F08/F10–13 | 부분: 각 Source 의미 검토 필요 |
| INV-12 실행 정체성 | F08 | 부분: Jenkins에 한정 |
| INV-13 cursor 소유 | F09 | 미구현: 범용 cursor를 지원한다고 선언하지 않음 |
| INV-14 wire 정밀도 | F17 | 부분: 기존 숫자 ID 제약 |
| INV-15 불변 결과 | F06 | 기존·로그·트레이스 경로 복사 구현 |
| INV-16 소비자 공통 계약 | F18 | Application 완료, 추가 소비자 보류 |
| INV-17 비밀·endpoint | F03 | endpoint/tenant binding 완료, 임의 외부 Provider의 진단 redaction 강제는 남음 |
| INV-18 읽기 전용 | F01/F03 | 완료 범위: 관리 대상 변경 API 없음, Argo 로그인은 명시적 인증 |
| INV-19 설정·migration | F15/F16 | unknown 필드 보존 완료, DB migration은 변경 사례 필요 |
| INV-20 호환 거절 | F16 | 미래 DB/설정 거절 완료, 여러 릴리스 지원 창은 미정 |
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
| T01 다중 연결·tenant·비밀 | 기존 연결 격리, 신규 tenant HTTP/binding 테스트 통과 |
| T02 선언과 구현 불일치 | Registry 거절 테스트, logs/traces 실제 정의 생성·독립 Source 검증 |
| T03 발견 403·직접 읽기 성공 | Application/UI 회귀 테스트 있음 |
| T04 단일 capability/target timeout | module 실패 fixture, 12초 deadline 구현. target별 시간 예산은 미구현 |
| T05 HTTP 200 stale/empty | Prometheus nonfinite 및 rule 실패·빈값·미래값·불완전 결과 테스트 통과 |
| T06 cursor 만료·반복·scope | cursor 미지원, 미검증 |
| T07 새 항목 삽입·동일 시각 | Loki 동일 ns·중복 보존 통과. 페이지 연속 조회는 미지원 |
| T08 opaque ID·attempt·재사용 | 번호 재사용 archive 테스트 있음, 두 번째 CI 없음 |
| T09 늦은 RUNNING | Storage terminal 보존 테스트 있음 |
| T10 편집·삭제·교체 후 늦은 응답 | Application과 UI 테스트 있음 |
| T11 slice/map 재사용 | Collector/Registry 복사 테스트 있음 |
| T12 429·크기·동시성 | 공유 HTTP 제한, 전역/연결 gate deadline, Loki 입력 한도 테스트 통과 |
| T13 비밀과 endpoint | TLS/redirect/auth 및 tenant binding 테스트 통과 |
| T14 없는 Provider·신규 schema | absent provider 읽기·unknown 필드 보존·미래 설정 거절 테스트 통과 |
| T15 migration 실패·재시도·backup | backup/미래 DB 거절 있음, migration 아직 없음 |
| T16 ns·큰 ID wire | HTTP fixture/JSON marshal/UI에서 ns·ID·큰 int64 문자열 보존 통과 |
| T17 metadata 무부작용 | Registry metadata 테스트 있음 |
| T18 종료·배수 | Application 테스트 있음, CLI/MCP 보류 |

## 원문 구조와 문서 위치

브리프 §1–3(목적·비교·현황)은 이 문서의 범위와 기준 커밋, §4–7(개념·계층·descriptor·수명주기)은 F01–06, §8(관측 모델)은 F07–13, §9(식별자·페이지·연계)는 F08/F09/F14, §10–12(설정·보안·호환성)는 F03/F15–17, §13(외부 확장)은 F19, §14–16(불변식·기록·ADR)은 위 대응표, §17–20(변경면·테스트·실행 절차)은 F20과 아래 이력에 대응한다. §21 참고자료는 원문에 유지한다.

기존 [ARCHITECTURE.md](ARCHITECTURE.md)는 구조 설명, [CONTRIBUTING.md](CONTRIBUTING.md)는 구현 방법, [VERIFICATION.md](VERIFICATION.md)는 실행 증거를 담당한다. 비공개 `private/provider-architecture-review/` 문서는 과거 검토 기록이며 현재 구현의 증거로 간주하지 않는다.

## 구현 순서와 커밋 이력

후속 API 쿼리는 인증 없는 HTTP GET JSON과 jq를 지원한다. 결과를 수동 조회 노드로 추가·수정할 수 있으며, 숫자/문자열은 값으로, 배열/객체는 JSON으로 표시한다. 큰 정수는 문자열 전달로 보존한다. Command는 비활성이고 YAML은 지원하지 않는다. 그래프 확대는 시간축에만 적용하고, 오른쪽 값 표시·축 눈금·마우스 조회를 제공한다. 상세 제한과 사용법은 README, 검증 범위는 VERIFICATION을 따른다.

1. `9aadaf1` — Application·포트 분리, lifecycle/소유권/발견 권한 테스트. Go race/vet, UI 20개, Windows 빌드 검증 완료.
2. `7d0d363` — 기능별 계약·현황·수용 조건 문서.
3. `6514950` — LogSource/TraceSource, envelope, tenant binding, Loki/Tempo, Desktop 메서드와 fixture.
4. `ac9ecd6` — 로그·트레이스 UI, 정확한 문자열 표시, 늦은 응답과 HTML 원문 처리 테스트.
5. `72c9ba3` — unknown 설정 보존·미래 schema 거절, 수집 예산·규칙 실패 처리, 5개 도구 네이티브 데모.
6. 최종 보완 — on-demand 연결을 자동 수집 지연으로 표시하지 않도록 수정, 문서와 검증 기록 갱신.

현재 live Loki/Tempo 서버와 실제 Windows 자격증명에 대한 검증은 수행하지 않았다. fixture 통과를 제품 버전 전체의 호환성 보장으로 해석하지 않는다.
