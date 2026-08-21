import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';

// =============================================================================
// ESP BFF GraphQL vs pre-GraphQL REST 聚合性能对比压测(Phase1 Task C,2026-08-21)
// =============================================================================
//
// 业务场景:"学生首页一屏取数" = 排行榜三件套(rankings + gap + regions)
//           + 学情两件套(reviewList + weakPoints),共 5 个下游数据片。
//
// 对比:
//   - scenario `graphql`:1 次 POST /graphql,聚合 5 个字段(5 下游 RTT → 1 客户端 RTT)
//   - scenario `rest`:5 次串行 GET(bff-java 经 RouteTable 转发下游,客户端 5 次 RTT)
//
// 端点(经源码 + AuthFilter + ingress 核实,见 report/loadtest/graphql-vs-rest-plan.md):
//   - GraphQL:POST http://bff-java:8090/graphql(spring-graphql 2.0.4,MVC 8090)
//       · AuthFilter 校验 JWT(RS256,subject=userId),非白名单,必须 Bearer
//       · schema:rankings/rankingGap/rankingRegions/reviewList/weakPoints(@auth)
//   - REST 等价(经 bff-java:8090 RouteTable,ingress /api/** 收敛):
//       · GET /api/gamification/rankings?type=xp&scope=global&scope_id=all&period=weekly&limit=20
//       · GET /api/gamification/rankings/gap?type=xp&scope=global&scope_id=all&period=weekly
//       · GET /api/gamification/rankings/regions
//       · GET /api/learning/review
//       · GET /api/learning/mastery/weak
//
// 鉴权:bff RS256 私钥在 K8s Secret,无法在 k6 内重签;故复用 BFF register/login
//   端点(白名单免鉴权)拿真实 JWT,与生产链路完全一致(AuthFilter parse + X-User-Id 注入)。
//   每 VU 独立邮箱避并发冲突(register 幂等,已存在则 login 回落)。
//
// 阶梯负载:10→50→100→200 VU,每档 30s(单节点铁律:≤200 防 OOM,见 README)。
//
// 用法(集群内 k6 pod,ClusterIP 直连,绕过 host port-forward):
//   kubectl -n esp-prod run k6-gql-vs-rest --rm -i --restart=Never \
//     --image=grafana/k6:0.54.0 --command -- k6 run - < deploy/k6/graphql-vs-rest.js
// 参数覆盖:
//   -e BFF=http://bff-java:8090 -e STAGE_VUS_1=10 -e STAGE_VUS_2=50 ...
//
// 单节点限制:压测机入集群(同 VM)= 单节点唯一正确姿势;AI 长尾 HPA 跟不上
// (esp-loadtest-probe-pitfalls),故本脚本只打聚合读路径(非 AI 推理),避开长尾。

const BFF = __ENV.BFF || 'http://bff-java:8090';
const EMAIL_PREFIX = `k6gql_${Date.now()}_`;

// 自定义指标:按 scenario 拆分端到端耗时,直接对比聚合 vs 串行往返
const graphqlDuration = new Trend('graphql_screen_duration_ms', true); // GraphQL 单请求一屏取数耗时
const restScreenDuration = new Trend('rest_screen_duration_ms', true);  // REST 5 串行一屏取数总耗时
const authErrors = new Counter('auth_errors');
const graphqlErrors = new Counter('graphql_errors');
const restErrors = new Counter('rest_errors');

// 聚合 GraphQL query:5 字段一次取全(对应 5 个 REST 端点)
const GRAPHQL_QUERY = `query StudentHomeScreen {
  rankings(type: "xp", scope: "global", scopeId: "all", period: "weekly", limit: 20) {
    entries { rank userId name score }
    mine { userId rank score gapToTop }
    asOf
  }
  rankingGap(type: "xp", scope: "global", scopeId: "all", period: "weekly") {
    myRank myScore total gapToTop gapToPrev percentile optedOut
  }
  rankingRegions { code label }
  reviewList { kpId masteryScore masteryInt nextReviewAt }
  weakPoints { kpId masteryScore masteryInt nextReviewAt }
}`;

