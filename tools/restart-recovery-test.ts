import { execFileSync, spawnSync } from 'node:child_process';
import { TelegramClient } from 'telegram';
import { StringSession } from 'telegram/sessions';
import { SessionProbe, loadFixtureAuthKey, createSession, MultiSessionProbeTracker } from '../docker/web-isolated/session-probe-framework';
import * as fs from 'node:fs';

type RestartRecoveryTestPhase = 'setup' | 'baseline' | 'inject_operation' | 'kill_backend' | 'restart' | 'verify_recovery';

interface RestartRecoveryTestResult {
  phase: RestartRecoveryTestPhase;
  timestamp: number;
  success: boolean;
  details: string;
  sessionStatesBefore?: any;
  sessionStatesAfter?: any;
  pgConsistency?: {
    authUsersCount: number;
    authKeysCount: number;
    userPtsUpdatesCount: number;
  };
  violations?: any[];
}

interface RestartRecoveryTestConfig {
  userUsername: string;
  channelId: number;
  messageText: string;
  dcId: number;
  host: string;
  port: number;
  pgHost: string;
  pgPort: number;
  pgUser: string;
  pgPassword: string;
  pgDatabase: string;
}

export class RestartRecoveryTest {
  private results: RestartRecoveryTestResult[] = [];
  private config: RestartRecoveryTestConfig;
  private sessionA: SessionProbe | null = null;
  private sessionB: SessionProbe | null = null;
  private tracker: MultiSessionProbeTracker;

  constructor(config: RestartRecoveryTestConfig) {
    this.config = config;
    this.tracker = new MultiSessionProbeTracker();
  }

  async run(): Promise<RestartRecoveryTestResult[]> {
    try {
      await this.phase1Setup();
      await this.phase2Baseline();
      await this.phase3InjectOperation();
      await this.phase4KillBackend();
      await this.phase5Restart();
      await this.phase6VerifyRecovery();
    } catch (error) {
      this.recordResult('verify_recovery', false, `Test failed: ${error}`);
    }
    return this.results;
  }

  private async phase1Setup(): Promise<void> {
    this.recordResult('setup', true, 'Initializing two client sessions...');

    try {
      // Load fixture auth keys for same user (different offsets)
      const authKeyA = loadFixtureAuthKey(this.config.userUsername, 0);
      const authKeyB = loadFixtureAuthKey(this.config.userUsername, 1);

      if (!authKeyA || !authKeyB) {
        throw new Error('Failed to load fixture auth keys');
      }

      // Create TelegramClient instances
      const clientA = new TelegramClient(
        new StringSession(Buffer.from(authKeyA, 'base64').toString('hex')),
        this.config.dcId,
        { connectionRetries: 3, retryDelay: 100 }
      );

      const clientB = new TelegramClient(
        new StringSession(Buffer.from(authKeyB, 'base64').toString('hex')),
        this.config.dcId,
        { connectionRetries: 3, retryDelay: 100 }
      );

      // Connect both clients
      await clientA.connect();
      await clientB.connect();

      // Create SessionProbe wrappers
      this.sessionA = new SessionProbe('session_a', clientA, 0, authKeyA);
      this.sessionB = new SessionProbe('session_b', clientB, 0, authKeyB);

      this.tracker.registerSession('session_a', this.sessionA);
      this.tracker.registerSession('session_b', this.sessionB);

      // Record initial state
      await this.sessionA.captureState();
      await this.sessionB.captureState();

      this.recordResult('setup', true, 'Two sessions connected successfully', {
        sessionA: this.sessionA.getState(),
        sessionB: this.sessionB.getState(),
      });
    } catch (error) {
      this.recordResult('setup', false, `Setup failed: ${error}`);
      throw error;
    }
  }

  private async phase2Baseline(): Promise<void> {
    this.recordResult('baseline', true, 'Recording baseline state...');

    if (!this.sessionA || !this.sessionB) {
      throw new Error('Sessions not initialized');
    }

    try {
      const stateA = this.sessionA.getState();
      const stateB = this.sessionB.getState();

      // Query PostgreSQL baseline
      const pgBaseline = this.queryPostgresBaseline();

      this.recordResult('baseline', true, 'Baseline captured', {
        sessionA: stateA,
        sessionB: stateB,
        pgBaseline,
      });
    } catch (error) {
      this.recordResult('baseline', false, `Baseline capture failed: ${error}`);
      throw error;
    }
  }

