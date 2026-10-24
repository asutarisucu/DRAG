// サービスと Docker のウィンドウを確かめる。権限が足りない操作は sudo モードを聞いてからやり直す。
// Docker は検証用のサーバーに置いた偽の docker (dev/testserver/fakedocker.sh) を使う。
// smoke.mjs と同じく wails dev の画面を開いて操作する
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let page;
const cleanup = () => sh('systemctl stop drag-e2e.service; rm -f /etc/systemd/system/drag-e2e.service; systemctl daemon-reload; rm -rf /var/lib/fakedocker/*', 'root');
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await page?.screenshot({ path: path.join(OUT, 'tools-failed.png') }).catch(() => {});
  cleanup();
  process.exit(1);
});
cleanup();
// printf に渡す \n は JSON の書き方で docker exec に届くので、JS では改行の文字にしておく
const made = sh(`printf '[Unit]\nDescription=DRAG e2e\n[Service]\nExecStart=/bin/sleep infinity\n[Install]\nWantedBy=multi-user.target\n' > /etc/systemd/system/drag-e2e.service && systemctl daemon-reload`, 'root');
if (made.startsWith('ERR') || !sh('systemctl list-unit-files --no-legend drag-e2e.service', 'root').includes('disabled')) throw new Error('サービスを用意できない: ' + made);

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
const dialog = page.locator('[role=dialog]:has-text("sudo モードを ON にする")');
const sudoBtn = page.locator('[data-sudo]');

// サービス
await page.click('[data-tool="services"]');
const svcWin = page.locator('[data-win]:has(.svc-list)');
await svcWin.waitFor();
ok('サービスの一覧が出る', await waitFor(async () => (await svcWin.locator('[data-service="ssh.service"]').textContent()).includes('稼働中')));
await svcWin.locator('input').fill('drag-e2e');
const row = svcWin.locator('[data-service="drag-e2e.service"]');
ok('名前で絞り込む', await waitFor(async () => (await svcWin.locator('[data-service]').count()) === 1 && (await row.textContent()).includes('停止')));
await row.locator('button:has-text("起動")').click();
await dialog.waitFor({ timeout: 5000 });
ok('起動に権限が要れば sudo モードを聞く', (await dialog.textContent()).includes('サービスを操作するには'));
await dialog.locator('input[type=password]').fill('drag');
await dialog.locator('button:has-text("ON にする")').click();
ok('ON にすると起動する', await waitFor(async () => (await row.textContent()).includes('稼働中')) && sh('systemctl is-active drag-e2e.service') === 'active');
ok('下に起動したと出る', (await svcWin.locator('[role=status]').textContent()).includes('drag-e2e.service を起動しました'));
await row.locator('button:has-text("ログ")').click();
const svcLog = page.locator('[data-win]:has-text("drag-e2e.service のログ")');
ok('サービスのログを開く', await waitFor(async () => (await svcLog.locator('[data-log]').textContent()).includes('DRAG e2e')), (await svcLog.locator('[data-log]').textContent().catch(() => '')).slice(-120));
await page.screenshot({ path: `${OUT}/tools-services.png` });
await svcLog.locator('button[title="閉じる"]').click();
await row.locator('button:has-text("停止")').click();
ok('ON の間は聞かずに止める', await waitFor(async () => (await row.textContent()).includes('停止') && sh('systemctl is-active drag-e2e.service') !== 'active') && await dialog.count() === 0);
await svcWin.locator('button[title="閉じる"]').click();

