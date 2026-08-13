import http from 'k6/http';
import { check, group } from 'k6';
import { Counter, Trend } from 'k6/metrics';

// ESP REST API 基线压测(k6,集群内执行,绕过 host port-forward)。
// 目标:测 gateway-go /api/scenarios 真实吞吐,与 Locust/host port-forward 对照。
//
// 用法(集群内 k6 pod,ClusterIP 直连):
//   kubectl -n esp-prod run k6-baseline --rm -i --restart=Never \
//     --image=grafana/k6:0.54.0 --command -- k6 run - < deploy/k6/scenarios-baseline.js
// 参数覆盖(示例):
//   -e HOST=http://gateway-go:80 -e BFF=http://bff-java:80 -e VUS=100
//
// 单节点铁律:压测机入集群(同 VM)= 单节点唯一正确姿势;VUS 上限勿超 200 防 OOM。

const HOST = __ENV.HOST || 'http://gateway-go:80';
const BFF = __ENV.BFF || 'http://bff-java:80';
const EMAIL = `k6_${__VU}@loadtest.com`; // 每 VU 独立账号,避开并发同邮箱冲突

// 自定义指标:scenarios 端点独立耗时,区分登录与业务
const scenariosDuration = new Trend('scenarios_duration_ms', true);
const loginErrors = new Counter('login_errors');

export const options = {
  scenarios: {
    baseline: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '10s', target: parseInt(__ENV.VUS || '50') },   // 爬坡
        { duration: '30s', target: parseInt(__ENV.VUS || '50') },   // 稳态压测
        { duration: '10s', target: 0 },                              // 回落
      ],
      gracefulStop: '5s',
    },
  },
  thresholds: {
    // 单节点基线:成功率 ≥99%,P95 < 3s(单节点+同VM压测机,门槛低于生产)
    'http_req_failed{endpoint:scenarios}': ['rate<0.01'],
    'scenarios_duration_ms': ['p(95)<3000'],
  },
};

// 每个 VU 启动时注册一次(role 必须 learner/teacher/admin,传 student 会被吞成 "email exists")
export function setup() {
  // setup 只跑一次,用共享账号登录拿 token 模板;真实 VU 内各自注册避免 setup 单点
  return { host: HOST };
}

export default function (data) {
  const host = data.host;
  let token = null;

  group('login', function () {
    // 并发注册:每个 VU 用唯一 __VU 邮箱。register 幂等(已存在则 conflict,改用 login)
    const regRes = http.post(
      `${BFF}/api/auth/register`,
      JSON.stringify({ email: EMAIL, password: 'test123', name: `k6-${__VU}`, role: 'learner' }),
      { headers: { 'Content-Type': 'application/json' }, tags: { endpoint: 'register' } }
    );
    if (regRes.status === 200 && regRes.json('token')) {
      token = regRes.json('token');
    } else {
      // 已注册过 → 登录
      const loginRes = http.post(
        `${BFF}/api/auth/login`,
        JSON.stringify({ email: EMAIL, password: 'test123' }),
        { headers: { 'Content-Type': 'application/json' }, tags: { endpoint: 'login' } }
      );
      if (loginRes.status === 200 && loginRes.json('token')) {
        token = loginRes.json('token');
      } else {
        loginErrors.add(1);
      }
    }
  });

  if (!token) return; // 登录失败跳过本轮

  // 稳态:反复打 scenarios(幂等只读,安全)
  const res = http.get(`${host}/api/scenarios`, {
    headers: { Authorization: `Bearer ${token}` },
    tags: { endpoint: 'scenarios' },
  });
  scenariosDuration.add(res.timings.duration);

  check(res, {
    'scenarios 200': (r) => r.status === 200,
    'scenarios has body': (r) => r.body && r.body.length > 0,
  });
}
