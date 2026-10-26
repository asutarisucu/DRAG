// ツリーの中のドラッグでの移動と、リソース監視・Docker の窓の「ツリーで表示」を確かめる。
// Docker は検証用のサーバーに置いた偽の docker (dev/testserver/fakedocker.sh) を使う。
// smoke.mjs と同じく wails dev の画面を開いて操作する
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const DIR = 'tree-e2e';
const H = `/home/asuta/${DIR}`;
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let page;
const cleanup = () => sh(`rm -rf ${H} /home/asuta/backups /var/lib/fakedocker/*`, 'root');
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await page?.screenshot({ path: path.join(OUT, 'tree-failed.png') }).catch(() => {});
  cleanup();
  process.exit(1);
});
cleanup();
sh(`mkdir -p ${H}/src/deep ${H}/dst /home/asuta/backups && echo f > ${H}/src/f.txt && echo n > ${H}/note.txt && head -c 30000000 /dev/zero > ${H}/src/deep/big.bin && tar czf ${H}/pack.tar.gz -C /etc hostname`);
// root のフォルダ。ここへ移すには sudo モードが要る
sh(`mkdir ${H}/locked`, 'root');
// 偽の docker の状態。ホームの中と外のフォルダをつなぐコンテナを置く
sh(`printf 'backup|alpine:3.20|exited||||bind:/home/asuta/backups:/backups\ncfg|alpine:3.20|exited||||bind:/etc/ssh:/cfg\n' > /var/lib/fakedocker/containers && printf 'alpine|3.20|7.8MB\n' > /var/lib/fakedocker/images && : > /var/lib/fakedocker/volumes`, 'root');

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
const at = p => page.locator(`.tree-row[data-path="${p}"]`);
const selected = async p => (await at(p).getAttribute('class').catch(() => '')).split(' ').includes('selected');

// ファイルの区画をいちばん狭くしても、見出しの「ファイル」は1行に入りきる
const resizer = page.locator('.cursor-col-resize').first();
const rb = await resizer.boundingBox();
const resize = async x => {
  await page.mouse.move(rb.x + rb.width / 2, rb.y + 100);
  await page.mouse.down();
  await page.mouse.move(x, rb.y + 100, { steps: 4 });
  await page.mouse.up();
};
await resize(0);
const title = page.locator('aside div:has(> button[title="すべて閉じる"]) > span');
const tb = await title.boundingBox(), hb = await page.locator('aside div:has(> button[title="すべて閉じる"])').boundingBox();
ok('狭くしても見出しは1行に入りきる', tb.height < 20 && tb.y >= hb.y && tb.y + tb.height <= hb.y + hb.height && await title.evaluate(s => s.scrollWidth <= s.clientWidth), JSON.stringify([tb, hb]));
await resize(rb.x + rb.width / 2);

// ドラッグでフォルダへ移す
await at(H).click();
await at(`${H}/note.txt`).dragTo(at(`${H}/dst`));
ok('ファイルをフォルダへドラッグすると移す', await waitFor(async () => sh(`ls ${H}/dst`) === 'note.txt') && await waitFor(async () => (await at(`${H}/dst/note.txt`).count()) === 1));
ok('移したと出る', (await page.locator('[role=status]').last().textContent()).includes('note.txt を ~/tree-e2e/dst/ へ移動しました'));

// PC のファイルをドラッグしてくると、ファイルの一覧に切り替え、ツリーの下に落とす先を出す
await page.click('nav button[title="セッション"]');
const dt = await page.evaluateHandle(() => { const d = new DataTransfer(); d.items.add(new File(['x'], 'pc.txt')); return d; });
await page.dispatchEvent('body', 'dragenter', { dataTransfer: dt });
ok('PC からのドラッグでファイルの一覧に切り替える', await waitFor(async () => (await page.locator('aside button[title="すべて閉じる"]').isVisible())));
ok('落とせることを出す', (await page.locator('.tree-drop').textContent()) === 'ここへドロップするとアップロード');
await page.dispatchEvent(`.tree-row[data-path="${H}/dst"]`, 'dragover', { dataTransfer: dt });
ok('行の上では落とす先を出す', await waitFor(async () => (await page.locator('.tree-drop').textContent()) === '~/tree-e2e/dst/ にアップロード'));
await page.dispatchEvent('body', 'drop', { dataTransfer: dt });
ok('落とすと消す', await waitFor(async () => (await page.locator('.tree-drop').count()) === 0));

