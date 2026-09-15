import { getDagDetail } from './packages/dsh-interactive-spec/src/history-service.ts';

const res = getDagDetail('tutor_monitor_web_v1', 'D:\\myprogram\\InsightTutor');
console.log('Result:', JSON.stringify(res, null, 2));
