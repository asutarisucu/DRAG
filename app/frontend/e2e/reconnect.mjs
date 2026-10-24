// 接続が切れたときのウィンドウを確かめる。ターミナルと tmux のウィンドウは残して再接続したら開き直し、
// ほかのセッションは「終了」と出して残す。切断・再接続の知らせと、下の欄の表示も見る。
// smoke.mjs と同じく、検証用のサーバーで asuta の sshd のプロセスを止めて切る
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let page;
const cleanup = () => sh('tmux kill-session -t e2e-r; true');
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await page?.screenshot({ path: path.join(OUT, 'reconnect-failed.png') }).catch(() => {});
  cleanup();
  process.exit(1);
});
cleanup();
sh('tmux new -d -s e2e-r');

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
const win = title => page.locator(`[data-win]:has-text("${title}")`).last();
const winText = title => win(title).locator('.xterm-rows').textContent();

ok('タブにユーザーとホストを出す', (await page.locator('header [title="asuta@127.0.0.1"]').count()) === 1);
ok('稼働時間に起動した日時を出す', await waitFor(async () => /\d+\/\d+ \d\d:\d\d の起動から連続で稼働/.test(await page.getAttribute('[data-sb="uptime"]', 'title'))));

// tmux のセッション、新しいターミナル、ターミナルから移した top をウィンドウで開く
await page.click('nav button[title="セッション"]');
await page.click('[data-session="tmux:e2e-r"]');
await waitFor(async () => sh('tmux ls').includes('(attached)'));
await page.click('button[title="新しいターミナル"]');
await waitFor(async () => (await winText('ターミナル 1')).includes('$'));
await page.click('[data-terminal] .xterm-screen');
await page.keyboard.type('top\n');
await page.locator('[data-terminal] [role=button]:has-text("top")').click();
await waitFor(async () => (await win('top').count()) === 1);
ok('移した top はグラフのアイコン', (await win('top').locator('svg').first().innerHTML()).includes('m7 15 4-5 3 3 5-6'));

// 切る
sh('pkill -f "sshd: asuta"', 'root');
ok('切れたら表示が出る', await waitFor(async () => (await page.locator('[role=alert]').count()) > 0, 30000));
ok('下の欄に切れていると出る', (await page.textContent('[data-sb="conn"]')).includes('接続が切れています'));
ok('ターミナルのウィンドウは残して待つ', await waitFor(async () => (await winText('ターミナル 1')).includes('再接続したら新しいシェルで開き直します')));
ok('tmux のウィンドウは残して待つ', await waitFor(async () => (await winText('e2e-r')).includes('再接続したらセッションにつなぎ直します')));
ok('ほかのセッションは終了と出す', await waitFor(async () => (await win('top · 終了').count()) === 1 && (await winText('top · 終了')).includes('接続が切れたため、終了しました')));
await page.screenshot({ path: `${OUT}/reconnect-lost.png` });

// 自動で再接続すると、開き直す
ok('再接続したと知らせる', await waitFor(async () => (await page.locator('[role=status]:has-text("testserver に再接続しました")').count()) > 0, 30000));
ok('tmux につなぎ直す', await waitFor(async () => sh('tmux ls').includes('e2e-r') && sh('tmux ls').includes('(attached)'), 10000));
ok('ターミナルのウィンドウを新しいシェルで開き直す', await waitFor(async () => (await winText('ターミナル 1')).includes('新しいシェルで開き直しました')));
// 前に出してから打つ。終了した top のウィンドウが上に重なっている
await page.locator(`[data-task]:has-text("ターミナル 1")`).click();
await win('ターミナル 1').locator('.xterm-screen').click();
await page.keyboard.type('echo W-$((3+3))\n');
ok('開き直したシェルで動く', await waitFor(async () => (await winText('ターミナル 1')).includes('W-6')));
ok('終了したウィンドウはそのまま', (await win('top · 終了').count()) === 1);
await page.screenshot({ path: `${OUT}/reconnect-back.png` });

// 切断すると知らせる
await page.locator('header [title="asuta@127.0.0.1"]').hover();
await page.click('header button[title="切断"]');
ok('切断したと知らせる', await waitFor(async () => (await page.locator('[role=status]:has-text("testserver から切断しました")').count()) > 0));

cleanup();
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
