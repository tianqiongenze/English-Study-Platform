import { useCallback, useEffect, useState } from "react";
import { api } from "../../api";
import type { ContentCourse, ContentItem, ContentLesson } from "@esp/types";

// 内容库管理端:课程/课时/题目 CRUD + 状态流转 + 批量导入(role=admin,经 BFF X-Role 放行)。
// status 生命周期:draft / pending / published / archived。
// P2-23:prompt() 全量替换为 React 内联表单。

const SKILL_LABEL: Record<string, string> = {
  vocab: "词汇", reading: "阅读", listening: "听力", grammar: "语法",
};

const STATUS_COLOR: Record<string, string> = {
  draft: "var(--esp-warning, #a16207)", pending: "var(--esp-primary, #2563eb)", published: "var(--esp-success, #16a34a)", archived: "var(--esp-color-text-muted, #6b7280)",
};

/* ---------- 内联表单:新建课程 ---------- */
function CourseForm({ onSubmit, onCancel, busy }: { onSubmit: (v: { code: string; title: string; exam_tag: string; cefr_range: string; description: string }) => void; onCancel: () => void; busy: boolean }) {
  const [code, setCode] = useState("");
  const [title, setTitle] = useState("");
  const [examTag, setExamTag] = useState("GENERAL");
  const [cefr, setCefr] = useState("A1-B2");
  const [desc, setDesc] = useState("");
  const form: React.CSSProperties = { display: "flex", flexDirection: "column", gap: 6, padding: 10, border: "1px solid var(--esp-color-border, #e5e7eb)", borderRadius: 8, marginBottom: 10 };
  const row: React.CSSProperties = { display: "flex", gap: 8, alignItems: "center" };
  return (
    <div style={form}>
      <div style={row}><label style={{ minWidth: 70, fontSize: 12 }}>代码</label><input value={code} onChange={e => setCode(e.target.value)} placeholder="cet4 / toefl" style={{ flex: 1 }} /></div>
      <div style={row}><label style={{ minWidth: 70, fontSize: 12 }}>标题</label><input value={title} onChange={e => setTitle(e.target.value)} placeholder="课程标题" style={{ flex: 1 }} /></div>
      <div style={row}><label style={{ minWidth: 70, fontSize: 12 }}>考试标签</label><select value={examTag} onChange={e => setExamTag(e.target.value)} style={{ flex: 1 }}><option>CET4</option><option>CET6</option><option>TOEFL</option><option>IELTS</option><option>GENERAL</option></select></div>
      <div style={row}><label style={{ minWidth: 70, fontSize: 12 }}>CEFR</label><input value={cefr} onChange={e => setCefr(e.target.value)} placeholder="A1-B2" style={{ flex: 1 }} /></div>
      <div style={row}><label style={{ minWidth: 70, fontSize: 12 }}>描述</label><input value={desc} onChange={e => setDesc(e.target.value)} placeholder="可选" style={{ flex: 1 }} /></div>
      <div style={row}>
        <button className="primary" disabled={busy || !code || !title} onClick={() => onSubmit({ code, title, exam_tag: examTag, cefr_range: cefr, description: desc })}>创建</button>
        <button onClick={onCancel} disabled={busy}>取消</button>
      </div>
    </div>
  );
}

/* ---------- 内联表单:新建课时 ---------- */
function LessonForm({ onSubmit, onCancel, busy }: { onSubmit: (v: { skill: string; title: string; seq: number; cefr: string }) => void; onCancel: () => void; busy: boolean }) {
  const [skill, setSkill] = useState("vocab");
  const [title, setTitle] = useState("");
  const [seq, setSeq] = useState(1);
  const [cefr, setCefr] = useState("B1");
  const form: React.CSSProperties = { display: "flex", flexDirection: "column", gap: 6, padding: 10, border: "1px solid var(--esp-color-border, #e5e7eb)", borderRadius: 8, marginBottom: 10 };
  const row: React.CSSProperties = { display: "flex", gap: 8, alignItems: "center" };
  return (
    <div style={form}>
      <div style={row}><label style={{ minWidth: 50, fontSize: 12 }}>技能</label><select value={skill} onChange={e => setSkill(e.target.value)} style={{ flex: 1 }}><option value="vocab">词汇</option><option value="reading">阅读</option><option value="listening">听力</option><option value="grammar">语法</option></select></div>
      <div style={row}><label style={{ minWidth: 50, fontSize: 12 }}>标题</label><input value={title} onChange={e => setTitle(e.target.value)} placeholder="课时标题" style={{ flex: 1 }} /></div>
      <div style={row}><label style={{ minWidth: 50, fontSize: 12 }}>序号</label><input type="number" value={seq} onChange={e => setSeq(Number(e.target.value))} style={{ flex: 1 }} /></div>
      <div style={row}><label style={{ minWidth: 50, fontSize: 12 }}>CEFR</label><input value={cefr} onChange={e => setCefr(e.target.value)} style={{ flex: 1 }} /></div>
      <div style={row}>
        <button className="primary" disabled={busy || !title} onClick={() => onSubmit({ skill, title, seq, cefr })}>创建</button>
        <button onClick={onCancel} disabled={busy}>取消</button>
      </div>
    </div>
  );
}

