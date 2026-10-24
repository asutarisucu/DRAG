// スマホの登録と、スマホの画面から PC アプリを通してサーバーを操作できることを確かめる。
// wails dev を DRAG_PHONE_LISTEN=127.0.0.1:18443 で動かしておく。PC の画面は smoke.mjs と同じく wails dev の画面を使う
import { chromium } from 'playwright-core';
import fs from 'node:fs';
import path from 'node:path';

const OUT = path.join(import.meta.dirname, 'out');
const PHONE = 'https://127.0.0.1:18443';
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let pc, phone;
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await pc?.screenshot({ path: path.join(OUT, 'phone-failed-pc.png') }).catch(() => {});
  await phone?.screenshot({ path: path.join(OUT, 'phone-failed-phone.png') }).catch(() => {});
  process.exit(1);
});
const waitFor = async (fn, ms = 8000) => { for (let t = 0; t < ms; t += 200) { if (await fn().catch(() => false)) return true; await new Promise(r => setTimeout(r, 200)); } return false; };

const browser = await chromium.launch({ channel: 'chrome' });
pc = await browser.newPage({ viewport: { width: 1440, height: 900 } });
pc.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pc pageerror ' + e.stack); });
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

// 設定のスマホのページ。直接公開は ON にせず、項目とポートの確かめだけを見る (ON にすると本物の Let's Encrypt に頼む)
await pc.click('button[title="設定"]');
await pc.click('[role=dialog] button:has-text("スマホ")');
const setText = () => pc.textContent('[role=dialog]');
ok('設定に LAN と直接公開の項目が出る', await waitFor(async () => (await setText()).includes('LAN 内で待ち受ける') && (await setText()).includes('インターネットに公開する') && (await setText()).includes('公開すると Let\'s Encrypt で取る')));
const portInput = pc.locator('[role=dialog] input[inputmode="numeric"]');
ok('公開するポートは 443 から始まる', await portInput.inputValue() === '443');
await portInput.fill('70000');
await portInput.press('Enter');
ok('範囲の外のポートは受け付けない', await waitFor(async () => (await setText()).includes('ポートは 1 から 65535 までです') && await portInput.inputValue() === '443'));

// 自分のドメインの証明書。公開は OFF のまま、入力と保存だけを見る (トークンは開発用の設定フォルダに入る)
const certForm = pc.locator('[role=dialog] [data-cert-domain]');
await pc.click('[role=dialog] button:has-text("自分のドメイン")');
ok('自分のドメインを選ぶと入力欄が出る', await certForm.isVisible() && (await setText()).includes('どのポートでも取れる'));
await certForm.locator('input:not([type=password])').fill('bad_domain');
await certForm.locator('button:has-text("保存")').click();
ok('形の違うドメインは受け付けない', await waitFor(async () => (await certForm.textContent()).includes('ドメインの形が違います')));
await certForm.locator('input:not([type=password])').fill('drag.example.com');
await certForm.locator('button:has-text("保存")').click();
const hadToken = await certForm.locator('input[type=password]').getAttribute('placeholder') === '保存済み。変えるときだけ入れる';
ok('トークンが無ければ入れるよう出す', hadToken || await waitFor(async () => (await certForm.textContent()).includes('Cloudflare の API トークンを入れてください')));
await certForm.locator('input[type=password]').fill('dummy-token-for-e2e');
await certForm.locator('button:has-text("保存")').click();
ok('保存するとトークンは保存済みと出す', await waitFor(async () => await certForm.locator('input[type=password]').getAttribute('placeholder') === '保存済み。変えるときだけ入れる'
  && await certForm.locator('input[type=password]').inputValue() === ''));
await pc.click('[role=dialog] button:has-text("IP アドレス")');
ok('IP アドレスに戻す', await waitFor(async () => (await certForm.count()) === 0 && (await pc.evaluate(() => window.go.main.App.PhoneState())).settings.cert === 'ip'));
// 開発用の設定フォルダに入れた試しのトークンを消す
fs.rmSync(path.join(process.env.APPDATA, 'DRAG-dev', 'cloudflare.token'), { force: true });
await pc.keyboard.press('Escape');

