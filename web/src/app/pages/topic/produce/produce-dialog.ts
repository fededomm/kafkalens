import { afterNextRender, Component, ElementRef, inject, Injector, input, output, signal, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Api, Header, KRecord, Partition, Toasts } from '../../../api';
import { Icon } from '../../../icon';
import { tplHtml, tplStub } from '../../../tpl';

@Component({
  selector: 'app-produce-dialog',
  imports: [FormsModule, Icon],
  templateUrl: './produce-dialog.html',
  styleUrl: './produce-dialog.css',
})
export class ProduceDialog {
  readonly topic = input.required<string>();
  readonly base = input.required<string>();
  readonly partitions = input<Partition[]>([]);
  readonly produced = output<void>();

  private readonly api = inject(Api);
  private readonly toasts = inject(Toasts);
  private readonly injector = inject(Injector);
  private readonly dlg = viewChild.required<ElementRef<HTMLDialogElement>>('dlg');
  private readonly keyInput = viewChild<ElementRef<HTMLInputElement>>('keyInput');
  private readonly valInput = viewChild<ElementRef<HTMLTextAreaElement>>('valInput');

  protected pr = { key: '', value: '', partition: -1, count: 1, headers: [] as Header[], keyNull: false, template: false };
  protected readonly producing = signal(false);
  protected readonly valueJsonState = signal<'json' | 'text' | 'invalid'>('text');
  protected readonly tplHtml = tplHtml;

  /** Opens the dialog empty, or prefilled with a copy of `from`. */
  open(from?: KRecord) {
    this.pr = {
      key: from?.key ?? '', value: from?.value ?? '', partition: -1, count: 1,
      headers: from ? from.headers.map((h) => ({ ...h })) : [], keyNull: from ? from.key == null : false,
      template: this.pr.template,
    };
    this.checkJson();
    this.dlg().nativeElement.showModal();
    afterNextRender(() => (from ? this.valInput() : this.keyInput() ?? this.valInput())?.nativeElement.focus(), { injector: this.injector });
  }

  /** Keeps the highlight layer (previous sibling) scrolled like its input; rAF so it runs after the mirror re-renders. */
  syncHl(e: Event) {
    const el = e.target as HTMLElement;
    requestAnimationFrame(() => {
      const hl = el.previousElementSibling;
      if (hl?.classList.contains('tpl-hl')) { hl.scrollTop = el.scrollTop; hl.scrollLeft = el.scrollLeft; }
    });
  }

  checkJson() {
    const v = (this.pr.template ? tplStub(this.pr.value) : this.pr.value).trim();
    if (!v || (v[0] !== '{' && v[0] !== '[')) return this.valueJsonState.set('text');
    try { JSON.parse(v); this.valueJsonState.set('json'); } catch { this.valueJsonState.set('invalid'); }
  }

  formatJson() {
    try { this.pr.value = JSON.stringify(JSON.parse(this.pr.value), null, 2); this.checkJson(); } catch { this.toasts.err('Value is not valid JSON'); }
  }

  async produce() {
    this.producing.set(true);
    try {
      const res = await this.api.post<{ count: number; partition: number; offset: number }>(`${this.base()}/produce`, {
        key: this.pr.keyNull ? null : this.pr.key, value: this.pr.value, partition: this.pr.partition,
        count: this.pr.count, headers: this.pr.headers.filter((h) => h.key), template: this.pr.template,
      });
      this.toasts.ok(res.count > 1 ? `Produced ${res.count} messages` : `Produced to partition ${res.partition} @ offset ${res.offset}`);
      this.dlg().nativeElement.close();
      this.produced.emit();
    } catch (e) {
      this.toasts.err(e);
    } finally {
      this.producing.set(false);
    }
  }
}
