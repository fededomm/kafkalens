import { Injectable, signal, WritableSignal } from '@angular/core';

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
  clusterId: string; brokers: Broker[]; topics: number; partitions: number;
  underReplicated: number; offlinePartitions: number; groups: number;
}
export interface ConfigEntry { key: string; value: string | null; sensitive: boolean; source: string; isDefault: boolean }
export interface TopicRow {
  name: string; partitions: number; replicationFactor: number; internal: boolean;
  messages: number; underReplicated: number; error?: string;
}
export interface Partition { id: number; leader: number; replicas: number[]; isr: number[]; start: number; end: number }
export interface TopicDetail {
  name: string; internal: boolean; partitions: Partition[]; configs: ConfigEntry[];
  groups: { group: string; state: string; lag: number }[]; groupsError?: string;
}
/** binary: value is not UTF-8 and travels as standard base64. */
export interface Header { key: string; value: string; binary?: boolean }
export interface KRecord {
  partition: number; offset: number; timestamp: number; key: string | null; value: string | null;
  binary: boolean; size: number; headers: Header[];
}
export interface BrowseReq { from: 'earliest' | 'latest' | 'offset' | 'timestamp'; partition: number; offset: number; timestamp: number; limit: number; filter: string }
export interface BrowseResp { records: KRecord[]; scanned: number; partial: boolean }
export interface GroupRow {
  group: string; state: string; members: number; coordinator: number;
  lag: number; topics: string[]; error?: string;
}
export interface GroupDetail {
  group: string; state: string; protocol: string; coordinator: number; lag: number;
  members: { id: string; instanceId: string | null; clientId: string; host: string; assigned: string[] }[];
  partitions: { topic: string; partition: number; committed: number; end: number; lag: number; member: string | null }[];
}

/** The MCP endpoint for AI clients; enabled is the sidebar switch, error why it can't run (e.g. port taken). */
export interface McpStatus { enabled: boolean; locked: boolean; url: string; token: string; error: string }

/** Go methods bound by the Wails desktop build (desktop.go); absent in browser mode. */
interface DesktopBridge {
  Tail(id: string, topic: string, session: string): Promise<void>;
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
      // subscribe before Tail starts the pump: Wails drops events nobody listens to yet
      const session = Math.random().toString(36).slice(2);
      const rt = (window as unknown as WailsWindow).runtime!;
      const offs = [
        rt.EventsOn(`tail:${session}`, (batch) => onRecords(batch as KRecord[])),
        rt.EventsOn(`tail:${session}:error`, (msg) => onError(msg as string)),
      ];
      const off = () => offs.forEach((o) => o());
      await d.Tail(id, topic, session).catch((e) => { off(); throw e; });
      return () => { off(); void d.StopTail(session); };
    }
    const es = new EventSource(`/api/c/${id}/topics/${Api.seg(topic)}/tail`);
    es.onmessage = (ev) => onRecords([JSON.parse(ev.data)]);
    // a Kafka error ends the stream: close before EventSource reconnects on its own
    es.addEventListener('fail', (ev) => { es.close(); onError(JSON.parse(ev.data)); });
    es.onerror = () => { if (es.readyState === EventSource.CLOSED) onError('Live stream closed'); };
    return () => es.close();
  }
}

/** Starred topics per cluster, kept in this browser's localStorage (not shared between browser and desktop modes). */
@Injectable({ providedIn: 'root' })
export class Favorites {
  private readonly byCluster = signal<Record<string, string[]>>(Favorites.read());

  private static read(): Record<string, string[]> {
    try { return JSON.parse(localStorage.getItem('favorites') ?? '{}'); } catch { return {}; }
  }

  has(cluster: string, topic: string) { return this.byCluster()[cluster]?.includes(topic) ?? false; }

  /** All favorites of a cluster, for bulk checks (filters, sorting). */
  set(cluster: string) { return new Set(this.byCluster()[cluster]); }

  toggle(cluster: string, topic: string) {
    this.byCluster.update((m) => {
      const l = m[cluster] ?? [];
      return { ...m, [cluster]: l.includes(topic) ? l.filter((t) => t !== topic) : [...l, topic] };
    });
    localStorage.setItem('favorites', JSON.stringify(this.byCluster()));
  }

  /** Drops all entries of a removed cluster. */
  forget(cluster: string) {
    this.byCluster.update(({ [cluster]: _, ...m }) => m);
    localStorage.setItem('favorites', JSON.stringify(this.byCluster()));
  }
}

