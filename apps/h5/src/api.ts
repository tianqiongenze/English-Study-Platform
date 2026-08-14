// H5 端 API 层 —— 薄委托到 @esp/api-client(P2-17)。
// 历史漂移修正:grade/writingRun 原硬编码 exam="toefl",现由调用方显式传参
// (H5 UI 默认 toefl,如需保持行为不变在调用处传 "toefl")。
import { ApiClient } from "@esp/api-client";
import { useAuth } from "./auth";

const storage = {
  getToken: () => useAuth.getState().token,
  setToken: (t: string) => t,
  clearToken: () => useAuth.getState().logout(),
};

export const client = new ApiClient({ storage });

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
  // 显式 exam 参数(原硬编码 toefl,调用方传 "toefl" 即旧行为)
  grade: client.grade.bind(client),
  polish: client.polish.bind(client),
  translate: client.translate.bind(client),
  visualWord: client.visualWord.bind(client),
  senseEmotion: client.senseEmotion.bind(client),
  writingRun: client.writingRun.bind(client),
  writingReview: client.writingReview.bind(client),
  writingState: client.writingState.bind(client),
  reportBehavior: client.reportBehavior.bind(client),
  recommend: client.recommend.bind(client),
};