export const options = {
  scenarios: {
    graphql: {
      executor: 'ramping-vus',
      exec: 'graphqlScreen',
      startVUs: 0,
      stages: [
        { duration: '30s', target: parseInt(__ENV.STAGE_VUS_1 || '10') },
        { duration: '30s', target: parseInt(__ENV.STAGE_VUS_2 || '50') },
        { duration: '30s', target: parseInt(__ENV.STAGE_VUS_3 || '100') },
        { duration: '30s', target: parseInt(__ENV.STAGE_VUS_4 || '200') },
        { duration: '10s', target: 0 },
      ],
      gracefulStop: '5s',
    },
    rest: {
      executor: 'ramping-vus',
      exec: 'restScreen',
      startVUs: 0,
      startTime: '150s', // 与 graphql 错峰串行(单节点铁律:禁两 scenario 并行抢 CPU)
      stages: [
        { duration: '30s', target: parseInt(__ENV.STAGE_VUS_1 || '10') },
        { duration: '30s', target: parseInt(__ENV.STAGE_VUS_2 || '50') },
        { duration: '30s', target: parseInt(__ENV.STAGE_VUS_3 || '100') },
        { duration: '30s', target: parseInt(__ENV.STAGE_VUS_4 || '200') },
        { duration: '10s', target: 0 },
      ],
      gracefulStop: '5s',
    },
  },
  thresholds: {
    // GraphQL 单请求 P95 应 < REST 5 端点串行总耗时 P95(聚合减 RTT 净收益)
    // 注意:仅聚合读路径成立;若下游本身慢,GraphQL 同步阻塞(block)反而放大长尾。
    'graphql_screen_duration_ms': ['p(95)<3000'],
    'rest_screen_duration_ms': ['p(95)<5000'],
    // 成功率门槛(单节点+同VM压测机,低于生产)
    'http_req_failed{screen:graphql}': ['rate<0.05'],
    'http_req_failed{screen:rest}': ['rate<0.05'],
  },
};

// 每 VU 独立账号拿 token(register 幂等,已存在则 login 回落)
function getToken(vuId) {
  const email = `${EMAIL_PREFIX}${vuId}@loadtest.com`;
  const regRes = http.post(
    `${BFF}/api/auth/register`,
    JSON.stringify({ email, password: 'test123', name: `k6gql-${vuId}`, role: 'learner' }),
    { headers: { 'Content-Type': 'application/json' }, tags: { screen: 'auth' } }
  );
  if (regRes.status === 200 && regRes.json('token')) {
    return regRes.json('token');
  }
  const loginRes = http.post(
    `${BFF}/api/auth/login`,
    JSON.stringify({ email, password: 'test123' }),
    { headers: { 'Content-Type': 'application/json' }, tags: { screen: 'auth' } }
  );
  if (loginRes.status === 200 && loginRes.json('token')) {
    return loginRes.json('token');
  }
  authErrors.add(1);
  return null;
}

// REST 5 端点串行一屏取数(客户端 5 次 RTT)
function restScreenFiveEndpoints(token) {
  const headers = { Authorization: `Bearer ${token}` };
  const t0 = Date.now();
  let failed = false;

  const r1 = http.get(
    `${BFF}/api/gamification/rankings?type=xp&scope=global&scope_id=all&period=weekly&limit=20`,
    { headers, tags: { screen: 'rest', rest_ep: 'rankings' } }
  );
  if (r1.status !== 200) { restErrors.add(1); failed = true; }

  const r2 = http.get(
    `${BFF}/api/gamification/rankings/gap?type=xp&scope=global&scope_id=all&period=weekly`,
    { headers, tags: { screen: 'rest', rest_ep: 'gap' } }
  );
  if (r2.status !== 200) { restErrors.add(1); failed = true; }

  const r3 = http.get(
    `${BFF}/api/gamification/rankings/regions`,
    { headers, tags: { screen: 'rest', rest_ep: 'regions' } }
  );
  if (r3.status !== 200) { restErrors.add(1); failed = true; }

  const r4 = http.get(
    `${BFF}/api/learning/review`,
    { headers, tags: { screen: 'rest', rest_ep: 'review' } }
  );
  if (r4.status !== 200) { restErrors.add(1); failed = true; }

  const r5 = http.get(
    `${BFF}/api/learning/mastery/weak`,
    { headers, tags: { screen: 'rest', rest_ep: 'weak' } }
  );
  if (r5.status !== 200) { restErrors.add(1); failed = true; }

  const totalMs = Date.now() - t0;
  restScreenDuration.add(totalMs);

  check(totalMs, {
    'rest screen 5 endpoints completed': () => true,
  });
  check(!failed, {
    'rest screen all 5 endpoints 200': (ok) => ok,
  });
}

export function graphqlScreen() {
  const token = getToken(__VU);
  if (!token) { sleep(0.5); return; }

  const res = http.post(
    `${BFF}/graphql`,
    JSON.stringify({ query: GRAPHQL_QUERY }),
    {
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      tags: { screen: 'graphql' },
    }
  );

  graphqlDuration.add(res.timings.duration);

  const ok = check(res, {
    'graphql 200': (r) => r.status === 200,
    'graphql no errors': (r) => {
      try {
        const body = r.json();
        return !body.errors || body.errors.length === 0;
      } catch (e) {
        return false;
      }
    },
    'graphql has data': (r) => {
      try {
        const body = r.json();
        return body.data && (body.data.rankings || body.data.reviewList || body.data.weakPoints);
      } catch (e) {
        return false;
      }
    },
  });
  if (!ok) graphqlErrors.add(1);
}

export function restScreen() {
  const token = getToken(__VU);
  if (!token) { sleep(0.5); return; }
  restScreenFiveEndpoints(token);
}

// 空默认函数(两个 scenario 各有 exec,不需要 default;留空避免 k6 警告)
export default function () {}
