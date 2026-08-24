import fs from 'node:fs';
import path from 'node:path';

const FINAL_ACTIONS = new Map([
  ['pass', 'PASSED'],
  ['fail', 'FAILED'],
  ['skip', 'SKIPPED'],
]);

export function createGoTestCollector({ root, sourceFiles = [] }) {
  const sourceIndex = indexGoTestSources(root, sourceFiles);
  const tests = new Map();
  const packageOutput = new Map();

  return {
    accept(line) {
      const event = parseGoTestEvent(line);
      if (!event) return null;

      if (!event.Test) {
        if (event.Package && event.Output?.trimEnd()) {
          const output = packageOutput.get(event.Package) ?? [];
          output.push(event.Output.trimEnd());
          packageOutput.set(event.Package, output);
        }
        if (event.Action === 'fail' && event.Package) {
          const id = `${event.Package}:package`;
          tests.set(id, {
            id,
            name: event.Package,
            description: 'El paquete no pudo compilar o completar su inicialización.',
            package: event.Package,
            status: 'FAILED',
            duration_ms: elapsedMilliseconds(event.Elapsed),
            output: packageOutput.get(event.Package) ?? [],
            error: failureMessage(packageOutput.get(event.Package) ?? []),
            source: null,
          });
        }
        return event;
      }

      const id = `${event.Package}:${event.Test}`;
      const parentName = event.Test.split('/')[0];
      const entry = tests.get(id) ?? {
        id,
        name: event.Test,
        description: describeGoTest(event.Test),
        package: event.Package,
        status: 'RUNNING',
        duration_ms: null,
        output: [],
        error: null,
        source: sourceIndex.get(parentName) ?? null,
      };

      if (event.Output) entry.output.push(event.Output.trimEnd());
      if (event.Action === 'run' || event.Action === 'cont') entry.status = 'RUNNING';
      if (FINAL_ACTIONS.has(event.Action)) {
        entry.status = FINAL_ACTIONS.get(event.Action);
        if (entry.status === 'FAILED') entry.error = failureMessage(entry.output);
      }
      if (typeof event.Elapsed === 'number') entry.duration_ms = elapsedMilliseconds(event.Elapsed);
      tests.set(id, entry);
      return event;
    },

    finish(exitCode) {
      for (const entry of tests.values()) {
        if (entry.status !== 'RUNNING') continue;
        entry.status = exitCode === 0 ? 'PASSED' : 'FAILED';
        if (entry.status === 'FAILED') entry.error = failureMessage(entry.output);
      }
    },

    snapshot() {
      return [...tests.values()];
    },

    summary() {
      const entries = [...tests.values()];
      return {
        total: entries.length,
        passed: entries.filter((entry) => entry.status === 'PASSED').length,
        failed: entries.filter((entry) => entry.status === 'FAILED').length,
        skipped: entries.filter((entry) => entry.status === 'SKIPPED').length,
        running: entries.filter((entry) => entry.status === 'RUNNING').length,
      };
    },
  };
}

export function parseGoTestEvent(line) {
  if (!line.startsWith('{')) return null;
  try {
    const event = JSON.parse(line);
    return typeof event.Action === 'string' ? event : null;
  } catch {
    return null;
  }
}

function indexGoTestSources(root, sourceFiles) {
  const index = new Map();
  for (const relativePath of sourceFiles) {
    const absolutePath = path.resolve(root, relativePath);
    if (!fs.existsSync(absolutePath)) continue;
    const lines = fs.readFileSync(absolutePath, 'utf8').split(/\r?\n/);
    for (let lineIndex = 0; lineIndex < lines.length; lineIndex += 1) {
      const match = lines[lineIndex].match(/^func\s+(Test[A-Za-z0-9_]+)\s*\(/);
      if (!match) continue;
      index.set(match[1], {
        path: relativePath.replaceAll('\\', '/'),
        line: lineIndex + 1,
        code: testFunctionSnippet(lines, lineIndex),
      });
    }
  }
  return index;
}

function testFunctionSnippet(lines, start) {
  let end = Math.min(lines.length, start + 160);
  for (let index = start + 1; index < end; index += 1) {
    if (/^func\s+/.test(lines[index])) {
      end = index;
      break;
    }
  }
  return lines.slice(start, end).join('\n').trimEnd();
}

function describeGoTest(name) {
  return name
    .replace(/^Test/, '')
    .replaceAll('/', ' / ')
    .replaceAll('_', ' - ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1 $2')
    .trim();
}

function elapsedMilliseconds(elapsed) {
  return typeof elapsed === 'number' ? Math.round(elapsed * 1_000) : null;
}

function failureMessage(output) {
  return (
    output
      .filter((line) => !/^=== RUN\s|^--- FAIL:/.test(line.trim()))
      .join('\n')
      .trim() || 'The test finished with a failure status.'
  );
}
