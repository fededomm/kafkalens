import { Injectable, signal } from '@angular/core';

export interface Cluster {
  id: string;
  name: string;
  color: string;
  brokers: string[];
  saslMechanism: '' | 'PLAIN' | 'SCRAM-SHA-256' | 'SCRAM-SHA-512';
  saslUser: string;
  saslPassword?: string;
  hasPassword?: boolean;
  tls: boolean;
  tlsSkipVerify: boolean;
  tlsCa: string;
  tlsCert: string;
  tlsKey?: string;
  hasTlsKey?: boolean;
  readOnly: boolean;
}

export interface Broker { id: number; host: string; port: number; rack: string | null; controller: boolean; leaders: number }
export interface Overview {
  clusterId: string; controller: number; brokers: Broker[]; topics: number; partitions: number;
  underReplicated: number; offlinePartitions: number; groups: number;
}
export interface ConfigEntry { key: string; value: string | null; sensitive: boolean; source: string; isDefault: boolean }
export interface TopicRow {
  name: string; partitions: number; replicationFactor: number; internal: boolean;
  messages: number; underReplicated: number; error?: string;
}
export interface Partition { id: number; leader: number; replicas: number[]; isr: number[]; start: number; end: number }
export interface TopicDetail {
  name: string; internal: boolean; id: string; partitions: Partition[]; configs: ConfigEntry[];
  groups: { group: string; state: string; lag: number }[];
}
export interface Header { key: string; value: string }
export interface KRecord {
  partition: number; offset: number; timestamp: number; key: string | null; value: string | null;
  binary: boolean; size: number; headers: Header[];
}
export interface BrowseReq { from: 'earliest' | 'latest' | 'offset' | 'timestamp'; partition: number; offset: number; timestamp: number; limit: number; filter: string }
export interface BrowseResp { records: KRecord[]; scanned: number; partial: boolean }
export interface GroupRow {
  group: string; state: string; protocolType: string; members: number; coordinator: number;
  lag: number; topics: string[]; error?: string;
}
export interface GroupDetail {
  group: string; state: string; protocol: string; protocolType: string; coordinator: number; lag: number;
  members: { id: string; instanceId: string | null; clientId: string; host: string; assigned: string[] }[];
  partitions: { topic: string; partition: number; committed: number; end: number; lag: number; member: string | null }[];
}

/** Go methods bound by the Wails desktop build (desktop.go); absent in browser mode. */
interface DesktopBridge {
  Tail(id: string, topic: string): Promise<string>;
  StopTail(session: string): Promise<void>;
  SaveFile(name: string, content: string): Promise<string>;
}
type WailsWindow = { go?: { main?: { Desktop?: DesktopBridge } }; runtime?: { EventsOn(name: string, cb: (data: unknown) => void): () => void } };
export const desktop = () => (window as unknown as WailsWindow).go?.main?.Desktop;

/** Thin fetch wrapper: the Go backend serves everything under /api (in the desktop build too, via Wails' asset server). */
@Injectable({ providedIn: 'root' })
export class Api {
  readonly clusters = signal<Cluster[]>([]);

  async refreshClusters() {
    this.clusters.set(await this.get<Cluster[]>('/clusters'));
  }

  get<T>(path: string) { return this.call<T>('GET', path); }
  post<T = unknown>(path: string, body?: unknown) { return this.call<T>('POST', path, body); }
  put<T = unknown>(path: string, body?: unknown) { return this.call<T>('PUT', path, body); }
  del<T = unknown>(path: string) { return this.call<T>('DELETE', path); }

  /** Encodes a path segment (topic and group names may contain dots, slashes…). */
  static seg(s: string) { return encodeURIComponent(s); }

  private async call<T>(method: string, path: string, body?: unknown): Promise<T> {
    const res = await fetch('/api' + path, {
      method,
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error ?? `${res.status} ${res.statusText}`);
    return data as T;
  }

  /** Streams new records of a topic; resolves to a stop function. SSE in the browser, Wails events on desktop. */
  async tail(id: string, topic: string, onRecords: (r: KRecord[]) => void, onError: (msg: string) => void) {
    const d = desktop();
    if (d) {
      const session = await d.Tail(id, topic);
      const off = (window as unknown as WailsWindow).runtime!.EventsOn(`tail:${session}`, (batch) => onRecords(batch as KRecord[]));
      return () => { off(); void d.StopTail(session); };
    }
    const es = new EventSource(`/api/c/${id}/topics/${Api.seg(topic)}/tail`);
    es.onmessage = (ev) => onRecords([JSON.parse(ev.data)]);
    es.onerror = () => { if (es.readyState === EventSource.CLOSED) onError('Live stream closed'); };
    return () => es.close();
  }
}

@Injectable({ providedIn: 'root' })
export class Toasts {
  readonly items = signal<{ id: number; kind: 'ok' | 'err'; text: string }[]>([]);
  private seq = 0;

  ok(text: string) { this.push('ok', text); }
  err(e: unknown) { this.push('err', e instanceof Error ? e.message : String(e)); }

  dismiss(id: number) { this.items.update((l) => l.filter((t) => t.id !== id)); }

  private push(kind: 'ok' | 'err', text: string) {
    const id = ++this.seq;
    this.items.update((l) => [...l, { id, kind, text }]);
    setTimeout(() => this.dismiss(id), kind === 'err' ? 8000 : 3500);
  }
}
