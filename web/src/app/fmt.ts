import { Pipe, PipeTransform } from '@angular/core';
import { desktop } from './api';

const nf = new Intl.NumberFormat();
const compact = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 });

/** 1234567 → "1.2M" (compact) or "1,234,567". */
@Pipe({ name: 'num' })
export class NumPipe implements PipeTransform {
  transform(v: number | null | undefined, mode: 'full' | 'compact' = 'full') {
    if (v == null || v < 0) return '—';
    return mode === 'compact' && v >= 10_000 ? compact.format(v) : nf.format(v);
  }
}

export function fmtTime(ms: number) {
  const d = new Date(ms);
  const pad = (n: number, w = 2) => String(n).padStart(w, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${pad(d.getMilliseconds(), 3)}`;
}

@Pipe({ name: 'time' })
export class TimePipe implements PipeTransform {
  transform(ms: number) { return fmtTime(ms); }
}

export function fmtBytes(n: number) {
  if (n < 1024) return `${n} B`;
  const u = ['KB', 'MB', 'GB', 'TB'];
  let i = -1;
  do { n /= 1024; i++; } while (n >= 1024 && i < u.length - 1);
  return `${n.toFixed(1)} ${u[i]}`;
}

@Pipe({ name: 'bytes' })
export class BytesPipe implements PipeTransform {
  transform(n: number) { return fmtBytes(n); }
}

/** Pretty-prints JSON if the string parses, otherwise returns null. */
export function prettyJson(s: string | null): string | null {
  if (!s) return null;
  const t = s.trimStart();
  if (t[0] !== '{' && t[0] !== '[') return null;
  try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return null; }
}

/** Escapes and colors a pretty-printed JSON string for [innerHTML]. */
export function highlightJson(pretty: string): string {
  const esc = pretty.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  return esc.replace(
    /("(?:\\u[\da-fA-F]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(?:true|false|null)\b|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g,
    (m) => {
      let cls = 'j-num';
      if (m[0] === '"') cls = m.endsWith(':') ? 'j-key' : 'j-str';
      else if (m === 'true' || m === 'false') cls = 'j-bool';
      else if (m === 'null') cls = 'j-null';
      return `<span class="${cls}">${m}</span>`;
    },
  );
}

export function download(name: string, content: string, type = 'application/json') {
  // desktop webviews don't reliably handle <a download>: use a native save dialog
  const d = desktop();
  if (d) return void d.SaveFile(name, content);
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob([content], { type }));
  a.download = name;
  a.click();
  URL.revokeObjectURL(a.href);
}
