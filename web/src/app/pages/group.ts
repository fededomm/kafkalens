import { Component, computed, DestroyRef, effect, ElementRef, inject, input, signal, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { Api, GroupDetail, Toasts } from '../api';
import { NumPipe } from '../fmt';
import { DateTime } from '../datetime';
import { Icon } from '../icon';
import { stateClass } from './groups';

@Component({
  selector: 'app-group',
  imports: [FormsModule, RouterLink, NumPipe, Icon, DateTime],
  template: `
<div class="page">
  <header class="page-head">
    <div class="title">
      <nav class="crumbs dim"><a [routerLink]="['/c', id(), 'groups']">Consumer groups</a> <app-icon name="chevron-right" /></nav>
      <h1 class="mono">{{ group() }}</h1>
      @if (d(); as g) {
        <div class="row wrap">
          <span class="badge {{ cls(g.state) }}"><span class="dot"></span>{{ g.state }}</span>
          <span class="badge">{{ g.members.length }} members</span>
          @if (g.protocol) { <span class="badge">{{ g.protocol }}</span> }
          <span class="badge">coordinator {{ g.coordinator }}</span>
        </div>
      }
    </div>
    <div class="row">
      <label class="check"><input type="checkbox" [ngModel]="auto()" (ngModelChange)="setAuto($event)" /> Auto-refresh</label>
      <button class="btn" (click)="load()" aria-label="Refresh"><app-icon name="refresh" /></button>
      <button class="btn" (click)="openReset()" [disabled]="readOnly() || !d()?.partitions?.length"><app-icon name="rewind" /> Reset offsets</button>
      <button class="btn danger icon" (click)="remove()" [disabled]="readOnly()" aria-label="Delete group" title="Delete group"><app-icon name="trash" /></button>
    </div>
  </header>

  @if (error(); as e) { <div class="alert err"><app-icon name="alert" /> {{ e }}</div> }

  <div class="grid-stats">
    <div class="card stat"><span class="label">Total lag</span><span class="value" [class.warn]="(d()?.lag ?? 0) > 0" [class.ok]="d()?.lag === 0">{{ d()?.lag | num }}</span></div>
    <div class="card stat"><span class="label">Topics</span><span class="value">{{ byTopic().length }}</span></div>
    <div class="card stat"><span class="label">Partitions</span><span class="value">{{ d()?.partitions?.length ?? '—' }}</span></div>
    <div class="card stat"><span class="label">Members</span><span class="value">{{ d()?.members?.length ?? '—' }}</span></div>
  </div>

  @for (t of byTopic(); track t.topic) {
    <section class="card">
      <div class="card-head">
        <h2><a class="mono" [routerLink]="['/c', id(), 'topics', t.topic]">{{ t.topic }}</a></h2>
        <span class="muted">lag <b [class.lag-on]="t.lag > 0" [class.lag-hot]="t.lag > 1000">{{ t.lag | num }}</b></span>
      </div>
      <div class="table-wrap">
        <table class="t">
          <thead><tr><th class="part">Partition</th><th class="num">Committed</th><th class="num">End offset</th><th class="num">Lag</th><th>Progress</th><th>Consumer</th></tr></thead>
          <tbody>
            @for (p of t.parts; track p.partition) {
              <tr>
                <td class="mono part">{{ p.partition }}</td>
                <td class="num mono">{{ p.committed < 0 ? '—' : (p.committed | num) }}</td>
                <td class="num mono">{{ p.end | num }}</td>
                <td class="num" [class.dim]="p.lag === 0" [class.lag-on]="p.lag > 0" [class.lag-hot]="p.lag > 1000">{{ p.lag | num }}</td>
                <td><div class="bar" [class.warn]="p.lag > 0" [title]="p.lag > 0 ? p.lag + ' behind' : 'caught up'"><i [style.width.%]="p.end > 0 && p.committed >= 0 ? (p.committed / p.end) * 100 : 100"></i></div></td>
                <td class="mono dim">{{ p.member ?? '—' }}</td>
              </tr>
            }
          </tbody>
        </table>
      </div>
    </section>
  }

  <section class="card">
    <div class="card-head"><h2>Members</h2></div>
    <div class="table-wrap">
      <table class="t">
        <thead><tr><th>Client ID</th><th>Host</th><th>Member ID</th><th>Assigned partitions</th></tr></thead>
        <tbody>
          @for (m of d()?.members ?? []; track m.id) {
            <tr>
              <td class="mono">{{ m.clientId }}</td>
              <td class="mono dim">{{ m.host }}</td>
              <td class="mono dim trunc member">{{ m.instanceId ?? m.id }}</td>
              <td class="assigned">@for (a of m.assigned; track a) { <span class="badge mono">{{ a }}</span> }</td>
            </tr>
          } @empty {
            <tr><td colspan="4"><div class="empty"><app-icon name="groups" /><span>No active members — the group is idle, so offsets can be reset.</span></div></td></tr>
          }
        </tbody>
      </table>
    </div>
  </section>
</div>

<dialog #resetDlg class="dlg">
  <form (ngSubmit)="reset()">
    <div class="dlg-head">
      <div><h2>Reset offsets</h2><p>The group must have no active members. Consumers resume from the new position on restart.</p></div>
      <button type="button" class="btn ghost icon sm" (click)="resetDlg.close()" aria-label="Close"><app-icon name="x" /></button>
    </div>
    <div class="dlg-body">
      @if ((d()?.members?.length ?? 0) > 0) {
        <div class="alert warn"><app-icon name="alert" /> {{ d()?.members?.length }} consumer(s) are still connected — stop them first.</div>
      }
      <div class="form-grid">
        <label class="field"><span>Topic</span>
          <select class="input" name="topic" [(ngModel)]="rs.topic">
            @for (t of byTopic(); track t.topic) { <option [value]="t.topic">{{ t.topic }}</option> }
          </select>
        </label>
        <label class="field"><span>Partition</span>
          <select class="input" name="part" [(ngModel)]="rs.partition">
            <option [ngValue]="null">All partitions</option>
            @for (p of partsOf(rs.topic); track p) { <option [ngValue]="p">Partition {{ p }}</option> }
          </select>
        </label>
        <div class="field full"><span>Move to</span>
          <div class="seg">
            @for (o of targets; track o.v) {
              <button type="button" [attr.aria-pressed]="rs.to === o.v" [class.on]="rs.to === o.v" (click)="rs.to = o.v">{{ o.l }}</button>
            }
          </div>
        </div>
        @if (rs.to === 'offset') {
          <label class="field full"><span>Offset</span><input class="input mono" type="number" min="0" name="off" [(ngModel)]="rs.offset" /></label>
        }
        @if (rs.to === 'timestamp') {
          <div class="field full"><span>Date & time</span><app-datetime name="ts" label="Date & time" [(ngModel)]="rs.time" /></div>
        }
      </div>
    </div>
    <div class="dlg-foot">
      <button type="button" class="btn ghost" (click)="resetDlg.close()">Cancel</button>
      <button class="btn primary" [disabled]="busy() || (d()?.members?.length ?? 0) > 0">@if (busy()) { <span class="spin"></span> } Reset</button>
    </div>
  </form>
</dialog>
`,
  styles: `
.title { display: grid; gap: 6px; min-width: 0; }
.title h1 { font-size: 20px; word-break: break-all; }
.crumbs { display: flex; align-items: center; gap: 4px; font-size: 12.5px; }
.part { width: 90px; }
.bar { max-width: 200px; }
.member { max-width: 260px; }
.assigned { display: flex; gap: 4px; flex-wrap: wrap; }
.assigned .badge { height: 20px; font-weight: 500; }
`,
})
export class GroupPage {
  readonly id = input.required<string>();
  readonly group = input.required<string>();
  private readonly api = inject(Api);
  private readonly toasts = inject(Toasts);
  private readonly router = inject(Router);
  private readonly resetDlg = viewChild.required<ElementRef<HTMLDialogElement>>('resetDlg');

  protected readonly d = signal<GroupDetail | null>(null);
  protected readonly error = signal<string | null>(null);
  protected readonly busy = signal(false);
  protected readonly auto = signal(false);
  protected readonly readOnly = computed(() => this.api.clusters().find((c) => c.id === this.id())?.readOnly ?? false);
  private timer: ReturnType<typeof setInterval> | undefined;

  protected readonly byTopic = computed(() => {
    const m = new Map<string, { topic: string; lag: number; parts: GroupDetail['partitions'] }>();
    for (const p of this.d()?.partitions ?? []) {
      const t = m.get(p.topic) ?? { topic: p.topic, lag: 0, parts: [] };
      t.lag += Math.max(p.lag, 0);
      t.parts.push(p);
      m.set(p.topic, t);
    }
    return [...m.values()];
  });

  protected readonly targets = [
    { v: 'earliest', l: 'Earliest' }, { v: 'latest', l: 'Latest' }, { v: 'timestamp', l: 'Time' }, { v: 'offset', l: 'Offset' },
  ];
  protected rs = { topic: '', partition: null as number | null, to: 'earliest', offset: 0, time: '' };
  protected cls = stateClass;

  constructor() {
    effect(() => { this.id(); this.group(); this.d.set(null); this.load(); });
    inject(DestroyRef).onDestroy(() => clearInterval(this.timer));
  }

  async load() {
    try {
      this.d.set(await this.api.get<GroupDetail>(`/c/${this.id()}/groups/${Api.seg(this.group())}`));
      this.error.set(null);
    } catch (e) {
      this.error.set(e instanceof Error ? e.message : String(e));
    }
  }

  setAuto(on: boolean) {
    this.auto.set(on);
    clearInterval(this.timer);
    if (on) this.timer = setInterval(() => this.load(), 3000);
  }

  partsOf(topic: string) {
    return this.byTopic().find((t) => t.topic === topic)?.parts.map((p) => p.partition) ?? [];
  }

  openReset() {
    this.rs = { topic: this.byTopic()[0]?.topic ?? '', partition: null, to: 'earliest', offset: 0, time: '' };
    this.resetDlg().nativeElement.showModal();
  }

  async reset() {
    this.busy.set(true);
    try {
      await this.api.post(`/c/${this.id()}/groups/${Api.seg(this.group())}/reset`, {
        topic: this.rs.topic, to: this.rs.to, partition: this.rs.partition, offset: this.rs.offset,
        timestamp: this.rs.time ? new Date(this.rs.time).getTime() : 0,
      });
      this.resetDlg().nativeElement.close();
      this.toasts.ok('Offsets reset');
      this.load();
    } catch (e) {
      this.toasts.err(e);
    } finally {
      this.busy.set(false);
    }
  }

  async remove() {
    if (!confirm(`Delete consumer group "${this.group()}"? Its committed offsets are lost.`)) return;
    try {
      await this.api.del(`/c/${this.id()}/groups/${Api.seg(this.group())}`);
      this.toasts.ok(`Deleted ${this.group()}`);
      this.router.navigate(['/c', this.id(), 'groups']);
    } catch (e) {
      this.toasts.err(e);
    }
  }
}
