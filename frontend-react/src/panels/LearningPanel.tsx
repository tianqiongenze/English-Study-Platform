import { useState, useEffect } from "react";
import { api } from "../api";
import type { Mastery } from "../types";

export default function LearningPanel({ mode = "event" }: { mode?: "event" | "review" | "difficulty" }) {
  const [userId, setUserId] = useState("1");
  const [event, setEvent] = useState("");
  const [result, setResult] = useState<string>("");
  // FR-2.1 首次进入学习中心触发 placement 定级
  const [placed, setPlaced] = useState(false);
  const [placeInfo, setPlaceInfo] = useState("");

  useEffect(() => {
    if (placed) return;
    (async () => {
      try {
        const r = await api.placement(Number(userId), [1, 2, 3, 4, 5], 0.4);
        setPlaceInfo(`能力定级完成:初始化 ${r.initialized} 个知识点基线(后续难度据此调整)`);
      } catch (e) { setPlaceInfo(`定级跳过:${e}`); }
      finally { setPlaced(true); }
    })();
  }, [userId, placed]);

  async function reportEvent() {
    try {
      const d = await api.reportEvent(Number(userId), "answer", event || "{}");
      setResult(JSON.stringify(d));
    } catch (e) { setResult(`错误: ${e}`); }
  }
  async function review() {
    try {
      const d: Mastery[] = await api.review(Number(userId));
      setResult(d.length ? d.map((m) => `kp${m.kpId}: ${m.masteryScore} (下次 ${m.nextReviewAt})`).join("\n") : "今日无复习任务");
    } catch (e) { setResult(`错误: ${e}`); }
  }
  async function weak() {
    try {
      const d: Mastery[] = await api.weak(Number(userId));
      setResult(d.length ? d.map((m) => `kp${m.kpId}: ${m.masteryScore}`).join("\n") : "无数据");
    } catch (e) { setResult(`错误: ${e}`); }
  }
  async function difficulty() {
    try {
      const d = await api.difficulty(3, true);
      setResult(`下一难度: ${d.next_difficulty}, 倍速: ${d.speed_multiplier}, 脚手架: ${d.scaffolding}`);
    } catch (e) { setResult(`错误: ${e}`); }
  }
  // 用户行为上报(ADR-observability-rebuild §4):页面停留/练习完成等行为落 app_logs
  useEffect(() => {
    const t0 = Date.now();
    return () => {
      const duration = Date.now() - t0;
      api.reportBehavior(Number(userId), "page_stay", "LearningPanel", duration).catch(() => {});
    };
  }, [userId]);
  async function recommend() {
    try {
      const d = await api.recommend(Number(userId));
      const parts: string[] = [];
      parts.push(`复习清单:${(d.review as any[]).length || 0} 项`);
      parts.push(`薄弱知识点:${(d.weak_points as any[]).length || 0} 项`);
      parts.push(`相似场景推荐:${(d.similar_scenarios as any[]).length || 0} 项`);
      setResult(parts.join("\n"));
    } catch (e) { setResult(`错误: ${e}`); }
  }

  if (mode === "review") {
    return (
      <div>
        <label>用户 ID: <input value={userId} onChange={(e) => setUserId(e.target.value)} style={{ width: 80 }} /></label>
        <button className="primary" onClick={review}>查询今日复习</button>
        <button onClick={weak}>薄弱知识点</button>
        <pre className="log">{result}</pre>
      </div>
    );
  }
  if (mode === "difficulty") {
    return (
      <div>
        <p>模拟一次答题(当前难度 3,答对)以触发动态难度调整。</p>
        <button className="primary" onClick={difficulty}>计算下一难度</button>
        <pre className="log">{result}</pre>
      </div>
    );
  }
  return (
    <div>
      <label>用户 ID: <input value={userId} onChange={(e) => { setUserId(e.target.value); setPlaced(false); }} style={{ width: 80 }} /></label>
      {placeInfo && <p className="hint">💡 {placeInfo}</p>}
      <p>
        <button className="primary" onClick={reportEvent}>上报学情事件</button>{" "}
        <button onClick={review}>今日复习</button>{" "}
        <button onClick={weak}>薄弱知识点</button>{" "}
        <button onClick={recommend}>为你推荐</button>
      </p>
      <input value={event} onChange={(e) => setEvent(e.target.value)} placeholder='事件 payload JSON,如 {"kp_id":1,"correct":true}' />
      <pre className="log">{result}</pre>
    </div>
  );
}
