import { ChangeDetectorRef, Component, ElementRef, inject, input, output, signal, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Api, Toasts } from '../api';
import { Icon } from '../icon';

@Component({
  selector: 'app-create-topic-dialog',
  imports: [FormsModule, Icon],
  template: `
<dialog #dlg class="dlg">
  <form (ngSubmit)="create()" #f="ngForm">
    <div class="dlg-head">
      <div><h2>Create topic</h2><p>Leave partitions / replicas empty to use broker defaults.</p></div>
      <button type="button" class="btn ghost icon sm" (click)="dlg.close()" aria-label="Close"><app-icon name="x" /></button>
    </div>
    <div class="dlg-body form-grid">
      <div class="field full">
        <span class="row">Configuration
          <span class="spacer"></span>
          <button type="button" class="btn ghost sm" (click)="file.click()"><app-icon name="download" /> Import JSON</button>
          <input #file type="file" accept=".json,application/json" hidden (change)="importConfig($event)" />
        </span>
        @if (configKeys().length) {
          <span class="hint row">{{ configKeys().length }} imported: <span class="mono">{{ configKeys().join(', ') }}</span>
            <button type="button" class="btn ghost sm" (click)="nt.configs = {}">Clear</button></span>
        } @else {
          <span class="hint">Optional: prefill from a topic's exported config</span>
        }
      </div>
      <label class="field full"><span>Name</span>
        <input class="input mono" name="name" required pattern="[a-zA-Z0-9._-]+" [(ngModel)]="nt.name" autofocus placeholder="orders.v1" />
        <span class="hint">Letters, digits, dot, underscore, dash</span>
      </label>
      <label class="field"><span>Partitions</span><input class="input" type="number" min="1" name="p" [(ngModel)]="nt.partitions" /></label>
      <label class="field"><span>Replication factor</span><input class="input" type="number" min="1" name="rf" [(ngModel)]="nt.replicationFactor" /></label>
      <label class="field"><span>Cleanup policy</span>
        <select class="input" name="cp" [(ngModel)]="nt.cleanup">
          <option value="">{{ nt.configs['cleanup.policy'] ? 'Imported (' + nt.configs['cleanup.policy'] + ')' : 'Default' }}</option><option value="delete">delete</option><option value="compact">compact</option><option value="compact,delete">compact,delete</option>
        </select>
      </label>
      <label class="field"><span>Retention</span>
        <select class="input" name="ret" [(ngModel)]="nt.retention">
          <option value="">{{ nt.configs['retention.ms'] ? 'Imported (' + nt.configs['retention.ms'] + ' ms)' : 'Default' }}</option><option value="3600000">1 hour</option><option value="86400000">1 day</option>
          <option value="604800000">7 days</option><option value="2592000000">30 days</option><option value="-1">Forever</option>
        </select>
      </label>
    </div>
    <div class="dlg-foot">
      <button type="button" class="btn ghost" (click)="dlg.close()">Cancel</button>
      <button class="btn primary" [disabled]="!f.valid || busy()">@if (busy()) { <span class="spin"></span> } Create</button>
    </div>
  </form>
</dialog>
`,
})
export class CreateTopicDialog {
  readonly id = input.required<string>();
  readonly created = output<void>();
  private readonly api = inject(Api);
  private readonly toasts = inject(Toasts);
  private readonly cdr = inject(ChangeDetectorRef);
  private readonly dlg = viewChild.required<ElementRef<HTMLDialogElement>>('dlg');

  protected readonly busy = signal(false);
  protected nt = this.emptyNt();
  protected configKeys = () => Object.keys(this.nt.configs);

  private emptyNt() {
    return { name: '', partitions: null as number | null, replicationFactor: null as number | null, cleanup: '', retention: '', configs: {} as Record<string, string> };
  }

  open() {
    this.nt = this.emptyNt();
    this.cdr.markForCheck(); // OnPush: open() is called from the parent, nothing else re-renders the dialog
    this.dlg().nativeElement.showModal();
  }

  /** Prefills the dialog from a file written by the topic page's Export config. Name is left as typed. */
  async importConfig(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = ''; // allow re-importing the same file
    if (!file) return;
    try {
      const j = JSON.parse(await file.text());
      if (!j || typeof j !== 'object' || (j.configs != null && typeof j.configs !== 'object')) throw new Error('not a topic export');
      const configs: Record<string, string> = {};
      for (const [k, v] of Object.entries(j.configs ?? {})) if (v != null) configs[k] = String(v);
      const n = (x: unknown) => (Number.isInteger(x) && (x as number) > 0 ? (x as number) : null);
      this.nt = { ...this.nt, partitions: n(j.partitions), replicationFactor: n(j.replicationFactor), cleanup: '', retention: '', configs };
      this.cdr.markForCheck(); // zoneless: nt is a plain field set after an await, nothing else re-renders the dialog
      this.toasts.ok(`Imported ${Object.keys(configs).length} configs from ${file.name}`);
    } catch (err) {
      this.toasts.err(`${file.name}: ${err instanceof Error ? err.message : err}`);
    }
  }

  create() {
    const configs: Record<string, string> = { ...this.nt.configs };
    if (this.nt.cleanup) configs['cleanup.policy'] = this.nt.cleanup;
    if (this.nt.retention) configs['retention.ms'] = this.nt.retention;
    return this.toasts.guard(async () => {
      await this.api.post(`/c/${this.id()}/topics`, {
        name: this.nt.name, partitions: this.nt.partitions ?? 0, replicationFactor: this.nt.replicationFactor ?? 0, configs,
      });
      this.dlg().nativeElement.close();
      this.toasts.ok(`Created ${this.nt.name}`);
      this.created.emit();
    }, this.busy);
  }
}
