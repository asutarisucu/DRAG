// sudo モードを確かめる。ログインしたユーザーの権限で読めない・書けないときだけパスワードを聞き、
// ON の間は扱え、OFF に戻すとまた扱えなくなること。root でログインしたときは聞かないこと。
// smoke.mjs と同じく wails dev の画面を開いて操作する
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

// ホームに root の持ち物を置く。ファイルは書けず、フォルダの中には作れない。ファイル自体はホームの中なので消せる
const F = '/home/asuta/e2e-root.conf';
const RD = '/home/asuta/e2e-rootdir';
sh(`rm -rf ${F} ${RD}; echo a=1 > ${F}; mkdir ${RD} && echo x > ${RD}/inner.txt`, 'root');

const browser = await chromium.launch({ channel: 'chrome' });
page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
page.on('console', m => { if (m.type() === 'error' && !m.text().includes('404')) errors.push('console ' + m.text()); });
await page.goto('http://localhost:34115');
// 前のテストで開いたままの接続を閉じる。wails dev では画面を読み込み直しても Go の側の接続は残る
await page.evaluate(async () => { for (const s of await window.go.main.App.Servers()) await window.go.main.App.Disconnect(s.id); });
await page.evaluate(() => localStorage.removeItem('drag-settings'));
await page.reload();

// user で testserver につなぐ。user を変えると別の保存済みのサーバーになる
const connect = async user => {
  await page.click('section button:has-text("testserver")');
  const dlg = page.locator('[role=dialog]');
  await dlg.locator('label:has-text("ユーザー") input').fill(user);
  if (user !== 'asuta') await dlg.locator('label:has-text("名前") input').fill(`testserver-${user}`);
  await dlg.locator('input[type=password]').fill('drag');
  await dlg.locator('button:has-text("接続")').click();
  await page.waitForSelector('[role=dialog] button:has-text("開く")', { timeout: 10000 });
  await page.click('[role=dialog] button:has-text("開く")');
  await page.waitForSelector('.tree-row');
};
await page.waitForSelector('text=サーバーに接続');
await connect('asuta');

const row = name => page.locator(`.tree-row:has(> span:text-is("${name}"))`).first();
const top = () => page.$eval('[data-tree-top]', e => e.dataset.treeTop);
const sudoBtn = page.locator('[data-sudo]');
const dialog = page.locator('[role=dialog]:has-text("sudo モードを ON にする")');
const waitFor = async (fn, ms = 8000) => { for (let t = 0; t < ms; t += 200) { if (await fn().catch(() => false)) return true; await page.waitForTimeout(200); } return false; };
const lock = name => row(name).locator('[title*="書き込む権限がありません"]');

ok('はじめは OFF でホームを見ている', await sudoBtn.getAttribute('data-sudo') === 'off' && await top() === '/home/asuta');
await row('e2e-root.conf').waitFor();
ok('書けないものに鍵が付く', await lock('e2e-root.conf').count() === 1 && await lock('e2e-rootdir').count() === 1 && await lock('.bashrc').count() === 0);

// 読める場所へは聞かずに移る
await page.click('button[title="上の階層へ"]');
ok('読めるならホームの外へも聞かずに上がる', await waitFor(async () => await top() === '/home') && await dialog.count() === 0);
await page.click('button[title="上の階層へ"]');
ok('/ へも聞かずに上がる', await waitFor(async () => await top() === '/') && await dialog.count() === 0);

// 読めないフォルダを開くとパスワードを聞く。やめると開かない
await row('root').click();
await dialog.waitFor({ timeout: 3000 });
ok('読めないフォルダを開くとパスワードを聞く', (await dialog.textContent()).includes('/root を開くには'));
await dialog.locator('button:has-text("キャンセル")').click();
ok('やめると開けないと出る', await waitFor(async () => (await page.locator('aside').textContent()).includes('開けません')));

