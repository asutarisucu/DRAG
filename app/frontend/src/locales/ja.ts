// 画面の文言。キーは英語、値は mock/ の文言に合わせる
const ja = {
  brandSub: 'DRAG Runs As GUI',
  connect: {
    open: 'サーバーに接続',
    title: 'サーバーに接続',
    name: '名前',
    user: 'ユーザー',
    port: 'ポート',
    host: 'ホスト',
    auth: '認証',
    authPassword: 'パスワード',
    authKey: '鍵ファイル',
    password: 'パスワード',
    keyFile: '鍵ファイルのパス',
    passphrase: '鍵のパスフレーズ',
    cancel: 'キャンセル',
    submit: '接続',
    connecting: '接続しています',
    newKeyTitle: 'このサーバーに初めて接続します',
    newKeyText: 'サーバーの鍵の指紋が、サーバーの管理者から聞いたものと同じかを確かめてください。',
    mismatchTitle: 'サーバーの鍵が、前に接続したときと違います',
    mismatchText: '別のサーバーになりすまされている恐れがあります。サーバーを入れ直したなど理由が分からないときは、接続しないでください。',
    trust: '信頼して接続',
  },
  files: {
    title: 'ファイル',
    empty: '空のフォルダ',
    error: '開けません: {{error}}',
  },
  terminal: {
    title: 'ターミナル',
    ended: 'シェルが終了しました',
  },
  desktop: {
    empty: 'ファイルやセッションをここへドラッグ',
  },
  tabs: {
    close: '切断',
    add: 'サーバーを追加',
  },
  status: {
    lost: '{{name}} との接続が切れました',
  },
  activity: {
    files: 'ファイル',
    sessions: 'セッション',
    settings: '設定',
  },
}

export default ja
export type Messages = typeof ja
