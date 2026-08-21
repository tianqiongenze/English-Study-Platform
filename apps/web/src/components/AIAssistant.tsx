import { useState, useRef, useEffect } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { FloatButton, Input, Button, Typography, Badge, Tag, Tooltip } from "antd";
import { RobotOutlined, SendOutlined, CloseOutlined, LoadingOutlined } from "@ant-design/icons";
import { api } from "../api";
import { useAuth } from "../auth";
import { fadeUp, EASE, DUR_SLOW } from "../motion";

const { Text, Paragraph } = Typography;
const { TextArea } = Input;

type Msg = { role: "user" | "ai"; text: string };

// AI 助手浮层(全前端 AI 驱动,经 gateway LCO 三层缓存)。
// 常驻右下角悬浮按钮,点击展开对话面板。各页面通过全局 FloatButton 覆盖。
// LLM 链路:前端 → /api/llm/chat → gateway-go → llm.NewWithCache()(L1 Redis + L2 Milvus + L3 Neo4j)。
export default function AIAssistant() {
  const { user } = useAuth();
  const [open, setOpen] = useState(false);
  const [messages, setMessages] = useState<Msg[]>([
    { role: "ai", text: "你好!我是洁语 AI 学习助手 🤖 有任何英语学习问题(口语/写作/词汇/语法)随时问我。" },
  ]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => { scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" }); }, [messages, open]);

  async function send() {
    const text = input.trim();
    if (!text || loading) return;
    setInput("");
    setMessages((m) => [...m, { role: "user", text }]);
    setLoading(true);
    try {
      const r = await api.llmChat(text);
      setMessages((m) => [...m, { role: "ai", text: r.reply }]);
    } catch (e) {
      setMessages((m) => [...m, { role: "ai", text: `请求失败:${e}` }]);
    } finally {
      setLoading(false);
    }
  }

  if (!user) return null; // 未登录不显示

  return (
    <>
      {/* 悬浮按钮 */}
      <FloatButton
        icon={<RobotOutlined />}
        type="primary"
        shape="circle"
        style={{ right: 28, bottom: 28, width: 52, height: 52, fontSize: 24 }}
        onClick={() => setOpen((v) => !v)}
        tooltip={<div>AI 助手</div>}
      />

      {/* 对话面板 */}
      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ opacity: 0, y: 30, scale: 0.9 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 30, scale: 0.9 }}
            transition={{ duration: DUR_SLOW, ease: EASE }}
            style={{
              position: "fixed", right: 24, bottom: 96, width: 380, height: 540,
              background: "var(--esp-color-bg-container, #fff)",
              borderRadius: 16, boxShadow: "var(--esp-shadow-2, 0 6px 16px rgba(0,0,0,0.12))",
              zIndex: 1000, display: "flex", flexDirection: "column", overflow: "hidden",
              border: "1px solid var(--esp-color-border, #f0f0f0)",
            }}
          >
            {/* 头部 */}
            <div style={{
              padding: "12px 16px", background: "var(--esp-brand-gradient, linear-gradient(135deg,#1677ff,#69b7ff))",
              color: "#fff", display: "flex", alignItems: "center", justifyContent: "space-between",
            }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <RobotOutlined style={{ fontSize: 20 }} />
                <div>
                  <div style={{ fontWeight: 600, fontSize: 15 }}>AI 学习助手</div>
                  <div style={{ fontSize: 11, opacity: 0.85 }}>LCO 三层缓存 · 多级智能</div>
                </div>
              </div>
              <Button type="text" size="small" icon={<CloseOutlined style={{ color: "#fff" }} />} onClick={() => setOpen(false)} />
            </div>

            {/* 消息区 */}
            <div ref={scrollRef} style={{ flex: 1, overflow: "auto", padding: 12, background: "var(--esp-color-bg-layout, #f5f5f5)" }}>
              {messages.map((m, i) => (
                <motion.div key={i} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }}
                  transition={{ duration: 0.2, ease: EASE }}
                  style={{ display: "flex", justifyContent: m.role === "user" ? "flex-end" : "flex-start", marginBottom: 8 }}>
                  <div style={{
                    maxWidth: "80%", padding: "8px 12px", borderRadius: 12,
                    background: m.role === "user" ? "var(--esp-brand, #1677ff)" : "var(--esp-color-bg-container, #fff)",
                    color: m.role === "user" ? "#fff" : "var(--esp-color-text, #000)",
                    fontSize: 14, boxShadow: "0 1px 2px rgba(0,0,0,0.06)",
                    whiteSpace: "pre-wrap", wordBreak: "break-word",
                  }}>
                    {m.text}
                  </div>
                </motion.div>
              ))}
              {loading && (
                <div style={{ display: "flex", gap: 4, padding: "4px 12px", color: "var(--esp-color-text-secondary, #999)" }}>
                  <LoadingOutlined /> AI 思考中...
                </div>
              )}
            </div>

            {/* 输入区 */}
            <div style={{ padding: 8, borderTop: "1px solid var(--esp-color-border, #f0f0f0)", background: "var(--esp-color-bg-container, #fff)" }}>
              <div style={{ display: "flex", gap: 4, alignItems: "flex-end" }}>
                <TextArea value={input} onChange={(e) => setInput(e.target.value)} placeholder="问我英语学习问题…"
                  autoSize={{ minRows: 1, maxRows: 3 }}
                  onPressEnter={(e) => { if (!e.shiftKey) { e.preventDefault(); send(); } }}
                  style={{ flex: 1, resize: "none" }} />
                <Button type="primary" icon={<SendOutlined />} onClick={send} loading={loading} disabled={!input.trim()} />
              </div>
              <div style={{ fontSize: 10, color: "var(--esp-color-text-tertiary, #bbb)", marginTop: 4 }}>
                Enter 发送 · 经 LCO 三层缓存(LLM 调用优化)
              </div>
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  );
}