// 違うパスワードでは ON にならない
await row('root').click();
await row('root').click();
await dialog.waitFor();
await dialog.locator('input[type=password]').fill('wrong');
await dialog.locator('button:has-text("ON にする")').click();
ok('違うパスワードは窓に出る', await waitFor(async () => (await dialog.textContent()).includes('sudo のパスワードが違います')), (await dialog.textContent()).slice(-60));
ok('違うパスワードでは OFF のまま', await sudoBtn.getAttribute('data-sudo') === 'off');
await dialog.locator('input[type=password]').fill('drag');
await dialog.locator('button:has-text("ON にする")').click();
ok('正しいパスワードで ON になり、/root の中が見える', await waitFor(async () => await sudoBtn.getAttribute('data-sudo') === 'on' && (await page.locator('.tree-row[data-path^="/root/"]').count()) > 0));
await page.screenshot({ path: `${OUT}/sudo-on.png` });

// OFF に戻すと読めなくなる
await sudoBtn.click();
ok('OFF に戻すと /root はまた読めない', await waitFor(async () => await sudoBtn.getAttribute('data-sudo') === 'off' && (await page.locator('.tree-row[data-path^="/root/"]').count()) === 0));
await page.click('button[title="ホームへ"]');
await waitFor(async () => await top() === '/home/asuta');

// 自分の権限で消せるものは聞かずに消す。root のファイルでも、ホームの中なら消せる
await row('e2e-root.conf').click({ button: 'right' });
await page.click('[role=menu] >> text=削除');
ok('自分の権限で消せるものは聞かない', await waitFor(async () => sh(`test -e ${F} && echo yes || echo no`) === 'no') && await dialog.count() === 0);
await page.locator('[role=status] button:has-text("元に戻す")').click();
ok('元に戻せる', await waitFor(async () => sh(`cat ${F}`) === 'a=1'));

// 権限で失敗したときだけ聞く。root のフォルダの中には作れない
await row('e2e-rootdir').click({ button: 'right' });
await page.click('[role=menu] >> text=新しいファイル');
await dialog.waitFor({ timeout: 3000 });
ok('書けないフォルダに作ろうとすると聞く', (await dialog.textContent()).includes('~/e2e-rootdir に作るには'));
await dialog.locator('button:has-text("キャンセル")').click();
ok('やめると作らない', await waitFor(async () => (await page.locator('[role=status]').textContent()).includes('権限がありません')) && sh(`ls ${RD}`) === 'inner.txt');

// エディターは読み取り専用で開き、ON にすると保存できる
await row('e2e-root.conf').dblclick();
const ta = page.locator('[data-win] textarea');
await ta.waitFor();
await waitFor(async () => (await ta.inputValue()) === 'a=1\n');
ok('書けないファイルは読み取り専用で開く', await ta.getAttribute('readonly') !== null && await page.locator('[data-win] button:has-text("読み取り専用")').count() === 1);
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
await page.click('[data-win] button[title="閉じる"]');

// root でログインしたときは、どこへ行っても聞かない
await page.evaluate(async () => { for (const s of await window.go.main.App.Servers()) await window.go.main.App.Disconnect(s.id); });
await page.reload();
await page.waitForSelector('text=サーバーに接続');
await connect('root');
ok('root はホームが /root', await top() === '/root');
await page.click('button[title="上の階層へ"]');
ok('root は聞かずに / へ上がる', await waitFor(async () => await top() === '/') && await dialog.count() === 0);
await row('etc').click();
await row('hostname').waitFor();
ok('root には鍵を付けない', await lock('hostname').count() === 0);
await row('home').click();
await row('asuta').click();
ok('root はほかの利用者のホームも聞かずに開く', await waitFor(async () => (await page.locator('.tree-row[data-path="/home/asuta/e2e-rootdir"]').count()) > 0) && await dialog.count() === 0);
ok('root のあいだ sudo は OFF のまま', await sudoBtn.getAttribute('data-sudo') === 'off');

// あと片付け。このテストで保存した、検証用のサーバーの root のものだけを消す
await page.evaluate(async () => { for (const s of await window.go.main.App.SavedServers()) if (s.user === 'root' && s.host === '127.0.0.1' && s.port === 2222) await window.go.main.App.RemoveSavedServer(s.id); });
sh(`rm -rf ${F} ${RD}`, 'root');
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