/**
 * Named groups of topics per cluster, stored like Favorites. Entries are topic names or prefix rules written
 * "orders.*" ('*' is not allowed in topic names), so later topics with the prefix join on their own.
 * A folder exists while it holds at least one entry.
 */
@Injectable({ providedIn: 'root' })
export class Folders {
  private readonly byCluster = signal<Record<string, Record<string, string[]>>>(Folders.read());

  private static read(): Record<string, Record<string, string[]>> {
    try { return JSON.parse(localStorage.getItem('folders') ?? '{}'); } catch { return {}; }
  }

  names(cluster: string) { return Object.keys(this.byCluster()[cluster] ?? {}).sort((a, b) => a.localeCompare(b)); }

  /** Prefixes ignore case, like the topic search: clusters often mix "ContiStream-" and "Contistream-". */
  static startsWith(topic: string, prefix: string) { return topic.toLowerCase().startsWith(prefix.toLowerCase()); }

  has(cluster: string, folder: string, topic: string) {
    return this.byCluster()[cluster]?.[folder]?.some((e) => e === topic || (e.endsWith('*') && Folders.startsWith(topic, e.slice(0, -1)))) ?? false;
  }

  /** The prefix rule that puts topic in folder, if any: such a topic can't be taken out on its own. */
  rule(cluster: string, folder: string, topic: string) {
    return this.byCluster()[cluster]?.[folder]?.find((e) => e.endsWith('*') && Folders.startsWith(topic, e.slice(0, -1)));
  }

  /** Adds or removes topics or rules; adding to an unknown folder creates it, removing the last entry drops it. */
  put(cluster: string, folder: string, topics: string[], on: boolean) {
    const fs = { ...this.byCluster()[cluster] }, drop = new Set(topics);
    const l = [...(fs[folder] ?? []).filter((t) => !drop.has(t)), ...(on ? topics : [])];
    if (l.length) fs[folder] = l; else delete fs[folder];
    this.save(cluster, fs);
  }

  remove(cluster: string, folder: string) {
    const fs = { ...this.byCluster()[cluster] };
    delete fs[folder];
    this.save(cluster, fs);
  }

  /** Drops all folders of a removed cluster. */
  forget(cluster: string) {
    this.byCluster.update(({ [cluster]: _, ...m }) => m);
    localStorage.setItem('folders', JSON.stringify(this.byCluster()));
  }

  private save(cluster: string, fs: Record<string, string[]>) {
    this.byCluster.update((m) => ({ ...m, [cluster]: fs }));
    localStorage.setItem('folders', JSON.stringify(this.byCluster()));
  }
}

/**
 * window.confirm in the app's look: app.html shows the dialog while a question is pending.
 * With typed, the action stays disabled until the user types that text (e.g. the topic name). Navigating answers false.
 */
@Injectable({ providedIn: 'root' })
export class Confirm {
  readonly req = signal<{ title: string; text: string; action: string; typed?: string; done: (ok: boolean) => void } | null>(null);
  readonly typed = signal('');

  ask(title: string, text: string, action: string, typed?: string) {
    this.req()?.done(false);
    this.typed.set('');
    return new Promise<boolean>((done) => this.req.set({ title, text, action, typed, done }));
  }

  answer(ok: boolean) {
    this.req()?.done(ok);
    this.req.set(null);
  }
}

@Injectable({ providedIn: 'root' })
export class Toasts {
  readonly items = signal<{ id: number; kind: 'ok' | 'err'; text: string }[]>([]);
  private seq = 0;

  ok(text: string) { this.push('ok', text); }
  err(e: unknown) { this.push('err', e instanceof Error ? e.message : String(e)); }

  /** Runs an action with busy set, toasting its error if it fails. */
  async guard(fn: () => Promise<unknown>, busy?: WritableSignal<boolean>) {
    busy?.set(true);
    try { await fn(); } catch (e) { this.err(e); } finally { busy?.set(false); }
  }

  dismiss(id: number) { this.items.update((l) => l.filter((t) => t.id !== id)); }

  private push(kind: 'ok' | 'err', text: string) {
    const id = ++this.seq;
    this.items.update((l) => [...l, { id, kind, text }]);
    setTimeout(() => this.dismiss(id), kind === 'err' ? 8000 : 3500);
  }
}
