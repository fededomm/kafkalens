import { Component, computed, ElementRef, inject, input, resource, signal, viewChild } from '@angular/core';
import { RouterLink } from '@angular/router';
import { Api, Broker, ConfigEntry, Overview, Toasts } from '../api';
import { NumPipe } from '../fmt';
import { Icon } from '../icon';
import { ConfigTable } from './config-table';

@Component({
  selector: 'app-overview',
  imports: [RouterLink, NumPipe, Icon, ConfigTable],
  template: `
<div class="page">
  <header class="page-head">
    <div>
      <h1>Overview</h1>
      <p class="sub mono">{{ data()?.clusterId ?? '…' }}</p>
    </div>
    <button class="btn" (click)="res.reload()" [disabled]="res.isLoading()">
      @if (res.isLoading()) { <span class="spin"></span> } @else { <app-icon name="refresh" /> } Refresh
    </button>
  </header>

  @if (res.error(); as e) {
    <div class="alert err"><app-icon name="alert" /> <div><b>Can't reach the cluster.</b> {{ e.message }}</div></div>
  }

  <div class="grid-stats">
    <button class="card stat" (click)="brokersSec.scrollIntoView({ behavior: 'smooth' })">
      <span class="label">Brokers</span><span class="value">{{ data()?.brokers?.length | num }}</span>
    </button>
    <a class="card stat" [routerLink]="['/c', id(), 'topics']">
      <span class="label">Topics</span><span class="value">{{ data()?.topics | num }}</span>
    </a>
    <div class="card stat"><span class="label">Partitions</span><span class="value">{{ data()?.partitions | num }}</span></div>
    <a class="card stat" [routerLink]="['/c', id(), 'groups']">
      <span class="label">Consumer groups</span><span class="value">{{ data()?.groups | num }}</span>
    </a>
    <div class="card stat">
      <span class="label">Under-replicated</span>
      <span class="value" [class.warn]="(data()?.underReplicated ?? 0) > 0" [class.ok]="data()?.underReplicated === 0">{{ data()?.underReplicated | num }}</span>
    </div>
    <div class="card stat">
      <span class="label">Offline partitions</span>
      <span class="value" [class.err]="(data()?.offlinePartitions ?? 0) > 0" [class.ok]="data()?.offlinePartitions === 0">{{ data()?.offlinePartitions | num }}</span>
    </div>
  </div>

  <section class="card" #brokersSec>
    <div class="card-head"><h2>Brokers</h2><span class="dim">Click a broker to inspect its configuration</span></div>
    <div class="table-wrap">
      <table class="t">
        <thead><tr><th>ID</th><th>Host</th><th>Rack</th><th>Role</th><th class="num">Partition leaders</th><th>Leader share</th></tr></thead>
        <tbody>
          @for (b of data()?.brokers ?? []; track b.id) {
            <tr class="clickable" (click)="openBroker(b)" tabindex="0" (keydown.enter)="openBroker(b)">
              <td class="mono">{{ b.id }}</td>
              <td class="mono">{{ b.host }}:{{ b.port }}</td>
              <td class="dim">{{ b.rack ?? '—' }}</td>
              <td>@if (b.controller) { <span class="badge accent">Controller</span> } @else { <span class="dim">Broker</span> }</td>
              <td class="num">{{ b.leaders | num }}</td>
              <td><div class="bar"><i [style.width.%]="share(b)"></i></div></td>
            </tr>
          } @empty {
            @if (res.isLoading()) {
              @for (i of [1, 2, 3]; track i) { <tr><td colspan="6"><div class="skeleton"></div></td></tr> }
            }
          }
        </tbody>
      </table>
    </div>
  </section>
</div>

<dialog #dlg class="dlg wide" (click)="$event.target === dlg && dlg.close()">
  <div class="dlg-head">
    <div><h2>Broker {{ broker()?.id }}</h2><p class="mono">{{ broker()?.host }}:{{ broker()?.port }}</p></div>
    <button class="btn ghost icon sm" (click)="dlg.close()" aria-label="Close"><app-icon name="x" /></button>
  </div>
  <div class="cfg-body">
    @if (brokerCfg(); as cfg) { <app-config-table [configs]="cfg" /> } @else { <div class="empty"><span class="spin"></span></div> }
  </div>
</dialog>
`,
  styles: `
a.stat, button.stat { color: inherit; transition: border-color .15s; }
button.stat { font: inherit; text-align: left; cursor: pointer; }
a.stat:hover, button.stat:hover { text-decoration: none; border-color: var(--border-strong); }
.bar { max-width: 160px; }
.cfg-body { margin-top: 14px; border-top: 1px solid var(--border); max-height: 65vh; overflow: auto; }
`,
})
export class OverviewPage {
  readonly id = input.required<string>();
  private readonly api = inject(Api);
  private readonly toasts = inject(Toasts);
  private readonly dlg = viewChild.required<ElementRef<HTMLDialogElement>>('dlg');

  protected readonly res = resource({ params: () => this.id(), loader: ({ params }) => this.api.get<Overview>(`/c/${params}/overview`) });
  protected readonly data = computed(() => (this.res.hasValue() ? this.res.value() : null));
  protected readonly broker = signal<Broker | null>(null);
  protected readonly brokerCfg = signal<ConfigEntry[] | null>(null);

  share(b: Broker) {
    const total = this.data()?.partitions || 1;
    return Math.round((b.leaders / total) * 100);
  }

  async openBroker(b: Broker) {
    this.broker.set(b);
    this.brokerCfg.set(null);
    const dlg = this.dlg().nativeElement;
    dlg.showModal();
    await this.toasts.guard(async () => {
      const cfg = await this.api.get<ConfigEntry[]>(`/c/${this.id()}/brokers/${b.id}/configs`);
      if (this.broker() === b) this.brokerCfg.set(cfg);
    });
    if (this.broker() === b && !this.brokerCfg()) dlg.close(); // failed: the toast explains, no endless spinner
  }
}
