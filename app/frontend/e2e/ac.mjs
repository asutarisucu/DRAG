// サーバーへの接続だけを AnyConnect 互換の VPN に通す。試験用の ocserv (dev/ocserv) を先に動かしておく
//   cd dev/ocserv && docker compose up -d
// グループ ops でログインし、その VPN の中の 192.168.50.1:22 から、検証用のサーバーへ SSH でつなぐ
//   node e2e/ac.mjs
import { chromium } from 'playwright-core';
import path from 'node:path';

const OUT = path.join(import.meta.dirname, 'out');
const CA = path.join(import.meta.dirname, '../../../dev/ocserv/out/ca.crt');
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
  for (const v of await A.VPNs()) if (v.name === 'e2e-ac') await A.DeleteVPN(v.id);
  for (const s of await A.SavedServers()) if (s.name === 'ac-e2e') await A.RemoveSavedServer(s.id);
});
try {
  await page.goto('http://localhost:34115');
  await cleanup();
  await page.reload();
  await page.waitForSelector('text=サーバーに接続');

  await page.click('button:has-text("サーバーに接続")');
  const dlg = page.locator('[role=dialog]');
  const field = label => dlg.locator(`label:has(span:text-is("${label}")) input`).first();
  await field('名前').fill('ac-e2e');
  await field('ホスト').fill('192.168.50.1');
  await field('ポート').fill('22');
  await field('ユーザー').fill('asuta');
  await field('パスワード').fill('drag');
  await dlg.locator('[data-vpn-select]').selectOption('new');
  const form = dlg.locator('[data-vpn-new]');
  await form.locator('select').first().selectOption('ac');
  await form.locator('input[aria-label="名前"]').fill('e2e-ac');
  const vf = label => form.locator(`label:has(span:text-is("${label}")) input`);
  ok('AnyConnect 互換はサーバーとグループとユーザー名とパスワードを聞く',
    await vf('サーバー').count() === 1 && await vf('グループ').count() === 1 && await vf('ユーザー名').count() === 1 && await vf('パスワード').count() === 1);
  ok('グループは空ならサーバーの既定', (await vf('グループ').getAttribute('placeholder')) === '空ならサーバーの既定');
  await vf('サーバー').fill('127.0.0.1:16443');
  await vf('グループ').fill('ops');
  await vf('ユーザー名').fill('asuta');
  await vf('パスワード').fill('drag');
  await form.locator('input[type=file][accept*=".crt"]').setInputFiles(CA);
  ok('CA の証明書を読み込む', await waitFor(async () => (await form.locator('[data-vpn-ca]').textContent()).includes('ca.crt で確かめます')));
  await page.screenshot({ path: `${OUT}/ac-add.png` });

  await dlg.locator('button[type=submit]').click();
  if (await page.waitForSelector('text=信頼して接続', { timeout: 10000 }).then(() => true, () => false)) await page.click('text=信頼して接続');
  await page.waitForSelector('[role=dialog] button:has-text("開く")', { timeout: 30000 });
  const steps = (await dlg.locator('ol').textContent()).replace(/\s+/g, ' ');
  ok('AnyConnect 互換でつないでから SSH でつなぐ', /AnyConnect 互換 · 127\.0\.0\.1:16443 · この PC は 192\.168\.50\.\d+/.test(steps) && steps.includes('asuta@192.168.50.1:22'), steps);
  await page.click('[role=dialog] button:has-text("開く")');
  await page.waitForSelector('.tree-row');
  ok('VPN の中のサーバーのファイルを出す', await page.locator('.tree-row').count() > 0);
  ok('下の欄に VPN を出す', (await page.locator('[data-sb=vpn]').textContent()) === 'VPN e2e-ac');
  await cleanup();
} catch (e) {
  results.push('止まった: ' + e.message.split('\n')[0]);
  await page.screenshot({ path: `${OUT}/ac-fail.png` }).catch(() => {});
}
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
