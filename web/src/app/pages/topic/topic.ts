import { Component, computed, effect, ElementRef, inject, input, signal, untracked, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { Api, Favorites, Toasts, TopicDetail } from '../../api';
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
  imports: [FormsModule, RouterLink, NumPipe, Icon, ConfigTable, TopicMessages, TopicPartitions, TopicConsumers, ProduceDialog],
  templateUrl: './topic.html',
  styleUrl: './topic.css',
})
export class TopicPage {
  readonly id = input.required<string>();
  readonly topic = input.required<string>();
  private readonly api = inject(Api);
  protected readonly fav = inject(Favorites);
  private readonly toasts = inject(Toasts);
  private readonly router = inject(Router);
  private readonly deleteDlg = viewChild.required<ElementRef<HTMLDialogElement>>('deleteDlg');
  protected readonly messages = viewChild(TopicMessages);

  protected readonly tab = signal<Tab>('messages');
  protected readonly detail = signal<TopicDetail | null>(null);
  protected readonly error = signal<string | null>(null);
  protected readonly readOnly = computed(() => this.api.clusters().find((c) => c.id === this.id())?.readOnly ?? false);
  protected readonly totalMessages = computed(() => (this.detail()?.partitions ?? []).reduce((s, p) => s + (p.end - p.start), 0));
  protected readonly replication = computed(() => this.detail()?.partitions[0]?.replicas.length ?? 0);
  protected readonly base = computed(() => `/c/${this.id()}/topics/${Api.seg(this.topic())}`);

  protected confirmText = '';

  constructor() {
    effect(() => {
      this.base();
      untracked(() => {
        this.detail.set(null);
        this.loadDetail();
      });
    });
  }

  async loadDetail() {
    try {
      this.detail.set(await this.api.get<TopicDetail>(this.base()));
      this.error.set(null);
    } catch (e) {
      this.error.set(e instanceof Error ? e.message : String(e));
    }
  }

  onProduced() {
    this.loadDetail();
    this.messages()?.refresh();
  }

  /** Topic-level overrides plus layout, in the shape the Create topic dialog imports. */
  exportConfig() {
    const d = this.detail();
    if (!d) return;
    const configs = Object.fromEntries(d.configs.filter((c) => c.source === 'DYNAMIC_TOPIC_CONFIG' && !c.sensitive).map((c) => [c.key, c.value]));
    download(`${this.topic()}-export.json`, JSON.stringify({ name: d.name, partitions: d.partitions.length, replicationFactor: this.replication(), configs }, null, 2));
  }

  async setConfig(c: { key: string; value: string | null }) {
    try {
      await this.api.put(`${this.base()}/configs`, { [c.key]: c.value });
      this.toasts.ok(c.value == null ? `${c.key} reset to default` : `${c.key} updated`);
      this.loadDetail();
    } catch (e) {
      this.toasts.err(e);
    }
  }

  askDelete() {
    this.confirmText = '';
    this.deleteDlg().nativeElement.showModal();
  }

  async deleteTopic() {
    try {
      await this.api.del(this.base());
      this.deleteDlg().nativeElement.close();
      this.toasts.ok(`Deleted ${this.topic()}`);
      this.router.navigate(['/c', this.id(), 'topics']);
    } catch (e) {
      this.toasts.err(e);
    }
  }
}
