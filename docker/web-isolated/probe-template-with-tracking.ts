import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';
import { authSession, loadFixtureAuthKey } from './fixture-auth';
import { SessionProbe, MultiSessionProbeTracker } from './session-probe-framework';
import * as fs from 'node:fs';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURES = [
  { phone: '12025550101', username: 'isolated_test_01' },
  { phone: '12025550102', username: 'isolated_test_02' },
  { phone: '12025550103', username: 'isolated_test_03' },
];

type Actor = {
  client: any;
  id: bigint | number;
  username: string;
  probe?: SessionProbe;
};

type ProbeReport = {
  testName: string;
  timestamp: number;
  phases: Array<{
    name: string;
    success: boolean;
    ptsBefore?: number;
    ptsAfter?: number;
    violations: any[];
  }>;
  crossSessionConsistency: {
    synchronized: boolean;
    violations: any[];
  };
  layer229Compliance: {
    constructorMatches: number;
    constructorMismatches: number;
    errorCodeViolations: number;
  };
};

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
let randomIDCounter = 0n;

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: any) {
  return value?.className || value?.constructor?.name || typeof value;
}

function randomID() {
  randomIDCounter += 1n;
  return BigInt(Date.now()) * 1_000_000n + randomIDCounter;
}

async function loadGramJs(root: string) {
  const requireModule = createRequire(import.meta.url);
  const clientModule = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts'));
  const sessionModule = requireModule(path.join(root, 'src/lib/gramjs/sessions/Memory.ts'));
  const callbackSessionModule = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts'));
  const tlModule = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts'));
  const connectionModule = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'));
  return {
    TelegramClient: clientModule.default,
    MemorySession: sessionModule.default,
    CallbackSession: callbackSessionModule.default,
    Api: tlModule.Api,
    Connection: connectionModule.ConnectionTCPObfuscated,
  };
}

async function login(gram: any, fixture: typeof FIXTURES[number], sessionProbe: boolean = true): Promise<Actor> {
  const authKey = loadFixtureAuthKey(fixture.username);
  const session = authKey
    ? authSession(gram, authKey, 2, process.env.TG_DC_HOST || '127.0.0.1', Number(process.env.TG_DC_PORT || '31443'))
    : new gram.MemorySession();

  const client = new gram.TelegramClient(session, API_ID, API_HASH, {
    connection: gram.Connection,
    fallbackConnection: gram.Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 15,
    requestRetries: 2,
    connectionRetries: 2,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });

  try {
    if (authKey) {
      await client.connect();
    } else {
      await client.start({
        phoneNumber: fixture.phone,
        phoneCode: async () => '12345',
        password: async () => '',
        firstAndLastNames: async () => ['Isolated', 'Probe Test'],
        qrCode: async () => {},
        onError: (error: Error) => { throw error; },
        webAuthTokenFailed: () => {},
        onPasskeyOption: () => {},
        initialMethod: 'phoneNumber',
      });
    }

    const me = await client.getMe();
    requireCondition(me?.id !== undefined, `login failed for ${fixture.username}`);

    let probe: SessionProbe | undefined;
    if (sessionProbe) {
      probe = new SessionProbe(`session_${fixture.username}`, client, me.id, authKey || '');
      await probe.connect();
      await probe.captureState();
    }

    return { client, id: me.id, username: fixture.username, probe };
  } catch (error) {
    client.destroy();
    throw error;
  }
}

export class ProbeWithSessionTracking {
  private tracker: MultiSessionProbeTracker;
  private report: ProbeReport;
  private gram: any;

  constructor(testName: string) {
    this.tracker = new MultiSessionProbeTracker();
    this.report = {
      testName,
      timestamp: Date.now(),
      phases: [],
      crossSessionConsistency: { synchronized: true, violations: [] },
      layer229Compliance: {
        constructorMatches: 0,
        constructorMismatches: 0,
        errorCodeViolations: 0,
      },
    };
  }

  async initialize(gramjsRoot: string): Promise<void> {
    this.gram = await loadGramJs(gramjsRoot);
  }

  protected async recordPhase(
    name: string,
    actor: Actor,
    action: () => Promise<void>
  ): Promise<void> {
    const probe = actor.probe;
    if (!probe) {
      await action();
      return;
    }

    const ptsBefore = probe.getState().pts;
    try {
      await action();
      await probe.captureState();
      const ptsAfter = probe.getState().pts;

      const violations = probe.getViolations();
      this.report.layer229Compliance.constructorMismatches += violations
        .filter(v => v.issue === 'constructor_mismatch').length;
      this.report.layer229Compliance.errorCodeViolations += violations
        .filter(v => v.issue === 'error_code_noncanonical').length;

      this.report.phases.push({
        name,
        success: true,
        ptsBefore,
        ptsAfter,
        violations,
      });
    } catch (error) {
      this.report.phases.push({
        name,
        success: false,
        ptsBefore,
        violations: [{ type: 'error', error: String(error) }],
      });
      throw error;
    }
  }

  protected registerSession(sessionId: string, probe: SessionProbe): void {
    this.tracker.registerSession(sessionId, probe);
  }

  protected verifySessionConsistency(sessionIds: string[]): void {
    this.tracker.assertConsistentOrdering(sessionIds, sessionIds);
    this.tracker.assertSeqAligned(sessionIds);

    const violations = this.tracker.getCrossSessionViolations();
    if (violations.length > 0) {
      this.report.crossSessionConsistency.synchronized = false;
      this.report.crossSessionConsistency.violations = violations;
    }
  }

  protected loginActor(fixture: typeof FIXTURES[number]): Promise<Actor> {
    return login(this.gram, fixture, true);
  }

  exportReport(): string {
    return JSON.stringify(this.report, null, 2);
  }

  saveReport(suffix: string): string {
    const filename = `/tmp/teamgram-probe-${this.report.testName}-${suffix}-${this.report.timestamp}.json`;
    fs.writeFileSync(filename, this.exportReport());
    return filename;
  }
}

// Example usage in a probe test
export async function runProbeWithTracking(
  testName: string,
  gramjsRoot: string,
  testFn: (probe: ProbeWithSessionTracking, fixtures: typeof FIXTURES) => Promise<void>
): Promise<void> {
  const probe = new ProbeWithSessionTracking(testName);
  await probe.initialize(gramjsRoot);
  await testFn(probe, FIXTURES);
  const reportFile = probe.saveReport('final');
  console.log(`\n✓ Report saved to ${reportFile}`);
  console.log(probe.exportReport());
}
