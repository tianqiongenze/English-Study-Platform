import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import { api } from "../../api";
import { useI18n, MedalIcon } from "@esp/ui-tokens";
import type { CommunityCircle, RankingEntry, RankingGap, RankingMine, RegionOption } from "@esp/types";
import { Card, Segmented, Select, Table, Typography, Statistic, Row, Col, Progress, Tag, Empty, Alert, Switch, Space } from "antd";
import { fadeUp, stagger, EASE } from "../../motion";

const { Title, Text } = Typography;

// 排行榜 —— antd Segmented + Table + 领奖台动画 + recharts 风格 Statistic。
const PERIODS = [{ key: "daily", label: "今日" }, { key: "weekly", label: "本周" }, { key: "monthly", label: "本月" }] as const;
const CATEGORIES = [
  { key: "vocab", label: "单词" }, { key: "reading", label: "阅读" }, { key: "listening", label: "听力" },
  { key: "grammar", label: "语法" }, { key: "cloze", label: "完形" }, { key: "answer", label: "题库练习" },
  { key: "practice_attempt", label: "自适应练习" }, { key: "session", label: "会话场景" },
  { key: "speech", label: "口语" }, { key: "writing", label: "写作" },
];

function fmtScore(v: number): string { return Math.round(v).toLocaleString(); }
function medalTier(rank: number): "gold" | "silver" | "bronze" { return rank === 1 ? "gold" : rank === 2 ? "silver" : "bronze"; }

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
    } catch { /* gap 非必需 */ }
  }
  useEffect(() => { load(); /* eslint-disable-next-line */ }, [type, period, scope, regionCode, circleId, cat]);

  async function toggleOptOut() {
    const next = !optedOut;
    setBusy(true);
    try { await api.setRankingOptOut(next); setOptedOut(next); setMsg(next ? t("ranking.optout") : t("ranking.optin")); await load(); }
    catch (e) { setErr(`设置失败: ${e}`); } finally { setBusy(false); }
  }

  function scoreLabel() { return type === "xp" ? t("ranking.xp") : "学习进度"; }
  function scopeLabel() {
    if (scope === "global") return t("ranking.global");
    if (scope === "region") return regionCode ? (regionCode.split("/").pop() ?? regionCode) : t("ranking.region");
    if (scope === "circle") { const c = circles.find((x) => String(x.id) === circleId); return c ? c.name : t("community.title"); }
    return CATEGORIES.find((x) => x.key === cat)?.label ?? "类别";
  }

  // 前三名领奖台(动画)
  const top3 = list.slice(0, 3);
  const podiumOrder = [top3[1], top3[0], top3[2]].filter(Boolean); // 银金铜 居中排列

  const columns = [
    {
      title: "排名", dataIndex: "rank", key: "rank", width: 80,
      render: (rank: number) => rank <= 3 ? (
        <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
          <MedalIcon tier={medalTier(rank)} size={18} />
          <Text strong style={{ color: rank === 1 ? "#faad14" : undefined }}>{rank}</Text>
        </span>
      ) : <Tag>{rank}</Tag>,
    },
    { title: "用户", dataIndex: "name", key: "name", render: (name: string, r: RankingEntry) => name || `用户#${r.user_id}` },
    { title: scoreLabel(), dataIndex: "score", key: "score", align: "right" as const,
      render: (s: number, r: RankingEntry) => <Text strong style={{ color: r.rank === 1 ? "#1677ff" : undefined }}>{fmtScore(s)}</Text> },
  ];

  return (
    <motion.div variants={stagger} initial="hidden" animate="visible" className="motion-fade">
      {/* 工具栏 */}
      <motion.div variants={fadeUp}>
        <Card size="small" style={{ marginBottom: 12 }}>
          <Space wrap style={{ width: "100%", justifyContent: "space-between" }}>
            <Space wrap>
              <Segmented value={type} onChange={(v) => setType(v as "xp" | "prog")}
                options={[{ label: t("ranking.xp"), value: "xp" }, { label: "学习进度", value: "prog" }]} size="small" />
              <Segmented value={period} onChange={(v) => setPeriod(v as "daily" | "weekly" | "monthly")}
                options={PERIODS.map((p) => ({ label: p.label, value: p.key }))} size="small" />
            </Space>
            <Space size="small">
              <Text type="secondary" style={{ fontSize: 12 }}>{t("ranking.optin")}</Text>
              <Switch checked={optedOut} onChange={toggleOptOut} disabled={busy} size="small" />
            </Space>
          </Space>
        </Card>
      </motion.div>

      <motion.div variants={fadeUp}>
        <Card size="small" style={{ marginBottom: 12 }}>
          <Space wrap>
            <Segmented value={scope} onChange={(v) => setScope(v as typeof scope)} size="small"
              options={[
                { label: t("ranking.global"), value: "global" },
                { label: t("ranking.region"), value: "region" },
                { label: t("community.title"), value: "circle" },
                { label: "类别", value: "category" },
              ]} />
            {scope === "region" && (
              <Select size="small" value={regionCode || undefined} onChange={(v) => { setRegionCode(v); setScope("region"); }} style={{ width: 160 }}
                placeholder="选择地域" options={regions.length ? regions.map((r) => ({ value: r.code, label: `${r.label}(${r.code})` })) : [{ value: "", label: "无地域数据" }]} />
            )}
            {scope === "circle" && (
              <Select size="small" value={circleId || undefined} onChange={setCircleId} style={{ width: 180 }}
                placeholder="选择圈子" options={circles.map((c) => ({ value: String(c.id), label: c.name }))} />
            )}
            {scope === "category" && (
              <Select size="small" value={cat} onChange={setCat} style={{ width: 140 }}
                options={CATEGORIES.map((c) => ({ value: c.key, label: c.label }))} />
            )}
          </Space>
        </Card>
      </motion.div>

      {err && <Alert type="error" message={err} style={{ marginBottom: 12 }} closable />}
      {msg && <Alert type="success" message={msg} style={{ marginBottom: 12 }} />}
      {optedOut && <Alert type="warning" message="你已退出排行,榜单不会展示你。可开启右上开关重新加入。" style={{ marginBottom: 12 }} />}

      {/* 我的排名卡 */}
      <motion.div variants={fadeUp}>
        <Card size="small" style={{ marginBottom: 12 }}>
          {mine ? (
            <Row gutter={16}>
              <Col span={6}><Statistic title={`${scopeLabel()} · ${PERIODS.find((p) => p.key === period)?.label}`} value={mine.rank > 0 ? `第 ${mine.rank} 名` : "未上榜"} /></Col>
              <Col span={6}><Statistic title={scoreLabel()} value={fmtScore(mine.score)} valueStyle={{ color: "var(--esp-brand, #1677ff)" }} /></Col>
              <Col span={6}><Statistic title="距第 1 名" value={fmtScore(mine.gap_to_top)} /></Col>
              {gap && <><Col span={3}><Statistic title="超越" value={`${gap.percentile.toFixed(1)}%`} /></Col><Col span={3}><Statistic title="榜内人数" value={gap.total} /></Col></>}
            </Row>
          ) : (
            <Text type="secondary">完成学习动作(作答/口语/写作)后,你的排名会显示在这里(约 2 秒内刷新)。</Text>
          )}
          {gap && mine && <Progress percent={gap.percentile} size="small" style={{ marginTop: 12 }} />}
        </Card>
      </motion.div>

      {/* 前三名领奖台(动画) */}
      {top3.length === 3 && (
        <motion.div variants={fadeUp}>
          <Card size="small" style={{ marginBottom: 12, background: "var(--esp-color-bg-subtle, #fafafa)" }}>
            <Row gutter={16} align="bottom" justify="center" style={{ textAlign: "center" }}>
              {podiumOrder.map((r, idx) => {
                const realRank = r.rank;
                const height = realRank === 1 ? 100 : realRank === 2 ? 70 : 50;
                const color = realRank === 1 ? "#faad14" : realRank === 2 ? "#8c8c8c" : "#d48806";
                return (
                  <Col key={r.user_id} span={8}>
                    <motion.div initial={{ height: 0, opacity: 0 }} animate={{ height, opacity: 1 }}
                      transition={{ duration: 0.5, ease: EASE, delay: idx * 0.1 }}
                      style={{ background: `linear-gradient(180deg, ${color}33 0%, ${color}11 100%)`, borderRadius: "8px 8px 0 0", padding: "12px 8px", border: `2px solid ${color}`, borderBottom: "none" }}>
                      <MedalIcon tier={medalTier(realRank)} size={28} />
                      <div style={{ fontWeight: 700, marginTop: 4 }}>{r.name || `用户#${r.user_id}`}</div>
                      <div style={{ fontSize: 18, fontWeight: 700, color }}>{fmtScore(r.score)}</div>
                    </motion.div>
                  </Col>
                );
              })}
            </Row>
          </Card>
        </motion.div>
      )}

      {/* 榜单列表 */}
      <motion.div variants={fadeUp}>
        <Card title={`排名 · ${scopeLabel()} · ${scoreLabel()}`} size="small" extra={asOf && <Text type="secondary" style={{ fontSize: 11 }}>更新于 {new Date(asOf).toLocaleTimeString()}</Text>}>
          {list.length ? (
            <Table dataSource={list} columns={columns} rowKey="user_id" size="small" pagination={false} />
          ) : (
            <Empty description={scope === "circle" && !circles.length ? "先加入圈子查看圈子榜" : scope === "region" && !regions.length ? "暂无地域数据" : "该榜单暂无数据"} />
          )}
        </Card>
      </motion.div>
    </motion.div>
  );
}
