import { Component, computed, effect, inject, input, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { Api, GroupRow } from '../api';
import { NumPipe } from '../fmt';
import { Icon } from '../icon';

export function stateClass(s: string) {
  return s === 'Stable' ? 'ok' : s === 'Empty' ? '' : s === 'Dead' ? 'err' : 'warn';
}

@Component({
  selector: 'app-groups',
  imports: [FormsModule, RouterLink, NumPipe, Icon],
  template: `
<div class="page">
  <header class="page-head">
    <div>
      <h1>Consumer groups</h1>
      <p class="sub">{{ visible().length | num }} groups · total lag {{ totalLag() | num }}</p>
    </div>
    <div class="row">
      <label class="check"><input type="checkbox" [ngModel]="auto()" (ngModelChange)="setAuto($event)" /> Auto-refresh 5s</label>
      <button class="btn" (click)="load()" [disabled]="loading()" aria-label="Refresh">
        @if (loading()) { <span class="spin"></span> } @else { <app-icon name="refresh" /> }
      </button>
    </div>
  </header>

  <section class="card">
    <div class="card-head">
      <div class="search grow">
        <app-icon name="search" />
        <input class="input" placeholder="Filter by group or topic…" [ngModel]="q()" (ngModelChange)="q.set($event)" aria-label="Filter groups" />
      </div>
      <div class="seg" role="group" aria-label="State filter">
        @for (s of states; track s) {
          <button type="button" [class.on]="state() === s" (click)="state.set(s)">{{ s }}</button>
        }
      </div>
    </div>
    @if (error(); as e) { <div class="alert err m"><app-icon name="alert" /> {{ e }}</div> }
    <div class="table-wrap">
      <table class="t">
        <thead><tr><th>Group</th><th>State</th><th class="num">Members</th><th>Topics</th><th class="num">Lag</th><th class="num">Coordinator</th></tr></thead>
        <tbody>
          @for (g of visible(); track g.group) {
            <tr class="clickable" (click)="open(g)">
              <td class="name nowrap"><a [routerLink]="[g.group]" (click)="$event.stopPropagation()">{{ g.group }}</a></td>
              <td><span class="badge {{ cls(g.state) }}"><span class="dot"></span>{{ g.state || 'Unknown' }}</span></td>
              <td class="num">{{ g.members }}</td>
              <td><div class="topics">
                @for (t of g.topics.slice(0, 3); track t) { <span class="badge mono">{{ t }}</span> }
                @if (g.topics.length > 3) { <span class="dim">+{{ g.topics.length - 3 }}</span> }
              </div></td>
              <td class="num" [class.dim]="g.lag === 0" [class.lag-on]="g.lag > 0" [class.lag-hot]="g.lag > 1000">{{ g.lag | num }}</td>
              <td class="num mono dim">{{ g.coordinator }}</td>
            </tr>
          } @empty {
            @if (loading() && !groups()) {
              @for (i of [1, 2, 3, 4]; track i) { <tr><td colspan="6"><div class="skeleton"></div></td></tr> }
            } @else {
              <tr><td colspan="6"><div class="empty"><app-icon name="groups" /><strong>No consumer groups</strong><span>Groups appear once a consumer commits offsets.</span></div></td></tr>
            }
          }
        </tbody>
      </table>
    </div>
  </section>
</div>
`,
  styles: `
.grow { flex: 1; max-width: 420px; }
.topics { display: flex; gap: 4px; flex-wrap: wrap; align-items: center; }
.topics .badge { height: 20px; font-weight: 500; }
.nowrap { white-space: nowrap; }
.m { margin: 12px 14px; }
`,
})
export class GroupsPage {
  readonly id = input.required<string>();
  private readonly api = inject(Api);
  private readonly router = inject(Router);

  protected readonly states = ['All', 'Stable', 'Empty', 'Rebalancing'];
  protected readonly groups = signal<GroupRow[] | null>(null);
  protected readonly loading = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly q = signal('');
  protected readonly state = signal('All');
  protected readonly auto = signal(false);
  private timer: ReturnType<typeof setInterval> | undefined;

  protected readonly visible = computed(() => {
    const q = this.q().toLowerCase();
    const st = this.state();
    return (this.groups() ?? []).filter(
      (g) =>
        (st === 'All' || (st === 'Rebalancing' ? g.state.includes('Rebalanc') : g.state === st)) &&
        (!q || g.group.toLowerCase().includes(q) || g.topics.some((t) => t.toLowerCase().includes(q))),
    );
  });
  protected readonly totalLag = computed(() => this.visible().reduce((s, g) => s + Math.max(g.lag, 0), 0));

  protected cls = stateClass;

  constructor() {
    effect((onCleanup) => {
      this.id();
      this.groups.set(null);
      this.load();
      onCleanup(() => clearInterval(this.timer));
    });
  }

  setAuto(on: boolean) {
    this.auto.set(on);
    clearInterval(this.timer);
    if (on) this.timer = setInterval(() => this.load(), 5000);
  }

  async load() {
    this.loading.set(true);
    try {
      this.groups.set(await this.api.get<GroupRow[]>(`/c/${this.id()}/groups`));
      this.error.set(null);
    } catch (e) {
      this.error.set(e instanceof Error ? e.message : String(e));
    } finally {
      this.loading.set(false);
    }
  }

  open(g: GroupRow) { this.router.navigate(['/c', this.id(), 'groups', g.group]); }
}
