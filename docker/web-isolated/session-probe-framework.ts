import { execFileSync } from 'node:child_process';

type SessionEvent = {
  timestamp: number;
  type: 'connect' | 'rpc_call' | 'update' | 'error' | 'disconnect';
  method?: string;
  result?: any;
  error?: string;
  pts?: number;
  seq?: number;
  qts?: number;
  date?: number;
};

type SessionState = {
  sessionId: string;
  userId: number;
  authKeyId: string;
  connectedAt: number | null;
  pts: number;
  seq: number;
  qts: number;
  date: number;
  lastUpdate: number;
  events: SessionEvent[];
};

type Layer229Violation = {
  method: string;
  issue: 'constructor_mismatch' | 'parameter_order' | 'flags_invalid' | 'error_code_noncanonical';
  details: string;
};

export class SessionProbe {
  private state: SessionState;
  private client: any;
  private violations: Layer229Violation[] = [];
  private Api: any;

  constructor(sessionId: string, client: any, userId: number, authKeyId: string, Api?: any) {
    this.state = {
      sessionId,
      userId,
      authKeyId,
      connectedAt: null,
      pts: 0,
      seq: 0,
      qts: 0,
      date: 0,
      lastUpdate: 0,
      events: [],
    };
    this.client = client;
    this.Api = Api;
  }

  async connect(): Promise<void> {
    try {
      this.state.connectedAt = Date.now();
      await this.client.connect();
      this.recordEvent({
        type: 'connect',
        timestamp: this.state.connectedAt,
      });
    } catch (error) {
      this.recordEvent({
        type: 'error',
        timestamp: Date.now(),
        error: `Connection failed: ${error}`,
      });
      throw error;
    }
  }

  async rpc<T>(
    methodName: string,
    request: any,
    expectedConstructor?: string
  ): Promise<T> {
    const startTime = Date.now();
    try {
      const result = await this.client.invoke(request);

      // Validate Layer 229 constructor
      if (expectedConstructor && result.CONSTRUCTOR_ID !== undefined) {
        const actualConstructor = this.getConstructorName(result);
        if (!actualConstructor.includes(expectedConstructor)) {
          this.violations.push({
            method: methodName,
            issue: 'constructor_mismatch',
            details: `Expected ${expectedConstructor}, got ${actualConstructor}`,
          });
        }
      }

      // Validate error code (should be lowercase, camelCase)
      if (result._ && typeof result._ === 'string') {
        if (result._.includes('_')) {
          this.violations.push({
            method: methodName,
            issue: 'error_code_noncanonical',
            details: `Error code uses underscore: ${result._}`,
          });
        }
      }

      this.recordEvent({
        type: 'rpc_call',
        timestamp: startTime,
        method: methodName,
        result: this.sanitizeForLogging(result),
      });

      return result;
    } catch (error: any) {
      this.recordEvent({
        type: 'error',
        timestamp: Date.now(),
        method: methodName,
        error: error.message || String(error),
      });
      throw error;
    }
  }

  async captureState(): Promise<void> {
    try {
      if (!this.Api) {
        throw new Error('Api object not provided to SessionProbe');
      }
      const stateResp = await this.rpc('updates.getState', new this.Api.updates.GetState(), 'updates.State');
      if (stateResp.pts !== undefined) this.state.pts = stateResp.pts;
      if (stateResp.seq !== undefined) this.state.seq = stateResp.seq;
      if (stateResp.qts !== undefined) this.state.qts = stateResp.qts;
      if (stateResp.date !== undefined) this.state.date = stateResp.date;
      this.state.lastUpdate = Date.now();

      this.recordEvent({
        type: 'update',
        timestamp: this.state.lastUpdate,
        pts: this.state.pts,
        seq: this.state.seq,
        qts: this.state.qts,
        date: this.state.date,
      });
    } catch (error) {
      this.recordEvent({
        type: 'error',
        timestamp: Date.now(),
        error: `Failed to capture state: ${error}`,
      });
    }
  }

  assertPtsIncremented(before: number, after: number): void {
    if (after <= before) {
      this.violations.push({
        method: 'internal_assertion',
        issue: 'flags_invalid',
        details: `PTS did not increment: ${before} -> ${after}`,
      });
    }
  }

