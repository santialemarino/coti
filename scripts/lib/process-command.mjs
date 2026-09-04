import fs from 'node:fs';

export function packageManagerInvocation(args = []) {
  const cli = process.env.npm_execpath;
  if (cli && fs.existsSync(cli)) {
    return { command: process.execPath, args: [cli, ...args] };
  }
  if (process.platform === 'win32') {
    return {
      command: process.env.ComSpec ?? 'cmd.exe',
      args: ['/d', '/s', '/c', 'pnpm', ...args],
    };
  }
  return { command: 'pnpm', args };
}
