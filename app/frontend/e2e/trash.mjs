// ゴミ箱の窓、ツリーの見出しのボタン、ターミナルのクリアを確かめる。
// smoke.mjs と同じく wails dev の画面を開いて操作する
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const DIR = 'trash-e2e';
const TRASH = '/home/asuta/.local/share/Trash';
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let page;
const cleanup = () => sh(`rm -rf /home/asuta/${DIR} ${TRASH}/files/* ${TRASH}/info/*`, 'root');
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await page?.screenshot({ path: path.join(OUT, 'trash-failed.png') }).catch(() => {});
  cleanup();
  process.exit(1);
});
cleanup();
sh(`mkdir -p /home/asuta/${DIR}/sub && cd /home/asuta/${DIR} && echo a > a.txt && echo b > b.txt && touch sub/x`);

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
const waitFor = async (fn, ms = 8000) => { for (let t = 0; t < ms; t += 200) { if (await fn().catch(() => false)) return true; await page.waitForTimeout(200); } return false; };
const row = name => page.locator(`.tree-row:has(> span:text-is("${name}"))`).first();
const head = title => page.locator(`aside button[title="${title}"]`);

// 見出しのゴミ箱のボタンで、空のゴミ箱の窓が開く
await head('ゴミ箱').click();
const win = page.locator('[data-win]:has-text("ゴミ箱")').last();
await win.locator('.win-toolbar').waitFor();
ok('空のゴミ箱を開く', await waitFor(async () => (await win.textContent()).includes('ゴミ箱は空です') && (await win.textContent()).includes('0 件')));
ok('空なら空にするボタンは押せない', await win.locator('button:has-text("ゴミ箱を空にする")').isDisabled());

// ツリーで消すと、開いているゴミ箱の窓に出る
await row(DIR).click();
for (const name of ['a.txt', 'b.txt', 'sub']) {
  await row(name).click();
  await page.keyboard.press('Delete');
  await win.locator(`[data-trash]:has-text("${name}")`).first().waitFor({ timeout: 5000 }).catch(() => {});
  // 削除した時刻は秒までなので、並びを見るために秒をずらす
  await page.waitForTimeout(1100);
}
const names = () => win.locator('[data-trash] .dk-name > div:first-child').allTextContents();
ok('消したものが新しい順に出る', JSON.stringify(await names()) === JSON.stringify(['sub', 'b.txt', 'a.txt']), (await names()).join(', '));
const aRow = win.locator('[data-trash]:has-text("a.txt")');
ok('元の場所と消した時刻が出る', /~\/trash-e2e\/ · \d\d:\d\d に削除/.test(await aRow.textContent()), await aRow.textContent());
ok('件数が出る', (await win.locator('.win-toolbar').textContent()).includes('3 件'));
await page.screenshot({ path: `${OUT}/trash-window.png` });

// 元に戻すと、ツリーで選ばれる
await aRow.locator('button:has-text("元に戻す")').click();
ok('元に戻す', await waitFor(async () => sh(`ls /home/asuta/${DIR}`).split('\n').includes('a.txt') && !(await names()).includes('a.txt')));
ok('戻したものをツリーで選ぶ', await waitFor(async () => (await row('a.txt').getAttribute('class')).split(' ').includes('selected')));
ok('戻したと出る', (await win.locator('[role=status]').textContent()).includes('a.txt を元に戻しました'));

// 1つだけ完全に削除する
await win.locator('[data-trash]:has-text("b.txt") button:has-text("完全に削除")').click();
ok('完全に削除', await waitFor(async () => JSON.stringify(await names()) === '["sub"]') && !sh(`ls ${TRASH}/files ${TRASH}/info`).includes('b.txt'));

// 自分では消せないもの (sudo モードで消したフォルダ) は、sudo モードを聞いてから消す
sh(`mkdir -p ${TRASH}/files/rootdir/in && touch ${TRASH}/files/rootdir/in/f && printf '[Trash Info]\nPath=/etc/rootdir\nDeletionDate=2026-10-24T09:05:00\n' > ${TRASH}/info/rootdir.trashinfo && chmod 755 ${TRASH}/files/rootdir/in`, 'root');
// DRAG の外で変わったものは、窓を開き直すと読む
await win.locator('button[title="閉じる"]').click();
await head('ゴミ箱').click();
const rootRow = win.locator('[data-trash="rootdir"]');
ok('ほかで消したものも開き直すと出る', await waitFor(async () => (await rootRow.count()) === 1));
ok('前の日に消したものは日付も出る', (await rootRow.textContent()).includes('/etc/ · 10/24 09:05 に削除'), await rootRow.textContent());
await rootRow.locator('button:has-text("完全に削除")').click();
const dialog = page.locator('[role=dialog]:has-text("sudo モードを ON にする")');
await dialog.waitFor({ timeout: 5000 });
ok('消せなければ sudo モードを聞く', (await dialog.textContent()).includes('ゴミ箱の中のものを消すには'));
await dialog.locator('input[type=password]').fill('drag');
await dialog.locator('button:has-text("ON にする")').click();
ok('ON にすると消せる', await waitFor(async () => (await rootRow.count()) === 0) && !sh(`ls ${TRASH}/files`).includes('rootdir'));
await page.click('[data-sudo]');

// 空にするのは確かめてから
await win.locator('button:has-text("ゴミ箱を空にする")').click();
ok('空にする前に確かめる', (await win.locator('.win-toolbar').textContent()).includes('すべて完全に削除しますか'));
await win.locator('button:has-text("やめる")').click();
ok('やめると何もしない', (await names()).length === 1);
await win.locator('button:has-text("ゴミ箱を空にする")').click();
await win.locator('button:has-text("空にする"):not(:has-text("ゴミ箱"))').click();
ok('空にする', await waitFor(async () => (await win.textContent()).includes('ゴミ箱は空です')) && sh(`find ${TRASH}/files ${TRASH}/info -mindepth 1 | wc -l`) === '0');

// 見出しの新しいファイル・新しいフォルダーは、選んでいるフォルダに作る
await row(DIR).click();
await head('新しいファイル').click();
const input = page.locator('.tree-row input');
await input.waitFor();
await input.fill('made.txt');
await input.press('Enter');
ok('選んだフォルダに新しいファイル', await waitFor(async () => sh(`ls /home/asuta/${DIR}`).split('\n').includes('made.txt')));
await row('made.txt').click();
await head('新しいフォルダー').click();
await input.waitFor();
await input.fill('made-dir');
await input.press('Enter');
ok('ファイルを選んでいればそのフォルダに作る', await waitFor(async () => sh(`ls -d /home/asuta/${DIR}/made-dir`) === `/home/asuta/${DIR}/made-dir`));
ok('アップロードのボタンがある', await head('アップロード').isVisible());

// すべて閉じる
ok('開いたフォルダの中が出ている', (await page.locator('.tree-row[data-d="1"]').count()) > 0);
await head('すべて閉じる').click();
ok('すべて閉じる', await waitFor(async () => (await page.locator('.tree-row[data-d="1"]').count()) === 0));

// ターミナルのクリア
await page.click('[data-terminal] .xterm-screen');
await page.keyboard.type('echo CLEAR-$((1+1))\n');
const termText = () => page.$eval('[data-terminal] .xterm-rows', e => e.textContent);
await waitFor(async () => (await termText()).includes('CLEAR-2'));
await page.click('[data-terminal] button[title="クリア"]');
ok('ターミナルのクリア', await waitFor(async () => !(await termText()).includes('CLEAR-')));

cleanup();
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
