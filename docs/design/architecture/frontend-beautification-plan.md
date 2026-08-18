# ESP 前端全面美化方案(2026-08-17)

> 用户指令:现有前端"只有纯配色没图片、主题不可切换、无 i18n",不达大厂标准。要求先联网研究"最好最美最吸引各类人群"方案再重构。
> 研究输入:①联网设计/主题/图片调研(antd v6 CSS变量+darkAlgorithm 已核实;Unsplash/Pexels 无 key 直链;lucide/DiceBear 无 key)②联网 i18n 调研(react-i18next 统一栈)③代码库现状探查(4 端技术栈 + ui-tokens 资产闲置)。
> 本方案为权威落地稿,重构子任务按端拆分派 agent 执行。

## 0. 现状基线(探查实测)

| 端 | 目录 | 栈 | 现状 |
|---|---|---|---|
| Web | apps/web | React19 + Vite7 + router7,无 UI 库 | styles.css 31 行硬编码 slate;182 处 hex;0 图 0 图标库 0 i18n;366 行硬编码中文 |
| H5 | apps/h5 | React19 + Vite7 + antd-mobile5(仅 Profile 用) | 同 web 的 slate;49 hex;0 图;97 行硬编码中文 |
| 小程序 | apps/miniprogram | Taro4 + React18(weapp+h5) | tabBar+5 薄页;0 图 0 图标;inline style 散落 |
| Portal | frontend-portal | 静态 HTML | 已较好(:root 变量+渐变+卡片),无 logo/真二维码 |

**关键资产(已建未用)**:`packages/ui-tokens/tokens.css`(light + `html[data-theme=dark]` 全套)、`packages/ui-tokens/src/i18n.ts`(24 key en/zh)。重构核心 = 把硬编码 slate/中文迁回 tokens + 接入 i18n,再补图片资产与主题切换。

## 1. 三大支柱

### 1.1 主题系统(3 主题 × 强度 = 可切换)
- **架构**:CSS 变量 + `html[data-theme]`。三端统一读写 `localStorage['esp_theme']`(小程序 `Taro.setStorageSync('esp_theme')`),取值 `light | dark | system`;`system` 跟随 `prefers-color-scheme`。
- **主题集**:①Light(Daybreak)②Dark(Midnight)③加品牌色可换(Coral 珊瑚橙 / Forest 森林绿 / Grape 葡萄紫)。实现:变量 `--esp-brand-*`,`data-accent="coral|forest|grape"` 覆盖,三套即可体感丰富。
- **落地机制**:
  - web(无 UI 库):纯 tokens.css,`ThemeProvider`(读 localStorage→设 documentElement dataset→监听 matchMedia)挂在 App 根。
  - h5(antd-mobile):antd-mobile 用 `--adm-*` CSS 变量,在 `html[data-theme=dark]` 下覆盖 `--adm-color-background/--adm-color-text/--adm-color-primary` 等核心 8 项(实测 antd-mobile 5.42 纯 CSS 变量,`:root:root` 可覆盖;`data-prefers-color-scheme` 可直接借用)。
  - 小程序:WXSS 支持 `var(--x)`(基础库 2.11+);根 View `style` 里把 8 个核心色写死成变量回退;`page` 级样式用 `page[data-theme=dark]` 类选择器(Taro 下用 `Taro.setNavigationBarColor`)。首屏防闪烁:app.tsx 同步读 storage 注入。
- **切换 UI**:顶栏 太阳/月亮 图标 + 品牌色随行下拉;按钮三态(亮/暗/跟随系统)。

### 1.2 图片资产(合规·离线兜底·性能)
- **不依赖 key 的直链 CDN**(Unsplash/Pexels 免费商用合规,热链无需 key):
  - `https://images.unsplash.com/photo-<id>?auto=format&fit=crop&w=800&q=60`
  - `https://images.pexels.com/photos/<id>/pexels-photo-<id>.jpeg?auto=compress&cs=tinysrgb&w=800`
- **图片用途(每端)**:登录/品牌 hero、功能场景卡(口语=咖啡馆/写作=书桌/学习=书架/排行=奖杯/圈子=人群)、空态插画(内联 SVG 优先,离线稳)。
- **本地兜底**:`<img loading="lazy" onError→隐藏并显示 CSS 渐变 fallback>`,渐变可用 `--esp-brand-*` 做 mesh,暗色模式不断链。
- **头像**:DiceBear(无 key: `https://api.dicebear.com/9.x/<style>/svg?seed=<uid>`)给排行榜/圈子成员;数字人保留现有 Avatar.tsx 内联 SVG。
- **图标**:web 引入 `lucide-react`(tree-shake);h5 `antd-mobile-icons`;小程序用内联 SVG 组件(避免包体)。
- **Portal**:换真 logo(SVG 内联)+ 各端入口方块配 unsplash 缩略图。

