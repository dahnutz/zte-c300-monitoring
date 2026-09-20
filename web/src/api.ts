import type { CollectionRun, CountResult, Envelope, EthEvent, LiveOnu, ONUSample, StatusEvent, UnauthList } from "./types";

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(path, { headers: { Accept: "application/json" }, cache: "no-store", signal });
  const text = await res.text();
  let body: Envelope<T> | { status?: string; error?: string; message?: string };
  try {
    body = JSON.parse(text) as Envelope<T>;
  } catch {
    throw new Error(res.ok ? "invalid JSON" : `${res.status} ${text.slice(0, 160)}`);
  }
  if (!res.ok) {
    const errBody = body as { status?: string; data?: unknown; error_code?: string };
    const data = errBody.data;
    const msg =
      (typeof data === "string" && data) ||
      errBody.status ||
      `${res.status} ${path}`;
    throw new Error(String(msg));
  }
  if ("data" in body) {
    return body.data;
  }
  return body as T;
}

export function samples(query: Record<string, string | number | undefined>): Promise<ONUSample[]> {
  return getJSON<ONUSample[]>(`/api/v1/history/samples?${qs(query)}`);
}

export function counts(query: Record<string, string | number | undefined>): Promise<CountResult> {
  return getJSON<CountResult>(`/api/v1/history/samples?${qs(query)}`);
}

export function runs(limit = 50): Promise<CollectionRun[]> {
  return getJSON<CollectionRun[]>(`/api/v1/history/runs?limit=${limit}`);
}

export function statusEvents(query: Record<string, string | number | undefined>): Promise<StatusEvent[]> {
  return getJSON<StatusEvent[]>(`/api/v1/history/status-events?${qs(query)}`);
}

export function ethEvents(query: Record<string, string | number | undefined>): Promise<EthEvent[]> {
  return getJSON<EthEvent[]>(`/api/v1/history/eth-events?${qs(query)}`);
}

export function unauth(): Promise<UnauthList> {
  return getJSON<UnauthList>("/api/v1/history/unauth");
}

export async function liveOnu(board: number, pon: number, onu: number, expectedSerial: string, signal?: AbortSignal): Promise<LiveOnu> {
  const result = await getJSON<LiveOnu>(`/api/v1/board/${board}/pon/${pon}/onu/${onu}`, signal);
  if (!expectedSerial || result.serial_number !== expectedSerial || result.board !== board || result.pon !== pon || result.onu_id !== onu) {
    throw new Error("ONU identity could not be confirmed at this position. Refresh the inventory before querying again.");
  }
  return result;
}

export async function latestInventory(): Promise<{ run?: CollectionRun; rows: ONUSample[] }> {
  const runList = await runs(20);
  const run = runList
    .filter((r) => r.finished_at && r.status !== "running")
    .reduce<CollectionRun | undefined>((best, r) => !best || r.id > best.id ? r : best, undefined);
  if (!run) return { rows: [] };
  return { run, rows: await samples({ run: run.id, limit: 2000 }) };
}

export function inventoryNotice(run: CollectionRun | undefined, rows: ONUSample[]): string {
  if (!run) return "No finished collection in the latest 20 runs. No current inventory is available.";
  const incomplete = run.status !== "ok" || run.pons_error > 0 || rows.length !== run.onus_sampled || rows.length >= 2000;
  return `Stored run ${run.id} · finished ${formatTime(run.finished_at)} · ${rows.length} rows shown of ${run.onus_sampled} sampled · ${run.pons_error} PON errors. ` +
    (incomplete ? "Incomplete or limited snapshot; missing rows do not prove an ONU is absent. " : "") +
    "Check the collection time; this page does not refresh automatically.";
}

export function discoveryCount(list: UnauthList | null): number | string {
  return list && (list.status === "ok" || list.status === "empty") ? list.count : "—";
}

export type CollectorVersion = {
  version: string;
  api_version?: string;
  commit?: string;
  build_time?: string;
  uptime?: string;
};

export function version(): Promise<CollectorVersion> {
  return fetch("/version", { headers: { Accept: "application/json" } }).then(async (res) => {
    if (!res.ok) {
      throw new Error(`version ${res.status}`);
    }
    return res.json() as Promise<CollectorVersion>;
  });
}

function qs(query: Record<string, string | number | undefined>): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === "") {
      continue;
    }
    params.set(key, String(value));
  }
  return params.toString();
}

export function formatPower(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) {
    return "—";
  }
  return `${value.toFixed(2)} dBm`;
}

export function formatTime(value?: string): string {
  if (!value) {
    return "—";
  }
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) {
    return value;
  }
  return d.toISOString().replace("T", " ").replace(/\.\d+Z$/, "Z");
}

export function formatSince(value?: string): string {
  if (!value) {
    return "—";
  }
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) {
    return value;
  }
  const sec = Math.max(0, Math.round((Date.now() - d.getTime()) / 1000));
  if (sec < 60) {
    return `${sec}s`;
  }
  if (sec < 3600) {
    return `${Math.floor(sec / 60)}m`;
  }
  if (sec < 86400) {
    return `${Math.floor(sec / 3600)}h`;
  }
  return `${Math.floor(sec / 86400)}d`;
}

export function rxClass(value: number | null | undefined, status: string): string {
  if (status !== "Online" || value === null || value === undefined) {
    return "unknown";
  }
  if (value <= -28) {
    return "crit";
  }
  if (value <= -25) {
    return "warn";
  }
  return "ok";
}
