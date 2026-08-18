import { useEffect, useState } from "react";
import { useAuth } from "../auth";
import { useNavigate } from "react-router-dom";
import { api } from "../api";
import { useI18n, resolvePersona, DigitalHumanPicker, DigitalHuman, PERSONAS, type Persona } from "@esp/ui-tokens";
import type { SessionSummary } from "../types";

// 个人中心:用户信息卡 + 数字人形象选择 + 会话历史(会话22-3 C 批次 + 数字人 persona)。
export default function Profile() {
  const { user, logout } = useAuth();
  const nav = useNavigate();
  const { t, locale } = useI18n();
  const [history, setHistory] = useState<SessionSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  // 数字人 persona:服务端持久化(跨设备一致),未选时按 userId 确定性派生
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
    } catch { /* 静默:沿用本地值 */ }
    setSaving(false);
  }

  function resetPersona() {
    const def = resolvePersona(undefined, user?.id);
    setPersonaId(def.id);
    setPersona(def);
  }

  return (
    <div className="app">
      <h1>{t("profile.center")}</h1>
      <section style={{ marginBottom: 16 }}>
        <pre>{JSON.stringify(user, null, 2)}</pre>
      </section>

      <h2>{t("persona.title")}</h2>
      <p className="hint">{t("persona.hint")}</p>
      <div style={{ display: "flex", gap: 16, alignItems: "center", margin: "12px 0" }}>
        <DigitalHuman persona={persona} emotion="happy" size={96} />
        <div>
          <div style={{ fontWeight: 600 }}>{locale === "zh" ? persona.name.zh : persona.name.en}</div>
          <div className="hint">{t("persona.current")}</div>
        </div>
      </div>
      <DigitalHumanPicker locale={locale} value={personaId} onChange={pick} />
      <div style={{ display: "flex", gap: 8, marginTop: 12 }}>
        <button className="primary" onClick={savePersona} disabled={saving}>{saving ? t("action.loading") : t("persona.save")}</button>
        <button onClick={resetPersona}>{t("persona.reset")}</button>
        {savedMsg && <span className="hint" style={{ color: "var(--esp-color-success)" }}>{t("persona.saved")}</span>}
      </div>

      <h2 style={{ marginTop: 24 }}>{t("profile.history")}</h2>
      {loading && <p>{t("action.loading")}</p>}
      {err && <p style={{ color: "crimson" }}>{t("error.loadFailed")}:{err}</p>}
      {!loading && !err && history.length === 0 && <p className="hint">{t("common.none")}</p>}
      {!loading && !err && history.length > 0 && (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th align="left">{t("common.thread")}</th>
              <th align="left">{t("common.status")}</th>
              <th align="left">{t("common.started")}</th>
              <th align="left">{t("common.ended")}</th>
            </tr>
          </thead>
          <tbody>
            {history.map((s) => (
              <tr key={s.id} style={{ borderBottom: "1px solid var(--esp-color-border)" }}>
                <td>{s.scenario}</td>
                <td>{s.status}</td>
                <td>{s.started_at}</td>
                <td>{s.ended_at ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <div style={{ marginTop: 16 }}>
        <button onClick={() => { logout(); nav("/auth/login"); }}>{t("auth.logout")}</button>
      </div>
    </div>
  );
}