### 1.3 i18n(统一 i18next,三端同源)
- **栈**:`i18next + react-i18next`(三端全 React,一套 API)。`packages/i18n` 新共享包(或并入 ui-tokens)收语言包,现有 24 key 平移+扩展 to 常用命名空间(common/auth/nav/lesson/writing/speaking/ranking/community/content/error)。en/zh 镜像;CI 校验 key 集合一致。
- **探测与持久化**:web/h5 `i18next-browser-languagedetector`,key `esp_locale`;小程序自定义 detector(`Taro.getSystemInfoSync().language` + `Taro.setStorageSync('esp_locale')`)。登录后异步同步后端 user profile(后续)。
- **联动**:h5 antd-mobile `ConfigProvider locale`(zh-CN/en-US)包一层;`<html lang>` 同步。
- **AI 内容不翻译**:AI 生成(批改/翻译/引导)走后端按 `Accept-Language` 生成,前端不翻 LLM 输出。
- **切换 UI**:顶栏地球图标 → 中文/English;设置页三档(跟随系统/中文/English)。

## 2. 视觉分层(吸引各类人群)

| 场景 | 视觉诉求 | 手法 |
|---|---|---|
| 登录/品牌 | 信任+活力 | hero 图+品牌渐变文字+C 位资产;圆角色卡、软阴影、微动效(hover lift) |
| 学习页 | 沉浸专注 | 高对比正文、低饱和底、内容为首;进度环+连续天数徽章(激励) |
| 排行榜 | 竞技激励 | 前三名金银铜 medal 色、trophy 图标、XP 数字大字、自适应地域标签 |
| 圈子社区 | 温暖连接 | 头像墙、卡片间距宽松、CTA 色随 brand |
| 教师/管理 | 克制高效 | 白卡片+表格密度+satus 徽章;不喧宾夺主 |
| 空态/错误 | 友善 | 内联 SVG 插画+一句文案+行动按钮 |

- 色彩心理:蓝=信任(学习核心)、绿=进步/成功、橙=激励(口语)、紫=创意(写作)。品牌色随 accent 主题变化字体权重分级(regular/medium/bold)。

## 3. 分端落地清单

### 3.1 apps/web(不引 UI 库,纯 tokens 最省)
1. `styles.css` 重构 → 全部 `var(--esp-*)`;删硬编码 slate;正文字号/行高/圆角/阴影进变量。
2. `ThemeProvider.tsx` + `I18nApp.tsx`(i18next init)+ App.tsx 挂载 + `<html>` 切换。
3. Login:hero 分部(unsplash 学习图 + 品牌渐变标题 + 表单卡)。LearningLayout:顶栏加主题/语言切换 + logo(SVG);内容区渐入动画。
4. 新增 5 张场景卡图 / 6 处空态插画(内联 SVG)/ 头像 DiceBear(排行榜、圈子)。
5. RankingPage/CommunityPage/ContentLibrary 卡片升级(圆角/阴影/悬停/medal 色)。
6. 硬编码中文 → `t()` 迁移(366 行)。
7. index.html lang 动态。

### 3.2 apps/h5
1. tokens 接入 + antd-mobile 8 项 `--adm-*` 暗色映射。
2. ConfigProvider locale 双语;语言/主题切换入顶部栏。
3. Home 卡图(unsplash 竖图 w=600)、Speech/Writing/Learning 页沉浸背景;Profile 列表升级(头像/渐变压层)。
4. 中文 → `t()`(97 行)。

### 3.3 apps/miniprogram(Taro)
1. `app.config.ts`:navigationBar/主题色改品牌;tabBar 换彩色图标(现无图标)。
2. 自定义 i18n detector(Taro storage)+ setNavigationBarTitle 联动。
3. 根布局注入核心 CSS 变量(内联 fallback);页面卡片圆角/阴影升级。
4. 登录页 hero(weapp 支持网络图 image mode=aspectFill)+ 头像占位。

### 3.4 frontend-portal
1. logo(SVG)顶部;各端入口卡用 unsplash 缩略图 + hover 放大。
2. `data-theme` 切换(默认跟随系统)+ 语言(en/zh 两套文案,JS 切换)+ 保留连通性探测。

## 4. 执行顺序与验证
1. packages/ui-tokens 扩展(accent 变量集 + i18n 命名空间 JSON)- 先做(各端依赖)。
2. 四端并行重构(esp-builder 完成后,避免 apps/web 冲突)。
3. 每端构建→部署→浏览器/真机人工验:主题切换生效、中英切换生效、图片加载(暗色 fallback)、pages 无 404。
4. 全端截图录入 docs/test + README 新功能小节增补截图章节。
5. 双轨落盘 PROGRESS + Redis。

## 5. 风险与诚实
- 图片依赖外网:内网/断网时 fallback 生效(渐变+插画),功能不依赖图。
- 小程序真机主题 note:微信基础库 CSS 变量支持 2.11+;低版本回退纯色。
- 主题"三 accent × 亮暗"为视觉丰富度,设计 token 先行,后续可扩任意色。