import { Component, computed, input, output, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ConfigEntry } from '../api';
import { Icon } from '../icon';

/** Searchable config list; emits edits when `editable` is set. */
@Component({
  selector: 'app-config-table',
  imports: [FormsModule, Icon],
  template: `
<div class="cfg-tools row">
  <div class="search grow">
    <app-icon name="search" />
    <input class="input" placeholder="Filter configs…" [ngModel]="q()" (ngModelChange)="q.set($event)" aria-label="Filter configs" />
  </div>
  <label class="check"><input type="checkbox" [ngModel]="onlyOverrides()" (ngModelChange)="onlyOverrides.set($event)" /> Overrides only</label>
  <span class="spacer"></span>
  <ng-content />
</div>
<div class="table-wrap">
  <table class="t">
    <thead><tr><th>Key</th><th>Value</th><th>Source</th>@if (editable()) {<th></th>}</tr></thead>
    <tbody>
      @for (c of rows(); track c.key) {
        <tr>
          <td class="mono key">{{ c.key }}</td>
          <td class="mono val">
            @if (editing() === c.key) {
              <form class="row" (ngSubmit)="commit(c)">
                <input class="input mono" name="v" [(ngModel)]="draft" autofocus (keydown.escape)="editing.set(null)" />
                <button class="btn primary sm">Save</button>
                <button type="button" class="btn ghost sm" (click)="editing.set(null)">Cancel</button>
              </form>
            } @else if (c.sensitive) {
              <span class="dim">•••••• (sensitive)</span>
            } @else {
              <span [class.dim]="c.isDefault">{{ c.value ?? '—' }}</span>
            }
          </td>
          <td><span class="badge" [class.accent]="!c.isDefault">{{ label(c.source) }}</span></td>
          @if (editable()) {
            <td class="acts">
              @if (editing() !== c.key) {
                <button class="btn ghost icon sm" (click)="start(c)" [attr.aria-label]="'Edit ' + c.key"><app-icon name="edit" /></button>
                @if (!c.isDefault && c.source === 'DYNAMIC_TOPIC_CONFIG') {
                  <button class="btn ghost sm" (click)="changed.emit({ key: c.key, value: null })" title="Reset to default">Reset</button>
                }
              }
            </td>
          }
        </tr>
      } @empty {
        <tr><td colspan="4" class="dim center">No configs match</td></tr>
      }
    </tbody>
  </table>
</div>
`,
  styles: `
.cfg-tools { padding: 12px 14px; border-bottom: 1px solid var(--border); gap: 14px; }
.grow { flex: 1; max-width: 360px; }
.key { white-space: nowrap; }
.val { word-break: break-all; width: 50%; }
.val form .input { height: 28px; }
.acts { white-space: nowrap; text-align: right; width: 1%; }
.center { text-align: center; padding: 24px; }
`,
})
export class ConfigTable {
  readonly configs = input.required<ConfigEntry[]>();
  readonly editable = input(false);
  readonly changed = output<{ key: string; value: string | null }>();

  protected readonly q = signal('');
  protected readonly onlyOverrides = signal(false);
  protected readonly editing = signal<string | null>(null);
  protected draft = '';

  protected readonly rows = computed(() => {
    const q = this.q().toLowerCase();
    return this.configs().filter(
      (c) => (!this.onlyOverrides() || !c.isDefault) && (!q || c.key.includes(q) || (c.value ?? '').toLowerCase().includes(q)),
    );
  });

  label(src: string) {
    return src.replace(/_CONFIG$/, '').replace(/_/g, ' ').toLowerCase();
  }

  start(c: ConfigEntry) {
    this.draft = c.value ?? '';
    this.editing.set(c.key);
  }

  commit(c: ConfigEntry) {
    this.editing.set(null);
    if (this.draft !== c.value) this.changed.emit({ key: c.key, value: this.draft });
  }
}
