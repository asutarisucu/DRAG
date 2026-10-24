// lanserve を、スマホの表示の Chrome で開いて確かめる。証明書の警告は飛ばす
// lanserve を -listen 127.0.0.1:8443 で起動してから、出てきた 127.0.0.1 の URL を渡す
//   node lan.mjs "https://127.0.0.1:8443/?t=..."
import { chromium } from 'playwright-core';
import path from 'node:path';
import tls from 'node:tls';
import https from 'node:https';

const out = f => path.join(import.meta.dirname, 'out', f);
const url = process.argv[2];
const origin = new URL(url).origin;
const results = [];
const ok = (name, cond, extra = '') => results.push(`${cond ? 'OK ' : 'NG '} ${name}${extra ? '  ' + extra : ''}`);

// 証明書の中身
const cert = await new Promise((res, rej) => {
  const s = tls.connect({ host: '127.0.0.1', port: +new URL(url).port, rejectUnauthorized: false }, () => { res(s.getPeerCertificate()); s.end(); });
  s.on('error', rej);
});
ok('証明書の SAN', /IP Address:127\.0\.0\.1/.test(cert.subjectaltname), cert.subjectaltname);
ok('証明書の用途', (cert.ext_key_usage || []).includes('1.3.6.1.5.5.7.3.1'), `${cert.ext_key_usage} ${cert.valid_from} 〜 ${cert.valid_to}`);

// 合言葉なしと、違う接続元
const plain = async (path, headers = {}) => {
  const r = await fetch(origin + path, { headers, redirect: 'manual' }).catch(e => ({ status: String(e.cause?.code || e) }));
  return r.status;
};
process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';
ok('合言葉なしでは開けない', (await plain('/')) === 403);
ok('違う合言葉では開けない', (await plain('/?t=WRONG')) === 403);

// CHROME_PATH が無ければ、入っている Google Chrome を使う
const browser = await chromium.launch(process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : { channel: 'chrome' });
const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 3, ignoreHTTPSErrors: true,
  userAgent: 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Mobile Safari/537.36' });
const page = await ctx.newPage();
const errors = [];
page.on('pageerror', e => errors.push(e.message));
await page.goto(url);
ok('合言葉の URL から本体に移る', page.url() === origin + '/?registered=1', page.url());
await page.waitForSelector('#checks li:nth-child(7)', { timeout: 10000 });
const checks = await page.$$eval('#checks li', ls => ls.map(l => l.textContent));
for (const c of checks) ok('ページ: ' + c.replace(/^(OK|NG) /, ''), c.startsWith('OK'));
await page.waitForTimeout(500);
await page.click('#term');
await page.keyboard.type('echo DRAG-$((40+2))\n');
await page.waitForTimeout(800);
const screen = await page.$eval('.xterm-rows', e => e.textContent);
ok('打ったコマンドが動く', screen.includes('DRAG-42'));
await page.screenshot({ path: out('lan-phone.png') });

// 別の接続元から WebSocket を開こうとする
const cookies = await ctx.cookies();
const c = cookies.find(c => c.name === 'drag');
ok('cookie の属性', c && c.httpOnly && c.secure && c.sameSite === 'Strict', JSON.stringify({ httpOnly: c?.httpOnly, secure: c?.secure, sameSite: c?.sameSite }));
// fetch は Upgrade の見出しを送れないので、https.request で送る
const upgrade = headers => new Promise(res => {
  const req = https.request(origin + '/ws', { rejectUnauthorized: false, headers: { Connection: 'Upgrade', Upgrade: 'websocket', 'Sec-WebSocket-Version': '13', 'Sec-WebSocket-Key': 'dGhlIHNhbXBsZSBub25jZQ==', ...headers } });
  req.on('response', r => { res(r.statusCode); r.resume(); });
  req.on('upgrade', (r, sock) => { res(r.statusCode); sock.destroy(); });
  req.on('error', e => res(String(e)));
  req.end();
});
const ws = await upgrade({ Cookie: `drag=${c.value}`, Origin: 'https://evil.example' });
ok('別の接続元からの WebSocket は断る', ws === 403, String(ws));
const ws2 = await upgrade({ Origin: origin });
ok('cookie なしの WebSocket は断る', ws2 === 403, String(ws2));
const ws3 = await upgrade({ Cookie: `drag=${c.value}`, Origin: origin });
ok('同じ接続元と cookie なら WebSocket を開く', ws3 === 101, String(ws3));

await browser.close();

// 127.0.0.1 はそれだけで安全なページとして扱われるので、別の名前で開き直し、
// スマホと同じく証明書の警告の画面から先へ進む
const port = new URL(url).port;
const b2 = await chromium.launch({ ...(process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : { channel: 'chrome' }), args: ['--host-resolver-rules=MAP drag.test 127.0.0.1'] });
const ctx2 = await b2.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, deviceScaleFactor: 3 });
const p2 = await ctx2.newPage();
await p2.goto(`https://drag.test:${port}/?t=${new URL(url).searchParams.get('t')}`).catch(() => {});
const warned = await p2.waitForSelector('#details-button', { timeout: 5000 }).catch(() => null);
ok('証明書の警告が出る', !!warned);
if (warned) {
  await p2.click('#details-button');
  await p2.click('#proceed-link');
  await p2.waitForSelector('#checks li:nth-child(7)', { timeout: 10000 }).catch(() => {});
  const checks2 = await p2.$$eval('#checks li', ls => ls.map(l => l.textContent));
  for (const c of checks2) ok('警告から進んだページ: ' + c.replace(/^(OK|NG) /, ''), c.startsWith('OK'));
  await p2.screenshot({ path: out('lan-phone-proceed.png') });
}
await b2.close();
console.log(results.join('\n'));
console.log(errors.length ? errors.join('\n') : 'ページのエラーなし');
