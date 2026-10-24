// 検証用のサーバー (dev/testserver のコンテナ) でコマンドを実行する。失敗したら 'ERR ' と標準エラーを返す。
// この PC では、テストを続けて流すと、プロセスを起こすのが EPERM で失敗することがあるので、少し待ってやり直す
import { execSync } from 'node:child_process';

export function sh(cmd, user = 'asuta') {
  for (let i = 0; ; i++) {
    try {
      return execSync(`docker exec -u ${user} testserver-testserver-1 sh -c ${JSON.stringify(cmd)}`, { stdio: ['ignore', 'pipe', 'pipe'] }).toString().trim();
    } catch (e) {
      if (e.code === 'EPERM' && i < 5) {
        Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 1000);
        continue;
      }
      return 'ERR ' + String(e.stderr ?? e.message).trim();
    }
  }
}
