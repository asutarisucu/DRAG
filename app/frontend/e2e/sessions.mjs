// 左の欄のセッションの一覧と、ウィンドウからターミナルへ戻す操作を確かめる。smoke.mjs と同じく wails dev の画面を開いて操作する
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let page;
// 途中で止まっても、そこまでの結果と画面を残す
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await page?.screenshot({ path: path.join(import.meta.dirname, 'out', 'sessions-failed.png') }).catch(() => {});
  process.exit(1);
});
sh('screen -S e2e-s -X quit; tmux kill-session -t e2e-t; true');
sh('screen -dmS e2e-s; tmux new -d -s e2e-t');

const browser = await chromium.launch({ channel: 'chrome' });
page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
page.on('console', m => { if (m.type() === 'error' && !m.text().includes('404')) errors.push('console ' + m.text()); });
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

const panel = '[data-terminal]';
const panelText = () => page.$eval(`${panel} .xterm-rows`, e => [...e.children].map(r => r.textContent).join('\n'));
const waitFor = async (fn, ms = 8000) => { for (let t = 0; t < ms; t += 200) { if (await fn()) return true; await page.waitForTimeout(200); } return false; };
await waitFor(async () => (await panelText()).includes('$'));

// 一覧
await page.click('button[title="セッション"]');
await page.waitForSelector('[data-session^="tmux:e2e-t"]', { timeout: 8000 }).catch(() => {});
const screenItem = page.locator('[data-session*=".e2e-s"]');
ok('screen と tmux が並ぶ', await screenItem.count() === 1 && await page.locator('[data-session="tmux:e2e-t"]').count() === 1,
  (await page.locator('aside').textContent()).replace(/\s+/g, ' ').slice(0, 160));

// クリックでウィンドウに開き、閉じるとデタッチ
await page.click('[data-session="tmux:e2e-t"]');
await page.waitForSelector('[data-win]', { timeout: 5000 }).catch(() => {});
const tmuxWin = await page.$eval('[data-win]', e => e.dataset.win).catch(() => '');
ok('クリックでウィンドウに開く', !!tmuxWin);
ok('一覧にウィンドウで開いていると出る', await waitFor(async () => (await page.textContent('[data-session="tmux:e2e-t"]')).includes('ウィンドウ'), 6000));
await page.waitForTimeout(1000);
await page.screenshot({ path: `${OUT}/sessions-tmux.png` });
await page.click(`[data-win="${tmuxWin}"] button[title="閉じる"]`);
await page.waitForSelector('[role=status]:has-text("からデタッチしました。")', { timeout: 5000 }).then(() => ok('閉じるとデタッチ', true), () => ok('閉じるとデタッチ', false));
ok('tmux のセッションは残る', sh('tmux ls').startsWith('e2e-t:'), sh('tmux ls'));

// ターミナルへドラッグすると、自動でウィンドウへ移さずにターミナルで開く
await screenItem.dragTo(page.locator(panel));
await page.waitForTimeout(1500);
const chip = await page.locator(`${panel} [draggable=true]`).textContent().catch(() => '');
ok('ターミナルへドラッグするとそこで開く', chip.includes('screen -x') && (await page.$$('[data-win]')).length === 0, chip);
ok('一覧にターミナルで開いていると出る', await waitFor(async () => (await screenItem.textContent()).includes('ターミナル'), 6000));
await page.click(`${panel} .xterm-screen`);
await page.keyboard.press('Control+a');
await page.keyboard.press('d');
ok('デタッチするとチップが消える', await waitFor(async () => (await page.locator(`${panel} [draggable=true]`).count()) === 0));

// デスクトップへドラッグするとその位置に開く
const desk = await page.$eval('[data-desktop]', e => { const r = e.getBoundingClientRect(); return { x: r.x, y: r.y }; });
await screenItem.dragTo(page.locator('[data-desktop]'), { targetPosition: { x: 400, y: 200 } });
await page.waitForSelector('[data-win]', { timeout: 5000 }).catch(() => {});
const scrRect = await page.$eval('[data-win]', e => { const r = e.getBoundingClientRect(); return { x: Math.round(r.x), y: Math.round(r.y) }; }).catch(() => null);
ok('デスクトップへドラッグするとその位置に開く', !!scrRect && Math.abs(scrRect.x - (desk.x + 360)) <= 2 && Math.abs(scrRect.y - (desk.y + 184)) <= 2, JSON.stringify(scrRect));
await page.click('[data-win] button[title="閉じる"]');
await page.waitForTimeout(500);

// 自動で移した ssh を、ウィンドウのバーをターミナルへドラッグして戻す
await page.click(`${panel} .xterm-screen`);
await page.keyboard.type('ssh -o StrictHostKeyChecking=no asuta@localhost\n');
await page.waitForSelector('[data-win]', { timeout: 5000 }).catch(() => {});
const sshWin = await page.$eval('[data-win]', e => e.dataset.win).catch(() => '');
await page.waitForTimeout(1200);
const bar = await page.locator(`[data-win="${sshWin}"] > div`).first().boundingBox();
const tp = await page.locator(panel).boundingBox();
await page.mouse.move(bar.x + 120, bar.y + 15);
await page.mouse.down();
await page.mouse.move(tp.x + 300, tp.y + 120, { steps: 10 });
const label = await page.locator(panel).textContent();
await page.screenshot({ path: `${OUT}/sessions-return.png` });
await page.mouse.up();
await page.waitForTimeout(800);
ok('ターミナルの上でここで離すと戻すと出る', label.includes('ここで離すとターミナルに戻す'));
const back = await panelText();
ok('ウィンドウからターミナルに戻す', (await page.$$('[data-win]')).length === 0 && back.includes('password:') && back.includes('をターミナルに戻しました'), back.split('\n').filter(Boolean).slice(-3).join(' | '));
await page.click(`${panel} .xterm-screen`);
await page.keyboard.type('drag\n');
await waitFor(async () => (await panelText()).includes('Last login'));
await page.keyboard.type('echo BACK-$((5+5))\n');
ok('戻したセッションで続けて打てる', await waitFor(async () => (await panelText()).includes('BACK-10')));
await page.keyboard.type('exit\n');

sh('screen -S e2e-s -X quit; tmux kill-session -t e2e-t; true');
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
