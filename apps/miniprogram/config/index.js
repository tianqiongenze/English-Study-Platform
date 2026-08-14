// ESP 小程序端 Taro 配置。生产需 wss+备案域名(ADR-14)。
// 网关地址:开发期用本机 IP:8090(BFF)/ :8080(gateway-go WS);生产替换为正式域名。
const BASE_HTTP = process.env.TARO_APP_API || "http://localhost:8090";
const BASE_WS = process.env.TARO_APP_WS || "ws://localhost:8080";

const config = {
  projectName: "esp-miniprogram",
  date: "2026-7-31",
  designWidth: 750,
  deviceRatio: { 640: 2.34 / 2, 750: 1, 828: 1.81 / 2 },
  sourceRoot: "src",
  outputRoot: "dist",
  plugins: [],
  defineConstants: { TARO_APP_API: JSON.stringify(BASE_HTTP), TARO_APP_WS: JSON.stringify(BASE_WS) },
  copy: { patterns: [], options: {} },
  framework: "react",
  compiler: "webpack5",
  mini: {
    postcss: {
      pxtransform: { enable: true, config: {} },
      url: { enable: true, config: { limit: 1024 } },
      cssModules: { enable: false, config: { namingPattern: "module", generateScopedName: "[name]__[local]___[hash:base64:5]" } },
    },
  },
  h5: {
    publicPath: "/", staticDirectory: "static",
    miniCssExtractPluginOption: { ignoreOrder: true, filename: "css/[name].[hash].css", chunkFilename: "css/[name].[chunkhash].css" },
  },
};
module.exports = config;
