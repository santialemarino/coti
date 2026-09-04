import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

import { writeRFQDashboard } from './lib/rfq-report.mjs';

const ROOT = process.cwd();
const ARTIFACTS = path.join(ROOT, '.artifacts', 'rfq-eval');
const DEFAULT_CASES = path.join(ROOT, 'scripts', 'fixtures', 'rfq-eval-cases.json');

export function parseReportArgs(args) {
  const options = {};
  for (let i = 0; i < args.length; i += 1) {
    if (args[i] === '--help' || args[i] === '-h') {
      options.help = true;
      continue;
    }
    const key = { '--input': 'inputPath', '--output': 'outputPath', '--cases': 'casesPath' }[
      args[i]
    ];
    if (!key || !args[i + 1]) throw new Error(`Unknown or incomplete argument: ${args[i]}`);
    options[key] = args[i + 1];
    i += 1;
  }
  return options;
}

function usage() {
  return `Usage: pnpm report:rfq [options]

Builds an interactive HTML dashboard from an RFQ evaluation JSON report.

Options:
  --input <path>   JSON report (default: latest report under .artifacts/rfq-eval)
  --output <path>  HTML destination (default: beside the input report)
  --cases <path>   Case definitions used to enrich older reports
  --help           Show this help`;
}

function latestReport() {
  if (!fs.existsSync(ARTIFACTS)) throw new Error('No RFQ evaluation report exists yet');
  const reports = fs
    .readdirSync(ARTIFACTS)
    .filter((name) => name.endsWith('.json'))
    .map((name) => path.join(ARTIFACTS, name))
    .sort((left, right) => fs.statSync(right).mtimeMs - fs.statSync(left).mtimeMs);
  if (reports.length === 0) throw new Error('No RFQ evaluation report exists yet');
  return reports[0];
}

function main() {
  const options = parseReportArgs(process.argv.slice(2));
  if (options.help) {
    console.log(usage());
    return;
  }
  const inputPath = path.resolve(options.inputPath ?? latestReport());
  const outputPath = path.resolve(
    options.outputPath ?? inputPath.replace(/\.json$/i, '') + '.html',
  );
  const casesPath = path.resolve(options.casesPath ?? DEFAULT_CASES);
  const report = JSON.parse(fs.readFileSync(inputPath, 'utf8'));
  const definitions = fs.existsSync(casesPath)
    ? JSON.parse(fs.readFileSync(casesPath, 'utf8'))
    : [];
  writeRFQDashboard(report, definitions, outputPath);
  console.log(`Dashboard: ${path.relative(ROOT, outputPath)}`);
}

const entrypoint = process.argv[1] ? pathToFileURL(path.resolve(process.argv[1])).href : '';
if (import.meta.url === entrypoint) {
  try {
    main();
  } catch (error) {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  }
}
