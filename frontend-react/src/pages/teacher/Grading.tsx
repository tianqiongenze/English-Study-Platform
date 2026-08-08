import { useState } from "react";
import { api } from "../../api";

// 教师作业评阅页(原型 3.6 / F14)
// 流程:输入作业 ID → 调 auto-grade(写作智能体批量自动批改)→ 展示自动分/反馈 → 教师终审 finalize。
export default function Grading() {
  const [assignmentId, setAssignmentId] = useState("1");
  const [classId, setClassId] = useState("1");
  const [graded, setGraded] = useState<number | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(false);
  // 终审:教师对某 submission 调整最终分后 finalize
  const [subId, setSubId] = useState("");
  const [finalScore, setFinalScore] = useState("");
  const [finalized, setFinalized] = useState<number | null>(null);

  async function autoGrade() {
    setErr(""); setGraded(null); setLoading(true);
    try {
      const r = await api.autoGrade(Number(assignmentId), classId ? Number(classId) : undefined);
      setGraded(r.graded);
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }

  async function finalize() {
    setErr(""); setFinalized(null); setLoading(true);
    try {
      const r = await api.finalize(Number(subId), Number(finalScore), classId ? Number(classId) : undefined);
      setFinalized(r.finalized);
    } catch (e) { setErr(`${e}`); }
    finally { setLoading(false); }
  }

  return (
    <div>
      <h2>作业评阅(写作智能体自动批改 + 教师终审)</h2>
      <p className="hint">原型 3.6 / F14:先批量自动评阅(写作智能体),再对单个 submission 终审确认最终分。</p>

      <h3>1. 批量自动评阅</h3>
      <label>作业 ID: <input value={assignmentId} onChange={(e) => setAssignmentId(e.target.value)} style={{ width: 80 }} /></label>
      <label>班级 ID: <input value={classId} onChange={(e) => setClassId(e.target.value)} style={{ width: 80 }} /></label>
      <button className="primary" onClick={autoGrade} disabled={loading}>{loading ? "批改中..." : "批量自动评阅"}</button>
      {graded !== null && <p>✅ 已自动批改 <b>{graded}</b> 份提交(详见各 submission 的 score/feedback)。</p>}

      <h3>2. 教师终审(可选,调整最终分)</h3>
      <label>提交 ID: <input value={subId} onChange={(e) => setSubId(e.target.value)} style={{ width: 80 }} /></label>
      <label>最终分: <input value={finalScore} onChange={(e) => setFinalScore(e.target.value)} style={{ width: 80 }} placeholder="0-30" /></label>
      <button className="primary" onClick={finalize} disabled={loading || !subId || !finalScore}>确认终审</button>
      {finalized !== null && <p>✅ 已终审 <b>{finalized}</b> 份提交,分数已落库。</p>}

      {err && <p className="err">{err}</p>}
    </div>
  );
}
