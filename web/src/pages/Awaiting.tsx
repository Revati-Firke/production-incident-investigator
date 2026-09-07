import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, type Remediation } from "../api";

export default function Awaiting() {
  const [items, setItems] = useState<Remediation[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .listAwaiting()
      .then((d) => setItems(d.items || []))
      .catch((e) => setError(String(e.message || e)));
  }, []);

  return (
    <section className="panel">
      <h1>Approval queue</h1>
      {error && <p className="error">{error}</p>}
      {items.length === 0 && <p className="muted">No proposals waiting.</p>}
      <ul className="feed">
        {items.map((r) => (
          <li key={r.id}>
            <Link to={`/incidents/${r.incident_id}`}>{r.summary}</Link>
            <span>
              {r.risk_level} · {r.status}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
