// サーバーへの接続だけを VPN に通す。試験用の WireGuard の相手 (dev/wgpeer) を立て、
// その VPN の中の 10.99.0.1:22 から検証用のサーバーへ SSH でつなぐ
//   node e2e/vpn.mjs
import { chromium } from 'playwright-core';
import path from 'node:path';
import { spawn } from 'node:child_process';
import { existsSync, readFileSync, rmSync } from 'node:fs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
const waitFor = async (f, ms = 10000) => { const end = Date.now() + ms; while (Date.now() < end) { if (await f().catch(() => false)) return true; await new Promise(r => setTimeout(r, 150)); } return false; };

// WireGuard の相手を立て、DRAG に読ませる .conf を待つ
const confPath = path.join(OUT, 'wg.conf');
rmSync(confPath, { force: true });
// go が PATH に無いときは、既定の入れ場所のものを使う
const GO = existsSync('C:/Program Files/Go/bin/go.exe') ? 'C:/Program Files/Go/bin/go.exe' : 'go';
const peer = spawn(GO, ['run', '.', '-conf', confPath, '-forward', '127.0.0.1:2222'], { cwd: path.join(import.meta.dirname, '../../../dev/wgpeer'), stdio: 'ignore' });
const stop = () => { try { process.kill(peer.pid); } catch { } spawn('taskkill', ['/F', '/IM', 'wgpeer.exe', '/T'], { stdio: 'ignore' }); };
if (!await waitFor(async () => existsSync(confPath), 120000)) { console.log('止まった: WireGuard の相手が立たない'); stop(); process.exit(1); }
const conf = readFileSync(confPath, 'utf8');
const port = conf.match(/Endpoint = 127\.0\.0\.1:(\d+)/)[1];

