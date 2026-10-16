// Client-side mirror of the template funcs in template.go — keep the two in sync.
const FUNCS: Record<string, [min: number, max: number]> = {
  uuid: [0, 0], i: [0, 0], now: [0, 0], unixMs: [0, 0], randInt: [2, 2], pick: [1, Infinity], seq: [1, Infinity],
};
const ACTION = /\{\{[\s\S]*?\}\}|\{\{/g; // a closed action, or an unclosed {{
const TOKEN = /"(?:[^"\\]|\\.)*"|`[^`]*`|\S+/g;
const LITERAL = /^(?:"(?:[^"\\]|\\.)*"|`[^`]*`|-?\d+(?:\.\d+)?)$/;

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

/** True if `{{…}}` calls a known func with literal args of the right arity (or is a comment). */
export function tplValid(action: string): boolean {
  if (action.length < 4 || !action.endsWith('}}')) return false;
  const body = action.slice(2, -2).replace(/^-\s/, '').replace(/\s-$/, '').trim(); // {{- x -}} trim markers
  if (/^\/\*[\s\S]*\*\/$/.test(body)) return true;
  const [fn, ...args] = body.match(TOKEN) ?? [];
  const arity = fn ? FUNCS[fn] : undefined;
  if (!arity || args.length < arity[0] || args.length > arity[1] || !args.every((a) => LITERAL.test(a))) return false;
  return fn !== 'randInt' || args.every((a) => /^-?\d+$/.test(a));
}

/** Escaped text with actions in <mark class="tpl"> (plus "bad" when invalid), for [innerHTML]. */
export function tplHtml(v: string): string {
  let out = '', last = 0;
  for (const m of v.matchAll(ACTION)) {
    out += esc(v.slice(last, m.index)) + `<mark class="tpl${tplValid(m[0]) ? '' : ' bad'}">${esc(m[0])}</mark>`;
    last = m.index + m[0].length;
  }
  return out + esc(v.slice(last)) + '\n'; // trailing \n keeps textarea and mirror heights in sync
}

/** Replaces every action with 0 so a templated value can still be JSON-checked. */
export const tplStub = (v: string) => v.replace(ACTION, '0');
