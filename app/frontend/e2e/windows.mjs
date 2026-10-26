// デスクトップのウィンドウとサイドバーの動きを確かめる。サイドバーを畳む、ドラッグの案内、
// Docker のコンテナをデスクトップへドラッグしてシェルを開く、開く・最小化・閉じる。
// Docker は検証用のサーバーに置いた偽の docker (dev/testserver/fakedocker.sh) を使う
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let page;
const cleanup = () => sh('rm -rf /var/lib/fakedocker/*', 'root');
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await page?.screenshot({ path: path.join(OUT, 'windows-failed.png') }).catch(() => {});
  cleanup();
  process.exit(1);
});
cleanup();

const browser = await chromium.launch({ channel: 'chrome' });
page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
await page.goto('http://localhost:34115');
// 前のテストで開いたままの接続を閉じる。wails dev では画面を読み込み直しても Go の側の接続は残る
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
const asideWidth = () => page.$eval('aside', e => e.getBoundingClientRect().width);

// 出している一覧のボタンをもう一度押すと、サイドバーを畳む
const w0 = await asideWidth();
await page.click('nav button[title="ファイル"]');
ok('同じボタンでサイドバーを畳む', await waitFor(async () => (await asideWidth()) <= 1 && (await page.$eval('[data-desktop]', e => e.getBoundingClientRect().left)) < 60), `${w0} → ${await asideWidth()} / ${await page.$eval('[data-desktop]', e => e.getBoundingClientRect().left)}`);
await page.click('nav button[title="セッション"]');
ok('ほかのボタンで開いて切り替える', await waitFor(async () => (await asideWidth()) === w0) && (await page.isVisible('aside :text("セッション")')));
await page.click('nav button[title="ファイル"]');

// デスクトップへのドラッグの案内。フォルダとバイナリは受け付けない
const label = async (types) => {
  const dt = await page.evaluateHandle(ts => { const d = new DataTransfer(); for (const t of ts) d.setData(t, '1'); return d; }, types);
  await page.dispatchEvent('[data-desktop]', 'dragover', { dataTransfer: dt });
  const text = await page.locator('[data-desktop] .drop-tag').last().textContent({ timeout: 500 }).catch(() => '');
  await page.dispatchEvent('[data-desktop]', 'dragleave', { dataTransfer: dt });
  return text;
};
ok('ファイルは「ここで開く」', (await label(['application/x-drag-path'])) === 'ここで開く');
ok('セッションは「ここで開く」', (await label(['application/x-drag-session'])) === 'ここで開く');
ok('コンテナは「シェルを開く」', (await label(['application/x-drag-container'])) === 'シェルを開く');
ok('ターミナルのコマンドは「ここに移す」', (await label(['application/x-drag-fg'])) === 'ここに移す');
ok('フォルダは受け付けない', (await label(['application/x-drag-path', 'application/x-drag-dir'])) === '');
ok('バイナリは受け付けない', (await label(['application/x-drag-path', 'application/x-drag-bin'])) === '');

// Docker の動いているコンテナを、デスクトップへドラッグしてシェルを開く
await page.click('[data-tool="docker"]');
const dialog = page.locator('[role=dialog]:has-text("sudo モードを ON にする")');
await dialog.waitFor({ timeout: 5000 });
await dialog.locator('input[type=password]').fill('drag');
await dialog.locator('button:has-text("ON にする")').click();
const dk = page.locator('[data-win]:has(.dk-list)');
await dk.locator('[data-container="web"] .dk-row').waitFor();
ok('動いているコンテナはドラッグできる', (await dk.locator('[data-container="web"] .dk-row').getAttribute('draggable')) === 'true'
  && (await dk.locator('[data-container="backup"] .dk-row').getAttribute('draggable')) === 'false');
await dk.locator('[data-container="web"] .dk-row').dragTo(page.locator('[data-desktop]'), { targetPosition: { x: 300, y: 220 } });
const shWin = page.locator('[data-win]:has-text("docker exec -it web sh")');
ok('落とすとシェルを開く', await waitFor(async () => (await shWin.count()) === 1));
ok('落とした場所に開く', await waitFor(async () => Math.abs((await shWin.boundingBox()).x - (await page.locator('[data-desktop]').boundingBox()).x - 260) < 30));

// 最小化はタスクバーへ縮めてから隠し、元に戻すと出る
const id = await shWin.getAttribute('data-win');
await shWin.locator('button[title="最小化"]').click();
ok('最小化すると隠れる', await waitFor(async () => !(await page.isVisible(`[data-win="${id}"]`))));
await page.click(`[data-task="${id}"]`);
ok('タスクバーで元に戻す', await waitFor(async () => await page.isVisible(`[data-win="${id}"]`)));
await page.screenshot({ path: `${OUT}/windows-container-shell.png` });

// 閉じるとすぐに一覧から消える
await shWin.locator('button[title="閉じる"]').click();
ok('閉じる', await waitFor(async () => (await page.locator(`[data-win="${id}"]`).count()) === 0 && (await page.locator(`[data-task="${id}"]`).count()) === 0));
await page.click('[data-sudo]');

cleanup();
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
