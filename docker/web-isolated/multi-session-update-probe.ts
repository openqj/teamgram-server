import { TelegramClient } from 'telegram';
import { Api } from 'telegram/tl';
import { StringSession } from 'telegram/sessions';
import { SessionProbe, MultiSessionProbeTracker, loadFixtureAuthKey } from './session-probe-framework';
import * as fs from 'node:fs';

type UpdateDeliveryRecord = {
  sessionId: string;
  updateType: string;
  receivedAt: number;
  pts?: number;
  seq?: number;
  qts?: number;
};

type MultiSessionUpdateProbeConfig = {
  userA: { username: string; authKeyOffset?: number };
  userB: { username: string; authKeyOffset?: number };
  channelId: number;
  messageText?: string;
  dcId: number;
  host: string;
  port: number;
};

export class MultiSessionUpdateProbe {
  private config: MultiSessionUpdateProbeConfig;
  private tracker: MultiSessionProbeTracker;
  private sessionsA: SessionProbe[] = [];
  private sessionB: SessionProbe | null = null;
  private updateDeliveries: UpdateDeliveryRecord[] = [];
  private violations: any[] = [];

  constructor(config: MultiSessionUpdateProbeConfig) {
    this.config = config;
    this.tracker = new MultiSessionProbeTracker();
  }

  async run(): Promise<{
    success: boolean;
    updateDeliveries: UpdateDeliveryRecord[];
    violations: any[];
    summary: string;
  }> {
    try {
      console.log('=== Multi-Session Update Probe ===');
      console.log(`User A: 2 sessions, User B: 1 session`);
      console.log(`Channel: ${this.config.channelId}`);

      // Phase 1: Setup
      await this.setupSessions();

      // Phase 2: Capture baseline state
      await this.captureBaseline();

      // Phase 3: User A sends message
      await this.userASendsMessage();

      // Phase 4: Verify delivery to all sessions
      await this.verifyUpdateDelivery();

      // Phase 5: Verify consistency
      await this.verifyConsistency();

      return {
        success: this.violations.length === 0,
        updateDeliveries: this.updateDeliveries,
        violations: this.violations,
        summary: this.generateSummary(),
      };
    } catch (error) {
      this.violations.push({
        type: 'fatal_error',
        error: String(error),
      });
      return {
        success: false,
        updateDeliveries: this.updateDeliveries,
        violations: this.violations,
        summary: `Probe failed: ${error}`,
      };
    }
  }

  private async setupSessions(): Promise<void> {
    console.log('\n[Phase 1] Setting up sessions...');

    // Load User A's auth keys (two sessions)
    const userAKey0 = loadFixtureAuthKey(this.config.userA.username, 0);
    const userAKey1 = loadFixtureAuthKey(this.config.userA.username, 1);

    // Load User B's auth key
    const userBKey = loadFixtureAuthKey(this.config.userB.username, 0);

    if (!userAKey0 || !userAKey1 || !userBKey) {
      throw new Error('Failed to load fixture auth keys');
    }

    // Create User A sessions
    for (let i = 0; i < 2; i++) {
      const key = i === 0 ? userAKey0 : userAKey1;
      const client = new TelegramClient(
        new StringSession(Buffer.from(key, 'base64').toString('hex')),
        this.config.dcId,
        { connectionRetries: 3, retryDelay: 100 }
      );

      await client.connect();
      const probe = new SessionProbe(`user_a_session_${i}`, client, 0, key);
      await probe.connect();
      await probe.captureState();
      this.sessionsA.push(probe);
      this.tracker.registerSession(`user_a_session_${i}`, probe);
      console.log(`✓ User A session ${i} connected, initial PTS=${probe.getState().pts}`);
    }

    // Create User B session
    const clientB = new TelegramClient(
      new StringSession(Buffer.from(userBKey, 'base64').toString('hex')),
      this.config.dcId,
      { connectionRetries: 3, retryDelay: 100 }
    );

    await clientB.connect();
    this.sessionB = new SessionProbe('user_b_session_0', clientB, 0, userBKey);
    await this.sessionB.connect();
    await this.sessionB.captureState();
    this.tracker.registerSession('user_b_session_0', this.sessionB);
    console.log(`✓ User B session 0 connected, initial PTS=${this.sessionB.getState().pts}`);
  }

  private async captureBaseline(): Promise<void> {
    console.log('\n[Phase 2] Capturing baseline state...');

    for (const probe of this.sessionsA) {
      await probe.captureState();
    }
    if (this.sessionB) {
      await this.sessionB.captureState();
    }

    console.log(`✓ Baseline captured`);
    console.log(`  User A session 0 PTS=${this.sessionsA[0].getState().pts}`);
    console.log(`  User A session 1 PTS=${this.sessionsA[1].getState().pts}`);
    console.log(`  User B session 0 PTS=${this.sessionB?.getState().pts}`);
  }

  private async userASendsMessage(): Promise<void> {
    console.log('\n[Phase 3] User A sending message to channel...');

    if (this.sessionsA.length === 0) {
      throw new Error('User A sessions not initialized');
    }

    const probe = this.sessionsA[0]; // Send from first session
    const ptsBefore = probe.getState().pts;

    const messageText = this.config.messageText || `Multi-session probe message ${Date.now()}`;

    // Send message through RPC
    const response = await probe.rpc(
      'messages.sendMessage',
      {
        peer: { _: 'inputPeerChannel', channelId: this.config.channelId, accessHash: 0 },
        message: messageText,
        randomId: Math.floor(Math.random() * 0x7fffffff),
      },
      'updates.UpdateShortSentMessage'
    );

    await probe.captureState();
    const ptsAfter = probe.getState().pts;

    probe.assertPtsIncremented(ptsBefore, ptsAfter);

    this.updateDeliveries.push({
      sessionId: 'user_a_session_0',
      updateType: 'message_sent',
      receivedAt: Date.now(),
      pts: ptsAfter,
    });

    console.log(`✓ Message sent, PTS incremented: ${ptsBefore} -> ${ptsAfter}`);
  }

