import { Component, computed, ElementRef, forwardRef, input, signal, viewChild } from '@angular/core';
import { ControlValueAccessor, NG_VALUE_ACCESSOR } from '@angular/forms';
import { Icon } from './icon';

let seq = 0;
const pad = (n: number) => String(n).padStart(2, '0');
const sameDay = (a: Date, b: Date) => a.toDateString() === b.toDateString();
/** Local "YYYY-MM-DDTHH:mm:ss", the format datetime-local reads and writes. */
const fmt = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
// 2024-01-01 is a Monday: weeks start on Monday
const WEEKDAYS = Array.from({ length: 7 }, (_, i) => new Date(2024, 0, 1 + i).toLocaleDateString(undefined, { weekday: 'narrow' }));

/**
 * Native datetime-local field (typing, segments, locale order) with a themed calendar popover
 * in place of the browser's picker, whose colors can't be styled. ngModel value: "YYYY-MM-DDTHH:mm:ss" or "".
 */
@Component({
  selector: 'app-datetime',
  imports: [Icon],
  providers: [{ provide: NG_VALUE_ACCESSOR, useExisting: forwardRef(() => DateTime), multi: true }],
  template: `
<input #field class="input" type="datetime-local" step="1" [value]="value()" [disabled]="disabled()" [attr.aria-label]="label() || null"
  (input)="set(field.value)" (blur)="touched()" />
<button type="button" class="pick" [attr.popovertarget]="id" [disabled]="disabled()" aria-label="Open calendar"><app-icon name="calendar" /></button>
<div #panel class="cal" [id]="id" popover role="dialog" aria-label="Choose a date" (beforetoggle)="onToggle($event)">
  <div class="head">
    <button type="button" class="btn ghost icon sm" (click)="shift(-1)" aria-label="Previous month"><app-icon name="chevron-right" class="flip" /></button>
    <strong>{{ title() }}</strong>
    <button type="button" class="btn ghost icon sm" (click)="shift(1)" aria-label="Next month"><app-icon name="chevron-right" /></button>
  </div>
  <div class="grid">
    @for (w of weekdays; track $index) { <span class="wd">{{ w }}</span> }
    @for (d of days(); track d.key) {
      <button type="button" class="day" [class.out]="d.out" [class.today]="d.today" [class.on]="d.on" [attr.aria-pressed]="d.on"
        [attr.aria-label]="d.label" (click)="pick(d.date)">{{ d.date.getDate() }}</button>
    }
  </div>
  <div class="foot">
    <button type="button" class="btn ghost sm" (click)="clear()">Clear</button>
    <button type="button" class="btn ghost sm" (click)="now()">Now</button>
  </div>
</div>`,
  styles: `
:host { position: relative; display: block; }
.input { padding-right: 34px; }
.input::-webkit-calendar-picker-indicator { display: none; }
.pick {
  position: absolute; right: 4px; top: 50%; translate: 0 -50%; display: grid; place-items: center; width: 26px; height: 26px;
  border: 0; border-radius: var(--radius-sm); background: none; color: var(--text-3); cursor: pointer; --size: 15px;
}
.pick:hover:not(:disabled) { color: var(--accent); background: var(--hover); }
.pick:disabled { cursor: default; opacity: 0.5; }
.cal {
  inset: auto; margin: 0; width: 252px; padding: 10px; color: var(--text);
  background: var(--surface); border: 1px solid var(--border-strong); border-radius: var(--radius); box-shadow: var(--shadow);
}
.head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; font-size: 13px; text-transform: capitalize; }
.flip { rotate: 180deg; }
.grid { display: grid; grid-template-columns: repeat(7, 1fr); gap: 2px; }
.wd { text-align: center; font-size: 11px; color: var(--text-3); padding: 4px 0; text-transform: uppercase; }
.day { height: 30px; border: 0; border-radius: var(--radius-sm); background: none; color: var(--text); font: 12.5px var(--font); cursor: pointer; }
.day:hover { background: var(--hover); }
.day.out { color: var(--text-3); opacity: 0.6; }
.day.today { box-shadow: inset 0 0 0 1px var(--accent); }
.day.on { background: var(--accent); color: var(--accent-ink); font-weight: 600; }
.foot { display: flex; justify-content: space-between; margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--border); }
`,
})
export class DateTime implements ControlValueAccessor {
  readonly label = input('');
  protected readonly id = `dt-${++seq}`;
  protected readonly weekdays = WEEKDAYS;
  protected readonly value = signal('');
  protected readonly disabled = signal(false);
  private readonly view = signal(new Date()); // any day of the month on screen
  private readonly panel = viewChild.required<ElementRef<HTMLElement>>('panel');
  private readonly host = viewChild.required<ElementRef<HTMLInputElement>>('field');

  private onChange = (_: string) => {};
  protected touched = () => {};

  private readonly selected = computed(() => (this.value() ? new Date(this.value()) : null));
  protected readonly title = computed(() => this.view().toLocaleDateString(undefined, { month: 'long', year: 'numeric' }));
  protected readonly days = computed(() => {
    const v = this.view(), sel = this.selected(), today = new Date();
    const first = new Date(v.getFullYear(), v.getMonth(), 1);
    const start = new Date(first.getFullYear(), first.getMonth(), 1 - ((first.getDay() + 6) % 7));
    return Array.from({ length: 42 }, (_, i) => {
      const date = new Date(start.getFullYear(), start.getMonth(), start.getDate() + i);
      return {
        key: date.toDateString(), date, out: date.getMonth() !== v.getMonth(), today: sameDay(date, today),
        on: !!sel && sameDay(date, sel), label: date.toLocaleDateString(undefined, { dateStyle: 'full' }),
      };
    });
  });

  writeValue(v: string | null) { this.value.set(v ?? ''); }
  registerOnChange(fn: (v: string) => void) { this.onChange = fn; }
  registerOnTouched(fn: () => void) { this.touched = fn; }
  setDisabledState(d: boolean) { this.disabled.set(d); }

  protected set(v: string) { this.value.set(v); this.onChange(v); }

  protected shift(months: number) {
    const v = this.view();
    this.view.set(new Date(v.getFullYear(), v.getMonth() + months, 1));
  }

  /** Picks a day, keeping the time already typed (midnight if none). */
  protected pick(day: Date) {
    const t = this.selected();
    const d = new Date(day);
    if (t && !isNaN(+t)) d.setHours(t.getHours(), t.getMinutes(), t.getSeconds(), 0);
    this.close(fmt(d));
  }
  protected now() { this.close(fmt(new Date())); }
  protected clear() { this.close(''); }

  private close(v: string) {
    this.set(v);
    this.panel().nativeElement.hidePopover();
    this.host().nativeElement.focus();
  }

  /** On open: show the selected month and place the panel under the field (above it when there's no room). */
  protected onToggle(e: Event) {
    if ((e as ToggleEvent).newState !== 'open') return;
    const sel = this.selected();
    this.view.set(sel && !isNaN(+sel) ? sel : new Date());
    const r = this.host().nativeElement.getBoundingClientRect(), s = this.panel().nativeElement.style;
    s.left = `${Math.max(8, Math.min(r.right - 252, innerWidth - 260))}px`;
    const below = innerHeight - r.bottom > 330;
    s.top = below ? `${r.bottom + 4}px` : 'auto';
    s.bottom = below ? 'auto' : `${innerHeight - r.top + 4}px`;
  }
}
