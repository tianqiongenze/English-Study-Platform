// ESP monorepo ESLint flat config(全仓统一,packages 共享包最严格,apps 放宽 Taro/JS 配置文件)
import js from "@eslint/js";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";

export default tseslint.config(
  { ignores: ["**/dist/**", "**/build/**", "**/.taro/**", "**/node_modules/**", "**/*.config.js", "**/*.d.ts"] },
  {
    files: ["apps/*/src/**/*.{ts,tsx}", "packages/*/src/**/*.{ts,tsx}"],
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": "off",
      "@typescript-eslint/no-unused-vars": ["warn", { argsIgnorePattern: "^_", varsIgnorePattern: "^_" }],
      "@typescript-eslint/no-explicit-any": "warn",
      "no-empty": ["error", { allowEmptyCatch: true }],
    },
  },
  {
    // 小程序 wx.* / Taro 全局与宽松风格
    files: ["apps/miniprogram/src/**/*.{ts,tsx}"],
    languageOptions: {
      globals: {
        wx: "readonly",
        TARO_APP_API: "readonly",
        TARO_APP_WS: "readonly",
      },
    },
    rules: {
      "@typescript-eslint/no-explicit-any": "off",
    },
  },
  {
    // 测试文件放宽
    files: ["**/*.test.{ts,tsx}"],
    rules: {
      "@typescript-eslint/no-explicit-any": "off",
    },
  }
);
