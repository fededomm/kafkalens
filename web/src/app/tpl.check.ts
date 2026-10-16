// Run: node --experimental-strip-types src/app/tpl.check.ts
import assert from 'node:assert/strict';
import { tplHtml, tplStub, tplValid } from './tpl.ts';

for (const ok of ['{{uuid}}', '{{ i }}', '{{- now -}}', '{{randInt 1 100}}', '{{seq "a" "b c"}}', '{{pick 1 `x`}}', '{{/* note */}}'])
  assert.ok(tplValid(ok), ok);
for (const ko of ['{{', '{{}}', '{{nope}}', '{{uuid 1}}', '{{randInt 1}}', '{{randInt "a" 2}}', '{{seq}}', '{{seq a}}', '{{{i}}'])
  assert.ok(!tplValid(ko), ko);

assert.equal(tplHtml('<{{i}}'), '&lt;<mark class="tpl">{{i}}</mark>\n');
assert.equal(tplHtml('a {{ b'), 'a <mark class="tpl bad">{{</mark> b\n');
assert.equal(tplHtml('{"a":{"b":1}}'), '{"a":{"b":1}}\n'); // stray }} is plain text, as in Go
assert.equal(JSON.parse(tplStub('{"n": {{i}}, "id": "{{uuid}}"}')).n, 0);
console.log('tpl ok');
