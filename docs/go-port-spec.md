# Go alpha scope and acceptance

The user requested an MIT public Go replacement in
`wooto/lol-replay-recorder`, initially concentrating on Windows player-POV
recording. Subsequent research expanded the requested library to collecting
live observer data from game start and serving it later, independently of
OP.GG. The user explicitly authorized publishing the library and requested
recent implementation references and multiple Luna agents at max effort.

## Deliverables

- Preserve the implemented Windows local-ROFL `recorder.RecordFull` API.
- Add an experimental `observer` package for bounded opaque chunk/keyframe
  downloads, durable custom archives, cancellation, and explicit incomplete
  results. Never overwrite an existing archive destination.
- Mark an archive complete only when an observed game-end marker and the
  expected contiguous payload ranges agree. Reject contradictory positive
  end-chunk/end-keyframe identifiers, malformed IDs, missing payloads, and
  unsupported shapes. Ordinary metadata may be cached: a false `gameEnded` flag
  or lower `last*Id` does not override an explicit final chunk snapshot.
- Expose a caller-owned HTTP replay handler only for completed validated
  archives. Preserve original payload bytes and known metadata values; do not
  invent keyframe-to-chunk mappings or use per-IP playback counters.
- Optional `discovery` resolves a caller-selected Riot ID and waits for its
  observable active game through Riot APIs. Caller supplies its own API key.
  Respect rate-limit waits and fail promptly on authorization errors.
- Provide examples for local ROFL recording, observer collection, and a
  numeric-loopback replay server. No implicit account login, OP.GG integration,
  executable downloads, game-memory access, upload pipeline, or player crawler.
- Maintain standard-library-only Go dependencies, MIT attribution, and Windows
  minimum/stable Go checks plus Linux race/vulnerability checks.
- Publish `v0.1.0-alpha.1` on the public repository after tests and review.
  Stable publication remains gated on live acceptance. Do not label this alpha
  as current-patch KR-compatible, a FULL observer-to-video pipeline, or approved
  by Riot. Archive completion means observed protocol coverage, not proof of
  match-time-zero coverage or actual game-client playback.

## Verification

Use public HTTP/archive boundaries with deterministic fixtures to verify
payload preservation, live progress, late data availability, missing final
data, contradictory end markers, cancellations, input bounds, unsafe paths,
wrong game identity, method/route handling, and independent concurrent clients.
Check the complete module with vet, build, formatting, Windows tests, Linux
race checks, and vulnerability scanning. Confirm the published tag is fetchable
as a Go module. Live acceptance remains explicitly pending.

## 관전 단축키 사전 점검 및 설정 요구사항

녹화 준비에 관전 단축키 점검을 포함한다. 기본 동작은 읽기 전용이며, 자동 설정은 호출자가 명시적으로 활성화했을 때만 수행한다. 설정 연동은 앱/별도 어댑터에서 맡아 녹화 코어가 LCU 로그인이나 특정 외부 사이트에 종속되지 않도록 한다.

1. 실제 적용된 관전 선수 슬롯 키를 읽고 `Config.SelectionKeys`와 비교한다. 공식 기본값은 ORDER 1–5, CHAOS Q/W/E/R/T이다. 설정에 항목이 없거나 설정 API에 관전 그룹이 없으면 검증되지 않은 기본값으로 표시한다. 정상으로 간주하지 않는다.
2. 중복·미지정·충돌 키와 지원하지 않는 조합을 검사한다. Ctrl+1 등의 일반 플레이 키와 관전 선택 키를 구분한다. 현재 SelectionKeys는 단일 Windows VK만 지원한다.
3. 자동 설정 모드에서는 확인된 관전 키 항목만 변경한다. 원본 백업을 먼저 확보하고 전체 단축키 초기화는 하지 않는다. 적용 후 다시 읽어 검증한다. 적용에 재시작이 필요하면 새 리플레이 실행 전에 처리한다. 현재 실행 중인 사용자 소유 게임에는 적용하지 않는다.
4. 설정 읽기 검증과 실제 입력 검증은 별도로 수행한다. 소유한 리플레이 창에서 해당 슬롯 키를 두 번 입력하고, 대상 및 기본 관전 추적이 작동하는지 확인한다. 설정 저장이나 Windows 입력 개수만으로 카메라 포커싱 성공을 선언하지 않는다.
5. 설정을 검증할 수 없거나 입력·대상 포커싱이 실패하면 녹화를 시작하지 않고 원인별 오류를 반환한다. 현재 FPS attachment readback을 기본 관전 포커싱 증거로 사용하지 않는다.

테스트는 합의된 RecordFull/실행 훅/HTTP API 경계에서 한 RED→GREEN씩 추가한다. 불일치 시 실행 금지, 자동 설정 후 재조회, 적용 실패, 취소, 기존 설정 보존을 검증한다. 입력 도구 오류 87은 단축키 설정 변경으로 해결됐다고 주장하지 않는다.

아직 현재 패치의 관전 설정 이벤트 이름과 쓰기/재적용 경로는 확인되지 않았다. 이 요구사항은 구현 계약이며 해당 경로를 추측하여 설정 파일을 수정하지 않는다.

공식 기준: https://support.riotgames.com/en-us/league-of-legends/gameplay/replays-faq-pro-tips
