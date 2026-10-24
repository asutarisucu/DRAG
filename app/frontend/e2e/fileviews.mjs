// ファイルをデスクトップで開くウィンドウ (エディター、ログ、画像、圧縮ファイル) を確かめる。
// smoke.mjs と同じく wails dev の画面を開いて操作する
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
// 途中で止まっても、そこまでの結果を出す
process.on('uncaughtException', e => { console.log(results.join('\n')); console.log('止まった: ' + e.message.split('\n')[0]); process.exit(1); });

const D = '/home/asuta/e2e-views';
sh(`rm -rf ${D} && mkdir -p ${D}/pack && cd ${D} && echo a=1 > conf.properties && echo b=2 >> conf.properties`);
sh(`cd ${D} && for i in $(seq 1 4000); do echo "[12:00:00 INFO]: line $i"; done > server.log && echo '[12:00:01 WARN]: Can not keep up' >> server.log`);
// 2 × 2 の PNG
sh(`cd ${D} && echo iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAFklEQVR4nGP4z8DwnwEIGP4zMDAwAAAeDwL+7m9dPQAAAABJRU5ErkJggg== | base64 -d > dot.png`);
sh(`cd ${D}/pack && mkdir mc && echo 'motd=hello' > mc/server.properties && echo '[INFO]: Done (1.0s)!' > mc/latest.log && tar czf ../backup.tar.gz mc && cd .. && rm -rf pack`);

const browser = await chromium.launch({ channel: 'chrome' });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
page.on('console', m => { if (m.type() === 'error' && !m.text().includes('404')) errors.push('console ' + m.text()); });
await page.goto('http://localhost:34115');
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
const win = title => page.locator(`[data-win]:has(span:text-is("${title}"))`).first();
const waitFor = async (fn, ms = 8000) => { for (let t = 0; t < ms; t += 200) { if (await fn().catch(() => false)) return true; await page.waitForTimeout(200); } return false; };
await row('e2e-views').click();
await row('conf.properties').waitFor();

// エディター。ダブルクリックで開き、書き換えて Ctrl+S
await row('conf.properties').dblclick();
await page.waitForSelector('[data-win] textarea');
const ta = page.locator('[data-win] textarea');
await waitFor(async () => (await ta.inputValue()) === 'a=1\nb=2\n');
ok('ダブルクリックでエディターに開く', (await ta.inputValue()) === 'a=1\nb=2\n');
await ta.click();
await page.keyboard.press('Control+End');
await page.keyboard.type('c=3');
ok('変えると題名に ● が付く', await waitFor(async () => (await page.locator('[data-win] span:has-text("● conf.properties")').count()) > 0));
await page.keyboard.press('Control+s');
ok('Ctrl+S で保存する', await waitFor(async () => sh(`cat ${D}/conf.properties`) === 'a=1\nb=2\nc=3'));
ok('保存すると ● が消える', await waitFor(async () => (await page.locator('[data-win] span:text-is("conf.properties")').count()) > 0));
// サーバーで書き換えられたあとに保存すると、上書きするかを聞く
sh(`sleep 1; echo 'other=1' > ${D}/conf.properties`);
await ta.click();
await page.keyboard.type('\nd=4');
await page.keyboard.press('Control+s');
const askedChanged = await page.waitForSelector('[role=dialog]:has-text("サーバーで書き換えられています")', { timeout: 5000 }).then(() => true, () => false);
ok('サーバーで変わっていたら上書きするかを聞く', askedChanged);
if (askedChanged) await page.click('[role=dialog] button:has-text("上書きする")');
ok('上書きする', await waitFor(async () => sh(`cat ${D}/conf.properties`).endsWith('d=4')));
// 保存していない変更があれば、閉じる前に聞く
await ta.click();
await page.keyboard.type('\ne=5');
await page.click('[data-win] button[title="閉じる"]');
const askedUnsaved = await page.waitForSelector('[role=dialog]:has-text("保存していない変更があります")', { timeout: 3000 }).then(() => true, () => false);
ok('保存していなければ閉じる前に聞く', askedUnsaved);
if (askedUnsaved) await page.click('[role=dialog] button:has-text("保存せずに閉じる")');
ok('保存せずに閉じる', await waitFor(async () => (await page.$$('[data-win]')).length === 0) && !sh(`cat ${D}/conf.properties`).includes('e=5'));

