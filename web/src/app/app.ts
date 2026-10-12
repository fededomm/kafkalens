import { Component, computed, effect, ElementRef, HostListener, inject, signal, viewChild } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { filter, map } from 'rxjs';
import { Api, GroupRow, Toasts, TopicRow } from './api';
import { Icon } from './icon';

interface PaletteItem { label: string; hint: string; icon: string; link: string[] }

@Component({
  selector: 'app-root',
  imports: [RouterOutlet, RouterLink, RouterLinkActive, FormsModule, Icon],
  templateUrl: './app.html',
  styleUrl: './app.css',
})
export class App {
  protected readonly api = inject(Api);
  protected readonly toasts = inject(Toasts);
  private readonly router = inject(Router);

  private readonly url = toSignal(
    this.router.events.pipe(filter((e) => e instanceof NavigationEnd), map((e) => e.urlAfterRedirects)),
    { initialValue: this.router.url },
  );
  protected readonly clusterId = computed(() => /^\/c\/([^/]+)/.exec(this.url())?.[1] ?? null);
  protected readonly cluster = computed(() => this.api.clusters().find((c) => c.id === this.clusterId()) ?? null);

  protected readonly theme = signal<'dark' | 'light'>((localStorage.getItem('theme') as 'dark' | 'light') ?? 'dark');
  protected readonly switcherOpen = signal(false);

  // command palette
  private readonly paletteDlg = viewChild<ElementRef<HTMLDialogElement>>('palette');
  private readonly toastLayer = viewChild.required<ElementRef<HTMLElement>>('toastLayer');
  protected readonly query = signal('');
  protected readonly active = signal(0);
  private readonly index = signal<PaletteItem[]>([]);
  protected readonly results = computed(() => {
    const q = this.query().trim().toLowerCase();
    const all = this.index();
    // ponytail: substring + prefix boost, swap for a fuzzy matcher if clusters get thousands of topics
    const hits = q ? all.filter((i) => i.label.toLowerCase().includes(q)) : all;
    return hits.sort((a, b) => +b.label.toLowerCase().startsWith(q) - +a.label.toLowerCase().startsWith(q)).slice(0, 50);
  });

  constructor() {
    this.api.refreshClusters().catch((e) => this.toasts.err(e));
    effect(() => {
      document.documentElement.dataset['theme'] = this.theme();
      localStorage.setItem('theme', this.theme());
    });
    effect(() => {
      // re-show on every change: the most recently shown top-layer element paints on top of open dialogs
      const el = this.toastLayer().nativeElement;
      if (el.matches(':popover-open')) el.hidePopover();
      if (this.toasts.items().length) el.showPopover();
    });
    effect(() => {
      const c = this.cluster();
      document.documentElement.style.setProperty('--cluster', c?.color || 'var(--accent)');
    });
  }

  toggleTheme() { this.theme.update((t) => (t === 'dark' ? 'light' : 'dark')); }

  @HostListener('document:keydown', ['$event'])
  onKey(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      this.openPalette();
    }
  }

  async openPalette() {
    const dlg = this.paletteDlg()?.nativeElement;
    if (!dlg || dlg.open) return;
    this.query.set('');
    this.active.set(0);
    const items: PaletteItem[] = this.api.clusters().map((c) => ({ label: c.name, hint: 'Cluster', icon: 'server', link: ['/c', c.id] }));
    this.index.set(items);
    dlg.showModal();
    const id = this.clusterId();
    if (!id) return;
    const [topics, groups] = await Promise.all([
      this.api.get<TopicRow[]>(`/c/${id}/topics`).catch(() => []),
      this.api.get<GroupRow[]>(`/c/${id}/groups`).catch(() => []),
    ]);
    this.index.set([
      ...topics.map((t) => ({ label: t.name, hint: 'Topic', icon: 'topics', link: ['/c', id, 'topics', t.name] })),
      ...groups.map((g) => ({ label: g.group, hint: 'Consumer group', icon: 'groups', link: ['/c', id, 'groups', g.group] })),
      ...items,
    ]);
  }

  paletteKey(e: KeyboardEvent) {
    const n = this.results().length;
    if (e.key === 'ArrowDown') { e.preventDefault(); this.active.update((i) => (i + 1) % Math.max(n, 1)); }
    if (e.key === 'ArrowUp') { e.preventDefault(); this.active.update((i) => (i - 1 + n) % Math.max(n, 1)); }
    if (e.key === 'Enter') { const it = this.results()[this.active()]; if (it) this.go(it); }
  }

  go(it: PaletteItem) {
    this.paletteDlg()?.nativeElement.close();
    this.router.navigate(it.link);
  }
}
