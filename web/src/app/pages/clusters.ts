import { Component, ElementRef, inject, signal, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { Api, Cluster, Toasts } from '../api';
import { Icon } from '../icon';

const COLORS = ['#2dd4bf', '#60a5fa', '#a78bfa', '#f472b6', '#fb923c', '#facc15', '#4ade80', '#f87171'];

const blank = (): Cluster => ({
  id: '', name: '', color: COLORS[0], brokers: [], saslMechanism: '', saslUser: '', saslPassword: '',
  tls: false, tlsSkipVerify: false, tlsCa: '', tlsCert: '', tlsKey: '', readOnly: false,
});

@Component({
  selector: 'app-clusters',
  imports: [FormsModule, RouterLink, Icon],
  template: `
<div class="page">
  <header class="page-head">
    <div>
      <h1>Clusters</h1>
      <p class="sub">Connections are stored locally on this machine.</p>
    </div>
    <button class="btn primary" (click)="edit()"><app-icon name="plus" /> Add cluster</button>
  </header>

  @if (api.clusters().length === 0) {
    <div class="card empty">
      <app-icon name="plug" />
      <strong>No clusters yet</strong>
      <span>Add a connection to start browsing topics, messages and consumer groups.</span>
      <button class="btn primary" (click)="edit()"><app-icon name="plus" /> Add your first cluster</button>
    </div>
  } @else {
    <div class="cards">
      @for (c of api.clusters(); track c.id) {
        <article class="card ccard" [style.--c]="c.color || 'var(--accent)'">
          <a class="ccard-main" [routerLink]="['/c', c.id]">
            <div class="row">
              <span class="cdot"></span>
              <h2>{{ c.name }}</h2>
              @if (c.readOnly) { <span class="badge warn"><app-icon name="lock" /> Read-only</span> }
            </div>
            <div class="mono dim brokers">{{ c.brokers.join(', ') }}</div>
            <div class="row wrap">
              <span class="badge">{{ c.tls ? 'TLS' : 'Plaintext' }}</span>
              @if (c.saslMechanism) { <span class="badge">SASL {{ c.saslMechanism }}</span> }
              <span class="spacer"></span><span class="open">Open →</span>
            </div>
          </a>
          <div class="ccard-actions">
            <button class="btn ghost icon sm" (click)="edit(c)" [attr.aria-label]="'Edit ' + c.name"><app-icon name="edit" /></button>
            <button class="btn ghost icon sm" (click)="remove(c)" [attr.aria-label]="'Delete ' + c.name"><app-icon name="trash" /></button>
          </div>
        </article>
      }
    </div>
  }
</div>

<dialog #dlg class="dlg" (close)="testResult.set(null)">
  <form (ngSubmit)="save()" #f="ngForm">
    <div class="dlg-head">
      <div>
        <h2>{{ form().id ? 'Edit cluster' : 'Add cluster' }}</h2>
        <p>Bootstrap servers, security and safety options.</p>
      </div>
      <button type="button" class="btn ghost icon sm" (click)="dlg.close()" aria-label="Close"><app-icon name="x" /></button>
    </div>
    <div class="dlg-body">
      <div class="form-grid">
        <label class="field full"><span>Name</span>
          <input class="input" name="name" required [(ngModel)]="form().name" placeholder="e.g. Production EU" autofocus />
        </label>
        <label class="field full"><span>Bootstrap servers</span>
          <input class="input mono" name="brokers" required [(ngModel)]="brokers" placeholder="broker1:9092, broker2:9092" />
          <span class="hint">Comma-separated host:port list</span>
        </label>
        <div class="field full"><span>Color</span>
          <div class="row swatches" role="radiogroup" aria-label="Cluster color">
            @for (col of colors; track col) {
              <button type="button" class="swatch" role="radio" [attr.aria-checked]="form().color === col"
                [attr.aria-label]="col" [style.background]="col" [class.on]="form().color === col" (click)="form().color = col"></button>
            }
          </div>
        </div>
        <label class="field"><span>SASL mechanism</span>
          <select class="input" name="sasl" [(ngModel)]="form().saslMechanism">
            <option value="">None</option>
            <option>PLAIN</option>
            <option>SCRAM-SHA-256</option>
            <option>SCRAM-SHA-512</option>
          </select>
        </label>
        <div class="field"><span>Transport</span>
          <label class="check"><input type="checkbox" name="tls" [(ngModel)]="form().tls" /> Use TLS</label>
          @if (form().tls) {
            <label class="check"><input type="checkbox" name="skip" [(ngModel)]="form().tlsSkipVerify" /> Skip certificate verification</label>
          }
        </div>
        @if (form().tls) {
          <label class="field full"><span>CA certificate (PEM)</span>
            <textarea class="input mono" name="ca" rows="3" [(ngModel)]="form().tlsCa" placeholder="-----BEGIN CERTIFICATE----- (empty = system CAs)"></textarea>
          </label>
          <label class="field"><span>Client certificate (PEM, mTLS)</span>
            <textarea class="input mono" name="cert" rows="3" [(ngModel)]="form().tlsCert"></textarea>
          </label>
          <label class="field"><span>Client key (PEM, mTLS)</span>
            <textarea class="input mono" name="key" rows="3" [(ngModel)]="form().tlsKey" autocomplete="off"
              [placeholder]="form().hasTlsKey ? '•••••••• (unchanged)' : ''"></textarea>
          </label>
        }
        @if (form().saslMechanism) {
          <label class="field"><span>Username</span>
            <input class="input" name="user" [(ngModel)]="form().saslUser" autocomplete="off" />
          </label>
          <label class="field"><span>Password</span>
            <input class="input" type="password" name="pass" [(ngModel)]="form().saslPassword" autocomplete="new-password"
              [placeholder]="form().hasPassword ? '•••••••• (unchanged)' : ''" />
          </label>
        }
        <label class="check full ro">
          <input type="checkbox" name="ro" [(ngModel)]="form().readOnly" />
          <span><b>Read-only</b> — block create, delete, produce and offset resets. A guard against mistakes, not access control: restrict the SASL user with broker ACLs.</span>
        </label>
      </div>
      @if (form().saslMechanism === 'PLAIN' && !form().tls) {
        <div class="alert warn"><app-icon name="alert" /> SASL PLAIN without TLS sends the password in clear text.</div>
      }
      @if (form().tls && form().tlsSkipVerify) {
        <div class="alert warn"><app-icon name="alert" /> Certificate verification is off: anyone on the network path can impersonate the broker. Prefer adding the CA certificate.</div>
      }
      @if (testResult(); as r) {
        <div class="alert" [class.ok]="r.ok" [class.err]="!r.ok"><app-icon [name]="r.ok ? 'check' : 'alert'" /> {{ r.text }}</div>
      }
    </div>
    <div class="dlg-foot">
      <button type="button" class="btn" (click)="test()" [disabled]="testing() || !f.valid">
        @if (testing()) { <span class="spin"></span> } @else { <app-icon name="zap" /> }
        Test connection
      </button>
      <span class="spacer"></span>
      <button type="button" class="btn ghost" (click)="dlg.close()">Cancel</button>
      <button class="btn primary" [disabled]="!f.valid">Save</button>
    </div>
  </form>
</dialog>
`,
  styles: `
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 14px; }
.ccard { position: relative; border-top: 3px solid var(--c); transition: border-color .15s, transform .15s var(--ease), box-shadow .15s; }
.ccard:hover { border-color: var(--border-strong); border-top-color: var(--c); transform: translateY(-1px); box-shadow: var(--shadow); }
.ccard-main { display: grid; gap: 10px; padding: 18px 20px; color: inherit; }
.ccard-main:hover { text-decoration: none; }
.cdot { width: 10px; height: 10px; border-radius: 3px; background: var(--c); box-shadow: 0 0 0 4px color-mix(in srgb, var(--c) 20%, transparent); }
.brokers { font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ccard-actions { position: absolute; top: 12px; right: 10px; display: flex; gap: 2px; }
.ccard-actions .btn:last-child:hover { color: var(--err); }
.open { font-size: 12.5px; color: var(--text-3); transition: color .15s; }
.ccard:hover .open { color: var(--accent); }
.swatch { width: 26px; height: 26px; border-radius: 50%; border: 2px solid transparent; cursor: pointer; outline-offset: 2px; }
.swatch.on { box-shadow: 0 0 0 2px var(--surface), 0 0 0 4px currentColor; color: var(--text); }
.ro { align-items: flex-start; padding: 12px; border-radius: 8px; background: var(--surface-2); line-height: 1.45; }
.ro input { margin-top: 2px; }
.ro b { color: var(--text); }
`,
})
export class ClustersPage {
  protected readonly api = inject(Api);
  private readonly toasts = inject(Toasts);
  private readonly router = inject(Router);
  private readonly dlg = viewChild.required<ElementRef<HTMLDialogElement>>('dlg');

  protected readonly colors = COLORS;
  protected readonly form = signal<Cluster>(blank());
  protected brokers = '';
  protected readonly testing = signal(false);
  protected readonly testResult = signal<{ ok: boolean; text: string } | null>(null);

  edit(c?: Cluster) {
    this.form.set(c ? { ...blank(), ...c, saslPassword: '', tlsKey: '' } : blank());
    this.brokers = c?.brokers.join(', ') ?? '';
    this.dlg().nativeElement.showModal();
  }

  private payload(): Cluster {
    return { ...this.form(), brokers: this.brokers.split(/[,\s]+/).filter(Boolean) };
  }

  async test() {
    this.testing.set(true);
    this.testResult.set(null);
    try {
      const r = await this.api.post<{ clusterId: string; brokers: number; latencyMs: number }>('/clusters/test', this.payload());
      this.testResult.set({ ok: true, text: `Connected to ${r.clusterId} — ${r.brokers} broker(s), ${r.latencyMs} ms` });
    } catch (e) {
      this.testResult.set({ ok: false, text: e instanceof Error ? e.message : String(e) });
    } finally {
      this.testing.set(false);
    }
  }

  async save() {
    const p = this.payload();
    try {
      const saved = p.id ? await this.api.put<Cluster>(`/clusters/${p.id}`, p) : await this.api.post<Cluster>('/clusters', p);
      await this.api.refreshClusters();
      this.dlg().nativeElement.close();
      this.toasts.ok(`Saved ${saved.name}`);
      if (!p.id) this.router.navigate(['/c', saved.id]);
    } catch (e) {
      this.toasts.err(e);
    }
  }

  async remove(c: Cluster) {
    if (!confirm(`Remove the connection "${c.name}"? The cluster itself is not touched.`)) return;
    try {
      await this.api.del(`/clusters/${c.id}`);
      await this.api.refreshClusters();
      this.toasts.ok(`Removed ${c.name}`);
    } catch (e) {
      this.toasts.err(e);
    }
  }
}
