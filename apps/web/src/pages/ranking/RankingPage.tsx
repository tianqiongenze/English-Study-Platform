import { useEffect, useState } from "react";
import { api } from "../../api";
import { useI18n, MedalIcon } from "@esp/ui-tokens";
import type {
  CommunityCircle,
  RankingEntry,
  RankingGap,
  RankingMine,
  RegionOption,
} from "@esp/types";

// 排行榜(功能拓展阶段3):XP/进度双榜 × 全球/地区/圈子/类别维度 × 日/周/月周期。
// 数据源 = gamification-go rankings 端点(Redis ZSET 主存,2s 近实时扇出自 learning_events)。

const PERIODS: { key: "daily" | "weekly" | "monthly"; label: string }[] = [
  { key: "daily", label: "今日" },
  { key: "weekly", label: "本周" },
  { key: "monthly", label: "本月" },
];
// category 桶的来源:content 事件 category=item_type(vocab/reading/listening/grammar/cloze),
// 其余事件回落到类型名(answer/practice_attempt/…),见 gamification-go categoryOf。
const CATEGORIES: { key: string; label: string }[] = [
  { key: "vocab", label: "单词" },
  { key: "reading", label: "阅读" },
  { key: "listening", label: "听力" },
  { key: "grammar", label: "语法" },
  { key: "cloze", label: "完形" },
  { key: "answer", label: "题库练习" },
  { key: "practice_attempt", label: "自适应练习" },
  { key: "session", label: "会话场景" },
  { key: "speech", label: "口语" },
  { key: "writing", label: "写作" },
];

function fmtScore(v: number): string {
  const n = Math.round(v);
  return n.toLocaleString();
}

function medalTier(rank: number): "gold" | "silver" | "bronze" {
  return rank === 1 ? "gold" : rank === 2 ? "silver" : "bronze";
}

