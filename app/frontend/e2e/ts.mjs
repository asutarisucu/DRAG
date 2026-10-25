// Tailscale の VPN を足す欄を確かめる。ログインのボタンは押さない (押すと Tailscale のサーバーへつなぎ、ブラウザが開く)。
// tailnet へのログインとつなぐところは、Headscale を相手にした Go の試験 (internal/vpn/tsvpn) で確かめる
//   node e2e/ts.mjs
import { chromium } from 'playwright-core';
import path from 'node:path';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];

const browser = await chromium.launch({ channel: 'chrome' });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
page.on('console', m => { if (m.type() === 'error' && !m.text().includes('404')) errors.push('console ' + m.text()); });
try {
  await page.goto('http://localhost:34115');
  await page.waitForSelector('text=サーバーに接続');
  await page.click('button:has-text("サーバーに接続")');
  const dlg = page.locator('[role=dialog]');
  const field = label => dlg.locator(`label:has(span:text-is("${label}")) input`).first();
  await field('名前').fill('ts-e2e');
  await field('ホスト').fill('peer');
  await field('ユーザー').fill('asuta');
  await field('パスワード').fill('drag');
  await dlg.locator('[data-vpn-select]').selectOption('new');
  const form = dlg.locator('[data-vpn-new]');
  await form.locator('select').first().selectOption('ts');
  ok('Tailscale はブラウザでのログインのボタンだけを出す',
    await form.locator('[data-vpn-ts] button:has-text("ブラウザでログイン")').count() === 1 &&
    await form.locator('label:has(span:text-is("サーバー"))').count() === 0 &&
    await form.locator('textarea').count() === 0);
  await page.screenshot({ path: `${OUT}/ts-add.png` });
  await dlg.locator('button[type=submit]').click();
  await form.locator('[data-vpn-err]').waitFor({ timeout: 5000 }).catch(() => {});
  ok('ログインしていなければ接続しない', await form.locator('[data-vpn-err]').textContent().then(t => t.includes('ログインしてください'), () => false));
  await page.click('[role=dialog] button:has-text("キャンセル")');
} catch (e) {
  results.push('止まった: ' + e.message.split('\n')[0]);
  await page.screenshot({ path: `${OUT}/ts-fail.png` }).catch(() => {});
}
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
