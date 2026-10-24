// デスクトップのウィンドウと、ターミナルからの切り出しを確かめる。smoke.mjs と同じく wails dev の画面を開いて操作する
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
sh('screen -S e2e -X quit; true');

const browser = await chromium.launch({ channel: 'chrome' });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
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

const panel = 'section:has(> div > span:text-is("ターミナル"))';
const panelText = () => page.$eval(`${panel} .xterm-rows`, e => [...e.children].map(r => r.textContent).join('\n'));
const winText = id => page.$eval(`[data-win="${id}"] .xterm-rows`, e => [...e.children].map(r => r.textContent).join('\n'));
const typeIn = async (selector, text) => { await page.click(`${selector} .xterm-screen`); await page.keyboard.type(text); };
const waitText = async (get, s, ms = 8000) => {
  for (let t = 0; t < ms; t += 200) { if ((await get()).includes(s)) return true; await page.waitForTimeout(200); }
  return false;
};
const winIds = () => page.$$eval('[data-win]', es => es.map(e => e.dataset.win));
await waitText(panelText, '$');

// ssh は自動でウィンドウに移る。パスワードの入力からウィンドウで続ける
await typeIn(panel, 'ssh -o StrictHostKeyChecking=no asuta@localhost\n');
await page.waitForSelector('[data-win]', { timeout: 5000 }).catch(() => {});
let ids = await winIds();
ok('ssh が自動でウィンドウに移る', ids.length === 1, await page.$eval('[data-win]', e => e.textContent).catch(() => ''));
const sshWin = ids[0];
ok('ターミナルに移したことが出る', await waitText(panelText, 'をウィンドウに移しました'));
ok('ウィンドウにパスワードの入力が出る', await waitText(() => winText(sshWin), 'password:'), (await winText(sshWin)).split('\n').filter(Boolean).slice(0, 3).join(' | '));
await typeIn(`[data-win="${sshWin}"]`, 'drag\n');
await waitText(() => winText(sshWin), 'asuta@');
await page.keyboard.type('echo INWIN-$((3+4))\n');
ok('ウィンドウの中の ssh で打てる', await waitText(() => winText(sshWin), 'INWIN-7'));
await page.screenshot({ path: `${OUT}/desktop-ssh.png` });

// ターミナルには同じフォルダで新しいシェルが開いている
await typeIn(panel, 'echo PANEL-$((1+1))\n');
ok('ターミナルは新しいシェルで続く', await waitText(panelText, 'PANEL-2'));

// ssh を抜けると「終了」になる
await typeIn(`[data-win="${sshWin}"]`, 'exit\n');
ok('ssh を抜けると終了と出る', await waitText(() => page.$eval(`[data-win="${sshWin}"]`, e => e.textContent), '· 終了'));

// 自動で移さないコマンドは、見出しのチップをドラッグして移す
await typeIn(panel, 'sleep 30\n');
await page.waitForSelector(`${panel} [draggable=true]:has-text("sleep 30")`, { timeout: 5000 }).catch(() => {});
ok('前面のコマンドが見出しに出る', await page.isVisible(`${panel} [draggable=true]:has-text("sleep 30")`));
await page.dragAndDrop(`${panel} [draggable=true]`, '[data-desktop]', { targetPosition: { x: 700, y: 120 } });
await page.waitForTimeout(800);
ids = await winIds();
const sleepWin = ids.find(i => i !== sshWin);
ok('チップをデスクトップへドラッグすると移る', !!sleepWin && (await page.$eval(`[data-win="${sleepWin}"]`, e => e.textContent)).includes('sleep 30'));
await typeIn(`[data-win="${sleepWin}"]`, '\x03');
await page.keyboard.press('Control+c');
ok('Ctrl+C で終了と出る', await waitText(() => page.$eval(`[data-win="${sleepWin}"]`, e => e.textContent), '· 終了'));

// screen は閉じるとデタッチになり、セッションは残る
await typeIn(panel, 'screen -S e2e\n');
await page.waitForFunction(n => document.querySelectorAll('[data-win]').length === n, 3, { timeout: 5000 }).catch(() => {});
ids = await winIds();
const scrWin = ids.find(i => i !== sshWin && i !== sleepWin);
ok('screen が自動でウィンドウに移る', !!scrWin);
await page.waitForTimeout(800);
await page.click(`[data-win="${scrWin}"] button[title="閉じる"]`);
await page.waitForSelector('[role=status]:has-text("からデタッチしました。")', { timeout: 5000 }).then(() => ok('閉じるとデタッチと出る', true), () => ok('閉じるとデタッチと出る', false));
await page.waitForTimeout(800);
const screens = sh('screen -ls');
ok('screen のセッションは残る', screens.includes('.e2e') && screens.includes('(Detached)'), screens.replace(/\s+/g, ' '));
sh('screen -S e2e -X quit');

