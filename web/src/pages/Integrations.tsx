import { useEffect, useState } from "react";
import { api } from "../api";

export default function Integrations() {
  const [data, setData] = useState<Record<string, { provider: string; configured: boolean }> | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .integrations()
      .then(setData)
      .catch((e) => setError(String(e.message || e)));
  }, []);

  return (
    <section className="panel">
      <h1>Integrations</h1>
      {error && <p className="error">{error}</p>}
      <table>
        <thead>
          <tr>
            <th>System</th>
            <th>Provider</th>
            <th>Configured</th>
          </tr>
        </thead>
        <tbody>
          {data &&
            Object.entries(data).map(([name, v]) => (
              <tr key={name}>
                <td>{name}</td>
                <td>{v.provider}</td>
                <td>{v.configured ? "yes" : "no"}</td>
              </tr>
            ))}
        </tbody>
      </table>
    </section>
  );
}