// 登録していない端末には画面を出さない
const ctx = await browser.newContext({
  viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, ignoreHTTPSErrors: true,
  userAgent: 'Mozilla/5.0 (Linux; Android 15; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36',
  locale: 'ja-JP',
});
phone = await ctx.newPage();
phone.on('pageerror', e => errors.push('phone pageerror ' + e.stack));
let res = await phone.goto(PHONE + '/');
ok('登録していない端末には画面を出さない', res.status() === 403 && (await phone.textContent('h1')) === 'この端末は登録されていません');

// PC で QR コードを出す
await pc.click('header button:has-text("スマホを登録")');
await pc.waitForSelector('svg[data-qr^="https://"]');
// 前からある端末は覚えておき、あと片付けではこのテストで登録したものだけを無効にする
const deviceIds = () => pc.$$eval('[data-device]', l => l.map(e => e.dataset.device));
await pc.waitForSelector('[role=dialog] [data-device], [role=dialog] :text("まだありません")');
const known = new Set(await deviceIds());
// 開発中の React は effect を2回動かすので、QR コードが作り直されて落ち着くのを待つ
let url = '';
for (let prev = null; url !== prev; await pc.waitForTimeout(600)) { prev = url; url = await pc.$eval('svg[data-qr]', e => e.dataset.qr); }
ok('QR コードに登録の URL が入る', url.startsWith(PHONE + '/pair?t='), url.replace(/t=.*/, 't=…'));
ok('残り時間が出る', /で無効になります/.test(await pc.textContent('[role=dialog]')));
await pc.screenshot({ path: `${OUT}/phone-pair.png` });

// スマホで読むと登録され、画面が出る
res = await phone.goto(url);
await phone.waitForSelector('text=この端末を登録しました', { timeout: 8000 }).then(() => ok('登録すると知らせる', true), () => ok('登録すると知らせる', false));
ok('URL からトークンが消える', phone.url() === PHONE + '/', phone.url());
ok('PC の一覧に端末が出る', await waitFor(async () => (await deviceIds()).length === known.size + 1));
ok('PC に登録を知らせる', await waitFor(async () => (await pc.locator('[role=status]:has-text("スマホを登録しました")').count()) > 0));
ok('QR コードを作り直す', await waitFor(async () => (await pc.$eval('svg[data-qr]', e => e.dataset.qr)) !== url));
// 同じ QR コードはもう使えない
const other = await browser.newContext({ ignoreHTTPSErrors: true, locale: 'ja-JP' });
const p2 = await other.newPage();
res = await p2.goto(url);
ok('使った QR コードは2度使えない', res.status() === 410 && (await p2.textContent('h1')) === 'この QR コードは使えません');
await other.close();

// スマホの画面に、PC でつないだサーバーが出て、PC アプリを通して操作できる
ok('PC でつないだサーバーのタブが出る', await waitFor(async () => (await phone.locator('header:has-text("testserver")').count()) > 0));
ok('ファイルツリーが出る', await waitFor(async () => (await phone.locator('.tree-row').count()) > 0));

// スマホで打ったコマンドは、PC の履歴に操作として残る
await phone.tap('nav [data-mtab="terminal"]');
await phone.locator('[data-terminal] .xterm-screen').tap();
await phone.keyboard.type('echo OP-$((40+2))\n');
ok('スマホで打ったコマンドが履歴に残る', await waitFor(async () => (await pc.$$eval('[role=dialog] [data-kind="op"]', l => l.map(r => r.textContent).join(' / '))).includes('入力 echo OP-$((40+2))')));
await phone.screenshot({ path: `${OUT}/phone-desktop-layout.png` });

// 端末を無効にすると、スマホの画面は切れる
const added = (await deviceIds()).filter(id => !known.has(id));
for (const id of added) await pc.click(`[data-device="${id}"] button:has-text("無効にする")`);
ok('PC の一覧から消える', await waitFor(async () => (await deviceIds()).length === known.size));
ok('スマホに切れたと出る', await waitFor(async () => (await phone.locator('[role=alert]:has-text("PC アプリとの接続が切れました")').count()) > 0));
res = await phone.goto(PHONE + '/');
ok('無効にした端末には画面を出さない', res.status() === 403);

const hist = await pc.$$eval('[role=dialog] details div.grid > div', l => l.map(r => r.textContent));
ok('履歴に登録とログインが残る', hist.some(h => h.includes('登録')) && hist.some(h => h.includes('ログイン')) && hist.some(h => h.includes('使えない QR コード')), hist.slice(0, 4).join(' / '));

console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
