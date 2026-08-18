import { useState, useEffect } from "react";
import { api } from "../api";
import type { Mastery, DifficultyResp } from "../types";

export default function Learning() {
  const [uid, setUid] = useState("1");
  const [out, setOut] = useState("");
  const [placeInfo, setPlaceInfo] = useState("");
  const [placed, setPlaced] = useState(false);
  // FR-2.1 首次进入触发 placement 定级
  useEffect(() => {
    if (placed) return;
    (async () => {
      try { const r = await api.placement(Number(uid), [1, 2, 3, 4, 5], 0.4); setPlaceInfo(`能力定级完成:初始化 ${r.initialized} 个知识点基线`); }
      catch (e) { setPlaceInfo(`定级跳过:${e}`); } finally { setPlaced(true); }
    })();
  }, [uid, placed]);

  async function event() { try { setOut(JSON.stringify(await api.reportEvent(Number(uid), "answer", "{}"))); } catch (e) { setOut(`错误:${e}`); } }
  async function review() { try { const d: Mastery[] = await api.review(Number(uid)); setOut(d.map((m) => `kp${m.kpId}:${m.masteryScore}`).join("\n") || "无"); } catch (e) { setOut(`错误:${e}`); } }
  async function weak() { try { const d: Mastery[] = await api.weak(Number(uid)); setOut(d.map((m) => `kp${m.kpId}:${m.masteryScore}`).join("\n") || "无"); } catch (e) { setOut(`错误:${e}`); } }
  // FR-2.2 动态难度调整
  async function difficulty() { try { const d: DifficultyResp = await api.difficulty(3, true); setOut(`下一难度:${d.next_difficulty} 倍速:${d.speed_multiplier} 脚手架:${d.scaffolding}`); } catch (e) { setOut(`错误:${e}`); } }

  return (
    <div className="panel immersive">
      <h3>自适应学习</h3>
      <input value={uid} onChange={(e) => { setUid(e.target.value); setPlaced(false); }} placeholder="用户 ID" style={{ width: "40%" }} />
      {placeInfo && <p className="hint">💡 {placeInfo}</p>}
      <div style={{ display: "flex", gap: 6, margin: "6px 0", flexWrap: "wrap" }}>
        <button className="primary" onClick={event}>上报</button>
        <button onClick={review}>复习</button>
        <button onClick={weak}>薄弱点</button>
        <button onClick={difficulty}>难度调整</button>
      </div>
      <pre className="log">{out}</pre>
    </div>
  );
}
