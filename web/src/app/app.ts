import { afterRenderEffect, Component, computed, effect, ElementRef, HostListener, inject, signal, viewChild } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, NavigationStart, Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { filter, map } from 'rxjs';
import { Api, Confirm, McpStatus, Toasts } from './api';
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
  protected readonly confirm = inject(Confirm);
  private readonly router = inject(Router);
  private readonly confirmDlg = viewChild.required<ElementRef<HTMLDialogElement>>('confirmDlg');

  private readonly url = toSignal(
    this.router.events.pipe(filter((e) => e instanceof NavigationEnd), map((e) => e.urlAfterRedirects)),
    { initialValue: this.router.url },
  );
  protected readonly clusterId = computed(() => /^\/c\/([^/]+)/.exec(this.url())?.[1] ?? null);
  protected readonly cluster = computed(() => this.api.clusters().find((c) => c.id === this.clusterId()) ?? null);

  protected readonly version = signal(''); // from wails.json via /api/version
  protected readonly theme = signal<'dark' | 'light'>((localStorage.getItem('theme') as 'dark' | 'light') ?? 'dark');
  protected readonly switcherOpen = signal(false);
  protected readonly mcp = signal<McpStatus | null>(null); // null: backend without MCP status
  protected readonly mcpOpen = signal(false);

  // command palette
  private readonly paletteDlg = viewChild.required<ElementRef<HTMLDialogElement>>('palette');
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
    this.api.get<{ version: string }>('/version').then((r) => this.version.set(r.version), () => {});
    this.loadMcp();
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
    // after render, so showModal focuses the typed-name input when there is one (else Cancel)
    afterRenderEffect(() => {
      // Esc closes the dialog natively: its close event answers false
      const dlg = this.confirmDlg().nativeElement;
      if (this.confirm.req()) { if (!dlg.open) dlg.showModal(); } else if (dlg.open) dlg.close();
    });
    // the asking page goes away (Ctrl+K and Back work under the modal): never act on its behalf
    this.router.events.subscribe((e) => e instanceof NavigationStart && this.confirm.answer(false));
    effect(() => {
      const c = this.cluster();
      document.documentElement.style.setProperty('--cluster', c?.color || 'var(--accent)');
    });
  }

  /** Also on focus and when the panel opens: a restarted backend turns MCP back on (and may have a new token). */
  @HostListener('window:focus')
  loadMcp() { this.api.get<McpStatus>('/mcp').then((s) => this.mcp.set(s), () => {}); }

  toggleMcpPanel() {
    this.mcpOpen.update((o) => !o);
    if (this.mcpOpen()) this.loadMcp();
  }

  /** The switch shows the server's answer, not the click: a refused change leaves it as it was. */
  setMcp(on: boolean) {
    return this.toasts.guard(async () => this.mcp.set(await this.api.put<McpStatus>('/mcp', { enabled: on })));
  }

  // user scope: KafkaLens serves every project; the URL quoted for zsh ([::1] is a glob there)
  claudeCmd(m: McpStatus) { return `claude mcp add --transport http --scope user kafkalens "${m.url}" --header "Authorization: Bearer ${m.token}"`; }

  copy(text: string, what: string) {
    return this.toasts.guard(async () => { await navigator.clipboard.writeText(text); this.toasts.ok(`${what} copied`); });
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
    const dlg = this.paletteDlg().nativeElement;
    if (dlg.open) return;
    this.query.set('');
    this.active.set(0);
    const items: PaletteItem[] = this.api.clusters().map((c) => ({ label: c.name, hint: 'Cluster', icon: 'server', link: ['/c', c.id] }));
    this.index.set(items);
    dlg.showModal();
    const id = this.clusterId();
    if (!id) return;
    const [topics, groups] = await Promise.all([
      this.api.get<string[]>(`/c/${id}/topics?names`).catch(() => []),
      this.api.get<string[]>(`/c/${id}/groups?names`).catch(() => []),
    ]);
    if (!dlg.open || this.clusterId() !== id) return; // late answer: palette closed, maybe reopened in another cluster
    this.index.set([
      ...topics.map((t) => ({ label: t, hint: 'Topic', icon: 'topics', link: ['/c', id, 'topics', t] })),
      ...groups.map((g) => ({ label: g, hint: 'Consumer group', icon: 'groups', link: ['/c', id, 'groups', g] })),
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
    this.paletteDlg().nativeElement.close();
    this.router.navigate(it.link);
  }
}
