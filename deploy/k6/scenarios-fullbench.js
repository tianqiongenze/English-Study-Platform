// scenarios-fullbench.js — ESP 全服务×全语言端点基准(k6,集群内 ClusterIP 直连)
// 每个 k6 run 只测一个端点(-e EP=xxx),constant-vus 稳态 45s,产出该端点纯 RPS/P95。
// 与 report/crdb_fullbench.py 同端点模型(c20/c60)可比;重叠端点(core 5)用于 CRDB-vs-旧TiDB 对照。
//
// 用法(k6 Job,deploy/k6/k6.yaml):
//   kubectl -n esp-prod create job --from=cronjob/...   # 或 kubectl apply 后按需打 job
// 参数:EP=端点名,VUS=并发数。JWT 从 env 注入(esp-bench-tokens configmap):
//   LEARNER_TOKENS(逗号分隔 RS256 learner token,签 sub=uid,复用全量测试预签)
//   TEACHER_TOKEN(teacher 角色 token)
import http from 'k6/http';
import { check } from 'k6';

const EP = __ENV.EP || 'gw_scenarios';
const VUS = parseInt(__ENV.VUS || '20');
const DUR = __ENV.DUR || '45s';
const TOKENS = (__ENV.LEARNER_TOKENS || '').split(',').filter(Boolean);
const TEACHER = __ENV.TEACHER_TOKEN || '';

// 预签 token 数(默认 20 个 learner)。uid 从 token sub 解析失败则按 token 下标推断。
const NTOK = TOKENS.length || 1;

// 每端点一个 unique 指标名,便于 CSV 提取 RPS/P95
import { Trend, Rate } from 'k6/metrics';
const dur = new Trend('bench_duration_ms', true);
const fail = new Rate('bench_failed');

export const options = {
  scenarios: {
    bench: { executor: 'constant-vus', vus: VUS, duration: DUR, gracefulStop: '10s' },
  },
  thresholds: {
    // 单节点(9核/30G,loadavg~24)降级门槛:成功率≥95%,P95<5s(LLM 端点另行放宽)
    'http_req_failed': ['rate<0.05'],
    'bench_duration_ms': ['p(95)<5000'],
  },
};

function authHeader(i) {
  const t = TOKENS[i % NTOK];
  return t ? { Authorization: `Bearer ${t}` } : {};
}
function uidOf(i) {
  // token sub=uid 时由 token 决定;此处按 NTOK 分布取 10000100+i
  return 10000100 + i;
}

