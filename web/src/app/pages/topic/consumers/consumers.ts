import { Component, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { TopicDetail } from '../../../api';
import { NumPipe } from '../../../fmt';
import { Icon } from '../../../icon';

@Component({
  selector: 'app-topic-consumers',
  imports: [RouterLink, NumPipe, Icon],
  template: `
<section class="card">
  <div class="table-wrap">
    <table class="t">
      <thead><tr><th>Consumer group</th><th>State</th><th class="num">Lag on this topic</th></tr></thead>
      <tbody>
        @for (g of groups(); track g.group) {
          <tr class="clickable" [routerLink]="['/c', id(), 'groups', g.group]">
            <td class="name"><a [routerLink]="['/c', id(), 'groups', g.group]">{{ g.group }}</a></td>
            <td><span class="badge" [class.ok]="g.state === 'Stable'" [class.warn]="g.state.includes('Rebalanc')">{{ g.state }}</span></td>
            <td class="num" [class.dim]="g.lag === 0" [class.lag-on]="g.lag > 0" [class.lag-hot]="g.lag > 1000">{{ g.lag | num }}</td>
          </tr>
        } @empty {
          <tr><td colspan="3"><div class="empty"><app-icon name="groups" /><span>No consumer group has committed offsets on this topic.</span></div></td></tr>
        }
      </tbody>
    </table>
  </div>
</section>
`,
})
export class TopicConsumers {
  readonly id = input.required<string>();
  readonly groups = input<TopicDetail['groups']>([]);
}
