import { Component, computed, effect, ElementRef, inject, input, signal, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { Api, Toasts, TopicRow } from '../api';
import { NumPipe } from '../fmt';
import { Icon } from '../icon';

type SortKey = 'name' | 'partitions' | 'replicationFactor' | 'messages';

@Component({
  selector: 'app-topics',
  imports: [FormsModule, RouterLink, NumPipe, Icon],
  template: `
<div class="page">
  <header class="page-head">
    <div>
      <h1>Topics</h1>
      <p class="sub">{{ visible().length | num }} of {{ (topics() ?? []).length | num }} topics</p>
    </div>
    <div class="row">
      <button class="btn" (click)="load()" [disabled]="loading()" aria-label="Refresh">
        @if (loading()) { <span class="spin"></span> } @else { <app-icon name="refresh" /> }
      </button>
      <button class="btn primary" (click)="openCreate()" [disabled]="readOnly()"><app-icon name="plus" /> Create topic</button>
    </div>
  </header>

  <section class="card">
    <div class="card-head">
      <div class="search grow">
        <app-icon name="search" />
        <input class="input" placeholder="Filter topics…  (regex allowed)" [ngModel]="q()" (ngModelChange)="q.set($event)" aria-label="Filter topics" />
      </div>
      <label class="check"><input type="checkbox" [ngModel]="showInternal()" (ngModelChange)="showInternal.set($event)" /> Show internal</label>
    </div>

    @if (error(); as e) { <div class="alert err m"><app-icon name="alert" /> {{ e }}</div> }

    <div class="table-wrap">
      <table class="t">
        <thead>
          <tr>
            <th class="sortable" (click)="sortBy('name')">Name<span class="arrow">{{ arrow('name') }}</span></th>
            <th class="sortable num" (click)="sortBy('partitions')">Partitions<span class="arrow">{{ arrow('partitions') }}</span></th>
            <th class="sortable num" (click)="sortBy('replicationFactor')">Replicas<span class="arrow">{{ arrow('replicationFactor') }}</span></th>
            <th class="sortable num" (click)="sortBy('messages')">Messages<span class="arrow">{{ arrow('messages') }}</span></th>
            <th>Health</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          @for (t of visible(); track t.name) {
            <tr class="clickable" (click)="open(t)">
              <td class="name">
                <a [routerLink]="[t.name]" (click)="$event.stopPropagation()">{{ t.name }}</a>
                @if (t.internal) { <span class="badge sm-badge">internal</span> }
              </td>
              <td class="num">{{ t.partitions | num }}</td>
              <td class="num">{{ t.replicationFactor | num }}</td>
              <td class="num" [title]="t.messages | num">{{ t.messages | num: 'compact' }}</td>
              <td>
                @if (t.error) { <span class="badge err" [title]="t.error">Error</span> }
                @else if (t.underReplicated) { <span class="badge warn"><span class="dot"></span>{{ t.underReplicated }} under-replicated</span> }
                @else { <span class="healthy"><span class="dot"></span>Healthy</span> }
              </td>
              <td class="acts">
                <button class="btn ghost icon sm row-act" (click)="$event.stopPropagation(); askDelete(t)" [disabled]="readOnly() || t.internal"
                  [attr.aria-label]="'Delete ' + t.name"><app-icon name="trash" /></button>
              </td>
            </tr>
          } @empty {
            @if (loading() && !topics()) {
              @for (i of [1, 2, 3, 4, 5]; track i) { <tr><td colspan="6"><div class="skeleton"></div></td></tr> }
            } @else {
              <tr><td colspan="6"><div class="empty"><app-icon name="inbox" /><strong>No topics</strong><span>Nothing matches the current filter.</span></div></td></tr>
            }
          }
        </tbody>
      </table>
    </div>
  </section>
</div>

<dialog #createDlg class="dlg">
  <form (ngSubmit)="create()" #f="ngForm">
    <div class="dlg-head">
      <div><h2>Create topic</h2><p>Leave partitions / replicas empty to use broker defaults.</p></div>
      <button type="button" class="btn ghost icon sm" (click)="createDlg.close()" aria-label="Close"><app-icon name="x" /></button>
    </div>
    <div class="dlg-body form-grid">
      <label class="field full"><span>Name</span>
        <input class="input mono" name="name" required pattern="[a-zA-Z0-9._-]+" [(ngModel)]="nt.name" autofocus placeholder="orders.v1" />
        <span class="hint">Letters, digits, dot, underscore, dash</span>
      </label>
      <label class="field"><span>Partitions</span><input class="input" type="number" min="1" name="p" [(ngModel)]="nt.partitions" /></label>
      <label class="field"><span>Replication factor</span><input class="input" type="number" min="1" name="rf" [(ngModel)]="nt.replicationFactor" /></label>
      <label class="field"><span>Cleanup policy</span>
        <select class="input" name="cp" [(ngModel)]="nt.cleanup">
          <option value="">Default</option><option value="delete">delete</option><option value="compact">compact</option><option value="compact,delete">compact,delete</option>
        </select>
      </label>
      <label class="field"><span>Retention</span>
        <select class="input" name="ret" [(ngModel)]="nt.retention">
          <option value="">Default</option><option value="3600000">1 hour</option><option value="86400000">1 day</option>
          <option value="604800000">7 days</option><option value="2592000000">30 days</option><option value="-1">Forever</option>
        </select>
      </label>
    </div>
    <div class="dlg-foot">
      <button type="button" class="btn ghost" (click)="createDlg.close()">Cancel</button>
      <button class="btn primary" [disabled]="!f.valid || busy()">@if (busy()) { <span class="spin"></span> } Create</button>
    </div>
  </form>
</dialog>

<dialog #deleteDlg class="dlg">
  <form (ngSubmit)="confirmDelete()">
    <div class="dlg-head">
      <div><h2>Delete topic</h2><p>This permanently deletes <b class="mono">{{ toDelete()?.name }}</b> and all of its messages.</p></div>
    </div>
    <div class="dlg-body">
      <label class="field"><span>Type the topic name to confirm</span>
        <input class="input mono" name="confirm" [(ngModel)]="confirmText" autocomplete="off" />
      </label>
    </div>
    <div class="dlg-foot">
      <button type="button" class="btn ghost" (click)="deleteDlg.close()">Cancel</button>
      <button class="btn danger solid" [disabled]="confirmText !== toDelete()?.name || busy()">Delete topic</button>
    </div>
  </form>
</dialog>
`,
  styles: `
.grow { flex: 1; max-width: 420px; }
.sm-badge { margin-left: 8px; height: 18px; font-size: 10.5px; }
.acts { width: 1%; }
.healthy { display: inline-flex; align-items: center; gap: 6px; font-size: 12.5px; color: var(--text-2); }
.healthy .dot { color: var(--ok); }
.m { margin: 12px 14px; }
`,
})
export class TopicsPage {
  readonly id = input.required<string>();
  private readonly api = inject(Api);
  private readonly toasts = inject(Toasts);
  private readonly router = inject(Router);
  private readonly createDlg = viewChild.required<ElementRef<HTMLDialogElement>>('createDlg');
  private readonly deleteDlg = viewChild.required<ElementRef<HTMLDialogElement>>('deleteDlg');

  protected readonly topics = signal<TopicRow[] | null>(null);
  protected readonly loading = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly q = signal('');
  protected readonly showInternal = signal(false);
  protected readonly sort = signal<{ key: SortKey; dir: 1 | -1 }>({ key: 'name', dir: 1 });
  protected readonly readOnly = computed(() => this.api.clusters().find((c) => c.id === this.id())?.readOnly ?? false);

  protected readonly visible = computed(() => {
    const q = this.q().trim();
    let re: RegExp | null = null;
    try { re = q ? new RegExp(q, 'i') : null; } catch { re = null; }
    const { key, dir } = this.sort();
    return (this.topics() ?? [])
      .filter((t) => (this.showInternal() || !t.internal) && (!q || (re ? re.test(t.name) : t.name.includes(q))))
      .sort((a, b) => (a[key] < b[key] ? -dir : a[key] > b[key] ? dir : 0));
  });

  protected nt = { name: '', partitions: null as number | null, replicationFactor: null as number | null, cleanup: '', retention: '' };
  protected readonly toDelete = signal<TopicRow | null>(null);
  protected confirmText = '';

  constructor() {
    effect(() => { this.id(); this.topics.set(null); this.load(); });
  }

  async load() {
    this.loading.set(true);
    try {
      this.topics.set(await this.api.get<TopicRow[]>(`/c/${this.id()}/topics`));
      this.error.set(null);
    } catch (e) {
      this.error.set(e instanceof Error ? e.message : String(e));
    } finally {
      this.loading.set(false);
    }
  }

  sortBy(key: SortKey) {
    this.sort.update((s) => ({ key, dir: s.key === key ? (-s.dir as 1 | -1) : key === 'name' ? 1 : -1 }));
  }
  arrow(key: SortKey) {
    const s = this.sort();
    return s.key === key ? (s.dir === 1 ? '↑' : '↓') : '';
  }

  open(t: TopicRow) { this.router.navigate(['/c', this.id(), 'topics', t.name]); }

  openCreate() {
    this.nt = { name: '', partitions: null, replicationFactor: null, cleanup: '', retention: '' };
    this.createDlg().nativeElement.showModal();
  }

  async create() {
    const configs: Record<string, string> = {};
    if (this.nt.cleanup) configs['cleanup.policy'] = this.nt.cleanup;
    if (this.nt.retention) configs['retention.ms'] = this.nt.retention;
    this.busy.set(true);
    try {
      await this.api.post(`/c/${this.id()}/topics`, {
        name: this.nt.name, partitions: this.nt.partitions ?? 0, replicationFactor: this.nt.replicationFactor ?? 0, configs,
      });
      this.createDlg().nativeElement.close();
      this.toasts.ok(`Created ${this.nt.name}`);
      this.load();
    } catch (e) {
      this.toasts.err(e);
    } finally {
      this.busy.set(false);
    }
  }

  askDelete(t: TopicRow) {
    this.toDelete.set(t);
    this.confirmText = '';
    this.deleteDlg().nativeElement.showModal();
  }

  async confirmDelete() {
    const t = this.toDelete();
    if (!t || this.confirmText !== t.name) return;
    this.busy.set(true);
    try {
      await this.api.del(`/c/${this.id()}/topics/${Api.seg(t.name)}`);
      this.deleteDlg().nativeElement.close();
      this.toasts.ok(`Deleted ${t.name}`);
      this.topics.update((l) => l?.filter((x) => x.name !== t.name) ?? null);
    } catch (e) {
      this.toasts.err(e);
    } finally {
      this.busy.set(false);
    }
  }
}
