// スマホの画面の作りを確かめる。下のタブで区画を切り替え、ツリーは … と長押しのメニュー、ターミナルは足したキーで操作する。
// phone.mjs と同じく wails dev を DRAG_PHONE_LISTEN=127.0.0.1:18443 で動かしておく
import { chromium } from 'playwright-core';
import path from 'node:path';
import { sh } from './server.mjs';

const OUT = path.join(import.meta.dirname, 'out');
const PHONE = 'https://127.0.0.1:18443';
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);
const errors = [];
let pc, phone;
process.on('uncaughtException', async e => {
  console.log(results.join('\n'));
  console.log('止まった: ' + e.message.split('\n')[0]);
  await phone?.screenshot({ path: path.join(OUT, 'mobile-failed.png') }).catch(() => {});
  console.log(await phone?.evaluate(() => [...document.querySelectorAll('[data-win]')].map(w => `${w.className} ${JSON.stringify(w.getBoundingClientRect())}`).join('\n')).catch(() => ''));
  process.exit(1);
});
const waitFor = async (fn, ms = 8000) => { for (let t = 0; t < ms; t += 200) { if (await fn().catch(() => false)) return true; await new Promise(r => setTimeout(r, 200)); } return false; };

const D = '/home/asuta/e2e-mobile';
sh(`rm -rf ${D} && mkdir -p ${D}/sub && echo hello > ${D}/note.txt`);

// PC でサーバーにつなぎ、スマホを登録する
const browser = await chromium.launch({ channel: 'chrome' });
pc = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await pc.goto('http://localhost:34115');
// 前のテストで開いたままの接続を閉じる。wails dev では画面を読み込み直しても Go の側の接続は残る
await pc.evaluate(async () => { for (const s of await window.go.main.App.Servers()) await window.go.main.App.Disconnect(s.id); });
await pc.evaluate(() => localStorage.removeItem('drag-settings'));
await pc.reload();
await pc.waitForSelector('text=サーバーに接続');
await pc.click('section button:has-text("testserver")');
await pc.fill('[role=dialog] input[type=password]', 'drag');
await pc.click('[role=dialog] button:has-text("接続")');
await pc.waitForSelector('[role=dialog] button:has-text("開く")', { timeout: 10000 });
await pc.click('[role=dialog] button:has-text("開く")');
await pc.waitForSelector('.tree-row');
await pc.click('header button:has-text("スマホを登録")');
await pc.waitForSelector('svg[data-qr^="https://"]');
// 開発中の React は effect を2回動かすので、QR コードが作り直されて落ち着くのを待つ
let url = '';
for (let prev = null; url !== prev; await pc.waitForTimeout(600)) { prev = url; url = await pc.$eval('svg[data-qr]', e => e.dataset.qr); }

const ctx = await browser.newContext({
  viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, ignoreHTTPSErrors: true, locale: 'ja-JP',
  userAgent: 'Mozilla/5.0 (Linux; Android 15; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36',
});
phone = await ctx.newPage();
phone.on('pageerror', e => errors.push('phone pageerror ' + e.stack));
await phone.goto(url);
await phone.waitForSelector('nav [data-mtab="files"]', { timeout: 10000 });

const shown = sel => phone.locator(sel).first().isVisible().catch(() => false);
ok('下に3つのタブが出て、はじめはデスクトップ', (await phone.$$('nav [data-mtab]')).length === 3 && await shown('[data-desktop]') && !(await shown('[data-terminal]')) && !(await shown('.tree-row')));
const head = await phone.locator('header').textContent();
ok('上にはサーバーの名前だけを出す', head.includes('testserver') && !head.includes('127.0.0.1') && !head.includes('スマホを登録'), head);
const bar = await phone.$eval('[data-desktop]', e => { const t = e.parentElement.querySelector('footer'); return t && t.getBoundingClientRect().top < e.getBoundingClientRect().top; });
ok('タスクバーはデスクトップの上', bar);
await phone.screenshot({ path: `${OUT}/mobile-desktop.png` });

