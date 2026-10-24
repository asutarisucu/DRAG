// sudo モードを確かめる。ホームの外へ上がるとき、root のファイルを変えるときにパスワードを聞き、
// ON の間は扱え、OFF に戻すとホームに戻ること。smoke.mjs と同じく wails dev の画面を開いて操作する
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
  await page?.screenshot({ path: path.join(OUT, 'sudo-failed.png') }).catch(() => {});
  process.exit(1);
});

const F = '/home/asuta/e2e-root.conf';
sh(`rm -f ${F}; echo a=1 > ${F}`, 'root');

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

const row = name => page.locator(`.tree-row:has(> span:text-is("${name}"))`).first();
const top = () => page.$eval('[data-tree-top]', e => e.dataset.treeTop);
const sudoBtn = page.locator('[data-sudo]');
const dialog = page.locator('[role=dialog]:has-text("sudo モードを ON にする")');
const waitFor = async (fn, ms = 8000) => { for (let t = 0; t < ms; t += 200) { if (await fn().catch(() => false)) return true; await page.waitForTimeout(200); } return false; };

ok('はじめは OFF でホームを見ている', await sudoBtn.getAttribute('data-sudo') === 'off' && await top() === '/home/asuta');
await row('e2e-root.conf').waitFor();
ok('root のファイルに鍵が付く', await row('e2e-root.conf').locator('[title*="root のファイル"]').count() === 1);

// ホームの外へ上がるとパスワードを聞く。やめるとそのまま
await page.click('button[title="上の階層へ"]');
await dialog.waitFor({ timeout: 3000 });
ok('ホームの外へ上がるとパスワードを聞く', (await dialog.textContent()).includes('ホームの外を開くには'));
await dialog.locator('button:has-text("キャンセル")').click();
ok('やめるとホームのまま', await dialog.count() === 0 && await top() === '/home/asuta');

// 違うパスワードでは ON にならない
await page.click('button[title="上の階層へ"]');
await dialog.waitFor();
await dialog.locator('input[type=password]').fill('wrong');
await dialog.locator('button:has-text("ON にする")').click();
ok('違うパスワードは窓に出る', await waitFor(async () => (await dialog.textContent()).includes('sudo のパスワードが違います')), (await dialog.textContent()).slice(-60));
ok('違うパスワードでは OFF のまま', await sudoBtn.getAttribute('data-sudo') === 'off');
await dialog.locator('input[type=password]').fill('drag');
await dialog.locator('button:has-text("ON にする")').click();
ok('正しいパスワードで ON になり、上の階層へ移る', await waitFor(async () => await sudoBtn.getAttribute('data-sudo') === 'on' && await top() === '/home'));
await page.screenshot({ path: `${OUT}/sudo-on.png` });

// ON の間はホームの外も開ける
await page.click('button[title="上の階層へ"]');
ok('ON なら聞かずに / へ上がる', await waitFor(async () => await top() === '/') && await dialog.count() === 0);
await row('root').click();
ok('/root の中が見える', await waitFor(async () => (await page.locator('.tree-row[data-path^="/root/"]').count()) > 0));

// OFF に戻すとホームに戻る
await sudoBtn.click();
ok('OFF に戻すとホームに戻る', await waitFor(async () => await sudoBtn.getAttribute('data-sudo') === 'off' && await top() === '/home/asuta'));

// OFF の間は root のファイルを消す前に聞く
await row('e2e-root.conf').click({ button: 'right' });
await page.click('text=削除');
await dialog.waitFor({ timeout: 3000 });
ok('root のファイルを消す前に聞く', (await dialog.textContent()).includes('e2e-root.conf を削除するには'));
await dialog.locator('button:has-text("キャンセル")').click();
ok('やめると消えない', sh(`ls ${F}`) === F);

// エディターは読み取り専用で開き、ON にすると保存できる
await row('e2e-root.conf').dblclick();
const ta = page.locator('[data-win] textarea');
await ta.waitFor();
await waitFor(async () => (await ta.inputValue()) === 'a=1\n');
ok('OFF なら読み取り専用で開く', await ta.getAttribute('readonly') !== null && await page.locator('[data-win] button:has-text("読み取り専用")').count() === 1);
await page.click('[data-win] button:has-text("読み取り専用")');
await dialog.waitFor();
await dialog.locator('input[type=password]').fill('drag');
await page.keyboard.press('Enter');
ok('ON にすると書けるようになる', await waitFor(async () => await ta.getAttribute('readonly') === null));
await ta.click();
await page.keyboard.press('Control+End');
await page.keyboard.type('b=2');
await page.keyboard.press('Control+s');
ok('root のファイルを保存できる', await waitFor(async () => sh(`cat ${F}`) === 'a=1\nb=2'), sh(`cat ${F}`));
ok('持ち主は root のまま', sh(`stat -c %U ${F}`) === 'root');

await sudoBtn.click();
ok('OFF に戻すとエディターは読み取り専用に戻る', await waitFor(async () => await ta.getAttribute('readonly') !== null));

sh(`rm -f ${F}`, 'root');
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
