const API = "/api/v1";

export type ApiEnvelope<T> = {
  success: boolean;
  data: T;
  error?: { code: string; message: string } | null;
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const apiKey = localStorage.getItem("opspilot_api_key");
  if (apiKey) headers.set("X-API-Key", apiKey);

  const res = await fetch(`${API}${path}`, { ...init, headers });
  const body = (await res.json()) as ApiEnvelope<T>;
  if (!res.ok || !body.success) {
    throw new Error(body.error?.message || `Request failed (${res.status})`);
  }
  return body.data;
}

export const api = {
  listIncidents: (status?: string) =>
    request<{ items: Incident[]; total: number }>(
      `/incidents?limit=50${status ? `&status=${encodeURIComponent(status)}` : ""}`,
    ),
  getIncident: (id: string) => request<Incident>(`/incidents/${id}`),
  getTimeline: (id: string) => request<TimelineEvent[]>(`/incidents/${id}/timeline`),
  getInvestigation: (id: string) => request<Investigation>(`/incidents/${id}/investigation`),
  getEvidence: (id: string) => request<Evidence[]>(`/incidents/${id}/evidence`),
  getRemediations: (id: string) => request<Remediation[]>(`/incidents/${id}/remediations`),
  listAwaiting: () => request<{ items: Remediation[]; total: number }>(`/remediations/awaiting`),
  approve: (incidentId: string, proposalId: string, actor: string, comment: string) =>
    request<Remediation>(`/incidents/${incidentId}/remediations/${proposalId}/approve`, {
      method: "POST",
      body: JSON.stringify({ actor, comment }),
    }),
  reject: (incidentId: string, proposalId: string, actor: string, comment: string) =>
    request<Remediation>(`/incidents/${incidentId}/remediations/${proposalId}/reject`, {
      method: "POST",
      body: JSON.stringify({ actor, comment }),
    }),
  searchRAG: (query: string) =>
    request<{ hits: RagHit[] }>(`/rag/search`, {
      method: "POST",
      body: JSON.stringify({ query, top_k: 5 }),
    }),
  integrations: () => request<Record<string, { provider: string; configured: boolean }>>(`/integrations`),
};

export type Incident = {
  id: string;
  title: string;
  status: string;
  severity: string;
  service: string;
  environment: string;
  description?: string;
  created_at?: string;
};

export type TimelineEvent = {
  id: string;
  event_type?: string;
  message?: string;
  to_status?: string;
  created_at?: string;
};

export type Investigation = {
  id: string;
  status: string;
  confidence?: number;
  root_cause?: unknown;
  reasoning_summary?: string;
};

export type Evidence = {
  id: string;
  tool_name: string;
  status: string;
  summary?: string;
};

export type Remediation = {
  id: string;
  incident_id: string;
  summary: string;
  risk_level: string;
  status: string;
  actions?: { title: string; tool?: string }[];
};

export type RagHit = {
  document_title: string;
  content: string;
  score: number;
  source_type: string;
};
