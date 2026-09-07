import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, type Incident } from "../api";

export default function IncidentList() {
  const [items, setItems] = useState<Incident[]>([]);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .listIncidents(status || undefined)
      .then((d) => setItems(d.items || []))
      .catch((e) => setError(String(e.message || e)));
  }, [status]);

  return (
    <section className="panel">
      <div className="panel-head">
        <h1>Incidents</h1>
        <select value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">All statuses</option>
          <option value="WAITING_FOR_APPROVAL">Waiting for approval</option>
          <option value="ROOT_CAUSE_IDENTIFIED">Root cause identified</option>
          <option value="RESOLVED">Resolved</option>
          <option value="RECEIVED">Received</option>
        </select>
      </div>
      {error && <p className="error">{error}</p>}
      <table>
        <thead>
          <tr>
            <th>Title</th>
            <th>Service</th>
            <th>Severity</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {items.map((i) => (
            <tr key={i.id}>
              <td>
                <Link to={`/incidents/${i.id}`}>{i.title}</Link>
              </td>
              <td>{i.service}</td>
              <td>
                <span className={`pill sev-${i.severity}`}>{i.severity}</span>
              </td>
              <td>
                <span className="pill">{i.status}</span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
