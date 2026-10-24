// PC アプリの画面を通しで確かめる。wails dev の画面をブラウザーで開いて操作する
// 先に検証用のサーバー (dev/testserver) と、app/ で wails dev を起動しておく
//   node e2e/smoke.mjs
// 接続が切れたときを試すため、検証用のサーバーで asuta の sshd のプロセスを止める
import { chromium } from 'playwright-core';
import path from 'node:path';
import { execSync } from 'node:child_process';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];

const browser = await chromium.launch({ channel: 'chrome' });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
page.on('console', m => { if (m.type() === 'error' && !m.text().includes('404')) errors.push('console ' + m.text()); });
await page.goto('http://localhost:34115');
await page.evaluate(() => localStorage.clear());
await page.reload();
await page.waitForSelector('text=サーバーに接続');

// 接続。保存済みに testserver があれば選ぶ
await page.click('button:has-text("サーバーに接続")');
await page.waitForSelector('[role=dialog]');
const field = label => page.locator(`[role=dialog] label:has(span:text-is("${label}")) input`);
const savedBtn = page.locator('[role=dialog] button:has-text("testserver")');
if (await savedBtn.count()) {
  await savedBtn.first().click();
  ok('保存済みを選ぶと入力が埋まる', (await field('ホスト').inputValue()) === '127.0.0.1' && (await field('ポート').inputValue()) === '2222');
} else {
  await field('名前').fill('testserver');
  await field('ホスト').fill('127.0.0.1');
  await field('ポート').fill('2222');
  await field('ユーザー').fill('asuta');
}
await field('パスワード').fill('drag');
await page.click('[role=dialog] button:has-text("接続")');
if (await page.waitForSelector('text=信頼して接続', { timeout: 3000 }).then(() => true, () => false)) await page.click('text=信頼して接続');
await page.waitForSelector('[role=dialog] button:has-text("開く")', { timeout: 10000 });
const steps = (await page.textContent('[role=dialog] ol')).replace(/\s+/g, ' ');
ok('手順の表示', steps.includes('asuta@127.0.0.1:2222') && steps.includes('Ubuntu 24.04') && steps.includes('Docker はない'), steps);
await page.screenshot({ path: `${OUT}/app2-steps.png` });
await page.click('[role=dialog] button:has-text("開く")');
await page.waitForSelector('.tree-row');

const termText = () => page.$eval('.xterm-rows', e => [...e.children].map(r => r.textContent).join('\n'));
const typeCmd = async cmd => { await page.click('.xterm-screen'); await page.keyboard.type(cmd + '\n'); };
await page.waitForFunction(() => document.querySelector('.xterm-rows').textContent.includes('$'), null, { timeout: 10000 });
await typeCmd('echo BEFORE-$((1+1))');
await page.waitForFunction(() => document.querySelector('.xterm-rows').textContent.includes('BEFORE-2'), null, { timeout: 5000 }).catch(() => {});
ok('ターミナル', (await termText()).includes('BEFORE-2'));

// 接続を切る。サーバー側で、このユーザーの sshd のプロセスを止める
execSync('docker exec testserver-testserver-1 pkill -f "sshd: asuta"');
const banner = await page.waitForSelector('[role=alert]', { timeout: 30000 }).then(() => true, () => false);
const bannerText = banner ? (await page.textContent('[role=alert]')).replace(/\s+/g, ' ') : '';
ok('切れたら表示が出る', banner && bannerText.includes('接続が切れました'), bannerText);
await page.screenshot({ path: `${OUT}/app2-lost.png` });
const back = await page.waitForSelector('[role=alert]', { state: 'detached', timeout: 30000 }).then(() => true, () => false);
ok('自動で再接続する', back);
await page.waitForFunction(() => document.querySelector('.xterm-rows').textContent.includes('新しいシェルで開き直しました'), null, { timeout: 10000 }).catch(() => {});
await page.waitForTimeout(800);
await typeCmd('echo AFTER-$((2+2))');
await page.waitForFunction(() => document.querySelector('.xterm-rows').textContent.includes('AFTER-4'), null, { timeout: 5000 }).catch(() => {});
const after = await termText();
ok('再接続のあと新しいシェルで動く', after.includes('新しいシェルで開き直しました') && after.includes('AFTER-4'));
await page.screenshot({ path: `${OUT}/app2-reconnected.png` });

// 設定
const count = async () => (await termText()).split('シェルが終了しました').length + (await termText()).split('開き直しました').length
const before = await count();
await page.click('button[title="設定"]');
await page.waitForSelector('[role=dialog] h3');
const h = () => page.$eval('[role=dialog]', e => Math.round(e.getBoundingClientRect().height));
const h1 = await h();
await page.click('[role=dialog] button:has-text("ターミナル")');
const h2 = await h();
await page.fill('[role=dialog] input[type=range]', '17');
const h3 = await h();
ok('設定の窓の高さが変わらない', h1 === h2 && h2 === h3, `${h1} ${h2} ${h3}`);
await page.click('[role=dialog] button:has-text("外観")');
await page.click('[role=dialog] button:has-text("English")');
const en = await page.textContent('aside');
ok('英語に切り替わる', en.includes('Files'), en.slice(0, 20));
await page.click('[role=dialog] button:has-text("Light")');
await page.screenshot({ path: `${OUT}/app2-light-en.png` });
ok('ライトのテーマ', (await page.evaluate(() => document.documentElement.dataset.theme)) === 'light');
await page.click('[role=dialog] button:has-text("Restore defaults")');
ok('初期設定に戻る', (await page.evaluate(() => document.documentElement.dataset.theme)) === 'dark' && (await page.textContent('aside')).includes('ファイル'));
const afterCount = await count();
ok('言語を替えてもシェルは開き直さない', before === afterCount, `${before} ${afterCount}`);
await page.keyboard.press('Escape');

// 区画の幅
const aside = () => page.$eval('aside', e => Math.round(e.getBoundingClientRect().width));
const w0 = await aside();
const rz = await page.$('aside + div');
const box = await rz.boundingBox();
await page.mouse.move(box.x + 2, box.y + 200);
await page.mouse.down();
await page.mouse.move(box.x + 102, box.y + 200, { steps: 5 });
await page.mouse.up();
const w1 = await aside();
ok('ファイルツリーの幅を変えられる', w1 === w0 + 100, `${w0} → ${w1}`);
const saved = await page.evaluate(() => JSON.parse(localStorage.getItem('drag-settings')).sidebarW);
ok('幅を覚えている', saved === w1, String(saved));

// 切断すると最初の画面に戻り、保存済みに出る
await page.hover('header >> text=testserver');
await page.click('header button[title="切断"]');
await page.waitForSelector('text=保存済み');
ok('切断すると最初の画面で保存済みに出る', await page.isVisible('section button:has-text("testserver")'));
await page.screenshot({ path: `${OUT}/app2-welcome.png` });

console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