/* ---------- 内联表单:新建题目 ---------- */
function ItemForm({ onSubmit, onCancel, busy }: { onSubmit: (v: { item_type: string; prompt: object; answer: object; explanation: string; difficulty: number; cefr: string }) => void; onCancel: () => void; busy: boolean }) {
  const [itemType, setItemType] = useState("vocab");
  const [promptRaw, setPromptRaw] = useState('{"word":"abandon","meaning_zh":"放弃"}');
  const [answerRaw, setAnswerRaw] = useState('{"correct_option":0}');
  const [explanation, setExplanation] = useState("");
  const [difficulty, setDifficulty] = useState(3);
  const [cefr, setCefr] = useState("B1");
  const [parseErr, setParseErr] = useState("");
  const form: React.CSSProperties = { display: "flex", flexDirection: "column", gap: 6, padding: 10, border: "1px solid var(--esp-color-border, #e5e7eb)", borderRadius: 8, marginBottom: 10 };
  const row: React.CSSProperties = { display: "flex", gap: 8, alignItems: "center" };
  function handleSubmit() {
    setParseErr("");
    try {
      const promptObj = JSON.parse(promptRaw);
      const answerObj = answerRaw ? JSON.parse(answerRaw) : {};
      onSubmit({ item_type: itemType, prompt: promptObj, answer: answerObj, explanation, difficulty, cefr });
    } catch (e) { setParseErr(`JSON 解析失败: ${e}`); }
  }
  return (
    <div style={form}>
      <div style={row}><label style={{ minWidth: 60, fontSize: 12 }}>题型</label><select value={itemType} onChange={e => setItemType(e.target.value)} style={{ flex: 1 }}><option value="vocab">词汇</option><option value="reading">阅读</option><option value="listening">听力</option><option value="grammar">语法</option></select></div>
      <div><label style={{ fontSize: 12 }}>Prompt JSON</label><textarea value={promptRaw} onChange={e => setPromptRaw(e.target.value)} rows={3} style={{ width: "100%", fontSize: 11 }} /></div>
      <div><label style={{ fontSize: 12 }}>Answer JSON</label><textarea value={answerRaw} onChange={e => setAnswerRaw(e.target.value)} rows={2} style={{ width: "100%", fontSize: 11 }} /></div>
      <div style={row}><label style={{ minWidth: 60, fontSize: 12 }}>解析</label><input value={explanation} onChange={e => setExplanation(e.target.value)} style={{ flex: 1 }} /></div>
      <div style={row}>
        <label style={{ fontSize: 12 }}>难度</label><input type="number" min={1} max={5} value={difficulty} onChange={e => setDifficulty(Number(e.target.value))} style={{ width: 50 }} />
        <label style={{ fontSize: 12 }}>CEFR</label><input value={cefr} onChange={e => setCefr(e.target.value)} style={{ width: 60 }} />
      </div>
      {parseErr && <div style={{ color: "var(--esp-color-error, #dc2626)", fontSize: 11 }}>{parseErr}</div>}
      <div style={row}>
        <button className="primary" disabled={busy} onClick={handleSubmit}>创建</button>
        <button onClick={onCancel} disabled={busy}>取消</button>
      </div>
    </div>
  );
}

function CourseRow({ c, onSelect, sel }: { c: ContentCourse; onSelect: () => void; sel: boolean }) {
  return (
    <div
      onClick={onSelect}
      style={{
        cursor: "pointer", padding: "8px", marginTop: 6, borderRadius: 6,
        border: sel ? "1px solid var(--esp-primary, #0ea5e9)" : "1px solid var(--esp-color-border, #f3f4f6)",
        background: sel ? "var(--esp-primary-bg, #f0f9ff)" : "var(--esp-color-bg, #fff)", display: "flex", justifyContent: "space-between", alignItems: "center",
      }}
    >
      <div>
        <div style={{ fontSize: 13, fontWeight: 600 }}>{c.title}</div>
        <div style={{ fontSize: 11, color: "var(--esp-color-text-secondary, #777)" }}>{c.code} · {c.exam_tag} · CEFR {c.cefr_range}</div>
      </div>
      <span style={{ fontSize: 11, color: "var(--esp-color-text-on, #fff)", background: STATUS_COLOR[c.status] || "var(--esp-color-text-muted, #999)", padding: "1px 8px", borderRadius: 10 }}>{c.status}</span>
    </div>
  );
}

