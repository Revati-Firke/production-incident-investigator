import { FormEvent, useState } from "react";
import { api, type RagHit } from "../api";

export default function Knowledge() {
  const [query, setQuery] = useState("connection pool");
  const [hits, setHits] = useState<RagHit[]>([]);
  const [error, setError] = useState("");

  async function onSearch(e: FormEvent) {
    e.preventDefault();
    setError("");
    try {
      const data = await api.searchRAG(query);
      setHits(data.hits || []);
    } catch (err) {
      setError(String((err as Error).message || err));
    }
  }

  return (
    <section className="panel">
      <h1>Knowledge search</h1>
      <form className="actions" onSubmit={onSearch}>
        <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search runbooks…" />
        <button type="submit">Search</button>
      </form>
      {error && <p className="error">{error}</p>}
      <ul className="feed">
        {hits.map((h, i) => (
          <li key={i}>
            <strong>
              {h.document_title} · {(h.score * 100).toFixed(0)}%
            </strong>
            <span>{h.content.slice(0, 240)}…</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
