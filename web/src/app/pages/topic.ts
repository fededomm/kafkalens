import { afterNextRender, Component, computed, DestroyRef, effect, ElementRef, Injector, inject, input, signal, untracked, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { Api, BrowseReq, BrowseResp, Header, KRecord, Toasts, TopicDetail } from '../api';
import { BytesPipe, download, highlightJson, NumPipe, prettyJson, TimePipe } from '../fmt';
import { Icon } from '../icon';
import { ConfigTable } from './config-table';

type Tab = 'messages' | 'partitions' | 'configs' | 'consumers';
const LIVE_CAP = 2000;

@Component({
  selector: 'app-topic',
  imports: [FormsModule, RouterLink, NumPipe, TimePipe, BytesPipe, Icon, ConfigTable],
  templateUrl: './topic.html',
  styleUrl: './topic.css',
})
export class TopicPage {
  readonly id = input.required<string>();
  readonly topic = input.required<string>();
  private readonly api = inject(Api);
  private readonly toasts = inject(Toasts);
  private readonly router = inject(Router);
  private readonly produceDlg = viewChild.required<ElementRef<HTMLDialogElement>>('produceDlg');
  private readonly partsDlg = viewChild.required<ElementRef<HTMLDialogElement>>('partsDlg');
  private readonly deleteDlg = viewChild.required<ElementRef<HTMLDialogElement>>('deleteDlg');
  private readonly keyInput = viewChild<ElementRef<HTMLInputElement>>('keyInput');
  private readonly valInput = viewChild<ElementRef<HTMLTextAreaElement>>('valInput');
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  protected readonly tab = signal<Tab>('messages');
  protected readonly detail = signal<TopicDetail | null>(null);
  protected readonly error = signal<string | null>(null);
  protected readonly readOnly = computed(() => this.api.clusters().find((c) => c.id === this.id())?.readOnly ?? false);
  protected readonly totalMessages = computed(() => (this.detail()?.partitions ?? []).reduce((s, p) => s + (p.end - p.start), 0));
  protected readonly replication = computed(() => this.detail()?.partitions[0]?.replicas.length ?? 0);

  // ---- browse ----
  protected readonly req = signal<BrowseReq>({ from: 'latest', partition: -1, offset: 0, timestamp: 0, limit: 100, filter: '' });
  protected timeInput = '';
  protected readonly records = signal<KRecord[]>([]);
  protected readonly meta = signal<{ scanned: number; partial: boolean; ms: number } | null>(null);
  protected readonly browsing = signal(false);
  protected readonly selected = signal<KRecord | null>(null);
  protected readonly raw = signal(false);

  protected readonly selValue = computed(() => this.render(this.selected()?.value ?? null));
  protected readonly selKey = computed(() => this.render(this.selected()?.key ?? null));

  // ---- live tail ----
  protected readonly live = signal(false);
  protected readonly livePaused = signal(false);
  private stopTail: (() => void) | null = null;
  private buffer: KRecord[] = [];

  // ---- produce ----
  protected pr = { key: '', value: '', partition: -1, count: 1, headers: [] as Header[], keyNull: false };
  protected readonly producing = signal(false);
  protected readonly valueJsonState = signal<'json' | 'text' | 'invalid'>('text');

  protected newPartitions = 0;
  protected confirmText = '';

  constructor() {
    effect(() => {
      this.id(); this.topic();
      untracked(() => {
        this.stopLive();
        this.detail.set(null);
        this.records.set([]);
        this.selected.set(null);
        this.loadDetail().then(() => this.browse());
      });
    });
    inject(DestroyRef).onDestroy(() => this.stopLive());
  }

  private base() { return `/c/${this.id()}/topics/${Api.seg(this.topic())}`; }

  async loadDetail() {
    try {
      this.detail.set(await this.api.get<TopicDetail>(this.base()));
      this.error.set(null);
    } catch (e) {
      this.error.set(e instanceof Error ? e.message : String(e));
    }
  }

  patch(p: Partial<BrowseReq>) { this.req.update((r) => ({ ...r, ...p })); }

  async browse() {
    this.stopLive();
    const r = { ...this.req() };
    if (r.from === 'timestamp') r.timestamp = this.timeInput ? new Date(this.timeInput).getTime() : Date.now() - 3600_000;
    this.browsing.set(true);
    const t0 = performance.now();
    try {
      const res = await this.api.post<BrowseResp>(`${this.base()}/browse`, r);
      // newest first reads better for "latest", chronological for the others
      const recs = r.from === 'latest' ? [...res.records].sort((a, b) => b.timestamp - a.timestamp || b.offset - a.offset) : res.records;
      this.records.set(recs);
      this.meta.set({ scanned: res.scanned, partial: res.partial, ms: Math.round(performance.now() - t0) });
      this.selected.set(recs[0] ?? null);
    } catch (e) {
      this.toasts.err(e);
    } finally {
      this.browsing.set(false);
    }
  }

  toggleLive() {
    if (this.live()) return this.stopLive();
    this.records.set([]);
    this.selected.set(null);
    this.meta.set(null);
    this.live.set(true);
    this.livePaused.set(false);
    let scheduled = false;
    const onRecords = (rs: KRecord[]) => {
      this.buffer.push(...rs);
      // batch DOM updates to one per frame under high throughput
      if (scheduled || this.livePaused()) return;
      scheduled = true;
      requestAnimationFrame(() => { scheduled = false; this.flush(); });
    };
    const onError = (msg: string) => { this.toasts.err(msg); this.stopLive(); };
    this.api.tail(this.id(), this.topic(), onRecords, onError).then(
      (stop) => { if (this.live()) this.stopTail = stop; else stop(); },
      (e) => onError(String(e)),
    );
  }

  private flush() {
    if (!this.buffer.length) return;
    const f = this.req().filter.toLowerCase();
    const add = this.buffer.reverse().filter((r) => !f || [r.key, r.value, ...r.headers.map((h) => h.key + '=' + h.value)].some((s) => s?.toLowerCase().includes(f)));
    this.buffer = [];
    this.records.update((l) => [...add, ...l].slice(0, LIVE_CAP));
  }

  togglePause() {
    this.livePaused.update((p) => !p);
    if (!this.livePaused()) this.flush();
  }

  stopLive() {
    this.stopTail?.();
    this.stopTail = null;
    this.buffer = [];
    this.live.set(false);
  }

  private render(s: string | null) {
    if (s == null) return { kind: 'null' as const, html: '', text: '' };
    const pretty = this.raw() ? null : prettyJson(s);
    return pretty ? { kind: 'json' as const, html: highlightJson(pretty), text: pretty } : { kind: 'text' as const, html: '', text: s };
  }

  preview(s: string | null) {
    if (s == null) return null;
    return s.length > 160 ? s.slice(0, 160) + '…' : s;
  }

  rowKey(e: KeyboardEvent) {
    const list = this.records();
    const i = list.indexOf(this.selected()!);
    if (e.key === 'ArrowDown' || e.key === 'j') { e.preventDefault(); this.select(list[Math.min(i + 1, list.length - 1)]); }
    if (e.key === 'ArrowUp' || e.key === 'k') { e.preventDefault(); this.select(list[Math.max(i - 1, 0)]); }
  }

  select(r: KRecord | undefined) {
    if (!r) return;
    this.selected.set(r);
    afterNextRender(() => this.host.nativeElement.querySelector('tr.selected')?.scrollIntoView({ block: 'nearest' }), { injector: this.injector });
  }

  async copy(text: string, what: string) {
    await navigator.clipboard.writeText(text);
    this.toasts.ok(`${what} copied`);
  }

  exportJson() {
    download(`${this.topic()}-${Date.now()}.json`, JSON.stringify(this.records(), null, 2));
  }

  // ---- produce ----
  openProduce(from?: KRecord) {
    this.pr = {
      key: from?.key ?? '', value: from?.value ?? '', partition: -1, count: 1,
      headers: from ? from.headers.map((h) => ({ ...h })) : [], keyNull: from ? from.key == null : false,
    };
    this.checkJson();
    this.produceDlg().nativeElement.showModal();
    afterNextRender(() => (from ? this.valInput() : this.keyInput() ?? this.valInput())?.nativeElement.focus(), { injector: this.injector });
  }

  checkJson() {
    const v = this.pr.value.trim();
    if (!v || (v[0] !== '{' && v[0] !== '[')) return this.valueJsonState.set('text');
    try { JSON.parse(v); this.valueJsonState.set('json'); } catch { this.valueJsonState.set('invalid'); }
  }

  formatJson() {
    try { this.pr.value = JSON.stringify(JSON.parse(this.pr.value), null, 2); this.checkJson(); } catch { this.toasts.err('Value is not valid JSON'); }
  }

  async produce() {
    this.producing.set(true);
    try {
      const res = await this.api.post<{ count: number; partition: number; offset: number }>(`${this.base()}/produce`, {
        key: this.pr.keyNull ? null : this.pr.key, value: this.pr.value, partition: this.pr.partition,
        count: this.pr.count, headers: this.pr.headers.filter((h) => h.key),
      });
      this.toasts.ok(res.count > 1 ? `Produced ${res.count} messages` : `Produced to partition ${res.partition} @ offset ${res.offset}`);
      this.produceDlg().nativeElement.close();
      this.loadDetail();
      if (!this.live()) this.browse();
    } catch (e) {
      this.toasts.err(e);
    } finally {
      this.producing.set(false);
    }
  }

  // ---- admin ----
  openParts() {
    this.newPartitions = (this.detail()?.partitions.length ?? 0) + 1;
    this.partsDlg().nativeElement.showModal();
  }

  async addPartitions() {
    try {
      await this.api.post(`${this.base()}/partitions`, { total: this.newPartitions });
      this.partsDlg().nativeElement.close();
      this.toasts.ok(`Topic now has ${this.newPartitions} partitions`);
      this.loadDetail();
    } catch (e) {
      this.toasts.err(e);
    }
  }

  async setConfig(c: { key: string; value: string | null }) {
    try {
      await this.api.put(`${this.base()}/configs`, { [c.key]: c.value });
      this.toasts.ok(c.value == null ? `${c.key} reset to default` : `${c.key} updated`);
      this.loadDetail();
    } catch (e) {
      this.toasts.err(e);
    }
  }

  askDelete() {
    this.confirmText = '';
    this.deleteDlg().nativeElement.showModal();
  }

  async deleteTopic() {
    try {
      await this.api.del(this.base());
      this.deleteDlg().nativeElement.close();
      this.toasts.ok(`Deleted ${this.topic()}`);
      this.router.navigate(['/c', this.id(), 'topics']);
    } catch (e) {
      this.toasts.err(e);
    }
  }
}