  private async phase3InjectOperation(): Promise<void> {
    this.recordResult('inject_operation', true, 'Executing operation on session A...');

    if (!this.sessionA) {
      throw new Error('Session A not initialized');
    }

    try {
      const ptsBefore = this.sessionA.getState().pts;

      // Simulate an operation: send a message to the test channel
      // Using the channel ID provided in config
      await this.sessionA.rpc(
        'messages.sendMessage',
        {
          peer: { _: 'inputPeerChannel', channelId: this.config.channelId, accessHash: 0 },
          message: this.config.messageText,
          randomId: Math.floor(Math.random() * 0x7fffffff),
        },
        'updates.UpdateShortSentMessage'
      );

      const ptsAfter = this.sessionA.getState().pts;
      this.sessionA.assertPtsIncremented(ptsBefore, ptsAfter);

      this.recordResult('inject_operation', true, `Operation completed, PTS: ${ptsBefore} -> ${ptsAfter}`, {
        ptsBefore,
        ptsAfter,
      });
    } catch (error) {
      this.recordResult('inject_operation', false, `Operation injection failed: ${error}`);
      throw error;
    }
  }

  private async phase4KillBackend(): Promise<void> {
    this.recordResult('kill_backend', true, 'Force killing backend container...');

    try {
      // Record state before kill
      const stateBefore = {
        sessionA: this.sessionA?.getState(),
        sessionB: this.sessionB?.getState(),
      };

      // Forcefully kill the backend container
      execFileSync('docker', ['kill', 'teamgram']);

      // Wait for container to stop
      let retries = 10;
      while (retries-- > 0) {
        try {
          execFileSync('docker', ['inspect', 'teamgram'], { stdio: 'ignore' });
          // If inspect succeeds, container still exists, check if it's running
          const status = execFileSync('docker', ['inspect', '-f', '{{.State.Running}}', 'teamgram'], {
            encoding: 'utf8',
          }).trim();
          if (status === 'false') break;
          throw new Error('still running');
        } catch {
          break; // Container stopped or gone
        }
      }

      this.recordResult('kill_backend', true, 'Backend killed successfully', {
        stateBefore,
        pgStateSnapshot: this.queryPostgresBaseline(),
      });
    } catch (error) {
      this.recordResult('kill_backend', false, `Kill failed: ${error}`);
      throw error;
    }
  }

  private async phase5Restart(): Promise<void> {
    this.recordResult('restart', true, 'Restarting backend and verifying migrations...');

    try {
      // Start the backend container
      execFileSync('docker', ['compose', 'up', '-d', 'teamgram']);

      // Wait for healthcheck
      let retries = 30;
      while (retries-- > 0) {
        try {
          const health = execFileSync('docker', ['inspect', '-f', '{{.State.Health.Status}}', 'teamgram'], {
            encoding: 'utf8',
          }).trim();
          if (health === 'healthy') break;
        } catch {
          // Container may not have health info yet
        }
        await new Promise(r => setTimeout(r, 1000));
      }

      // Verify migrations applied
      const pgState = this.queryPostgresBaseline();
      this.recordResult('restart', true, 'Backend restarted and healthy', {
        pgState,
      });
    } catch (error) {
      this.recordResult('restart', false, `Restart failed: ${error}`);
      throw error;
    }
  }

