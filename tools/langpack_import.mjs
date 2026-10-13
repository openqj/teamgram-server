#!/usr/bin/env node

import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const languages = process.argv.slice(2);
if (!languages.length) {
  console.error('usage: node tools/langpack_import.mjs <lang-code> [...]');
  process.exit(1);
}

const outputDirectory = new URL('../app/bff/apifull/internal/core/langpacks/weba/', import.meta.url);
await mkdir(outputDirectory, { recursive: true });
const run = promisify(execFile);
const temporaryDirectory = await mkdtemp(join(tmpdir(), 'teamgram-langpack-'));

try {
  for (const language of languages) {
    const pagePath = join(temporaryDirectory, `${language}.html`);
    const cookiePath = join(temporaryDirectory, `${language}.cookie`);
    await run('curl', [
      '-L', '--fail', '--silent', '--show-error', '--max-time', '30',
      '-c', cookiePath,
      `https://translations.telegram.org/${language}/weba`,
      '-o', pagePath,
    ]);

    const page = await readFile(pagePath, 'utf8');
    const hash = page.match(/api\?hash=([a-z0-9]+)/)?.[1];
    if (!hash) {
      throw new Error(`missing translations API hash for ${language}`);
    }

    const body = new URLSearchParams({
      method: 'getLangPackFull',
      lang: language,
      lang_pack: 'weba',
    }).toString();
    const { stdout } = await run('curl', [
      '-L', '--fail', '--silent', '--show-error', '--max-time', '60',
      '-b', cookiePath, '-c', cookiePath,
      '-X', 'POST', `https://translations.telegram.org/api?hash=${hash}`,
      '--data', body,
      '-H', 'X-Requested-With: XMLHttpRequest',
      '-H', `Referer: https://translations.telegram.org/${language}/weba`,
    ], { maxBuffer: 20 * 1024 * 1024 });

    const payload = JSON.parse(stdout);
    const sourceEntries = payload.data?.weba;
    if (!Array.isArray(sourceEntries)) {
      throw new Error(`invalid ${language} pack response`);
    }

    const entries = sourceEntries
      .filter(({ value, def_value }) => value !== undefined || (language === 'en' && def_value !== undefined))
      .map(({ key, value, def_value }) => ({ key, value: value ?? def_value }));
    const output = JSON.stringify({ version: 1, entries });
    await writeFile(join(outputDirectory.pathname, `${language}.json`), `${output}\n`, 'utf8');
    console.log(`${language}: ${entries.length} translated strings`);
  }
} finally {
  await rm(temporaryDirectory, { recursive: true, force: true });
}
