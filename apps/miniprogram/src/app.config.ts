export default defineAppConfig({
  pages: ["pages/index/index", "pages/login/index", "pages/speech/index", "pages/writing/index", "pages/learning/index"],
  window: { backgroundTextStyle: "light", navigationBarBackgroundColor: "#0f172a", navigationBarTitleText: "Home-Tutor", navigationBarTextStyle: "white" },
  tabBar: {
    color: "#64748b",
    selectedColor: "#0ea5e9",
    backgroundColor: "#ffffff",
    borderStyle: "black",
    list: [
      { pagePath: "pages/speech/index", text: "陪练" },
      { pagePath: "pages/learning/index", text: "学习" },
      { pagePath: "pages/index/index", text: "我的" },
    ],
  },
});
