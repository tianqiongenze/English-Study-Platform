import { NavLink, Outlet, useLocation } from "react-router-dom";
import { Layout, Menu, Space, Typography } from "antd";
import { motion, AnimatePresence } from "framer-motion";
import { useI18n, ThemeSwitcher, AccentPicker, LangSwitcher, LogoIcon } from "@esp/ui-tokens";
import { useAuth } from "../auth";
import AppHeader from "../components/AppHeader";
import { pageTransition } from "../motion";

const { Header, Content } = Layout;

// 学习端 Layout —— antd Layout.Header + Menu horizontal(替原 .tabs)+ framer-motion 页面转场。
// AppHeader 品牌区美化(LogoIcon + 渐变标题 + 切换器)。
export default function LearningLayout() {
  const { t } = useI18n();
  const { user } = useAuth();
  const location = useLocation();

  const menuItems = [
    { key: "/learning/speech", label: <NavLink to="/learning/speech">{t("nav.speaking")}</NavLink> },
    { key: "/learning/writing", label: <NavLink to="/learning/writing">{t("nav.writing")}</NavLink> },
    { key: "/learning/knowledge", label: <NavLink to="/learning/knowledge">{t("nav.learning")}</NavLink> },
    { key: "/learning/review", label: <NavLink to="/learning/review">复习清单</NavLink> },
    { key: "/learning/difficulty", label: <NavLink to="/learning/difficulty">动态难度</NavLink> },
    { key: "/learning/content", label: <NavLink to="/learning/content">{t("nav.content")}</NavLink> },
    { key: "/learning/community", label: <NavLink to="/learning/community">{t("nav.community")}</NavLink> },
    { key: "/learning/ranking", label: <NavLink to="/learning/ranking">{t("nav.ranking")}</NavLink> },
    ...(user?.role === "admin" ? [{ key: "/admin", label: <NavLink to="/admin">管理员监控</NavLink> }] : []),
  ];

  // 当前选中的菜单项(按 path 前缀匹配)
  const selectedKey = menuItems
    .map((i) => i.key)
    .filter((k) => location.pathname.startsWith(k))
    .sort((a, b) => b.length - a.length)[0] || "/learning/speech";

  return (
    <Layout style={{ minHeight: "100vh", background: "var(--esp-color-bg-layout)" }}>
      <Header style={{
        display: "flex", alignItems: "center", justifyContent: "space-between",
        padding: "0 24px", height: 64,
        background: "var(--esp-color-bg-container)",
        boxShadow: "var(--esp-shadow-1)",
        position: "sticky", top: 0, zIndex: 100,
      }}>
        <Space size="large">
          <NavLink to="/learning/speech" style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <LogoIcon size={28} />
            <Typography.Text strong style={{
              fontSize: 18,
              background: "var(--esp-brand-gradient)",
              WebkitBackgroundClip: "text", WebkitTextFillColor: "transparent",
              backgroundClip: "text",
            }}>{t("app.title")}</Typography.Text>
          </NavLink>
          <Menu
            mode="horizontal"
            selectedKeys={[selectedKey]}
            items={menuItems}
            style={{ flex: 1, minWidth: 0, borderBottom: "none", background: "transparent" }}
          />
        </Space>
        <Space>
          <LangSwitcher />
          <AccentPicker />
          <ThemeSwitcher />
        </Space>
      </Header>
      <Content style={{ maxWidth: 1280, margin: "0 auto", padding: "24px", width: "100%" }}>
        {/* framer-motion 页面转场:key=pathname 触发 Outlet 内容转场动画 */}
        <AnimatePresence mode="wait">
          <motion.div
            key={location.pathname}
            variants={pageTransition}
            initial="initial"
            animate="animate"
            exit="exit"
          >
            <Outlet />
          </motion.div>
        </AnimatePresence>
      </Content>
    </Layout>
  );
}
