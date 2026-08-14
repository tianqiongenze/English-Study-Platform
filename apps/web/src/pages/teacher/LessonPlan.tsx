import { useState } from "react";
import { api } from "../../api";
import type { LessonPlan } from "../../types";

export default function LessonPlan() {
  const [topic, setTopic] = useState("Travel");
  const [level, setLevel] = useState("B1");
  const [plan, setPlan] = useState<LessonPlan | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");

  async function generate() {
    setLoading(true); setErr("");
    try {
      setPlan(await api.lessonPlan(topic, level));
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }

  return (
    <div>
      <h2>一键教案生成</h2>
      <label>主题: <input value={topic} onChange={(e) => setTopic(e.target.value)} /></label>
      <label>级别:
        <select value={level} onChange={(e) => setLevel(e.target.value)}>
          <option value="A2">A2</option><option value="B1">B1</option><option value="B2">B2</option><option value="C1">C1</option>
        </select>
      </label>
      <button className="primary" onClick={generate} disabled={loading}>{loading ? "生成中..." : "生成教案"}</button>
      {err && <p className="err">{err}</p>}
      {plan && (
        <div>
          <h3>课文</h3><pre>{plan.passage}</pre>
          <h3>练习 ({plan.exercises.length})</h3><pre>{JSON.stringify(plan.exercises, null, 2)}</pre>
          <h3>游戏 ({plan.games.length})</h3><pre>{JSON.stringify(plan.games, null, 2)}</pre>
          <h3>讨论 ({plan.discussion.length})</h3><pre>{JSON.stringify(plan.discussion, null, 2)}</pre>
        </div>
      )}
    </div>
  );
}
