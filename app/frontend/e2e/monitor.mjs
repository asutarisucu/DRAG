// リソース監視を確かめる。下の欄に稼働時間・CPU・メモリ・ネットワークを出し、押すと詳細を開く。
// smoke.mjs と同じく wails dev の画面を開いて操作する
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let page;
// ほかのユーザーのホームに置く 3 MiB のファイル。sudo モードのときだけ数える
const cleanup = () => sh('rm -f /home/zuser/e2e-big', 'zuser');
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await page?.screenshot({ path: path.join(OUT, 'monitor-failed.png') }).catch(() => {});
  cleanup();
  process.exit(1);
});
sh('head -c 3145728 /dev/zero > /home/zuser/e2e-big', 'zuser');

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
const waitFor = async (fn, ms = 8000) => { for (let t = 0; t < ms; t += 200) { if (await fn().catch(() => false)) return true; await page.waitForTimeout(200); } return false; };
const text = sel => page.locator(sel).first().textContent();

// 下の欄
ok('下の欄に稼働時間が出る', await waitFor(async () => /\d+ (日|時間|分)/.test(await text('[data-sb="uptime"]'))), await text('[data-sb="uptime"]').catch(() => ''));
ok('CPU とメモリの値とグラフが出る', await waitFor(async () => /CPU.*\d+%/.test(await text('[data-sb="cpu"]')) && /メモリ.*[\d.]+ \/ [\d.]+ GB/.test(await text('[data-sb="mem"]')) && await page.locator('[data-sb="cpu"] svg polyline').count() === 1));
ok('ネットワークの下りと上りが出る', /B\/s.*B\/s/.test(await text('[data-sb="net"]')), await text('[data-sb="net"]'));

// 下の欄の CPU を押すと、リソース監視が CPU の詳細で開く
await page.click('[data-sb="cpu"]');
const win = page.locator('[data-win]:has([data-mon])');
await win.waitFor();
ok('CPU の詳細で開く', await win.locator('[data-mon="cpu"]').count() === 1 && (await win.textContent()).includes('リソース監視 · CPU') && (await win.boundingBox()).height >= 450);
ok('プロセスの一覧が出る', await waitFor(async () => (await win.locator('[data-procs] .prow:not(.phead)').count()) > 0 && (await win.locator('[data-procs]').textContent()).includes('sshd')));
ok('コアごとの使用率とロードアベレージが出る', await win.locator('.core').count() > 0 && (await win.textContent()).includes('ロードアベレージ'));
await page.screenshot({ path: `${OUT}/monitor-cpu.png` });

// 24 時間の記録に切り替える
await win.locator('[data-range] button:has-text("24 時間")').click();
ok('24 時間の記録に切り替える', await waitFor(async () => (await win.locator('svg.spark.big polyline').count()) === 1 || (await win.textContent()).includes('まだ記録がありません')));

