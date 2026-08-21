// 管理员统一运维平台 Layout —— antd v6 Layout + Sider + Menu。
// 收编原 5 个平级 admin 路由(监控/内容/治理/中间件/诊断)+ 新增"API 测试"为侧栏导航。
// 路由结构(见 App.tsx):/admin 嵌套,<Outlet/> 渲染子路由;index=AdminDashboard。
// 顶栏:标题 + 主题/accent/lang 切换器(复用 @esp/ui-tokens)+ 返回学习端入口。
// 鉴权:外层 RequireAuth role=admin 已守门,Layout 内不再重复判 role。

import { useMemo, type ReactNode } from "react";
import { Layout, Menu, theme as antdTheme } from "antd";
import {
  DashboardOutlined,
  DatabaseOutlined,
  SafetyCertificateOutlined,
  CloudServerOutlined,
  BugOutlined,
  ApiOutlined,
  ArrowLeftOutlined,
  ControlOutlined,
} from "@ant-design/icons";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useI18n, ThemeSwitcher, AccentPicker, LangSwitcher, LogoIcon } from "@esp/ui-tokens";
import { useAuth } from "../../auth";

const { Header, Sider, Content } = Layout;

interface NavItem {
  key: string;
  path: string;
  icon: ReactNode;
  labelZh: string;
  labelEn: string;
}

const NAV_ITEMS: NavItem[] = [
  { key: "monitor", path: "/admin", icon: <DashboardOutlined />, labelZh: "实时监控", labelEn: "Monitoring" },
  { key: "ops", path: "/admin/ops", icon: <ControlOutlined />, labelZh: "运维操作", labelEn: "Operations" },
  { key: "content", path: "/admin/content", icon: <DatabaseOutlined />, labelZh: "内容管理", labelEn: "Content" },
  { key: "governance", path: "/admin/governance", icon: <SafetyCertificateOutlined />, labelZh: "服务治理", labelEn: "Governance" },
  { key: "middleware", path: "/admin/middleware", icon: <CloudServerOutlined />, labelZh: "中间件", labelEn: "Middleware" },
  { key: "debug", path: "/admin/debug", icon: <BugOutlined />, labelZh: "深度诊断", labelEn: "Diagnostics" },
  { key: "api-test", path: "/admin/api-test", icon: <ApiOutlined />, labelZh: "API 测试", labelEn: "API Testing" },
];

export default function AdminLayout() {
  const { t, locale } = useI18n();
  const { user } = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const { token: themeToken } = antdTheme.useToken();

  // 当前选中项:/admin 精确匹配,子路径前缀匹配。
  const selectedKey = useMemo(() => {
    const p = location.pathname;
    if (p === "/admin") return "monitor";
    const item = NAV_ITEMS.find((n) => n.path !== "/admin" && p.startsWith(n.path));
    return item?.key ?? "monitor";
  }, [location.pathname]);

  const menuItems = NAV_ITEMS.map((n) => ({
    key: n.key,
    icon: n.icon,
    label: (
      <NavLink to={n.path} end={n.path === "/admin"}>
        {locale === "zh" ? n.labelZh : n.labelEn}
      </NavLink>
    ),
  }));

  return (
    <Layout style={{ minHeight: "100vh" }}>
      <Sider
        breakpoint="lg"
        collapsedWidth={0}
        width={220}
        style={{ overflow: "auto", height: "100vh", position: "sticky", top: 0, left: 0 }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 8,
            height: 56,
            padding: "0 20px",
            color: themeToken.colorTextLightSolid,
            fontWeight: 600,
            fontSize: 16,
          }}
        >
          <LogoIcon size={26} />
          <span>{locale === "zh" ? "洁语 · 运维平台" : "Jieyu Ops"}</span>
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selectedKey]}
          items={menuItems}
          style={{ borderRight: 0 }}
        />
      </Sider>

      <Layout>
        <Header
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "0 24px",
            boxShadow: themeToken.boxShadowTertiary,
            position: "sticky",
            top: 0,
            zIndex: 10,
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
            <a
              onClick={() => navigate("/learning/speech")}
              style={{ color: themeToken.colorText, display: "flex", alignItems: "center", gap: 6, cursor: "pointer" }}
            >
              <ArrowLeftOutlined />
              <span>{locale === "zh" ? "返回学习端" : "Back"}</span>
            </a>
            <span style={{ color: themeToken.colorTextSecondary, fontSize: 13 }}>
              {user?.email ?? ""}
            </span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <LangSwitcher />
            <AccentPicker />
            <ThemeSwitcher />
          </div>
        </Header>

        <Content style={{ margin: 0, padding: 24, minHeight: 280, background: themeToken.colorBgLayout }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
