import { Component, computed, ElementRef, inject, input, linkedSignal, resource, signal, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { Api, Confirm, Favorites, Folders, Toasts, TopicRow } from '../api';
import { NumPipe } from '../fmt';
import { Icon } from '../icon';
import { CreateTopicDialog } from './create-topic-dialog';

type SortKey = 'name' | 'partitions' | 'replicationFactor' | 'messages';

// rows rendered per step: building thousands of table rows at once costs seconds
const PAGE = 200;

@Component({
  selector: 'app-topics',
  imports: [FormsModule, RouterLink, NumPipe, Icon, CreateTopicDialog],
  template: `
<div class="page">
  <header class="page-head">
    <div>
      <h1>Topics</h1>
      <p class="sub">{{ visible().length | num }} of {{ (topics() ?? []).length | num }} topics</p>
    </div>
    <div class="row">
      <button class="btn" (click)="res.reload()" [disabled]="res.isLoading()" aria-label="Refresh">
        @if (res.isLoading()) { <span class="spin"></span> } @else { <app-icon name="refresh" /> }
      </button>
      <button class="btn primary" (click)="create.open()" [disabled]="readOnly()"><app-icon name="plus" /> Create topic</button>
    </div>
  </header>

  <section class="card">
    <div class="card-head">
      <div class="search grow">
        <app-icon name="search" />
        <input class="input" placeholder="Filter topics…  (regex allowed)" [(ngModel)]="q" aria-label="Filter topics" />
      </div>
      <div class="row toggles">
        <div class="row">
          @if (folderNames().length) {
            <select class="input folder-sel" [(ngModel)]="folder" aria-label="Folder">
              <option value="">All folders</option>
              @for (f of folderNames(); track f) { <option [value]="f">{{ f }}</option> }
            </select>
            @if (folder()) {
              <button class="btn ghost icon sm" (click)="dropFolder()" title="Delete folder" [attr.aria-label]="'Delete folder ' + folder()"><app-icon name="trash" /></button>
            }
          }
          <button class="btn ghost sm" (click)="openPrefixFolder()"><app-icon name="folder" /> New folder</button>
        </div>
        <label class="check"><input type="checkbox" [(ngModel)]="favOnly" /> Favorites only</label>
        <label class="check"><input type="checkbox" [(ngModel)]="showInternal" /> Show internal</label>
      </div>
    </div>

    @if (res.error(); as e) { <div class="alert err m"><app-icon name="alert" /> {{ e.message }}</div> }

    <div class="table-wrap">
      <table class="t">
        <thead>
          <tr>
            <th class="fav"></th>
            <th class="sortable" [attr.aria-sort]="ariaSort('name')"><button type="button" (click)="sortBy('name')">Name<span class="arrow">{{ arrow('name') }}</span></button></th>
            <th class="sortable num" [attr.aria-sort]="ariaSort('partitions')"><button type="button" (click)="sortBy('partitions')">Partitions<span class="arrow">{{ arrow('partitions') }}</span></button></th>
            <th class="sortable num" [attr.aria-sort]="ariaSort('replicationFactor')"><button type="button" (click)="sortBy('replicationFactor')">Replicas<span class="arrow">{{ arrow('replicationFactor') }}</span></button></th>
            <th class="sortable num" [attr.aria-sort]="ariaSort('messages')"><button type="button" (click)="sortBy('messages')">Messages<span class="arrow">{{ arrow('messages') }}</span></button></th>
            <th>Health</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          @for (t of shown(); track t.name) {
            <tr class="clickable" (click)="open(t)">
              <td class="fav">
                <button class="btn ghost icon sm star" [class.on]="fav.has(id(), t.name)" (click)="$event.stopPropagation(); fav.toggle(id(), t.name)"
                  [attr.aria-pressed]="fav.has(id(), t.name)" [attr.aria-label]="'Favorite ' + t.name"><app-icon name="star" /></button>
              </td>
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
                <button class="btn ghost icon sm row-act" [class.on]="inAnyFolder(t.name)" (click)="$event.stopPropagation(); openFolders(t)"
                  title="Folders" [attr.aria-label]="'Folders of ' + t.name"><app-icon name="folder" /></button>
                <button class="btn ghost icon sm row-act" (click)="$event.stopPropagation(); askDelete(t)" [disabled]="readOnly() || t.internal"
                  [attr.aria-label]="'Delete ' + t.name"><app-icon name="trash" /></button>
              </td>
            </tr>
          } @empty {
            @if (res.isLoading() && !topics()) {
              @for (i of [1, 2, 3, 4, 5]; track i) { <tr><td colspan="7"><div class="skeleton"></div></td></tr> }
            } @else {
              <tr><td colspan="7"><div class="empty"><app-icon name="inbox" /><strong>No topics</strong><span>Nothing matches the current filter.</span></div></td></tr>
            }
          }
          @if (shown().length < visible().length) {
            <tr class="more"><td colspan="7">
              <button class="btn ghost sm" (click)="limit.set(limit() + PAGE * 5)">
                Show more <span class="dim">({{ shown().length | num }} of {{ visible().length | num }})</span>
              </button>
            </td></tr>
          }
        </tbody>
      </table>
    </div>
  </section>
</div>

<app-create-topic-dialog #create [id]="id()" (created)="res.reload()" />

<dialog #folderDlg class="dlg">
  <div class="dlg-head">
    <div><h2>Folders</h2><p>Group <b class="mono">{{ folderTopic() }}</b> with other topics. Saved in this browser.</p></div>
    <button type="button" class="btn ghost icon sm" (click)="folderDlg.close()" aria-label="Close"><app-icon name="x" /></button>
  </div>
  <div class="dlg-body">
    @for (f of folderNames(); track f) {
      @let rule = folders.rule(id(), f, folderTopic());
      <label class="check" [title]="rule ? 'Every topic starting with ' + rule.slice(0, -1) + ' is in this folder' : ''">
        <input type="checkbox" [ngModel]="folders.has(id(), f, folderTopic())" (ngModelChange)="folders.put(id(), f, [folderTopic()], $event)" [disabled]="!!rule" />
        {{ f }} @if (rule) { <span class="mono dim">{{ rule }}</span> }
      </label>
    }
    <form class="row" (ngSubmit)="addFolder()">
      <input class="input grow" name="folder" [(ngModel)]="newFolder" placeholder="New folder" aria-label="New folder name" autocomplete="off" />
      <button class="btn" [disabled]="!newFolder.trim()"><app-icon name="plus" /> Add</button>
    </form>
  </div>
</dialog>

<dialog #prefixDlg class="dlg">
  <form (ngSubmit)="createPrefixFolder()" #pf="ngForm">
    <div class="dlg-head">
      <div><h2>New folder</h2><p>Holds every topic starting with the prefix, including topics created later, or just the ones you choose.</p></div>
      <button type="button" class="btn ghost icon sm" (click)="prefixDlg.close()" aria-label="Close"><app-icon name="x" /></button>
    </div>
    <div class="dlg-body">
      <label class="field"><span>Topics starting with</span>
        <input class="input mono" name="prefix" pattern="[a-zA-Z0-9._-]+" [(ngModel)]="prefix" autofocus autocomplete="off" placeholder="orders." />
        <span class="hint">{{ prefixMatches() | num }} topics match now, ignoring case. Choosing topics below saves only those.</span>
      </label>
      <div class="field"><span>Topics</span>
        <details class="pick">
          <summary class="input">{{ chosen().length ? (chosen().length | num) + ' selected' : 'Choose topics…' }} <app-icon name="chevron" class="dim" /></summary>
          <div class="pick-list">
            <input class="input" [(ngModel)]="pickQ" [ngModelOptions]="{ standalone: true }" (keydown.enter)="$event.preventDefault()"
              placeholder="Filter…" aria-label="Filter topics to add" autocomplete="off" />
            @if (matching().length) {
              <label class="check all"><input type="checkbox" [checked]="allPicked()" (change)="pickAll($any($event.target).checked)" /> Select all ({{ matching().length | num }})</label>
            }
            @for (t of pickable(); track t) {
              <label class="check"><input type="checkbox" [checked]="picked().has(t)" (change)="togglePick(t)" /> <span class="mono">{{ t }}</span></label>
            } @empty { <span class="hint">No topics match</span> }
            @if (matching().length > PAGE) { <span class="hint">First {{ PAGE }} shown, filter to find more</span> }
          </div>
        </details>
      </div>
      <label class="field"><span>Name</span>
        <input class="input" name="name" [(ngModel)]="prefixName" autocomplete="off" [placeholder]="prefix() || 'orders'" />
        <span class="hint">Defaults to the prefix. An existing name adds to that folder.</span>
      </label>
    </div>
    <div class="dlg-foot">
      <button type="button" class="btn ghost" (click)="prefixDlg.close()">Cancel</button>
      <button class="btn primary" [disabled]="!pf.valid || !(prefix().trim() || (chosen().length && prefixName.trim()))">Create</button>
    </div>
  </form>
</dialog>

`,
  styles: `
.grow { flex: 1; max-width: 420px; }
.sm-badge { margin-left: 8px; height: 18px; font-size: 10.5px; }
.acts, .fav { width: 1%; }
td.fav { padding-right: 0; }
.healthy { display: inline-flex; align-items: center; gap: 6px; font-size: 12.5px; color: var(--text-2); }
.healthy .dot { color: var(--ok); }
.more td { text-align: center; padding: 10px; }
.toggles { gap: 18px; }
.folder-sel { width: auto; max-width: 200px; }
tr .row-act.on { opacity: 1; color: var(--accent); }
.pick summary { display: flex; align-items: center; justify-content: space-between; cursor: pointer; list-style: none; }
.pick summary::-webkit-details-marker { display: none; }
.pick-list { margin-top: 6px; padding: 8px; display: grid; gap: 6px; max-height: 240px; overflow: auto; border: 1px solid var(--border); border-radius: var(--radius-sm); }
.pick-list .all { padding-bottom: 6px; border-bottom: 1px solid var(--border); font-weight: 500; }
`,
})
export class TopicsPage {
  readonly id = input.required<string>();
  private readonly api = inject(Api);
  protected readonly fav = inject(Favorites);
  protected readonly folders = inject(Folders);
  private readonly toasts = inject(Toasts);
  private readonly confirm = inject(Confirm);
  private readonly router = inject(Router);
  private readonly folderDlg = viewChild.required<ElementRef<HTMLDialogElement>>('folderDlg');
  private readonly prefixDlg = viewChild.required<ElementRef<HTMLDialogElement>>('prefixDlg');

  protected readonly res = resource({ params: () => this.id(), loader: ({ params }) => this.api.get<TopicRow[]>(`/c/${params}/topics`) });
  protected readonly topics = computed(() => (this.res.hasValue() ? this.res.value() : null));
  protected readonly q = signal('');
  protected readonly showInternal = signal(false);
  protected readonly favOnly = signal(false);
  protected readonly sort = signal<{ key: SortKey; dir: 1 | -1 }>({ key: 'name', dir: 1 });
  protected readonly folderNames = computed(() => this.folders.names(this.id()));
  // back to all topics when the chosen folder disappears (emptied, deleted, other cluster)
  protected readonly folder = linkedSignal<string[], string>({
    source: this.folderNames,
    computation: (names, prev) => (prev && names.includes(prev.value) ? prev.value : ''),
  });
  protected readonly folderTopic = signal('');
  protected newFolder = '';
  protected readonly prefix = signal('');
  protected prefixName = '';
  protected readonly prefixMatches = computed(() => {
    const p = this.prefix().trim();
    return p ? (this.topics() ?? []).filter((t) => Folders.startsWith(t.name, p)).length : 0;
  });
  protected readonly picked = signal(new Set<string>());
  protected readonly pickQ = signal('');
  // checked topics still under the prefix: editing it after picking drops the ones it no longer covers
  protected readonly chosen = computed(() => [...this.picked()].filter((t) => Folders.startsWith(t, this.prefix().trim())));
  protected readonly matching = computed(() => {
    const p = this.prefix().trim();
    const q = this.pickQ().trim().toLowerCase();
    return (this.topics() ?? [])
      .filter((t) => (this.showInternal() || !t.internal) && Folders.startsWith(t.name, p) && t.name.toLowerCase().includes(q))
      .map((t) => t.name).sort((a, b) => a.localeCompare(b));
  });
  protected readonly pickable = computed(() => this.matching().slice(0, PAGE));
  protected readonly allPicked = computed(() => this.matching().every((t) => this.picked().has(t)));
  protected readonly readOnly = computed(() => this.api.clusters().find((c) => c.id === this.id())?.readOnly ?? false);

  protected readonly visible = computed(() => {
    const q = this.q().trim();
    let re: RegExp | null = null;
    try { re = q ? new RegExp(q, 'i') : null; } catch { re = null; }
    const { key, dir } = this.sort();
    const favs = this.fav.set(this.id());
    const fav = (t: TopicRow) => favs.has(t.name);
    const folder = this.folder();
    return (this.topics() ?? [])
      .filter((t) => (this.showInternal() || !t.internal) && (!this.favOnly() || fav(t)) && (!folder || this.folders.has(this.id(), folder, t.name)) && (!q || (re ? re.test(t.name) : t.name.includes(q))))
      // favorites stay pinned on top, each group in the chosen order
      .sort((a, b) => Number(fav(b)) - Number(fav(a)) || (a[key] < b[key] ? -dir : a[key] > b[key] ? dir : 0));
  });

  // back to the first page when the filters or the order change, not on a star or folder edit
  protected readonly limit = linkedSignal(() => (this.id(), this.q(), this.folder(), this.favOnly(), this.showInternal(), this.sort(), PAGE));
  protected readonly shown = computed(() => this.visible().slice(0, this.limit()));
  protected readonly PAGE = PAGE;

  sortBy(key: SortKey) {
    this.sort.update((s) => ({ key, dir: s.key === key ? (-s.dir as 1 | -1) : key === 'name' ? 1 : -1 }));
  }
  arrow(key: SortKey) {
    const s = this.sort();
    return s.key === key ? (s.dir === 1 ? '↑' : '↓') : '';
  }
  ariaSort(key: SortKey) {
    const s = this.sort();
    return s.key === key ? (s.dir === 1 ? 'ascending' : 'descending') : null;
  }

  open(t: TopicRow) { this.router.navigate(['/c', this.id(), 'topics', t.name]); }

  inAnyFolder(topic: string) { return this.folderNames().some((f) => this.folders.has(this.id(), f, topic)); }

  openFolders(t: TopicRow) {
    this.folderTopic.set(t.name);
    this.newFolder = '';
    this.folderDlg().nativeElement.showModal();
  }

  addFolder() {
    const name = this.newFolder.trim();
    if (!name) return;
    this.folders.put(this.id(), name, [this.folderTopic()], true);
    this.newFolder = '';
  }

  openPrefixFolder() {
    const q = this.q().trim();
    this.prefix.set(/^[\w.-]+$/.test(q) ? q : ''); // start from the search box when it holds a plain name
    this.prefixName = '';
    this.picked.set(new Set());
    this.pickQ.set('');
    this.prefixDlg().nativeElement.showModal();
  }

  /** Checks or unchecks every topic matching the filters, beyond the ones shown. */
  pickAll(on: boolean) {
    this.picked.update((s) => { const n = new Set(s); for (const t of this.matching()) if (on) n.add(t); else n.delete(t); return n; });
  }

  togglePick(t: string) {
    this.picked.update((s) => { const n = new Set(s); if (!n.delete(t)) n.add(t); return n; });
  }

  createPrefixFolder() {
    const p = this.prefix().trim();
    const name = this.prefixName.trim() || p;
    const chosen = this.chosen();
    if (!name || !(p || chosen.length)) return;
    // the prefix only narrows the list once topics are chosen
    this.folders.put(this.id(), name, chosen.length ? chosen : [p + '*'], true);
    this.folder.set(name);
    this.prefixDlg().nativeElement.close();
  }

  async dropFolder() {
    const f = this.folder();
    if (await this.confirm.ask('Delete folder', `Delete the folder "${f}"? Its topics are not touched.`, 'Delete folder')) this.folders.remove(this.id(), f);
  }

  async askDelete(t: TopicRow) {
    if (!(await this.confirm.ask('Delete topic', `This permanently deletes ${t.name} and all of its messages.`, 'Delete topic', t.name))) return;
    return this.toasts.guard(async () => {
      await this.api.del(`/c/${this.id()}/topics/${Api.seg(t.name)}`);
      this.toasts.ok(`Deleted ${t.name}`);
      // not while another cluster loads: set() would cancel that load
      if (this.res.hasValue()) this.res.value.update((l) => l?.filter((x) => x.name !== t.name));
    });
  }
}
