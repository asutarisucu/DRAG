// 保存した VPN を右クリックで編集する。試験用の ocserv (dev/ocserv) を先に動かしておく
//   cd dev/ocserv && docker compose up -d
//   node e2e/vpnedit.mjs
import { chromium } from 'playwright-core';
import fs from 'node:fs';
import path from 'node:path';

const OUT = path.join(import.meta.dirname, 'out');
const CA = fs.readFileSync(path.join(import.meta.dirname, '../../../dev/ocserv/out/ca.crt'), 'utf8');
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
  for (const v of await A.VPNs()) if (v.name.startsWith('e2e-edit')) await A.DeleteVPN(v.id);
});
try {
  await page.goto('http://localhost:34115');
  await page.waitForSelector('text=サーバーに接続');
  await cleanup();
  // グループを間違えたまま保存した VPN を、編集で直す
  const id = await page.evaluate(async ca => (await window.go.main.App.AddVPN({ name: 'e2e-edit', type: 'ac', config: '', server: '127.0.0.1:16443', ca, group: 'sales', psk: '', user: 'asuta', password: 'drag' })).id, CA);
  await page.reload();
  await page.waitForSelector('text=サーバーに接続');

  await page.click('button[title="設定"]');
  await page.locator('[role=dialog] button:has-text("VPN")').first().click();
  await page.locator('[data-vpn-row="e2e-edit"]').click({ button: 'right' });
  ok('設定の VPN の行を右クリックすると、編集と削除が出る', (await page.locator('[role=menu] [role=menuitem]').allTextContents()).join('|') === '編集|削除');
  await page.click('[role=menuitem]:has-text("編集")');
  const edit = page.locator('[data-vpn-edit]');
  await edit.waitFor();
  const vf = label => edit.locator(`label:has(span:text-is("${label}")) input`);
  ok('編集の窓に保存した値が入り、種類は変えられない',
    await edit.locator('select').first().isDisabled() && (await vf('サーバー').inputValue()) === '127.0.0.1:16443' && (await vf('グループ').inputValue()) === 'sales' && (await vf('パスワード').inputValue()) === '',
    `${await vf('グループ').inputValue()}`);
  ok('パスワードは空なら今のまま', (await vf('パスワード').getAttribute('placeholder')) === '変更しないときは空のまま');
  ok('CA の証明書も入っている', (await edit.locator('[data-vpn-ca]').count()) === 1);
  await page.screenshot({ path: `${OUT}/vpnedit.png` });
  await edit.locator('input[aria-label="名前"]').fill('e2e-edit2');
  await vf('グループ').fill('ops');
  await edit.locator('button[type=submit]').click();
  await edit.waitFor({ state: 'detached' });
  ok('保存すると一覧の名前が変わり、設定の窓は開いたまま', await page.locator('[data-vpn-row="e2e-edit2"]').count() === 1);
  const sum = await page.evaluate(async id => {
    const A = window.go.main.App;
    const d = await A.VPNDetail(id);
    // パスワードを入れずに保存したので、前のパスワードのままつながる
    const s = await A.ConnectVPN(id, '192.168.50.1');
    return { group: d.group, password: d.password, address: s.address };
  }, id);
  ok('グループが変わり、パスワードは前のままでつながる', sum.group === 'ops' && sum.password === '' && /^192\.168\.50\./.test(sum.address), JSON.stringify(sum));
  await page.keyboard.press('Escape');

  // 接続の画面の VPN の欄
  await page.click('button:has-text("サーバーに接続")');
  const dlg = page.locator('[role=dialog]');
  await dlg.locator('[data-vpn-select]').selectOption(id);
  await dlg.locator('[data-vpn-select]').click({ button: 'right' });
  ok('接続の画面の VPN の欄を右クリックすると、編集が出る', (await page.locator('[role=menu] [role=menuitem]').allTextContents()).join('|') === '編集');
  await page.click('[role=menuitem]:has-text("編集")');
  await page.locator('[data-vpn-edit]').waitFor();
  await page.keyboard.press('Escape');
  ok('Esc では編集の窓だけを閉じる', await page.locator('[data-vpn-edit]').count() === 0 && await page.locator('[data-vpn-select]').count() === 1);
  await page.keyboard.press('Escape');
  await cleanup();
} catch (e) {
  results.push('止まった: ' + e.message.split('\n')[0]);
  await page.screenshot({ path: `${OUT}/vpnedit-fail.png` }).catch(() => {});
}
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
