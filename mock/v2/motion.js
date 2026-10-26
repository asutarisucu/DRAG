// v2 のデザインの細かい動き。app.js は描き直すたびに要素を作り直すので、
// 描き直しを MutationObserver で見て、前の描画と比べて変わったところだけを動かす
(() => {
  const reduce = matchMedia('(prefers-reduced-motion: reduce)');
  const isMobile = () => matchMedia('(max-width: 720px)').matches;
  const still = () => reduce.matches;
  const EASE = 'cubic-bezier(.2,.8,.2,1)';
  const SPRING = 'cubic-bezier(.3,1.3,.5,1)';
  const $ = s => document.querySelector(s);
  const watch = (target, opts, fn) => new MutationObserver(fn).observe(target, opts);
  const activeServer = () => $('.server-tab.active')?.title ?? '';

  // 選んでいる項目の下地 (::before など) を、前に選んでいた項目の位置から滑らせる。
  // サーバーを切り替えたときは、別の画面なので滑らせない
  const slider = (box, sel, key, move, scope = () => '') => {
    let last = null;
    return () => {
      const cur = box.querySelector(sel);
      if (!cur) { last = null; return; }
      const r = cur.getBoundingClientRect();
      const k = key(cur), s = scope();
      if (last && last.s === s && last.k !== k && r.width && !still()) move(cur, last.r, r);
      last = { k, s, r };
    };
  };
  const slideX = (el, from, to) => el.animate(
    [{ transform: `translateX(${from.left - to.left}px) scaleX(${from.width / to.width})` }, { transform: 'none' }],
    { duration: 420, easing: SPRING, pseudoElement: '::before' });
  const slideMark = (el, from, to) => {
    const dx = from.left + from.width / 2 - (to.left + to.width / 2);
    el.animate([{ transform: `translateX(-50%) translateX(${dx}px)` }, { transform: 'translateX(-50%)' }], { duration: 420, easing: SPRING, pseudoElement: '::after' });
  };

  // 上のサーバーのタブ
  const tabs = $('#server-tabs');
  watch(tabs, { childList: true }, slider(tabs, '.server-tab.active', e => e.title, slideX));

  // 左の欄のボタン。ボタンは作り直さず class だけが変わる
  const act = $('#activitybar');
  const actSlide = slider(act, '.act-btn.active', e => e.dataset.view || e.id, (el, from, to) =>
    el.animate([{ transform: `translateY(${from.top - to.top}px)`, opacity: 1 }, { transform: 'none', opacity: 1 }], { duration: 420, easing: SPRING, pseudoElement: '::before' }));
  watch(act, { attributes: true, subtree: true, attributeFilter: ['class'] }, actSlide);
  actSlide();

  // 下のタスクバー。選んでいる項目の下地と印を滑らせ、新しく開いた窓の項目は弾ませて出す
  const tasks = $('#task-items');
  const taskSlide = slider(tasks, '.task-item.active', e => e.dataset.win, (el, from, to) => { slideX(el, from, to); slideMark(el, from, to); }, activeServer);
  let knownWins = null, winsOf = null;
  watch(tasks, { childList: true }, () => {
    taskSlide();
    const items = [...tasks.children];
    if (knownWins && winsOf === activeServer() && !still()) for (const e of items) if (!knownWins.has(e.dataset.win)) e.classList.add('enter');
    knownWins = new Set(items.map(e => e.dataset.win));
    winsOf = activeServer();
  });

  // ファイルツリー。開いたフォルダの中身を上から順に出し、矢印を回す。
  // サーバーを切り替えたときと最初の描画では、全部の行を上から順に出す
  const tree = $('#tree');
  let seen = new Map(), seenOf = null;
  watch(tree, { childList: true }, () => {
    const all = !seen.size || seenOf !== activeServer();
    const now = new Map();
    let k = 0;
    for (const row of tree.querySelectorAll('.tree-row[data-path]')) {
      const chev = row.querySelector('.chev');
      const open = chev ? chev.classList.contains('open') : null;
      now.set(row.dataset.path, open);
      if (still()) continue;
      if (all || !seen.has(row.dataset.path)) {
        if (k < 30) { row.style.setProperty('--delay', `${k * (all ? 14 : 24)}ms`); row.classList.add('enter'); }
        k++;
      } else if (chev && seen.get(row.dataset.path) !== open) {
        chev.animate([{ transform: `rotate(${open ? 0 : 90}deg)` }, { transform: `rotate(${open ? 90 : 0}deg)` }], { duration: 380, easing: SPRING });
      }
    }
    seen = now;
    seenOf = activeServer();
  });

  // ウィンドウ。開くときはぼかしから浮かび上がらせる。同時に開いた窓は少しずつずらす。
  // サーバーを切り替えたときは、そのサーバーの窓の層をふわっと出す
  const onLayer = layer => watch(layer, { childList: true, attributes: true, attributeFilter: ['hidden'] }, recs => {
    if (still() || isMobile()) return;
    let n = 0;
    for (const r of recs) {
      if (r.type === 'attributes') {
        if (!layer.hidden) layer.animate([{ opacity: 0, transform: 'translateY(8px)' }, { opacity: 1, transform: 'none' }], { duration: 320, easing: EASE });
        continue;
      }
      for (const w of r.addedNodes) {
        if (!w.classList?.contains('win')) continue;
        w.getAnimations().forEach(a => a.cancel());
        w.animate([
          { opacity: 0, transform: 'translateY(18px) scale(.94)', filter: 'blur(6px)' },
          { opacity: 1, transform: 'none', filter: 'blur(0)' },
        ], { duration: 520, delay: n++ * 80, easing: 'cubic-bezier(.2,1.15,.35,1)', fill: 'backwards' });
      }
    }
  });
  const desk = $('#windows');
  desk.querySelectorAll('.windows-layer').forEach(onLayer);
  watch(desk, { childList: true }, recs => recs.forEach(r => r.addedNodes.forEach(n => n.classList?.contains('windows-layer') && onLayer(n))));

  // ガラスの板は、ポインターの位置に光を当てる
  document.addEventListener('pointermove', e => {
    const card = e.target.closest?.('.saved-item, button.mon-card');
    if (!card) return;
    const r = card.getBoundingClientRect();
    card.style.setProperty('--mx', `${e.clientX - r.left}px`);
    card.style.setProperty('--my', `${e.clientY - r.top}px`);
  }, { passive: true });

  // ダイアログはばねで持ち上げ、メニューの項目は上から順に出す
  watch(document.body, { childList: true }, recs => {
    for (const r of recs) for (const n of r.addedNodes) {
      if (n.classList?.contains('menu')) [...n.children].forEach((c, i) => c.style.setProperty('--i', Math.min(i, 14)));
      const card = n.classList?.contains('overlay') && n.querySelector('.modal');
      if (!card || still()) continue;
      card.getAnimations().forEach(a => a.cancel());
      card.animate(isMobile()
        ? [{ transform: 'translateY(100%)' }, { transform: 'none' }]
        : [{ opacity: 0, transform: 'translateY(22px) scale(.95)' }, { opacity: 1, transform: 'none' }],
      { duration: isMobile() ? 420 : 460, easing: 'cubic-bezier(.2,1.15,.35,1)' });
    }
  });
})();
