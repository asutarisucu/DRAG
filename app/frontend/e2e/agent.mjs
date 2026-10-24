// AI エージェント用の dragctl を、開発用の DRAG (wails dev) に向けて動かし、
// スマホの登録 URL を出す・PC に知らせる・履歴に残す・サーバーを操作するを確かめる。
// dragctl は先に go build -o build/bin/dragctl.exe ./cmd/dragctl で作っておく
import { chromium } from 'playwright-core';
import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const BIN = path.join(import.meta.dirname, '..', '..', 'build', 'bin', 'dragctl.exe');
const PHONE = 'https://127.0.0.1:18443';
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let pc;
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await pc?.screenshot({ path: path.join(OUT, 'agent-failed.png') }).catch(() => {});
  process.exit(1);
});
// dragctl を動かし、終了コードと出力を返す
const ctl = (args, input = '') => {
  const r = spawnSync(BIN, ['-dev', ...args], { input, encoding: 'utf8', timeout: 60000 });
  return { code: r.status, out: r.stdout, err: r.stderr };
};
const waitFor = async (fn, ms = 8000) => { for (let t = 0; t < ms; t += 200) { if (await fn().catch(() => false)) return true; await new Promise(r => setTimeout(r, 200)); } return false; };
sh('rm -f /tmp/agent-e2e.txt');

const browser = await chromium.launch({ channel: 'chrome' });
pc = await browser.newPage({ viewport: { width: 1440, height: 900 } });
pc.on('pageerror', e => { if (!e.stack.includes('/wails/ipc.js')) errors.push('pc pageerror ' + e.stack); });
await pc.goto('http://localhost:34115');
await pc.evaluate(async () => { for (const s of await window.go.main.App.Servers()) await window.go.main.App.Disconnect(s.id); });
await pc.evaluate(() => localStorage.removeItem('drag-settings'));
await pc.reload();
await pc.waitForSelector('text=サーバーに接続');

// 状態を読む
const st = ctl(['status']);
ok('status で待ち受けの様子を返す', st.code === 0 && JSON.parse(st.out).phone.status.url === PHONE, st.err);

// 直接公開が OFF なら、公開の登録 URL は出せないと言う
const noPub = ctl(['pair', 'public']);
ok('直接公開が OFF なら、ON にするよう言う', noPub.code === 1 && noPub.err.includes('直接公開を ON にしてください'), noPub.err.trim());

// LAN の登録 URL を出すと、PC に知らせが出て履歴に残る
const pair = ctl(['pair', 'lan']);
const url = pair.code === 0 ? JSON.parse(pair.out).url : '';
ok('登録 URL を出す', url.startsWith(PHONE + '/pair?t='), pair.err);
ok('PC に知らせる', await waitFor(async () => (await pc.locator('[role=status]:has-text("AI エージェントが、スマホの登録 URL (LAN 内) を出しました")').count()) > 0));
const hist = JSON.parse(ctl(['history', '5']).out);
ok('履歴に残る', hist[0].kind === 'agent' && hist[0].what === 'スマホの登録 URL を発行' && hist[0].route === 'lan', JSON.stringify(hist[0]));

// スマホでその URL を開くと登録できる
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, ignoreHTTPSErrors: true, locale: 'ja-JP' });
const phone = await ctx.newPage();
await phone.goto(url);
ok('エージェントの URL でスマホを登録できる', await phone.waitForSelector('text=この端末を登録しました', { timeout: 8000 }).then(() => true, () => false));
const devs = JSON.parse(ctl(['devices']).out);
const mine = devs.at(-1);
ok('登録した端末が一覧に出る', !!mine && mine.route === 'lan' && !mine.hash);
// 無効にすると履歴に残る
ok('スマホを無効にする', ctl(['revoke', mine.id]).code === 0);
ok('無効にした操作も履歴に残る', JSON.parse(ctl(['history', '3']).out).some(h => h.kind === 'agent' && h.what === 'スマホを無効にする ' + mine.id));
await ctx.close();

// 保存済みのサーバーにつなぐと、PC の画面にタブが出る
const conn = ctl(['connect', 'testserver'], 'drag\n');
const sid = conn.code === 0 ? JSON.parse(conn.out).server : '';
ok('保存済みのサーバーにつなぐ', /^s\d+$/.test(sid), conn.err.trim());
ok('PC の画面にタブが出る', await waitFor(async () => (await pc.locator('header :text("testserver")').count()) > 0));

// コマンドの実行。出力と終了コードをそのまま返す
const ex = ctl(['exec', 'testserver', 'echo', 'AG-$((20+22));', 'exit', '4']);
ok('コマンドの出力と終了コード', ex.out === 'AG-42\n' && ex.code === 4, JSON.stringify(ex));
// ファイルを書いて読む
ok('ファイルを書く', ctl(['write', sid, '/tmp/agent-e2e.txt'], 'hello\nagent\n').code === 0 && sh('cat /tmp/agent-e2e.txt') === 'hello\nagent');
ok('ファイルを読む', ctl(['cat', sid, '/tmp/agent-e2e.txt']).out === 'hello\nagent\n');
ok('フォルダの中を読む', JSON.parse(ctl(['ls', sid, '/tmp']).out).some(e => e.name === 'agent-e2e.txt'));
// sudo モードはエージェントの分だけ。PC の画面は OFF のまま
ok('sudo モードを ON にする', ctl(['sudo', sid, 'on'], 'drag\n').code === 0 && ctl(['exec', sid, 'id -u']).out === '0\n');
ok('PC の画面の sudo モードは OFF のまま', (await pc.getAttribute('[data-sudo]', 'data-sudo')) === 'off');
ctl(['sudo', sid, 'off']);
ok('切断すると PC のタブも閉じる', ctl(['disconnect', sid]).code === 0 && await waitFor(async () => (await pc.locator('header :text("testserver")').count()) === 0));

sh('rm -f /tmp/agent-e2e.txt');
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