// 自分の中と、いまあるフォルダには移さない
await at(`${H}/src`).click();
await at(`${H}/src`).dragTo(at(`${H}/src/deep`));
await at(`${H}/src`).dragTo(at(H));
await page.waitForTimeout(800);
ok('自分の中といまある所には移さない', sh(`ls ${H}`).split('\n').includes('src') && sh(`ls ${H}/src`).split('\n').join(',') === 'deep,f.txt');

// 権限の無いフォルダへは、sudo モードを聞いてから移す
await at(`${H}/src/f.txt`).dragTo(at(`${H}/locked`));
const dialog = page.locator('[role=dialog]:has-text("sudo モードを ON にする")');
await dialog.waitFor({ timeout: 5000 });
ok('権限が無ければ sudo モードを聞く', (await dialog.textContent()).includes('f.txt を移動するには'));
await dialog.locator('input[type=password]').fill('drag');
await dialog.locator('button:has-text("ON にする")').click();
ok('ON にすると移す', await waitFor(async () => sh(`ls ${H}/locked`, 'root') === 'f.txt'));
await page.click('[data-sudo]');

// キー操作。↑↓ で選ぶ行を動かし、→← でフォルダを開け閉めし、Enter で開く
await page.click('aside button[title="すべて閉じる"]');
await at(H).click();
await at(`${H}/dst`).waitFor();
await page.keyboard.press('ArrowDown');
ok('↓ で次の行を選ぶ', await waitFor(async () => await selected(`${H}/dst`)));
await page.keyboard.press('ArrowRight');
ok('→ でフォルダを開く', await waitFor(async () => (await at(`${H}/dst/note.txt`).count()) === 1));
await page.keyboard.press('ArrowLeft');
ok('← でフォルダを閉じる', await waitFor(async () => (await at(`${H}/dst/note.txt`).count()) === 0));
await page.keyboard.press('ArrowLeft');
ok('閉じたフォルダで ← を押すと親を選ぶ', await waitFor(async () => await selected(H)));
await page.keyboard.press('ArrowDown');
await page.keyboard.press('Enter');
await page.keyboard.press('ArrowDown');
await page.keyboard.press('Enter');
const ed = page.locator('[data-win]:has(textarea)');
ok('Enter でフォルダを開き、ファイルを開く', await waitFor(async () => (await ed.count()) === 1 && (await ed.locator('textarea').inputValue()) === 'n\n'));

// 開いているファイルの名前を変えると、ウィンドウもついてくる。編集中の中身は残す
await ed.locator('textarea').click();
await page.keyboard.press('Control+End');
await page.keyboard.type('edited');
await at(`${H}/dst/note.txt`).click();
await page.keyboard.press('F2');
await page.locator('.tree-row input').fill('renamed.txt');
await page.locator('.tree-row input').press('Enter');
ok('名前を変えるとウィンドウの題名が変わる', await waitFor(async () => (await ed.locator('span:has-text("● renamed.txt")').count()) > 0));
ok('編集中の中身は残る', (await ed.locator('textarea').inputValue()) === 'n\nedited');
await ed.locator('textarea').click();
await page.keyboard.press('Control+s');
ok('新しい名前で保存する', await waitFor(async () => sh(`cat ${H}/dst/renamed.txt`) === 'n\nedited') && sh(`ls ${H}/dst`) === 'renamed.txt');
await ed.locator('button[title="閉じる"]').click();

// 圧縮ファイルの大きさと、マウスを乗せた行の … ボタン
const pack = at(`${H}/pack.tar.gz`);
ok('圧縮ファイルの行に大きさが出る', /\d+(\.\d+)? B$/.test(await pack.textContent()), await pack.textContent());
ok('… ボタンはふだん隠す', !(await pack.locator('button[title="操作"]').isVisible()));
await pack.hover();
ok('マウスを乗せると … ボタンが出る', await pack.locator('button[title="操作"]').isVisible());
await pack.locator('button[title="操作"]').click();
ok('… ボタンでメニューを出す', await page.locator('[role=menu]').isVisible());
await page.keyboard.press('Escape');