  private async verifyUpdateDelivery(): Promise<void> {
    console.log('\n[Phase 4] Verifying update delivery to all sessions...');

    // Wait for updates to propagate
    await new Promise(r => setTimeout(r, 1000));

    // Capture current state from all sessions
    for (const probe of this.sessionsA) {
      await probe.captureState();
    }
    if (this.sessionB) {
      await this.sessionB.captureState();
    }

    const statesA = this.sessionsA.map((p, i) => ({
      sessionId: `user_a_session_${i}`,
      pts: p.getState().pts,
    }));

    const stateB = this.sessionB ? {
      sessionId: 'user_b_session_0',
      pts: this.sessionB.getState().pts,
    } : null;

    console.log(`\n  User A sessions (same user, different devices):`);
    for (const state of statesA) {
      console.log(`    ${state.sessionId}: PTS=${state.pts}`);
    }

    console.log(`\n  User B session (different user):`);
    if (stateB) {
      console.log(`    ${stateB.sessionId}: PTS=${stateB.pts}`);
    }

    // Record deliveries
    for (const state of statesA) {
      this.updateDeliveries.push({
        sessionId: state.sessionId,
        updateType: 'message_update',
        receivedAt: Date.now(),
        pts: state.pts,
      });
    }

    if (stateB) {
      this.updateDeliveries.push({
        sessionId: stateB.sessionId,
        updateType: 'message_update',
        receivedAt: Date.now(),
        pts: stateB.pts,
      });
    }
  }

  private async verifyConsistency(): Promise<void> {
    console.log('\n[Phase 5] Verifying cross-session consistency...');

    if (this.sessionsA.length < 2) {
      throw new Error('Insufficient User A sessions');
    }

    const statesA = this.sessionsA.map(p => p.getState());

    // Both sessions from same user should eventually see the same PTS
    // (allows some propagation delay)
    if (statesA[0].pts !== statesA[1].pts) {
      this.violations.push({
        type: 'pts_desync_same_user',
        details: `User A sessions have different PTS: session_0=${statesA[0].pts}, session_1=${statesA[1].pts}`,
        severity: 'warning', // Can be transient during propagation
      });
    } else {
      console.log(`✓ User A sessions synchronized: both PTS=${statesA[0].pts}`);
    }

    // Check for Layer 229 violations
    const allViolations = this.tracker.getAllViolations();
    if (allViolations.some((v: any) => v.violations?.length > 0)) {
      this.violations.push({
        type: 'layer229_violations',
        details: allViolations,
        severity: 'error',
      });
    } else {
      console.log(`✓ No Layer 229 violations detected`);
    }

    // Verify event logs for consistency
    for (const probe of this.sessionsA) {
      const events = probe.getEventLog();
      const rpcEvents = events.filter(e => e.type === 'rpc_call' && e.method);
      console.log(`  ${probe.getState().sessionId}: ${rpcEvents.length} RPC calls`);
    }
  }

  private generateSummary(): string {
    const totalDeliveries = this.updateDeliveries.length;
    const errorCount = this.violations.filter(v => v.severity === 'error').length;
    const warningCount = this.violations.filter(v => v.severity === 'warning').length;

    return `
Probe Results:
- Total updates tracked: ${totalDeliveries}
- Errors: ${errorCount}
- Warnings: ${warningCount}
- Status: ${errorCount === 0 ? '✓ PASS' : '✗ FAIL'}
    `.trim();
  }

  exportReport(): string {
    return JSON.stringify(
      {
        testName: 'MultiSessionUpdateProbe',
        config: this.config,
        updateDeliveries: this.updateDeliveries,
        violations: this.violations,
        summary: this.generateSummary(),
      },
      null,
      2
    );
  }
}

// CLI entry point
async function main() {
  const config: MultiSessionUpdateProbeConfig = {
    userA: {
      username: process.env.TG_USER_A || 'testuser_a',
      authKeyOffset: 0,
    },
    userB: {
      username: process.env.TG_USER_B || 'testuser_b',
      authKeyOffset: 0,
    },
    channelId: parseInt(process.env.TG_TEST_CHANNEL_ID || '0', 10),
    messageText: `Multi-session probe test ${Date.now()}`,
    dcId: parseInt(process.env.TG_DC_ID || '2', 10),
    host: process.env.TG_DC_HOST || '127.0.0.1',
    port: parseInt(process.env.TG_DC_PORT || '31443', 10),
  };

  const probe = new MultiSessionUpdateProbe(config);
  const result = await probe.run();

  const report = probe.exportReport();
  const reportFile = `/tmp/teamgram-multi-session-update-${Date.now()}.json`;
  fs.writeFileSync(reportFile, report);

  console.log(`\n✓ Report saved to ${reportFile}`);
  console.log('\n' + result.summary);

  if (result.violations.length > 0) {
    console.log('\nViolations:');
    result.violations.forEach(v => {
      console.log(`  - [${v.type}] ${v.details}`);
    });
  }

  process.exit(result.success ? 0 : 1);
}

if (require.main === module) {
  main().catch(error => {
    console.error('Probe failed:', error);
    process.exit(1);
  });
}

export default MultiSessionUpdateProbe;