export default function () {
  const i = __VU - 1; // 0..VUS-1
  const uid = uidOf(i);
  const H = { 'Content-Type': 'application/json' };
  let url, method, body, headers, checkName;

  switch (EP) {
    // ---- core 5(与旧 fullbench 重叠,CRDB-vs-旧TiDB 对照) ----
    case 'learning_event':
      method = 'POST'; url = 'http://learning-java:80/api/learning/event';
      headers = { ...authHeader(i), 'Content-Type': 'application/json' };
      body = JSON.stringify({ user_id: uid, event_type: 'practice_attempt', payload: JSON.stringify({ kp_id: (uid % 100) + 1, correct: true }) });
      checkName = 'event 202/200';
      break;
    case 'learning_review':
      method = 'GET'; url = `http://learning-java:80/api/learning/review?userId=${uid}`;
      headers = authHeader(i);
      checkName = 'review 200';
      break;
    case 'teacher_kpstats':
      method = 'GET'; url = `http://teacher-java:80/api/teacher/${uid}/kp-stats`;
      headers = { Authorization: `Bearer ${TEACHER}` };
      checkName = 'kpstats 200';
      break;
    case 'bff_scenarios':
      method = 'GET'; url = 'http://bff-java:8090/api/scenarios';
      headers = authHeader(i);
      checkName = 'bff scenarios 200';
      break;
    case 'gw_scenarios':
      method = 'GET'; url = 'http://gateway-go:80/api/scenarios';
      headers = authHeader(i);
      checkName = 'gw scenarios 200';
      break;

    // ---- full matrix(全服务×全语言) ----
    case 'bff_login':
      method = 'POST'; url = 'http://bff-java:8090/api/auth/login';
      headers = H;
      body = JSON.stringify({ email: `bench_${(i % 5) + 1}@esp.test`, password: 'test123' });
      checkName = 'login 200';
      break;
    case 'reading_passages':
      method = 'GET'; url = 'http://reading-go:80/api/reading/passages?level=B1&limit=20';
      headers = {}; checkName = 'reading passages 200';
      break;
    case 'listening_mat':
      method = 'GET'; url = 'http://listening-go:80/api/listening/materials?limit=20';
      headers = {}; checkName = 'listening materials 200';
      break;
    case 'gamification_prof':
      method = 'GET'; url = `http://gamification-go:80/api/gamification/profile/${uid}`;
      headers = {}; checkName = 'gamification profile 200';
      break;
    case 'gamification_pts':
      method = 'POST'; url = 'http://gamification-go:80/api/gamification/points';
      headers = H; body = JSON.stringify({ user_id: uid, points: 5, reason: 'bench' });
      checkName = 'gamification points 200';
      break;
    case 'vocab_due':
      method = 'GET'; url = `http://vocab-srs-go:80/api/vocab/due?uid=${uid}&limit=20`;
      headers = {}; checkName = 'vocab due 200';
      break;
    case 'vocab_review':
      method = 'POST'; url = 'http://vocab-srs-go:80/api/vocab/review';
      headers = H; body = JSON.stringify({ user_id: uid, word_id: (uid % 50) + 1, quality: 3 });
      checkName = 'vocab review 200';
      break;
    case 'emotion_recent':
      method = 'GET'; url = `http://emotion-svc:80/api/emotion/recent/${uid}`;
      headers = { 'X-User-Id': String(uid) };
      checkName = 'emotion recent 200';
      break;
    case 'writingcoach_tpl':
      method = 'GET'; url = 'http://writing-coach:80/api/writing-coach/templates?category=IELTS';
      headers = {}; checkName = 'writingcoach templates 200';
      break;
    case 'assessment_report':
      method = 'GET'; url = `http://assessment-rs:80/api/assessment/report/${uid}`;
      headers = {}; checkName = 'assessment report 200';
      break;
    case 'llm_dialogue':
      method = 'GET'; url = `http://llm-orchestrator-rs:80/api/dialogue/sessions?user_id=${uid}&limit=20`;
      headers = {}; checkName = 'llm dialogue sessions 200';
      break;
    case 'course_path':
      method = 'GET'; url = `http://course-path-go:80/api/path/?user_id=${uid}`;
      headers = {}; checkName = 'course path 200';
      break;
    case 'writing_grade':
      // LLM 绑定端点:单请求秒级,仅低并发定性;RPS 表单独标注
      method = 'POST'; url = 'http://writing-py:80/api/writing/grade';
      headers = { 'X-User-Id': String(uid), 'Content-Type': 'application/json' };
      body = JSON.stringify({ prompt: 'Write an essay about learning English.', essay: 'This is a benchmark essay about learning English effectively.', exam: 'ielts', user_id: uid });
      checkName = 'writing grade 200';
      break;
    default:
      method = 'GET'; url = 'http://gateway-go:80/api/scenarios';
      headers = authHeader(i); checkName = 'default gw scenarios';
  }

  let res;
  if (method === 'POST') {
    res = http.post(url, body, { headers, tags: { endpoint: EP } });
  } else {
    res = http.get(url, { headers, tags: { endpoint: EP } });
  }
  dur.add(res.timings.duration, { endpoint: EP });
  fail.add(res.status >= 400, { endpoint: EP });
  const ok = res.status >= 200 && res.status < 300;
  check(res, { [checkName]: () => ok });
}
