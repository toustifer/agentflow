import { handleDagRequest } from './packages/dsh-interactive-spec/src/routes';
import { test } from 'vitest';

test('reproduce handleDagRequest', async () => {
  const req = new Request('http://127.0.0.1:3080/api/agentflow/dag?cwd=D:%5Cmyprogram%5CInsightTutor&dag_id=tutor_monitor_web_v1');
  const res = await handleDagRequest(req);
  const data = await res.json();
  console.log('handleDagRequest data:', data);
});
