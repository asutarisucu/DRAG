'use strict';
// スマホで QR を読んだあとに開くページのモック。?state= で表示を切り替える

const ICONS = {
  phone: '<rect x="7" y="2" width="10" height="20" rx="2.5"/><path d="M11 18h2"/>',
  check: '<path d="m5 12 5 5 9-10"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  lock: '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/>',
  pc: '<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/>',
};

function el(tag, props, ...kids) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(props || {})) {
    if (v == null || v === false) continue;
    if (k === 'class') e.className = v;
    else if (k.startsWith('on')) e.addEventListener(k.slice(2), v);
    else e.setAttribute(k, v);
  }
  for (const kid of kids.flat()) if (kid != null && kid !== false) e.append(kid);
  return e;
}
const ic = name => {
  const s = el('span', { class: 'ic' });
  s.innerHTML = `<svg viewBox="0 0 24 24" aria-hidden="true">${ICONS[name]}</svg>`;
  return s;
};

// 本体のモックで選んだテーマと色を使う
try {
  const s = JSON.parse(localStorage.getItem('drag-mock-settings') || '{}');
  if (s.theme) document.documentElement.dataset.theme = s.theme;
  if (s.accent) document.documentElement.style.setProperty('--accent', s.accent);
} catch { /* 読めなければ初期の見た目 */ }

const pc = { name: 'DESKTOP-01', url: 'https://203.0.113.50' };
const card = document.getElementById('card');
const brand = () => [
  el('img', { class: 'pair-brand logo-dark', src: 'img/logo-dark.png', alt: 'DRAG Runs As GUI' }),
  el('img', { class: 'pair-brand logo-light', src: 'img/logo-light.png', alt: 'DRAG Runs As GUI' }),
];

function guessDevice() {
  const ua = navigator.userAgent;
  const os = /iPhone/.test(ua) ? 'iPhone' : /iPad/.test(ua) ? 'iPad' : /Android/.test(ua) ? 'Android' : 'この端末';
  const br = /Edg\//.test(ua) ? 'Edge' : /CriOS|Chrome/.test(ua) ? 'Chrome' : /Safari/.test(ua) ? 'Safari' : 'ブラウザー';
  return `${os} (${br})`;
}

const views = {
  pair() {
    const name = el('input', { class: 'input', value: guessDevice(), spellcheck: 'false' });
    let left = 252;
    const timer = el('div', { class: 'pair-foot' });
    const tick = () => { timer.textContent = `この QR コードは ${Math.floor(left / 60)}:${String(left % 60).padStart(2, '0')} で無効になります`; };
    tick();
    const iv = setInterval(() => { left--; if (left <= 0) { clearInterval(iv); show('expired'); } else tick(); }, 1000);
    return [
      ...brand(),
      el('div', { class: 'pair-title' }, 'この端末を登録します'),
      el('div', { class: 'pair-server' }, ic('pc'), el('div', {}, el('div', {}, pc.name), el('div', { class: 'mono' }, pc.url))),
      el('label', { class: 'field' }, el('span', { class: 'field-label' }, '端末の名前'), name),
      el('button', { class: 'btn primary', onclick: () => { clearInterval(iv); show('done', name.value.trim() || guessDevice()); } }, '登録する'),
      timer,
    ];
  },
  done(device) {
    return [
      el('div', { class: 'pair-mark ok' }, ic('check')),
      el('div', { class: 'pair-title' }, '登録しました'),
      el('div', { class: 'pair-text' }, `${device} を登録しました。`, el('br'), '次からはこの端末でページを開くだけで使えます。', el('br'), 'PC で DRAG が動いている間だけ開けます。'),
      el('a', { class: 'btn primary', href: 'index.html' }, 'DRAG を開く'),
    ];
  },
  expired() {
    return [
      el('div', { class: 'pair-mark warn' }, ic('clock')),
      el('div', { class: 'pair-title' }, 'この QR コードは使えません'),
      el('div', { class: 'pair-text' }, '期限が切れたか、もう使われています。', el('br'), 'PC アプリの「スマホを登録」で作り直してください。'),
    ];
  },
  unregistered() {
    return [
      ...brand(),
      el('div', { class: 'pair-mark info' }, ic('lock')),
      el('div', { class: 'pair-title' }, 'この端末は登録されていません'),
      el('div', { class: 'pair-text' }, 'PC アプリの「スマホを登録」に出る QR コードを、この端末で読み取ってください。'),
    ];
  },
};

function show(state, arg) {
  card.replaceChildren(...(views[state] || views.pair)(arg));
}

show(new URLSearchParams(location.search).get('state') || 'pair');
