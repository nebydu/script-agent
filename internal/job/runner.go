// Package job는 BE로부터 받은 Command를 dispatch하고 실제 Job
// (SCRIPT_JOB / LOG_JOB)을 실행한다. spec §4 / §5.1 / §5.2 참조.
package job

import (
	"context"

	"monitoring/script-agent/internal/model"
)

// Runner는 한 가지 JobType의 Job을 실행한다.
//   - JobTypeScript → ScriptRunner (단계 D)
//   - JobTypeLog    → LogRunner    (단계 E)
//
// Run의 책임:
//   - JobResult를 spec §5.2 형식대로 채워 반환 (agent_id, execution_id,
//     job_type, status, started_at/finished_at, script|log 분기 포함).
//   - JOB_EXECUTED audit의 target 정보(SCRIPT/LOG_FILE + 경로)도 함께 반환.
//   - 비즈니스 레벨 실패(스크립트 종료 코드 != 0, 패턴 매칭 실패 등)는
//     JobResult.Status = FAIL로 보고하고 nil error 반환.
//   - 시스템 레벨 실패 (ctx 취소 등)만 error로 반환. Dispatcher는 이때
//     발행을 생략한다.
type Runner interface {
	Run(ctx context.Context, agentID string, cmd model.Command) (model.JobResult, model.Target, error)
}
