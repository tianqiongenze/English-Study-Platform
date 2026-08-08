import { useState } from "react";
import { api } from "../../api";
import type { ClassAnalytics } from "../../types";

export default function Analytics() {
  const [classId, setClassId] = useState("1");
  const [rows, setRows] = useState<ClassAnalytics[]>([]);
  const [err, setErr] = useState("");

  async function load() {
    setErr("");
    try { setRows(await api.classAnalytics(Number(classId))); }
    catch (e) { setErr(`${e}`); }
  }

  return (
    <div>
      <h2>班级学情分析(TiFlash OLAP)</h2>
      <label>班级 ID: <input value={classId} onChange={(e) => setClassId(e.target.value)} style={{ width: 80 }} /></label>
      <button className="primary" onClick={load}>查询</button>
      {err && <p className="err">{err}</p>}
      {rows.length > 0 && (
        <table className="analytics">
          <thead><tr><th>内容</th><th>平均分</th><th>人数</th></tr></thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.id}><td>{r.content}</td><td>{r.avgScore}</td><td>{r.count}</td></tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
