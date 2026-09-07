import { FormEvent, useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import {
  api,
  type Evidence,
  type Incident,
  type Investigation,
  type Remediation,
  type TimelineEvent,
} from "../api";

export default function IncidentDetail() {
  const { id = "" } = useParams();
  const [incident, setIncident] = useState<Incident | null>(null);
  const [timeline, setTimeline] = useState<TimelineEvent[]>([]);
  const [investigation, setInvestigation] = useState<Investigation | null>(null);
  const [evidence, setEvidence] = useState<Evidence[]>([]);
  const [remediations, setRemediations] = useState<Remediation[]>([]);
  const [actor, setActor] = useState("operator");
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = () => {
    if (!id) return;
    Promise.all([
      api.getIncident(id),
      api.getTimeline(id),
      api.getInvestigation(id).catch(() => null),
      api.getEvidence(id).catch(() => []),
      api.getRemediations(id).catch(() => []),
    ])
      .then(([inc, tl, inv, ev, rem]) => {
        setIncident(inc);
        setTimeline(tl || []);
        setInvestigation(inv);
        setEvidence(ev || []);
        setRemediations(rem || []);
      })
      .catch((e) => setError(String(e.message || e)));
  };

  useEffect(load, [id]);

  const proposed = remediations.find((r) => r.status === "proposed");

  async function onDecide(approve: boolean, e: FormEvent) {
    e.preventDefault();
    if (!proposed) return;
    setBusy(true);
    setError("");
    try {
      if (approve) await api.approve(id, proposed.id, actor, comment);
      else await api.reject(id, proposed.id, actor, comment);
      load();
    } catch (err) {
      setError(String((err as Error).message || err));
    } finally {
      setBusy(false);
    }
  }

  if (!incident) return <p className="muted">Loading…</p>;

  return (
    <section className="detail">
      <div className="panel">
        <p className="eyebrow">{incident.service} · {incident.environment}</p>
        <h1>{incident.title}</h1>
        <div className="meta">
          <span className={`pill sev-${incident.severity}`}>{incident.severity}</span>
          <span className="pill">{incident.status}</span>
        </div>
        {incident.description && <p className="lede">{incident.description}</p>}
        {error && <p className="error">{error}</p>}
      </div>

      {investigation && (
        <div className="panel">
          <h2>Root cause</h2>
          <p className="muted">
            Investigation {investigation.status}
            {investigation.confidence != null ? ` · confidence ${(investigation.confidence * 100).toFixed(0)}%` : ""}
          </p>
          {investigation.reasoning_summary && <p>{investigation.reasoning_summary}</p>}
          {investigation.root_cause && (
            <pre className="code">{JSON.stringify(investigation.root_cause, null, 2)}</pre>
          )}
        </div>
      )}

      {proposed && (
        <div className="panel accent">
          <h2>Awaiting approval</h2>
          <p>{proposed.summary}</p>
          <p className="muted">Risk: {proposed.risk_level}</p>
          <ul>
            {(proposed.actions || []).map((a, idx) => (
              <li key={idx}>
                {a.title}
                {a.tool ? ` (${a.tool})` : ""}
              </li>
            ))}
          </ul>
          <form className="actions" onSubmit={(e) => onDecide(true, e)}>
            <input value={actor} onChange={(e) => setActor(e.target.value)} placeholder="Actor" required />
            <input value={comment} onChange={(e) => setComment(e.target.value)} placeholder="Comment" />
            <button type="submit" disabled={busy}>
              Approve
            </button>
            <button type="button" className="ghost" disabled={busy} onClick={(e) => onDecide(false, e as unknown as FormEvent)}>
              Reject
            </button>
          </form>
        </div>
      )}

      <div className="grid-2">
        <div className="panel">
          <h2>Timeline</h2>
          <ul className="feed">
            {timeline.map((ev) => (
              <li key={ev.id}>
                <strong>{ev.to_status || ev.event_type}</strong>
                <span>{ev.message}</span>
              </li>
            ))}
          </ul>
        </div>
        <div className="panel">
          <h2>Evidence</h2>
          <ul className="feed">
            {evidence.map((ev) => (
              <li key={ev.id}>
                <strong>{ev.tool_name}</strong>
                <span>{ev.status}</span>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  );
}
