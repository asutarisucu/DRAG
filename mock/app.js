'use strict';
// DRAG の画面モック。サーバーとのやり取りはすべて作り物で、ブラウザの中だけで動く。

/* ---------- 道具 ---------- */

const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => [...r.querySelectorAll(s)];

function el(tag, props, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(props || {})) {
    if (v == null || v === false) continue;
    if (k === 'class') e.className = v;
    else if (k === 'style') e.style.cssText = v;
    else if (k === 'dataset') Object.assign(e.dataset, v);
    else if (k.startsWith('on') && typeof v === 'function') e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v === true ? '' : v);
  }
  for (const kid of kids.flat(Infinity)) {
    if (kid == null || kid === false) continue;
    e.append(kid instanceof Node ? kid : String(kid));
  }
  return e;
}

const ICONS = {
  folder: '<path d="M3 7.5A2.5 2.5 0 0 1 5.5 5H9l2 2h7.5A2.5 2.5 0 0 1 21 9.5v8a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5z"/>',
  file: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>',
  log: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M9 13h6M9 17h4"/>',
  image: '<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="10" r="2"/><path d="m21 16-5-5-9 9"/>',
  archive: '<rect x="3" y="4" width="18" height="5" rx="1"/><path d="M5 9v10a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9M10 13h4"/>',
  code: '<path d="m8 8-4 4 4 4M16 8l4 4-4 4M14 5l-4 14"/>',
  terminal: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="m7 9 3 3-3 3M13 15h4"/>',
  layers: '<path d="m12 3 9 5-9 5-9-5z"/><path d="m3 13 9 5 9-5"/>',
  files: '<path d="M9 3h6l4 4v10a2 2 0 0 1-2 2H9a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z"/><path d="M15 3v4h4M4 8v11a2 2 0 0 0 2 2h8"/>',
  chart: '<path d="M4 4v16h16"/><path d="m7 15 4-5 3 3 5-6"/>',
  server: '<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>',
  qr: '<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><path d="M14 14h3v3h-3zM20 14v.01M14 20h.01M17 20h4v-3"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  x: '<path d="M6 6l12 12M18 6 6 18"/>',
  min: '<path d="M5 12h14"/>',
  max: '<rect x="5" y="5" width="14" height="14" rx="1.5"/>',
  restore: '<rect x="5" y="8" width="11" height="11" rx="1.5"/><path d="M8 8V6.5A1.5 1.5 0 0 1 9.5 5h8A1.5 1.5 0 0 1 19 6.5v8a1.5 1.5 0 0 1-1.5 1.5H16"/>',
  grid: '<rect x="4" y="4" width="7" height="7" rx="1"/><rect x="13" y="4" width="7" height="7" rx="1"/><rect x="4" y="13" width="7" height="7" rx="1"/><rect x="13" y="13" width="7" height="7" rx="1"/>',
  gear: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>',
  upload: '<path d="M12 16V4M7 9l5-5 5 5M4 20h16"/>',
  download: '<path d="M12 4v12M7 11l5 5 5-5M4 20h16"/>',
  more: '<circle class="fill" cx="5" cy="12" r="1.6"/><circle class="fill" cx="12" cy="12" r="1.6"/><circle class="fill" cx="19" cy="12" r="1.6"/>',
  chevron: '<path d="m9 6 6 6-6 6"/>',
  phone: '<rect x="7" y="2" width="10" height="20" rx="2.5"/><path d="M11 18h2"/>',
  grip: '<circle class="fill" cx="9" cy="6" r="1.4"/><circle class="fill" cx="15" cy="6" r="1.4"/><circle class="fill" cx="9" cy="12" r="1.4"/><circle class="fill" cx="15" cy="12" r="1.4"/><circle class="fill" cx="9" cy="18" r="1.4"/><circle class="fill" cx="15" cy="18" r="1.4"/>',
  lock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/>',
  link: '<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>',
  box: '<path d="m12 3 8 4.5v9L12 21l-8-4.5v-9z"/><path d="m4 7.5 8 4.5 8-4.5M12 12v9"/>',
  db: '<ellipse cx="12" cy="6" rx="7" ry="3"/><path d="M5 6v12c0 1.7 3.1 3 7 3s7-1.3 7-3V6M5 12c0 1.7 3.1 3 7 3s7-1.3 7-3"/>',
  window: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18"/>',
  eraser: '<path d="m7 21-4-4 10-10 7 7-7 7zM14 21h7M8 12l7 7"/>',
  collapse: '<path d="m7 15 5-5 5 5"/><path d="M5 20h14M5 4h14"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  down: '<path d="M12 5v14M6 13l6 6 6-6"/>',
  up: '<path d="M12 19V5M6 11l6-6 6 6"/>',
  trash: '<path d="M4 7h16M10 11v6M14 11v6M6 7l1 12a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-12M9 7V4h6v3"/>',
  copy: '<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/>',
  cut: '<circle cx="6" cy="18" r="3"/><circle cx="18" cy="18" r="3"/><path d="M8.1 16 18 4M15.9 16 6 4"/>',
  paste: '<rect x="5" y="4" width="14" height="17" rx="2"/><path d="M9 4V3h6v1M9 10h6M9 14h6"/>',
  edit: '<path d="M4 20h4L19 9l-4-4L4 16z"/><path d="m13 7 4 4"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5v.01"/>',
  home: '<path d="m3 11 9-7 9 7"/><path d="M5 10v10h14V10"/>',
  filePlus: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M12 11v6M9 14h6"/>',
  folderPlus: '<path d="M3 7.5A2.5 2.5 0 0 1 5.5 5H9l2 2h7.5A2.5 2.5 0 0 1 21 9.5v8a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5z"/><path d="M12 10.5v6M9 13.5h6"/>',
};

function ic(name, cls) {
  const s = el('span', { class: 'ic' + (cls ? ' ' + cls : '') });
  s.innerHTML = `<svg viewBox="0 0 24 24" aria-hidden="true">${ICONS[name] || ''}</svg>`;
  return s;
}

const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
const rand = (a, b) => a + Math.random() * (b - a);
const pick = a => a[Math.floor(Math.random() * a.length)];
const last = a => a[a.length - 1];
const pad2 = n => String(n).padStart(2, '0');
const sleep = ms => new Promise(r => setTimeout(r, ms));
const isMobile = () => matchMedia('(max-width: 720px)').matches;
const clock = (d = new Date()) => `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`;

function hashStr(s) {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619);
  return h >>> 0;
}
function mulberry32(a) {
  return () => {
    a |= 0; a = a + 0x6D2B79F5 | 0;
    let t = Math.imul(a ^ a >>> 15, 1 | a);
    t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
    return ((t ^ t >>> 14) >>> 0) / 4294967296;
  };
}
const hex = s => hashStr(s).toString(16).padStart(8, '0') + hashStr(s + '#').toString(16).slice(0, 4);

function fmtSize(b) {
  if (b < 1024) return `${b} B`;
  const u = ['KB', 'MB', 'GB'];
  let v = b / 1024, i = 0;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${u[i]}`;
}

function tokenize(s) {
  const out = [];
  const re = /"([^"]*)"|'([^']*)'|(\S+)/g;
  let m;
  while ((m = re.exec(s))) out.push(m[1] ?? m[2] ?? m[3]);
  return out;
}

/* ---------- 作り物のデータ ---------- */

const TEXT = {
  props: `#Minecraft server properties
#Fri Oct 24 04:00:12 JST 2026
motd=DRAG test server
server-port=25565
max-players=20
online-mode=true
white-list=true
difficulty=normal
gamemode=survival
view-distance=10
simulation-distance=8
spawn-protection=0
enable-rcon=false`,
  ops: `[
  {
    "uuid": "00000000-0000-0000-0000-000000000001",
    "name": "asuta",
    "level": 4,
    "bypassesPlayerLimit": false
  }
]`,
  start: `#!/bin/bash
cd "$(dirname "$0")"
java -Xms4G -Xmx6G -jar paper.jar nogui`,
  velocity: `config-version = "2.7"
bind = "0.0.0.0:25577"
motd = "DRAG proxy"
show-max-players = 20
online-mode = true

[servers]
lobby = "127.0.0.1:25565"
try = ["lobby"]`,
  notes: `毎日 4:00 にバックアップ (cron)
mc のコンソールは screen -r mc
proxy は screen -r proxy`,
  bashrc: `# ~/.bashrc
case $- in
    *i*) ;;
      *) return;;
esac

HISTSIZE=1000
alias ll='ls -alF'
export EDITOR=vim`,
  compose: `services:
  mariadb:
    image: mariadb:11
    restart: always
    volumes:
      - ./data:/var/lib/mysql
  app:
    image: node:22
    restart: always
    depends_on:
      - mariadb`,
  appenv: `NODE_ENV=production
DB_HOST=mariadb
DB_NAME=app`,
  backuplog: `[2026-10-23 04:00:01] backup start
[2026-10-23 04:00:36] mariadb dump ok (411 MB)
[2026-10-23 04:00:51] gzip ok
[2026-10-23 04:00:52] backup done
[2026-10-24 04:00:01] backup start
[2026-10-24 04:00:37] mariadb dump ok (412 MB)
[2026-10-24 04:00:52] gzip ok
[2026-10-24 04:00:53] backup done`,
  indexjs: `import http from 'node:http';

http.createServer((req, res) => {
  res.end('ok');
}).listen(3000);`,
  pkg: `{
  "name": "app",
  "version": "1.0.0",
  "type": "module",
  "scripts": { "start": "node index.js" }
}`,
  applog: `[09:00:02 INFO]: listening on :3000
[09:14:51 INFO]: GET / 200 3ms
[09:15:07 WARN]: slow response GET /api/items 1204ms
[09:20:33 INFO]: GET / 200 2ms`,
  router: `hostname=router
lan_ip=192.168.1.1
wifi_ssid=home`,
  syslog: `Oct 24 09:00:01 router cron: job started
Oct 24 09:00:01 router dnsmasq: read /etc/hosts
Oct 24 09:12:44 router dropbear: password auth succeeded for 'admin'`,
};

const MC_MSGS = [
  ['INFO', 'asuta joined the game'],
  ['INFO', 'kuro joined the game'],
  ['INFO', '<asuta> 回路できた'],
  ['INFO', '<kuro> おつかれ'],
  ['INFO', '<kuro> 村人の取引所どこ?'],
  ['INFO', 'Saving the game (this may take a moment!)'],
  ['INFO', 'Saved the game'],
  ['INFO', 'kuro has made the advancement [Hot Stuff]'],
  ['WARN', "Can't keep up! Is the server overloaded? Running 2034ms or 40 ticks behind"],
  ['INFO', '[LuckPerms] Performing a sync...'],
  ['INFO', 'kuro left the game'],
];

function mcInitialLog() {
  const lines = [];
  let t = Date.now() - 45 * 60000;
  const at = () => clock(new Date(t));
  for (const b of ['Starting minecraft server version 1.21.4', 'Loading properties', 'Default game type: SURVIVAL',
    'Preparing level "world"', 'Preparing start region for dimension minecraft:overworld', 'Done (12.384s)! For help, type "help"']) {
    lines.push(`[${at()} INFO]: ${b}`);
    t += 900;
  }
  for (let i = 0; i < 26; i++) {
    t += rand(30000, 90000);
    const [lv, msg] = pick(MC_MSGS);
    lines.push(`[${at()} ${lv}]: ${msg}`);
  }
  return lines.join('\n');
}

function velocityLog() {
  const t = clock(new Date(Date.now() - 50 * 60000));
  return [`[${t} INFO]: Booting up Velocity 3.4.0...`, `[${t} INFO]: Loading localizations...`,
    `[${t} INFO]: Listening on /[0:0:0:0:0:0:0:0%0]:25577`, `[${t} INFO]: Done (1.32s)!`,
    `[${t} INFO]: [connected player] asuta (/203.0.113.50:51234) has connected`].join('\n');
}

const F = (name, size, content) => ({ name, dir: false, size, content });
const D = (name, children, open = false) => ({ name, dir: true, open, children });

const DEFS = [
  {
    id: 'mc', name: 'mc-server', host: '203.0.113.10', lan: '192.168.1.10', user: 'asuta',
    tree: () => D('asuta', [
      D('mc', [
        D('logs', [F('latest.log', 6210, mcInitialLog()), F('2026-10-23-1.log.gz', 48213)], true),
        D('plugins', [F('LuckPerms.jar', 2203118), F('EssentialsX.jar', 1452210)]),
        D('world', [F('level.dat', 18210), D('region', [F('r.0.0.mca', 6291456), F('r.-1.0.mca', 5242880)])]),
        F('paper.jar', 52428800), F('server.properties', 1180, TEXT.props), F('ops.json', 120, TEXT.ops),
        F('server-icon.png', 4210), F('start.sh', 96, TEXT.start),
      ], true),
      D('proxy', [D('logs', [F('latest.log', 412, velocityLog())]), F('velocity.toml', 2410, TEXT.velocity)]),
      D('backups', [F('mc-2026-10-23.tar.gz', 734003200), F('mc-2026-10-24.tar.gz', 736100352)]),
      F('notes.txt', 120, TEXT.notes), F('.bashrc', 3771, TEXT.bashrc),
    ], true),
    screens: [{ name: 'mc', desc: 'Minecraft サーバー' }, { name: 'proxy', desc: 'Velocity' }],
    tmux: [{ name: 'work', desc: '2 ウィンドウ' }],
    docker: {
      containers: [
        { name: 'web', image: 'nginx:1.27', state: 'running', up: '3 days', upJa: '3 日', ports: ['8080→80'], mounts: [{ type: 'volume', src: 'web-data', dst: '/usr/share/nginx/html' }] },
        { name: 'redis', image: 'redis:7', state: 'running', up: '3 days', upJa: '3 日', ports: [], mounts: [{ type: 'volume', src: 'redis-data', dst: '/data' }] },
        { name: 'backup', image: 'alpine:3.20', state: 'exited', exit: 0, ago: '5 hours', agoJa: '5 時間前', ports: [], mounts: [{ type: 'bind', src: '/home/asuta/backups', dst: '/backups' }] },
      ],
      images: [{ name: 'nginx:1.27', size: 192e6 }, { name: 'redis:7', size: 117e6 }, { name: 'alpine:3.20', size: 7.8e6 }, { name: 'node:22', size: 1.1e9 }],
      volumes: [{ name: 'web-data', size: 24e6 }, { name: 'redis-data', size: 3.1e6 }, { name: 'old-data', size: 51e6 }],
    },
    services: [
      { name: 'mc-server.service', desc: 'Minecraft サーバー', active: true },
      { name: 'nginx.service', desc: 'Web サーバー', active: false },
      { name: 'docker.service', desc: 'Docker', active: true },
      { name: 'cron.service', desc: '定期実行', active: true },
      { name: 'ssh.service', desc: 'OpenSSH サーバー', active: true },
    ],
    procs: [
      { pid: 1342, user: 'asuta', name: 'java', cmd: 'java -Xms4G -Xmx6G -jar paper.jar nogui', w: 60, mem: 4310 },
      { pid: 1410, user: 'asuta', name: 'java', cmd: 'java -Xmx512M -jar velocity.jar', w: 6, mem: 612 },
      { pid: 812, user: 'root', name: 'dockerd', cmd: '/usr/bin/dockerd -H fd://', w: 5, mem: 94 },
      { pid: 845, user: 'root', name: 'containerd', cmd: '/usr/bin/containerd', w: 3, mem: 51 },
      { pid: 2210, user: '101', name: 'nginx', cmd: 'nginx: worker process', w: 4, mem: 12, container: 'web' },
      { pid: 2302, user: '999', name: 'redis-server', cmd: 'redis-server *:6379', w: 3, mem: 9, container: 'redis' },
      { pid: 655, user: 'root', name: 'sshd', cmd: 'sshd: asuta@pts/0', w: 1, mem: 9 },
      { pid: 701, user: 'root', name: 'cron', cmd: '/usr/sbin/cron -f', w: .5, mem: 3 },
      { pid: 1, user: 'root', name: 'systemd', cmd: '/sbin/init', w: .5, mem: 13 },
    ],
    net: {
      listen: [[22, 'sshd'], [8080, 'docker-proxy'], [25565, 'java'], [25577, 'java']],
      conns: [['198.51.100.7:51022', '25577', 'java', 'Velocity'], ['198.51.100.23:49811', '25577', 'java', 'Velocity'], ['203.0.113.50:50122', '22', 'sshd', 'SSH']],
    },
    mounts: [['/', 98], ['/mnt/backup', 916, 23]],
    base: { cpu: 28, mem: 63, disk: 43 },
    uptimeH: 12 * 24 + 3.2,
  },
  {
    id: 'db', name: 'db01', host: '203.0.113.11', lan: '192.168.1.11', user: 'asuta',
    tree: () => D('asuta', [
      D('app', [
        D('data', [F('ibdata1', 12582912), F('aria_log.00000001', 16384), D('app', [F('items.ibd', 4194304), F('users.ibd', 1048576)])]),
        F('docker-compose.yml', 310, TEXT.compose), F('app.env', 52, TEXT.appenv),
      ], true),
      D('backups', [F('db-2026-10-23.sql.gz', 431232000), F('db-2026-10-24.sql.gz', 432013312)]),
      D('logs', [F('backup.log', 412, TEXT.backuplog)]),
      F('.bashrc', 3771, TEXT.bashrc),
    ], true),
    screens: [],
    tmux: [{ name: 'db', desc: '1 ウィンドウ' }],
    docker: {
      containers: [
        { name: 'mariadb', image: 'mariadb:11', project: 'app', state: 'running', up: '12 days', upJa: '12 日', ports: ['3306'], mounts: [{ type: 'bind', src: '/home/asuta/app/data', dst: '/var/lib/mysql' }] },
        { name: 'app', image: 'node:22', project: 'app', state: 'running', up: '12 days', upJa: '12 日', ports: ['3000→3000'], mounts: [{ type: 'volume', src: 'app_node_modules', dst: '/app/node_modules' }] },
      ],
      images: [{ name: 'mariadb:11', size: 407e6 }, { name: 'node:22', size: 1.1e9 }, { name: 'adminer:4', size: 118e6 }],
      volumes: [{ name: 'app_node_modules', size: 210e6 }],
      compose: { app: '/home/asuta/app/docker-compose.yml' },
    },
    services: [
      { name: 'mariadb.service', desc: 'MariaDB', active: true },
      { name: 'docker.service', desc: 'Docker', active: true },
      { name: 'ssh.service', desc: 'OpenSSH サーバー', active: true },
    ],
    procs: [
      { pid: 903, user: '999', name: 'mariadbd', cmd: 'mariadbd --user=mysql', w: 45, mem: 820, container: 'mariadb' },
      { pid: 1188, user: 'node', name: 'node', cmd: 'node index.js', w: 18, mem: 184, container: 'app' },
      { pid: 790, user: 'root', name: 'dockerd', cmd: '/usr/bin/dockerd -H fd://', w: 6, mem: 88 },
      { pid: 801, user: 'root', name: 'containerd', cmd: '/usr/bin/containerd', w: 3, mem: 47 },
      { pid: 640, user: 'root', name: 'sshd', cmd: 'sshd: asuta@pts/0', w: 1, mem: 9 },
      { pid: 1, user: 'root', name: 'systemd', cmd: '/sbin/init', w: .5, mem: 12 },
    ],
    net: {
      listen: [[22, 'sshd'], [3000, 'docker-proxy'], [3306, 'docker-proxy']],
      conns: [['172.18.0.3:40122', '3306', 'mariadbd', 'MariaDB'], ['203.0.113.10:50122', '22', 'sshd', 'SSH']],
    },
    mounts: [['/', 98]],
    base: { cpu: 12, mem: 48, disk: 71 },
    uptimeH: 41 * 24 + 6.5,
  },
];

const SAVED = [
  { name: 'web01', host: '198.51.100.24', user: 'asuta', port: 22 },
  { name: 'pi', host: '192.168.1.20', user: 'pi', port: 22 },
  { name: 'router', host: '192.168.1.1', user: 'admin', port: 22, router: true },
];

// あとから接続したサーバーに入れる作り物のデータ
const GENERIC = {
  procs: [
    { pid: 1102, user: 'asuta', name: 'node', cmd: 'node index.js', w: 20, mem: 160, container: 'app' },
    { pid: 690, user: 'www-data', name: 'nginx', cmd: 'nginx: worker process', w: 5, mem: 14 },
    { pid: 780, user: 'root', name: 'dockerd', cmd: '/usr/bin/dockerd -H fd://', w: 5, mem: 80 },
    { pid: 620, user: 'root', name: 'sshd', cmd: 'sshd: asuta@pts/0', w: 1, mem: 8 },
    { pid: 1, user: 'root', name: 'systemd', cmd: '/sbin/init', w: .5, mem: 11 },
  ],
  docker: {
    containers: [{ name: 'app', image: 'node:22', state: 'running', up: '2 hours', upJa: '2 時間', ports: ['3000→3000'], mounts: [{ type: 'bind', src: '~/app', dst: '/app' }] }],
    images: [{ name: 'node:22', size: 1.1e9 }],
    volumes: [],
  },
  net: { listen: [[22, 'sshd'], [80, 'nginx'], [3000, 'docker-proxy']], conns: [['203.0.113.50:50122', '22', 'sshd', 'SSH']] },
  mounts: [['/', 49]],
};
const ROUTER = {
  procs: [
    { pid: 1, user: 'root', name: 'procd', cmd: '/sbin/procd', w: 1, mem: 2 },
    { pid: 412, user: 'root', name: 'dropbear', cmd: '/usr/sbin/dropbear -F -P /var/run/dropbear.pid', w: 2, mem: 1 },
    { pid: 980, user: 'dnsmasq', name: 'dnsmasq', cmd: '/usr/sbin/dnsmasq -C /var/etc/dnsmasq.conf', w: 3, mem: 2 },
    { pid: 1201, user: 'root', name: 'hostapd', cmd: '/usr/sbin/hostapd -s -g /var/run/hostapd/global', w: 4, mem: 3 },
  ],
  net: { listen: [[22, 'dropbear'], [53, 'dnsmasq'], [80, 'uhttpd']], conns: [['192.168.1.5:50122', '22', 'dropbear', 'SSH']] },
  mounts: [['/', 1]],
};

const HIST = [
  ['今日 09:12', 'Pixel 8', '198.51.100.77', 'ログイン'],
  ['10/23 22:40', 'iPad', '192.168.1.31', 'ログイン'],
  ['10/21 18:03', 'Pixel 8', '198.51.100.77', '登録'],
];

// スマホが踏み台にするこの PC の待ち受け
const PC = { lanUrl: 'https://192.168.1.20:8443', ip: '203.0.113.50', domain: 'drag.example.com' };
const publicUrl = () => {
  const host = state.settings.cert === 'domain' ? PC.domain : PC.ip;
  return state.settings.publicPort === 443 ? `https://${host}` : `https://${host}:${state.settings.publicPort}`;
};

const HELP_LINES = `このモックで使えるコマンド
  ls cd pwd cat echo clear whoami hostname uname date
  ssh <host>      docker ps / exec -it <name> bash
  screen -ls / -r <name> / -S <name>   (抜けるときは Ctrl+A のあと D)
  tmux ls / attach -t <name> / new -s <name>   (Ctrl+B のあと D)
  mysql  psql  python3  top  tail -f <file>  sleep <秒>`;

/* ---------- 状態 ---------- */

const DEFAULT_SETTINGS = {
  lang: 'ja', theme: 'dark', accent: '#5b9dff', desk: 'gradient', term: 'dark', termSize: 13,
  sb: { uptime: true, cpu: true, mem: true, net: true },
  autoOn: true, autoCmds: ['ssh', 'docker exec', 'docker attach', 'screen', 'tmux', 'mysql', 'psql'],
  snap: true, followLog: true, uploadConflict: 'ask',
  phoneLan: true, phonePublic: true, publicPort: 443, cert: 'ip',
};
const ACCENTS = [['#5b9dff', '青'], ['#8b7bff', '紫'], ['#3fcf8e', '緑'], ['#f1a33c', '橙'], ['#ff6b9d', '桃'], ['#2fc3d9', '水色']];

const state = {
  list: [], active: null, view: 'files', settings: loadSettings(), sbRefs: null,
  drag: null, ctrlArmed: false, menu: null, menuBackdrop: null, uploadTarget: null, taskDragged: false,
  devices: [{ name: 'Pixel 8', last: '今日 09:12', ip: '198.51.100.77', route: 'public' }, { name: 'iPad', last: '10/23 22:40', ip: '192.168.1.31', route: 'lan' }],
};
const cur = () => state.active;
let uploadSeq = 0;
const TEXT_EXT = /\.(txt|log|md|json|ya?ml|toml|properties|sh|js|ts|py|conf|ini|env|csv|xml|html|css)$/i;

/* ---------- ファイルツリー ---------- */

function link(node, parent) {
  node.parent = parent;
  if (parent) node.path = parent.path === '/' ? `/${node.name}` : `${parent.path}/${node.name}`;
  if (node.dir) node.children.forEach(c => link(c, node));
}

function findNode(srv, path) {
  let n = srv.root;
  for (const part of path.split('/')) {
    if (!part) continue;
    n = n.dir ? n.children.find(c => c.name === part) : null;
    if (!n) return null;
  }
  return n;
}

const isUnder = (n, dir) => n === dir || n.path.startsWith(dir.path === '/' ? '/' : dir.path + '/');
const inHome = (srv, n) => isUnder(n, srv.homeNode);
const setOwner = (n, owner) => { n.owner = owner; if (n.dir) n.children.forEach(c => setOwner(c, owner)); };

