import { useCallback, useEffect, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { api } from "../../api";
import { useI18n } from "@esp/ui-tokens";
import SafeImg, { UNSPLASH } from "../../components/SafeImg";
import type { CommunityCircle, CommunityComment, CommunityMember, CommunityPost } from "@esp/types";
import { Card, Segmented, Input, Button, Typography, Tag, List, Avatar, Empty, Alert, Modal, Row, Col, Space } from "antd";
import { PlusOutlined, TeamOutlined, MessageOutlined, FlagOutlined, ArrowLeftOutlined } from "@ant-design/icons";
import { fadeUp, stagger, EASE, DUR_SLOW } from "../../motion";

const { Title, Text, Paragraph } = Typography;
const { TextArea } = Input;

function fmt(t: string): string {
  if (!t) return "";
  const d = new Date(t);
  return isNaN(d.getTime()) ? t : `${d.toLocaleDateString()} ${d.toLocaleTimeString().slice(0, 5)}`;
}

// 圈子社区 —— antd Card 网格 + List + Modal 发帖/评论 + framer-motion 悬浮。
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
  // Modal 状态(替原生 prompt)
  const [createModal, setCreateModal] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createDesc, setCreateDesc] = useState("一起学英语");
  const [joinModal, setJoinModal] = useState<CommunityCircle | null>(null);
  const [joinCode, setJoinCode] = useState("");

  const load = useCallback(async (mine: boolean) => {
    setErr("");
    try { setCircles(await api.listCircles(q, mine)); } catch (e) { setErr(`加载圈子失败: ${e}`); }
  }, [q]);

  useEffect(() => { load(tab === "mine"); }, [tab, load]);

  async function openDetail(c: CommunityCircle) {
    setErr("");
    try {
      const fresh = await api.getCircle(c.id);
      const [ms, ps] = await Promise.all([api.listCircleMembers(c.id), api.listPosts(c.id)]);
      setDetail(fresh); setMembers(ms); setPosts(ps); setExpanded(null); setComments({});
    } catch (e) { setErr(`打开圈子失败: ${e}`); }
  }
  async function createCircle() {
    if (!createName) return;
    setBusy(true);
    try {
      const r = await api.createCircle({ name: createName, description: createDesc });
      setMsg(`圈子已创建!邀请码: ${r.invite_code}`);
      setCreateModal(false); setCreateName(""); setCreateDesc("一起学英语");
      await load(tab === "mine");
    } catch (e) { setErr(`创建失败: ${e}`); } finally { setBusy(false); }
  }
  async function doJoin() {
    if (!joinModal || !joinCode) return;
    setBusy(true);
    try {
      await api.joinCircle(joinModal.id, joinCode.trim());
      setMsg("加入成功!");
      setJoinModal(null); setJoinCode("");
      await load(tab === "mine");
      if (detail?.id === joinModal.id) await openDetail(joinModal);
    } catch (e) { setErr(`加入失败: ${e}`); } finally { setBusy(false); }
  }
  async function leave(c: CommunityCircle) {
    Modal.confirm({
      title: `退出「${c.name}」?`, okText: "退出", okType: "danger", cancelText: "取消",
      onOk: async () => {
        try { await api.leaveCircle(c.id); setMsg("已退出圈子"); setDetail(null); await load(tab === "mine"); }
        catch (e) { setErr(`退出失败: ${e}`); }
      },
    });
  }
  async function post() {
    if (!detail || !newPost.trim()) return;
    try { await api.createPost(detail.id, newPost.trim()); setNewPost(""); setPosts(await api.listPosts(detail.id)); }
    catch (e) { setErr(`发帖失败: ${e}`); }
  }
  async function toggleComments(p: CommunityPost) {
    if (expanded === p.id) { setExpanded(null); return; }
    setExpanded(p.id);
    try { const list = await api.listComments(p.id); setComments((prev) => ({ ...prev, [p.id]: list })); }
    catch (e) { setErr(`加载评论失败: ${e}`); }
  }
  async function addComment(p: CommunityPost) {
    const body = (commentDraft[p.id] || "").trim(); if (!body) return;
    try {
      await api.createComment(p.id, body);
      setCommentDraft((prev) => ({ ...prev, [p.id]: "" }));
      const list = await api.listComments(p.id); setComments((prev) => ({ ...prev, [p.id]: list }));
      setPosts(await api.listPosts(detail!.id));
    } catch (e) { setErr(`评论失败: ${e}`); }
  }
  async function reportPost(p: CommunityPost) {
    let reason = ""; Modal.confirm({
      title: "举报帖子", content: <Input onChange={(e) => { reason = e.target.value; }} placeholder="举报原因" />,
      okText: "举报", okType: "danger", cancelText: "取消",
      onOk: async () => {
        if (!reason) return;
        try { const r = await api.reportContent("post", p.id, reason); setMsg(r.auto_hidden ? "已自动隐藏该违规帖" : `已举报(累计 ${r.report_count} 次)`); setPosts(await api.listPosts(detail!.id)); }
        catch (e) { setErr(`举报失败: ${e}`); }
      },
    });
  }

  if (detail) {
    const joined = detail.joined;
    return (
      <motion.div initial={{ opacity: 0, x: 20 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: DUR_SLOW, ease: EASE }} className="motion-fade">
        <Card size="small" style={{ marginBottom: 12 }}>
          <Space style={{ width: "100%", justifyContent: "space-between" }}>
            <Space>
              <Button icon={<ArrowLeftOutlined />} onClick={() => setDetail(null)}>{t("action.back")}</Button>
              <Title level={4} style={{ margin: 0 }}>{detail.name}</Title>
            </Space>
            <Space>
              <Text type="secondary" style={{ fontSize: 12 }}>{detail.owner_name} 创建 · {detail.member_count}/{detail.member_limit} 人</Text>
              {joined && detail.my_role === "owner" && <Tag color="gold">圈主 · 邀请码 {detail.invite_code}</Tag>}
              {joined && detail.my_role !== "owner" && <Button size="small" danger onClick={() => leave(detail)}>退出</Button>}
            </Space>
          </Space>
          {detail.description && <Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0 }}>{detail.description}</Paragraph>}
        </Card>

        {err && <Alert type="error" message={err} style={{ marginBottom: 12 }} closable />}
        {msg && <Alert type="success" message={msg} style={{ marginBottom: 12 }} closable />}

        <Row gutter={12}>
          <Col xs={24} md={7}>
            <Card title={<span><TeamOutlined /> 成员({members.length})</span>} size="small">
              <List size="small" dataSource={members} locale={{ emptyText: "无" }} renderItem={(m) => (
                <List.Item>
                  <List.Item.Meta
                    avatar={<Avatar size="small">{(m.name || "?").charAt(0)}</Avatar>}
                    title={<Text style={{ fontSize: 13 }}>{m.name || `用户#${m.user_id}`}</Text>}
                    description={m.role === "owner" ? <Tag color="gold">圈主</Tag> : <Tag>成员</Tag>}
                  />
                </List.Item>
              )} />
            </Card>
          </Col>
          <Col xs={24} md={17}>
            {joined ? (
              <Card size="small" style={{ marginBottom: 12 }}>
                <Space.Compact style={{ width: "100%" }}>
                  <TextArea value={newPost} onChange={(e) => setNewPost(e.target.value)} placeholder="分享学习心得/求组队/问经验…" autoSize={{ minRows: 1, maxRows: 3 }} />
                  <Button type="primary" icon={<PlusOutlined />} onClick={post}>{t("community.post")}</Button>
                </Space.Compact>
              </Card>
            ) : (
              <Alert type="info" message="加入圈子后即可参与讨论" style={{ marginBottom: 12 }} />
            )}
            <motion.div variants={stagger} initial="hidden" animate="visible">
              <AnimatePresence>
                {posts.length ? posts.map((p) => (
                  <motion.div key={p.id} variants={fadeUp} layout>
                    <Card size="small" style={{ marginBottom: 8 }} hoverable>
                      <Space style={{ width: "100%", justifyContent: "space-between" }}>
                        <Text type="secondary" style={{ fontSize: 12 }}>{p.user_name || `用户#${p.user_id}`} · {fmt(p.created_at)}</Text>
                        {joined && <Button size="small" type="text" icon={<FlagOutlined />} onClick={() => reportPost(p)}>举报</Button>}
                      </Space>
                      <Paragraph style={{ margin: "6px 0", whiteSpace: "pre-wrap" }}>{p.body}</Paragraph>
                      <Button size="small" type="link" icon={<MessageOutlined />} onClick={() => toggleComments(p)}>
                        {t("community.comment")}({p.comment_count}){expanded === p.id ? " · 收起" : ""}
                      </Button>
                      <AnimatePresence>
                        {expanded === p.id && (
                          <motion.div initial={{ height: 0, opacity: 0 }} animate={{ height: "auto", opacity: 1 }} exit={{ height: 0, opacity: 0 }} style={{ overflow: "hidden" }}>
                            <List size="small" dataSource={comments[p.id] || []} locale={{ emptyText: "无评论" }} renderItem={(c) => (
                              <List.Item style={{ padding: "4px 0", fontSize: 12 }}>
                                <Text><Text strong>{c.user_name || `用户#${c.user_id}`}:</Text> {c.body}</Text>
                              </List.Item>
                            )} />
                            {joined && (
                              <Space.Compact style={{ width: "100%", marginTop: 6 }}>
                                <Input size="small" value={commentDraft[p.id] || ""} onChange={(e) => setCommentDraft((prev) => ({ ...prev, [p.id]: e.target.value }))} placeholder="回复…" />
                                <Button size="small" type="primary" onClick={() => addComment(p)}>{t("community.publish")}</Button>
                              </Space.Compact>
                            )}
                          </motion.div>
                        )}
                      </AnimatePresence>
                    </Card>
                  </motion.div>
                )) : <Empty description={t("community.empty")} image={Empty.PRESENTED_IMAGE_SIMPLE} />}
              </AnimatePresence>
            </motion.div>
          </Col>
        </Row>
      </motion.div>
    );
  }

  return (
    <motion.div variants={stagger} initial="hidden" animate="visible" className="motion-fade">
      <SafeImg src={UNSPLASH.forest} alt="" tone={3} className="scene-banner">
        <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: DUR_SLOW, ease: EASE }} style={{ position: "relative", zIndex: 1 }}>
          <Title level={3} style={{ color: "#fff", margin: 0, textShadow: "0 2px 12px rgba(0,0,0,0.4)" }}>{t("community.title")}</Title>
          <Text style={{ color: "rgba(255,255,255,0.9)" }}>学习圈子 · 组队打卡 · 经验交流</Text>
        </motion.div>
      </SafeImg>

      <Card size="small" style={{ marginTop: 12, marginBottom: 12 }}>
        <Space style={{ width: "100%", justifyContent: "space-between" }} wrap>
          <Segmented value={tab} onChange={(v) => setTab(v as "discover" | "mine")}
            options={[{ label: "发现圈子", value: "discover" }, { label: "我的圈子", value: "mine" }]} />
          <Space>
            {tab === "discover" && <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="搜索圈子…" style={{ width: 180 }} size="small" />}
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModal(true)} disabled={busy}>创建圈子</Button>
          </Space>
        </Space>
      </Card>

      {err && <Alert type="error" message={err} style={{ marginBottom: 12 }} closable />}
      {msg && <Alert type="success" message={msg} style={{ marginBottom: 12 }} closable />}

      <Row gutter={[12, 12]}>
        {circles.map((c) => (
          <Col key={c.id} xs={24} sm={12} md={8}>
            <motion.div variants={fadeUp} whileHover={{ y: -4 }} transition={{ duration: 0.2, ease: EASE }}>
              <Card size="small" hoverable onClick={() => openDetail(c)} title={c.name}
                extra={!c.joined && <Button size="small" type="primary" onClick={(e) => { e.stopPropagation(); setJoinModal(c); }}>加入</Button>}>
                <Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 4 }}>{c.description || "暂无简介"}</Paragraph>
                <Space size="small">
                  <Tag icon={<TeamOutlined />}>{c.member_count}/{c.member_limit}</Tag>
                  {c.joined && <Tag color="success">已加入</Tag>}
                  <Text type="secondary" style={{ fontSize: 11 }}>{c.owner_name}</Text>
                </Space>
              </Card>
            </motion.div>
          </Col>
        ))}
      </Row>
      {!circles.length && <Empty description={tab === "mine" ? "还没有加入任何圈子" : "暂无圈子,点击右上角创建"} style={{ marginTop: 40 }} />}

      {/* 创建圈子 Modal */}
      <Modal title="创建圈子" open={createModal} onOk={createCircle} onCancel={() => setCreateModal(false)} okText="创建" cancelText="取消" confirmLoading={busy}>
        <Space direction="vertical" style={{ width: "100%" }} size="middle">
          <Input value={createName} onChange={(e) => setCreateName(e.target.value)} placeholder="圈子名称" />
          <Input value={createDesc} onChange={(e) => setCreateDesc(e.target.value)} placeholder="圈子简介" />
        </Space>
      </Modal>
      {/* 加入圈子 Modal */}
      <Modal title={joinModal ? `加入「${joinModal.name}」` : ""} open={!!joinModal} onOk={doJoin} onCancel={() => setJoinModal(null)} okText="加入" cancelText="取消" confirmLoading={busy}>
        <Input value={joinCode} onChange={(e) => setJoinCode(e.target.value)} placeholder="输入邀请码" />
      </Modal>
    </motion.div>
  );
}