// Docker。docker のグループでないので、OFF のときは聞く
await sudoBtn.click();
await waitFor(async () => await sudoBtn.getAttribute('data-sudo') === 'off');
await page.click('[data-tool="docker"]');
await dialog.waitFor({ timeout: 5000 });
ok('Docker を開くと sudo モードを聞く', (await dialog.textContent()).includes('Docker を使うには'));
await dialog.locator('button:has-text("キャンセル")').click();
const dkWin = page.locator('[data-win]:has-text("Docker")').last();
ok('やめると権限が無いと出る', await waitFor(async () => (await dkWin.textContent()).includes('Docker を使う権限がありません')));
await dkWin.locator('button:has-text("ON にする")').click();
await dialog.locator('input[type=password]').fill('drag');
await dialog.locator('button:has-text("ON にする")').click();
ok('ON にするとコンテナが出る', await waitFor(async () => (await dkWin.locator('[data-container]').count()) === 3));
ok('Compose ごとにまとめる', (await dkWin.locator('[data-project="app"]').count()) === 1 && (await dkWin.textContent()).includes('Compose なし'));
ok('動いているコンテナの CPU とメモリが出る', await waitFor(async () => /CPU [\d.]+% · [\d.]+MiB/.test(await dkWin.locator('[data-container="web"] .dk-stat').textContent()), 10000));
await dkWin.locator('[data-container="web"] .dk-row').click({ position: { x: 20, y: 20 } });
ok('開くとマウントが出る', await waitFor(async () => (await dkWin.locator('[data-container="web"] .dk-detail').textContent()).includes('web-data → /usr/share/nginx/html')));
const webText = await dkWin.locator('[data-container="web"]').textContent();
ok('状態を日本語で出す', webText.includes('nginx:1.27 · 稼働 3 日') && webText.includes('稼働中 (3 日)'), webText);
ok('詳細に Compose のファイルを出す', webText.includes('Compose~/app/compose.yaml'));
await page.screenshot({ path: `${OUT}/tools-docker.png` });
await dkWin.locator('[data-container="web"] button:has-text("停止")').click();
ok('コンテナを止める', await waitFor(async () => (await dkWin.locator('[data-container="web"]').textContent()).includes('停止 2 時間前')));
ok('止めたコンテナは終了コードを出す', (await dkWin.locator('[data-container="web"]').textContent()).includes('停止 (終了コード 0 · 2 時間前)'));
await dkWin.locator('[data-container="backup"] button:has-text("削除")').click();
await dkWin.locator('[data-container="backup"] button.danger').click();
ok('確かめてから削除する', await waitFor(async () => (await dkWin.locator('[data-container]').count()) === 2));
await dkWin.locator('[data-container="redis"] button:has-text("ログ")').click();
const dkLog = page.locator('[data-win]:has-text("redis のログ")');
ok('コンテナのログを追う', await waitFor(async () => /redis log line [6-9]/.test(await dkLog.locator('[data-log]').textContent()), 10000));
await dkLog.locator('button[title="閉じる"]').click();

await dkWin.locator('.seg button:has-text("イメージ")').click();
ok('使っているイメージは消せない', await dkWin.locator('[data-image="nginx:1.27"] button:has-text("削除")').isDisabled());
await dkWin.locator('[data-image="hello-world:latest"] button:has-text("削除")').click();
ok('使っていないイメージを消す', await waitFor(async () => (await dkWin.locator('[data-image="hello-world:latest"]').count()) === 0));
await dkWin.locator('input[placeholder="イメージ名"]').fill('busybox');
await page.keyboard.press('Enter');
ok('イメージを取得する', await waitFor(async () => (await dkWin.locator('[data-image="busybox:latest"]').count()) === 1));
await dkWin.locator('input[placeholder="イメージ名"]').fill('nginx:1.27');
await page.keyboard.press('Enter');
ok('あるイメージは取得しない', await waitFor(async () => (await dkWin.locator('[role=status]').textContent()).includes('nginx:1.27 はもうあります')));

await dkWin.locator('.seg button:has-text("ボリューム")').click();
ok('ボリュームの大きさが出る', await waitFor(async () => (await dkWin.locator('[data-volume="web-data"]').textContent()).includes('12.5MB · /var/lib/docker/volumes/web-data/_data')));
ok('使っているボリュームは消せない', await dkWin.locator('[data-volume="redis-data"] button:has-text("削除")').isDisabled());
await dkWin.locator('[data-volume="old-data"] button:has-text("削除")').click();
ok('使っていないボリュームを消す', await waitFor(async () => (await dkWin.locator('[data-volume="old-data"]').count()) === 0));

await dkWin.locator('.seg button:has-text("コンテナ")').click();
await dkWin.locator('[data-container="redis"] button:has-text("シェル")').click();
ok('シェルはセッションのウィンドウで開く', await waitFor(async () => (await page.locator('[data-win]:has-text("docker exec -it redis sh")').count()) === 1));

cleanup();
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