// ログ。メニューの「デスクトップで開く」で開き、末尾から出て、追記を追いかける
await row('server.log').click({ button: 'right' });
await page.click('[role=menu] button:has-text("デスクトップで開く")');
await page.waitForSelector('[data-log] div');
const logText = () => page.$eval('[data-log]', e => e.textContent);
ok('ログを末尾から出す', await waitFor(async () => (await logText()).includes('Can not keep up') && !(await logText()).includes('line 1]') ));
sh(`echo '[12:00:02 ERROR]: Something broke' >> ${D}/server.log`);
ok('追記を追いかける', await waitFor(async () => (await logText()).includes('Something broke')));
const errColor = await page.$eval('[data-log]', e => [...e.querySelectorAll('span')].find(s => s.textContent.includes('Something broke'))?.className ?? '');
ok('エラーの行に色が付く', errColor.includes('text-err'), errColor);
// 上へスクロールすると前を読み足す
const before = await page.$eval('[data-log]', e => e.children.length);
for (let i = 0; i < 6; i++) {
  await page.$eval('[data-log]', e => { e.scrollTop = 0; e.dispatchEvent(new Event('scroll')); });
  await page.waitForTimeout(400);
}
const after = await page.$eval('[data-log]', e => e.children.length);
ok('上へスクロールすると前を読み足す', after > before && (await logText()).includes('line 1['.replace('[', '')), `${before} → ${after}`);
await page.fill('[data-win] input[placeholder="絞り込み"]', 'WARN');
const filtered = await page.$eval('[data-log]', e => e.children.length);
ok('絞り込み', filtered === 1, String(filtered));
await page.screenshot({ path: `${OUT}/fileviews-log.png` });
await page.click('[data-win] button[title="閉じる"]');

// 画像
await row('dot.png').dblclick();
await page.waitForSelector('[data-win] img');
ok('画像を出す', await waitFor(async () => (await page.textContent('[data-win]')).includes('2 × 2')), await page.textContent('[data-win]'));
await page.click('[data-win] button[title="閉じる"]');

// 圧縮ファイル。サーバーには展開せずに中を見る
await row('backup.tar.gz').click({ button: 'right' });
await page.click('[role=menu] button:has-text("解凍して閲覧")');
await page.waitForSelector('[data-entry="mc/server.properties"]', { timeout: 10000 }).catch(() => {});
ok('圧縮ファイルの中の一覧', await page.isVisible('[data-entry="mc/server.properties"]') && await page.isVisible('[data-entry="mc/latest.log"]'));
await page.click('[data-entry="mc/server.properties"]');
ok('中のファイルを開く', await waitFor(async () => (await page.textContent('[data-win] pre')).includes('motd=hello')));
ok('サーバーには展開しない', sh(`ls ${D}`).split('\n').sort().join(' ') === 'backup.tar.gz conf.properties dot.png server.log', sh(`ls ${D}`).replace(/\n/g, ' '));
await page.screenshot({ path: `${OUT}/fileviews-archive.png` });
await page.click('[data-win] button[title="閉じる"]');

// ツリーからドラッグする。ファイルはデスクトップで開き、フォルダはターミナルで cd
await row('conf.properties').dragTo(page.locator('[data-desktop]'), { targetPosition: { x: 300, y: 150 } });
ok('ファイルをデスクトップへドラッグすると開く', await waitFor(async () => (await page.$$('[data-win] textarea')).length === 1));
await page.click('[data-win] button[title="閉じる"]');
await row('e2e-views').dragTo(page.locator('[data-terminal]'));
ok('フォルダをターミナルへドラッグすると cd', await waitFor(async () => (await page.$eval('[data-terminal] .xterm-rows', e => e.textContent)).includes('~/e2e-views$')));

sh(`rm -rf ${D}`);
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
