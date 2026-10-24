// PC で開いているターミナルを、スマホから続けて操作できることを確かめる。
// スマホでは PC の幅のまま描き、はみ出す分は横にスクロールする。閉じても PC のシェルは残る。
// phone.mjs と同じく wails dev を DRAG_PHONE_LISTEN=127.0.0.1:18443 で動かしておく
import { chromium } from 'playwright-core';
import path from 'node:path';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let pc, phone;
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await phone?.screenshot({ path: path.join(OUT, 'shared-failed.png') }).catch(() => {});
  process.exit(1);
});
const waitFor = async (fn, ms = 8000) => { for (let t = 0; t < ms; t += 200) { if (await fn().catch(() => false)) return true; await new Promise(r => setTimeout(r, 200)); } return false; };
const rowsText = (page, sel) => page.$eval(`${sel} .xterm-rows`, e => [...e.children].map(r => r.textContent).join('\n'));

const browser = await chromium.launch({ channel: 'chrome' });
pc = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await pc.goto('http://localhost:34115');
// 前のテストで開いたままの接続を閉じる。wails dev では画面を読み込み直しても Go の側の接続は残る
await pc.evaluate(async () => { for (const s of await window.go.main.App.Servers()) await window.go.main.App.Disconnect(s.id); });
await pc.evaluate(() => localStorage.removeItem('drag-settings'));
await pc.reload();
await pc.waitForSelector('text=サーバーに接続');
await pc.click('section button:has-text("testserver")');
await pc.fill('[role=dialog] input[type=password]', 'drag');
await pc.click('[role=dialog] button:has-text("接続")');
await pc.waitForSelector('[role=dialog] button:has-text("開く")', { timeout: 10000 });
await pc.click('[role=dialog] button:has-text("開く")');
await pc.waitForSelector('.tree-row');
const pcTerm = '[data-terminal]';
await waitFor(async () => (await rowsText(pc, pcTerm)).includes('$'));
await pc.click(`${pcTerm} .xterm-screen`);
await pc.keyboard.type('echo PC-$((1+1))\n');
await waitFor(async () => (await rowsText(pc, pcTerm)).includes('PC-2'));
// 桁数は、同じ文字の大きさで描いた画面の幅で比べる
const screenW = (page, sel) => page.$eval(`${sel} .xterm-screen`, e => Math.round(e.getBoundingClientRect().width));
const pcW = await screenW(pc, pcTerm);

await pc.click('header button:has-text("スマホを登録")');
await pc.waitForSelector('svg[data-qr^="https://"]');
// 前からある端末は覚えておき、あと片付けではこのテストで登録したものだけを無効にする
const deviceIds = () => pc.$$eval('[data-device]', l => l.map(e => e.dataset.device));
await pc.waitForSelector('[role=dialog] [data-device], [role=dialog] :text("まだありません")');
const known = new Set(await deviceIds());
// 開発中の React は effect を2回動かすので、QR コードが作り直されて落ち着くのを待つ
let url = '';
for (let prev = null; url !== prev; await pc.waitForTimeout(600)) { prev = url; url = await pc.$eval('svg[data-qr]', e => e.dataset.qr); }
await pc.keyboard.press('Escape');

const ctx = await browser.newContext({
  viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, ignoreHTTPSErrors: true, locale: 'ja-JP',
  userAgent: 'Mozilla/5.0 (Linux; Android 15; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36',
});
phone = await ctx.newPage();
phone.on('pageerror', e => errors.push('phone pageerror ' + e.stack));
await phone.goto(url);
await phone.waitForSelector('nav [data-mtab="files"]', { timeout: 10000 });

// セッションの一覧に PC のターミナルが出る
await phone.tap('nav [data-mtab="files"]');
await phone.tap('aside button:has-text("セッション")');
const item = phone.locator('[data-other-shell]').first();
ok('PC で開いているシェルが一覧に出る', await waitFor(async () => (await item.count()) > 0 && (await item.textContent()).includes('PC で開いている'), 10000));
await item.tap();
const view = phone.locator('[data-shared]');
ok('開くとデスクトップに移る', await waitFor(async () => await view.isVisible()));
ok('PC の出力の続きが出る', await waitFor(async () => (await rowsText(phone, '[data-shared]')).includes('PC-2')));
const sz = await view.evaluate(e => ({ sw: e.scrollWidth, cw: e.clientWidth }));
const phoneW = await screenW(phone, '[data-shared]');
ok('PC の幅のまま描き、横にスクロールできる', Math.abs(phoneW - pcW) <= 2 && sz.sw > sz.cw, JSON.stringify({ ...sz, phoneW, pcW }));
await phone.screenshot({ path: `${OUT}/shared-phone.png` });

// スマホで打つと PC のターミナルにも出る
await view.locator('.xterm-screen').tap();
await phone.keyboard.type('echo PHONE-$((3+4))\n');
ok('スマホで打ったものが PC に出る', await waitFor(async () => (await rowsText(pc, pcTerm)).includes('PHONE-7')));

// PC で幅を変えると、スマホの描く幅も合わせる
await pc.setViewportSize({ width: 1200, height: 900 });
await waitFor(async () => (await screenW(pc, pcTerm)) < pcW - 20);
const pcW2 = await screenW(pc, pcTerm);
ok('PC で幅を変えるとスマホも合わせる', pcW2 < pcW && await waitFor(async () => Math.abs((await screenW(phone, '[data-shared]')) - pcW2) <= 2), `${pcW} → ${pcW2} / phone ${await screenW(phone, '[data-shared]')}`);

// スマホで閉じても PC のシェルは残る
await phone.locator('[data-win] button[title="閉じる"]').tap();
await pc.click(`${pcTerm} .xterm-screen`);
await pc.keyboard.type('echo STILL-$((4+4))\n');
ok('スマホで閉じても PC のシェルは残る', await waitFor(async () => (await rowsText(pc, pcTerm)).includes('STILL-8')));

// あと片付け。登録した端末を無効にする
await pc.click('header button:has-text("スマホを登録")');
await pc.waitForSelector('[data-device]');
const added = (await deviceIds()).filter(id => !known.has(id));
for (const id of added) await pc.click(`[data-device="${id}"] button:has-text("無効にする")`);
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
