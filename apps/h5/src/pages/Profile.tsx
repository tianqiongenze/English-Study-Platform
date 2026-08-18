import { useEffect, useState } from "react";
import { useAuth } from "../auth";
import { useNavigate } from "react-router-dom";
import { List, Button, Card, DotLoading } from "antd-mobile";
import { useI18n, resolvePersona, DigitalHumanPicker, DigitalHuman, type Persona } from "@esp/ui-tokens";
import { api } from "../api";
import type { SessionSummary } from "../types";

// H5 个人中心:用户信息 + 数字人形象选择 + 会话历史(antd-mobile,会话22-3 + 数字人 persona)。
export default function Profile() {
  const { user, logout } = useAuth();
  const nav = useNavigate();
  const { t, locale } = useI18n();
  const [history, setHistory] = useState<SessionSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  // 数字人 persona
  const [personaId, setPersonaId] = useState<string>("");
  const [persona, setPersona] = useState<Persona>(() => resolvePersona(undefined, user?.id));
  const [saving, setSaving] = useState(false);
  const [savedMsg, setSavedMsg] = useState(false);

  useEffect(() => {
    let alive = true;
    api.sessionHistory()
      .then((r) => { if (alive) { setHistory(r); setLoading(false); } })
      .catch((e) => { if (alive) { setErr(String(e.message || e)); setLoading(false); } });
    api.getDigitalHuman()
      .then((r) => { if (alive) { setPersonaId(r.persona_id); setPersona(resolvePersona(r.persona_id, user?.id)); } })
      .catch(() => { if (alive) setPersonaId(persona.id); });
    return () => { alive = false; };
  }, []);

  function pick(id: string) {
    setPersonaId(id);
    setPersona(resolvePersona(id, user?.id));
  }
  async function savePersona() {
    setSaving(true);
    try {
      await api.setDigitalHuman(personaId);
      setSavedMsg(true);
      window.setTimeout(() => setSavedMsg(false), 2000);
    } catch { /* 沿用本地值 */ }
    setSaving(false);
  }
  function resetPersona() {
    const def = resolvePersona(undefined, user?.id);
    setPersonaId(def.id);
    setPersona(def);
  }

  return (
    <div className="app" style={{ padding: 12 }}>
      <Card className="profile-card" title={t("persona.title")}>
        <p className="hint" style={{ margin: "0 0 8px" }}>{t("persona.hint")}</p>
        <div style={{ display: "flex", gap: 12, alignItems: "center", margin: "8px 0 12px" }}>
          <DigitalHuman persona={persona} emotion="happy" size={80} />
          <div>
            <div style={{ fontWeight: 600, fontSize: "var(--esp-font-size-lg)", color: "var(--esp-color-text)" }}>{locale === "zh" ? persona.name.zh : persona.name.en}</div>
            <div className="hint">{t("persona.current")}</div>
          </div>
        </div>
        <DigitalHumanPicker locale={locale} value={personaId} onChange={pick} />
        <div style={{ display: "flex", gap: 8, marginTop: 12, alignItems: "center" }}>
          <Button color="primary" size="small" loading={saving} onClick={savePersona}>{t("persona.save")}</Button>
          <Button size="small" onClick={resetPersona}>{t("persona.reset")}</Button>
          {savedMsg && <span className="hint" style={{ color: "var(--esp-color-success)" }}>{t("persona.saved")}</span>}
        </div>
      </Card>

      <h3 style={{ marginTop: 16, marginBottom: 8 }}>{t("profile.history")}</h3>
      {loading && <span><DotLoading /> {t("action.loading")}</span>}
      {err && <div style={{ color: "var(--esp-color-error)" }}>{t("error.loadFailed")}:{err}</div>}
      {!loading && !err && history.length === 0 && <div style={{ color: "var(--esp-color-text-tertiary)" }}>{t("empty.records")}</div>}
      {!loading && !err && history.length > 0 && (
        <List>
          {history.map((s) => (
            <List.Item key={s.id} description={`${t("common.status")}:${s.status} · ${t("common.ended")}:${s.ended_at ?? t("common.inProgress")}`}>
              {s.scenario}
              <span style={{ fontSize: "0.7rem", color: "var(--esp-color-text-tertiary)", display: "block" }}>{t("common.started")}:{s.started_at}</span>
            </List.Item>
          ))}
        </List>
      )}

      <div style={{ marginTop: 16 }}>
        <Button block color="danger" onClick={() => { logout(); nav("/login"); }}>{t("auth.logout")}</Button>
      </div>
    </div>
  );
}
