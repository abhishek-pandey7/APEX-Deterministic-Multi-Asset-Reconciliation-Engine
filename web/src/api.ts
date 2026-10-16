// Types mirror the JSON the Go API returns (internal/store, internal/recon).

export type RunSummary = {
  run_id: number;
  ledger_entries: number;
  custodian_entries: number;
  matches: Record<string, number>; // "EXACT/AUTO" -> count
  breaks: Record<string, number>; // category -> count
  output_sha256: string;
  duration_ms: number;
};

export type Run = {
  id: number;
  status: "RUNNING" | "SUCCEEDED" | "FAILED";
  engine_version: string;
  rule_version: string;
  input_file_ids: number[];
  stats: RunSummary | { error: string } | null;
  output_sha256: string;
  started_at: string;
  finished_at: string | null;
};

export type BreakStatus = "OPEN" | "ASSIGNED" | "PROPOSED" | "APPROVED" | "REJECTED" | "CLOSED";

export type Break = {
  id: number;
  run_id: number;
  category: string;
  account_id: string;
  currency: string;
  amount_minor: number;
  amount: string;
  opened_on: string;
  reason: string;
  status: BreakStatus;
  status_by: string;
  status_since: string;
  legs: number;
};

export type BreakPage = {
  breaks: Break[];
  total: number;
  by_category: Record<string, number>;
  by_status: Record<string, number>;
};

export type Entry = {
  entry_id: number;
  side: "LEDGER" | "CUSTODIAN";
  external_id: string;
  amount: string;
  value_date: string;
  txn_type: string;
  reference: string;
};

export type AuditEvent = {
  id: number;
  status: BreakStatus;
  actor: string;
  reason: string;
  created_at: string;
};

export type BreakDetail = Break & { entries: Entry[]; history: AuditEvent[] };

export type ProgressEvent = { stage: string; message: string; run_id?: number };

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new ApiError(res.status, body.error ?? `The server answered ${res.status}.`);
  }
  return body as T;
}

export const api = {
  runs: () => request<Run[]>("/api/runs"),
  breaks: (runId: number, q: { category?: string; status?: string; account?: string; offset: number; limit: number }) => {
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(q)) if (v !== undefined && v !== "") params.set(k, String(v));
    return request<BreakPage>(`/api/runs/${runId}/breaks?${params}`);
  },
  breakDetail: (id: number) => request<BreakDetail>(`/api/breaks/${id}`),
  startRun: () => request<{ job: string; events: string }>("/api/runs", { method: "POST" }),
  addEvent: (id: number, actor: string, status: BreakStatus, reason: string) =>
    request<AuditEvent>(`/api/breaks/${id}/events`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Actor": actor },
      body: JSON.stringify({ status, reason }),
    }),
};
