import { afterNextRender, Component, DestroyRef, effect, ElementRef, inject, Injector, input, output, signal, untracked } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Api, BrowseReq, BrowseResp, KRecord, Partition, Toasts } from '../../../api';
import { download, NumPipe, TimePipe } from '../../../fmt';
import { Icon } from '../../../icon';
import { DateTime } from '../../../datetime';
import { Inspector } from './inspector';

const LIVE_CAP = 2000;

@Component({
  selector: 'app-topic-messages',
  imports: [FormsModule, NumPipe, TimePipe, Icon, DateTime, Inspector],
  templateUrl: './messages.html',
  styleUrl: './messages.css',
})
export class TopicMessages {
  readonly id = input.required<string>();
  readonly topic = input.required<string>();
  readonly base = input.required<string>();
  readonly partitions = input<Partition[]>([]);
  readonly readOnly = input(false);
  /** Re-send: produce a copy of this record. */
  readonly resend = output<KRecord>();

  private readonly api = inject(Api);
  private readonly toasts = inject(Toasts);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  // ---- browse ----
  private static readonly BROWSE_DEFAULTS: Omit<BrowseReq, 'filter'> = { from: 'latest', partition: -1, offset: 0, timestamp: 0, limit: 1000 };
  protected readonly req = signal<BrowseReq>({ ...TopicMessages.BROWSE_DEFAULTS, filter: '' });
  /** "More options" criteria apply only while the panel is open; closed = defaults (values are kept for reopening). */
  protected readonly moreOpts = signal(false);
  protected timeInput = '';
  readonly records = signal<KRecord[]>([]);
  protected readonly meta = signal<{ scanned: number; partial: boolean; ms: number } | null>(null);
  protected readonly browsing = signal(false);
  protected readonly selected = signal<KRecord | null>(null);

  // ---- live tail ----
  readonly live = signal(false);
  protected readonly livePaused = signal(false);
  private stopTail: (() => void) | null = null;
  private buffer: KRecord[] = [];
  private filterTimer?: ReturnType<typeof setTimeout>;

  constructor() {
    effect(() => {
      this.id(); this.topic();
      untracked(() => {
        this.stopLive();
        this.records.set([]);
        this.selected.set(null);
        this.browse();
      });
    });
    inject(DestroyRef).onDestroy(() => { this.stopLive(); clearTimeout(this.filterTimer); });
  }

  patch(p: Partial<BrowseReq>) { this.req.update((r) => ({ ...r, ...p })); }

  /** Applies the filter as you type: re-fetch when browsing, narrow the shown records when tailing. */
  onFilter(filter: string) {
    this.patch({ filter });
    clearTimeout(this.filterTimer);
    this.filterTimer = setTimeout(() => {
      if (!this.live()) { this.browse(); return; }
      const m = this.matcher();
      this.records.update((l) => l.filter(m));
    }, 300);
  }

  private matcher() {
    const f = this.req().filter.toLowerCase();
    return (r: KRecord) => !f || [r.key, r.value, ...r.headers.map((h) => h.key + '=' + h.value)].some((s) => s?.toLowerCase().includes(f));
  }

  /** Re-fetch after a produce; a live tail picks new records up by itself. */
  refresh() { if (!this.live()) this.browse(); }

  async browse() {
    this.stopLive();
    const r = this.moreOpts() ? { ...this.req() } : { ...this.req(), ...TopicMessages.BROWSE_DEFAULTS };
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
    const add = this.buffer.reverse().filter(this.matcher());
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

  exportJson() {
    download(`${this.topic()}-${Date.now()}.json`, JSON.stringify(this.records(), null, 2));
  }
}
