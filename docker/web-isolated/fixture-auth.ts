import { execFileSync } from 'node:child_process';

type GramJs = {
  CallbackSession: new (data: any, callback: (session?: any) => void) => any;
};

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres',
    'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

export function loadFixtureAuthKey(username: string, offset = 0): string | undefined {
  if (!Number.isInteger(offset) || offset < 0) {
    throw new Error(`fixture auth key offset must be a non-negative integer: ${offset}`);
  }
  const value = postgres(`
    SELECT encode(k.body, 'base64')
    FROM users u
    JOIN auth_users au ON au.user_id=u.id
    JOIN auth_keys k ON k.auth_key_id=au.auth_key_id
    JOIN auth_key_infos i ON i.auth_key_id=au.auth_key_id
    WHERE u.username='${username.replaceAll("'", "''")}'
      AND u.deleted=false AND au.state=0 AND au.deleted=false
      AND k.deleted=false AND i.deleted=false AND i.auth_key_type=0
    ORDER BY au.date_active DESC, au.id DESC
    OFFSET ${offset}
    LIMIT 1
  `);
  return value || undefined;
}

export function authSession(gram: GramJs, authKeyB64: string, dcId: number, host: string, port: number): any {
  const session = new gram.CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(authKeyB64, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    session._serverAddress = host;
    session._port = port;
  };
  return session;
}
