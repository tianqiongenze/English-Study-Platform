import { useCallback, useEffect, useState } from "react";
import { api } from "../../api";
import { useI18n } from "@esp/ui-tokens";
import SafeImg, { UNSPLASH } from "../../components/SafeImg";
import type {
  CommunityCircle,
  CommunityComment,
  CommunityMember,
  CommunityPost,
} from "@esp/types";

// 圈子社区(功能拓展阶段2):发现/搜索圈子 → 建圈/邀请码加入 → 圈内发帖/评论 → 举报。
// 视图:列表(发现/我的)⇄ 圈子详情(成员 + 帖子流 + 评论)。

function fmt(t: string): string {
  if (!t) return "";
  const d = new Date(t);
  return isNaN(d.getTime()) ? t : `${d.toLocaleDateString()} ${d.toLocaleTimeString().slice(0, 5)}`;
}

export default function CommunityPage() {
  const { t } = useI18n();
  const [tab, setTab] = useState<"discover" | "mine">("discover");
  const [circles, setCircles] = useState<CommunityCircle[]>([]);
  const [q, setQ] = useState("");
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);
  const [detail, setDetail] = useState<CommunityCircle | null>(null);
  const [members, setMembers] = useState<CommunityMember[]>([]);
  const [posts, setPosts] = useState<CommunityPost[]>([]);
  const [expanded, setExpanded] = useState<number | null>(null);
  const [comments, setComments] = useState<Record<number, CommunityComment[]>>({});
  const [newPost, setNewPost] = useState("");
  const [commentDraft, setCommentDraft] = useState<Record<number, string>>({});

  const load = useCallback(async (mine: boolean) => {
    setErr("");
    try {
      setCircles(await api.listCircles(q, mine));
    } catch (e) { setErr(`加载圈子失败: ${e}`); }
  }, [q]);

  useEffect(() => {
    load(tab === "mine");
  }, [tab, load]);

  async function openDetail(c: CommunityCircle) {
    setErr("");
    try {
      const fresh = await api.getCircle(c.id);
      const [ms, ps] = await Promise.all([api.listCircleMembers(c.id), api.listPosts(c.id)]);
      setDetail(fresh); setMembers(ms); setPosts(ps); setExpanded(null); setComments({});
    } catch (e) { setErr(`打开圈子失败: ${e}`); }
  }

  async function createCircle() {
    const name = prompt("圈子名称:"); if (!name) return;
    const desc = prompt("圈子简介:", "一起学英语"); if (desc === null) return;
    setBusy(true);
    try {
      const r = await api.createCircle({ name, description: desc });
      setMsg(`圈子已创建!邀请码: ${r.invite_code}(分享给同学加入)`);
      await load(tab === "mine");
    } catch (e) { setErr(`创建失败: ${e}`); } finally { setBusy(false); }
  }

  async function join(c: CommunityCircle) {
    const code = prompt(`加入「${c.name}」请输入邀请码:`);
    if (!code) return;
    setBusy(true);
    try {
      await api.joinCircle(c.id, code.trim());
      setMsg("加入成功!");
      await load(tab === "mine");
      if (detail?.id === c.id) await openDetail(c);
    } catch (e) { setErr(`加入失败: ${e}`); } finally { setBusy(false); }
  }

  async function leave(c: CommunityCircle) {
    if (!confirm(`确定退出「${c.name}」?`)) return;
    try {
      await api.leaveCircle(c.id);
      setMsg("已退出圈子");
      setDetail(null);
      await load(tab === "mine");
    } catch (e) { setErr(`退出失败: ${e}`); }
  }

  async function post() {
    if (!detail || !newPost.trim()) return;
    try {
      await api.createPost(detail.id, newPost.trim());
      setNewPost("");
      setPosts(await api.listPosts(detail.id));
    } catch (e) { setErr(`发帖失败: ${e}`); }
  }

  async function toggleComments(p: CommunityPost) {
    if (expanded === p.id) { setExpanded(null); return; }
    setExpanded(p.id);
    try {
      const list = await api.listComments(p.id);
      setComments((prev) => ({ ...prev, [p.id]: list }));
    } catch (e) { setErr(`加载评论失败: ${e}`); }
  }

  async function addComment(p: CommunityPost) {
    const body = (commentDraft[p.id] || "").trim();
    if (!body) return;
    try {
      await api.createComment(p.id, body);
      setCommentDraft((prev) => ({ ...prev, [p.id]: "" }));
      const list = await api.listComments(p.id);
      setComments((prev) => ({ ...prev, [p.id]: list }));
      setPosts(await api.listPosts(detail!.id));
    } catch (e) { setErr(`评论失败: ${e}`); }
  }

  async function reportPost(p: CommunityPost) {
    const reason = prompt("举报原因:"); if (!reason) return;
    try {
      const r = await api.reportContent("post", p.id, reason);
      setMsg(r.auto_hidden ? "已自动隐藏该违规帖" : `已举报(累计 ${r.report_count} 次)`);
      setPosts(await api.listPosts(detail!.id));
    } catch (e) { setErr(`举报失败: ${e}`); }
  }

  if (detail) {
    const joined = detail.joined;
    return (
      <div className="motion-fade">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8 }}>
          <h2 style={{ margin: 0 }}>{detail.name}</h2>
          <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
            <span style={{ fontSize: 12, color: "var(--esp-color-text-tertiary)" }}>
              {detail.owner_name} 创建 · {detail.member_count}/{detail.member_limit} 人 · {joined && detail.my_role === "owner" ? `我是圈主 · 邀请码 ${detail.invite_code}` : ""}
            </span>
            {joined && detail.my_role !== "owner" && (
              <button style={{ fontSize: 11 }} onClick={() => leave(detail)}>退出圈子</button>
            )}
            <button style={{ fontSize: 11 }} onClick={() => setDetail(null)}>← {t("action.back")}</button>
          </div>
        </div>
        <p style={{ fontSize: 13, color: "var(--esp-color-text-secondary)" }}>{detail.description}</p>
        {err && <p className="err">{err}</p>}
        {msg && <p style={{ color: "var(--esp-color-success)", fontSize: 13 }}>{msg}</p>}

        <div style={{ display: "grid", gridTemplateColumns: "220px 1fr", gap: 12, alignItems: "start", marginTop: 8 }}>
          <div className="card" style={{ padding: 8 }}>
            <b style={{ fontSize: 13 }}>成员({members.length})</b>
            {members.map((m) => (
              <div key={m.user_id} style={{ display: "flex", justifyContent: "space-between", fontSize: 12, padding: "4px 0", borderBottom: "1px solid var(--esp-color-border)" }}>
                <span>{m.name || `用户#${m.user_id}`}</span>
                <span style={{ color: m.role === "owner" ? "var(--esp-color-warning)" : "var(--esp-color-text-tertiary)" }}>{m.role === "owner" ? "圈主" : "成员"}</span>
              </div>
            ))}
          </div>

          <div>
            {joined ? (
              <div style={{ display: "flex", gap: 8, marginBottom: 10 }}>
                <input value={newPost} onChange={(e) => setNewPost(e.target.value)} placeholder="分享学习心得/求组队/问经验…" style={{ flex: 1, fontSize: 13 }} />
                <button className="primary" onClick={post}>{t("community.post")}</button>
              </div>
            ) : (
              <div style={{ fontSize: 13, color: "var(--esp-color-text-tertiary)", marginBottom: 10 }}>加入圈子后即可参与讨论</div>
            )}
            {posts.map((p) => (
              <div key={p.id} className="card card--hover" style={{ padding: 10, marginBottom: 8 }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <div style={{ fontSize: 12, color: "var(--esp-color-text-secondary)" }}>{p.user_name || `用户#${p.user_id}`} · {fmt(p.created_at)}</div>
                  {joined && <button style={{ fontSize: 11 }} onClick={() => reportPost(p)}>举报</button>}
                </div>
                <div style={{ fontSize: 14, margin: "6px 0", whiteSpace: "pre-wrap" }}>{p.body}</div>
                <button style={{ fontSize: 11 }} onClick={() => toggleComments(p)}>
                  {t("community.comment")}({p.comment_count}){expanded === p.id ? " · 收起" : ""}
                </button>
                {expanded === p.id && (
                  <div style={{ marginTop: 8 }}>
                    {(comments[p.id] || []).map((c) => (
                      <div key={c.id} style={{ fontSize: 12, padding: "4px 0", borderBottom: "1px solid var(--esp-color-border)" }}>
                        <b>{c.user_name || `用户#${c.user_id}`}:</b> {c.body}
                      </div>
                    ))}
                    {joined && (
                      <div style={{ display: "flex", gap: 6, marginTop: 6 }}>
                        <input
                          value={commentDraft[p.id] || ""}
                          onChange={(e) => setCommentDraft((prev) => ({ ...prev, [p.id]: e.target.value }))}
                          placeholder="回复…" style={{ flex: 1, fontSize: 12 }}
                        />
                        <button style={{ fontSize: 11 }} onClick={() => addComment(p)}>{t("community.publish")}</button>
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
            {!posts.length && <div style={{ fontSize: 13, color: "var(--esp-color-text-tertiary)" }}>{t("community.empty")}</div>}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="motion-fade">
      {/* 圈子横幅 */}
      <SafeImg src={UNSPLASH.forest} alt="" tone={3} className="scene-banner">
        <span className="scene-banner__label">{t("community.title")}</span>
      </SafeImg>

      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8 }}>
        <div>
          <button className={`primary ${tab === "discover" ? "active" : ""}`} onClick={() => setTab("discover")} style={{ marginRight: 8 }}>发现圈子</button>
          <button className={`primary ${tab === "mine" ? "active" : ""}`} onClick={() => setTab("mine")}>我的圈子</button>
        </div>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          {tab === "discover" && (
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="搜索圈子…" style={{ fontSize: 13, width: 180 }} />
          )}
          <button className="primary" onClick={createCircle} disabled={busy}>+ 创建圈子</button>
        </div>
      </div>
      {err && <p className="err">{err}</p>}
      {msg && <p style={{ color: "var(--esp-color-success)", fontSize: 13 }}>{msg}</p>}

      <div style={{ marginTop: 10 }}>
        {circles.map((c) => (
          <div key={c.id} className="card card--hover" style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: 12, marginBottom: 8 }}>
            <div style={{ cursor: "pointer", flex: 1 }} onClick={() => openDetail(c)}>
              <div style={{ fontSize: 14, fontWeight: 600 }}>{c.name}</div>
              <div style={{ fontSize: 12, color: "var(--esp-color-text-secondary)" }}>{c.description}</div>
              <div style={{ fontSize: 11, color: "var(--esp-color-text-tertiary)", marginTop: 2 }}>
                {c.owner_name} 创建 · {c.member_count}/{c.member_limit} 人{c.joined ? " · 已加入" : ""}
              </div>
            </div>
            {!c.joined && <button style={{ fontSize: 12 }} onClick={() => join(c)}>加入</button>}
          </div>
        ))}
        {!circles.length && <div style={{ fontSize: 13, color: "var(--esp-color-text-tertiary)" }}>{tab === "mine" ? "还没有加入任何圈子" : "暂无圈子,点击右上角创建"}</div>}
      </div>
    </div>
  );
}