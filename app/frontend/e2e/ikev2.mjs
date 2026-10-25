// サーバーへの接続だけを IKEv2 に通す。試験用の IKEv2 のサーバー (dev/ikev2server) を先に動かしておく
//   cd dev/ikev2server && docker compose up -d
// その VPN の中の 10.20.0.1:22 から、検証用のサーバーへ SSH でつなぐ
//   node e2e/ikev2.mjs
import { chromium } from 'playwright-core';
import path from 'node:path';

const OUT = path.join(import.meta.dirname, 'out');
const CA = path.join(import.meta.dirname, '../../../dev/ikev2server/out/ca.crt');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
const waitFor = async (f, ms = 10000) => { const end = Date.now() + ms; while (Date.now() < end) { if (await f().catch(() => false)) return true; await new Promise(r => setTimeout(r, 150)); } return false; };

const browser = await chromium.launch({ channel: 'chrome' });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
page.on('console', m => { if (m.type() === 'error' && !m.text().includes('404')) errors.push('console ' + m.text()); });
const cleanup = () => page.evaluate(async () => {
  const A = window.go.main.App;
  for (const s of await A.Servers()) await A.Disconnect(s.id);
  for (const v of await A.VPNs()) if (v.name === 'e2e-ikev2') await A.DeleteVPN(v.id);
  for (const s of await A.SavedServers()) if (s.name === 'ikev2-e2e') await A.RemoveSavedServer(s.id);
});
try {
  await page.goto('http://localhost:34115');
  await cleanup();
  await page.reload();
  await page.waitForSelector('text=サーバーに接続');

  await page.click('button:has-text("サーバーに接続")');
  const dlg = page.locator('[role=dialog]');
  const field = label => dlg.locator(`label:has(span:text-is("${label}")) input`).first();
  await field('名前').fill('ikev2-e2e');
  await field('ホスト').fill('10.20.0.1');
  await field('ポート').fill('22');
  await field('ユーザー').fill('asuta');
  await field('パスワード').fill('drag');
  await dlg.locator('[data-vpn-select]').selectOption('new');
  const form = dlg.locator('[data-vpn-new]');
  await form.locator('select').first().selectOption('ikev2');
  await form.locator('input[aria-label="名前"]').fill('e2e-ikev2');
  const vf = label => form.locator(`label:has(span:text-is("${label}")) input`);
  ok('IKEv2 はサーバーとユーザー名とパスワードを聞く', await vf('サーバー').count() === 1 && await vf('ユーザー名').count() === 1 && await vf('パスワード').count() === 1);
  await vf('サーバー').fill('127.0.0.1:14500');
  await vf('ユーザー名').fill('asuta');
  await dlg.locator('button[type=submit]').click();
  ok('パスワードが無いと接続しない', await waitFor(async () => (await form.locator('[data-vpn-err]').textContent()).includes('パスワード')));
  await vf('パスワード').fill('drag');
  await form.locator('input[type=file][accept*=".crt"]').setInputFiles(CA);
  ok('CA の証明書を読み込む', await waitFor(async () => (await form.locator('[data-vpn-ca]').textContent()).includes('ca.crt で確かめます')));
  await page.screenshot({ path: `${OUT}/ikev2-add.png` });

  await dlg.locator('button[type=submit]').click();
  if (await page.waitForSelector('text=信頼して接続', { timeout: 10000 }).then(() => true, () => false)) await page.click('text=信頼して接続');
  await page.waitForSelector('[role=dialog] button:has-text("開く")', { timeout: 30000 });
  const steps = (await dlg.locator('ol').textContent()).replace(/\s+/g, ' ');
  ok('IKEv2 につないでから SSH でつなぐ', /IKEv2 · 127\.0\.0\.1:14500 · この PC は 10\.10\.10\.\d+/.test(steps) && steps.includes('asuta@10.20.0.1:22'), steps);
  await page.screenshot({ path: `${OUT}/ikev2-steps.png` });
  await page.click('[role=dialog] button:has-text("開く")');
  await page.waitForSelector('.tree-row');
  ok('VPN の中のサーバーのファイルを出す', await page.locator('.tree-row').count() > 0);
  ok('下の欄に VPN を出す', (await page.locator('[data-sb=vpn]').textContent()) === 'VPN e2e-ikev2');
  await cleanup();
} catch (e) {
  results.push('止まった: ' + e.message.split('\n')[0]);
  await page.screenshot({ path: `${OUT}/ikev2-fail.png` }).catch(() => {});
}
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
