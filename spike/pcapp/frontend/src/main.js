import '@xterm/xterm/css/xterm.css';
import './style.css';
import { Terminal } from '@xterm/xterm';
import { Connect, Input, Resize } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

const status = document.getElementById('status');
const term = new Terminal({
    cols: 100,
    rows: 30,
    fontSize: 15,
    fontFamily: 'Consolas, "BIZ UDGothic", "MS Gothic", monospace',
    cursorBlink: true,
});
term.open(document.getElementById('term'));
term.focus();
window.term = term;

EventsOn('out', b64 => {
    const s = atob(b64);
    const b = new Uint8Array(s.length);
    for (let i = 0; i < s.length; i++) b[i] = s.charCodeAt(i);
    term.write(b);
});
EventsOn('closed', reason => { status.textContent = `切断 ${reason}`; });
term.onData(d => Input(d));
term.onResize(({ cols, rows }) => Resize(cols, rows));

Connect(term.cols, term.rows).then(err => {
    status.textContent = err ? `接続できない ${err}` : '接続しました';
});
