import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { motion } from "framer-motion";
import { Form, Input, Button, Segmented, Typography, message, Divider } from "antd";
import { RocketOutlined, MailOutlined, LockOutlined, UserOutlined } from "@ant-design/icons";
import { useAuth } from "../auth";
import { api } from "../api";
import { useI18n, LogoIcon } from "@esp/ui-tokens";
import SafeImg, { UNSPLASH } from "../components/SafeImg";
import { fadeUp, stagger, EASE, DUR_SLOW } from "../motion";

const { Title, Text, Paragraph } = Typography;

// 洁语(Verity)登录页 —— 互联网大厂级 split-screen 招牌页(对标多邻国/猿辅导)。
// 左:Unsplash 学习场景 hero + 品牌渐变 overlay + framer-motion 入场动效。
// 右:antd Form + Segmented(login/register)+ 品牌渐变标题 + 动效 stagger。
export default function Login() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState("learner");
  const [loading, setLoading] = useState(false);
  const { setAuth } = useAuth();
  const { t } = useI18n();
  const nav = useNavigate();

  async function submit() {
    setLoading(true);
    try {
      const res = mode === "login"
        ? await api.login(email, password)
        : await api.register(email, password, name || email.split("@")[0], role);
      setAuth(res.token, res.user);
      message.success(mode === "login" ? "登录成功" : "注册成功");
      nav(res.user.role === "teacher" ? "/teacher/lesson-plan" : "/learning/speech");
    } catch (e) {
      message.error(`${e}`);
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="auth-page" style={{ display: "flex", minHeight: "100vh", background: "var(--esp-color-bg-layout)" }}>
      {/* 左:hero(品牌氛围 + Unsplash 学习场景) */}
      <motion.section
        className="auth-hero"
        aria-hidden="true"
        initial={{ opacity: 0, scale: 1.05 }}
        animate={{ opacity: 1, scale: 1 }}
        transition={{ duration: 0.6, ease: EASE }}
        style={{ flex: 1, position: "relative", overflow: "hidden", display: "flex", alignItems: "flex-end", minHeight: "100vh" }}
      >
        <SafeImg src={UNSPLASH.study} alt="" tone={1} />
        {/* 品牌渐变 overlay */}
        <div style={{
          position: "absolute", inset: 0,
          background: "var(--esp-brand-gradient)", opacity: 0.85,
          mixBlendMode: "multiply",
        }} />
        {/* hero 文案 */}
        <motion.div
          variants={stagger}
          initial="hidden"
          animate="visible"
          style={{ position: "relative", padding: "48px", color: "#fff", zIndex: 1 }}
        >
          <motion.div variants={fadeUp} style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 16 }}>
            <LogoIcon size={44} />
            <Text style={{ color: "#fff", fontSize: 28, fontWeight: 700 }}>{t("app.title")}</Text>
          </motion.div>
          <motion.div variants={fadeUp}>
            <Title level={2} style={{ color: "#fff", margin: 0, textShadow: "0 2px 12px rgba(0,0,0,0.3)" }}>
              {t("app.tagline")}
            </Title>
          </motion.div>
          <motion.div variants={fadeUp}>
            <Paragraph style={{ color: "rgba(255,255,255,0.9)", fontSize: 16, marginTop: 12, maxWidth: 480 }}>
              AI 英语教育平台 · 三端(Web/H5/小程序)· 口语陪练 / 写作批改 / 个性化学习路径
            </Paragraph>
          </motion.div>
          <motion.div variants={fadeUp} style={{ display: "flex", gap: 24, marginTop: 24 }}>
            {[{ icon: <RocketOutlined />, label: "5000 CCU" }, { icon: <UserOutlined />, label: "三端同步" }, { icon: <MailOutlined />, label: "AI 陪练" }].map((f) => (
              <div key={f.label} style={{ display: "flex", alignItems: "center", gap: 6, color: "rgba(255,255,255,0.85)" }}>
                {f.icon} <span style={{ fontSize: 14 }}>{f.label}</span>
              </div>
            ))}
          </motion.div>
        </motion.div>
      </motion.section>

      {/* 右:登录表单卡 */}
      <motion.main
        className="auth-main"
        initial={{ opacity: 0, x: 30 }}
        animate={{ opacity: 1, x: 0 }}
        transition={{ duration: DUR_SLOW, ease: EASE, delay: 0.15 }}
        style={{ width: 460, display: "flex", alignItems: "center", justifyContent: "center", padding: "48px 40px", background: "var(--esp-color-bg-container)" }}
      >
        <motion.div
          variants={stagger}
          initial="hidden"
          animate="visible"
          style={{ width: "100%", maxWidth: 360 }}
        >
          <motion.div variants={fadeUp} style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 8 }}>
            <LogoIcon size={32} />
            <Text style={{ fontSize: 18, fontWeight: 600, color: "var(--esp-brand)" }}>{t("app.title")}</Text>
          </motion.div>

          <motion.div variants={fadeUp}>
            <Title level={2} style={{ marginTop: 0, marginBottom: 4 }}>
              {mode === "login" ? t("auth.welcome") : t("auth.register")}
            </Title>
            <Text type="secondary">{t("auth.enter")}</Text>
          </motion.div>

          <motion.div variants={fadeUp} style={{ margin: "20px 0" }}>
            <Segmented
              block
              value={mode}
              onChange={(v) => setMode(v as "login" | "register")}
              options={[
                { label: t("auth.login"), value: "login" },
                { label: t("auth.register"), value: "register" },
              ]}
            />
          </motion.div>

          <motion.div variants={fadeUp}>
            <Form layout="vertical" onFinish={submit}>
              {mode === "register" && (
                <Form.Item label={t("auth.name")}>
                  <Input prefix={<UserOutlined />} value={name} onChange={(e) => setName(e.target.value)} placeholder={t("auth.name")} />
                </Form.Item>
              )}
              <Form.Item label={t("auth.email")} name="email" rules={[{ required: true, message: "请输入邮箱" }]}>
                <Input prefix={<MailOutlined />} value={email} onChange={(e) => setEmail(e.target.value)} type="email" placeholder={t("auth.email")} />
              </Form.Item>
              <Form.Item label={t("auth.password")} name="password" rules={[{ required: true, message: "请输入密码" }]}>
                <Input.Password prefix={<LockOutlined />} value={password} onChange={(e) => setPassword(e.target.value)} placeholder={t("auth.password")} />
              </Form.Item>
              {mode === "register" && (
                <Form.Item label="角色">
                  <Segmented value={role} onChange={(v) => setRole(v as string)} options={[
                    { label: "学习者", value: "learner" },
                    { label: t("nav.teacher"), value: "teacher" },
                  ]} />
                </Form.Item>
              )}
              <Form.Item style={{ marginBottom: 8 }}>
                <Button type="primary" htmlType="submit" loading={loading} block size="large">
                  {mode === "login" ? t("auth.login") : "注册并登录"}
                </Button>
              </Form.Item>
            </Form>
          </motion.div>

          <motion.div variants={fadeUp}>
            <Divider style={{ margin: "12px 0" }} />
            <div style={{ textAlign: "center" }}>
              {mode === "register" ? t("auth.haveAccount") : t("auth.noAccount")}{" "}
              <Button type="link" onClick={() => setMode(mode === "login" ? "register" : "login")} style={{ padding: 0 }}>
                {mode === "login" ? t("auth.register") : t("auth.login")}
              </Button>
            </div>
          </motion.div>
        </motion.div>
      </motion.main>
    </div>
  );
}