// ファイル。行の … からメニューを出す
await phone.tap('nav [data-mtab="files"]');
await phone.locator('.tree-row[data-path="/home/asuta/e2e-mobile"]').waitFor();
const rowH = await phone.$eval('.tree-row', e => e.getBoundingClientRect().height);
ok('ツリーの行は指で押せる高さ', rowH >= 39, `${rowH}px`);
await phone.locator('.tree-row[data-path="/home/asuta/e2e-mobile"]').tap();
await phone.locator('.tree-row[data-path="/home/asuta/e2e-mobile/note.txt"] button[title="操作"]').tap();
const menu = phone.locator('[role=menu]');
await menu.waitFor();
const mbox = await menu.boundingBox();
ok('メニューは下から出て、パスを上に書く', mbox.y + mbox.height > 844 - 30 && (await menu.textContent()).includes('~/e2e-mobile/note.txt'), JSON.stringify(mbox));
ok('スマホのメニューにはダウンロードとアップロードがない', !(await menu.textContent()).includes('ダウンロード'));
await phone.screenshot({ path: `${OUT}/mobile-menu.png` });
await menu.locator('button:has-text("デスクトップで開く")').tap();
ok('ファイルを開くとデスクトップに移り、区画いっぱいに出る', await waitFor(async () => {
  const w = await phone.locator('[data-win]').first().boundingBox();
  const d = await phone.locator('[data-desktop]').boundingBox();
  return w && d && d.height > 500 && Math.abs(w.width - d.width) < 2 && Math.abs(w.height - d.height) < 2;
}));
ok('エディターに中身が出る', await waitFor(async () => (await phone.locator('[data-win] textarea').inputValue()) === 'hello\n'));
await phone.locator('[data-win] button[title="閉じる"]').tap();

// 長押しでもメニューを出す
await phone.tap('nav [data-mtab="files"]');
const sub = phone.locator('.tree-row[data-path="/home/asuta/e2e-mobile/sub"]');
const sb = await sub.boundingBox();
const cdp = await ctx.newCDPSession(phone);
const touch = async (type, x, y) => cdp.send('Input.dispatchTouchEvent', { type, touchPoints: type === 'touchEnd' ? [] : [{ x, y }] });
await touch('touchStart', sb.x + 60, sb.y + sb.height / 2);
await phone.waitForTimeout(700);
ok('長押しでメニューを出す', await menu.isVisible().catch(() => false));
await touch('touchEnd');
await menu.locator('button:has-text("ターミナルで cd")').click();
ok('cd するとターミナルに移る', await waitFor(async () => await shown('[data-terminal]') && !(await shown('.tree-row'))));
const termText = () => phone.$eval('[data-terminal] .xterm-rows', e => [...e.children].map(r => r.textContent).join('\n'));
ok('cd がシェルに届く', await waitFor(async () => (await termText()).includes('cd ~/e2e-mobile/sub')));

// ターミナルの足したキー
ok('足したキーの列が出る', (await phone.$$('[data-terminal] button[data-key]')).length === 11);
await phone.locator('[data-terminal] .xterm-screen').tap();
await phone.keyboard.type('sleep 30\n');
await phone.waitForTimeout(800);
await phone.tap('[data-terminal] button[data-key="Ctrl"]');
await phone.keyboard.type('c');
await phone.keyboard.type('echo AFTER-$((2+3))\n');
ok('Ctrl を押してから c で止める', await waitFor(async () => (await termText()).includes('AFTER-5')));
await phone.tap('[data-terminal] button[data-key="↑"]');
await phone.keyboard.press('Enter');
ok('↑ で前のコマンドを出す', await waitFor(async () => ((await termText()).match(/AFTER-5/g) ?? []).length >= 2));
await phone.screenshot({ path: `${OUT}/mobile-terminal.png` });

// 設定は下から出し、高さを画面に合わせる
await phone.tap('header button[title="設定"]');
const dlg = await phone.locator('[role=dialog]').boundingBox();
ok('設定は下から出す', dlg && dlg.y + dlg.height >= 843 && dlg.width >= 389, JSON.stringify(dlg));
ok('スマホの設定にスマホのページはない', !(await phone.locator('[role=dialog]').textContent()).includes('LAN 内で待ち受ける') && (await phone.$$('[role=dialog] button:has-text("スマホ")')).length === 0);
await phone.screenshot({ path: `${OUT}/mobile-settings.png` });

// あと片付け。登録した端末を無効にする
await phone.keyboard.press('Escape');
const devs = await pc.$$('[data-device]');
for (const d of devs) if ((await d.textContent()).includes('Android')) await d.$eval('button', b => b.click());
sh(`rm -rf ${D}`);
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
await browser.close().catch(() => {});
process.exit(0);
