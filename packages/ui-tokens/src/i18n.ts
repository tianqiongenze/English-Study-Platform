// 极简 i18n(en/zh):零依赖,类型安全 key。
// 三端共用文案真源;应用各自持久化语言偏好。

export type Locale = "en" | "zh";

const messages = {
  en: {
    "app.title": "Home Tutor",
    "auth.login": "Login",
    "auth.register": "Register",
    "auth.logout": "Logout",
    "auth.email": "Email",
    "auth.password": "Password",
    "auth.name": "Name",
    "nav.speaking": "Speaking",
    "nav.writing": "Writing",
    "nav.learning": "Learning",
    "nav.teacher": "Teacher",
    "action.submit": "Submit",
    "action.cancel": "Cancel",
    "action.retry": "Retry",
    "action.loading": "Loading...",
    "error.network": "Network error, please retry",
    "error.unauthorized": "Please login first",
    "result.score": "Score",
    "result.feedback": "Feedback",
    "scenarios.title": "Pick a scenario",
    "scenarios.start": "Start session",
    "writing.grade": "Grade my essay",
    "writing.polish": "Polish",
    "writing.translate": "Translate",
  },
  zh: {
    "app.title": "AI 英语家教",
    "auth.login": "登录",
    "auth.register": "注册",
    "auth.logout": "退出登录",
    "auth.email": "邮箱",
    "auth.password": "密码",
    "auth.name": "姓名",
    "nav.speaking": "口语陪练",
    "nav.writing": "写作批改",
    "nav.learning": "学习",
    "nav.teacher": "教师",
    "action.submit": "提交",
    "action.cancel": "取消",
    "action.retry": "重试",
    "action.loading": "加载中...",
    "error.network": "网络错误,请重试",
    "error.unauthorized": "请先登录",
    "result.score": "得分",
    "result.feedback": "反馈",
    "scenarios.title": "选择场景",
    "scenarios.start": "开始会话",
    "writing.grade": "批改作文",
    "writing.polish": "润色",
    "writing.translate": "翻译",
  },
} as const;

export type MessageKey = keyof (typeof messages)["en"];

export function createI18n(initial: Locale = "zh") {
  let locale: Locale = initial;
  return {
    get locale(): Locale {
      return locale;
    },
    setLocale(l: Locale): void {
      locale = l;
    },
    t(key: MessageKey): string {
      return messages[locale][key] ?? messages.en[key] ?? key;
    },
  };
}