// 行の無いところの右クリックは、いちばん上のフォルダのメニュー。ホームはコピーだけできる
const treeBox = page.locator('aside div[tabindex="0"]').first();
const bb = await treeBox.boundingBox();
await treeBox.click({ button: 'right', position: { x: 60, y: bb.height - 12 } });
ok('行の無いところで右クリックすると、いちばん上のメニュー', await waitFor(async () => (await page.locator('[role=menu] button:has-text("ゴミ箱を開く")').count()) === 1));
ok('ホームはコピーできる', !(await page.locator('[role=menu] button:has-text("Ctrl+C")').isDisabled()) && await page.locator('[role=menu] button:has-text("Ctrl+X")').isDisabled());
await page.keyboard.press('Escape');

// リソース監視の大きいフォルダから、ツリーで表示する。セッションの一覧を出していてもファイルに切り替える
await page.click('aside button[title="すべて閉じる"]');
await page.click('nav button[title="セッション"]');
await page.click('[data-tool="monitor"]');
const mon = page.locator('[data-win]:has([data-mon])');
await mon.locator('.mon-card:has-text("ディスク")').click().catch(() => {});
await mon.locator('.mon-head button:has-text("ディスク")').click();
// ディスク全体ではホームは ~ の1行なので、左クリックでホームの中を数え、右クリックでツリーに表示する
const homeRow = mon.locator('.dir-row').filter({ has: page.locator('span.font-mono', { hasText: /^~$/ }) });
await waitFor(async () => (await homeRow.count()) === 1, 20000);
await homeRow.click();
const dirRow = mon.locator('.dir-row').filter({ has: page.locator('span.font-mono', { hasText: /^tree-e2e\/src$/ }) });
ok('大きいフォルダに出る', await waitFor(async () => (await dirRow.count()) === 1, 20000));
await dirRow.click({ button: 'right' });
ok('ファイルの一覧に切り替える', await waitFor(async () => (await page.locator('aside button[title="すべて閉じる"]').isVisible())));
ok('大きいフォルダをツリーで開いて選ぶ', await waitFor(async () => await selected(`${H}/src`) && (await at(`${H}/src/deep`).count()) === 1));
ok('ツリーで表示した行を光らせる', (await at(`${H}/src`).getAttribute('class')).includes('flash'));

// フォルダのプロパティは、中のファイルを数えて大きさを出す
await at(`${H}/src`).click({ button: 'right' });
await page.click('[role=menu] button:has-text("プロパティ")');
ok('フォルダのプロパティに中のファイルの合計を出す', await waitFor(async () => (await page.textContent('[role=dialog]')).includes('29 MB (1 個のファイル)')), await page.textContent('[role=dialog]'));
await page.keyboard.press('Escape');

// Docker のバインドマウントから、ツリーで表示する。ホームの外ならツリーのいちばん上を / にする
await page.click('[data-tool="docker"]');
// 検証用のサーバーの asuta は docker のグループにいないので、sudo モードで使う
await dialog.waitFor({ timeout: 5000 });
await dialog.locator('input[type=password]').fill('drag');
await dialog.locator('button:has-text("ON にする")').click();
const dk = page.locator('[data-win]:has(.dk-list)');
await dk.locator('[data-container="cfg"] .dk-row').click();
await dk.locator('[data-container="cfg"] button.mlink:has-text("/etc/ssh")').click();
ok('ホームの外のマウントは / から開く', await waitFor(async () => (await page.locator('[data-tree-top="/"]').count()) === 1 && await selected('/etc/ssh')));
await dk.locator('[data-container="backup"] .dk-row').click();
await dk.locator('[data-container="backup"] button.mlink:has-text("~/backups")').click();
ok('ホームの中のマウントを選ぶ', await waitFor(async () => await selected('/home/asuta/backups')));
await page.screenshot({ path: `${OUT}/tree-reveal.png` });
await page.click('[data-tree-top] button[title="ホームへ"]');
await page.click('[data-sudo]');

cleanup();
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