  assertSeqIncremented(before: number, after: number): void {
    if (after <= before) {
      this.violations.push({
        method: 'internal_assertion',
        issue: 'flags_invalid',
        details: `SEQ did not increment: ${before} -> ${after}`,
      });
    }
  }

  getState(): Readonly<SessionState> {
    return Object.freeze({ ...this.state });
  }

  getViolations(): Layer229Violation[] {
    return [...this.violations];
  }

  getEventLog(): SessionEvent[] {
    return [...this.state.events];
  }

  private recordEvent(event: Omit<SessionEvent, 'timestamp'> & { timestamp: number }): void {
    this.state.events.push(event as SessionEvent);
  }

  private getConstructorName(obj: any): string {
    if (obj._ && typeof obj._ === 'string') return obj._;
    if (obj.constructor && obj.constructor.name) return obj.constructor.name;
    return typeof obj;
  }

  private sanitizeForLogging(obj: any): any {
    if (typeof obj !== 'object' || obj === null) return obj;
    if (Buffer.isBuffer(obj)) return '[Buffer]';
    if (Array.isArray(obj)) return obj.map(v => this.sanitizeForLogging(v));
    if (obj._ && typeof obj._ === 'string') {
      const result: any = { _: obj._ };
      if (obj.pts !== undefined) result.pts = obj.pts;
      if (obj.seq !== undefined) result.seq = obj.seq;
      if (obj.qts !== undefined) result.qts = obj.qts;
      if (obj.date !== undefined) result.date = obj.date;
      return result;
    }
    return { ...obj };
  }
}

export interface MultiSessionProbeOptions {
  userA: { username: string; authKeyOffset?: number };
  userB: { username: string; authKeyOffset?: number };
  host: string;
  port: number;
  dcId: number;
}

export class MultiSessionProbeTracker {
  private sessions: Map<string, SessionProbe> = new Map();
  private crossSessionViolations: Array<{
    type: 'order_mismatch' | 'pts_desync' | 'seq_desync';
    details: string;
  }> = [];

  registerSession(sessionId: string, probe: SessionProbe): void {
    this.sessions.set(sessionId, probe);
  }

  assertConsistentOrdering(sessionIds: string[], expectedOrder: string[]): void {
    const actual = sessionIds.map(id => {
      const probe = this.sessions.get(id);
      return probe?.getState().pts ?? -1;
    });

    for (let i = 1; i < actual.length; i++) {
      if (actual[i] !== actual[i - 1]) {
        this.crossSessionViolations.push({
          type: 'pts_desync',
          details: `Session ${expectedOrder[i]} PTS ${actual[i]} != ${actual[i - 1]}`,
        });
      }
    }
  }

  assertSeqAligned(sessionIds: string[]): void {
    const seqs = sessionIds.map(id => this.sessions.get(id)?.getState().seq ?? -1);
    for (let i = 1; i < seqs.length; i++) {
      if (seqs[i] !== seqs[i - 1]) {
        this.crossSessionViolations.push({
          type: 'seq_desync',
          details: `Session ${i} SEQ ${seqs[i]} != ${seqs[i - 1]}`,
        });
      }
    }
  }

  getCrossSessionViolations() {
    return [...this.crossSessionViolations];
  }

  getAllViolations() {
    const violations: any[] = [];
    for (const [sessionId, probe] of this.sessions) {
      violations.push({
        sessionId,
        violations: probe.getViolations(),
      });
    }
    violations.push({ crossSession: this.crossSessionViolations });
    return violations;
  }
}

export function loadFixtureAuthKey(username: string, offset = 0): string | undefined {
  if (!Number.isInteger(offset) || offset < 0) {
    throw new Error(`fixture auth key offset must be a non-negative integer: ${offset}`);
  }
  const value = execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-U', 'teamgram', '-d', 'teamgram', '-c', `
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
  `,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  return value || undefined;
}

export function createSession(
  gram: any,
  authKeyB64: string,
  dcId: number,
  host: string,
  port: number
): any {
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