export default function RankingPage() {
  const { t } = useI18n();
  const [type, setType] = useState<"xp" | "prog">("xp");
  const [period, setPeriod] = useState<"daily" | "weekly" | "monthly">("weekly");
  const [scope, setScope] = useState<"global" | "region" | "circle" | "category">("global");
  const [regions, setRegions] = useState<RegionOption[]>([]);
  const [regionCode, setRegionCode] = useState("");
  const [circles, setCircles] = useState<CommunityCircle[]>([]);
  const [circleId, setCircleId] = useState("");
  const [cat, setCat] = useState("vocab");
  const [list, setList] = useState<RankingEntry[]>([]);
  const [mine, setMine] = useState<RankingMine | null>(null);
  const [gap, setGap] = useState<RankingGap | null>(null);
  const [asOf, setAsOf] = useState("");
  const [optedOut, setOptedOut] = useState(false);
  const [err, setErr] = useState("");
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.rankingRegions().then(setRegions).catch((e) => setErr(`加载地域失败: ${e}`));
    api.listCircles("", true).then(setCircles).catch(() => {});
  }, []);

  function scopeId(): string {
    if (scope === "region") return regionCode;
    if (scope === "circle") return circleId;
    if (scope === "category") return cat;
    return "all";
  }

  async function load() {
    if (scope === "region" && !regionCode) return;
    if (scope === "circle" && !circleId) return;
    setErr("");
    try {
      const r = await api.rankings({ type, scope, scope_id: scopeId(), period, limit: 50 });
      setList(r.list);
      setMine(r.mine && "rank" in r.mine ? (r.mine as RankingMine) : null);
      setAsOf(r.as_of);
    } catch (e) { setErr(`加载榜单失败: ${e}`); }
    try {
      const g = await api.rankingGap({ type, scope, scope_id: scopeId(), period });
      setGap(g);
      if (g.opted_out) setOptedOut(true);
    } catch { /* gap 非必需(未登录身份时 401) */ }
  }

  useEffect(() => { load(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [type, period, scope, regionCode, circleId, cat]);

  function pickRegion(code: string) {
    setRegionCode(code);
    setScope("region");
  }

  async function toggleOptOut() {
    const next = !optedOut;
    setBusy(true);
    try {
      await api.setRankingOptOut(next);
      setOptedOut(next);
      setMsg(next ? t("ranking.optout") : t("ranking.optin"));
      await load();
    } catch (e) { setErr(`设置失败: ${e}`); } finally { setBusy(false); }
  }

  function scoreLabel(): string {
    return type === "xp" ? t("ranking.xp") : "学习进度";
  }

  function scopeLabel(): string {
    if (scope === "global") return t("ranking.global");
    if (scope === "region") return regionCode ? regionCode.split("/").pop() ?? regionCode : t("ranking.region");
    if (scope === "circle") {
      const c = circles.find((x) => String(x.id) === circleId);
      return c ? c.name : t("community.title");
    }
    return CATEGORIES.find((x) => x.key === cat)?.label ?? "类别";
  }

  return (
    <div className="motion-fade">
      {/* 工具栏:类型 / 周期 / 维度 / opt-out */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 8 }}>
        <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
          <span style={{ fontSize: 13, color: "var(--esp-color-text-secondary)" }}>{t("ranking.title")}:</span>
          {(["xp", "prog"] as const).map((p) => (
            <button key={p} className={`primary ${type === p ? "active" : ""}`} onClick={() => setType(p)} style={{ fontSize: 12 }}>
              {p === "xp" ? t("ranking.xp") : "学习进度"}
            </button>
          ))}
          <span style={{ display: "inline-block", width: 1, height: 16, background: "var(--esp-color-border)" }} />
          {PERIODS.map((p) => (
            <button key={p.key} className={`primary ${period === p.key ? "active" : ""}`} onClick={() => setPeriod(p.key)} style={{ fontSize: 12 }}>
              {p.label}
            </button>
          ))}
        </div>
        <label style={{ fontSize: 12, display: "flex", alignItems: "center", gap: 4, color: "var(--esp-color-text-secondary)" }}>
          <input type="checkbox" checked={optedOut} onChange={toggleOptOut} disabled={busy} />
          {t("ranking.optin")}
        </label>
      </div>

      {/* 维度 tab + 上下文选择器 */}
      <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", marginTop: 10 }}>
        {(
          [
            { key: "global", label: t("ranking.global") },
            { key: "region", label: t("ranking.region") },
            { key: "circle", label: t("community.title") },
            { key: "category", label: "类别" },
          ] as { key: typeof scope; label: string }[]
        ).map((s) => (
          <button key={s.key} className={`primary ${scope === s.key ? "active" : ""}`} onClick={() => setScope(s.key)} style={{ fontSize: 12 }}>
            {s.label}
          </button>
        ))}
        {scope === "region" && (
          <select value={regionCode} onChange={(e) => pickRegion(e.target.value)} style={{ fontSize: 12 }}>
            {regions.length === 0 && <option value="">无地域数据</option>}
            {regions.map((r) => (
              <option key={r.code} value={r.code}>{r.label}({r.code})</option>
            ))}
          </select>
        )}
        {scope === "circle" && (
          <select value={circleId} onChange={(e) => setCircleId(e.target.value)} style={{ fontSize: 12 }}>
            {circles.length === 0 && <option value="">未加入圈子</option>}
            {circles.map((c) => (
              <option key={c.id} value={String(c.id)}>{c.name}</option>
            ))}
          </select>
        )}
        {scope === "category" && (
          <select value={cat} onChange={(e) => setCat(e.target.value)} style={{ fontSize: 12 }}>
            {CATEGORIES.map((c) => (
              <option key={c.key} value={c.key}>{c.label}</option>
            ))}
          </select>
        )}
      </div>

      {err && <p className="err">{err}</p>}
      {msg && <p style={{ color: "var(--esp-color-success)", fontSize: 13 }}>{msg}</p>}
      {optedOut && (
        <p style={{ fontSize: 12, color: "var(--esp-color-warning)", background: "var(--esp-color-bg-subtle)", border: "1px solid var(--esp-color-warning)", borderRadius: 6, padding: "6px 8px" }}>
          你已退出排行,榜单不会展示你。可勾选右上「{t("ranking.optin")}」重新加入。
        </p>
      )}

      {/* 我的排名卡 */}
      <div className="card card--hover" style={{ display: "flex", gap: 24, flexWrap: "wrap", margin: "12px 0", flexDirection: "row" }}>
        {mine ? (
          <>
            <div style={{ fontSize: 13 }}>
              <span style={{ color: "var(--esp-color-text-tertiary)" }}>{scopeLabel()} · {type === "xp" ? t("ranking.xp") : "进度"} · {PERIODS.find((p) => p.key === period)?.label}</span>
              <div style={{ fontSize: 20, fontWeight: 700, marginTop: 2, color: "var(--esp-color-text)" }}>
                {mine.rank > 0 ? `第 ${mine.rank} 名` : "未上榜"}
              </div>
            </div>
            <div style={{ fontSize: 13 }}>
              <div style={{ color: "var(--esp-color-text-tertiary)" }}>{scoreLabel()}</div>
              <div style={{ fontSize: 18, fontWeight: 600, color: "var(--esp-brand)" }}>{fmtScore(mine.score)}</div>
            </div>
            <div style={{ fontSize: 13 }}>
              <div style={{ color: "var(--esp-color-text-tertiary)" }}>距第 1 名</div>
              <div style={{ fontSize: 18, fontWeight: 600 }}>{fmtScore(mine.gap_to_top)}</div>
            </div>
            {gap && (
              <>
                <div style={{ fontSize: 13 }}>
                  <div style={{ color: "var(--esp-color-text-tertiary)" }}>超越</div>
                  <div style={{ fontSize: 18, fontWeight: 600 }}>{gap.percentile.toFixed(1)}%</div>
                </div>
                <div style={{ fontSize: 13 }}>
                  <div style={{ color: "var(--esp-color-text-tertiary)" }}>榜内人数</div>
                  <div style={{ fontSize: 18, fontWeight: 600 }}>{gap.total}</div>
                </div>
              </>
            )}
          </>
        ) : (
          <div style={{ fontSize: 13, color: "var(--esp-color-text-tertiary)" }}>
            完成学习动作(作答/口语/写作/内容学习)后,你的排名会显示在这里(约 2 秒内刷新)。
          </div>
        )}
      </div>

      {/* 榜单列表 */}
      <div className="card" style={{ paddingTop: 12 }}>
        <div style={{ display: "flex", justifyContent: "space-between", fontSize: 11, color: "var(--esp-color-text-tertiary)", padding: "0 8px 4px" }}>
          <span>排名 · {scopeLabel()} · {scoreLabel()}</span>
          {asOf && <span>更新于 {new Date(asOf).toLocaleTimeString()}</span>}
        </div>
        {list.map((r) => (
          <div key={r.user_id} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", borderBottom: "1px solid var(--esp-color-border)", padding: "10px 8px" }}>
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              {r.rank <= 3 ? (
                <span className={`rank-medal rank-medal--${medalTier(r.rank)}`}>
                  <MedalIcon tier={medalTier(r.rank)} size={16} />
                  {r.rank}
                </span>
              ) : (
                <span className="rank-badge">{r.rank}</span>
              )}
              <span style={{ fontSize: 14, fontWeight: r.rank <= 3 ? 600 : 400 }}>{r.name || `用户#${r.user_id}`}</span>
            </div>
            <span style={{ fontSize: 14, fontWeight: 600, color: r.rank === 1 ? "var(--esp-brand)" : "var(--esp-color-text)" }}>{fmtScore(r.score)}</span>
          </div>
        ))}
        {!list.length && !err && (
          <div style={{ fontSize: 13, color: "var(--esp-color-text-tertiary)", padding: 16, textAlign: "center" }}>
            {scope === "circle" && !circles.length ? "先加入圈子,才能查看圈子榜" : scope === "region" && !regions.length ? "暂无地域维度数据" : "该榜单暂无数据"}
          </div>
        )}
      </div>
    </div>
  );
}