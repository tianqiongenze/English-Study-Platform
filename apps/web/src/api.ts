// Web 端 API 层 —— 薄委托到 @esp/api-client(P2-17 monorepo 统一 SDK)。
// 本地签名保持组件兼容;签名漂移处(如 register role)由 SDK 统一。
// P1-13:token 改 httpOnly cookie(服务端 Set-Cookie 下发),前端不注入 Authorization header,
// 同源请求/WS 握手由浏览器自动携带 esp_token cookie(BFF/网关侧已兼容 cookie 验签)。
import { ApiClient, CookieTokenStorage } from "@esp/api-client";

export const client = new ApiClient({ storage: new CookieTokenStorage() });

export const api = {
  login: client.login.bind(client),
  register: client.register.bind(client),
  scenarios: client.scenarios.bind(client),
  createSession: client.createSession.bind(client),
  sessionHistory: () => client.sessionHistory(),
  reportEvent: client.reportEvent.bind(client),
  review: client.review.bind(client),
  weak: client.weak.bind(client),
  placement: client.placement.bind(client),
  difficulty: client.difficulty.bind(client),
  grade: client.grade.bind(client),
  polish: client.polish.bind(client),
  translate: client.translate.bind(client),
  lessonPlan: client.lessonPlan.bind(client),
  classAnalytics: client.classAnalytics.bind(client),
  autoGrade: client.autoGrade.bind(client),
  finalize: client.finalize.bind(client),
  senseEmotion: client.senseEmotion.bind(client),
  visualWord: client.visualWord.bind(client),
  writingRun: client.writingRun.bind(client),
  writingReview: client.writingReview.bind(client),
  writingState: client.writingState.bind(client),
  reportBehavior: client.reportBehavior.bind(client),
  recommend: client.recommend.bind(client),
};