// ホームの外のファイル。root が持ち、sudo モードのときだけ開ける
function systemTree(srv, home) {
  const u = srv.user;
  const etc = D('etc', [
    F('hosts', 64, `127.0.0.1 localhost\n127.0.1.1 ${srv.name}\n`),
    F('fstab', 240, '# <file system> <mount point> <type> <options> <dump> <pass>\n/dev/disk/by-uuid/00000000-0000-0000-0000-000000000001 / ext4 defaults 0 1\n/dev/disk/by-uuid/00000000-0000-0000-0000-000000000002 /mnt/backup ext4 defaults,nofail 0 2\n'),
    D('nginx', [
      F('nginx.conf', 330, 'user www-data;\nworker_processes auto;\npid /run/nginx.pid;\n\nevents {\n    worker_connections 768;\n}\n\nhttp {\n    sendfile on;\n    include /etc/nginx/mime.types;\n    access_log /var/log/nginx/access.log;\n    error_log /var/log/nginx/error.log;\n    include /etc/nginx/sites-enabled/*;\n}\n'),
      D('sites-enabled', [F('default', 160, 'server {\n    listen 80 default_server;\n    root /var/www/html;\n    index index.html;\n    location / {\n        try_files $uri $uri/ =404;\n    }\n}\n')]),
    ]),
    D('ssh', [F('sshd_config', 230, 'Include /etc/ssh/sshd_config.d/*.conf\nPort 22\nPermitRootLogin no\nPasswordAuthentication yes\nKbdInteractiveAuthentication no\nUsePAM yes\nX11Forwarding no\nSubsystem sftp /usr/lib/openssh/sftp-server\n')]),
    D('systemd', [D('system', [
      ...(srv.id === 'mc' ? [F('mc-server.service', 250, `[Unit]\nDescription=Minecraft サーバー\nAfter=network.target\n\n[Service]\nUser=${u}\nWorkingDirectory=/home/${u}/mc\nExecStart=/usr/bin/screen -DmS mc ./start.sh\nRestart=on-failure\n\n[Install]\nWantedBy=multi-user.target\n`)] : []),
    ])]),
  ]);
  const log = D('log', [
    F('syslog', 620, [`Oct 24 09:00:01 ${srv.name} CRON[2210]: (root) CMD (command -v debian-sa1 > /dev/null && debian-sa1 1 1)`,
      `Oct 24 09:12:44 ${srv.name} systemd[1]: Started session-31.scope - Session 31 of User ${u}.`,
      `Oct 24 09:17:01 ${srv.name} CRON[2390]: (root) CMD (cd / && run-parts --report /etc/cron.hourly)`].join('\n')),
    F('auth.log', 540, [`Oct 24 09:12:44 ${srv.name} sshd[3101]: Accepted password for ${u} from 203.0.113.50 port 50122 ssh2`,
      `Oct 24 09:12:44 ${srv.name} sshd[3101]: pam_unix(sshd:session): session opened for user ${u}(uid=1000) by (uid=0)`,
      `Oct 24 09:20:13 ${srv.name} sudo: ${u} : TTY=pts/0 ; PWD=/home/${u} ; USER=root ; COMMAND=/usr/bin/systemctl restart nginx`].join('\n')),
    D('nginx', [F('access.log', 0, ''), F('error.log', 0, '')]),
  ]);
  const sys = [etc, D('opt', []), Object.assign(D('root', [F('.bashrc', 3106, TEXT.bashrc)]), { private: true }),
    D('usr', [D('bin', []), D('lib', [])]), D('var', [log, D('lib', [D('docker', [])])])];
  sys.forEach(n => setOwner(n, 'root'));
  const homeDir = D('home', [home]);
  homeDir.owner = 'root';
  const root = D('', [...sys, homeDir], true);
  root.owner = 'root';
  root.path = '/';
  return root;
}

function resolvePath(srv, cwd, p) {
  if (!p || p === '~') return srv.home;
  let base, rest;
  if (p.startsWith('/')) { base = []; rest = p; }
  else if (p.startsWith('~')) { base = srv.home.split('/').filter(Boolean); rest = p.slice(1); }
  else { base = cwd.split('/').filter(Boolean); rest = p; }
  for (const seg of rest.split('/')) {
    if (!seg || seg === '.') continue;
    if (seg === '..') base.pop(); else base.push(seg);
  }
  return '/' + base.join('/');
}

const tilde = (srv, p) => p === srv.home ? '~' : p.startsWith(srv.home + '/') ? '~' + p.slice(srv.home.length) : p;

