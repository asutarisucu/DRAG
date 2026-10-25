// サーバーへの接続だけを OpenVPN に通す。試験用の OpenVPN のサーバー (dev/ovpnserver) を先に動かしておく
//   cd dev/ovpnserver && docker compose up -d
// その VPN の中の 10.8.0.1:22 から、検証用のサーバーへ SSH でつなぐ
//   node e2e/ovpn.mjs
import { chromium } from 'playwright-core';
import path from 'node:path';
import { readFileSync } from 'node:fs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
const waitFor = async (f, ms = 10000) => { const end = Date.now() + ms; while (Date.now() < end) { if (await f().catch(() => false)) return true; await new Promise(r => setTimeout(r, 150)); } return false; };

// a.ovpn は UDP 1194、tls-crypt、ユーザー名とパスワードで入るサーバー
const conf = readFileSync(path.join(import.meta.dirname, '../../../dev/ovpnserver/out/a.ovpn'), 'utf8');

const browser = await chromium.launch({ channel: 'chrome' });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
page.on('console', m => { if (m.type() === 'error' && !m.text().includes('404')) errors.push('console ' + m.text()); });
try {
  await page.goto('http://localhost:34115');
  await page.evaluate(async () => {
    const A = window.go.main.App;
    for (const s of await A.Servers()) await A.Disconnect(s.id);
    for (const v of await A.VPNs()) if (v.name === 'e2e-ovpn') await A.DeleteVPN(v.id);
    for (const s of await A.SavedServers()) if (s.name === 'ovpn-e2e') await A.RemoveSavedServer(s.id);
  });
  await page.reload();
  await page.waitForSelector('text=サーバーに接続');

  await page.click('button:has-text("サーバーに接続")');
  const dlg = page.locator('[role=dialog]');
  const field = label => dlg.locator(`label:has(span:text-is("${label}")) input`).first();
  await field('名前').fill('ovpn-e2e');
  await field('ホスト').fill('10.8.0.1');
  await field('ポート').fill('22');
  await field('ユーザー').fill('asuta');
  await field('パスワード').fill('drag');
  await dlg.locator('[data-vpn-select]').selectOption('new');
  const form = dlg.locator('[data-vpn-new]');
  await form.locator('select').selectOption('ovpn');
  await form.locator('textarea').fill(conf);
  await form.locator('input[aria-label="名前"]').fill('e2e-ovpn');
  ok('.ovpn を読むとつなぎ先を出す', await waitFor(async () => (await form.locator('[data-vpn-sum]').textContent()) === 'つなぎ先 127.0.0.1:1194 (UDP) · ユーザー名とパスワードを使う'),
    await form.locator('[data-vpn-sum]').textContent().catch(() => ''));
  const vpnUser = form.locator('label:has(span:text-is("ユーザー名")) input');
  ok('auth-user-pass があるとユーザー名とパスワードを聞く', await vpnUser.count() === 1);
  await dlg.locator('button[type=submit]').click();
  ok('VPN のユーザー名がないと接続しない', await waitFor(async () => (await form.locator('[data-vpn-err]').textContent()).includes('ユーザー名とパスワード')));
  await vpnUser.fill('asuta');
  await form.locator('label:has(span:text-is("パスワード")) input').fill('drag');
  await page.screenshot({ path: `${OUT}/ovpn-add.png` });

  await dlg.locator('button[type=submit]').click();
  if (await page.waitForSelector('text=信頼して接続', { timeout: 10000 }).then(() => true, () => false)) await page.click('text=信頼して接続');
  await page.waitForSelector('[role=dialog] button:has-text("開く")', { timeout: 30000 });
  const steps = (await dlg.locator('ol').textContent()).replace(/\s+/g, ' ');
  ok('OpenVPN につないでから SSH でつなぐ', /VPN \(e2e-ovpn\) につなぐ/.test(steps) && /OpenVPN · 127\.0\.0\.1:1194 \(UDP\) · この PC は 10\.8\.0\.\d+/.test(steps) && steps.includes('asuta@10.8.0.1:22'), steps);
  await page.screenshot({ path: `${OUT}/ovpn-steps.png` });
  await page.click('[role=dialog] button:has-text("開く")');
  await page.waitForSelector('.tree-row');
  ok('VPN の中のサーバーのファイルを出す', await page.locator('.tree-row').count() > 0);
  ok('下の欄に VPN を出す', (await page.locator('[data-sb=vpn]').textContent()) === 'VPN e2e-ovpn');

  // 後片付け
  await page.hover('header >> text=ovpn-e2e');
  await page.click('header button[title="切断"]');
  await page.waitForSelector('text=保存済み');
  await page.evaluate(async () => {
    const A = window.go.main.App;
    for (const v of await A.VPNs()) if (v.name === 'e2e-ovpn') await A.DeleteVPN(v.id);
    for (const s of await A.SavedServers()) if (s.name === 'ovpn-e2e') await A.RemoveSavedServer(s.id);
  });
} catch (e) {
  results.push('止まった: ' + e.message.split('\n')[0]);
  await page.screenshot({ path: `${OUT}/ovpn-fail.png` }).catch(() => {});
}
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