export default function ContentAdmin() {
  const [courses, setCourses] = useState<ContentCourse[]>([]);
  const [lessons, setLessons] = useState<ContentLesson[]>([]);
  const [items, setItems] = useState<ContentItem[]>([]);
  const [selCourse, setSelCourse] = useState<ContentCourse | null>(null);
  const [selLesson, setSelLesson] = useState<ContentLesson | null>(null);
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");
  const [importJSON, setImportJSON] = useState("");
  const [busy, setBusy] = useState(false);
  // 表单控制
  const [showCourseForm, setShowCourseForm] = useState(false);
  const [showLessonForm, setShowLessonForm] = useState(false);
  const [showItemForm, setShowItemForm] = useState(false);

  const loadCourses = useCallback(async () => {
    try {
      const list = await api.listCourses(undefined, "all");
      setCourses(list); setErr("");
    } catch (e) { setErr(`加载课程失败: ${e}`); }
  }, []);

  useEffect(() => { loadCourses(); }, [loadCourses]);

  async function openCourse(c: ContentCourse) {
    setSelCourse(c); setSelLesson(null); setItems([]);
    try { setLessons(await api.listLessons(c.id)); } catch (e) { setErr(`加载课时失败: ${e}`); }
  }

  async function openLesson(l: ContentLesson) {
    setSelLesson(l); setItems([]);
    try { setItems(await api.listItems(l.id)); } catch (e) { setErr(`加载题目失败: ${e}`); }
  }

  async function doCreateCourse(v: { code: string; title: string; exam_tag: string; cefr_range: string; description: string }) {
    setBusy(true);
    try {
      const r = await api.createCourse(v);
      setMsg(`课程已创建 id=${r.id}`); setShowCourseForm(false); await loadCourses();
    } catch (e) { setErr(`创建课程失败: ${e}`); } finally { setBusy(false); }
  }

  async function doCreateLesson(v: { skill: string; title: string; seq: number; cefr: string }) {
    if (!selCourse) return;
    setBusy(true);
    try {
      const r = await api.createLesson({ course_id: selCourse.id, ...v });
      setMsg(`课时已创建 id=${r.id}`); setShowLessonForm(false); await openCourse(selCourse);
    } catch (e) { setErr(`创建课时失败: ${e}`); } finally { setBusy(false); }
  }

  async function doCreateItem(v: { item_type: string; prompt: object; answer: object; explanation: string; difficulty: number; cefr: string }) {
    if (!selLesson) return;
    setBusy(true);
    try {
      const r = await api.createItem({
        lesson_id: selLesson.id, ...v, tags: [], source: "esp-admin", license: "esp-original",
      });
      setMsg(`题目已创建 id=${r.id}`); setShowItemForm(false); await openLesson(selLesson);
    } catch (e) { setErr(`创建题目失败: ${e}`); } finally { setBusy(false); }
  }

  async function setCourseStatus(c: ContentCourse, status: string) {
    try {
      await api.updateCourse(c.id, { title: c.title, status, cefr_range: c.cefr_range });
      setMsg(`课程 ${c.code} -> ${status}`); await loadCourses();
    } catch (e) { setErr(`状态更新失败: ${e}`); }
  }

  async function doImport() {
    if (!selCourse) { setErr("请先选择课程"); return; }
    setBusy(true);
    try {
      const arr = JSON.parse(importJSON);
      const r = await api.importContent(arr);
      setMsg(`导入成功 ${r.created} 条`);
      if (selLesson) await openLesson(selLesson);
      setImportJSON("");
    } catch (e) { setErr(`导入失败: ${e}`); } finally { setBusy(false); }
  }

  return (
    <div>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8 }}>
        <h2>内容库管理</h2>
        <div style={{ display: "flex", gap: 8 }}>
          <button className="primary" onClick={() => setShowCourseForm(true)} disabled={busy}>+ 新建课程</button>
          <a href="/web/admin" style={{ fontSize: 13 }}>&larr; 返回运维台</a>
        </div>
      </div>
      {err && <p className="err">{err}</p>}
      {msg && <p style={{ color: "var(--esp-success, #166534)", fontSize: 13 }}>{msg}</p>}

      {showCourseForm && <CourseForm onSubmit={doCreateCourse} onCancel={() => setShowCourseForm(false)} busy={busy} />}

      <div style={{ display: "grid", gridTemplateColumns: "260px 220px 1fr", gap: 12, alignItems: "start" }}>
        <div style={{ border: "1px solid var(--esp-color-border, #e5e7eb)", borderRadius: 8, padding: 8 }}>
          <b style={{ fontSize: 13 }}>课程(全部状态)</b>
          {courses.map((c) => <CourseRow key={c.id} c={c} sel={selCourse?.id === c.id} onSelect={() => openCourse(c)} />)}
          {!courses.length && <div style={{ fontSize: 12, color: "var(--esp-color-text-muted, #999)", marginTop: 6 }}>无课程</div>}
          {selCourse && (
            <div style={{ marginTop: 8, display: "flex", gap: 6 }}>
              <button style={{ fontSize: 11 }} onClick={() => setCourseStatus(selCourse, selCourse.status === "published" ? "archived" : "published")}>
                {selCourse.status === "published" ? "归档" : "发布"}
              </button>
            </div>
          )}
        </div>

        <div style={{ border: "1px solid var(--esp-color-border, #e5e7eb)", borderRadius: 8, padding: 8 }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <b style={{ fontSize: 13 }}>课时</b>
            {selCourse && <button onClick={() => setShowLessonForm(true)} disabled={busy} style={{ fontSize: 11 }}>+ 课时</button>}
          </div>
          {showLessonForm && <LessonForm onSubmit={doCreateLesson} onCancel={() => setShowLessonForm(false)} busy={busy} />}
          {lessons.map((l) => (
            <div
              key={l.id} onClick={() => openLesson(l)}
              style={{
                cursor: "pointer", padding: "8px", marginTop: 6, borderRadius: 6,
                border: selLesson?.id === l.id ? "1px solid var(--esp-primary, #0ea5e9)" : "1px solid var(--esp-color-border, #f3f4f6)",
                background: selLesson?.id === l.id ? "var(--esp-primary-bg, #f0f9ff)" : "var(--esp-color-bg, #fff)",
              }}
            >
              <div style={{ fontSize: 13, fontWeight: 600 }}>{l.title}</div>
              <div style={{ fontSize: 11, color: "var(--esp-color-text-secondary, #777)" }}>{SKILL_LABEL[l.skill] || l.skill} · seq {l.seq}</div>
            </div>
          ))}
          {!selCourse && <div style={{ fontSize: 12, color: "var(--esp-color-text-muted, #999)", marginTop: 6 }}>先选课程</div>}
        </div>

        <div>
          {selLesson && (
            <>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 8 }}>
                <span style={{ fontSize: 13, color: "var(--esp-color-text-secondary, #666)" }}>{selLesson.title} · {items.length} 题</span>
                <button onClick={() => setShowItemForm(true)} disabled={busy} style={{ fontSize: 11 }}>+ 新建题目</button>
              </div>
              {showItemForm && <ItemForm onSubmit={doCreateItem} onCancel={() => setShowItemForm(false)} busy={busy} />}
              <div style={{ maxHeight: 420, overflow: "auto", border: "1px solid var(--esp-color-border, #f3f4f6)", borderRadius: 8 }}>
                {items.map((it) => (
                  <div key={it.id} style={{ padding: "6px 10px", borderBottom: "1px solid var(--esp-color-border, #f3f4f6)", display: "flex", justifyContent: "space-between", alignItems: "center", gap: 8 }}>
                    <div style={{ fontSize: 12, flex: 1, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      <b>{it.item_type}</b> · {it.prompt?.word || it.prompt?.title || it.prompt?.question || "(无标题)"}
                      <span style={{ color: "var(--esp-color-text-muted, #999)" }}> · {it.cefr} · 难度{it.difficulty}</span>
                    </div>
                  </div>
                ))}
                {!items.length && <div style={{ fontSize: 12, color: "var(--esp-color-text-muted, #999)", padding: 10 }}>无题目</div>}
              </div>
            </>
          )}

          {selCourse && (
            <div style={{ marginTop: 12 }}>
              <b style={{ fontSize: 13 }}>批量导入(JSON 数组,需含 lesson_id)</b>
              <textarea
                value={importJSON}
                onChange={(e) => setImportJSON(e.target.value)}
                placeholder='[{"lesson_id":120...,"item_type":"vocab","prompt":{...},"answer":{...},"difficulty":3,"cefr":"B1"}]'
                style={{ width: "100%", minHeight: 90, fontSize: 12, marginTop: 4 }}
              />
              <button className="primary" onClick={doImport} disabled={busy || !importJSON} style={{ marginTop: 6 }}>
                {busy ? "处理中..." : "导入"}
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
