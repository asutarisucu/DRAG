// サーバーへの接続だけを L2TP/IPsec に通す。試験用の SoftEther VPN (dev/softether) を先に動かしておく
//   cd dev/softether && docker compose up -d
// 事前共有キー dragpsk でつなぎ、VPN の中から検証用のサーバー (dev/softether/out/target) へ SSH でつなぐ
//   node e2e/l2tp.mjs
import { chromium } from 'playwright-core';
import fs from 'node:fs';
import path from 'node:path';

const OUT = path.join(import.meta.dirname, 'out');
const TARGET = fs.readFileSync(path.join(import.meta.dirname, '../../../dev/softether/out/target'), 'utf8').trim();
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];

const browser = await chromium.launch({ channel: 'chrome' });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
page.on('console', m => { if (m.type() === 'error' && !m.text().includes('404')) errors.push('console ' + m.text()); });
const cleanup = () => page.evaluate(async () => {
  const A = window.go.main.App;
  for (const s of await A.Servers()) await A.Disconnect(s.id);
  for (const v of await A.VPNs()) if (v.name === 'e2e-l2tp') await A.DeleteVPN(v.id);
  for (const s of await A.SavedServers()) if (s.name === 'l2tp-e2e') await A.RemoveSavedServer(s.id);
});
try {
  await page.goto('http://localhost:34115');
  await cleanup();
  await page.reload();
  await page.waitForSelector('text=サーバーに接続');

  await page.click('button:has-text("サーバーに接続")');
  const dlg = page.locator('[role=dialog]');
  const field = label => dlg.locator(`label:has(span:text-is("${label}")) input`).first();
  await field('名前').fill('l2tp-e2e');
  await field('ホスト').fill(TARGET);
  await field('ポート').fill('22');
  await field('ユーザー').fill('asuta');
  await field('パスワード').fill('drag');
  await dlg.locator('[data-vpn-select]').selectOption('new');
  const form = dlg.locator('[data-vpn-new]');
  await form.locator('select').first().selectOption('l2tp');
  await form.locator('input[aria-label="名前"]').fill('e2e-l2tp');
  const vf = label => form.locator(`label:has(span:text-is("${label}")) input`);
  ok('L2TP/IPsec はサーバーと事前共有キーとユーザー名とパスワードを聞く',
    await vf('サーバー').count() === 1 && await vf('事前共有キー').count() === 1 && await vf('ユーザー名').count() === 1 && await vf('パスワード').count() === 1);
  ok('L2TP/IPsec は CA の証明書を聞かない', await form.locator('input[type=file][accept*=".crt"]').count() === 0);
  await vf('サーバー').fill('127.0.0.1:15501');
  await vf('ユーザー名').fill('asuta');
  await vf('パスワード').fill('drag');
  await dlg.locator('button[type=submit]').click();
  await form.locator('[data-vpn-err]').waitFor({ timeout: 5000 }).catch(() => {});
  ok('事前共有キーが無ければ止める', await form.locator('[data-vpn-err]').textContent().then(t => t.includes('事前共有キー'), () => false));
  await vf('事前共有キー').fill('dragpsk');
  await page.screenshot({ path: `${OUT}/l2tp-add.png` });

  await dlg.locator('button[type=submit]').click();
  if (await page.waitForSelector('text=信頼して接続', { timeout: 15000 }).then(() => true, () => false)) await page.click('text=信頼して接続');
  await page.waitForSelector('[role=dialog] button:has-text("開く")', { timeout: 30000 });
  const steps = (await dlg.locator('ol').textContent()).replace(/\s+/g, ' ');
  ok('L2TP/IPsec でつないでから SSH でつなぐ', /L2TP\/IPsec · 127\.0\.0\.1:15501 · この PC は 192\.168\.30\.\d+/.test(steps) && steps.includes(`asuta@${TARGET}:22`), steps);
  await page.click('[role=dialog] button:has-text("開く")');
  await page.waitForSelector('.tree-row');
  ok('VPN の中のサーバーのファイルを出す', await page.locator('.tree-row').count() > 0);
  ok('下の欄に VPN を出す', (await page.locator('[data-sb=vpn]').textContent()) === 'VPN e2e-l2tp');
  await cleanup();
} catch (e) {
  results.push('止まった: ' + e.message.split('\n')[0]);
  await page.screenshot({ path: `${OUT}/l2tp-fail.png` }).catch(() => {});
}
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
