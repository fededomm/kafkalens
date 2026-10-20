import { Component, ElementRef, inject, input, output, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Api, Partition, Toasts } from '../../../api';
import { NumPipe } from '../../../fmt';
import { Icon } from '../../../icon';

@Component({
  selector: 'app-topic-partitions',
  imports: [FormsModule, NumPipe, Icon],
  template: `
<section class="card">
  <div class="card-head">
    <h2>Partitions</h2>
    <button class="btn sm" (click)="openParts()" [disabled]="readOnly()"><app-icon name="plus" /> Add partitions</button>
  </div>
  <div class="table-wrap">
    <table class="t">
      <thead><tr><th class="num">ID</th><th class="num">Leader</th><th>Replicas</th><th>In-sync</th><th class="num">Start</th><th class="num">End</th><th class="num">Messages</th><th>Distribution</th></tr></thead>
      <tbody>
        @for (p of partitions(); track p.id) {
          <tr>
            <td class="num mono">{{ p.id }}</td>
            <td class="num mono">@if (p.leader < 0) { <span class="badge err">offline</span> } @else { {{ p.leader }} }</td>
            <td class="mono">{{ p.replicas.join(', ') }}</td>
            <td>
              @if (p.isr.length < p.replicas.length) { <span class="badge warn">{{ p.isr.join(', ') || 'none' }}</span> }
              @else { <span class="mono">{{ p.isr.join(', ') }}</span> }
            </td>
            <td class="num mono">{{ p.start | num }}</td>
            <td class="num mono">{{ p.end | num }}</td>
            <td class="num">{{ p.end - p.start | num }}</td>
            <td><div class="bar"><i [style.width.%]="total() ? ((p.end - p.start) / total()) * 100 : 0"></i></div></td>
          </tr>
        }
      </tbody>
    </table>
  </div>
</section>

<dialog #dlg class="dlg">
  <form (ngSubmit)="addPartitions()">
    <div class="dlg-head"><div><h2>Add partitions</h2><p>Partitions can only be increased. Keyed messages may map to different partitions afterwards.</p></div></div>
    <div class="dlg-body">
      <label class="field"><span>New total partitions</span>
        <input class="input" type="number" name="np" [min]="(partitions().length) + 1" [(ngModel)]="newPartitions" />
      </label>
    </div>
    <div class="dlg-foot">
      <button type="button" class="btn ghost" (click)="dlg.close()">Cancel</button>
      <button class="btn primary" [disabled]="newPartitions <= (partitions().length)">Update</button>
    </div>
  </form>
</dialog>
`,
  styles: `.bar { max-width: 180px; }`,
})
export class TopicPartitions {
  readonly partitions = input<Partition[]>([]);
  readonly total = input(0);
  readonly readOnly = input(false);
  readonly base = input.required<string>();
  readonly changed = output<void>();
  private readonly api = inject(Api);
  private readonly toasts = inject(Toasts);
  private readonly dlg = viewChild.required<ElementRef<HTMLDialogElement>>('dlg');

  protected newPartitions = 0;

  openParts() {
    this.newPartitions = this.partitions().length + 1;
    this.dlg().nativeElement.showModal();
  }

  async addPartitions() {
    try {
      await this.api.post(`${this.base()}/partitions`, { total: this.newPartitions });
      this.dlg().nativeElement.close();
      this.toasts.ok(`Topic now has ${this.newPartitions} partitions`);
      this.changed.emit();
    } catch (e) {
      this.toasts.err(e);
    }
  }
}