const browser = await chromium.launch({ channel: 'chrome' });
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
page.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pageerror ' + e.stack); });
page.on('console', m => { if (m.type() === 'error' && !m.text().includes('404')) errors.push('console ' + m.text()); });
try {
  await page.goto('http://localhost:34115');
  // 前の試験の接続と、試験で足した VPN と保存済みのサーバーを消しておく
  await page.evaluate(async () => {
    const A = window.go.main.App;
    for (const s of await A.Servers()) await A.Disconnect(s.id);
    for (const v of await A.VPNs()) if (v.name === 'e2e-wg') await A.DeleteVPN(v.id);
    for (const s of await A.SavedServers()) if (s.name === 'vpn-e2e') await A.RemoveSavedServer(s.id);
  });
  await page.reload();
  await page.waitForSelector('text=サーバーに接続');

  await page.click('button:has-text("サーバーに接続")');
  const dlg = page.locator('[role=dialog]');
  const field = label => dlg.locator(`label:has(span:text-is("${label}")) input`);
  const vpnSel = dlg.locator('[data-vpn-select]');
  await field('名前').fill('vpn-e2e');
  await field('ホスト').fill('10.99.0.1');
  await field('ポート').fill('22');
  await field('ユーザー').fill('asuta');
  await field('パスワード').fill('drag');
  ok('VPN の欄に「なし」と「追加」がある', (await vpnSel.locator('option').allTextContents()).join('|').startsWith('なし|') && (await vpnSel.locator('option').last().textContent()) === '新しい VPN を追加…');
  await vpnSel.selectOption('new');
  const form = dlg.locator('[data-vpn-new]');
  const kinds = await form.locator('select option').evaluateAll(os => os.map(o => `${o.textContent}${o.disabled ? '(x)' : ''}`));
  ok('種類は WireGuard を選べ、ほかは準備中で選べない', kinds[0] === 'WireGuard' && kinds.slice(1).every(k => k.endsWith('(準備中)(x)')), kinds.join('|'));
  await dlg.locator('button[type=submit]').click();
  ok('設定がないと接続しない', await waitFor(async () => (await form.locator('[data-vpn-err]').textContent()).includes('設定ファイル')));
  await form.locator('textarea').fill('[Interface]\nPrivateKey = abc\n');
  ok('読めない設定はその理由を出す', await waitFor(async () => (await form.locator('[data-vpn-err]').textContent()).includes('鍵の形')));
  await form.locator('textarea').fill(conf);
  await form.locator('input[aria-label="名前"]').fill('e2e-wg');
  ok('設定を読むとつなぎ先を出す', await waitFor(async () => (await form.locator('[data-vpn-sum]').textContent()) === `つなぎ先 127.0.0.1:${port} · この PC は 10.99.0.2 · 通す範囲 10.99.0.0/24`),
    await form.locator('[data-vpn-sum]').textContent().catch(() => ''));
  await page.screenshot({ path: `${OUT}/vpn-add.png` });

  await dlg.locator('button[type=submit]').click();
  if (await page.waitForSelector('text=信頼して接続', { timeout: 8000 }).then(() => true, () => false)) await page.click('text=信頼して接続');
  await page.waitForSelector('[role=dialog] button:has-text("開く")', { timeout: 20000 });
  const steps = (await dlg.locator('ol').textContent()).replace(/\s+/g, ' ');
  ok('VPN につないでから SSH でつなぐ', steps.includes('VPN (e2e-wg) につなぐ') && steps.includes(`WireGuard · 127.0.0.1:${port} · この PC は 10.99.0.2`) && steps.includes('asuta@10.99.0.1:22') && steps.includes('Ubuntu'), steps);
  await page.screenshot({ path: `${OUT}/vpn-steps.png` });
  await page.click('[role=dialog] button:has-text("開く")');
  await page.waitForSelector('.tree-row');
  ok('VPN の中のサーバーのファイルを出す', await page.locator('.tree-row').count() > 0);
  ok('タブに VPN の印を出す', await page.locator('header [data-tab-vpn]').count() === 1);
  ok('下の欄に VPN を出す', (await page.locator('[data-sb=vpn]').textContent()) === 'VPN e2e-wg');
  await page.screenshot({ path: `${OUT}/vpn-open.png` });

  // 足した VPN は保存され、次の接続で選べる。保存済みのサーバーを選ぶと、その VPN も選ぶ
  await page.hover('header >> text=vpn-e2e');
  await page.click('header button[title="切断"]');
  await page.waitForSelector('text=保存済み');
  await page.click('button:has-text("サーバーに接続")');
  await dlg.locator('button:has-text("vpn-e2e")').click();
  ok('保存済みのサーバーに VPN を出す', (await dlg.locator('button:has-text("vpn-e2e") [data-saved-vpn]').textContent()) === 'e2e-wg');
  ok('保存済みのサーバーを選ぶと VPN も選ぶ', (await vpnSel.locator('option:checked').textContent()) === 'e2e-wg (WireGuard)');
  await page.keyboard.press('Escape');

  // 設定の VPN のページで見て消す
  await page.click('button[title="設定"]');
  await page.click('[role=dialog] button:has-text("VPN")');
  const row = dlg.locator('div.border-b:has([data-vpn-remove="e2e-wg"])');
  ok('設定に VPN と使うサーバーを出す', (await row.textContent()).includes(`WireGuard · 127.0.0.1:${port}`) && (await row.textContent()).includes('vpn-e2e で使う'), await row.textContent());
  await page.screenshot({ path: `${OUT}/vpn-settings.png` });
  await dlg.locator('[data-vpn-remove="e2e-wg"]').click();
  ok('VPN を消す', await waitFor(async () => (await dlg.locator('[data-vpn-remove="e2e-wg"]').count()) === 0));
  const saved = await page.evaluate(async () => (await window.go.main.App.SavedServers()).find(s => s.name === 'vpn-e2e'));
  ok('消した VPN を使っていたサーバーは VPN なしに戻す', saved && !saved.vpn, JSON.stringify(saved));
  await page.evaluate(async id => window.go.main.App.RemoveSavedServer(id), saved?.id);
} catch (e) {
  results.push('止まった: ' + e.message.split('\n')[0]);
  await page.screenshot({ path: `${OUT}/vpn-fail.png` }).catch(() => {});
}
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
stop();
process.exit(0);