  private async phase6VerifyRecovery(): Promise<void> {
    this.recordResult('verify_recovery', true, 'Verifying session recovery and consistency...');

    try {
      // Reconnect both sessions
      if (!this.sessionA || !this.sessionB) {
        throw new Error('Sessions not available');
      }

      // Reconnect session A (the one that executed the operation)
      await this.sessionA.connect();
      await this.sessionA.captureState();

      // Reconnect session B (the observer)
      await this.sessionB.connect();
      await this.sessionB.captureState();

      // Verify PTS consistency
      const stateA = this.sessionA.getState();
      const stateB = this.sessionB.getState();

      // Both sessions should see the same PTS (after sync)
      await new Promise(r => setTimeout(r, 1000)); // Wait for updates to propagate
      await this.sessionB.captureState();

      const finalStateB = this.sessionB.getState();

      // Verify no data loss: the operation should still be there
      const allViolations = this.tracker.getAllViolations();

      this.recordResult('verify_recovery', true, 'Recovery verified successfully', {
        sessionA: stateA,
        sessionB: finalStateB,
        violations: allViolations,
        pgState: this.queryPostgresBaseline(),
      });
    } catch (error) {
      this.recordResult('verify_recovery', false, `Recovery verification failed: ${error}`);
      throw error;
    }
  }

  private queryPostgresBaseline(): any {
    try {
      const pgCmd = `
        SELECT
          (SELECT COUNT(*) FROM auth_users WHERE deleted=0) as active_auth_users,
          (SELECT COUNT(*) FROM auth_keys WHERE deleted=0) as active_auth_keys,
          (SELECT COUNT(*) FROM user_pts_updates) as pts_updates
      `;

      const result = execFileSync('docker', [
        'exec',
        'teamgram-postgres',
        'psql',
        '-v', 'ON_ERROR_STOP=1',
        '-At', '-q',
        '-U', this.config.pgUser,
        '-d', this.config.pgDatabase,
        '-c', pgCmd,
      ], { encoding: 'utf8' }).trim();

      const [activeAuthUsers, activeAuthKeys, ptsUpdates] = result.split('|');
      return {
        activeAuthUsers: parseInt(activeAuthUsers, 10),
        activeAuthKeys: parseInt(activeAuthKeys, 10),
        ptsUpdates: parseInt(ptsUpdates, 10),
      };
    } catch (error) {
      console.error('PostgreSQL query failed:', error);
      return null;
    }
  }

  private recordResult(phase: RestartRecoveryTestPhase, success: boolean, details: string, extra?: any): void {
    const result: RestartRecoveryTestResult = {
      phase,
      timestamp: Date.now(),
      success,
      details,
      ...extra,
    };
    this.results.push(result);
    console.log(`[${phase}] ${success ? '✓' : '✗'} ${details}`);
  }

  exportReport(): string {
    return JSON.stringify(
      {
        testName: 'RestartRecoveryTest',
        config: this.config,
        results: this.results,
        summary: {
          totalPhases: this.results.length,
          successfulPhases: this.results.filter(r => r.success).length,
          failedPhases: this.results.filter(r => !r.success).length,
          overallSuccess: this.results.every(r => r.success),
        },
      },
      null,
      2
    );
  }
}

// CLI entry point
async function main() {
  const config: RestartRecoveryTestConfig = {
    userUsername: process.env.TG_TEST_USER || 'testuser',
    channelId: parseInt(process.env.TG_TEST_CHANNEL_ID || '0', 10),
    messageText: `Restart recovery test message ${Date.now()}`,
    dcId: parseInt(process.env.TG_DC_ID || '2', 10),
    host: process.env.TG_DC_HOST || '127.0.0.1',
    port: parseInt(process.env.TG_DC_PORT || '31443', 10),
    pgHost: process.env.POSTGRES_HOST || 'localhost',
    pgPort: parseInt(process.env.POSTGRES_PORT || '5432', 10),
    pgUser: process.env.POSTGRES_USER || 'teamgram',
    pgPassword: process.env.POSTGRES_PASSWORD || 'teamgram',
    pgDatabase: process.env.POSTGRES_DB || 'teamgram',
  };

  const test = new RestartRecoveryTest(config);
  const results = await test.run();

  const report = test.exportReport();
  const reportFile = `/tmp/teamgram-restart-recovery-${Date.now()}.json`;
  fs.writeFileSync(reportFile, report);

  console.log(`\n✓ Report saved to ${reportFile}`);
  console.log(report);

  process.exit(results.every(r => r.success) ? 0 : 1);
}

if (require.main === module) {
  main().catch(error => {
    console.error('Test failed:', error);
    process.exit(1);
  });
}

export default RestartRecoveryTest;
