// PC のファイルのドロップでのアップロードと、同じ名前のときの確認、プロパティを確かめる。
// ブラウザーではファイルのパスを取れないので、Wails がドロップで出す "wails:file-drop" の知らせを画面の中から送る
import { chromium } from 'playwright-core';
import path from 'node:path';
import fs from 'node:fs';
import os from 'node:os';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];

// PC の側に送るものを作る。フォルダの中に 3 MB のファイルを置き、進み具合が出るようにする
const local = fs.mkdtempSync(path.join(os.tmpdir(), 'drag-e2e-'));
fs.writeFileSync(path.join(local, 'メモ.txt'), 'hello\n');
fs.mkdirSync(path.join(local, 'data'));
fs.writeFileSync(path.join(local, 'data', 'big.bin'), Buffer.alloc(3 * 1024 * 1024, 7));
const DIR = 'e2e-upload';
sh(`rm -rf /home/asuta/${DIR} && mkdir /home/asuta/${DIR}`);

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
// 行の真ん中にドロップしたことにする
const drop = async (target, paths) => {
  const b = await target.boundingBox();
  await page.evaluate(([x, y, paths]) => window.runtime.EventsEmit('wails:file-drop', x, y, paths), [b.x + b.width / 2, b.y + b.height / 2, paths]);
};

await drop(row(DIR), [path.join(local, 'メモ.txt'), path.join(local, 'data')]);
const seen = new Set();
for (let i = 0; i < 30 && ![...seen].some(t => t.includes('アップロードしました')); i++) {
  for (const t of await page.locator('[role=status]').allTextContents()) seen.add(t);
  await page.waitForTimeout(500);
}
const found = sh(`cd /home/asuta/${DIR} && find . | sort`).split('\n').join(' ');
ok('フォルダの行に落とすとそのフォルダへ送る', found === '. ./data ./data/big.bin ./メモ.txt', found);
ok('大きさが合う', sh(`stat -c %s /home/asuta/${DIR}/data/big.bin`) === String(3 * 1024 * 1024));
ok('送り終えたら知らせる', [...seen].some(t => t.includes('2 件を ~/e2e-upload/ にアップロードしました')), [...seen].join(' / '));
await page.screenshot({ path: `${OUT}/upload-after.png` });

// 同じものをファイルの行に落とすと、そのファイルのあるフォルダへ送ろうとして、同じ名前を聞かれる。送ったあとはフォルダが開いている
await row('メモ.txt').waitFor({ timeout: 5000 });
await drop(row('メモ.txt'), [path.join(local, 'メモ.txt')]);
const asked = await page.waitForSelector('[role=dialog]:has-text("同じ名前があります")', { timeout: 5000 }).then(() => true, () => false);
ok('同じ名前があると聞く', asked);
await page.screenshot({ path: `${OUT}/upload-conflict.png` });
await page.click('[role=dialog] button:has-text("両方残す")');
await row('メモ (2).txt').waitFor({ timeout: 5000 }).catch(() => {});
ok('両方残す', sh(`ls /home/asuta/${DIR}`).split('\n').includes('メモ (2).txt'));

// 設定で「上書き」にすると聞かずに上書きする
fs.writeFileSync(path.join(local, 'メモ.txt'), 'changed\n');
await page.evaluate(() => {
  const s = JSON.parse(localStorage.getItem('drag-settings') ?? '{}');
  localStorage.setItem('drag-settings', JSON.stringify({ ...s, uploadConflict: 'overwrite' }));
});
await page.click('button[title="設定"]');
await page.click('[role=dialog] button:has-text("操作")');
await page.selectOption('[role=dialog] select', 'overwrite');
await page.keyboard.press('Escape');
await drop(row(DIR), [path.join(local, 'メモ.txt')]);
await page.waitForTimeout(1500);
ok('設定が上書きなら聞かずに上書きする', !(await page.isVisible('[role=dialog]')) && sh(`cat /home/asuta/${DIR}/メモ.txt`) === 'changed');

// プロパティ
await row('data').click({ button: 'right' });
await page.click('[role=menu] button:has-text("プロパティ")');
await page.waitForSelector('[role=dialog]:has-text("プロパティ")');
const props = (await page.textContent('[role=dialog]')).replace(/\s+/g, ' ');
ok('プロパティ', props.includes('フォルダー') && props.includes('asuta:asuta') && props.includes('drwx'), props);
await page.screenshot({ path: `${OUT}/upload-props.png` });
await page.keyboard.press('Escape');

sh(`rm -rf /home/asuta/${DIR}`);
fs.rmSync(local, { recursive: true });
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
