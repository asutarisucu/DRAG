// 検証3: PC の幅 (120 桁) のターミナルを、スマホの幅で横スクロールさせて表示できるか
// 検証用のコンテナで DRAG_LISTEN=0.0.0.0:8080 を付けてエージェントを動かしてから node phone.mjs
import { chromium } from 'playwright-core';
import path from 'node:path';
const out = f => path.join(import.meta.dirname, 'out', f);
const base = 'http://127.0.0.1:8080';
const id = (await (await fetch(base + '/sessions', { method: 'POST' })).text()).trim();

// CHROME_PATH が無ければ、入っている Google Chrome を使う
const b = await chromium.launch(process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : { channel: 'chrome' });
const ctx = await b.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 3, hasTouch: true, isMobile: true });
const errs = [];
const open = async (q) => {
  const p = await ctx.newPage();
  p.on('pageerror', e => errs.push(e.message));
  await p.goto(`${base}/?id=${id}&${q}`);
  await p.waitForFunction(() => window.term && term.buffer.active.getLine(0)?.translateToString(true).length > 0);
  return p;
};
const lines = p => p.evaluate(() => {
  const buf = term.buffer.active;
  const r = [];
  for (let y = buf.viewportY; y < buf.viewportY + term.rows; y++) r.push(buf.getLine(y)?.translateToString(true) ?? '');
  return r;
});
const waitText = (p, s) => p.waitForFunction(s => {
  const buf = term.buffer.active;
  for (let y = 0; y < buf.length; y++) if (buf.getLine(y)?.translateToString(true).includes(s)) return true;
  return false;
}, s, { timeout: 5000 });
// synthesizeScrollGesture は素の横スクロール要素でも動かなかったので、タッチイベントを直接送る
const swipe = async (p, dx, dy, x = 300, y = 400, steps = 20) => {
  const cdp = await p.context().newCDPSession(p);
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x, y }] });
  for (let i = 1; i <= steps; i++) {
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: x + dx * i / steps, y: y + dy * i / steps }] });
    await p.waitForTimeout(16);
  }
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await p.waitForTimeout(600);
};
const result = (name, ok, detail = '') => console.log(`${ok ? 'OK' : 'NG'}  ${name}  ${detail}`);

const p = await open('cols=120&rows=30&resize');
// 端末をタップすると入力欄にフォーカスが移る (スマホではキーボードが出る)
await p.locator('#term').tap({ position: { x: 100, y: 100 } });
const focused = await p.evaluate(() => document.activeElement?.classList.contains('xterm-helper-textarea'));
result('タップで入力欄にフォーカス', focused);

await p.keyboard.type(`printf '%114s%s\\n' '' 'RIGHT!'; seq 1 100; echo DONE`);
await p.keyboard.press('Enter');
await waitText(p, 'DONE');
await p.waitForTimeout(300);

const m = await p.evaluate(() => {
  const w = document.getElementById('wrap');
  return { scrollW: w.scrollWidth, clientW: w.clientWidth, pageW: document.documentElement.scrollWidth, termW: document.querySelector('.xterm-screen').getBoundingClientRect().width };
});
result('120 桁が画面の幅を超えて描かれる', m.scrollW > m.clientW, JSON.stringify(m));
result('ページ全体は横にはみ出さない', m.pageW === 390, `pageW=${m.pageW}`);
const typedLeft = await p.evaluate(() => wrap.scrollLeft);
result('打っている間は入力位置が見える位置まで横に動く', typedLeft > 0, `scrollLeft=${typedLeft}`);
await p.evaluate(() => wrap.scrollLeft = 0);
await p.screenshot({ path: out('v-1-left.png') });

await swipe(p, -250, 0);
await swipe(p, -250, 0);
const left = await p.evaluate(() => document.getElementById('wrap').scrollLeft);
result('横のスワイプで右端まで動く', left >= m.scrollW - m.clientW - 1, `scrollLeft=${left}`);
await p.screenshot({ path: out('v-2-right.png') });

const before = await p.evaluate(() => term.buffer.active.viewportY);
await swipe(p, 0, 300);
const after = await p.evaluate(() => term.buffer.active.viewportY);
const leftAfter = await p.evaluate(() => document.getElementById('wrap').scrollLeft);
result('縦のスワイプでスクロールバックが戻る', after < before, `viewportY ${before} -> ${after}, scrollLeft=${leftAfter}`);
await p.screenshot({ path: out('v-3-scrollback.png') });

// 全画面のアプリを開いたまま、別のページでつなぎ直す
await p.keyboard.type('vi /etc/os-release');
await p.keyboard.press('Enter');
await p.waitForFunction(() => term.buffer.active.type === 'alternate', null, { timeout: 5000 });
await p.waitForTimeout(500);
const orig = await lines(p);
const p2 = await open('cols=120&rows=30');
await p2.waitForTimeout(800);
const re = await lines(p2);
const type2 = await p2.evaluate(() => term.buffer.active.type);
const same = orig.every((l, i) => l === re[i]);
result('vi を開いたままつなぎ直すと同じ画面になる', type2 === 'alternate' && same, `buffer=${type2}`);
if (!same) orig.forEach((l, i) => { if (l !== re[i]) console.log(`  ${i}: ${JSON.stringify(l)} / ${JSON.stringify(re[i])}`); });
await p2.screenshot({ path: out('v-4-reattach-vi.png') });
await p.keyboard.type(':q!');
await p.keyboard.press('Enter');

console.log('errors:', errs.length ? errs : 'none');
await b.close();