// ほかの項目
await win.locator('.mon-head button:has-text("メモリ")').click();
ok('メモリの内訳が出る', await waitFor(async () => (await win.locator('.legend').textContent()).includes('キャッシュ')) && (await win.textContent()).includes('スワップ'));
await win.locator('.mon-head button:has-text("ディスク")').click();
ok('マウントが出る', await waitFor(async () => (await win.locator('.mount').count()) > 0 && (await win.locator('.mount').first().textContent()).startsWith('/')));
// 大きいフォルダはディスクの最上位から数える。ほかのユーザーのホームは読めないので数えない
const zuser = win.locator('.dir-row:has-text("/home/zuser")');
ok('大きいフォルダをディスクの最上位から数える', await waitFor(async () => (await win.locator('.dir-row:has-text("/usr")').count()) > 0, 20000), await win.locator('.dir-row').allTextContents().then(x => x.join(' | ')));
ok('読めないフォルダは数えていないと出す', await win.locator('[data-dirs-denied]').count() === 1 && await zuser.count() === 0);
await page.screenshot({ path: `${OUT}/monitor-disk.png` });
// sudo モードを ON にすると数え直し、ほかのユーザーのホームも数える
await win.locator('[data-dirs-denied] button').click();
const sudoDlg = page.locator('[role=dialog]:has-text("sudo モードを ON にする")');
ok('sudo のパスワードを聞く', (await sudoDlg.textContent()).includes('読めないフォルダも数えるには'));
await sudoDlg.locator('input[type=password]').fill('drag');
await sudoDlg.locator('button:has-text("ON にする")').click();
ok('ON にすると、ほかのユーザーのホームも数える', await waitFor(async () => (await zuser.count()) === 1 && (await win.locator('[data-dirs-denied]').count()) === 0, 20000), await win.locator('.dir-row').allTextContents().then(x => x.join(' | ')));
ok('数えた大きさを出す', /\d+(\.\d)? MB/.test(await zuser.textContent()), await zuser.textContent().catch(() => ''));
await page.screenshot({ path: `${OUT}/monitor-disk-sudo.png` });
// 左クリックすると、そのフォルダの中のフォルダの大きさを出す
const row = p => win.locator('.dir-row').filter({ has: page.locator('span.font-mono', { hasText: new RegExp(`^${p}$`) }) });
const paths = () => win.locator('.dir-row span.font-mono').allTextContents();
await row('/usr').click();
const nav = win.locator('[data-dir-nav]');
// 中では、開いたフォルダからの相対パスで出す
ok('左クリックでフォルダの中を相対パスで出す', await waitFor(async () => (await nav.count()) === 1 && (await row('lib').count()) === 1 && (await paths()).every(p => !p.startsWith('/') && !p.startsWith('~')), 20000), (await paths()).join(' | '));
ok('開いたフォルダと戻り先を出す', (await nav.textContent()).includes('‹ ディスク全体') && (await nav.textContent()).includes('/usr'), await nav.textContent());
await row('lib').click();
ok('さらに中へ入れる', await waitFor(async () => (await nav.textContent()).includes('/usr/lib') && (await paths()).length > 0 && (await row('lib').count()) === 0 && (await paths()).every(p => !p.startsWith('/')), 20000), (await paths()).join(' | '));
ok('戻り先は前に開いたフォルダ', (await nav.locator('button').textContent()).includes('‹ /usr'));
await page.screenshot({ path: `${OUT}/monitor-disk-in.png` });
await nav.locator('button').click();
ok('戻る', await waitFor(async () => (await row('lib').count()) === 1 && (await nav.textContent()).includes('ディスク全体')));
await nav.locator('button').click();
ok('ディスク全体に戻る', await waitFor(async () => (await nav.count()) === 0 && (await row('/var').count()) === 1));
await page.click('[data-sudo]');
cleanup();
await win.locator('.mon-head button:has-text("ネットワーク")').click();
ok('待ち受け中のポートに 22 が出る', await waitFor(async () => /\b22\b/.test(await win.locator('[data-listen]').textContent())));
ok('接続中に SSH の接続が出る', await waitFor(async () => (await win.locator('[data-conns] .crow:not(.chead)').count()) > 0));
await page.screenshot({ path: `${OUT}/monitor-net.png` });

// 概要に戻る
await win.locator('.mon-head button:has-text("概要")').click();
ok('概要に4つのカードが出る', await waitFor(async () => (await win.locator('[data-card]').count()) === 4));
await page.screenshot({ path: `${OUT}/monitor-overview.png` });

// タスクバーの左のボタンでも開き、2つ目は開かない
await page.click('[data-tool="monitor"]');
ok('タスクバーのボタンは同じウィンドウを前に出す', await page.locator('[data-win]:has([data-mon])').count() === 1);

// 下の欄に出す項目を設定で選ぶ
await page.click('button[title="設定"]');
await page.locator('[role=dialog] label:has-text("CPU") input').uncheck();
await page.keyboard.press('Escape');
ok('設定で外した項目は下の欄に出さない', await page.locator('[data-sb="cpu"]').count() === 0 && await page.locator('[data-sb="mem"]').count() === 1);

console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
