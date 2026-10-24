// 設定の外観とターミナルの配色を確かめる。アクセントの色、デスクトップの背景、ターミナルの配色。
// smoke.mjs と同じく wails dev の画面を開いて操作する
import { chromium } from 'playwright-core';
import path from 'node:path';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let page;
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await page?.screenshot({ path: path.join(OUT, 'look-failed.png') }).catch(() => {});
  process.exit(1);
});

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
await page.waitForSelector('[data-terminal] .xterm-rows');

const root = () => page.evaluate(() => ({
  accent: document.documentElement.style.getPropertyValue('--accent'),
  desk: document.documentElement.dataset.desk,
  term: document.documentElement.dataset.term,
}));
const style = (sel, prop) => page.$eval(sel, (e, p) => getComputedStyle(e)[p], prop);
const dlg = page.locator('[role=dialog]');
const openSettings = async () => {
  await page.click('button[title="設定"]');
  await dlg.waitFor();
};

// アクセントの色
await openSettings();
ok('はじめは青', (await root()).accent === '#5b9dff' && (await dlg.locator('[data-accent="#5b9dff"]').getAttribute('class')).includes('border-text'));
await dlg.locator('[data-accent="#8b7bff"]').click();
ok('候補を選ぶと変わる', (await root()).accent === '#8b7bff' && (await dlg.locator('[data-accent="#8b7bff"]').getAttribute('class')).includes('border-text'));
await dlg.locator('button:has-text("操作")').first().click();
ok('ON のスイッチの色も変わる', (await style('[role=dialog] [role=switch][aria-checked=true]', 'backgroundColor')) === 'rgb(139, 123, 255)');
await dlg.locator('button:has-text("外観")').first().click();
await dlg.locator('input[type=color]').fill('#e04040');
ok('好きな色を選べる', (await root()).accent === '#e04040');

// デスクトップの背景
const bg = () => style('[data-desktop] .desk-bg, .desk-bg', 'backgroundImage');
ok('はじめはグラデーション', (await bg()).split('radial-gradient').length - 1 === 3);
await dlg.locator('button:has-text("ドット")').click();
ok('ドット', (await root()).desk === 'dots' && (await bg()).split('radial-gradient').length - 1 === 1);
await dlg.locator('button:has-text("無地")').click();
ok('無地', (await root()).desk === 'plain' && (await bg()) === 'none');
await page.screenshot({ path: `${OUT}/look-accent.png` });

// ターミナルの配色。xterm.js の画面と、ログやエディターの背景が変わる
await dlg.locator('button:has-text("ターミナル")').first().click();
const preview = '[role=dialog] [data-term-preview]';
await page.waitForSelector(preview);
ok('はじめはダーク', (await root()).term === 'dark' && (await style(preview, 'backgroundColor')) === 'rgb(10, 13, 17)');
await dlg.locator('button:has-text("Solarized")').click();
ok('Solarized の見本', (await root()).term === 'solarized' && (await style(preview, 'backgroundColor')) === 'rgb(0, 43, 54)');
ok('ターミナルの文字の色が変わる', await page.waitForFunction(() => getComputedStyle(document.querySelector('[data-terminal] .xterm-rows')).color === 'rgb(147, 161, 161)', null, { timeout: 3000 }).then(() => true, () => false));
await dlg.locator('button:has-text("ライト")').click();
ok('ライト', (await style(preview, 'backgroundColor')) === 'rgb(251, 251, 253)' && await page.waitForFunction(() => getComputedStyle(document.querySelector('[data-terminal] .xterm-rows')).color === 'rgb(43, 50, 64)', null, { timeout: 3000 }).then(() => true, () => false));
await page.screenshot({ path: `${OUT}/look-term.png` });

// 読み込み直しても残る
await page.keyboard.press('Escape');
await page.reload();
await page.waitForSelector('text=サーバーに接続');
const kept = await root();
ok('読み込み直しても残る', kept.accent === '#e04040' && kept.desk === 'plain' && kept.term === 'light', JSON.stringify(kept));

// 初期設定に戻す
await openSettings();
await dlg.locator('button:has-text("初期設定に戻す")').click();
const reset = await root();
ok('初期設定に戻す', reset.accent === '#5b9dff' && reset.desk === 'gradient' && reset.term === 'dark', JSON.stringify(reset));
await page.keyboard.press('Escape');

await page.evaluate(() => localStorage.removeItem('drag-settings'));
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
