// ほかのユーザーの screen のセッションを、sudo モードのときに一覧に出し、そのユーザーになってウィンドウで開けることを確かめる。
// 検証用のサーバーの zuser の screen を使う。smoke.mjs と同じく wails dev の画面を開いて操作する
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let page;
const cleanup = () => sh('screen -S e2e-z -X quit; true', 'zuser');
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await page?.screenshot({ path: path.join(OUT, 'screens-failed.png') }).catch(() => {});
  cleanup();
  process.exit(1);
});
cleanup();
sh('screen -dmS e2e-z', 'zuser');
const sockMode = () => sh('stat -c %a /run/screen/S-zuser/*.e2e-z', 'root');

const browser = await chromium.launch({ channel: 'chrome' });
page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
await page.goto('http://localhost:34115');
await page.evaluate(async () => { for (const s of await window.go.main.App.Servers()) await window.go.main.App.Disconnect(s.id); });
await page.evaluate(() => localStorage.removeItem('drag-settings'));
await page.reload();
await page.waitForSelector('text=サーバーに接続');
await page.click('section button:has-text("testserver")');
await page.fill('[role=dialog] input[type=password]', 'drag');
await page.click('[role=dialog] button:has-text("接続")');
await page.waitForSelector('[role=dialog] button:has-text("開く")', { timeout: 10000 });
await page.click('[role=dialog] button:has-text("開く")');
await page.waitForSelector('.tree-row');
const waitFor = async (fn, ms = 8000) => { for (let t = 0; t < ms; t += 200) { if (await fn().catch(() => false)) return true; await page.waitForTimeout(200); } return false; };

// 下の欄のメモリは、使っている量と全体の量
ok('下の欄のメモリは GB で出す', await waitFor(async () => /メモリ.*\d+(\.\d)? \/ \d+(\.\d)? GB/.test(await page.textContent('[data-sb="mem"]'))), await page.textContent('[data-sb="mem"]').catch(() => ''));

// sudo モードが OFF なら、ほかのユーザーの screen は見えない
await page.click('nav button[title="セッション"]');
const item = page.locator('[data-session^="screen:zuser/"][data-session$=".e2e-z"]');
await page.waitForSelector('aside :text("screen")');
await page.waitForTimeout(1500);
ok('sudo モードが OFF なら出さない', (await item.count()) === 0);

// ON にすると、持ち主を付けて出す
await page.click('[data-sudo]');
const dialog = page.locator('[role=dialog]:has-text("sudo モードを ON にする")');
await dialog.locator('input[type=password]').fill('drag');
await dialog.locator('button:has-text("ON にする")').click();
ok('ON にすると、ほかのユーザーの screen を出す', await waitFor(async () => (await item.count()) === 1, 10000));
ok('持ち主の名前を出す', (await item.textContent()).includes('zuser のセッション'), await item.textContent());

// 開くと、そのユーザーになって screen につなぐ。sudo のパスワードはウィンドウの中で聞かれる
await item.click();
const win = page.locator('[data-win]:has-text("zuser/")').last();
ok('ウィンドウで開く', await waitFor(async () => (await win.count()) === 1));
ok('sudo のパスワードを聞く', await waitFor(async () => (await win.locator('.xterm-rows').textContent()).includes('password for asuta')));
await win.locator('.xterm-screen').click();
await page.keyboard.type('drag\n');
ok('screen につながる', await waitFor(async () => sockMode() === '700'), sockMode());
ok('一覧にウィンドウで開いていると出る', await waitFor(async () => (await item.textContent()).includes('ウィンドウ')));
await page.screenshot({ path: `${OUT}/screens-other.png` });

// 閉じるとデタッチし、セッションは残る
await win.locator('button[title="閉じる"]').click();
ok('閉じるとデタッチする', await waitFor(async () => sockMode() === '600'), sockMode());
ok('セッションは残る', sh('screen -ls', 'zuser').includes('e2e-z'));
await page.click('[data-sudo]');

cleanup();
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
