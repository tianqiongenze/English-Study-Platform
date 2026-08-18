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
  // content 统一内容库(功能拓展阶段1)
  listCourses: client.listCourses.bind(client),
  getCourse: client.getCourse.bind(client),
  listLessons: client.listLessons.bind(client),
  listItems: client.listItems.bind(client),
  submitAnswer: client.submitAnswer.bind(client),
  createCourse: client.createCourse.bind(client),
  updateCourse: client.updateCourse.bind(client),
  createLesson: client.createLesson.bind(client),
  createItem: client.createItem.bind(client),
  updateItem: client.updateItem.bind(client),
  importContent: client.importContent.bind(client),
  // community 圈子社区(功能拓展阶段2)
  createCircle: client.createCircle.bind(client),
  listCircles: client.listCircles.bind(client),
  getCircle: client.getCircle.bind(client),
  listCircleMembers: client.listCircleMembers.bind(client),
  joinCircle: client.joinCircle.bind(client),
  leaveCircle: client.leaveCircle.bind(client),
  createPost: client.createPost.bind(client),
  listPosts: client.listPosts.bind(client),
  createComment: client.createComment.bind(client),
  listComments: client.listComments.bind(client),
  reportContent: client.reportContent.bind(client),
  // gamification 排行榜(功能拓展阶段3)
  rankings: client.rankings.bind(client),
  rankingGap: client.rankingGap.bind(client),
  rankingRegions: client.rankingRegions.bind(client),
  setRankingOptOut: client.setRankingOptOut.bind(client),
  // 数字人 persona(功能拓展:per-user 形象,X-User-Id 由 BFF 注入)
  getDigitalHuman: () => client.getDigitalHuman(),
  setDigitalHuman: (persona_id: string) => client.setDigitalHuman(persona_id),
};
