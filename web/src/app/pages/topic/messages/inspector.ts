import { Component, computed, inject, input, output, signal } from '@angular/core';
import { KRecord, Toasts } from '../../../api';
import { BytesPipe, highlightJson, prettyJson, TimePipe } from '../../../fmt';
import { Icon } from '../../../icon';

/** Detail pane for the selected record. Used as an attribute on the layout's <aside>. */
@Component({
  selector: '[app-inspector]',
  imports: [TimePipe, BytesPipe, Icon],
  templateUrl: './inspector.html',
  styleUrl: './inspector.css',
})
export class Inspector {
  readonly record = input<KRecord | null>(null);
  readonly readOnly = input(false);
  readonly resend = output<KRecord>();
  private readonly toasts = inject(Toasts);

  protected readonly raw = signal(false);
  protected readonly selValue = computed(() => this.render(this.record()?.value ?? null));
  protected readonly selKey = computed(() => this.render(this.record()?.key ?? null));

  private render(s: string | null) {
    if (s == null) return { kind: 'null' as const, html: '', text: '' };
    const pretty = this.raw() ? null : prettyJson(s);
    return pretty ? { kind: 'json' as const, html: highlightJson(pretty), text: pretty } : { kind: 'text' as const, html: '', text: s };
  }

  async copy(text: string, what: string) {
    await navigator.clipboard.writeText(text);
    this.toasts.ok(`${what} copied`);
  }
}
