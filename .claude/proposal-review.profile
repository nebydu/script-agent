# proposal-review.profile — script-agent consumer 델타 (H6)
# /proposal-review command가 Codex 리뷰에 주입할 script-agent 문맥. 도메인 결정은 이 파일에만 둔다.
# 골격: monitoring-harness plugin shared/analysis/proposal-review-runner.sh
#
# 적용 조건: harness a455246 이상(runner의 git rev-parse fallback 포함) — 그 이전 캐시에서는
#   command 컨텍스트가 이 profile을 못 찾아 degraded(문맥 없는 리뷰)로 동작한다.
# drift 완화: 기준 문서를 추가/이동하는 작업의 DoD에 "이 profile 문맥 목록 갱신"을 포함한다.
# dry-run: 이 repo에서 `/proposal-review` 호출 → 출력 JSON context 필드가
#   "profile: .../proposal-review.profile"이면 주입 성공, "none"이면 degraded.

# repo 루트 기준 절대경로로 해석 (호출 cwd 무관하게 동작)
# - git rev-parse는 cwd 의존이라 repo 밖 cwd에서 source 시 즉사, 다른 repo cwd에서는
#   엉뚱한 root로 조용히 진행하는 결함이 있었다 → profile 파일 위치 기준으로 산출.
# - 의미: 이 파일이 놓인 디렉터리(.claude/)의 부모 = repo root. convention 위치를
#   벗어난 곳에 profile을 두면 그 위치 기준으로 해석된다. (infra b47fcef와 동일 조치)
_SA_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# 문맥 문서 — ground truth 우선순위(.claude/CLAUDE.md §2)를 반영.
# 통합본 master-design.md(170KB)는 매 리뷰 주입 비용이 커서 제외(아래 POLICY에서 안내).
# ../monitoring-meta 형제 경로는 workspace 배치 의존 — 없으면 runner가 warn 후 건너뛴다.
PROPOSAL_REVIEW_CONTEXT_DOCS=(
  "$_SA_ROOT/README.md"
  "$_SA_ROOT/AGENTS.md"
  "$_SA_ROOT/../monitoring-meta/docs/phase0-snapshot/monitoring-demo-message-spec-v0.2.1.md"
  "$_SA_ROOT/../monitoring-meta/docs/kafka-payloads.md"
  "$_SA_ROOT/../monitoring-meta/docs/envelope.md"
)

PROPOSAL_REVIEW_POLICY="script-agent는 Go 런타임 코드 repo다. 결정 리뷰 시 지킬 기준: 방향 판단의 최상위 기준은 통합본(이 입력에 미포함, 170KB라 제외 — ../monitoring-meta/docs/master-design.md)이고, 데모 spec v0.2.1은 Phase 0 회귀 방지 가드다. 제안이 'Phase 0 유지'인지 'Phase 1+ 선반영'인지 분류하지 않았으면 결함으로 지적하라. 통합본의 Phase 1+ 목표를 Phase 0 코드에 무조건 강제하는 제안, [Open]/미결정 ADR을 결정된 것으로 전제한 제안, 형제 repo(hub/monitoring-meta/infra)를 script-agent가 직접 수정하는 것을 전제한 제안은 block 대상이다. 코드 작업 spec은 ../monitoring-meta/handoff/<work-id>/<work-id>-script-agent.md 경유가 원칙이므로 이를 우회하는 프로세스 제안도 결함으로 지적하라. 통합본이 직접 쟁점인데 발췌가 없으면 missing_context로 지적하라."
