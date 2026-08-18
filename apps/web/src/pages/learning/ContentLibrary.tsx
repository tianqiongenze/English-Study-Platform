import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "../../api";
import { useI18n, type MessageKey } from "@esp/ui-tokens";
import SafeImg, { UNSPLASH } from "../../components/SafeImg";
import type { ContentCourse, ContentItem, ContentLesson } from "@esp/types";

// 内容库:统一共享学习内容(四六级/托福/雅思)。浏览课程→课时→题目,逐题作答。
// 数据源 content-go(经 BFF /api/content),X-User-Id 由 BFF 注入。

const EXAMS = [
  { tag: "", label: "全部" },
  { tag: "CET4", label: "四级 CET-4" },
  { tag: "CET6", label: "六级 CET-6" },
  { tag: "TOEFL", label: "托福 TOEFL" },
  { tag: "IELTS", label: "雅思 IELTS" },
];

const SKILL_LABEL: Record<string, string> = {
  vocab: "词汇", reading: "阅读", listening: "听力", grammar: "语法",
};

function ItemCard({ item, onDone, t }: { item: ContentItem; onDone: () => void; t: (k: MessageKey, vars?: Record<string, string | number>) => string }) {
  const [pick, setPick] = useState<number | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<{ correct: boolean; score: number; explanation: string } | null>(null);
  const p = item.prompt || {};

  const isMC = item.item_type !== "vocab";

  async function answer(optionIndex?: number) {
    if (submitting || result) return;
    setSubmitting(true);
    try {
      const body = isMC ? { option_index: optionIndex } : {};
      const r = await api.submitAnswer(item.id, body, 0);
      setResult(r);
      setPick(optionIndex ?? null);
      if (r.correct) onDone();
    } catch (e) {
      setResult({ correct: false, score: 0, explanation: `提交失败: ${e}` });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="card" style={{ padding: 14, margin: "8px 0" }}>
      {isMC ? (
        <>
          <p style={{ margin: "0 0 8px" }}>{p.question || p.title}</p>
          {isMC && p.text && <details style={{ fontSize: 13, color: "var(--esp-color-text-secondary)", marginBottom: 8 }}><summary>阅读全文</summary><p style={{ whiteSpace: "pre-wrap" }}>{p.text}</p></details>}
          {isMC && p.script && <details style={{ fontSize: 13, color: "var(--esp-color-text-secondary)", marginBottom: 8 }}><summary>听力原文</summary><p style={{ whiteSpace: "pre-wrap" }}>{p.script}</p></details>}
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {(p.options || []).map((opt, i) => {
              const chosen = pick === i;
              const correct = result ? (result.correct ? chosen : i === 0 && false) : false;
              return (
                <button
                  key={i}
                  disabled={!!result || submitting}
                  onClick={() => answer(i)}
                  style={{
                    textAlign: "left", padding: "8px 10px", borderRadius: 6,
                    border: chosen ? (result?.correct ? "1px solid var(--esp-color-success)" : "1px solid var(--esp-color-error)") : "1px solid var(--esp-color-border)",
                    background: chosen ? "var(--esp-color-bg-subtle)" : "var(--esp-color-bg-container)",
                    color: "var(--esp-color-text)",
                  }}
                >
                  {String.fromCharCode(65 + i)}. {opt}
                </button>
              );
            })}
          </div>
        </>
      ) : (
        <>
          <div style={{ display: "flex", gap: 10, alignItems: "flex-start" }}>
            <div style={{ fontSize: 20, fontWeight: 700 }}>{p.word}</div>
            <div style={{ fontSize: 13, color: "var(--esp-color-text-secondary)" }}>
              {p.phonetic ? `/${p.phonetic}/` : ""} {p.pos ? `<${p.pos}>` : ""}
            </div>
            {item.cefr && <span style={{ fontSize: 11, color: "var(--esp-color-success)", background: "var(--esp-color-bg-subtle)", padding: "1px 8px", borderRadius: 10 }}>{item.cefr}</span>}
          </div>
          <div style={{ fontSize: 14, margin: "6px 0" }}>
            <div><b>{p.meaning_en}</b></div>
            <div style={{ color: "var(--esp-color-text-secondary)" }}>{p.meaning_zh}</div>
          </div>
          {p.example && <div style={{ fontSize: 13, color: "var(--esp-color-text-secondary)", fontStyle: "italic" }}>例: {p.example}</div>}
          <button className="primary" disabled={!!result || submitting} onClick={() => answer()} style={{ marginTop: 8 }}>
            {submitting ? "提交中…" : result ? "已完成" : "标记已掌握"}
          </button>
        </>
      )}
      {result && (
        <div style={{ marginTop: 8, fontSize: 13, color: result.correct ? "var(--esp-color-success)" : "var(--esp-color-error)" }}>
          {result.correct ? "✓ 正确" : "✗ 未答对"}{result.score ? ` · ${t("result.score")} ${result.score}` : ""}
          {result.explanation && <div style={{ color: "var(--esp-color-text-secondary)", marginTop: 4 }}>{result.explanation}</div>}
        </div>
      )}
    </div>
  );
}

export default function ContentLibrary() {
  const { t } = useI18n();
  const [examTag, setExamTag] = useState("");
  const [courses, setCourses] = useState<ContentCourse[]>([]);
  const [lessons, setLessons] = useState<ContentLesson[]>([]);
  const [items, setItems] = useState<ContentItem[]>([]);
  const [selCourse, setSelCourse] = useState<ContentCourse | null>(null);
  const [selLesson, setSelLesson] = useState<ContentLesson | null>(null);
  const [err, setErr] = useState("");

  const loadCourses = useCallback(async () => {
    try {
      const list = await api.listCourses(examTag);
      setCourses(list);
      setErr("");
    } catch (e) {
      setErr(`加载课程失败: ${e}`);
    }
  }, [examTag]);

  useEffect(() => { loadCourses(); }, [loadCourses]);

  async function openCourse(c: ContentCourse) {
    setSelCourse(c); setSelLesson(null); setItems([]);
    try {
      setLessons(await api.listLessons(c.id));
    } catch (e) { setErr(`加载课时失败: ${e}`); }
  }

  async function openLesson(l: ContentLesson) {
    setSelLesson(l); setItems([]);
    try {
      setItems(await api.listItems(l.id));
    } catch (e) { setErr(`加载题目失败: ${e}`); }
  }

  const totalItems = useMemo(() => items.length, [items]);

  return (
    <div className="motion-fade">
      {/* 内容库横幅(森林绿阅读主题) */}
      <SafeImg src={UNSPLASH.forest} alt="" tone={3} className="scene-banner">
        <span className="scene-banner__label">{t("content.title")}</span>
      </SafeImg>

      <h2>{t("content.title")}</h2>
      <div style={{ display: "flex", gap: 8, marginBottom: 10, flexWrap: "wrap" }}>
        {EXAMS.map((e) => (
          <button
            key={e.tag || "all"}
            className={examTag === e.tag ? "primary" : ""}
            onClick={() => setExamTag(e.tag)}
            style={examTag === e.tag ? undefined : { background: "var(--esp-color-bg-container)", border: "1px solid var(--esp-color-border)" }}
          >
            {e.label}
          </button>
        ))}
      </div>
      {err && <p className="err">{err}</p>}

      <div style={{ display: "grid", gridTemplateColumns: "240px 220px 1fr", gap: 12, alignItems: "start" }}>
        {/* 课程 */}
        <div className="card" style={{ padding: 8 }}>
          <b style={{ fontSize: 13 }}>课程</b>
          {courses.map((c) => (
            <div
              key={c.id}
              onClick={() => openCourse(c)}
              className="card--hover"
              style={{
                cursor: "pointer", padding: "8px", marginTop: 6, borderRadius: 6,
                border: selCourse?.id === c.id ? "1px solid var(--esp-brand)" : "1px solid var(--esp-color-border)",
                background: selCourse?.id === c.id ? "var(--esp-brand-soft)" : "var(--esp-color-bg-container)",
                transition: "all var(--esp-motion-duration-fast) var(--esp-motion-ease)",
              }}
            >
              <div style={{ fontSize: 13, fontWeight: 600 }}>{c.title}</div>
              <div style={{ fontSize: 11, color: "var(--esp-color-text-tertiary)" }}>{c.exam_tag} · CEFR {c.cefr_range}</div>
            </div>
          ))}
          {!courses.length && <div style={{ fontSize: 12, color: "var(--esp-color-text-tertiary)", marginTop: 6 }}>无课程</div>}
        </div>

        {/* 课时 */}
        <div className="card" style={{ padding: 8 }}>
          <b style={{ fontSize: 13 }}>课时</b>
          {lessons.map((l) => (
            <div
              key={l.id}
              onClick={() => openLesson(l)}
              className="card--hover"
              style={{
                cursor: "pointer", padding: "8px", marginTop: 6, borderRadius: 6,
                border: selLesson?.id === l.id ? "1px solid var(--esp-brand)" : "1px solid var(--esp-color-border)",
                background: selLesson?.id === l.id ? "var(--esp-brand-soft)" : "var(--esp-color-bg-container)",
                transition: "all var(--esp-motion-duration-fast) var(--esp-motion-ease)",
              }}
            >
              <div style={{ fontSize: 13, fontWeight: 600 }}>{l.title}</div>
              <div style={{ fontSize: 11, color: "var(--esp-color-text-tertiary)" }}>{SKILL_LABEL[l.skill] || l.skill} · {l.cefr || "-"}</div>
            </div>
          ))}
          {!selCourse && <div style={{ fontSize: 12, color: "var(--esp-color-text-tertiary)", marginTop: 6 }}>先选课程</div>}
          {selCourse && !lessons.length && <div style={{ fontSize: 12, color: "var(--esp-color-text-tertiary)", marginTop: 6 }}>无课时</div>}
        </div>

        {/* 题目 */}
        <div>
          {selLesson ? (
            <>
              <div style={{ fontSize: 13, color: "var(--esp-color-text-secondary)", marginBottom: 6 }}>
                {selLesson.title} · {totalItems} 题
              </div>
              {items.map((it) => <ItemCard key={it.id} item={it} onDone={() => {}} t={t} />)}
              {!items.length && <div style={{ color: "var(--esp-color-text-tertiary)" }}>该课时暂无题目</div>}
            </>
          ) : (
            <div style={{ color: "var(--esp-color-text-tertiary)", padding: 20, textAlign: "center", border: "1px dashed var(--esp-color-border)", borderRadius: 8 }}>
              选择左侧课程与课时开始学习
            </div>
          )}
        </div>
      </div>
    </div>
  );
}