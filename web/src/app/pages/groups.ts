import { Component, computed, effect, inject, input, resource, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { Api, GroupRow } from '../api';
import { NumPipe, stateClass } from '../fmt';
import { Icon } from '../icon';

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
      <label class="check"><input type="checkbox" [(ngModel)]="auto" /> Auto-refresh 5s</label>
      <button class="btn" (click)="res.reload()" [disabled]="res.isLoading()" aria-label="Refresh">
        @if (res.isLoading()) { <span class="spin"></span> } @else { <app-icon name="refresh" /> }
      </button>
    </div>
  </header>

  <section class="card">
    <div class="card-head">
      <div class="search grow">
        <app-icon name="search" />
        <input class="input" placeholder="Filter by group or topic…" [(ngModel)]="q" aria-label="Filter groups" />
      </div>
      <div class="seg" role="group" aria-label="State filter">
        @for (s of states; track s) {
          <button type="button" [class.on]="state() === s" (click)="state.set(s)">{{ s }}</button>
        }
      </div>
    </div>
    @if (res.error(); as e) { <div class="alert err m"><app-icon name="alert" /> {{ e.message }}</div> }
    <div class="table-wrap">
      <table class="t">
        <thead><tr><th>Group</th><th>State</th><th class="num">Members</th><th>Topics</th><th class="num">Lag</th><th class="num">Coordinator</th></tr></thead>
        <tbody>
          @for (g of visible(); track g.group) {
            <tr class="clickable" (click)="open(g)">
              <td class="name nowrap"><a [routerLink]="['/c', id(), 'groups', g.group]" (click)="$event.stopPropagation()">{{ g.group }}</a></td>
              <td>@if (g.error) { <span class="badge err" [title]="g.error">Error</span> } @else { <span class="badge {{ cls(g.state) }}"><span class="dot"></span>{{ g.state || 'Unknown' }}</span> }</td>
              <td class="num">{{ g.members }}</td>
              <td><div class="chips">
                @for (t of g.topics.slice(0, 3); track t) { <span class="badge mono">{{ t }}</span> }
                @if (g.topics.length > 3) { <span class="dim">+{{ g.topics.length - 3 }}</span> }
              </div></td>
              <td class="num" [class.dim]="g.lag === 0" [class.lag-on]="g.lag > 0" [class.lag-hot]="g.lag > 1000">{{ g.lag | num }}</td>
              <td class="num mono dim">{{ g.coordinator }}</td>
            </tr>
          } @empty {
            @if (res.isLoading() && !res.hasValue()) {
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
`,
})
export class GroupsPage {
  readonly id = input.required<string>();
  private readonly api = inject(Api);
  private readonly router = inject(Router);

  protected readonly states = ['All', 'Stable', 'Empty', 'Rebalancing'];
  protected readonly res = resource({ params: () => this.id(), loader: ({ params }) => this.api.get<GroupRow[]>(`/c/${params}/groups`) });
  protected readonly q = signal('');
  protected readonly state = signal('All');
  protected readonly auto = signal(false);

  protected readonly visible = computed(() => {
    const q = this.q().toLowerCase();
    const st = this.state();
    return (this.res.hasValue() ? this.res.value() : []).filter(
      (g) =>
        (st === 'All' || (st === 'Rebalancing' ? g.state.includes('Rebalanc') : g.state === st)) &&
        (!q || g.group.toLowerCase().includes(q) || g.topics.some((t) => t.toLowerCase().includes(q))),
    );
  });
  protected readonly totalLag = computed(() => this.visible().reduce((s, g) => s + Math.max(g.lag, 0), 0));

  protected cls = stateClass;

  constructor() {
    effect((onCleanup) => {
      if (!this.auto()) return;
      const t = setInterval(() => this.res.reload(), 5000);
      onCleanup(() => clearInterval(t));
    });
  }

  open(g: GroupRow) { this.router.navigate(['/c', this.id(), 'groups', g.group]); }
}
