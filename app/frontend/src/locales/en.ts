import type { Messages } from './ja'

const en: Messages = {
  brandSub: 'DRAG Runs As GUI',
  connect: {
    open: 'Connect to a server',
    title: 'Connect to a server',
    name: 'Name',
    user: 'User',
    port: 'Port',
    host: 'Host',
    auth: 'Authentication',
    authPassword: 'Password',
    authKey: 'Key file',
    password: 'Password',
    keyFile: 'Path to the key file',
    passphrase: 'Key passphrase',
    cancel: 'Cancel',
    submit: 'Connect',
    connecting: 'Connecting',
    newKeyTitle: 'This is the first connection to this server',
    newKeyText: 'Check that the fingerprint of the server key matches the one from the server administrator.',
    mismatchTitle: 'The server key differs from the last connection',
    mismatchText: 'Someone may be impersonating the server. Do not connect unless you know why the key changed, such as a reinstall.',
    trust: 'Trust and connect',
  },
  files: {
    title: 'Files',
    empty: 'Empty folder',
    error: 'Cannot open: {{error}}',
  },
  terminal: {
    title: 'Terminal',
    ended: 'The shell has exited',
  },
  desktop: {
    empty: 'Drag files or sessions here',
  },
  tabs: {
    close: 'Disconnect',
    add: 'Add a server',
  },
  status: {
    lost: 'Lost the connection to {{name}}',
  },
  activity: {
    files: 'Files',
    sessions: 'Sessions',
    settings: 'Settings',
  },
}

export default en