// ウィンドウの操作
const rect = id => page.$eval(`[data-win="${id}"]`, e => { const r = e.getBoundingClientRect(); return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) }; });
const bar = id => page.locator(`[data-win="${id}"] > div`).first();
const desk = await page.$eval('[data-desktop]', e => { const r = e.getBoundingClientRect(); return { x: r.x, y: r.y, w: r.width, h: r.height }; });
const r0 = await rect(sshWin);
const b = await bar(sshWin).boundingBox();
await page.mouse.move(b.x + 100, b.y + 15);
await page.mouse.down();
await page.mouse.move(b.x + 160, b.y + 75, { steps: 6 });
await page.mouse.up();
const r1 = await rect(sshWin);
ok('バーをドラッグして動かす', r1.x - r0.x === 60 && r1.y - r0.y === 60, JSON.stringify([r0, r1]));
// 左の端へ寄せると半分
const b1 = await bar(sshWin).boundingBox();
await page.mouse.move(b1.x + 100, b1.y + 15);
await page.mouse.down();
await page.mouse.move(desk.x + 3, b1.y + 100, { steps: 8 });
await page.mouse.up();
await page.waitForTimeout(300);
const r2 = await rect(sshWin);
ok('左の端へ寄せると半分になる', Math.abs(r2.w - desk.w / 2) <= 2 && Math.abs(r2.h - desk.h) <= 2 && Math.abs(r2.x - desk.x) <= 2, JSON.stringify(r2));
await bar(sshWin).dblclick({ position: { x: 100, y: 15 } });
const r3 = await rect(sshWin);
ok('ダブルクリックで最大化', Math.abs(r3.w - desk.w) <= 2, JSON.stringify(r3));
await page.click(`[data-win="${sshWin}"] button[title="最小化"]`);
ok('最小化するとタスクバーに残る', !(await page.isVisible(`[data-win="${sshWin}"]`)) && await page.isVisible(`[data-task="${sshWin}"]`));
await page.click(`[data-task="${sshWin}"]`);
ok('タスクバーで元に戻る', await page.isVisible(`[data-win="${sshWin}"]`));

// タスクバーの並べ替え
const order = () => page.$$eval('[data-task]', es => es.map(e => e.dataset.task));
const before = await order();
const t0 = await page.locator(`[data-task="${before[0]}"]`).boundingBox();
const t1 = await page.locator(`[data-task="${before[1]}"]`).boundingBox();
await page.mouse.move(t0.x + 20, t0.y + 16);
await page.mouse.down();
await page.mouse.move(t1.x + t1.width - 5, t0.y + 16, { steps: 8 });
await page.mouse.up();
const after = await order();
ok('タスクバーをドラッグで並べ替える', after[0] === before[1] && after[1] === before[0], `${before} → ${after}`);

// 新しいターミナル
await page.click('button[title="新しいターミナル"]');
await page.waitForSelector('[data-win]:has-text("ターミナル 1")', { timeout: 5000 }).catch(() => {});
const termWin = (await page.$eval('[data-win]:has-text("ターミナル 1")', e => e.dataset.win).catch(() => ''));
ok('新しいターミナルのウィンドウ', !!termWin);
await page.waitForTimeout(800);
await typeIn(`[data-win="${termWin}"]`, 'exit\n');
await page.waitForSelector(`[data-win="${termWin}"]`, { state: 'detached', timeout: 5000 }).then(() => ok('exit で閉じる', true), () => ok('exit で閉じる', false));

// 自動の切り出しを切ると、見出しに出るだけになる
await page.click('button[title="設定"]');
await page.click('[role=dialog] button:has-text("操作")');
await page.click('[role=dialog] [role=switch] >> nth=0');
await page.keyboard.press('Escape');
const n0 = (await winIds()).length;
await typeIn(panel, 'ssh -o StrictHostKeyChecking=no asuta@localhost\n');
await page.waitForTimeout(1200);
ok('自動の切り出しを切ると移さない', (await winIds()).length === n0 && await page.isVisible(`${panel} [draggable=true]:has-text("ssh")`));
await page.keyboard.press('Control+c');
await page.screenshot({ path: `${OUT}/desktop-final.png` });

console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
