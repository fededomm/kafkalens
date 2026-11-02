import { Component, computed, inject, input, resource, signal, viewChild } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { Api, Confirm, Favorites, Toasts, TopicDetail } from '../../api';
import { download, NumPipe } from '../../fmt';
import { Icon } from '../../icon';
import { ConfigTable } from '../config-table';
import { TopicConsumers } from './consumers/consumers';
import { TopicMessages } from './messages/messages';
import { TopicPartitions } from './partitions/partitions';
import { ProduceDialog } from './produce/produce-dialog';

type Tab = 'messages' | 'partitions' | 'configs' | 'consumers';

@Component({
  selector: 'app-topic',
  imports: [RouterLink, NumPipe, Icon, ConfigTable, TopicMessages, TopicPartitions, TopicConsumers, ProduceDialog],
  templateUrl: './topic.html',
  styleUrl: './topic.css',
})
export class TopicPage {
  readonly id = input.required<string>();
  readonly topic = input.required<string>();
  private readonly api = inject(Api);
  protected readonly fav = inject(Favorites);
  private readonly toasts = inject(Toasts);
  private readonly confirm = inject(Confirm);
  private readonly router = inject(Router);
  protected readonly messages = viewChild(TopicMessages);

  protected readonly tab = signal<Tab>('messages');
  protected readonly busy = signal(false);
  protected readonly base = computed(() => `/c/${this.id()}/topics/${Api.seg(this.topic())}`);
  protected readonly res = resource({ params: () => this.base(), loader: ({ params }) => this.api.get<TopicDetail>(params) });
  protected readonly detail = computed(() => (this.res.hasValue() ? this.res.value() : null));
  protected readonly readOnly = computed(() => this.api.clusters().find((c) => c.id === this.id())?.readOnly ?? false);
  protected readonly totalMessages = computed(() => (this.detail()?.partitions ?? []).reduce((s, p) => s + (p.end - p.start), 0));
  protected readonly replication = computed(() => this.detail()?.partitions[0]?.replicas.length ?? 0);
  // brokers refuse DeleteRecords unless cleanup.policy includes delete
  protected readonly compactOnly = computed(() => !(this.detail()?.configs.find((c) => c.key === 'cleanup.policy')?.value ?? 'delete').includes('delete'));

  onProduced() {
    this.res.reload();
    this.messages()?.refresh();
  }

  /** Topic-level overrides plus layout, in the shape the Create topic dialog imports. */
  exportConfig() {
    const d = this.detail();
    if (!d) return;
    const configs = Object.fromEntries(d.configs.filter((c) => c.source === 'DYNAMIC_TOPIC_CONFIG' && !c.sensitive).map((c) => [c.key, c.value]));
    const json = JSON.stringify({ name: d.name, partitions: d.partitions.length, replicationFactor: this.replication(), configs }, null, 2);
    return this.toasts.guard(async () => {
      const p = await download(`${this.topic()}-export.json`, json);
      if (p) this.toasts.ok(`Saved to ${p}`);
    });
  }

  setConfig(c: { key: string; value: string | null }) {
    return this.toasts.guard(async () => {
      await this.api.put(`${this.base()}/configs`, { [c.key]: c.value });
      this.toasts.ok(c.value == null ? `${c.key} reset to default` : `${c.key} updated`);
      this.res.reload();
    });
  }

  async truncate() {
    const t = this.topic();
    const text = `This permanently deletes all ${this.totalMessages().toLocaleString()} messages of "${t}". The topic, its partitions and configuration stay.`;
    if (!(await this.confirm.ask('Delete messages', text, 'Delete messages', t))) return;
    return this.toasts.guard(async () => {
      await this.api.del(`${this.base()}/records`);
      this.toasts.ok(`Deleted the messages of ${t}`);
      this.onProduced(); // reload counts and the messages tab
    }, this.busy);
  }

  async deleteTopic() {
    const t = this.topic();
    if (!(await this.confirm.ask('Delete topic', `This permanently deletes "${t}" and all of its messages.`, 'Delete topic', t))) return;
    return this.toasts.guard(async () => {
      await this.api.del(this.base());
      this.toasts.ok(`Deleted ${t}`);
      this.router.navigate(['/c', this.id(), 'topics']);
    }, this.busy);
  }
}
