// ファイルツリーの右クリックのメニューとキー操作を確かめる。smoke.mjs と同じく wails dev の画面を開いて操作する
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh as serverSh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
const sh = cmd => serverSh(cmd, 'root');

const DIR = 'e2e テスト';
sh(`rm -rf "/home/asuta/${DIR}" /home/asuta/.local/share/Trash`);

const browser = await chromium.launch({ channel: 'chrome' });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
page.on('console', m => { if (m.type() === 'error' && !m.text().includes('404')) errors.push('console ' + m.text()); });
await page.goto('http://localhost:34115');
await page.waitForSelector('text=サーバーに接続');

// 保存済みの testserver につなぐ
await page.click('section button:has-text("testserver")');
await page.fill('[role=dialog] input[type=password]', 'drag');
await page.click('[role=dialog] button:has-text("接続")');
await page.waitForSelector('[role=dialog] button:has-text("開く")', { timeout: 10000 });
await page.click('[role=dialog] button:has-text("開く")');
await page.waitForSelector('.tree-row');

const row = name => page.locator(`.tree-row:has(> span:text-is("${name}"))`).first();
const menu = async (target, label) => {
  await target.click({ button: 'right' });
  await page.click(`[role=menu] button:has-text("${label}")`);
};
const renameTo = async name => {
  const input = page.locator('.tree-row input');
  await input.waitFor();
  await input.fill(name);
  await input.press('Enter');
};
const toastText = async () => (await page.locator('[role=status]').last().textContent()) ?? '';

// 新しいフォルダーを作って名前を付ける
await menu(page.locator('aside .font-mono').first(), '新しいフォルダー');
await renameTo(DIR);
await row(DIR).waitFor();
ok('新しいフォルダー', sh(`ls -d "/home/asuta/${DIR}"`) === `/home/asuta/${DIR}`);

// その中に、引用符を含む名前のファイルを作る
await menu(row(DIR), '新しいファイル');
await renameTo("it's.txt");
await row("it's.txt").waitFor();
ok('新しいファイルと名前の変更', sh(`ls "/home/asuta/${DIR}"`) === "it's.txt");

// コピーして同じフォルダに貼り付ける
await menu(row("it's.txt"), 'コピー');
await menu(row(DIR), '貼り付け');
await row("it's - コピー.txt").waitFor({ timeout: 5000 }).catch(() => {});
ok('コピーと貼り付け', sh(`ls "/home/asuta/${DIR}"`).split('\n').includes("it's - コピー.txt"));

// F2 で名前を変える
await row("it's - コピー.txt").click();
await page.keyboard.press('F2');
await renameTo('renamed.txt');
await row('renamed.txt').waitFor({ timeout: 5000 }).catch(() => {});
ok('F2 で名前を変える', sh(`ls "/home/asuta/${DIR}"`).split('\n').includes('renamed.txt'));

// Delete でゴミ箱へ移し、元に戻す
await row('renamed.txt').click();
await page.keyboard.press('Delete');
await page.waitForSelector('[role=status]:has-text("ゴミ箱へ移しました")');
const trashed = sh(`ls "/home/asuta/${DIR}"; ls /home/asuta/.local/share/Trash/files`);
ok('Delete でゴミ箱へ移す', !trashed.split('\n').slice(0, 1).includes('renamed.txt') && trashed.includes('renamed.txt'), trashed.replace(/\n/g, ' / '));
await page.screenshot({ path: `${OUT}/files-trashed.png` });
await page.click('[role=status] button:has-text("元に戻す")');
await row('renamed.txt').waitFor({ timeout: 5000 }).catch(() => {});
ok('元に戻す', sh(`ls "/home/asuta/${DIR}"`).split('\n').includes('renamed.txt') && sh('ls /home/asuta/.local/share/Trash/files | wc -l') === '0');

// 切り取ってホームへ貼り付ける
await row('renamed.txt').click();
await page.keyboard.press('Control+x');
await page.locator('aside .font-mono').first().click();
await page.keyboard.press('Control+v');
await page.waitForSelector('[role=status]:has-text("移動しました")', { timeout: 5000 }).catch(() => {});
ok('切り取りと貼り付け', sh('ls /home/asuta/renamed.txt') === '/home/asuta/renamed.txt', await toastText());

// ターミナルで cd
await menu(row(DIR), 'ターミナルで cd');
await page.waitForFunction(n => document.querySelector('.xterm-rows').textContent.includes(`~/${n}$`), DIR, { timeout: 5000 }).catch(() => {});
const term = await page.$eval('.xterm-rows', e => e.textContent);
ok('ターミナルで cd', term.includes(`cd '/home/asuta/${DIR}'`) && term.includes(`~/${DIR}$`));

// コマンドが動いている間は送らない
await page.click('.xterm-screen');
await page.keyboard.type('sleep 3\n');
await page.waitForTimeout(400);
await menu(row(DIR), 'ターミナルで cd');
await page.waitForSelector('[role=status]:has-text("コマンドが動いています")', { timeout: 3000 }).then(() => ok('動いている間は送らない', true), () => ok('動いている間は送らない', false));

// パスをコピー
await menu(row('renamed.txt'), 'パスをコピー');
await page.waitForSelector('[role=status]:has-text("パスをコピーしました")', { timeout: 3000 }).then(() => ok('パスをコピー', true), () => ok('パスをコピー', false));
await page.screenshot({ path: `${OUT}/files-menu.png` });
await row(DIR).click({ button: 'right' });
await page.screenshot({ path: `${OUT}/files-contextmenu.png` });
await page.keyboard.press('Escape');

sh(`rm -rf "/home/asuta/${DIR}" /home/asuta/renamed.txt /home/asuta/.local/share/Trash`);
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
