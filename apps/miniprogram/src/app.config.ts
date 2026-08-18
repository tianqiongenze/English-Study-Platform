export default defineAppConfig({
  pages: ["pages/index/index", "pages/login/index", "pages/speech/index", "pages/writing/index", "pages/learning/index"],
  window: {
    backgroundTextStyle: "light",
    navigationBarBackgroundColor: "#1677ff", // 品牌 Daybreak(亮默认);暗 #1668dc 由 app.tsx/theme.ts 运行时 setNavigationBarColor 联动
    navigationBarTitleText: "洁语 · Verity",
    navigationBarTextStyle: "white",
  },
  tabBar: {
    color: "#64748b",
    selectedColor: "#1677ff", // 品牌蓝(原 #0ea5e9)
    backgroundColor: "#ffffff",
    borderStyle: "black",
    // 无自定义图标(weapp tabBar 允许纯文字);文案为语言不可继配置,保持中文
    list: [
      { pagePath: "pages/speech/index", text: "陪练" },
      { pagePath: "pages/learning/index", text: "学习" },
      { pagePath: "pages/index/index", text: "我的" },
    ],
  },
});