function pathArg(srv, p) {
  const t = tilde(srv, p);
  return /[\s'"$`\\!&|;<>()*?#]/.test(t) ? `'${p.replace(/'/g, `'\\''`)}'` : t;
}

const sortKids = n => [...n.children].sort((a, b) => (b.dir - a.dir) || a.name.localeCompare(b.name));

function fileKind(n) {
  if (n.dir) return 'dir';
  const name = n.name.toLowerCase();
  if (name.endsWith('.log')) return 'log';
  if (/\.(png|jpe?g|gif|webp)$/.test(name)) return 'img';
  if (/\.(gz|tgz|tar|zip|jar|7z)$/.test(name)) return 'arc';
  if (/\.(mca|dat|class|ibd|bin)$/.test(name) || /^ibdata\d|^aria_log/.test(name)) return 'bin';
  if (/\.(sh|json|ya?ml|toml|properties|js|ts|py|env|conf|ini)$/.test(name)) return 'code';
  return 'text';
}
const KIND_ICON = { dir: ['folder', 'f-dir'], log: ['log', 'f-log'], img: ['image', 'f-img'], arc: ['archive', 'f-arc'], bin: ['file', 'f-arc'], code: ['code', 'f-code'], text: ['file', 'f-file'] };

/* ---------- ログの配信 ---------- */

const logSubs = new Map();
function subscribeLog(srv, path, fn) {
  const k = srv.id + path;
  if (!logSubs.has(k)) logSubs.set(k, new Set());
  logSubs.get(k).add(fn);
  return () => logSubs.get(k)?.delete(fn);
}
function emitLog(srv, path, line, n = findNode(srv, path)) {
  if (n) {
    let lines = (n.content ? n.content + '\n' : '') + line;
    const arr = lines.split('\n');
    if (arr.length > 600) lines = arr.slice(-500).join('\n');
    n.content = lines;
    n.size = (n.size || 0) + line.length + 1;
  }
  logSubs.get(srv.id + path)?.forEach(f => f(line));
}
function logParts(line) {
  const m = line.match(/^(\[[^\]]*\]:?\s?)(.*)$/);
  const head = m ? m[1] : '', body = m ? m[2] : line;
  const c = /\bWARN\b/.test(head) || /\[Warning\]|" [45]\d\d /.test(body) ? 'warn'
    : /\bERROR\b/.test(head) || /\[ERROR\]/.test(body) ? 'err'
    : /joined the game|Done \(|backup done/.test(body) ? 'ok' : '';
  return { head, body, c };
}
const colorLog = line => { const p = logParts(line); return [{ t: p.head, c: 'dim' }, { t: p.body, c: p.c }]; };

/* ---------- セッション ---------- */

class Session {
  constructor(o = {}) {
    Object.assign(this, {
      kind: 'shell', match: '', title: '', icon: 'terminal', cmdline: '', promptText: '',
      raw: false, altScreen: false, detachable: false, footer: null, prefixKey: null,
    }, o);
    this.lines = []; this.history = []; this.subs = new Set();
    this.ended = false; this.timers = []; this.unsubs = []; this.window = null;
    this.onInput = this.onInput || null;
    this.onCtrlC = this.onCtrlC || null;
  }
  on(fn) { this.subs.add(fn); return () => this.subs.delete(fn); }
  emit(type, data) { for (const fn of [...this.subs]) fn(type, data); }
  prompt() { return typeof this.promptText === 'function' ? this.promptText() : this.promptText; }
  write(x, cls = '') {
    const add = Array.isArray(x) ? [x] : String(x).split('\n').map(t => [{ t, c: cls }]);
    this.lines.push(...add);
    if (this.lines.length > 800) { this.lines.splice(0, this.lines.length - 600); this.emit('reset'); }
    else this.emit('add', add);
  }
  appendLines(lines) { if (!lines.length) return; this.lines.push(...lines); this.emit('add', lines); }
  note(t, mark = '↗') { this.write(`${mark} ${t}`, 'note'); }
  echo(text) { this.write([{ t: this.prompt(), c: 'prompt' }, { t: text, c: 'cmd' }]); }
  setLines(rows) { this.lines = rows.map(r => Array.isArray(r) ? r : [{ t: r, c: '' }]); this.emit('reset'); }
  clear() { this.lines = []; this.emit('reset'); }
  input(text) {
    if (this.ended) return;
    this.echo(text);
    if (text.trim()) this.history.push(text);
    this.onInput?.(text, this);
    this.emit('prompt');
  }
  ctrlC() { if (this.onCtrlC) this.onCtrlC(this); else this.echo('^C'); this.emit('prompt'); }
  every(ms, fn) { this.timers.push(setInterval(fn, ms)); }
  later(ms, fn) { this.timers.push(setTimeout(fn, ms)); }
  end(msg) {
    if (this.ended) return;
    this.ended = true;
    this.timers.forEach(t => { clearInterval(t); clearTimeout(t); });
    this.unsubs.forEach(f => f());
    if (msg) this.write(msg, 'dim');
    this.emit('end');
  }
}

/* ---------- ターミナルの表示 ---------- */

function lineEl(segs) {
  const d = el('div', { class: 'tl' });
  for (const s of segs) d.append(s.c ? el('span', { class: 't-' + s.c }, s.t) : s.t);
  if (!d.textContent) d.append(' ');
  return d;
}

class TermView {
  constructor(root) {
    this.root = root;
    root.classList.add('term');
    this.outBase = el('div', { class: 'term-out' });
    this.outFg = el('div', { class: 'term-out' });
    this.promptEl = el('span', { class: 'prompt' });
    this.input = el('input', { class: 'term-input', spellcheck: 'false', autocomplete: 'off', autocapitalize: 'off', autocorrect: 'off', enterkeyhint: 'send', 'aria-label': 'コマンド' });
    this.inLine = el('div', { class: 'term-in' }, this.promptEl, this.input);
    this.scroller = el('div', { class: 'term-scroll' }, this.outBase, this.outFg, this.inLine);
    this.footerEl = el('div', { class: 'term-footer', hidden: true });
    root.append(this.scroller, this.footerEl);
    this.hist = -1;
    this.prefixArmed = false;
    // 自分でスクロールして読んでいる間は、新しい行が来ても末尾へ送らない
    this.stick = true;
    const userScroll = () => requestAnimationFrame(() => { this.stick = this.atBottom(); });
    this.scroller.addEventListener('wheel', userScroll, { passive: true });
    this.scroller.addEventListener('touchmove', userScroll, { passive: true });
    this.scroller.addEventListener('pointerup', userScroll);
    this.ro = new ResizeObserver(() => { if (this.stick) this.scroller.scrollTop = this.scroller.scrollHeight; });
    this.ro.observe(this.scroller);
    root.addEventListener('click', () => { if (!String(getSelection())) this.focus(); });
    this.input.addEventListener('keydown', e => this.onKey(e));
    this.input.addEventListener('input', e => this.onInput(e));
  }
  get active() { return this.fg || this.base; }
  focus() { this.input.focus({ preventScroll: true }); }
  atBottom() { const s = this.scroller; return s.scrollHeight - s.scrollTop - s.clientHeight < 40; }
  bind(sess, out) {
    out.replaceChildren(...sess.lines.map(lineEl));
    return sess.on((type, data) => {
      if (type === 'add') {
        for (const l of data) out.append(lineEl(l));
        while (out.childElementCount > 800) out.firstChild.remove();
        this.refresh(this.stick);
      } else if (type === 'reset') {
        out.replaceChildren(...sess.lines.map(lineEl));
        this.refresh(true);
      } else this.refresh(true);
    });
  }
  setBase(sess) { this.offBase?.(); this.base = sess; this.offBase = this.bind(sess, this.outBase); this.refresh(true); }
  setFg(sess) {
    this.offFg?.(); this.offFg = null;
    this.fg = sess || null;
    if (sess) this.offFg = this.bind(sess, this.outFg); else this.outFg.replaceChildren();
    this.refresh(true);
  }
  refresh(scroll) {
    const s = this.active;
    if (!s) return;
    this.outBase.hidden = !!(this.fg && this.fg.altScreen);
    this.promptEl.textContent = s.prompt();
    this.inLine.classList.toggle('raw', !!s.raw);
    this.inLine.hidden = s.ended;
    this.footerEl.hidden = !s.footer;
    if (s.footer) this.footerEl.textContent = s.footer;
    if (scroll) { this.scroller.scrollTop = this.scroller.scrollHeight; this.stick = true; }
  }
  insert(text, bare) {
    const i = this.input;
    const a = i.selectionStart ?? i.value.length, b = i.selectionEnd ?? a;
    const before = i.value.slice(0, a);
    const add = !bare && before && !/\s$/.test(before) ? ' ' + text : text;
    i.value = before + add + i.value.slice(b);
    this.focus();
    i.setSelectionRange(a + add.length, a + add.length);
  }
  onInput(e) {
    const s = this.active;
    if (s?.raw) { for (const ch of this.input.value) s.rawKey?.(ch); this.input.value = ''; return; }
    if (!state.ctrlArmed || !e.data) return;
    const ch = e.data.slice(-1);
    this.input.value = this.input.value.slice(0, -e.data.length);
    setCtrl(false);
    this.onKey({ key: ch, ctrlKey: true, preventDefault() {} });
  }
  onKey(e) {
    const s = this.active;
    if (!s || s.ended) return;
    const k = e.key.length === 1 ? e.key.toLowerCase() : e.key;
    if (s.prefixKey && e.ctrlKey && k === s.prefixKey) { e.preventDefault(); this.prefixArmed = true; return; }
    if (this.prefixArmed) {
      this.prefixArmed = false;
      if (k === 'd') { e.preventDefault(); s.emit('detach'); return; }
    }
    if (e.ctrlKey && k === 'c') {
      if (String(getSelection())) return;
      e.preventDefault(); this.input.value = ''; s.ctrlC(); return;
    }
    if (e.ctrlKey && k === 'l') { e.preventDefault(); s.clear(); return; }
    if (s.raw) {
      if (e.key.length === 1 || e.key === 'Escape') { e.preventDefault(); s.rawKey?.(e.key); }
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      const v = this.input.value;
      this.input.value = ''; this.hist = -1;
      s.input(v);
    } else if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
      e.preventDefault();
      const h = s.history;
      if (!h.length) return;
      if (e.key === 'ArrowUp') this.hist = this.hist < 0 ? h.length - 1 : Math.max(0, this.hist - 1);
      else this.hist = this.hist < 0 || this.hist >= h.length - 1 ? -1 : this.hist + 1;
      this.input.value = this.hist < 0 ? '' : h[this.hist];
    } else if (e.key === 'Tab') {
      e.preventDefault();
      const r = s.complete?.(this.input.value);
      if (r != null) this.input.value = r;
    }
  }
  dispose() { this.offBase?.(); this.offFg?.(); this.ro.disconnect(); }
}

/* ---------- シェルと前面のコマンド ---------- */

class ShellHost {
  constructor(srv, view, o = {}) {
    this.srv = srv; this.view = view; this.chip = !!o.chip; this.win = o.win || null;
    this.shell = makeShell(srv, this, o.cwd);
    this.fg = null;
    view.setBase(this.shell);
  }
  launch(sess) {
    const auto = state.settings.autoOn && state.settings.autoCmds.includes(sess.match);
    if (auto) this.popOutNew(sess); else this.setForeground(sess);
  }
  setForeground(sess) {
    this.fg = sess;
    this.view.setFg(sess);
    this.offFg = sess.on(t => {
      if (this.fg !== sess) return;
      if (t === 'end') this.release(!sess.altScreen);
      else if (t === 'detach') {
        this.release(false);
        this.shell.write(sess.kind === 'tmux' ? `[detached (from session ${sess.name})]` : `[detached from ${sess.pid}.${sess.name}]`, 'dim');
      }
    });
    this.changed();
  }
  release(merge) {
    const s = this.fg;
    if (!s) return null;
    this.offFg?.();
    this.fg = null;
    if (merge) this.shell.appendLines(s.lines);
    this.view.setFg(null);
    this.changed();
    return s;
  }
  popOut(pos) {
    if (!this.fg) return;
    const from = this.view.inLine.getBoundingClientRect();
    const s = this.release(false);
    this.shell.note(`${s.cmdline || s.title} をウィンドウに移しました`);
    showMoved(openSessionWindow(this.srv, s, pos), from, s);
  }
  popOutNew(sess) {
    const from = this.view.inLine.getBoundingClientRect();
    this.shell.note(`${sess.cmdline} をウィンドウに移しました`);
    showMoved(openSessionWindow(this.srv, sess), from, sess);
  }
  takeBack(win) {
    if (this.fg) { toast('ターミナルで別のコマンドが動いています', 'warn'); return false; }
    const s = win.session;
    closeWindow(win, { keepSession: true });
    this.shell.note(`${s.cmdline || s.title} をターミナルに戻しました`, '↙');
    this.setForeground(s);
    this.view.focus();
    return true;
  }
  changed() {
    if (this.chip && this.srv === cur()) updateChip();
    renderSide();
  }
}

function showMoved(win, from, sess) {
  if (!win) return;
  if (isMobile() && $('#app').dataset.mtab !== 'desktop') {
    toast(`${sess.title} をウィンドウに移しました`, 'info', { action: { label: '開く', fn: () => { setMTab('desktop'); focusWin(win); } } });
    return;
  }
  flyGhost(from, win.el, sess.title);
}

function flyGhost(from, toEl, label) {
  const to = toEl.getBoundingClientRect();
  if (!to.width || !from.width) return;
  const g = el('div', { class: 'fly-ghost' }, ic('terminal'), el('span', {}, label));
  document.body.append(g);
  g.animate([
    { left: `${from.left}px`, top: `${from.top - 4}px`, width: `${Math.min(from.width, 420)}px`, height: '30px', opacity: 1 },
    { left: `${to.left}px`, top: `${to.top}px`, width: `${to.width}px`, height: `${to.height}px`, opacity: 0 },
  ], { duration: 460, easing: 'cubic-bezier(.2,.7,.2,1)' }).onfinish = () => g.remove();
}

function makeShell(srv, host, cwd) {
  const s = new Session({ kind: 'shell', title: 'bash' });
  s.cwd = cwd || srv.home;
  s.promptText = () => `${srv.user}@${srv.name}:${tilde(srv, s.cwd)}$ `;
  s.onInput = line => runShell(srv, host, s, line);
  s.complete = text => completePath(srv, s, text);
  return s;
}

function completePath(srv, s, text) {
  const tok = text.match(/(\S*)$/)[1];
  const dirPart = tok.includes('/') ? tok.slice(0, tok.lastIndexOf('/') + 1) : '';
  const base = tok.slice(dirPart.length);
  const dir = findNode(srv, resolvePath(srv, s.cwd, dirPart || '.'));
  if (!dir?.dir) return null;
  const hits = sortKids(dir).filter(c => c.name.startsWith(base) && (base.startsWith('.') || !c.name.startsWith('.')));
  if (!hits.length) return null;
  const head = text.slice(0, text.length - tok.length) + dirPart;
  if (hits.length === 1) return head + hits[0].name + (hits[0].dir ? '/' : ' ');
  s.echo(text);
  s.write(hits.flatMap(h => [{ t: h.name, c: h.dir ? 'dir' : '' }, { t: '  ' }]));
  let common = hits[0].name;
  for (const h of hits) while (!h.name.startsWith(common)) common = common.slice(0, -1);
  return head + common;
}

function basicCmd(srv, s, c, a) {
  const out = (x, cls) => s.write(x, cls);
  switch (c) {
    case 'clear': s.clear(); return true;
    case 'pwd': out(s.cwd); return true;
    case 'whoami': out(srv.user); return true;
    case 'hostname': out(srv.name); return true;
    case 'date': out(new Date().toString()); return true;
    case 'echo': out(a.join(' ')); return true;
    case 'uname': out(a.includes('-a') ? `Linux ${srv.name} 6.8.0-45-generic #45-Ubuntu SMP x86_64 GNU/Linux` : 'Linux'); return true;
    case 'ls': case 'll': cmdLs(srv, s, c === 'll' ? ['-l', ...a] : a); return true;
    case 'cd': cmdCd(srv, s, a[0]); return true;
    case 'cat': cmdCat(srv, s, a); return true;
  }
  return false;
}

function cmdLs(srv, s, a) {
  const flags = a.filter(x => x.startsWith('-')).join('');
  const target = a.find(x => !x.startsWith('-'));
  const n = findNode(srv, resolvePath(srv, s.cwd, target || '.'));
  if (!n) return s.write(`ls: cannot access '${target}': No such file or directory`, 'err');
  if (!n.dir) return s.write(n.name);
  if (n.private) return s.write(`ls: cannot open directory '${target || n.path}': Permission denied`, 'err');
  const kids = sortKids(n).filter(k => flags.includes('a') || !k.name.startsWith('.'));
  const cls = k => k.dir ? 'dir' : fileKind(k) === 'arc' ? 'hl' : '';
  if (flags.includes('l')) {
    s.write(`total ${kids.length * 4}`);
    for (const k of kids) {
      s.write([{ t: k.dir ? 'drwxr-xr-x' : '-rw-r--r--', c: 'dim' }, { t: ` 1 ${(k.owner || srv.user).padEnd(5)} ${(k.owner || srv.user).padEnd(5)} ${String(k.dir ? 4096 : k.size).padStart(10)} Oct 24 09:00 ` }, { t: k.name, c: cls(k) }]);
    }
  } else if (kids.length) {
    s.write(kids.flatMap(k => [{ t: k.name, c: cls(k) }, { t: '  ' }]));
  }
}

function cmdCd(srv, s, arg) {
  const p = resolvePath(srv, s.cwd, arg || '~');
  const n = findNode(srv, p);
  if (!n) s.write(`cd: ${arg}: No such file or directory`, 'err');
  else if (!n.dir) s.write(`cd: ${arg}: Not a directory`, 'err');
  else if (n.private) s.write(`-bash: cd: ${arg}: Permission denied`, 'err');
  else s.cwd = p;
}

function cmdCat(srv, s, a) {
  for (const arg of a) {
    const n = findNode(srv, resolvePath(srv, s.cwd, arg));
    if (!n) s.write(`cat: ${arg}: No such file or directory`, 'err');
    else if (n.dir) s.write(`cat: ${arg}: Is a directory`, 'err');
    else if (n.content == null) s.write(`(${n.name} はバイナリのファイルです)`, 'dim');
    else s.write(n.content);
  }
}

function runShell(srv, host, s, line) {
  const t = line.trim();
  const a = tokenize(t);
  const c = a.shift();
  if (!c) return;
  if (basicCmd(srv, s, c, a)) return;
  const out = (x, cls) => s.write(x, cls);
  switch (c) {
    case 'help': return out(HELP_LINES, 'dim');
    case 'screen': return cmdScreen(srv, host, s, a, t);
    case 'tmux': return cmdTmux(srv, host, s, a, t);
    case 'ssh': {
      const target = a.filter(x => !x.startsWith('-')).pop();
      if (!target) return out('usage: ssh [-p port] destination', 'err');
      return host.launch(sessSSH(srv, target, t));
    }
    case 'docker': return cmdDocker(srv, host, s, a, t);
    case 'mysql': case 'mariadb': return host.launch(sessMysql(t));
    case 'psql': return host.launch(sessPsql(t));
    case 'python3': case 'python': return host.launch(sessPython(t));
    case 'top': case 'htop': return host.launch(sessTop(srv, t, c));
    case 'tail': return cmdTail(srv, host, s, a, t);
    case 'sleep': {
      const n = parseFloat(a[0]);
      if (!(n >= 0)) return out('sleep: missing operand', 'err');
      return host.launch(sessSleep(n, t));
    }
    case 'exit': case 'logout':
      if (host.win) return closeWindow(host.win);
      return out('このモックでは下のターミナルは閉じません', 'dim');
    case 'sudo': return out('sudo: このモックでは使えません', 'dim');
  }
  out(`${c}: command not found`, 'err');
}

function cmdScreen(srv, host, s, a, line) {
  const out = (x, c) => s.write(x, c);
  const usage = () => out('このモックでは screen -ls / -r 名前 / -S 名前 を使えます', 'dim');
  if (!a.length) return usage();
  if (a[0] === '-ls' || a[0] === '-list') {
    if (!srv.screens.length) return out(`No Sockets found in /run/screen/S-${srv.user}.`);
    out('There are screens on:');
    srv.screens.forEach((d, i) => {
      const p = srv.persist.get('screen:' + d.name);
      const attached = p && (p.window || srv.panel.fg === p);
      out(`\t${1201 + i * 37}.${d.name}\t(${attached ? 'Attached' : 'Detached'})`);
    });
    return out(`${srv.screens.length} Sockets in /run/screen/S-${srv.user}.`);
  }
  if (a[0] === '-S' && a[1]) {
    if (!srv.screens.some(d => d.name === a[1])) srv.screens.push({ name: a[1], desc: '新しいセッション' });
    return attachPersistent(srv, host, s, 'screen', a[1], line);
  }
  if (/^-(r|x|R|dr|rd|Dr|rD)$/.test(a[0])) {
    const name = a[1] || srv.screens[0]?.name;
    if (!name || !srv.screens.some(d => d.name === name)) return out(`There is no screen to be resumed matching ${a[1] || ''}.`);
    return attachPersistent(srv, host, s, 'screen', name, line);
  }
  usage();
}

function cmdTmux(srv, host, s, a, line) {
  const out = (x, c) => s.write(x, c);
  const sub = a[0];
  if (sub === 'ls' || sub === 'list-sessions') {
    if (!srv.tmux.length) return out('no server running on /tmp/tmux-1000/default');
    srv.tmux.forEach(d => out(`${d.name}: ${d.desc.replace(' ウィンドウ', ' windows')} (created Thu Oct 23 21:14:02 2026)`));
    return;
  }
  if (['attach', 'a', 'at', 'attach-session'].includes(sub)) {
    const ti = a.indexOf('-t');
    const name = ti >= 0 ? a[ti + 1] : srv.tmux[0]?.name;
    if (!name || !srv.tmux.some(d => d.name === name)) return out(name ? `can't find session: ${name}` : 'no sessions');
    return attachPersistent(srv, host, s, 'tmux', name, line);
  }
  if (sub === 'new' || sub === 'new-session') {
    const si = a.indexOf('-s');
    const name = si >= 0 ? a[si + 1] : String(srv.tmux.length);
    if (!srv.tmux.some(d => d.name === name)) srv.tmux.push({ name, desc: '1 ウィンドウ' });
    return attachPersistent(srv, host, s, 'tmux', name, line);
  }
  out('このモックでは tmux ls / attach -t 名前 / new -s 名前 を使えます', 'dim');
}

function attachPersistent(srv, host, s, kind, name, line) {
  const p = getPersistent(srv, kind, name);
  if (p.window) { restoreFocus(p.window); s.note(`${p.title} はウィンドウで開いています`); return; }
  if (srv.panel.fg === p) { s.write('このセッションは下のターミナルで開いています', 'dim'); return; }
  p.cmdline = line;
  host.launch(p);
}

function cmdDocker(srv, host, s, a, line) {
  const out = (x, c) => s.write(x, c);
  const d = srv.docker;
  if (!d) return out('docker: command not found', 'err');
  if (a[0] === 'ps') {
    out('CONTAINER ID   IMAGE          STATUS                  NAMES', 'hl');
    d.containers.filter(c => a.includes('-a') || c.state === 'running').forEach(c => {
      const st = c.state === 'running' ? `Up ${c.up}` : `Exited (${c.exit ?? 0}) ${c.ago} ago`;
      out(`${hex(c.name)}   ${c.image.padEnd(14)} ${st.padEnd(23)} ${c.name}`);
    });
    return;
  }
  if (a[0] === 'exec' || a[0] === 'attach') {
    const name = a.slice(1).find(x => !x.startsWith('-'));
    if (!name) return out(`"docker ${a[0]}" requires at least 1 argument.`, 'err');
    const c = d.containers.find(x => x.name === name);
    if (!c) return out(`Error response from daemon: No such container: ${name}`, 'err');
    if (c.state !== 'running') return out(`Error response from daemon: container ${hex(name)} is not running`, 'err');
    return host.launch(sessDocker(name, a[0], line));
  }
  out('このモックでは docker ps / exec / attach を使えます', 'dim');
}

function cmdTail(srv, host, s, a, line) {
  const follow = a.some(x => x === '-f' || x === '-F');
  const target = a.filter(x => !x.startsWith('-')).pop();
  if (!target) return s.write('tail: ファイルを指定してください', 'err');
  const n = findNode(srv, resolvePath(srv, s.cwd, target));
  if (!n || n.dir) return s.write(`tail: cannot open '${target}' for reading: No such file or directory`, 'err');
  if (!follow) { (n.content || '').split('\n').filter(Boolean).slice(-10).forEach(l => s.write(colorLog(l))); return; }
  host.launch(sessTail(srv, n, line));
}

/* ---------- コマンドごとのセッション ---------- */

function sessSSH(srv, target, cmdline) {
  const [user, host] = target.includes('@') ? target.split('@') : [srv.user, target];
  const s = new Session({ kind: 'ssh', match: 'ssh', title: `ssh: ${host}`, icon: 'link', cmdline, promptText: `${user}@${host}:~$ ` });
  s.write(`Welcome to Ubuntu 24.04.1 LTS (GNU/Linux 6.8.0-45-generic x86_64)\n\nLast login: Fri Oct 24 08:51:10 2026 from ${srv.host}`);
  s.onInput = line => {
    const [c] = tokenize(line.trim());
    if (!c) return;
    if (c === 'exit' || c === 'logout') { s.write('logout'); s.end(`Connection to ${host} closed.`); }
    else if (c === 'ls') s.write([{ t: 'backups', c: 'dir' }, { t: '  ' }, { t: 'logs', c: 'dir' }, { t: '  docker-compose.yml' }]);
    else if (c === 'hostname') s.write(host);
    else if (c === 'whoami') s.write(user);
    else if (c === 'pwd') s.write(`/home/${user}`);
    else s.write(`${c}: command not found`, 'err');
  };
  return s;
}

function sessDocker(name, sub, cmdline) {
  const s = new Session({ kind: 'docker', match: `docker ${sub}`, title: `docker: ${name}`, icon: 'box', cmdline, promptText: `root@${hex(name).slice(0, 12)}:/app# ` });
  s.onInput = line => {
    const [c] = tokenize(line.trim());
    if (!c) return;
    if (c === 'exit') s.end();
    else if (c === 'ls') s.write([{ t: 'node_modules', c: 'dir' }, { t: '  package.json  server.js' }]);
    else if (c === 'hostname') s.write(hex(name).slice(0, 12));
    else s.write(`bash: ${c}: command not found`, 'err');
  };
  return s;
}

function sessMysql(cmdline) {
  const s = new Session({ kind: 'db', match: 'mysql', title: 'mysql', icon: 'db', cmdline, promptText: 'MariaDB [(none)]> ' });
  s.write(`Welcome to the MariaDB monitor.  Commands end with ; or \\g.
Your MariaDB connection id is 31
Server version: 10.11.8-MariaDB-0ubuntu0.24.04.1 Ubuntu 24.04

Type 'help;' or '\\h' for help. Type '\\c' to clear the current input statement.
`);
  s.onInput = line => {
    const t = line.trim();
    if (!t) return;
    if (/^(exit|quit|\\q);?$/i.test(t)) { s.write('Bye'); s.end(); }
    else if (/^show databases;?$/i.test(t)) s.write('+--------------------+\n| Database           |\n+--------------------+\n| app                |\n| information_schema |\n| mysql              |\n+--------------------+\n3 rows in set (0.001 sec)');
    else if (/^use (\w+);?$/i.test(t)) { const db = t.match(/^use (\w+)/i)[1]; s.promptText = `MariaDB [${db}]> `; s.write('Database changed'); }
    else s.write('ERROR 1064 (42000): You have an error in your SQL syntax', 'err');
  };
  return s;
}

function sessPsql(cmdline) {
  const s = new Session({ kind: 'db', match: 'psql', title: 'psql', icon: 'db', cmdline, promptText: 'postgres=# ' });
  s.write('psql (16.4 (Ubuntu 16.4-0ubuntu0.24.04.2))\nType "help" for help.\n');
  s.onInput = line => {
    const t = line.trim();
    if (!t) return;
    if (t === '\\q') s.end();
    else if (t === '\\l') s.write('   Name    |  Owner\n-----------+----------\n app       | postgres\n postgres  | postgres\n(2 rows)');
    else s.write(`ERROR:  syntax error at or near "${t.split(/\s/)[0]}"`, 'err');
  };
  return s;
}

function sessPython(cmdline) {
  const s = new Session({ kind: 'proc', match: 'python3', title: 'python3', icon: 'terminal', cmdline, promptText: '>>> ' });
  s.write('Python 3.12.3 (main, Feb  4 2026, 14:48:35) [GCC 13.3.0] on linux\nType "help", "copyright", "credits" or "license" for more information.');
  s.onInput = line => {
    const t = line.trim();
    if (!t) return;
    if (/^(exit|quit)\(\)$/.test(t)) return s.end();
    const m = t.match(/^print\((['"])(.*)\1\)$/);
    if (m) return s.write(m[2]);
    if (/^[\d\s+\-*/().%]+$/.test(t)) {
      try { s.write(String(Function(`"use strict"; return (${t})`)())); } catch { s.write('SyntaxError: invalid syntax', 'err'); }
      return;
    }
    s.write(`Traceback (most recent call last):\n  File "<stdin>", line 1, in <module>\nNameError: name '${t.split(/\W/)[0] || t}' is not defined`, 'err');
  };
  s.onCtrlC = () => s.write('KeyboardInterrupt', 'err');
  return s;
}

function sessTail(srv, node, cmdline) {
  const s = new Session({ kind: 'proc', match: 'tail', title: `tail -f ${node.name}`, icon: 'log', cmdline, raw: true });
  (node.content || '').split('\n').filter(Boolean).slice(-10).forEach(l => s.write(colorLog(l)));
  s.unsubs.push(subscribeLog(srv, node.path, l => s.write(colorLog(l))));
  s.onCtrlC = () => { s.write('^C', 'dim'); s.end(); };
  return s;
}

function sessTop(srv, cmdline, name) {
  const s = new Session({ kind: 'proc', match: name, title: name, icon: 'chart', cmdline, raw: true, altScreen: true });
  const draw = () => {
    const cpu = last(srv.metrics.cpu), mem = last(srv.metrics.mem);
    const rows = liveProcs(srv).sort((x, y) => y.cpu - x.cpu).slice(0, 8);
    s.setLines([
      `top - ${clock()} up ${topUptime(Date.now() - srv.bootAt)},  1 user,  load average: ${(cpu / 40).toFixed(2)}, 0.51, 0.48`,
      'Tasks: 142 total,   1 running, 141 sleeping,   0 stopped,   0 zombie',
      `%Cpu(s): ${cpu.toFixed(1)} us,  1.2 sy,  0.0 ni, ${(98.8 - cpu).toFixed(1)} id`,
      `MiB Mem :   7838.2 total,   ${(7838 * (1 - mem / 100)).toFixed(1)} free,   ${(7838 * mem / 100).toFixed(1)} used`,
      '',
      [{ t: '    PID USER       %CPU  %MEM  COMMAND', c: 'hl' }],
      ...rows.map(p => `${String(p.pid).padStart(7)} ${p.user.padEnd(9)} ${(p.cpu * 4).toFixed(1).padStart(5)} ${(p.rss / MEM_TOTAL * 100).toFixed(1).padStart(5)}  ${p.name}`),
      '',
      [{ t: 'q で終了', c: 'dim' }],
    ]);
  };
  draw();
  s.every(1500, draw);
  s.rawKey = k => { if (k === 'q') s.end(); };
  s.onCtrlC = () => s.end();
  return s;
}

function topUptime(ms) {
  const m = Math.floor(ms / 60000), d = Math.floor(m / 1440);
  return `${d ? `${d} days, ` : ''}${Math.floor(m % 1440 / 60)}:${pad2(m % 60)}`;
}

function sessSleep(n, cmdline) {
  const s = new Session({ kind: 'proc', match: 'sleep', title: cmdline, icon: 'terminal', cmdline, raw: true });
  s.later(n * 1000, () => s.end());
  s.onCtrlC = () => { s.write('^C', 'dim'); s.end(); };
  return s;
}

// screen と tmux のセッションはサーバーで動き続けるので、サーバーごとに1つだけ持つ
function getPersistent(srv, kind, name) {
  const key = `${kind}:${name}`;
  if (srv.persist.has(key)) return srv.persist.get(key);
  const list = kind === 'screen' ? srv.screens : srv.tmux;
  const idx = list.findIndex(d => d.name === name);
  if (idx < 0) return null;
  let s;
  if (kind === 'screen' && srv.id === 'mc' && name === 'mc') s = sessMcConsole(srv);
  else if (kind === 'screen' && srv.id === 'mc' && name === 'proxy') s = sessVelocity(srv);
  else s = sessMiniShell(srv, kind, name);
  Object.assign(s, {
    key, name, kind, match: kind, detachable: true, altScreen: true, icon: 'layers',
    title: `${kind}: ${name}`, pid: 1201 + idx * 37, prefixKey: kind === 'screen' ? 'a' : 'b',
    cmdline: kind === 'screen' ? `screen -r ${name}` : `tmux attach -t ${name}`,
  });
  srv.persist.set(key, s);
  return s;
}

function sessMcConsole(srv) {
  const path = srv.home + '/mc/logs/latest.log';
  const s = new Session({ promptText: '> ' });
  (findNode(srv, path)?.content || '').split('\n').filter(Boolean).slice(-40).forEach(l => s.write(colorLog(l)));
  s.unsubs.push(subscribeLog(srv, path, l => s.write(colorLog(l))));
  s.onInput = line => {
    const t = line.trim().replace(/^\//, '');
    if (!t) return;
    const [c, ...a] = t.split(/\s+/);
    const log = msg => emitLog(srv, path, `[${clock()} INFO]: ${msg}`);
    if (c === 'list') log('There are 2 of a max of 20 players online: asuta, kuro');
    else if (c === 'say') log(`[Server] ${a.join(' ')}`);
    else if (c === 'save-all') { log('Saving the game (this may take a moment!)'); setTimeout(() => log('Saved the game'), 700); }
    else if (c === 'tps') log('TPS from last 1m, 5m, 15m: 20.0, 20.0, 19.98');
    else if (c === 'stop') s.write('このモックでは stop は動きません', 'dim');
    else log('Unknown or incomplete command, see below for error');
  };
  s.onCtrlC = () => s.write('^C', 'dim');
  return s;
}

function sessVelocity(srv) {
  const path = srv.home + '/proxy/logs/latest.log';
  const s = new Session({ promptText: '> ' });
  (findNode(srv, path)?.content || '').split('\n').filter(Boolean).forEach(l => s.write(colorLog(l)));
  s.unsubs.push(subscribeLog(srv, path, l => s.write(colorLog(l))));
  s.onInput = line => {
    const c = line.trim();
    if (!c) return;
    const log = m => emitLog(srv, path, `[${clock()} INFO]: ${m}`);
    if (c === 'glist') log('There are 2 players online.');
    else if (c === 'velocity version') log('Velocity 3.4.0');
    else if (c === 'end' || c === 'shutdown') s.write('このモックでは止まりません', 'dim');
    else log('Unknown command.');
  };
  s.onCtrlC = () => s.write('^C', 'dim');
  return s;
}

function sessMiniShell(srv, kind, name) {
  const s = new Session({});
  s.cwd = srv.home;
  s.promptText = () => `${srv.user}@${srv.name}:${tilde(srv, s.cwd)}$ `;
  if (kind === 'tmux') s.footer = `[${name}] 0:bash*  1:htop-`;
  s.complete = text => completePath(srv, s, text);
  s.onInput = line => {
    const [c, ...a] = tokenize(line.trim());
    if (!c) return;
    if (c === 'exit') {
      s.end('[exited]');
      srv.persist.delete(`${kind}:${name}`);
      const list = kind === 'screen' ? srv.screens : srv.tmux;
      list.splice(list.findIndex(d => d.name === name), 1);
      renderSide();
      return;
    }
    if (!basicCmd(srv, s, c, a)) s.write(`${c}: command not found`, 'err');
  };
  return s;
}

/* ---------- ウィンドウ ---------- */

let zTop = 10, winSeq = 0;

function hostRect() {
  const r = $('#windows').getBoundingClientRect();
  return { left: r.left, top: r.top, right: r.right, bottom: r.bottom, w: r.width || 900, h: r.height || 560 };
}
function setRect(win, r) {
  win.rect = { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.w), h: Math.round(r.h) };
  Object.assign(win.el.style, { left: win.rect.x + 'px', top: win.rect.y + 'px', width: win.rect.w + 'px', height: win.rect.h + 'px' });
}
function animRect(win, r) {
  win.el.classList.add('animating');
  setRect(win, r);
  setTimeout(() => win.el.classList.remove('animating'), 230);
}
const pointIn = (ev, elm) => { const r = elm.getBoundingClientRect(); return r.width > 0 && ev.clientX >= r.left && ev.clientX <= r.right && ev.clientY >= r.top && ev.clientY <= r.bottom; };

function openWindow(srv, o) {
  const win = { id: 'w' + (++winSeq), srv, type: o.type, title: o.title, icon: o.icon, node: o.node || null, session: o.session || null, cleanup: [], onClose: o.onClose, min: false, max: false, snapped: false, titleEl: null, btnMax: null, body: null, el: null };
  win.titleEl = el('span', { class: 'win-title' }, o.title);
  const movable = win.session && win.session.kind !== 'shell';
  win.btnMax = el('button', { class: 'win-btn pc-only', title: '最大化', onclick: e => { e.stopPropagation(); toggleMax(win); } }, ic('max'));
  const bar = el('div', { class: 'win-bar' },
    ic(o.icon, 'win-icon'), win.titleEl,
    el('div', { class: 'win-btns' },
      movable && el('button', { class: 'win-btn m-only', title: 'ターミナルに戻す', onclick: e => { e.stopPropagation(); if (srv.panel.takeBack(win)) setMTab('terminal'); } }, ic('download')),
      el('button', { class: 'win-btn pc-only', title: '最小化', onclick: e => { e.stopPropagation(); minimize(win); } }, ic('min')),
      win.btnMax,
      el('button', { class: 'win-btn close', title: '閉じる', onclick: e => { e.stopPropagation(); closeWindow(win); } }, ic('x'))));
  win.body = el('div', { class: 'win-body' });
  win.el = el('div', { class: 'win', dataset: { type: o.type } }, bar, win.body, el('div', { class: 'rz rz-e' }), el('div', { class: 'rz rz-s' }), el('div', { class: 'rz rz-se' }));
  srv.layer.append(win.el);
  const H = hostRect();
  const w = Math.min(o.w || 480, H.w - 16), h = Math.min(o.h || 300, H.h - 16);
  const n = srv.cascade++ % 6;
  const x = o.x ?? 40 + n * 28, y = o.y ?? 24 + n * 24;
  setRect(win, { x: clamp(x, 0, Math.max(0, H.w - w)), y: clamp(y, 0, Math.max(0, H.h - h)), w, h });
  srv.windows.push(win);
  o.build?.(win.body, win);
  wireWindow(win, bar);
  focusWin(win);
  if (!isMobile()) win.el.animate([{ opacity: 0, transform: 'scale(.97) translateY(8px)' }, { opacity: 1, transform: 'none' }], { duration: 170, easing: 'cubic-bezier(.2,.8,.2,1)' });
  return win;
}

function wireWindow(win, bar) {
  const elw = win.el;
  elw.addEventListener('pointerdown', () => { if (win.srv.focused !== win) focusWin(win); }, true);
  bar.addEventListener('dblclick', e => { if (!e.target.closest('.win-btn') && !isMobile()) toggleMax(win); });
  bar.addEventListener('pointerdown', e => {
    if (e.button !== 0 || e.target.closest('.win-btn') || isMobile()) return;
    e.preventDefault();
    const H = hostRect();
    const tp = $('#terminal-panel');
    const movable = win.session && win.session.kind !== 'shell';
    let start = { px: e.clientX, py: e.clientY, ...win.rect }, moved = false, mode = null;
    bar.setPointerCapture(e.pointerId);
    const move = ev => {
      if (!moved) {
        if (Math.hypot(ev.clientX - start.px, ev.clientY - start.py) < 4) return;
        moved = true;
        elw.classList.add('dragging');
        if (win.max || win.snapped) {
          // 最大化や半分のまま掴んだら、元の大きさに戻してから動かす
          const prev = win.prev || win.rect;
          const ratio = clamp((ev.clientX - H.left - win.rect.x) / win.rect.w, 0, 1);
          win.max = false; win.snapped = false;
          elw.classList.remove('max');
          updMaxBtn(win);
          start = { px: ev.clientX, py: ev.clientY, x: ev.clientX - H.left - prev.w * ratio, y: Math.max(0, ev.clientY - H.top - 16), w: prev.w, h: prev.h };
          setRect(win, start);
        }
      }
      const x = clamp(start.x + ev.clientX - start.px, -win.rect.w + 90, H.w - 90);
      const y = clamp(start.y + ev.clientY - start.py, 0, H.h - 34);
      setRect(win, { ...win.rect, x, y });
      const overTerm = movable && pointIn(ev, tp);
      if (overTerm) hoverDrop(tp, 'ここで離すとターミナルに戻す');
      mode = overTerm ? 'return' : !state.settings.snap ? null
        : ev.clientX <= H.left + 8 ? 'left' : ev.clientX >= H.right - 8 ? 'right' : ev.clientY <= H.top + 6 ? 'max' : null;
      snapPreview(mode, H);
    };
    const up = () => {
      bar.removeEventListener('pointermove', move);
      bar.removeEventListener('pointerup', up);
      bar.removeEventListener('pointercancel', up);
      elw.classList.remove('dragging');
      snapPreview(null);
      tp.classList.remove('drop-on');
      if (!moved) return;
      const before = { x: start.x, y: start.y, w: start.w, h: start.h };
      if (mode === 'return') win.srv.panel.takeBack(win);
      else if (mode === 'max') maximize(win, before);
      else if (mode === 'left' || mode === 'right') {
        win.prev = before; win.snapped = true;
        animRect(win, { x: mode === 'left' ? 0 : H.w / 2, y: 0, w: H.w / 2, h: H.h });
      }
    };
    bar.addEventListener('pointermove', move);
    bar.addEventListener('pointerup', up);
    bar.addEventListener('pointercancel', up);
  });
  for (const hd of $$('.rz', elw)) {
    hd.addEventListener('pointerdown', e => {
      if (e.button !== 0 || win.max) return;
      e.preventDefault(); e.stopPropagation();
      focusWin(win);
      hd.setPointerCapture(e.pointerId);
      const s = { px: e.clientX, py: e.clientY, ...win.rect };
      const ex = !hd.classList.contains('rz-s'), ey = !hd.classList.contains('rz-e');
      const move = ev => setRect(win, { ...win.rect, w: ex ? Math.max(260, s.w + ev.clientX - s.px) : s.w, h: ey ? Math.max(150, s.h + ev.clientY - s.py) : s.h });
      const up = () => { hd.removeEventListener('pointermove', move); hd.removeEventListener('pointerup', up); win.snapped = false; };
      hd.addEventListener('pointermove', move);
      hd.addEventListener('pointerup', up);
    });
  }
}

function snapPreview(mode, H) {
  const p = $('#snap-preview');
  if (!mode || mode === 'return') { p.hidden = true; return; }
  const r = mode === 'left' ? [0, 0, H.w / 2, H.h] : mode === 'right' ? [H.w / 2, 0, H.w / 2, H.h] : [0, 0, H.w, H.h];
  Object.assign(p.style, { left: r[0] + 6 + 'px', top: r[1] + 6 + 'px', width: r[2] - 12 + 'px', height: r[3] - 12 + 'px' });
  p.hidden = false;
}

function updMaxBtn(win) {
  win.btnMax.replaceChildren(ic(win.max ? 'restore' : 'max'));
  win.btnMax.title = win.max ? '元に戻す' : '最大化';
}
function maximize(win, prev) {
  win.prev = prev || { ...win.rect };
  win.max = true; win.snapped = false;
  win.el.classList.add('animating', 'max');
  setTimeout(() => win.el.classList.remove('animating'), 230);
  updMaxBtn(win);
}
function restoreMax(win) {
  win.max = false;
  win.el.classList.add('animating');
  win.el.classList.remove('max');
  setRect(win, win.prev || win.rect);
  setTimeout(() => win.el.classList.remove('animating'), 230);
  updMaxBtn(win);
}
const toggleMax = win => win.max ? restoreMax(win) : maximize(win);

function focusWin(win) {
  const srv = win.srv;
  win.el.style.zIndex = ++zTop;
  for (const w of srv.windows) {
    w.el.classList.toggle('focused', w === win);
    w.el.classList.toggle('m-active', w === win);
  }
  srv.focused = win;
  renderTaskbar();
}

function restoreFocus(win) {
  if (win.min) unminimize(win); else focusWin(win);
  if (isMobile()) setMTab('desktop');
}

function topWindow(srv) {
  return srv.windows.filter(w => !w.min).sort((a, b) => (+b.el.style.zIndex) - (+a.el.style.zIndex))[0] || null;
}

function minimize(win) {
  const srv = win.srv;
  win.min = true;
  const item = $(`.task-item[data-win="${win.id}"]`);
  const r = win.el.getBoundingClientRect(), t = item?.getBoundingClientRect();
  if (t && r.width) {
    const dx = t.left + t.width / 2 - (r.left + r.width / 2), dy = t.top + t.height / 2 - (r.top + r.height / 2);
    const anim = win.el.animate([{ transform: 'none', opacity: 1 }, { transform: `translate(${dx}px, ${dy}px) scale(.12)`, opacity: 0 }], { duration: 200, easing: 'cubic-bezier(.4,0,.8,.4)', fill: 'forwards' });
    anim.onfinish = () => { win.el.classList.add('min'); anim.cancel(); };
  } else win.el.classList.add('min');
  if (srv.focused === win) {
    srv.focused = null;
    const top = topWindow(srv);
    if (top) focusWin(top); else renderTaskbar();
  } else renderTaskbar();
}

function unminimize(win) {
  win.min = false;
  win.el.classList.remove('min');
  focusWin(win);
  const item = $(`.task-item[data-win="${win.id}"]`);
  const r = win.el.getBoundingClientRect(), t = item?.getBoundingClientRect();
  if (t && r.width) {
    const dx = t.left + t.width / 2 - (r.left + r.width / 2), dy = t.top + t.height / 2 - (r.top + r.height / 2);
    win.el.animate([{ transform: `translate(${dx}px, ${dy}px) scale(.12)`, opacity: 0 }, { transform: 'none', opacity: 1 }], { duration: 200, easing: 'cubic-bezier(.2,.6,.2,1)' });
  }
}

function closeWindow(win, o = {}) {
  if (win.closing) return;
  win.closing = true;
  const srv = win.srv;
  win.cleanup.forEach(f => f());
  win.onClose?.(win, o);
  const done = () => win.el.remove();
  if (isMobile()) done();
  else win.el.animate([{ opacity: 1, transform: 'none' }, { opacity: 0, transform: 'scale(.96)' }], { duration: 120, easing: 'ease-in', fill: 'forwards' }).onfinish = done;
  srv.windows = srv.windows.filter(w => w !== win);
  if (srv.focused === win) {
    srv.focused = null;
    const top = topWindow(srv);
    if (top) { focusWin(top); return; }
  }
  renderTaskbar();
}

function tileWindows() {
  const srv = cur();
  const ws = srv.windows.filter(w => !w.min);
  if (!ws.length) return;
  const H = hostRect(), g = 8;
  const cols = Math.ceil(Math.sqrt(ws.length)), rows = Math.ceil(ws.length / cols);
  const cw = (H.w - g * (cols + 1)) / cols, ch = (H.h - g * (rows + 1)) / rows;
  ws.forEach((w, i) => {
    w.max = false; w.snapped = false;
    w.el.classList.remove('max');
    updMaxBtn(w);
    animRect(w, { x: g + (i % cols) * (cw + g), y: g + Math.floor(i / cols) * (ch + g), w: cw, h: ch });
  });
}

/* ---------- ウィンドウの中身 ---------- */

function openSessionWindow(srv, sess, pos = {}) {
  if (sess.window) { restoreFocus(sess.window); return sess.window; }
  return openWindow(srv, {
    type: 'session', title: sess.title, icon: sess.icon, session: sess, w: 540, h: 320, ...pos,
    build(body, win) {
      const v = new TermView(body);
      v.setBase(sess);
      win.view = v;
      sess.window = win;
      const off = sess.on(t => {
        if (t === 'end') { win.el.classList.add('ended'); win.titleEl.textContent = `${sess.title} · 終了`; renderTaskbar(); }
        if (t === 'detach') closeWindow(win);
      });
      win.cleanup.push(() => { off(); v.dispose(); if (sess.window === win) sess.window = null; });
      setTimeout(() => { if (!isMobile()) v.focus(); }, 0);
    },
    onClose(_, o) {
      if (o.keepSession) return;
      if (sess.detachable) toast(`${sess.title} からデタッチしました。`);
      else if (!sess.ended) sess.end();
    },
  });
}

function openTerminalWindow(srv, pos = {}) {
  srv.termCount++;
  return openWindow(srv, {
    type: 'terminal', title: `ターミナル ${srv.termCount}`, icon: 'terminal', w: 520, h: 300, ...pos,
    build(body, win) {
      const v = new TermView(body);
      const host = new ShellHost(srv, v, { win, cwd: srv.panel.shell.cwd });
      win.view = v; win.host = host;
      win.cleanup.push(() => {
        if (host.fg && !host.fg.detachable) host.fg.end();
        v.dispose();
      });
      setTimeout(() => { if (!isMobile()) v.focus(); }, 0);
    },
  });
}

function openFile(srv, node, pos = {}) {
  const ex = srv.windows.find(w => w.node === node);
  if (ex) { restoreFocus(ex); return ex; }
  const k = fileKind(node);
  if (k === 'dir') { toast(`${node.name} はフォルダです`); return null; }
  if (k === 'arc') return openArchive(srv, node, pos);
  if (k === 'bin') { toast(`${node.name} はバイナリのファイルです。… のメニューからダウンロードできます`, 'warn'); return null; }
  if (k === 'log') return openLogWindow(srv, node, pos);
  if (k === 'img') return openImageWindow(srv, node, pos);
  return openEditor(srv, node, pos);
}

function openLogWindow(srv, node, pos = {}) {
  return openWindow(srv, {
    type: 'log', title: node.title || node.name, icon: 'log', node, w: 520, h: 320, ...pos,
    build(body, win) {
      const filter = el('input', { class: 'mini-input', placeholder: '絞り込み', spellcheck: 'false' });
      const followBox = el('input', { type: 'checkbox' });
      followBox.checked = state.settings.followLog;
      const out = el('div', { class: 'log-out' });
      body.append(el('div', { class: 'win-toolbar' }, el('span', { class: 'path' }, node.label || tilde(srv, node.path)), el('div', { class: 'grow' }), filter, el('label', { class: 'toggle' }, followBox, '追従')), out);
      const match = d => !filter.value || d.textContent.toLowerCase().includes(filter.value.toLowerCase());
      const add = (line, fresh) => {
        const p = logParts(line);
        const d = el('div', { class: 'log-line ' + p.c + (fresh ? ' new' : '') }, el('span', { class: 'lh' }, p.head), el('span', { class: 'lb' }, p.body));
        d.hidden = !match(d);
        out.append(d);
        while (out.childElementCount > 1000) out.firstChild.remove();
        if (followBox.checked) out.scrollTop = out.scrollHeight;
      };
      (node.content || '').split('\n').filter(Boolean).slice(-300).forEach(l => add(l, false));
      win.cleanup.push(subscribeLog(srv, node.path, l => add(l, true)));
      filter.addEventListener('input', () => { for (const d of out.children) d.hidden = !match(d); });
      const userScroll = () => requestAnimationFrame(() => { followBox.checked = out.scrollHeight - out.scrollTop - out.clientHeight < 8; });
      out.addEventListener('wheel', userScroll, { passive: true });
      out.addEventListener('touchmove', userScroll, { passive: true });
      out.addEventListener('pointerup', userScroll);
      followBox.addEventListener('change', () => { if (followBox.checked) out.scrollTop = out.scrollHeight; });
      const ro = new ResizeObserver(() => { if (followBox.checked) out.scrollTop = out.scrollHeight; });
      ro.observe(out);
      win.cleanup.push(() => ro.disconnect());
    },
  });
}

function openEditor(srv, node, pos = {}) {
  return openWindow(srv, {
    type: 'editor', title: node.name, icon: 'code', node, w: 540, h: 340, ...pos,
    build(body, win) {
      const gutter = el('div', { class: 'ed-gutter' });
      const ta = el('textarea', { class: 'ed-text', spellcheck: 'false', wrap: 'off' });
      ta.value = node.content ?? '';
      const status = el('span', { class: 'status-text' });
      const save = () => {
        node.content = ta.value;
        node.size = new Blob([ta.value]).size;
        setDirty(false);
        status.textContent = '保存しました';
        status.style.color = 'var(--ok)';
        setTimeout(() => { status.style.color = ''; upd(); }, 1800);
      };
      const setDirty = d => { win.titleEl.textContent = (d ? '● ' : '') + node.name; renderTaskbar(); };
      const upd = () => {
        const n = ta.value.split('\n').length;
        gutter.textContent = Array.from({ length: n }, (_, i) => i + 1).join('\n');
        status.textContent = `${n} 行`;
      };
      // root のファイルは sudo モードの間だけ書ける
      const locked = () => node.owner === 'root' && !srv.sudo;
      const trySave = () => locked() ? enableSudo(srv, `${node.name} を保存するには sudo モードを ON にしてください`, save) : save();
      const saveBtn = el('button', { class: 'btn small', onclick: trySave }, '保存');
      const lockBtn = el('button', { class: 'btn small lock-btn', title: 'sudo モードを ON にすると編集できます', onclick: () => enableSudo(srv, null) }, ic('lock'), '読み取り専用');
      const applyLock = () => { ta.readOnly = locked(); lockBtn.hidden = !locked(); saveBtn.hidden = locked(); };
      srv.sudoSubs.add(applyLock);
      win.cleanup.push(() => srv.sudoSubs.delete(applyLock));
      body.append(
        el('div', { class: 'win-toolbar' }, el('span', { class: 'path' }, tilde(srv, node.path)), el('div', { class: 'grow' }), status, lockBtn, saveBtn),
        el('div', { class: 'ed' }, gutter, ta));
      applyLock();
      ta.addEventListener('input', () => { upd(); setDirty(true); });
      ta.addEventListener('scroll', () => { gutter.scrollTop = ta.scrollTop; });
      ta.addEventListener('keydown', e => {
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') { e.preventDefault(); trySave(); }
        if (e.key === 'Tab') {
          e.preventDefault();
          const a = ta.selectionStart;
          ta.setRangeText('  ', a, ta.selectionEnd, 'end');
          upd(); setDirty(true);
        }
      });
      upd();
    },
  });
}

function openImageWindow(srv, node, pos = {}) {
  return openWindow(srv, {
    type: 'image', title: node.name, icon: 'image', node, w: 340, h: 320, ...pos,
    build(body) {
      const c = el('canvas', { class: 'pixel', width: 64, height: 64 });
      drawPixelIcon(c, node.path);
      body.append(el('div', { class: 'img-wrap' }, c), el('div', { class: 'img-info' }, `64 × 64 · ${fmtSize(node.size || 0)}`));
    },
  });
}

/* ---------- 圧縮ファイル ---------- */

const SQL_DUMP = `-- MariaDB dump 10.19  Distrib 10.11.8-MariaDB
--
-- Host: localhost    Database: app
-- ------------------------------------------------------

CREATE TABLE \`items\` (
  \`id\` int(11) NOT NULL AUTO_INCREMENT,
  \`name\` varchar(255) NOT NULL,
  \`price\` int(11) NOT NULL,
  \`created_at\` datetime NOT NULL DEFAULT current_timestamp(),
  PRIMARY KEY (\`id\`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO \`items\` VALUES (1,'ダイヤモンド',1200,'2026-10-01 12:00:00'),(2,'エメラルド',300,'2026-10-02 09:30:00'),(3,'ネザライト',5800,'2026-10-20 21:14:00');

CREATE TABLE \`users\` (
  \`id\` int(11) NOT NULL AUTO_INCREMENT,
  \`name\` varchar(64) NOT NULL,
  PRIMARY KEY (\`id\`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO \`users\` VALUES (1,'asuta'),(2,'kuro');

-- Dump completed on 2026-10-24  4:00:37`;

// 過去のログは、ファイルごとに同じ中身が出るように名前から乱数を作る
function pastLog(seed) {
  const r = mulberry32(hashStr(seed));
  const lines = [];
  let t = 4 * 3600;
  const at = () => `${pad2(Math.floor(t / 3600) % 24)}:${pad2(Math.floor(t / 60) % 60)}:${pad2(t % 60)}`;
  for (const b of ['Starting minecraft server version 1.21.4', 'Loading properties', 'Preparing level "world"', 'Done (11.902s)! For help, type "help"']) {
    lines.push(`[${at()} INFO]: ${b}`);
    t += 1;
  }
  for (let i = 0; i < 90; i++) {
    t += Math.floor(60 + r() * 600);
    const [lv, msg] = MC_MSGS[Math.floor(r() * MC_MSGS.length)];
    lines.push(`[${at()} ${lv}]: ${msg}`);
  }
  lines.push(`[${at()} INFO]: Stopping server`, `[${at()} INFO]: Saving worlds`);
  return lines.join('\n');
}

// 圧縮ファイルの中身。このモックでは名前から作る
function archiveData(node) {
  const n = node.name;
  if (!/\.(tar\.gz|tgz|tar|zip|jar|7z)$/i.test(n)) {
    const name = n.replace(/\.gz$/i, '');
    const text = /\.log$/i.test(name) ? pastLog(node.path) : /\.sql$/i.test(name) ? SQL_DUMP : '(このモックでは中身を用意していません)';
    return { kind: 'text', name, text };
  }
  if (/^mc-.*\.tar\.gz$/.test(n)) {
    return {
      kind: 'list', entries: [
        ['mc/server.properties', 1180, TEXT.props], ['mc/ops.json', 120, TEXT.ops], ['mc/start.sh', 96, TEXT.start],
        ['mc/paper.jar', 52428800], ['mc/plugins/LuckPerms.jar', 2203118], ['mc/plugins/EssentialsX.jar', 1452210],
        ['mc/world/level.dat', 18210], ['mc/world/region/r.0.0.mca', 6291456], ['mc/world/region/r.-1.0.mca', 5242880],
        ['mc/logs/latest.log', 6210, pastLog(node.path)],
      ],
    };
  }
  if (/\.jar$/i.test(n)) {
    const base = n.replace(/\.jar$/i, ''), pkg = base.toLowerCase();
    return {
      kind: 'list', entries: [
        ['META-INF/MANIFEST.MF', 74, 'Manifest-Version: 1.0\nCreated-By: Gradle\n'],
        ['plugin.yml', 420, `name: ${base}\nversion: 1.0.0\nmain: example.${pkg}.Plugin\napi-version: '1.21'\n`],
        ['config.yml', 1830, `# ${base} の設定\nenabled: true\n`],
        [`example/${pkg}/Plugin.class`, 18240], [`example/${pkg}/Commands.class`, 9310],
      ],
    };
  }
  return { kind: 'list', entries: [['README.md', 512, `# ${n}\n\nこのモックでは中身を用意していません。`]] };
}

function textView(name, text) {
  if (/\.log$/i.test(name)) {
    return el('div', { class: 'log-out' }, text.split('\n').map(l => {
      const p = logParts(l);
      return el('div', { class: 'log-line ' + p.c }, el('span', { class: 'lh' }, p.head), el('span', { class: 'lb' }, p.body));
    }));
  }
  const gutter = el('div', { class: 'ed-gutter' }, text.split('\n').map((_, i) => i + 1).join('\n'));
  const pre = el('pre', { class: 'ed-text' }, text);
  pre.addEventListener('scroll', () => { gutter.scrollTop = pre.scrollTop; });
  return el('div', { class: 'ed' }, gutter, pre);
}

// 解凍した中身はサーバーに書き出さず、ウィンドウの中だけで見せる
function openArchive(srv, node, pos = {}) {
  const ex = srv.windows.find(w => w.node === node);
  if (ex) { restoreFocus(ex); return ex; }
  const data = archiveData(node);
  return openWindow(srv, {
    type: 'archive', title: data.kind === 'text' ? data.name : node.name, icon: 'archive', node, w: 560, h: 360, ...pos,
    build(body) {
      const bar = el('div', { class: 'win-toolbar' });
      const main = el('div', { class: 'ar-main' });
      body.append(bar, main);
      if (data.kind === 'text') {
        bar.append(el('span', { class: 'path' }, tilde(srv, node.path)), el('div', { class: 'grow' }), el('span', { class: 'status-text' }, '解凍して表示 · 読み取り専用'));
        main.append(textView(data.name, data.text));
        return;
      }
      // パスの一覧をフォルダの木にする
      const root = { dir: true, children: new Map() };
      for (const [p, size, text] of data.entries) {
        const parts = p.split('/');
        let n = root;
        parts.forEach((seg, i) => {
          if (i === parts.length - 1) { n.children.set(seg, { name: seg, path: p, size, text }); return; }
          if (!n.children.has(seg)) n.children.set(seg, { name: seg, dir: true, open: i === 0, children: new Map() });
          n = n.children.get(seg);
        });
      }
      const total = data.entries.reduce((a, e) => a + e[1], 0);
      const showList = () => {
        bar.replaceChildren(el('span', { class: 'path' }, tilde(srv, node.path)), el('div', { class: 'grow' }), el('span', { class: 'status-text' }, `${data.entries.length} 件 · 展開すると ${fmtSize(total)}`));
        const list = el('div', { class: 'ar-list' });
        const walk = (n, d) => {
          const kids = [...n.children.values()].sort((a, b) => (!!b.dir - !!a.dir) || a.name.localeCompare(b.name));
          for (const c of kids) {
            if (c.dir) {
              list.append(el('div', { class: 'ar-row', style: `--d:${d}`, onclick: () => { c.open = !c.open; showList(); } },
                ic('chevron', 'chev' + (c.open ? ' open' : '')), ic('folder', 'f-dir'), el('span', { class: 'name' }, c.name)));
              if (c.open) walk(c, d + 1);
              continue;
            }
            const [icName, icCls] = KIND_ICON[fileKind(c)];
            const canRead = c.text != null;
            list.append(el('div', {
              class: 'ar-row' + (canRead ? '' : ' bin'), style: `--d:${d}`, title: canRead ? '開く' : 'バイナリのファイル',
              onclick: canRead ? () => showText(c) : null,
            }, el('span', { class: 'chev-space' }), ic(icName, icCls), el('span', { class: 'name' }, c.name), el('span', { class: 'num' }, fmtSize(c.size))));
          }
        };
        walk(root, 0);
        main.replaceChildren(list);
      };
      const showText = c => {
        bar.replaceChildren(el('button', { class: 'btn small ghost', onclick: showList }, '‹ 一覧'), el('span', { class: 'path' }, c.path), el('div', { class: 'grow' }), el('span', { class: 'status-text' }, '読み取り専用'));
        main.replaceChildren(textView(c.name, c.text));
      };
      showList();
    },
  });
}

function drawPixelIcon(canvas, seed) {
  const ctx = canvas.getContext('2d');
  const r = mulberry32(hashStr(seed));
  for (let y = 0; y < 16; y++) {
    for (let x = 0; x < 16; x++) {
      const grass = y < 4 || (y === 4 && r() < .45);
      if (grass) { const g = 120 + r() * 60; ctx.fillStyle = `rgb(${55 + r() * 30},${g},${45 + r() * 20})`; }
      else { const b = 95 + r() * 40; ctx.fillStyle = `rgb(${b + 35},${b - 5},${b - 45})`; }
      ctx.fillRect(x * 4, y * 4, 4, 4);
    }
  }
}

const MEM_TOTAL = 7838;
const MON_TABS = [['cpu', 'CPU'], ['mem', 'メモリ'], ['disk', 'ディスク'], ['net', 'ネットワーク']];
const fmtMB = mb => mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${mb.toFixed(0)} MB`;
const meter = (pct, cls = '') => el('div', { class: 'meter ' + cls }, el('i', { style: `width:${clamp(pct, 0, 100)}%` }));
const setMeter = (m, pct) => { m.firstChild.style.width = clamp(pct, 0, 100) + '%'; };
const section = (title, ...kids) => el('section', { class: 'mon-sec' }, title && el('h4', {}, title), ...kids);
const dirSize = n => n.dir ? n.children.reduce((a, c) => a + dirSize(c), 0) : (n.size || 0);

// 行を作り直さずに並べ替える。毎秒の更新でクリックが外れないようにするため
function keyed(box, items, key, create, update) {
  const map = box._rows || (box._rows = new Map());
  const seen = new Set();
  for (const it of items) {
    const k = key(it);
    seen.add(k);
    let row = map.get(k);
    if (!row) { row = create(it); map.set(k, row); }
    update(row, it);
    box.append(row);
  }
  for (const [k, row] of map) if (!seen.has(k)) { row.remove(); map.delete(k); }
}

function monCards(srv) {
  const m = srv.metrics;
  return [
    { key: 'cpu', label: 'CPU', hist: m.cpu, max: 100, val: `${last(m.cpu).toFixed(0)}%`, sub: '4 コア' },
    { key: 'mem', label: 'メモリ', hist: m.mem, max: 100, val: `${last(m.mem).toFixed(0)}%`, sub: `${(last(m.mem) / 100 * 7.6).toFixed(1)} / 7.6 GB` },
    { key: 'disk', label: 'ディスク', hist: m.disk, max: 100, val: `${last(m.disk).toFixed(0)}%`, sub: `${(last(m.disk) / 100 * srv.mounts[0][1]).toFixed(0)} / ${srv.mounts[0][1]} GB` },
    { key: 'net', label: 'ネットワーク', hist: m.net, max: Math.max(200, ...m.net) * 1.15, val: `${last(m.net).toFixed(0)} KB/s`, sub: `送信 ${last(m.tx).toFixed(0)} KB/s` },
  ];
}

function openMonitor(srv, pos = {}) {
  const ex = srv.windows.find(w => w.type === 'monitor');
  if (ex) { restoreFocus(ex); if (pos.view) ex.showView(pos.view); return ex; }
  return openWindow(srv, {
    type: 'monitor', title: 'リソース監視', icon: 'chart', w: 600, h: 210, ...pos,
    build(body, win) {
      const box = el('div', { class: 'mon-body' });
      body.append(box);
      let upd = () => {};
      win.showView = v => {
        const tab = MON_TABS.find(t => t[0] === v);
        win.titleEl.textContent = tab ? `リソース監視 · ${tab[1]}` : 'リソース監視';
        upd = tab ? monDetail(srv, box, v, win.showView) : monOverview(srv, box, win.showView);
        upd();
        if (tab) growWindow(win, 460, 600);
        renderTaskbar();
      };
      win.showView(pos.view || 'overview');
      const tick = () => upd();
      srv.tickers.add(tick);
      win.cleanup.push(() => srv.tickers.delete(tick));
    },
  });
}

function growWindow(win, minH, minW) {
  if (win.max || isMobile() || (win.rect.h >= minH && win.rect.w >= minW)) return;
  const H = hostRect();
  const w = Math.min(Math.max(win.rect.w, minW), H.w - 8), h = Math.min(Math.max(win.rect.h, minH), H.h - 8);
  animRect(win, { x: clamp(win.rect.x, 0, H.w - w), y: clamp(win.rect.y, 0, H.h - h), w, h });
}

function monOverview(srv, box, show) {
  const grid = el('div', { class: 'mon' });
  const cards = monCards(srv).map(c => {
    const val = el('div', { class: 'mon-value' }), sub = el('div', { class: 'mon-sub' }), sp = spark();
    grid.append(el('button', { class: 'mon-card', title: `${c.label} の詳細`, onclick: () => show(c.key) },
      el('div', { class: 'mon-label' }, c.label, el('span', { class: 'mon-more' }, '詳細', ic('chevron'))), val, sub, sp.el));
    return { val, sub, sp };
  });
  box.replaceChildren(grid);
  return () => monCards(srv).forEach((c, i) => {
    cards[i].val.textContent = c.val;
    cards[i].sub.textContent = c.sub;
    cards[i].sp.set(c.hist, c.max);
  });
}

function monDetail(srv, box, key, show) {
  const bigVal = el('div', { class: 'mon-value' }), bigSub = el('div', { class: 'mon-sub' }), sp = spark();
  sp.el.classList.add('big');
  const main = el('div');
  box.replaceChildren(
    el('div', { class: 'mon-head' },
      el('button', { class: 'btn small ghost', onclick: () => show('overview') }, '‹ 概要'),
      el('div', { class: 'seg' }, MON_TABS.map(([k, l]) => el('button', { class: k === key ? 'active' : '', onclick: () => show(k) }, l)))),
    el('div', { class: 'mon-scroll' }, el('div', { class: 'mon-top' }, el('div', {}, bigVal, bigSub), sp.el), main));
  const part = MON_DETAIL[key](srv, main);
  return () => {
    const c = monCards(srv).find(x => x.key === key);
    bigVal.textContent = c.val;
    bigSub.textContent = c.sub;
    sp.set(c.hist, c.max);
    part();
  };
}

function procTable(srv, sortKey) {
  const body = el('div');
  const wrap = el('div', { class: 'ptable' },
    el('div', { class: 'prow phead' }, el('span', {}, 'プロセス'), el('span', {}, 'PID'), el('span', {}, 'ユーザー'), el('span', {}, 'CPU'), el('span', {}, 'メモリ')),
    body);
  const update = () => {
    const list = liveProcs(srv).sort((a, b) => sortKey === 'cpu' ? b.cpu - a.cpu : b.rss - a.rss).slice(0, 10);
    keyed(body, list, p => p.pid, p => {
      const cpuM = meter(0), memM = meter(0, 'alt'), cpuT = el('span', { class: 'num' }), memT = el('span', { class: 'num' });
      const row = el('div', { class: 'prow' },
        el('div', { class: 'pname' },
          el('div', {}, el('span', { class: 'mono' }, p.name),
            p.container && el('button', { class: 'tag', title: 'Docker のウィンドウで見る', onclick: () => openDocker(srv, { focus: p.container }) }, ic('box'), p.container)),
          el('div', { class: 'pcmd' }, p.cmd)),
        el('span', { class: 'num' }, p.pid), el('span', { class: 'muted pu' }, p.user),
        el('div', { class: 'pcell' }, cpuM, cpuT), el('div', { class: 'pcell' }, memM, memT));
      Object.assign(row, { cpuM, memM, cpuT, memT });
      return row;
    }, (row, p) => {
      setMeter(row.cpuM, p.cpu);
      row.cpuT.textContent = `${p.cpu.toFixed(1)}%`;
      setMeter(row.memM, p.rss / MEM_TOTAL * 100);
      row.memT.textContent = fmtMB(p.rss);
    });
  };
  return { el: wrap, update };
}

const MON_DETAIL = {
  cpu(srv, main) {
    const cores = el('div', { class: 'cores' });
    const coreEls = srv.cores.map((_, i) => {
      const m = meter(0), t = el('span', { class: 'num' });
      cores.append(el('div', { class: 'core' }, el('span', { class: 'muted' }, `コア ${i}`), m, t));
      return { m, t };
    });
    const load = el('div', { class: 'mon-sub' });
    const pt = procTable(srv, 'cpu');
    main.append(section(null, pt.el), section('コアごとの使用率', cores, load));
    return () => {
      srv.cores.forEach((v, i) => { setMeter(coreEls[i].m, v); coreEls[i].t.textContent = `${v.toFixed(0)}%`; });
      const l = last(srv.metrics.cpu) / 25;
      load.textContent = `ロードアベレージ ${l.toFixed(2)} · ${(l * .9).toFixed(2)} · ${(l * .85).toFixed(2)}`;
      pt.update();
    };
  },
  mem(srv, main) {
    const comp = el('div', { class: 'stack' }), legend = el('div', { class: 'legend' });
    const pt = procTable(srv, 'mem');
    main.append(section(null, pt.el), section('内訳', comp, legend, el('div', { class: 'mon-sub' }, 'スワップ 0.2 / 2.0 GB')));
    return () => {
      const used = last(srv.metrics.mem) / 100 * 7.6, cache = 1.6, free = Math.max(0, 7.6 - used - cache);
      const parts = [['使用中', used, 'c1'], ['キャッシュ', cache, 'c2'], ['空き', free, 'c3']];
      comp.replaceChildren(...parts.map(([, v, c]) => el('i', { class: c, style: `width:${v / 7.6 * 100}%` })));
      legend.replaceChildren(...parts.map(([l, v, c]) => el('span', {}, el('b', { class: c }), `${l} ${v.toFixed(1)} GB`)));
      pt.update();
    };
  },
  disk(srv, main) {
    const mounts = el('div'), io = el('div', { class: 'mon-sub' }), dirs = el('div');
    const list = [];
    for (const c of srv.homeNode.children) {
      if (!c.dir) continue;
      list.push({ node: c, size: dirSize(c) });
      for (const g of c.children) if (g.dir) list.push({ node: g, size: dirSize(g) });
    }
    list.sort((a, b) => b.size - a.size);
    const top = list.slice(0, 6), max = top[0]?.size || 1;
    dirs.append(...top.map(x => el('button', { class: 'dir-row', title: 'ツリーで表示', onclick: () => revealInTree(srv, x.node.path) },
      ic('folder', 'f-dir'), el('span', { class: 'mono' }, tilde(srv, x.node.path)), meter(x.size / max * 100, 'alt'), el('span', { class: 'num' }, fmtSize(x.size)))));
    if (!top.length) dirs.append(el('div', { class: 'mon-sub' }, 'フォルダがありません'));
    main.append(section('マウント', mounts, io), section(null, dirs));
    return () => {
      mounts.replaceChildren(...srv.mounts.map(([mp, size, pct], i) => {
        const p = i === 0 ? last(srv.metrics.disk) : pct;
        return el('div', { class: 'mount' }, el('span', { class: 'mono' }, mp), meter(p, i ? 'alt' : ''), el('span', { class: 'num' }, `${Math.round(size * p / 100)} / ${size} GB`));
      }));
      io.textContent = `読み込み ${last(srv.metrics.dr).toFixed(0)} KB/s · 書き込み ${last(srv.metrics.dw).toFixed(0)} KB/s`;
    };
  },
  net(srv, main) {
    const ifs = el('div', { class: 'mon-sub' });
    const conns = el('div', { class: 'ctable' },
      el('div', { class: 'crow chead' }, el('span', {}, '相手'), el('span', {}, 'ポート'), el('span', {}, 'プロセス')),
      srv.net.conns.map(([remote, port, proc, label]) => el('div', { class: 'crow' }, el('span', { class: 'mono' }, remote), el('span', {}, `${port} · ${label}`), el('span', { class: 'mono muted' }, proc))));
    const listen = el('div', { class: 'chips' }, srv.net.listen.map(([port, proc]) => el('span', { class: 'chip sm' }, `${port} ${proc}`)));
    main.append(section('インターフェース', ifs), section('接続中', conns), section('待ち受け中のポート', listen));
    return () => { ifs.textContent = `eth0 · ${srv.lan} · 受信 ${last(srv.metrics.net).toFixed(0)} KB/s · 送信 ${last(srv.metrics.tx).toFixed(0)} KB/s`; };
  },
};

function spark() {
  const NS = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(NS, 'svg');
  svg.setAttribute('viewBox', '0 0 59 30');
  svg.setAttribute('preserveAspectRatio', 'none');
  svg.classList.add('spark');
  const area = document.createElementNS(NS, 'path'); area.setAttribute('class', 'spark-area');
  const line = document.createElementNS(NS, 'polyline'); line.setAttribute('class', 'spark-line');
  svg.append(area, line);
  return {
    el: svg,
    set(h, max) {
      const pts = h.map((v, i) => `${i},${(29 - v / max * 27).toFixed(2)}`);
      line.setAttribute('points', pts.join(' '));
      area.setAttribute('d', `M0,30 L${pts.join(' L')} L${h.length - 1},30 Z`);
    },
  };
}

function openServices(srv, pos = {}) {
  const ex = srv.windows.find(w => w.type === 'services');
  if (ex) { restoreFocus(ex); return ex; }
  return openWindow(srv, {
    type: 'services', title: 'サービス', icon: 'server', w: 600, h: 320, ...pos,
    build(body) {
      const list = el('div', { class: 'svc-list' });
      const foot = el('div', { class: 'win-foot' });
      const verbs = { start: '起動', stop: '停止', restart: '再起動' };
      const act = (svc, verb) => withSudo(srv, 'サービスを操作するには sudo モードを ON にしてください', () => {
        svc.busy = true; draw();
        const j = journalNode(srv, svc);
        const say = text => emitLog(srv, j.path, `${journalStamp()} ${srv.name} systemd[1]: ${text}`, j);
        if (verb !== 'start') say(`Stopping ${svc.name} - ${svc.desc}...`);
        setTimeout(() => {
          if (verb !== 'start') say(`Stopped ${svc.name} - ${svc.desc}.`);
          if (verb !== 'stop') { say(`Starting ${svc.name} - ${svc.desc}...`); say(`Started ${svc.name} - ${svc.desc}.`); }
          svc.busy = false; svc.active = verb !== 'stop'; draw();
          foot.textContent = `${svc.name} を${verbs[verb]}しました`;
          foot.classList.add('show');
          clearTimeout(foot._t);
          foot._t = setTimeout(() => foot.classList.remove('show'), 2600);
        }, 900);
      });
      const btn = (svc, verb, primary) => el('button', {
        class: 'btn small' + (primary ? ' primary' : ''), disabled: svc.busy, onclick: () => act(svc, verb),
      }, verbs[verb]);
      const draw = () => list.replaceChildren(...(srv.services.length ? srv.services.map(svc => el('div', { class: 'svc' },
        el('span', { class: 'svc-dot ' + (svc.busy ? 'busy' : svc.active ? 'on' : 'off') }),
        el('div', { style: 'min-width:0' }, el('div', { class: 'svc-name' }, svc.name), el('div', { class: 'svc-desc' }, `${svc.desc} · ${svc.busy ? '処理中' : svc.active ? '稼働中' : '停止'}`)),
        el('div', { class: 'svc-actions' },
          el('button', { class: 'btn small', onclick: () => openJournal(srv, svc) }, 'ログ'),
          svc.active ? [btn(svc, 'restart'), btn(svc, 'stop')] : [btn(svc, 'start', true)]),
      )) : [el('div', { class: 'empty' }, 'systemd が見つかりません')]));
      draw();
      body.append(list, foot);
    },
  });
}

/* ---------- Docker ---------- */

function dockerLine(c) {
  const d = new Date();
  const img = c.image.split(':')[0];
  if (img === 'nginx') {
    return `${pick(['198.51.100.7', '203.0.113.50', '192.0.2.14'])} - - [${pad2(d.getDate())}/Oct/${d.getFullYear()}:${clock(d)} +0900] "GET ${pick(['/', '/map/', '/favicon.ico', '/api/status', '/wp-login.php'])} HTTP/1.1" ${pick([200, 200, 200, 304, 404])} ${Math.floor(rand(200, 9000))} "-" "Mozilla/5.0"`;
  }
  if (img === 'redis') {
    return `1:M ${pad2(d.getDate())} Oct ${d.getFullYear()} ${clock(d)}.${String(d.getMilliseconds()).padStart(3, '0')} * ${pick(['1 changes in 3600 seconds. Saving...', 'Background saving started by pid 31', 'DB saved on disk', 'Background saving terminated with success'])}`;
  }
  if (img === 'mariadb') {
    return `${d.getFullYear()}-10-${pad2(d.getDate())} ${clock(d)} 31 ${pick(['[Note] InnoDB: Buffer pool(s) load completed', "[Warning] Aborted connection 31 to db: 'app' user: 'app' host: '172.18.0.3' (Got an error reading communication packets)"])}`;
  }
  if (img === 'node') return `[${clock(d)} ${pick(['INFO', 'INFO', 'INFO', 'WARN'])}]: ${pick(['GET / 200 3ms', 'GET /api/items 200 12ms', 'POST /api/items 201 18ms', 'GET /api/items 200 1204ms'])}`;
  return `${clock(d)} backup done`;
}

function cloneDocker(d) {
  const out = {
    containers: d.containers.map(c => ({ ...c, ports: [...c.ports], mounts: c.mounts.map(m => ({ ...m })) })),
    images: d.images.map(i => ({ ...i })), volumes: d.volumes.map(v => ({ ...v })), compose: { ...(d.compose || {}) },
  };
  for (const c of out.containers) {
    const lines = c.image.startsWith('alpine')
      ? ['04:00:01 backup start', '04:00:02 tar: removing leading "/" from member names', '04:03:40 backup done']
      : Array.from({ length: 14 }, () => dockerLine(c));
    c.logNode = { name: c.name, title: `${c.name} のログ`, label: `docker logs -f ${c.name}`, path: `docker:${c.name}`, content: lines.join('\n') };
  }
  return out;
}

function openDocker(srv, o = {}) {
  const ex = srv.windows.find(w => w.type === 'docker');
  if (ex) { restoreFocus(ex); if (o.focus) ex.dockerFocus?.(o.focus); return ex; }
  return openWindow(srv, {
    type: 'docker', title: 'Docker', icon: 'box', w: 700, h: 420, ...(o.pos || {}),
    build(body, win) {
      const d = srv.docker;
      if (!d) { body.append(el('div', { class: 'empty' }, 'このサーバーには Docker が入っていません')); return; }
      let tab = 'containers';
      const open = new Set();
      const stats = new Map();
      const seg = el('div', { class: 'seg' });
      const tools = el('div', { class: 'dk-tools' });
      const list = el('div', { class: 'dk-list' });
      const foot = el('div', { class: 'win-foot' });
      body.append(el('div', { class: 'win-toolbar' }, seg, el('div', { class: 'grow' }), tools), list, foot);

      const say = msg => {
        foot.textContent = msg;
        foot.classList.add('show');
        clearTimeout(foot._t);
        foot._t = setTimeout(() => foot.classList.remove('show'), 2600);
      };
      const btn = (label, fn, cls = '') => el('button', { class: 'btn small ' + cls, onclick: e => { e.stopPropagation(); fn(); } }, label);
      const busy = (c, apply, msg) => {
        c.busy = true; draw();
        setTimeout(() => { c.busy = false; apply(); draw(); say(msg); }, 800);
      };
      const startC = c => busy(c, () => Object.assign(c, { state: 'running', up: 'Less than a second', upJa: '1 分未満' }), `${c.name} を起動しました`);
      const stopC = c => busy(c, () => Object.assign(c, { state: 'exited', exit: 0, ago: 'Less than a second', agoJa: 'たった今' }), `${c.name} を停止しました`);
      const restartC = c => busy(c, () => Object.assign(c, { up: 'Less than a second', upJa: '1 分未満' }), `${c.name} を再起動しました`);
      const shellC = c => openSessionWindow(srv, sessDocker(c.name, 'exec', `docker exec -it ${c.name} sh`));
      const logsC = c => { const w = srv.windows.find(x => x.node === c.logNode); if (w) restoreFocus(w); else openLogWindow(srv, c.logNode); };
      const openCompose = proj => { const n = findNode(srv, d.compose[proj]); if (n) openFile(srv, n); };
      const users = test => d.containers.filter(test).map(c => c.name);
      const userChips = names => el('div', { class: 'dk-ports' }, names.length ? names.map(u => el('span', { class: 'chip sm' }, u)) : el('span', { class: 'muted small' }, '未使用'));

      const kv = (k, ...v) => [el('span', { class: 'muted' }, k), el('div', {}, ...v)];
      const detail = c => el('div', { class: 'dk-detail' },
        kv('ID', el('span', { class: 'mono' }, hex(c.name))),
        kv('状態', c.state === 'running' ? `稼働中 (${c.upJa})` : `停止 (終了コード ${c.exit ?? 0} · ${c.agoJa})`),
        kv('ポート', c.ports.length ? c.ports.join(', ') : 'なし'),
        kv('マウント', c.mounts.length ? c.mounts.map(m => el('div', {},
          m.type === 'bind'
            ? el('button', { class: 'mlink', title: 'ツリーで表示', onclick: () => revealInTree(srv, m.src) }, tilde(srv, m.src))
            : el('button', { class: 'mlink', title: 'ボリュームを見る', onclick: () => { tab = 'volumes'; draw(); } }, m.src),
          el('span', { class: 'muted mono' }, ` → ${m.dst}`))) : 'なし'),
        c.project && kv('Compose', el('button', { class: 'mlink', onclick: () => openCompose(c.project) }, tilde(srv, d.compose[c.project]))));

      const row = c => {
        const run = c.state === 'running', isOpen = open.has(c.name);
        const stat = el('span', { class: 'dk-stat' });
        stats.set(c.name, { stat, c });
        const acts = el('div', { class: 'dk-acts' }, c.confirm
          ? [el('span', { class: 'muted small' }, '削除しますか'), btn('やめる', () => { c.confirm = false; draw(); }),
            btn('削除', () => { d.containers.splice(d.containers.indexOf(c), 1); draw(); say(`${c.name} を削除しました`); }, 'danger')]
          : run ? [btn('シェル', () => shellC(c)), btn('ログ', () => logsC(c)), btn('再起動', () => restartC(c)), btn('停止', () => stopC(c))]
            : [btn('ログ', () => logsC(c)), btn('起動', () => startC(c), 'primary'), btn('削除', () => { c.confirm = true; draw(); })]);
        if (c.busy) acts.querySelectorAll('button').forEach(b => { b.disabled = true; });
        const drag = run && !isMobile();
        const head = el('div', { class: 'dk-row', draggable: drag ? 'true' : null, title: drag ? 'デスクトップへドラッグするとシェルを開く' : null },
          ic('chevron', 'chev' + (isOpen ? ' open' : '')),
          el('span', { class: 'svc-dot ' + (c.busy ? 'busy' : run ? 'on' : 'off') }),
          el('div', { class: 'dk-name' }, el('div', { class: 'mono' }, c.name), el('div', { class: 'muted small' }, `${c.image} · ${run ? `稼働 ${c.upJa}` : `停止 ${c.agoJa}`}`)),
          el('div', { class: 'dk-ports' }, c.ports.map(p => el('span', { class: 'chip sm' }, p))),
          stat, acts);
        head.addEventListener('click', e => {
          if (e.target.closest('button')) return;
          if (isOpen) open.delete(c.name); else open.add(c.name);
          draw();
        });
        head.addEventListener('dragstart', e => { e.dataTransfer.setData('text/plain', `docker exec -it ${c.name} sh`); state.drag = { kind: 'container', name: c.name }; });
        head.addEventListener('dragend', () => { state.drag = null; });
        return el('div', { class: 'dk-item' + (isOpen ? ' open' : ''), dataset: { name: c.name } }, head, isOpen && detail(c));
      };

      const updateStats = () => {
        for (const { stat, c } of stats.values()) {
          if (c.state !== 'running') { stat.textContent = ''; continue; }
          const ps = srv.procs.filter(p => p.container === c.name);
          const cpu = ps.reduce((a, p) => a + p.cpu, 0), mem = ps.reduce((a, p) => a + p.rss, 0);
          stat.textContent = `CPU ${cpu.toFixed(1)}% · ${fmtMB(mem || 6)}`;
        }
      };

      const drawContainers = () => {
        stats.clear();
        tools.replaceChildren();
        const groups = new Map();
        for (const c of d.containers) {
          const k = c.project || '';
          if (!groups.has(k)) groups.set(k, []);
          groups.get(k).push(c);
        }
        const kids = [];
        for (const [proj, cs] of groups) {
          const anyRun = cs.some(c => c.state === 'running');
          if (proj) {
            kids.push(el('div', { class: 'dk-group' }, ic('layers'), el('span', {}, proj),
              d.compose[proj] && el('button', { class: 'mlink', title: '開く', onclick: () => openCompose(proj) }, tilde(srv, d.compose[proj])),
              el('div', { class: 'grow' }),
              anyRun && btn('すべて再起動', () => cs.filter(c => c.state === 'running').forEach(restartC)),
              anyRun ? btn('すべて停止', () => cs.filter(c => c.state === 'running').forEach(stopC)) : btn('すべて起動', () => cs.forEach(startC), 'primary')));
          } else if (groups.size > 1) kids.push(el('div', { class: 'dk-group' }, el('span', {}, 'Compose なし')));
          kids.push(...cs.map(row));
        }
        list.replaceChildren(...(kids.length ? kids : [el('div', { class: 'empty' }, 'コンテナはありません')]));
        updateStats();
      };

      const drawImages = () => {
        const name = el('input', { class: 'mini-input', placeholder: 'イメージ名', spellcheck: 'false' });
        const pull = () => {
          const v = name.value.trim();
          if (!v) { name.focus(); return; }
          const full = v.includes(':') ? v : `${v}:latest`;
          if (d.images.some(i => i.name === full)) { say(`${full} はもうあります`); return; }
          const img = { name: full, size: 0, pulling: 0 };
          d.images.unshift(img);
          draw();
          const t0 = performance.now();
          const step = () => {
            img.pulling = Math.min(1, (performance.now() - t0) / 1800);
            if (img.meterEl) setMeter(img.meterEl, img.pulling * 100);
            if (img.pulling < 1) { requestAnimationFrame(step); return; }
            delete img.pulling;
            delete img.meterEl;
            img.size = rand(20e6, 400e6);
            draw();
            say(`${full} を取得しました`);
          };
          step();
        };
        name.addEventListener('keydown', e => { if (e.key === 'Enter') pull(); });
        tools.replaceChildren(name, btn('取得', pull, 'primary'));
        list.replaceChildren(...d.images.map(img => {
          const used = users(c => c.image === img.name);
          const pulling = img.pulling != null;
          if (pulling) img.meterEl = meter(img.pulling * 100);
          return el('div', { class: 'dk-row static' }, ic('archive', 'f-arc'),
            el('div', { class: 'dk-name' }, el('div', { class: 'mono' }, img.name), el('div', { class: 'muted small' }, pulling ? '取得中' : `${fmtSize(img.size)} · ${hex(img.name).slice(0, 12)}`)),
            pulling ? img.meterEl : userChips(used),
            el('div', { class: 'dk-acts' }, !pulling && el('button', {
              class: 'btn small', disabled: used.length > 0, title: used.length ? '使っているコンテナがあります' : null,
              onclick: () => { d.images.splice(d.images.indexOf(img), 1); draw(); say(`${img.name} を削除しました`); },
            }, '削除')));
        }));
      };

      const drawVolumes = () => {
        tools.replaceChildren();
        list.replaceChildren(...(d.volumes.length ? d.volumes.map(v => {
          const used = users(c => c.mounts.some(m => m.type === 'volume' && m.src === v.name));
          return el('div', { class: 'dk-row static' }, ic('db', 'f-code'),
            el('div', { class: 'dk-name' }, el('div', { class: 'mono' }, v.name), el('div', { class: 'muted small mono' }, `${fmtSize(v.size)} · /var/lib/docker/volumes/${v.name}/_data`)),
            userChips(used),
            el('div', { class: 'dk-acts' }, el('button', {
              class: 'btn small', disabled: used.length > 0, title: used.length ? '使っているコンテナがあります' : null,
              onclick: () => { d.volumes.splice(d.volumes.indexOf(v), 1); draw(); say(`${v.name} を削除しました`); },
            }, '削除')));
        }) : [el('div', { class: 'empty' }, 'ボリュームはありません')]));
      };

      const draw = () => {
        seg.replaceChildren(...[['containers', 'コンテナ', d.containers.length], ['images', 'イメージ', d.images.length], ['volumes', 'ボリューム', d.volumes.length]]
          .map(([k, l, n]) => el('button', { class: k === tab ? 'active' : '', onclick: () => { tab = k; draw(); } }, `${l} ${n}`)));
        if (tab === 'containers') drawContainers(); else if (tab === 'images') drawImages(); else drawVolumes();
      };
      win.dockerFocus = name => {
        tab = 'containers';
        open.add(name);
        draw();
        const it = list.querySelector(`.dk-item[data-name="${CSS.escape(name)}"]`);
        it?.scrollIntoView({ block: 'nearest' });
        it?.classList.add('flash');
      };
      draw();
      if (o.focus) requestAnimationFrame(() => win.dockerFocus(o.focus));
      const tick = () => { if (tab === 'containers') updateStats(); };
      srv.tickers.add(tick);
      win.cleanup.push(() => srv.tickers.delete(tick));
    },
  });
}

const withSudo = (srv, reason, fn) => srv.sudo ? fn() : enableSudo(srv, reason, fn);

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const journalStamp = (d = new Date()) => `${MONTHS[d.getMonth()]} ${pad2(d.getDate())} ${clock(d)}`;

// journalctl -u の中身。サービスを起動・停止すると行が足される
function journalNode(srv, svc) {
  if (!svc.logNode) {
    const boot = new Date(srv.bootAt);
    const line = (d, t) => `${journalStamp(d)} ${srv.name} systemd[1]: ${t}`;
    const lines = [line(boot, `Starting ${svc.name} - ${svc.desc}...`), line(boot, `Started ${svc.name} - ${svc.desc}.`)];
    if (!svc.active) {
      const d = new Date(Date.now() - 2 * 3600e3);
      lines.push(line(d, `Stopping ${svc.name} - ${svc.desc}...`), line(d, `Stopped ${svc.name} - ${svc.desc}.`));
    }
    svc.logNode = { name: svc.name, title: `${svc.name} のログ`, label: `journalctl -u ${svc.name} -f`, path: `journal:${svc.name}`, content: lines.join('\n') };
  }
  return svc.logNode;
}

function openJournal(srv, svc) {
  const node = journalNode(srv, svc);
  const ex = srv.windows.find(w => w.node === node);
  if (ex) restoreFocus(ex); else openLogWindow(srv, node);
}

/* ---------- サイドバー ---------- */

function setView(v) {
  if (state.view === v && !isMobile()) { $('#app').classList.toggle('side-collapsed'); return; }
  $('#app').classList.remove('side-collapsed');
  applyView(v);
}

function applyView(v) {
  state.view = v;
  $('#view-files').hidden = v !== 'files';
  $('#view-sessions').hidden = v !== 'sessions';
  $$('[data-view]').forEach(b => b.classList.toggle('active', b.dataset.view === v));
  if (v === 'sessions') renderSessions(); else renderTree();
}

function showFilesView() {
  $('#app').classList.remove('side-collapsed');
  if (state.view !== 'files') applyView('files');
}

function revealInTree(srv, path) {
  const n = findNode(srv, path);
  if (!n) { toast(`${tilde(srv, path)} はツリーにありません`, 'warn'); return; }
  const go = () => {
    if (!isUnder(n, srv.viewRoot)) srv.viewRoot = inHome(srv, n) ? srv.homeNode : srv.root;
    for (let p = n.parent; p; p = p.parent) p.open = true;
    if (n.dir) n.open = true;
    srv.selected = n.path;
    if (isMobile()) setMTab('files'); else showFilesView();
    renderTree();
    const row = $(`.tree-row[data-path="${CSS.escape(n.path)}"]`);
    row?.scrollIntoView({ block: 'nearest' });
    row?.classList.add('flash');
  };
  if (!inHome(srv, n) && !srv.sudo) enableSudo(srv, 'ホームの外を開くには sudo モードを ON にしてください', go); else go();
}

/* ---------- sudo モード ---------- */

function setSudo(srv, on) {
  srv.sudo = on;
  if (!on && !inHome(srv, srv.viewRoot)) srv.viewRoot = srv.homeNode;
  srv.sudoSubs.forEach(f => f());
  if (srv === cur()) { renderSudo(); renderTree(); }
}

function renderSudo() {
  const on = !!cur()?.sudo;
  const b = $('#sudo-btn');
  b.classList.toggle('on', on);
  b.title = on ? 'sudo モード: ON (押すと OFF にする)' : 'sudo モード: OFF (押すと ON にする)';
  $('#app').classList.toggle('sudo-on', on);
}

function enableSudo(srv, reason, then) {
  const pass = el('input', { type: 'password', class: 'input', autocomplete: 'off' });
  const go = () => {
    if (!pass.value) { pass.focus(); return; }
    m.close();
    setSudo(srv, true);
    then?.();
  };
  pass.addEventListener('keydown', e => { if (e.key === 'Enter') go(); });
  const body = el('div', {},
    reason && el('p', { class: 'conf-text' }, reason),
    el('label', { class: 'field' }, el('span', { class: 'field-label' }, `${srv.user}@${srv.name} の sudo のパスワード`), pass),
    el('div', { class: 'modal-foot' }, el('div', { class: 'grow' }),
      el('button', { class: 'btn', onclick: () => m.close() }, 'キャンセル'),
      el('button', { class: 'btn primary', onclick: go }, 'ON にする')));
  const m = modal({ title: 'sudo モードを ON にする', body, width: 420 });
  pass.focus();
}

// root のものに触る操作は、sudo モードが OFF ならまず ON にしてもらう
function guard(srv, nodes, what, fn) {
  if (!srv.sudo && nodes.some(n => n && n.owner === 'root')) enableSudo(srv, `${what}には sudo モードを ON にしてください`, fn);
  else fn();
}

/* ---------- ファイル操作 ---------- */

const isProtected = (srv, n) => n === srv.root || n === srv.homeNode || n === srv.homeNode.parent;
const cloneNode = n => n.dir
  ? { name: n.name, dir: true, open: false, owner: n.owner, children: n.children.map(cloneNode) }
  : { name: n.name, dir: false, size: n.size, content: n.content, owner: n.owner };

function copyName(dir, name) {
  if (!dir.children.some(c => c.name === name)) return name;
  const m = name.match(/^(.+?)(\.[^.]*)?$/);
  for (let i = 1; ; i++) {
    const n = `${m[1]} - コピー${i > 1 ? ` (${i})` : ''}${m[2] || ''}`;
    if (!dir.children.some(c => c.name === n)) return n;
  }
}

function setClip(srv, node, mode) {
  state.clip = { srv, node, mode };
  renderTree();
}

function paste(srv, dir) {
  const c = state.clip;
  if (!c || c.srv !== srv) return;
  guard(srv, [dir, c.mode === 'cut' && c.node, c.mode === 'cut' && c.node.parent], '貼り付ける', () => {
    if (c.mode === 'cut') {
      if (isUnder(dir, c.node)) { toast('フォルダをその中へは移せません', 'warn'); return; }
      state.clip = null;
      moveNode(srv, c.node, dir);
      return;
    }
    const copy = cloneNode(c.node);
    copy.name = copyName(dir, c.node.name);
    setOwner(copy, dir.owner === 'root' ? 'root' : undefined);
    dir.children.push(copy);
    link(copy, dir);
    dir.open = true;
    srv.selected = copy.path;
    renderTree();
    $(`.tree-row[data-path="${CSS.escape(copy.path)}"]`)?.classList.add('flash');
  });
}

function createIn(srv, dir, isDir) {
  guard(srv, [dir], `${tilde(srv, dir.path)} に作る`, () => {
    const name = uniqueName(dir, isDir ? '新しいフォルダー' : '新しいファイル.txt');
    const n = isDir ? D(name, []) : { name, dir: false, size: 0, content: '' };
    if (dir.owner === 'root') n.owner = 'root';
    dir.children.push(n);
    link(n, dir);
    dir.open = true;
    srv.selected = n.path;
    renderTree();
    startRename(srv, n);
  });
}

function startRename(srv, node) {
  if (isProtected(srv, node)) return;
  const row = $(`.tree-row[data-path="${CSS.escape(node.path)}"]`);
  if (!row) return;
  const input = el('input', { class: 'rename-input', value: node.name, spellcheck: 'false' });
  row.querySelector('.name').replaceWith(input);
  row.draggable = false;
  const dot = node.dir ? -1 : node.name.lastIndexOf('.');
  input.focus();
  input.setSelectionRange(0, dot > 0 ? dot : node.name.length);
  let done = false;
  const finish = ok => {
    if (done) return;
    done = true;
    const v = input.value.trim();
    if (ok && v && v !== node.name) {
      if (v.includes('/')) toast('名前に / は使えません', 'warn');
      else if (node.parent.children.some(c => c !== node && c.name === v)) toast(`${v} はもうあります`, 'warn');
      else {
        node.name = v;
        link(node, node.parent);
        srv.selected = node.path;
        srv.windows.filter(w => w.node === node).forEach(w => { w.titleEl.textContent = v; });
        renderTaskbar();
      }
    }
    renderTree();
    $('#tree').focus({ preventScroll: true });
  };
  input.addEventListener('keydown', e => {
    e.stopPropagation();
    if (e.key === 'Enter') finish(true);
    if (e.key === 'Escape') finish(false);
  });
  input.addEventListener('blur', () => finish(true));
  input.addEventListener('click', e => e.stopPropagation());
  input.addEventListener('dblclick', e => e.stopPropagation());
}

function renameNode(srv, node) {
  guard(srv, [node, node.parent], `${node.name} の名前を変える`, () => startRename(srv, node));
}

function deleteNode(srv, node) {
  if (isProtected(srv, node)) return;
  guard(srv, [node, node.parent], `${node.name} を削除する`, () => {
    const parent = node.parent;
    parent.children.splice(parent.children.indexOf(node), 1);
    const item = { id: 't' + Date.now() + Math.random(), node, from: parent.path, at: new Date() };
    srv.trash.unshift(item);
    if (state.clip?.node === node) state.clip = null;
    srv.selected = parent.path;
    renderTree();
    refreshTrash(srv);
    toast(`${node.name} をゴミ箱へ移しました`, 'info', { action: { label: '元に戻す', fn: () => restoreTrash(srv, item) } });
  });
}

function restoreTrash(srv, item) {
  if (!srv.trash.includes(item)) return;
  const parent = findNode(srv, item.from) || srv.homeNode;
  item.node.name = uniqueName(parent, item.node.name);
  parent.children.push(item.node);
  link(item.node, parent);
  parent.open = true;
  srv.trash.splice(srv.trash.indexOf(item), 1);
  srv.selected = item.node.path;
  renderTree();
  refreshTrash(srv);
  $(`.tree-row[data-path="${CSS.escape(item.node.path)}"]`)?.classList.add('flash');
}

const refreshTrash = srv => srv.windows.filter(w => w.type === 'trash').forEach(w => w.refresh());

function openTrash(srv) {
  const ex = srv.windows.find(w => w.type === 'trash');
  if (ex) { restoreFocus(ex); return ex; }
  return openWindow(srv, {
    type: 'trash', title: 'ゴミ箱', icon: 'trash', w: 520, h: 320,
    build(body, win) {
      const bar = el('div', { class: 'win-toolbar' });
      const list = el('div', { class: 'dk-list' });
      body.append(bar, list);
      let confirming = false;
      win.refresh = () => {
        const n = srv.trash.length;
        bar.replaceChildren(el('span', { class: 'status-text' }, `${n} 件`), el('div', { class: 'grow' }),
          ...(confirming
            ? [el('span', { class: 'muted small' }, 'すべて完全に削除しますか'),
              el('button', { class: 'btn small', onclick: () => { confirming = false; win.refresh(); } }, 'やめる'),
              el('button', { class: 'btn small danger', onclick: () => { srv.trash.length = 0; confirming = false; win.refresh(); } }, '空にする')]
            : [el('button', { class: 'btn small', disabled: !n, onclick: () => { confirming = true; win.refresh(); } }, 'ゴミ箱を空にする')]));
        list.replaceChildren(...(n ? srv.trash.map(it => {
          const [icName, icCls] = KIND_ICON[fileKind(it.node)];
          return el('div', { class: 'dk-row static' }, ic(icName, icCls),
            el('div', { class: 'dk-name' }, el('div', { class: 'mono' }, it.node.name), el('div', { class: 'muted small mono' }, `${tilde(srv, it.from)}/ · ${clock(it.at)} に削除`)),
            el('div', { class: 'dk-acts' },
              el('button', { class: 'btn small', onclick: () => restoreTrash(srv, it) }, '元に戻す'),
              el('button', { class: 'btn small', onclick: () => { srv.trash.splice(srv.trash.indexOf(it), 1); win.refresh(); } }, '完全に削除')));
        }) : [el('div', { class: 'empty' }, 'ゴミ箱は空です')]));
      };
      win.refresh();
    },
  });
}

function copyText(text, msg) {
  const done = () => toast(msg, 'ok');
  const fallback = () => {
    const ta = el('textarea', { style: 'position:fixed;opacity:0' });
    ta.value = text;
    document.body.append(ta);
    ta.select();
    document.execCommand('copy');
    ta.remove();
    done();
  };
  if (navigator.clipboard?.writeText) navigator.clipboard.writeText(text).then(done, fallback); else fallback();
}

function showProps(srv, node) {
  const kinds = { dir: 'フォルダー', log: 'ログ', img: '画像', arc: '圧縮ファイル', bin: 'バイナリ', code: 'テキスト (設定・コード)', text: 'テキスト' };
  const count = n => n.dir ? n.children.reduce((a, c) => a + count(c), 0) : 1;
  const owner = node.owner || srv.user;
  const perm = node.private ? 'drwx------' : node.dir ? 'drwxr-xr-x' : node.name.endsWith('.sh') ? '-rwxr-xr-x' : '-rw-r--r--';
  const [icName, icCls] = KIND_ICON[fileKind(node)];
  const kv = (k, v) => [el('span', { class: 'muted' }, k), el('span', { class: 'mono' }, v)];
  modal({
    title: 'プロパティ', width: 460,
    body: el('div', {},
      el('div', { class: 'props-head' }, ic(icName, icCls), el('span', {}, node.name || '/')),
      el('div', { class: 'props' },
        kv('種類', kinds[fileKind(node)]),
        kv('場所', node.parent ? node.parent.path : '-'),
        kv('サイズ', node.dir ? `${fmtSize(dirSize(node))} (${count(node)} 個のファイル)` : fmtSize(node.size || 0)),
        kv('所有者', `${owner}:${owner}`),
        kv('権限', perm),
        kv('更新日時', '2026/10/24 09:00'))),
  });
}

function renderSide() { if (state.view === 'sessions') renderSessions(); }

function renderTree() {
  const srv = cur();
  if (!srv) return;
  const box = $('#tree');
  const v = srv.viewRoot;
  const root = el('div', { class: 'tree-root' + (srv.selected === v.path ? ' selected' : ''), title: 'PC のファイルをここへドラッグするとアップロード' },
    v.parent && el('button', { class: 'nav-btn', title: '上の階層へ', onclick: e => { e.stopPropagation(); goUp(srv); } }, ic('up')),
    v !== srv.homeNode && el('button', { class: 'nav-btn', title: 'ホームへ', onclick: e => { e.stopPropagation(); srv.viewRoot = srv.homeNode; renderTree(); } }, ic('home')),
    ic('folder', 'ficon f-dir'), el('span', { class: 'name' }, v.path),
    el('button', { class: 'row-more', title: '操作', onclick: e => { e.stopPropagation(); const r = e.currentTarget.getBoundingClientRect(); nodeMenu(srv, v, r.left, r.bottom + 4); } }, ic('more')));
  root.addEventListener('click', () => { srv.selected = v.path; renderTree(); });
  root.addEventListener('contextmenu', e => { e.preventDefault(); e.stopPropagation(); srv.selected = v.path; renderTree(); nodeMenu(srv, v, e.clientX, e.clientY); });
  const rows = [root];
  const walk = (n, d) => {
    for (const c of sortKids(n)) {
      rows.push(treeRow(srv, c, d));
      if (c.dir && c.open) walk(c, d + 1);
    }
    for (const u of srv.uploads) if (u.dir === n) rows.push(pendingRow(u, d));
  };
  walk(v, 0);
  box.replaceChildren(...rows);
}

function goUp(srv) {
  const p = srv.viewRoot.parent;
  if (!p) return;
  const go = () => { srv.viewRoot = p; p.open = true; renderTree(); };
  if (!inHome(srv, p) && !srv.sudo) enableSudo(srv, 'ホームの外を開くには sudo モードを ON にしてください', go); else go();
}

function treeRow(srv, node, depth) {
  const [icName, icCls] = KIND_ICON[fileKind(node)];
  const row = el('div', {
    class: 'tree-row' + (depth === 0 ? ' d0' : '') + (srv.selected === node.path ? ' selected' : '')
      + (state.clip?.mode === 'cut' && state.clip.node === node ? ' cut' : ''),
    draggable: isMobile() ? null : 'true', style: `--d:${depth}`, dataset: { path: node.path },
  },
    node.dir ? ic('chevron', 'chev' + (node.open ? ' open' : '')) : el('span', { class: 'chev-space' }),
    ic(icName, 'ficon ' + icCls),
    el('span', { class: 'name' }, node.name),
    node.owner === 'root' && el('span', { class: 'row-lock', title: 'root のファイル。変更には sudo モードが要ります' }, ic('lock')),
    ['arc', 'bin'].includes(fileKind(node)) && el('span', { class: 'row-size' }, fmtSize(node.size || 0)),
    el('button', { class: 'row-more', title: '操作', onclick: e => { e.stopPropagation(); const r = e.currentTarget.getBoundingClientRect(); nodeMenu(srv, node, r.left, r.bottom + 4); } }, ic('more')));
  row.addEventListener('click', () => {
    if (row.dataset.lp) { delete row.dataset.lp; return; }
    srv.selected = node.path;
    if (node.dir) node.open = !node.open;
    renderTree();
  });
  row.addEventListener('dblclick', () => { if (!node.dir) openFile(srv, node); });
  row.addEventListener('contextmenu', e => {
    e.preventDefault();
    e.stopPropagation();
    srv.selected = node.path;
    renderTree();
    nodeMenu(srv, node, e.clientX, e.clientY);
  });
  row.addEventListener('dragstart', e => {
    e.dataTransfer.setData('application/x-drag-path', node.path);
    e.dataTransfer.setData('text/plain', node.path);
    e.dataTransfer.effectAllowed = 'copyMove';
    state.drag = { kind: 'path', node };
    row.classList.add('dragging');
  });
  row.addEventListener('dragend', () => { row.classList.remove('dragging'); state.drag = null; });
  // スマホでは長押しでメニュー
  row.addEventListener('pointerdown', e => {
    if (e.pointerType !== 'touch') return;
    const t = setTimeout(() => { row.dataset.lp = '1'; nodeMenu(srv, node, e.clientX, e.clientY); }, 480);
    const cancel = () => clearTimeout(t);
    row.addEventListener('pointerup', cancel, { once: true });
    row.addEventListener('pointercancel', cancel, { once: true });
  });
  return row;
}

// エクスプローラーと同じ並びにする。開く系 → 新規 → 切り取り・コピー・貼り付け → 名前・削除 → パス・ダウンロード・プロパティ
function nodeMenu(srv, node, x, y) {
  const k = fileKind(node);
  const toDesk = fn => () => { fn(); if (isMobile()) setMTab('desktop'); };
  const prot = isProtected(srv, node);
  const canPaste = state.clip?.srv === srv;
  const pasteTo = node.dir ? node : node.parent;
  const items = [
    k === 'arc' && { icon: 'archive', label: '解凍して閲覧', fn: toDesk(() => openArchive(srv, node)) },
    !node.dir && !['arc', 'bin'].includes(k) && { icon: 'window', label: 'デスクトップで開く', fn: toDesk(() => openFile(srv, node)) },
    node.dir
      ? { icon: 'terminal', label: 'ターミナルで cd', fn: () => cdTo(srv, node.path) }
      : { icon: 'terminal', label: 'ターミナルにパスを入力', fn: () => insertPath(srv, node.path) },
    { sep: true },
    node.dir && { icon: 'filePlus', label: '新しいファイル', fn: () => createIn(srv, node, false) },
    node.dir && { icon: 'folderPlus', label: '新しいフォルダー', fn: () => createIn(srv, node, true) },
    node.dir && { icon: 'upload', label: 'ここにアップロード', fn: () => guard(srv, [node], `${tilde(srv, node.path)} にアップロードする`, () => pickUpload(node)) },
    { sep: true },
    { icon: 'cut', label: '切り取り', hint: 'Ctrl+X', disabled: prot, fn: () => setClip(srv, node, 'cut') },
    { icon: 'copy', label: 'コピー', hint: 'Ctrl+C', disabled: prot && node !== srv.homeNode, fn: () => setClip(srv, node, 'copy') },
    { icon: 'paste', label: '貼り付け', hint: 'Ctrl+V', disabled: !canPaste, fn: () => paste(srv, pasteTo) },
    { sep: true },
    { icon: 'edit', label: '名前を変更', hint: 'F2', disabled: prot, fn: () => renameNode(srv, node) },
    { icon: 'trash', label: '削除', hint: 'Delete', disabled: prot, danger: true, fn: () => deleteNode(srv, node) },
    { sep: true },
    { icon: 'link', label: 'パスをコピー', fn: () => copyText(node.path, 'パスをコピーしました') },
    { icon: 'download', label: node.dir ? 'zip にしてダウンロード' : 'ダウンロード', disabled: node === srv.root, fn: () => download(node) },
    node === srv.viewRoot && { icon: 'trash', label: 'ゴミ箱を開く', fn: toDesk(() => openTrash(srv)) },
    { icon: 'info', label: 'プロパティ', fn: () => showProps(srv, node) },
  ];
  showMenu(x, y, items, isMobile() ? tilde(srv, node.path) : null);
}

function insertPath(srv, path) {
  const v = srv.panel.view;
  if (v.active?.raw) { toast('ターミナルでコマンドが動いています', 'warn'); return; }
  if (isMobile()) setMTab('terminal');
  v.insert(pathArg(srv, path));
}

function cdTo(srv, path) {
  if (srv.panel.fg) { toast('ターミナルでコマンドが動いています', 'warn'); return; }
  if (isMobile()) setMTab('terminal');
  srv.panel.shell.input(`cd ${pathArg(srv, path)}`);
  srv.panel.view.focus();
}

function download(node) {
  const blob = new Blob([node.content ?? `DRAG mock file: ${node.name}\n`], { type: 'application/octet-stream' });
  const url = URL.createObjectURL(blob);
  el('a', { href: url, download: node.dir ? `${node.name}.zip` : node.name }).click();
  setTimeout(() => URL.revokeObjectURL(url), 2000);
}

function pickUpload(node) {
  state.uploadTarget = node;
  $('#upload-input').click();
}

function uniqueName(dir, name) {
  if (!dir.children.some(c => c.name === name)) return name;
  const m = name.match(/^(.*?)(\.[^.]*)?$/);
  for (let i = 1; ; i++) {
    const n = `${m[1]} (${i})${m[2] || ''}`;
    if (!dir.children.some(c => c.name === n)) return n;
  }
}

function pendingRow(u, depth) {
  return el('div', { class: 'tree-row pending' + (depth === 0 ? ' d0' : ''), style: `--d:${depth}`, dataset: { up: u.id } },
    el('span', { class: 'chev-space' }),
    ic(u.isDir ? 'folder' : 'file', 'ficon ' + (u.isDir ? 'f-dir' : 'f-file')),
    el('span', { class: 'name' }, u.name),
    el('div', { class: 'up-meter' }, el('i', { style: `width:${u.p * 100}%` })),
    el('span', { class: 'row-size' }, `${Math.round(u.p * 100)}%`));
}

function updatePending(u) {
  const r = $(`.tree-row[data-up="${u.id}"]`);
  if (!r) return;
  r.querySelector('.up-meter i').style.width = `${u.p * 100}%`;
  r.querySelector('.row-size').textContent = `${Math.round(u.p * 100)}%`;
}

// ドロップされたものを { name, file } と { name, children } の木にする。フォルダも中身ごと読む
function dropSpecs(dt) {
  const got = [...(dt.items || [])].filter(i => i.kind === 'file').map(i => i.webkitGetAsEntry?.() || i.getAsFile()).filter(Boolean);
  return Promise.all((got.length ? got : [...dt.files]).map(readSpec));
}
async function readSpec(x) {
  if (x instanceof File) return { name: x.name, file: x };
  if (x.isFile) return { name: x.name, file: await new Promise(r => x.file(r, () => r(null))) };
  const reader = x.createReader(), children = [];
  for (;;) {
    const batch = await new Promise(r => reader.readEntries(r, () => r([])));
    if (!batch.length) break;
    children.push(...await Promise.all(batch.map(readSpec)));
  }
  return { name: x.name, children };
}
const specSize = s => s.children ? s.children.reduce((a, c) => a + specSize(c), 0) : (s.file?.size || 0);
async function specToNode(s, name = s.name) {
  if (s.children) {
    const n = D(name, []);
    for (const c of s.children) n.children.push(await specToNode(c));
    return n;
  }
  const n = { name, dir: false, size: s.file?.size || 0 };
  const f = s.file;
  if (f && f.size < 512 * 1024 && (/^text\//.test(f.type) || TEXT_EXT.test(f.name))) {
    try { n.content = await f.text(); } catch { /* 読めなければバイナリ扱い */ }
  }
  return n;
}
function mergeInto(dir, add) {
  for (const c of add.children) {
    const e = dir.children.find(x => x.name === c.name);
    if (e && e.dir && c.dir) { mergeInto(e, c); continue; }
    if (e) dir.children.splice(dir.children.indexOf(e), 1);
    dir.children.push(c);
    link(c, dir);
  }
}

function askConflict(srv, exist, more) {
  return new Promise(res => {
    const all = el('input', { type: 'checkbox' });
    let done = false;
    const choose = choice => { done = true; m.close(); res({ choice, all: all.checked }); };
    const body = el('div', {},
      el('p', { class: 'conf-text' }, el('span', { class: 'mono' }, tilde(srv, exist.path)), ' は、もうあります'),
      more && el('label', { class: 'toggle' }, all, 'このあとも同じにする'),
      el('div', { class: 'modal-foot' },
        el('button', { class: 'btn', onclick: () => choose('skip') }, 'スキップ'),
        el('div', { class: 'grow' }),
        el('button', { class: 'btn', onclick: () => choose('keep') }, '両方残す'),
        el('button', { class: 'btn primary', onclick: () => choose('overwrite') }, '上書き')));
    const m = modal({ title: '同じ名前があります', body, width: 460, onClose: () => { if (!done) res({ choice: 'skip', all: false }); } });
  });
}

async function upload(srv, dir, specs) {
  const pref = state.settings.uploadConflict;
  let fixed = pref === 'ask' ? null : { choice: pref, all: true }, count = 0, lastPath = null;
  for (let i = 0; i < specs.length; i++) {
    const s = specs[i];
    const exist = dir.children.find(c => c.name === s.name);
    let mode = 'new';
    if (exist) {
      const ans = fixed || await askConflict(srv, exist, i < specs.length - 1);
      if (ans.all) fixed = ans;
      if (ans.choice === 'skip') continue;
      mode = ans.choice;
    }
    const name = mode === 'keep' ? uniqueName(dir, s.name) : s.name;
    const u = { id: 'u' + (++uploadSeq), dir, name, isDir: !!s.children, p: 0 };
    srv.uploads.push(u);
    dir.open = true;
    if (srv === cur()) renderTree();
    const dur = clamp(specSize(s) / 8e6 * 1000, 500, 3500), t0 = performance.now();
    await new Promise(res => {
      const step = () => {
        u.p = Math.min(1, (performance.now() - t0) / dur);
        updatePending(u);
        if (u.p < 1) requestAnimationFrame(step); else res();
      };
      step();
    });
    srv.uploads.splice(srv.uploads.indexOf(u), 1);
    const node = await specToNode(s, name);
    if (dir.owner === 'root') setOwner(node, 'root');
    if (mode === 'overwrite' && exist.dir && node.dir) { mergeInto(exist, node); lastPath = exist.path; }
    else {
      if (mode === 'overwrite') dir.children.splice(dir.children.indexOf(exist), 1);
      dir.children.push(node);
      link(node, dir);
      lastPath = node.path;
    }
    count++;
  }
  if (lastPath) srv.selected = lastPath;
  if (srv === cur()) {
    renderTree();
    if (lastPath) $(`.tree-row[data-path="${CSS.escape(lastPath)}"]`)?.classList.add('flash');
  }
  if (count) toast(`${count} 件を ${tilde(srv, dir.path)}/ にアップロードしました`, 'ok');
}

function moveNode(srv, node, dir) {
  if (node === dir || node.parent === dir || dir.path.startsWith(node.path + '/')) return;
  node.parent.children.splice(node.parent.children.indexOf(node), 1);
  node.name = uniqueName(dir, node.name);
  dir.children.push(node);
  link(node, dir);
  dir.open = true;
  srv.selected = node.path;
  renderTree();
  toast(`${node.name} を ${tilde(srv, dir.path)}/ へ移動しました`, 'ok');
}

function renderSessions() {
  const srv = cur();
  if (!srv) return;
  const box = $('#session-list');
  const kids = [];
  const group = (label, items) => {
    kids.push(el('div', { class: 's-group' }, label));
    if (!items.length) kids.push(el('div', { class: 's-empty' }, 'なし'));
    kids.push(...items);
  };
  group('screen', srv.screens.map(d => sessItem(srv, 'screen', d)));
  group('tmux', srv.tmux.map(d => sessItem(srv, 'tmux', d)));
  group('DRAG のウィンドウ', srv.windows.filter(w => (w.session && !w.session.detachable) || w.host).map(w =>
    el('div', { class: 's-item', onclick: () => restoreFocus(w) }, ic(w.icon, 's-ic'),
      el('div', { class: 's-text' }, el('div', { class: 's-name' }, w.titleEl.textContent), el('div', { class: 's-meta' }, w.session?.cmdline || 'シェル')))));
  box.replaceChildren(...kids);
}

function sessItem(srv, kind, d) {
  const key = `${kind}:${d.name}`;
  const s = srv.persist.get(key);
  const where = s?.window ? 'ウィンドウ' : s && srv.panel.fg === s ? 'ターミナル' : null;
  const item = el('div', { class: 's-item', draggable: isMobile() ? null : 'true', onclick: () => openPersistent(srv, kind, d.name) },
    ic('layers', 's-ic'),
    el('div', { class: 's-text' }, el('div', { class: 's-name' }, d.name), el('div', { class: 's-meta' }, d.desc)),
    where && el('span', { class: 'pill' }, where));
  item.addEventListener('dragstart', e => {
    e.dataTransfer.setData('application/x-drag-session', key);
    state.drag = { kind: 'session', key };
  });
  item.addEventListener('dragend', () => { state.drag = null; });
  return item;
}

function openPersistent(srv, kind, name, pos) {
  const s = getPersistent(srv, kind, name);
  if (!s) return;
  if (isMobile()) setMTab('desktop');
  if (srv.panel.fg === s) srv.panel.popOut(pos);
  else openSessionWindow(srv, s, pos);
}

function attachToTerminal(srv, key) {
  const [kind, name] = key.split(':');
  const s = getPersistent(srv, kind, name);
  if (!s || srv.panel.fg === s) return;
  if (srv.panel.fg) { toast('ターミナルで別のコマンドが動いています', 'warn'); return; }
  if (s.window) closeWindow(s.window, { keepSession: true });
  srv.panel.shell.echo(s.cmdline);
  srv.panel.setForeground(s);
  srv.panel.view.focus();
}

/* ---------- 上のタブ・タスクバー・ステータスバー ---------- */

function renderTabs() {
  $('#server-tabs').replaceChildren(...state.list.map(srv => el('div', {
    class: 'server-tab' + (srv === cur() ? ' active' : ''), role: 'tab', title: `${srv.user}@${srv.host}`,
    onclick: () => switchServer(srv.id),
  },
    el('span', { class: 'dot' + dotClass(srv) }),
    el('span', { class: 'tab-name' }, srv.name),
    el('span', { class: 'tab-host' }, srv.host),
    el('button', { class: 'tab-close', title: '切断', onclick: e => { e.stopPropagation(); disconnect(srv); } }, ic('x')))));
}

function renderTaskbar() {
  const srv = cur();
  if (!srv) return;
  $('#task-items').replaceChildren(...srv.windows.map(w => {
    const item = el('div', {
      class: 'task-item' + (w === srv.focused && !w.min ? ' active' : '') + (w.min ? ' minimized' : ''),
      dataset: { win: w.id }, title: w.titleEl.textContent, role: 'button',
    }, ic(w.icon), el('span', { class: 'task-label' }, w.titleEl.textContent),
    el('button', { class: 'task-close', title: '閉じる', onclick: e => { e.stopPropagation(); closeWindow(w); } }, ic('x')));
    item.addEventListener('click', () => { if (!state.taskDragged) taskClick(w); });
    wireTaskDrag(item, w);
    return item;
  }));
  $('#task-items .task-item.active')?.scrollIntoView({ block: 'nearest', inline: 'nearest' });
  $('#desktop-empty').hidden = srv.windows.length > 0;
  renderSide();
}

// 下のタブはドラッグで並べ替える。並び順は srv.windows の順
function wireTaskDrag(item, w) {
  item.addEventListener('pointerdown', e => {
    if (e.button !== 0 || e.pointerType === 'touch' || e.target.closest('.task-close')) return;
    const box = $('#task-items');
    const items = [...box.children];
    const rects = items.map(i => i.getBoundingClientRect());
    const idx = items.indexOf(item);
    const startX = e.clientX;
    let moved = false, target = idx;
    item.setPointerCapture(e.pointerId);
    const move = ev => {
      const dx = ev.clientX - startX;
      if (!moved) {
        if (Math.abs(dx) < 5) return;
        moved = true;
        item.classList.add('reordering');
      }
      const cdx = clamp(dx, rects[0].left - rects[idx].left, last(rects).right - rects[idx].right);
      item.style.transform = `translateX(${cdx}px)`;
      const center = rects[idx].left + rects[idx].width / 2 + cdx;
      target = rects.filter((r, i) => i !== idx && center > r.left + r.width / 2).length;
      const shift = rects[idx].width + 4;
      items.forEach((it, i) => {
        if (i === idx) return;
        const s = idx < target && i > idx && i <= target ? -shift : idx > target && i >= target && i < idx ? shift : 0;
        it.style.transform = s ? `translateX(${s}px)` : '';
      });
    };
    const up = () => {
      item.removeEventListener('pointermove', move);
      item.removeEventListener('pointerup', up);
      item.removeEventListener('pointercancel', up);
      if (!moved) return;
      state.taskDragged = true;
      setTimeout(() => { state.taskDragged = false; }, 0);
      const arr = w.srv.windows;
      arr.splice(target, 0, arr.splice(idx, 1)[0]);
      renderTaskbar();
    };
    item.addEventListener('pointermove', move);
    item.addEventListener('pointerup', up);
    item.addEventListener('pointercancel', up);
  });
}

function taskClick(w) {
  if (isMobile()) { if (w.min) unminimize(w); else focusWin(w); return; }
  if (w.min) unminimize(w);
  else if (w === w.srv.focused) minimize(w);
  else focusWin(w);
}

function fmtUptime(ms) {
  const m = Math.floor(ms / 60000), d = Math.floor(m / 1440), h = Math.floor(m % 1440 / 60), mi = m % 60;
  return d ? `${d} 日 ${h} 時間` : h ? `${h} 時間 ${mi} 分` : `${mi} 分`;
}

function renderStatus() {
  const srv = cur();
  if (!srv) { $('#statusbar').replaceChildren(); state.sbRefs = null; return; }
  const sb = state.settings.sb;
  const refs = {};
  const boot = new Date(srv.bootAt);
  const gauge = (key, label, view, cls) => {
    refs[key] = { sp: spark(), t: el('span', { class: 'mono' }) };
    refs[key].sp.el.classList.add('sb-spark');
    if (cls) refs[key].sp.el.classList.add(cls);
    return el('button', { class: 'sb-item', title: `${label}の詳細`, onclick: () => openMonitor(srv, { view }) }, label, refs[key].sp.el, refs[key].t);
  };
  if (sb.net) Object.assign(refs, { rx: el('span', { class: 'mono' }), tx: el('span', { class: 'mono' }) });
  if (sb.uptime) refs.up = el('span');
  $('#statusbar').replaceChildren(...[
    el('span', { class: 'sb-item' }, el('span', { class: 'dot' + dotClass(srv) }),
      srv.conn !== 'ok' ? `${srv.user}@${srv.host} · 接続が切れています` : `${srv.user}@${srv.host}`),
    el('button', { class: 'sb-item', title: 'スマホを登録', onclick: () => openPair() }, ic('phone'), `スマホ ${state.devices.length}台`),
    el('span', { class: 'grow' }),
    sb.uptime && el('span', { class: 'sb-item', title: `${boot.getMonth() + 1}/${boot.getDate()} ${pad2(boot.getHours())}:${pad2(boot.getMinutes())} の起動から連続で稼働` }, ic('clock'), refs.up),
    sb.cpu && gauge('cpu', 'CPU', 'cpu', ''),
    sb.mem && gauge('mem', 'メモリ', 'mem', 'alt'),
    sb.net && el('button', { class: 'sb-item sb-net', title: 'ネットワークの詳細', onclick: () => openMonitor(srv, { view: 'net' }) },
      el('span', { class: 'down', title: '下り' }, ic('down')), refs.rx, el('span', { class: 'up', title: '上り' }, ic('up')), refs.tx),
  ].filter(Boolean));
  state.sbRefs = refs;
  updateStatus();
}

function updateStatus() {
  const srv = cur(), r = state.sbRefs;
  if (!srv || !r) return;
  const m = srv.metrics;
  if (r.up) r.up.textContent = `稼働 ${fmtUptime(Date.now() - srv.bootAt)}`;
  if (r.cpu) { r.cpu.t.textContent = `${last(m.cpu).toFixed(0)}%`; r.cpu.sp.set(m.cpu, 100); }
  if (r.mem) { r.mem.t.textContent = `${last(m.mem).toFixed(0)}%`; r.mem.sp.set(m.mem, 100); }
  if (r.rx) { r.rx.textContent = `${last(m.net).toFixed(0)} KB/s`; r.tx.textContent = `${last(m.tx).toFixed(0)} KB/s`; }
}

function updateChip() {
  const srv = cur();
  if (!srv) return;
  const chip = $('#fg-chip');
  const fg = srv.panel.fg;
  chip.hidden = !fg;
  if (fg) chip.replaceChildren(ic('grip', 'grip'), el('span', { class: 'mono' }, fg.cmdline || fg.title), el('span', { class: 'chip-hint' }, isMobile() ? 'ウィンドウに移す' : 'ウィンドウへドラッグ'));
}

/* ---------- サーバー ---------- */

const MKEYS = ['cpu', 'mem', 'disk', 'net', 'tx', 'dr', 'dw'];
function walk(v, target, step, lo, hi) { return clamp(v + rand(-step, step) + (target - v) * .08, lo, hi); }
function nextMetrics(b, v) {
  return {
    cpu: walk(v.cpu, b.cpu, 6, 2, 97), mem: walk(v.mem, b.mem, .8, 10, 95), disk: v.disk,
    net: walk(v.net, 140, 50, 3, 900), tx: walk(v.tx, 45, 20, 1, 400), dr: walk(v.dr, 30, 25, 0, 800), dw: walk(v.dw, 60, 35, 0, 900),
  };
}
function initMetrics(b) {
  const m = { base: b };
  MKEYS.forEach(k => { m[k] = []; });
  let v = { cpu: b.cpu, mem: b.mem, disk: b.disk, net: 120, tx: 40, dr: 30, dw: 60 };
  for (let i = 0; i < 60; i++) {
    v = nextMetrics(b, v);
    MKEYS.forEach(k => m[k].push(v[k]));
  }
  return m;
}
function stepMetrics(m) {
  const v = nextMetrics(m.base, Object.fromEntries(MKEYS.map(k => [k, last(m[k])])));
  MKEYS.forEach(k => { m[k].push(v[k]); m[k].shift(); });
}

// 止まっているコンテナの中のプロセスは数えない
const liveProcs = srv => srv.procs.filter(p => !p.container || srv.docker?.containers.some(c => c.name === p.container && c.state === 'running'));

// 全体の CPU 使用率を、重みに沿って各プロセスに割り振る
function stepProcs(srv) {
  const total = last(srv.metrics.cpu);
  const live = liveProcs(srv);
  const sumW = live.reduce((a, p) => a + p.w, 0) || 1;
  for (const p of srv.procs) {
    const on = live.includes(p);
    p.cpu = on ? total * p.w / sumW * rand(.75, 1.25) : 0;
    p.rss = on ? p.mem * rand(.985, 1.015) : 0;
  }
  srv.cores = srv.cores.map(() => clamp(total + rand(-15, 15), 0, 100));
}

function createServer(def) {
  const srv = {
    ...def, home: `/home/${def.user}`, windows: [], persist: new Map(), tickers: new Set(),
    cascade: 0, focused: null, selected: null, inited: false, termCount: 1, uploads: [], cores: [0, 0, 0, 0],
    screens: [...(def.screens || [])], tmux: [...(def.tmux || [])],
    services: (def.services || []).map(s => ({ ...s })),
    procs: def.procs.map(p => ({ ...p, cpu: 0, rss: p.mem })),
    docker: def.docker ? cloneDocker(def.docker) : null,
  };
  srv.root = systemTree(srv, def.tree());
  link(srv.root, null);
  srv.homeNode = findNode(srv, srv.home);
  srv.viewRoot = srv.homeNode;
  srv.sudo = false;
  srv.sudoSubs = new Set();
  srv.conn = 'ok';
  srv.trash = [];
  srv.metrics = initMetrics(def.base || { cpu: 15, mem: 40, disk: 30 });
  srv.bootAt = Date.now() - (def.uptimeH ?? rand(30, 600)) * 3600e3;
  stepProcs(srv);
  srv.layer = el('div', { class: 'windows-layer', hidden: true });
  $('#windows').append(srv.layer);
  srv.termRoot = el('div', { class: 'term-root', hidden: true });
  $('#terminal-host').append(srv.termRoot);
  srv.panel = new ShellHost(srv, new TermView(srv.termRoot), { chip: true });
  srv.panel.shell.write(def.router
    ? 'BusyBox v1.36.1 built-in shell (ash)'
    : 'Welcome to Ubuntu 24.04.1 LTS (GNU/Linux 6.8.0-45-generic x86_64)\n\nLast login: Fri Oct 24 09:12:44 2026 from 203.0.113.50', 'dim');
  return srv;
}

function switchServer(id) {
  const srv = state.list.find(s => s.id === id);
  if (!srv) return;
  state.active = srv;
  for (const s of state.list) { s.layer.hidden = s !== srv; s.termRoot.hidden = s !== srv; }
  renderSudo();
  renderConn();
  renderTabs(); renderTree(); renderSessions(); renderTaskbar(); renderStatus(); updateChip();
  requestAnimationFrame(() => initServerWindows(srv));
}

function initServerWindows(srv) {
  if (srv.inited) return;
  srv.inited = true;
  const H = hostRect();
  if (srv.id === 'mc') {
    const half = Math.floor((H.w - 36) / 2), top = Math.max(200, Math.floor(H.h * .58));
    openSessionWindow(srv, getPersistent(srv, 'screen', 'mc'), { x: 12, y: 12, w: half, h: top });
    openLogWindow(srv, findNode(srv, srv.home + '/mc/logs/latest.log'), { x: 24 + half, y: 12, w: half, h: top });
    openMonitor(srv, { x: 12, y: top + 24, w: Math.min(640, H.w - 24), h: Math.max(160, H.h - top - 36) });
    focusWin(srv.windows[0]);
    setTimeout(() => { if (!isMobile()) srv.panel.view.focus(); }, 50);
  } else if (srv.id === 'db') {
    const half = Math.floor((H.w - 36) / 2);
    openServices(srv, { x: 12, y: 12, w: half, h: H.h - 24 });
    openMonitor(srv, { x: 24 + half, y: 12, w: half, h: Math.min(260, H.h - 24) });
  }
}

function genericTree(def) {
  if (def.router) return D(def.user, [F('config.txt', 512, TEXT.router), D('logs', [F('system.log', 2048, TEXT.syslog)], true)], true);
  return D(def.user, [
    D('app', [F('index.js', 820, TEXT.indexjs), F('package.json', 310, TEXT.pkg)], true),
    D('logs', [F('app.log', 5120, TEXT.applog)]),
    F('.bashrc', 3771, TEXT.bashrc),
  ], true);
}

function addServer(def) {
  let id = def.name.replace(/[^\w-]/g, '') || 'srv';
  while (state.list.some(s => s.id === id)) id += '_';
  const srv = createServer({
    id, name: def.name, host: def.host, lan: def.host, user: def.user, router: def.router,
    tree: () => genericTree(def),
    services: def.router ? [] : [
      { name: 'nginx.service', desc: 'Web サーバー', active: true },
      { name: 'docker.service', desc: 'Docker', active: true },
      { name: 'ssh.service', desc: 'OpenSSH サーバー', active: true },
    ],
    docker: def.router ? null : {
      ...GENERIC.docker,
      containers: GENERIC.docker.containers.map(c => ({ ...c, mounts: c.mounts.map(m => ({ ...m, src: m.src.replace(/^~/, `/home/${def.user}`) })) })),
    },
    procs: (def.router ? ROUTER : GENERIC).procs,
    uptimeH: def.router ? 87 * 24 + 2 : undefined,
    net: (def.router ? ROUTER : GENERIC).net,
    mounts: (def.router ? ROUTER : GENERIC).mounts,
    base: def.router ? { cpu: 6, mem: 35, disk: 12 } : { cpu: 9, mem: 30, disk: 22 },
  });
  srv.inited = true;
  state.list.push(srv);
  hideWelcome();
  switchServer(srv.id);
  return srv;
}

function disconnect(srv) {
  const i = state.list.indexOf(srv);
  state.list.splice(i, 1);
  clearInterval(srv.retryTimer);
  srv.windows.slice().forEach(w => closeWindow(w, { keepSession: true }));
  srv.layer.remove();
  srv.termRoot.remove();
  toast(`${srv.name} から切断しました`);
  if (srv === cur()) {
    if (state.list.length) switchServer(state.list[Math.max(0, i - 1)].id);
    else { state.active = null; $('#server-tabs').replaceChildren(); showWelcome(); }
  } else renderTabs();
}

/* ---------- メニュー・トースト・モーダル ---------- */

function showMenu(x, y, items, title) {
  closeMenu();
  // 区切りが先頭・末尾・連続にならないように整える
  const list = items.filter(Boolean).filter((it, i, a) => !it.sep || (i > 0 && i < a.length - 1 && !a[i + 1].sep));
  const m = el('div', { class: 'menu', role: 'menu' },
    title && el('div', { class: 'menu-title' }, title),
    list.map(it => it.sep ? el('div', { class: 'menu-sep' }) : el('button', {
      class: it.danger ? 'danger' : null, disabled: it.disabled, onclick: () => { closeMenu(); it.fn(); },
    }, ic(it.icon), el('span', { class: 'grow' }, it.label), it.hint && el('span', { class: 'menu-hint' }, it.hint))));
  if (isMobile()) {
    state.menuBackdrop = el('div', { class: 'sheet-backdrop' });
    document.body.append(state.menuBackdrop);
  }
  document.body.append(m);
  if (!isMobile()) {
    const r = m.getBoundingClientRect();
    m.style.left = Math.max(8, Math.min(x, innerWidth - r.width - 8)) + 'px';
    m.style.top = Math.max(8, Math.min(y, innerHeight - r.height - 8)) + 'px';
  }
  state.menu = m;
  setTimeout(() => document.addEventListener('pointerdown', onDocDown, true), 0);
}
function onDocDown(e) { if (state.menu && !state.menu.contains(e.target)) closeMenu(); }
function closeMenu() {
  state.menu?.remove(); state.menu = null;
  state.menuBackdrop?.remove(); state.menuBackdrop = null;
  document.removeEventListener('pointerdown', onDocDown, true);
}

function toast(msg, kind = 'info', o = {}) {
  const text = el('span', { class: 'toast-text' }, msg);
  const bar = o.progress ? el('div', { class: 'progress' }, el('i')) : null;
  let timer;
  const close = () => { clearTimeout(timer); t.classList.add('out'); setTimeout(() => t.remove(), 200); };
  const t = el('div', { class: 'toast ' + kind },
    el('div', { class: 'toast-row' }, text, o.action && el('button', { class: 'toast-act', onclick: () => { o.action.fn(); close(); } }, o.action.label)),
    bar);
  $('#toasts').append(t);
  if (o.ms !== 0) timer = setTimeout(close, o.ms || 2800);
  return {
    setProgress: p => { if (bar) bar.firstChild.style.width = (p * 100).toFixed(1) + '%'; },
    done: (m, k = 'ok') => { text.textContent = m; t.className = 'toast ' + k; bar?.remove(); timer = setTimeout(close, 2400); },
    close,
  };
}

function modal({ title, body, width = 520, onClose }) {
  const close = () => {
    if (!ov.isConnected) return;
    ov.remove();
    document.removeEventListener('keydown', esc);
    onClose?.();
  };
  const esc = e => { if (e.key === 'Escape' && !state.menu) close(); };
  const card = el('div', { class: 'modal', style: `width: min(${width}px, 100%)`, role: 'dialog' },
    el('div', { class: 'modal-head' }, el('h2', {}, title), el('button', { class: 'icon-btn', title: '閉じる', onclick: () => close() }, ic('x'))),
    el('div', { class: 'modal-body' }, body));
  const ov = el('div', { class: 'overlay' }, card);
  ov.addEventListener('pointerdown', e => { if (e.target === ov) close(); });
  document.body.append(ov);
  document.addEventListener('keydown', esc);
  card.animate([{ opacity: 0, transform: 'translateY(10px) scale(.98)' }, { opacity: 1, transform: 'none' }], { duration: 180, easing: 'cubic-bezier(.2,.8,.2,1)' });
  return { close, card };
}

function hoverDrop(elm, label) {
  elm.dataset.drop = label;
  elm.classList.add('drop-on');
  clearTimeout(elm._dropT);
  elm._dropT = setTimeout(() => elm.classList.remove('drop-on'), 160);
}

/* ---------- 接続とセットアップ ---------- */

function openConnect(preset) {
  const sel = opts => el('select', { class: 'input' }, opts.map(([v, l]) => el('option', { value: v }, l)));
  const f = {
    name: el('input', { class: 'input' }),
    host: el('input', { class: 'input', spellcheck: 'false' }),
    port: el('input', { class: 'input', value: '22', inputmode: 'numeric' }),
    user: el('input', { class: 'input', spellcheck: 'false' }),
    auth: sel([['pass', 'パスワード'], ['key', '鍵ファイル']]),
    pass: el('input', { class: 'input', type: 'password' }),
  };
  let chosen = null;
  const saved = el('div', { class: 'saved' }, SAVED.map(s => el('button', {
    class: 'saved-item',
    onclick: e => {
      chosen = s;
      f.name.value = s.name; f.host.value = s.host; f.user.value = s.user; f.port.value = s.port;
      $$('.saved-item', saved).forEach(b => b.classList.toggle('active', b === e.currentTarget));
      f.pass.focus();
    },
  }, ic('server'), el('div', { style: 'min-width:0' }, el('div', { class: 'saved-name' }, s.name), el('div', { class: 'saved-host mono' }, `${s.user}@${s.host}`)))));
  const field = (label, input, cls = '') => el('label', { class: 'field ' + cls }, el('span', { class: 'field-label' }, label), input);
  const go = () => {
    const host = f.host.value.trim(), user = f.user.value.trim();
    if (!host) return f.host.focus();
    if (!user) return f.user.focus();
    m.close();
    runSetup({ name: f.name.value.trim() || host, host, user, port: f.port.value.trim() || '22', router: !!(chosen?.router && chosen.host === host) });
  };
  const body = el('div', {},
    el('span', { class: 'field-label' }, '保存済み'), saved,
    el('div', { class: 'form-grid' },
      field('名前', f.name), field('ユーザー', f.user), field('ポート', f.port),
      field('ホスト', f.host, 'span3'),
      field('認証', f.auth), field('パスワード', f.pass, 'span2')),
    el('div', { class: 'modal-foot' }, el('div', { class: 'grow' }), el('button', { class: 'btn', onclick: () => m.close() }, 'キャンセル'), el('button', { class: 'btn primary', onclick: go }, '接続')));
  body.addEventListener('keydown', e => { if (e.key === 'Enter' && e.target.tagName === 'INPUT') go(); });
  const m = modal({ title: 'サーバーに接続', body, width: 600 });
  if (preset) $$('.saved-item', saved)[SAVED.indexOf(preset)]?.click();
  else if (!isMobile()) f.host.focus();
}

/* ---------- サーバーが1台もないとき ---------- */

function showWelcome() {
  $('#app').classList.add('no-server');
  $('#welcome').hidden = false;
  $('#welcome-saved').replaceChildren(...SAVED.map(s => el('button', { class: 'saved-item', onclick: () => openConnect(s) },
    ic('server'), el('div', { style: 'min-width:0' }, el('div', { class: 'saved-name' }, s.name), el('div', { class: 'saved-host mono' }, `${s.user}@${s.host}`)))));
  $('#conn-banner').hidden = true;
  $('#app').classList.remove('conn-lost', 'sudo-on');
  renderStatus();
}

function hideWelcome() {
  $('#app').classList.remove('no-server');
  $('#welcome').hidden = true;
}

/* ---------- 接続が切れたとき ---------- */

const dotClass = srv => srv.conn === 'lost' ? ' lost' : srv.conn === 'reconnecting' ? ' reconn' : '';

function dropConnection(srv) {
  if (srv.conn !== 'ok') return;
  srv.retry = 0;
  nextRetry(srv);
}

function nextRetry(srv) {
  srv.retry++;
  srv.wait = srv.retry === 1 ? 3 : 5;
  srv.conn = 'lost';
  clearInterval(srv.retryTimer);
  srv.retryTimer = setInterval(() => {
    srv.wait--;
    if (srv.wait <= 0) tryReconnect(srv); else renderConn();
  }, 1000);
  renderConn();
}

// モックでは2回目の再接続で戻る
function tryReconnect(srv) {
  clearInterval(srv.retryTimer);
  srv.conn = 'reconnecting';
  renderConn();
  setTimeout(() => {
    if (!state.list.includes(srv)) return;
    if (srv.retry < 2) { nextRetry(srv); return; }
    srv.conn = 'ok';
    restartShells(srv);
    renderConn();
    toast(`${srv.name} に再接続しました`, 'ok');
  }, 1400);
}

// SSH の接続が切れると、その上のシェルも終わる。ターミナルは新しいシェルで開き直し、screen と tmux はつなぎ直す
function restartShells(srv) {
  for (const h of [srv.panel, ...srv.windows.map(w => w.host).filter(Boolean)]) {
    const fg = h.fg;
    if (fg?.detachable) h.release(false);
    else fg?.end();
    h.shell.write('接続が切れたため、前のシェルは終了しました。新しいシェルで開き直しました', 'dim');
  }
  for (const w of srv.windows) {
    const sess = w.session;
    if (!sess || sess.ended) continue;
    if (sess.detachable) sess.note('再接続して、セッションにつなぎ直しました', '↻');
    else sess.end('接続が切れたため、終了しました');
  }
}

function renderConn() {
  const srv = cur();
  const st = srv?.conn || 'ok';
  const b = $('#conn-banner');
  $('#app').classList.toggle('conn-lost', st !== 'ok');
  renderTabs();
  renderStatus();
  if (st === 'ok') { b.hidden = true; return; }
  b.hidden = false;
  b.replaceChildren(
    st === 'reconnecting' ? el('span', { class: 'conn-spin' }) : ic('link', 'conn-ic'),
    el('div', { class: 'grow' },
      el('div', { class: 'conn-title' }, `${srv.name} との接続が切れました`),
      el('div', { class: 'conn-sub' }, st === 'reconnecting' ? '再接続しています' : `${srv.wait} 秒後に再接続します (${srv.retry} 回目)`),
      el('div', { class: 'conn-sub' }, 'ターミナルのシェルは終了しました。screen と tmux のセッションは、再接続したらつなぎ直します')),
    el('button', { class: 'btn small', disabled: st === 'reconnecting', onclick: () => tryReconnect(srv) }, '今すぐ再接続'),
    el('button', { class: 'btn small', onclick: () => disconnect(srv) }, '切断'));
}

async function runSetup(def) {
  let aborted = false;
  const steps = [['ssh', 'SSH で接続'], ['probe', 'サーバーを確認']];
  const items = {};
  const list = el('ol', { class: 'steps' }, steps.map(([k, label]) => {
    const detail = el('div', { class: 'step-detail' });
    const li = el('li', { class: 'step' }, el('span', { class: 'step-ic' }), el('div', { style: 'min-width:0' }, el('div', { class: 'step-label' }, label), detail));
    items[k] = { li, detail };
    return li;
  }));
  const foot = el('div', { class: 'modal-foot' },
    el('div', { class: 'grow' }),
    el('button', { class: 'btn', onclick: () => m.close() }, 'やめる'));
  const m = modal({ title: `${def.name} に接続`, body: el('div', {}, list, foot), width: 540, onClose: () => { aborted = true; } });
  const set = (k, st, text) => { items[k].li.className = 'step ' + st; if (text != null) items[k].detail.textContent = text; };
  const wait = async ms => { await sleep(ms); if (aborted) throw new Error('abort'); };
  try {
    set('ssh', 'run'); await wait(700); set('ssh', 'done', `${def.user}@${def.host}:${def.port}`);
    set('probe', 'run'); await wait(600);
    set('probe', 'done', def.router ? 'Linux · mipsel · BusyBox · systemd と Docker はない' : 'Linux · x86_64 · Ubuntu 24.04 · systemd · Docker · screen · tmux');
    addServer(def);
    foot.replaceChildren(el('div', { class: 'grow' }), el('button', { class: 'btn primary', onclick: () => m.close() }, '開く'));
  } catch (e) {
    if (e.message !== 'abort') throw e;
  }
}

/* ---------- スマホの登録・設定 ---------- */

function drawQR(canvas, seed) {
  const N = 29, s = Math.floor(canvas.width / (N + 8)), off = Math.floor((canvas.width - s * N) / 2);
  const ctx = canvas.getContext('2d');
  ctx.fillStyle = '#fff';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  const grid = Array.from({ length: N }, () => Array(N).fill(null));
  for (const [fx, fy] of [[0, 0], [N - 7, 0], [0, N - 7]]) {
    for (let y = -1; y <= 7; y++) for (let x = -1; x <= 7; x++) {
      const gx = fx + x, gy = fy + y;
      if (gx < 0 || gy < 0 || gx >= N || gy >= N) continue;
      const ring = x === 0 || x === 6 || y === 0 || y === 6, core = x >= 2 && x <= 4 && y >= 2 && y <= 4;
      grid[gy][gx] = x >= 0 && x <= 6 && y >= 0 && y <= 6 && (ring || core);
    }
  }
  for (let i = 8; i < N - 8; i++) { grid[6][i] = i % 2 === 0; grid[i][6] = i % 2 === 0; }
  const r = mulberry32(hashStr(seed));
  ctx.fillStyle = '#0c0f13';
  for (let y = 0; y < N; y++) for (let x = 0; x < N; x++) {
    const v = grid[y][x] ?? r() < .5;
    if (v) ctx.fillRect(off + x * s, off + y * s, s, s);
  }
}

function openPair() {
  const routes = [state.settings.phoneLan && ['lan', 'LAN 内'], state.settings.phonePublic && ['public', '外から']].filter(Boolean);
  if (!routes.length) { toast('スマホの待ち受けが OFF です。設定の「スマホ」で ON にしてください', 'warn'); return; }
  let route = routes[routes.length - 1][0];
  const urlOf = r => r === 'lan' ? PC.lanUrl : publicUrl();
  const canvas = el('canvas', { class: 'qr', width: 222, height: 222 });
  const urlEl = el('div', { class: 'mono small' });
  const timer = el('div', { class: 'muted small' });
  let left = 300;
  const regen = () => { urlEl.textContent = urlOf(route); drawQR(canvas, urlOf(route) + Date.now()); left = 300; canvas.classList.remove('expired'); showLeft(); };
  const showLeft = () => {
    if (left > 0) { timer.textContent = `${Math.floor(left / 60)}:${pad2(left % 60)} で無効になります`; return; }
    canvas.classList.add('expired');
    timer.replaceChildren('期限が切れました ', el('button', { class: 'link-btn', onclick: regen }, '作り直す'));
  };
  const iv = setInterval(() => { if (left > 0) { left--; showLeft(); } }, 1000);
  const seg = el('div', { class: 'seg' });
  const drawSeg = () => seg.replaceChildren(...routes.map(([v, l]) => el('button', { class: v === route ? 'active' : '', onclick: () => { route = v; drawSeg(); regen(); } }, l)));
  drawSeg();
  regen();
  const routeName = r => r === 'lan' ? 'LAN 内' : '外から';
  const devs = el('div', { class: 'dev-list' });
  const drawDevs = () => devs.replaceChildren(...(state.devices.length ? state.devices.map(d => el('div', { class: 'dev' },
    ic('phone', 'dev-ic'),
    el('div', { class: 'dev-text' }, el('div', { class: 'dev-name' }, d.name), el('div', { class: 'muted small' }, `最終ログイン ${d.last} · ${d.ip} · ${routeName(d.route)}`)),
    el('button', { class: 'btn small', onclick: () => { state.devices = state.devices.filter(x => x !== d); drawDevs(); renderStatus(); } }, '無効にする'),
  )) : [el('div', { class: 'muted small' }, 'まだありません')]));
  drawDevs();
  const fake = el('button', {
    class: 'link-btn',
    onclick: () => {
      state.devices.unshift({ name: 'Android (Chrome)', last: 'たった今', ip: route === 'lan' ? '192.168.1.42' : '198.51.100.88', route });
      drawDevs(); renderStatus(); regen();
      toast('スマホを登録しました', 'ok');
    },
  }, '読み取ったことにする');
  const hist = el('details', { class: 'history' }, el('summary', {}, 'ログインの履歴'),
    el('div', { class: 'hist-rows' }, HIST.map(h => el('div', { class: 'hist-row' }, el('span', { class: 'mono' }, h[0]), el('span', {}, h[1]), el('span', { class: 'mono muted' }, h[2]), el('span', { class: 'muted' }, h[3])))));
  const body = el('div', { class: 'pair' },
    el('div', { class: 'pair-qr' }, routes.length > 1 && seg, canvas, urlEl, timer, fake),
    el('div', {}, el('span', { class: 'field-label' }, '登録済みの端末'), devs, hist));
  modal({ title: 'スマホを登録', body, width: 640, onClose: () => clearInterval(iv) });
}

/* ---------- 設定 ---------- */

function loadSettings() {
  let saved = {};
  try { saved = JSON.parse(localStorage.getItem('drag-mock-settings') || '{}'); } catch { /* 読めなければ初期設定で動かす */ }
  const d = structuredClone(DEFAULT_SETTINGS);
  return { ...d, ...saved, sb: { ...d.sb, ...(saved.sb || {}) } };
}
function saveSettings() {
  try { localStorage.setItem('drag-mock-settings', JSON.stringify(state.settings)); } catch { /* 保存できなくても画面は動かす */ }
}
function applySettings() {
  const s = state.settings, r = document.documentElement;
  r.dataset.theme = s.theme;
  r.dataset.term = s.term;
  r.dataset.desk = s.desk;
  r.style.setProperty('--accent', s.accent);
  r.style.setProperty('--term-size', `${s.termSize}px`);
  renderStatus();
}
function setSetting(key, value) {
  state.settings[key] = value;
  saveSettings();
  applySettings();
}

function openSettings() {
  const S = () => state.settings;
  const seg = (get, options, pick) => {
    const box = el('div', { class: 'seg' });
    const draw = () => box.replaceChildren(...options.map(([v, l]) => el('button', { class: v === get() ? 'active' : '', onclick: () => { pick(v); draw(); } }, l)));
    draw();
    return box;
  };
  const toggle = (key) => {
    const i = el('input', { type: 'checkbox' });
    i.checked = S()[key];
    i.addEventListener('change', () => setSetting(key, i.checked));
    return el('label', { class: 'switch' }, i, el('span'));
  };
  const row = (label, desc, ctl) => el('div', { class: 'set-row' }, el('div', {}, el('div', {}, label), desc && el('div', { class: 'set-desc' }, desc)), ctl);

  const accent = () => {
    const custom = el('input', { type: 'color', class: 'swatch-custom', title: '好きな色を選ぶ' });
    const dots = ACCENTS.map(([c, name]) => el('button', { class: 'swatch', title: name, style: `background:${c}`, onclick: () => { setSetting('accent', c); mark(); } }));
    const mark = () => {
      custom.value = S().accent;
      dots.forEach((b, i) => b.classList.toggle('active', ACCENTS[i][0] === S().accent.toLowerCase()));
    };
    custom.addEventListener('input', () => { setSetting('accent', custom.value); mark(); });
    mark();
    return el('div', { class: 'swatches' }, dots, custom);
  };
  const sbItems = () => el('div', { class: 'checks' }, [['uptime', '稼働日数'], ['cpu', 'CPU'], ['mem', 'メモリ'], ['net', 'ネットワーク']].map(([k, l]) => {
    const i = el('input', { type: 'checkbox' });
    i.checked = S().sb[k];
    i.addEventListener('change', () => { S().sb[k] = i.checked; saveSettings(); renderStatus(); });
    return el('label', { class: 'toggle' }, i, l);
  }));
  const termSize = () => {
    const r = el('input', { type: 'range', min: 11, max: 18, step: 1 });
    r.value = S().termSize;
    const v = el('span', { class: 'num' }, `${S().termSize}px`);
    r.addEventListener('input', () => { setSetting('termSize', +r.value); v.textContent = `${r.value}px`; });
    return el('div', { class: 'range' }, r, v);
  };
  const autoCmds = () => {
    const chips = el('div', { class: 'chips' });
    const draw = () => chips.replaceChildren(...S().autoCmds.map(c => el('span', { class: 'chip' }, c,
      el('button', { title: '外す', onclick: () => { setSetting('autoCmds', S().autoCmds.filter(x => x !== c)); draw(); } }, ic('x')))));
    draw();
    const add = el('input', { class: 'input', placeholder: 'コマンドを追加して Enter', spellcheck: 'false' });
    add.addEventListener('keydown', e => {
      const v = add.value.trim().replace(/\s+/g, ' ');
      if (e.key === 'Enter' && v && !S().autoCmds.includes(v)) { setSetting('autoCmds', [...S().autoCmds, v]); add.value = ''; draw(); }
    });
    return el('div', { class: 'set-sub' }, chips, add);
  };
  const port = () => {
    const i = el('input', { class: 'input set-select', value: S().publicPort, inputmode: 'numeric' });
    i.addEventListener('change', () => { const n = parseInt(i.value, 10); if (n > 0 && n < 65536) setSetting('publicPort', n); else i.value = S().publicPort; });
    return i;
  };
  const conflict = () => {
    const sel = el('select', { class: 'input set-select' }, [['ask', '毎回聞く'], ['overwrite', '上書き'], ['keep', '両方残す'], ['skip', 'スキップ']].map(([v, l]) => el('option', { value: v }, l)));
    sel.value = S().uploadConflict;
    sel.addEventListener('change', () => setSetting('uploadConflict', sel.value));
    return sel;
  };

  const pages = {
    look: ['外観', 'window', () => [
      row('言語', S().lang === 'en' ? 'モックの文言は日本語のまま' : null, seg(() => S().lang, [['ja', '日本語'], ['en', 'English']], v => { setSetting('lang', v); show('look'); })),
      row('テーマ', null, seg(() => S().theme, [['dark', 'ダーク'], ['light', 'ライト']], v => setSetting('theme', v))),
      row('アクセントの色', 'ボタン、選んでいる項目、グラフの色', accent()),
      row('デスクトップの背景', null, seg(() => S().desk, [['gradient', 'グラデーション'], ['dots', 'ドット'], ['plain', '無地']], v => setSetting('desk', v))),
      row('下の欄に出す項目', null, sbItems()),
    ]],
    term: ['ターミナル', 'terminal', () => [
      row('配色', null, seg(() => S().term, [['dark', 'ダーク'], ['light', 'ライト'], ['solarized', 'Solarized']], v => setSetting('term', v))),
      row('文字の大きさ', null, termSize()),
      el('div', { class: 'term-preview' },
        lineEl([{ t: 'asuta@mc-server:~$ ', c: 'prompt' }, { t: 'ls', c: 'cmd' }]),
        lineEl([{ t: 'backups', c: 'dir' }, { t: '  mc', c: 'dir' }, { t: '  notes.txt  ' }, { t: 'mc-2026-10-24.tar.gz', c: 'hl' }]),
        lineEl([{ t: '[09:41:07 WARN]: ', c: 'dim' }, { t: "Can't keep up! Is the server overloaded?", c: 'warn' }]),
        lineEl([{ t: 'foo: command not found', c: 'err' }])),
    ]],
    ops: ['操作', 'gear', () => [
      row('セッションを自動でウィンドウに移す', '下のコマンドを実行したとき', toggle('autoOn')),
      autoCmds(),
      row('ウィンドウを画面の端へドラッグして並べる', '左右の端で半分、上の端で最大化', toggle('snap')),
      row('ログのウィンドウを開いたら末尾を追いかける', null, toggle('followLog')),
      row('同じ名前のファイルをアップロードしたとき', null, conflict()),
    ]],
    phone: ['スマホ', 'phone', () => [
      row('LAN 内で待ち受ける', `${PC.lanUrl} · 自己署名の証明書`, toggle('phoneLan')),
      row('インターネットに公開する', 'ルーターで、下のポートをこの PC へ転送しておく', toggle('phonePublic')),
      row('公開するポート', 'IP アドレスの証明書は 80 番か 443 番で取る', port()),
      row('証明書', S().cert === 'ip' ? `Let's Encrypt · ${PC.ip} · 10/30 まで。数日おきに自動で更新する` : "Let's Encrypt · DNS-01 で取る",
        seg(() => S().cert, [['ip', 'IP アドレス'], ['domain', '自分のドメイン']], v => { setSetting('cert', v); show('phone'); })),
      ...(S().cert === 'domain' ? [el('div', { class: 'set-sub' },
        el('input', { class: 'input', value: PC.domain, spellcheck: 'false' }),
        el('input', { class: 'input', type: 'password', placeholder: 'DNS サービスの API キー' }))] : []),
    ]],
  };
  const nav = el('div', { class: 'set-nav' });
  const pane = el('div', { class: 'set-pane' });
  let page = 'look';
  const show = k => {
    page = k;
    $$('button[data-page]', nav).forEach(b => b.classList.toggle('active', b.dataset.page === k));
    pane.replaceChildren(el('h3', {}, pages[k][0]), ...pages[k][2]());
  };
  nav.append(...Object.entries(pages).map(([k, [label, icon]]) => el('button', { dataset: { page: k }, onclick: () => show(k) }, ic(icon), label)),
    el('div', { class: 'grow' }),
    el('button', { class: 'btn small', onclick: () => { state.settings = structuredClone(DEFAULT_SETTINGS); saveSettings(); applySettings(); show(page); } }, '初期設定に戻す'));
  show('look');
  modal({ title: '設定', body: el('div', { class: 'settings' }, nav, pane), width: 780 }).card.classList.add('settings-modal');
}

/* ---------- スマホの画面 ---------- */

function setMTab(t) {
  $('#app').dataset.mtab = t;
  $$('#mobile-nav button').forEach(b => b.classList.toggle('active', b.dataset.mtab === t));
  if (t === 'files') renderTree();
}

function buildExtraKeys() {
  const keys = [['Esc', 'Escape'], ['Tab', 'Tab'], ['Ctrl', 'Ctrl'], ['↑', 'ArrowUp'], ['↓', 'ArrowDown'], ['←', 'ArrowLeft'], ['→', 'ArrowRight'], ['|', '|'], ['/', '/'], ['~', '~'], ['-', '-']];
  for (const [label, key] of keys) {
    const b = el('button', { class: 'xkey', dataset: { key } }, label);
    b.addEventListener('pointerdown', e => e.preventDefault()); // 入力欄のフォーカスを外さない
    b.addEventListener('click', () => pressKey(key));
    $('#extra-keys').append(b);
  }
}

function pressKey(key) {
  const v = cur().panel.view, inp = v.input;
  if (key === 'Ctrl') return setCtrl(!state.ctrlArmed);
  if (key === 'ArrowLeft' || key === 'ArrowRight') {
    const p = clamp((inp.selectionStart ?? inp.value.length) + (key === 'ArrowLeft' ? -1 : 1), 0, inp.value.length);
    v.focus(); inp.setSelectionRange(p, p);
    return;
  }
  if (key.length === 1) {
    if (state.ctrlArmed) { setCtrl(false); v.onKey({ key, ctrlKey: true, preventDefault() {} }); }
    else v.insert(key, true);
    return;
  }
  v.onKey({ key, ctrlKey: false, preventDefault() {} });
  v.focus();
}

function setCtrl(on) {
  state.ctrlArmed = on;
  $('.xkey[data-key="Ctrl"]')?.classList.toggle('on', on);
}

/* ---------- 画面全体のつなぎ ---------- */

function wireResizer(handle, prop, axis, min, maxFn, invert) {
  handle.addEventListener('pointerdown', e => {
    e.preventDefault();
    handle.setPointerCapture(e.pointerId);
    handle.classList.add('active');
    const start = axis === 'x' ? e.clientX : e.clientY;
    const startVal = parseFloat(getComputedStyle(document.documentElement).getPropertyValue(prop));
    const move = ev => {
      const d = (axis === 'x' ? ev.clientX : ev.clientY) - start;
      document.documentElement.style.setProperty(prop, clamp(startVal + (invert ? -d : d), min, maxFn()) + 'px');
    };
    const up = () => { handle.removeEventListener('pointermove', move); handle.removeEventListener('pointerup', up); handle.classList.remove('active'); };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', up);
  });
}

function wireDesktopDrop() {
  const host = $('#windows');
  host.addEventListener('dragover', e => {
    const d = state.drag;
    if (!d && !e.dataTransfer.types.includes('application/x-drag-fg')) return;
    if (d?.kind === 'path') {
      if (d.node.dir || fileKind(d.node) === 'bin') return;
      e.preventDefault(); hoverDrop(host, 'ここで開く');
    } else if (d?.kind === 'session') {
      e.preventDefault(); hoverDrop(host, 'ここで開く');
    } else if (d?.kind === 'container') {
      e.preventDefault(); hoverDrop(host, 'シェルを開く');
    } else {
      e.preventDefault(); hoverDrop(host, 'ここに移す');
    }
  });
  host.addEventListener('drop', e => {
    e.preventDefault();
    host.classList.remove('drop-on');
    const srv = cur(), r = host.getBoundingClientRect();
    const pos = { x: e.clientX - r.left - 40, y: e.clientY - r.top - 16 };
    const d = state.drag;
    state.drag = null;
    if (e.dataTransfer.types.includes('application/x-drag-fg')) return srv.panel.popOut(pos);
    if (d?.kind === 'session') { const [kind, name] = d.key.split(':'); return openPersistent(srv, kind, name, pos); }
    if (d?.kind === 'container') return openSessionWindow(srv, sessDocker(d.name, 'exec', `docker exec -it ${d.name} sh`), pos);
    if (d?.kind === 'path') openFile(srv, d.node, pos);
  });
}

function wireTerminalDrop() {
  const tp = $('#terminal-panel');
  tp.addEventListener('dragover', e => {
    const d = state.drag;
    if (d?.kind === 'path') { e.preventDefault(); hoverDrop(tp, d.node.dir ? 'このフォルダへ cd' : 'パスを入力'); }
    else if (d?.kind === 'session') { e.preventDefault(); hoverDrop(tp, 'ターミナルで開く'); }
  });
  tp.addEventListener('drop', e => {
    e.preventDefault();
    tp.classList.remove('drop-on');
    const d = state.drag, srv = cur();
    state.drag = null;
    if (d?.kind === 'path') { if (d.node.dir) cdTo(srv, d.node.path); else insertPath(srv, d.node.path); }
    else if (d?.kind === 'session') attachToTerminal(srv, d.key);
  });
}

// ファイルの行に落としたらそのフォルダ、フォルダの行ならそのフォルダ、それ以外はホーム
function dropDir(srv, e) {
  const row = e.target.closest('.tree-row');
  const n = row?.dataset.path && findNode(srv, row.dataset.path);
  if (!n) return srv.viewRoot;
  return n.dir ? n : n.parent;
}

const OS_DROP_HINT = 'ここへドロップするとアップロード';
function showTreeDrop(text) {
  const lab = $('#tree-drop');
  lab.textContent = text;
  lab.hidden = false;
  clearTimeout(lab._t);
  lab._t = setTimeout(() => {
    $$('#tree .drop-on').forEach(x => x.classList.remove('drop-on'));
    if ($('#app').classList.contains('os-drag')) lab.textContent = OS_DROP_HINT; else lab.hidden = true;
  }, 160);
}

function wireTreeDrop() {
  const tree = $('#tree');
  tree.addEventListener('dragover', e => {
    const srv = cur(), files = e.dataTransfer.types.includes('Files'), d = state.drag;
    if (!files && d?.kind !== 'path') return;
    const dir = dropDir(srv, e);
    if (!files && (dir === d.node || dir === d.node.parent || dir.path.startsWith(d.node.path + '/'))) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = files ? 'copy' : 'move';
    const target = dir === srv.viewRoot ? $('.tree-root') : $(`.tree-row[data-path="${CSS.escape(dir.path)}"]`);
    $$('#tree .drop-on').forEach(x => { if (x !== target) x.classList.remove('drop-on'); });
    target?.classList.add('drop-on');
    showTreeDrop(`${tilde(srv, dir.path)}/ ${files ? 'にアップロード' : 'へ移動'}`);
  });
  tree.addEventListener('drop', e => {
    e.preventDefault();
    const srv = cur(), dir = dropDir(srv, e);
    $$('#tree .drop-on').forEach(x => x.classList.remove('drop-on'));
    $('#tree-drop').hidden = true;
    if (e.dataTransfer.types.includes('Files')) {
      const specs = dropSpecs(e.dataTransfer);
      guard(srv, [dir], `${tilde(srv, dir.path)} にアップロードする`, () => specs.then(s => upload(srv, dir, s)));
    } else if (state.drag?.kind === 'path') {
      const n = state.drag.node;
      guard(srv, [n, n.parent, dir], `${n.name} を移動する`, () => moveNode(srv, n, dir));
    }
  });
}

// エクスプローラーと同じキーで操作する
function wireTreeKeys() {
  const tree = $('#tree');
  tree.addEventListener('contextmenu', e => {
    e.preventDefault();
    const srv = cur();
    srv.selected = srv.viewRoot.path;
    renderTree();
    nodeMenu(srv, srv.viewRoot, e.clientX, e.clientY);
  });
  tree.addEventListener('keydown', e => {
    if (e.target.closest('input')) return;
    const srv = cur(), n = srv.selected && findNode(srv, srv.selected);
    const key = e.key.length === 1 ? e.key.toLowerCase() : e.key, mod = e.ctrlKey || e.metaKey;
    const act = fn => { e.preventDefault(); fn(); };
    if (mod && key === 'v') return act(() => paste(srv, n ? (n.dir ? n : n.parent) : srv.viewRoot));
    if (!n) return;
    if (mod && key === 'c') return act(() => setClip(srv, n, 'copy'));
    if (mod && key === 'x' && !isProtected(srv, n)) return act(() => setClip(srv, n, 'cut'));
    if (key === 'Delete') return act(() => deleteNode(srv, n));
    if (key === 'F2') return act(() => renameNode(srv, n));
    if (key === 'Enter') return act(() => { if (n.dir) { n.open = !n.open; renderTree(); } else openFile(srv, n); });
    if (key === 'ArrowUp' || key === 'ArrowDown') {
      return act(() => {
        const rows = $$('#tree .tree-row[data-path]').map(r => r.dataset.path);
        const i = rows.indexOf(n.path);
        const next = rows[clamp(i + (key === 'ArrowUp' ? -1 : 1), 0, rows.length - 1)];
        if (next) { srv.selected = next; renderTree(); $(`.tree-row[data-path="${CSS.escape(next)}"]`)?.scrollIntoView({ block: 'nearest' }); }
      });
    }
    if (key === 'ArrowRight' && n.dir && !n.open) return act(() => { n.open = true; renderTree(); });
    if (key === 'ArrowLeft') {
      return act(() => {
        if (n.dir && n.open) n.open = false;
        else if (n.parent && isUnder(n.parent, srv.viewRoot) && n.parent !== srv.viewRoot) srv.selected = n.parent.path;
        renderTree();
      });
    }
  });
}

// PC のファイルを画面に持ってきたら、ファイルツリーを出して落とし先を示す。
// ツリーの外に落としてもブラウザがファイルを開かないようにする
function wireOsDrag() {
  let depth = 0;
  const isFiles = e => e.dataTransfer?.types.includes('Files');
  const end = () => { depth = 0; $('#app').classList.remove('os-drag'); $('#tree-drop').hidden = true; };
  window.addEventListener('dragenter', e => {
    if (!isFiles(e) || depth++ > 0) return;
    $('#app').classList.add('os-drag');
    if (!isMobile()) showFilesView();
    $('#tree-drop').textContent = OS_DROP_HINT;
    $('#tree-drop').hidden = false;
  });
  window.addEventListener('dragleave', e => { if (isFiles(e) && --depth <= 0) end(); });
  window.addEventListener('dragover', e => {
    if (isFiles(e) && !e.defaultPrevented) { e.preventDefault(); e.dataTransfer.dropEffect = 'none'; }
  });
  window.addEventListener('drop', e => {
    if (isFiles(e) && !e.defaultPrevented) e.preventDefault();
    end();
  });
}

function wireChip() {
  const chip = $('#fg-chip');
  chip.addEventListener('dragstart', e => {
    e.dataTransfer.setData('application/x-drag-fg', '1');
    e.dataTransfer.effectAllowed = 'move';
    state.drag = { kind: 'fg' };
  });
  chip.addEventListener('dragend', () => { state.drag = null; });
  chip.addEventListener('click', () => { cur().panel.popOut(); if (isMobile()) setMTab('desktop'); });
}

function tick() {
  for (const s of state.list) {
    stepMetrics(s.metrics);
    stepProcs(s);
    s.tickers.forEach(f => f());
  }
  updateStatus();
}

function scheduleMcLog() {
  setTimeout(() => {
    const mc = state.list.find(s => s.id === 'mc');
    if (mc) {
      const [lv, msg] = pick(MC_MSGS);
      const line = `[${clock()} ${lv}]: ${msg}`;
      emitLog(mc, mc.home + '/mc/logs/latest.log', line);
      // サーバーの標準出力は journal にも出る
      const j = mc.services.find(s => s.name === 'mc-server.service')?.logNode;
      if (j) emitLog(mc, j.path, `${journalStamp()} mc-server java[1342]: ${line}`, j);
    }
    scheduleMcLog();
  }, rand(2500, 5000));
}

function boot() {
  applySettings();
  $$('[data-ic]').forEach(e => e.prepend(ic(e.dataset.ic)));
  $('#launcher').append(
    el('button', { class: 'launch-btn', title: '新しいターミナル', onclick: () => openTerminalWindow(cur()) }, ic('terminal')),
    el('button', { class: 'launch-btn', title: 'リソース監視', onclick: () => openMonitor(cur()) }, ic('chart')),
    el('button', { class: 'launch-btn', title: 'サービス', onclick: () => openServices(cur()) }, ic('server')),
    el('button', { class: 'launch-btn', title: 'Docker', onclick: () => openDocker(cur()) }, ic('box')));
  buildExtraKeys();

  $$('[data-view]').forEach(b => b.addEventListener('click', () => setView(b.dataset.view)));
  $$('#mobile-nav button').forEach(b => b.addEventListener('click', () => setMTab(b.dataset.mtab)));
  $('#add-server').addEventListener('click', openConnect);
  $('#btn-settings').addEventListener('click', openSettings);
  $('#sudo-btn').addEventListener('click', () => { const srv = cur(); if (srv.sudo) setSudo(srv, false); else enableSudo(srv, null); });
  $('#btn-trash').addEventListener('click', () => openTrash(cur()));
  const createHere = isDir => {
    const srv = cur(), n = srv.selected && findNode(srv, srv.selected);
    createIn(srv, n ? (n.dir ? n : n.parent) : srv.viewRoot, isDir);
  };
  $('#btn-newfile').addEventListener('click', () => createHere(false));
  $('#btn-newfolder').addEventListener('click', () => createHere(true));
  $('#m-settings').addEventListener('click', openSettings);
  $('#pair-btn').addEventListener('click', () => openPair());
  $('#btn-tile').addEventListener('click', tileWindows);
  $('#btn-clear').addEventListener('click', () => { const p = cur().panel; (p.fg || p.shell).clear(); });
  $('#btn-upload').addEventListener('click', () => {
    const srv = cur(), n = srv.selected && findNode(srv, srv.selected);
    pickUpload(n ? (n.dir ? n : n.parent) : srv.viewRoot);
  });
  $('#btn-collapse').addEventListener('click', () => {
    const close = n => { if (n.dir) { n.open = false; n.children.forEach(close); } };
    cur().viewRoot.children.forEach(close);
    renderTree();
  });
  $('#upload-input').addEventListener('change', e => {
    if (e.target.files.length) upload(cur(), state.uploadTarget || cur().viewRoot, [...e.target.files].map(f => ({ name: f.name, file: f })));
    e.target.value = '';
  });
  $('#help-btn').addEventListener('click', () => { $('#help').hidden = !$('#help').hidden; });

  wireResizer($('#sidebar-resizer'), '--sidebar-w', 'x', 170, () => innerWidth * .4, false);
  wireResizer($('#panel-resizer'), '--terminal-h', 'y', 110, () => innerHeight * .7, true);
  wireDesktopDrop();
  wireTerminalDrop();
  wireTreeDrop();
  wireTreeKeys();
  wireOsDrag();
  wireChip();
  document.addEventListener('keydown', e => { if (e.key === 'Escape') closeMenu(); });
  matchMedia('(max-width: 720px)').addEventListener('change', () => { renderTree(); renderSessions(); updateChip(); });

  $('#welcome-connect').addEventListener('click', () => openConnect());
  $('#demo-drop').addEventListener('click', () => { const srv = cur(); if (srv) dropConnection(srv); });
  if (new URLSearchParams(location.search).has('empty')) showWelcome();
  else {
    state.list = DEFS.map(createServer);
    switchServer('mc');
  }
  setMTab('desktop');
  setInterval(tick, 1000);
  scheduleMcLog();
  scheduleDockerLog();
}

function scheduleDockerLog() {
  setTimeout(() => {
    for (const srv of state.list) {
      const cs = srv.docker?.containers.filter(c => c.state === 'running' && !c.image.startsWith('alpine')) || [];
      if (!cs.length || Math.random() < .4) continue;
      const c = pick(cs);
      emitLog(srv, c.logNode.path, dockerLine(c), c.logNode);
    }
    scheduleDockerLog();
  }, rand(1500, 3500));
}

boot();
