import fs from 'node:fs';
import path from 'node:path';

// renderRFQDashboard returns a self-contained interactive report with no external assets.
export function renderRFQDashboard(report, definitions = []) {
  const byID = new Map(definitions.map((entry) => [entry.id, entry]));
  const view = {
    ...report,
    results: report.results.map((result) => {
      const definition = result.definition ?? byID.get(result.id) ?? null;
      return {
        ...result,
        description: definition?.description ?? result.description,
        message: definition?.message ?? result.message,
        definition,
      };
    }),
  };
  const encoded = Buffer.from(JSON.stringify(view), 'utf8').toString('base64');

  return `<!doctype html>
<html lang="es-AR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Reporte QA del motor RFQ</title>
  <style>
    :root {
      color-scheme: light;
      --page: #f3f5f7;
      --surface: #ffffff;
      --surface-raised: #f8fafb;
      --ink: #17212b;
      --muted: #5c6874;
      --line: #d8dee5;
      --accent: #155d88;
      --accent-soft: #e5f1f8;
      --pass: #167449;
      --pass-soft: #e7f5ed;
      --fail: #b42318;
      --fail-soft: #fdecea;
      --warning: #945f00;
      --warning-soft: #fff3d6;
      --code: #101820;
      --code-text: #dce6ee;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    * { box-sizing: border-box; }
    body { margin: 0; background: var(--page); color: var(--ink); }
    button { font: inherit; }
    button:focus-visible { outline: 3px solid color-mix(in srgb, var(--accent) 35%, transparent); outline-offset: 2px; }
    .app { min-height: 100vh; }
    .topbar { display: flex; min-height: 72px; align-items: center; justify-content: space-between; padding: 14px 24px; gap: 20px; border-bottom: 1px solid var(--line); background: var(--surface); }
    .brand h1 { margin: 0; font-size: 20px; line-height: 1.25; letter-spacing: 0; }
    .brand p { margin: 4px 0 0; color: var(--muted); font-size: 13px; }
    .run-meta { text-align: right; color: var(--muted); font-size: 12px; line-height: 1.5; }
    .summary { display: grid; grid-template-columns: repeat(4, minmax(120px, 1fr)); border-bottom: 1px solid var(--line); background: var(--surface); }
    .metric { min-height: 76px; padding: 14px 24px; border-right: 1px solid var(--line); }
    .metric:last-child { border-right: 0; }
    .metric-label { color: var(--muted); font-size: 12px; }
    .metric-value { margin-top: 5px; font-size: 24px; line-height: 1; font-weight: 700; }
    .metric-value.pass { color: var(--pass); }
    .metric-value.fail { color: var(--fail); }
    .workspace { display: grid; min-height: calc(100vh - 149px); grid-template-columns: minmax(280px, 360px) minmax(0, 1fr); }
    .sidebar { border-right: 1px solid var(--line); background: var(--surface); }
    .sidebar-tools { position: sticky; top: 0; z-index: 2; padding: 16px; border-bottom: 1px solid var(--line); background: var(--surface); }
    .filters { display: grid; grid-template-columns: repeat(3, 1fr); padding: 3px; gap: 3px; border: 1px solid var(--line); border-radius: 6px; background: var(--surface-raised); }
    .filter { min-height: 34px; border: 0; border-radius: 4px; background: transparent; color: var(--muted); cursor: pointer; font-size: 12px; font-weight: 650; }
    .filter:hover:not(:disabled) { background: var(--accent-soft); color: var(--accent); }
    .filter[aria-pressed="true"] { background: var(--surface); color: var(--ink); box-shadow: 0 1px 2px rgba(23, 33, 43, 0.12); }
    .filter:disabled { cursor: not-allowed; opacity: 0.45; }
    .test-list { display: grid; padding: 10px; gap: 6px; }
    .test-row { display: grid; width: 100%; min-height: 84px; grid-template-columns: 10px minmax(0, 1fr); align-items: start; padding: 12px; gap: 10px; border: 1px solid transparent; border-radius: 6px; background: transparent; color: inherit; text-align: left; cursor: pointer; }
    .test-row:hover { border-color: var(--line); background: var(--surface-raised); }
    .test-row[aria-current="true"] { border-color: color-mix(in srgb, var(--accent) 45%, var(--line)); background: var(--accent-soft); }
    .status-dot { width: 10px; height: 10px; margin-top: 4px; border-radius: 50%; background: var(--muted); }
    .status-dot.pass { background: var(--pass); }
    .status-dot.fail { background: var(--fail); }
    .test-id { overflow-wrap: anywhere; font-size: 13px; font-weight: 750; }
    .test-description { display: -webkit-box; overflow: hidden; margin-top: 5px; color: var(--muted); font-size: 12px; line-height: 1.35; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
    .test-runtime { margin-top: 7px; color: var(--muted); font-size: 11px; }
    .detail { min-width: 0; padding: 24px; }
    .detail-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 18px; }
    .detail h2 { margin: 0; overflow-wrap: anywhere; font-size: 24px; letter-spacing: 0; }
    .detail-lead { max-width: 760px; margin: 8px 0 0; color: var(--muted); font-size: 14px; line-height: 1.55; }
    .badge { flex: 0 0 auto; padding: 6px 9px; border-radius: 4px; font-size: 11px; font-weight: 800; }
    .badge.pass { background: var(--pass-soft); color: var(--pass); }
    .badge.fail { background: var(--fail-soft); color: var(--fail); }
    .detail-metrics { display: grid; grid-template-columns: repeat(4, minmax(110px, 1fr)); margin: 22px 0; border: 1px solid var(--line); border-radius: 6px; background: var(--surface); }
    .detail-metric { min-height: 70px; padding: 13px; border-right: 1px solid var(--line); }
    .detail-metric:last-child { border-right: 0; }
    .detail-metric span { display: block; color: var(--muted); font-size: 11px; }
    .detail-metric strong { display: block; overflow-wrap: anywhere; margin-top: 6px; font-size: 15px; }
    .failure-box { display: none; margin-bottom: 18px; padding: 14px 16px; border: 1px solid color-mix(in srgb, var(--fail) 30%, var(--line)); border-radius: 6px; background: var(--fail-soft); color: var(--fail); }
    .failure-box.visible { display: block; }
    .failure-box h3 { margin: 0 0 8px; font-size: 13px; }
    .failure-box ul { margin: 0; padding-left: 20px; font-size: 13px; line-height: 1.5; }
    .tabs { display: flex; overflow-x: auto; border-bottom: 1px solid var(--line); gap: 4px; }
    .tab { min-height: 42px; padding: 0 14px; border: 0; border-bottom: 3px solid transparent; background: transparent; color: var(--muted); cursor: pointer; font-size: 13px; font-weight: 700; white-space: nowrap; }
    .tab:hover { color: var(--accent); }
    .tab[aria-selected="true"] { border-bottom-color: var(--accent); color: var(--accent); }
    .panel { display: none; padding-top: 18px; }
    .panel.active { display: block; }
    .section { margin-bottom: 22px; }
    .section h3 { margin: 0 0 9px; font-size: 13px; }
    .message { margin: 0; padding: 14px 16px; border-left: 4px solid var(--accent); background: var(--surface); font-size: 14px; line-height: 1.55; }
    .items { width: 100%; border-collapse: collapse; border: 1px solid var(--line); background: var(--surface); font-size: 12px; }
    .items th, .items td { padding: 10px 12px; border-bottom: 1px solid var(--line); text-align: left; vertical-align: top; }
    .items th { background: var(--surface-raised); color: var(--muted); font-weight: 700; }
    .items td:first-child { max-width: 340px; overflow-wrap: anywhere; }
    .empty { padding: 28px; border: 1px dashed var(--line); border-radius: 6px; color: var(--muted); text-align: center; }
    .code-label { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; gap: 12px; color: var(--muted); font-size: 11px; }
    pre { max-height: 560px; overflow: auto; margin: 0; padding: 16px; border-radius: 6px; background: var(--code); color: var(--code-text); font: 12px/1.55 "Cascadia Code", "SFMono-Regular", Consolas, monospace; tab-size: 2; white-space: pre; }
    .http-block + .http-block { margin-top: 18px; }
    .http-status { display: inline-block; padding: 3px 6px; border-radius: 3px; background: var(--warning-soft); color: var(--warning); font-size: 11px; font-weight: 800; }
    @media (max-width: 900px) {
      .summary { grid-template-columns: repeat(2, 1fr); }
      .metric:nth-child(2) { border-right: 0; }
      .metric:nth-child(-n+2) { border-bottom: 1px solid var(--line); }
      .workspace { grid-template-columns: 1fr; }
      .sidebar { border-right: 0; border-bottom: 1px solid var(--line); }
      .sidebar-tools { position: static; }
      .test-list { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    }
    @media (max-width: 620px) {
      .topbar { align-items: flex-start; padding: 14px 16px; flex-direction: column; }
      .run-meta { text-align: left; }
      .summary { grid-template-columns: 1fr 1fr; }
      .metric { padding: 12px 16px; }
      .test-list { grid-template-columns: 1fr; }
      .detail { padding: 20px 16px; }
      .detail-head { flex-direction: column-reverse; }
      .detail-metrics { grid-template-columns: 1fr 1fr; }
      .detail-metric:nth-child(2) { border-right: 0; }
      .detail-metric:nth-child(-n+2) { border-bottom: 1px solid var(--line); }
      .items { display: block; overflow-x: auto; white-space: nowrap; }
    }
    @media (prefers-reduced-motion: reduce) { * { scroll-behavior: auto !important; } }
  </style>
</head>
<body>
  <div class="app">
    <header class="topbar">
      <div class="brand"><h1>Reporte QA del motor RFQ</h1><p>Extracción, matching y cotización desde WhatsApp</p></div>
      <div class="run-meta"><div id="run-time"></div><div id="api-url"></div></div>
    </header>
    <section class="summary" aria-label="Resumen de la ejecución">
      <div class="metric"><div class="metric-label">Casos ejecutados</div><div class="metric-value" id="total"></div></div>
      <div class="metric"><div class="metric-label">Aprobados</div><div class="metric-value pass" id="passed"></div></div>
      <div class="metric"><div class="metric-label">Fallidos</div><div class="metric-value fail" id="failed"></div></div>
      <div class="metric"><div class="metric-label">Duración total</div><div class="metric-value" id="duration"></div></div>
    </section>
    <main class="workspace">
      <aside class="sidebar">
        <div class="sidebar-tools">
          <div class="filters" role="group" aria-label="Filtrar resultados">
            <button class="filter" data-filter="all" aria-pressed="true">Todos</button>
            <button class="filter" data-filter="pass" aria-pressed="false">OK</button>
            <button class="filter" data-filter="fail" aria-pressed="false">Fallidos</button>
          </div>
        </div>
        <div class="test-list" id="test-list"></div>
      </aside>
      <article class="detail" id="detail">
        <div class="detail-head"><div><h2 id="case-id"></h2><p class="detail-lead" id="case-description"></p></div><span class="badge" id="case-status"></span></div>
        <div class="detail-metrics">
          <div class="detail-metric"><span>Duración</span><strong id="case-duration"></strong></div>
          <div class="detail-metric"><span>Ítems</span><strong id="case-items"></strong></div>
          <div class="detail-metric"><span>RFQ</span><strong id="case-rfq"></strong></div>
          <div class="detail-metric"><span>Cotización</span><strong id="case-quote"></strong></div>
        </div>
        <div class="failure-box" id="failure-box"><h3>Validaciones que fallaron</h3><ul id="failure-list"></ul></div>
        <div class="tabs" role="tablist">
          <button class="tab" role="tab" data-tab="overview" aria-selected="true">Resumen</button>
          <button class="tab" role="tab" data-tab="expected" aria-selected="false">Qué se valida</button>
          <button class="tab" role="tab" data-tab="response" aria-selected="false">Respuesta</button>
          <button class="tab" role="tab" data-tab="code" aria-selected="false">Código del caso</button>
        </div>
        <section class="panel active" data-panel="overview">
          <div class="section"><h3>Mensaje recibido por WhatsApp</h3><p class="message" id="case-message"></p></div>
          <div class="section"><h3>Ítems extraídos</h3><div id="items-view"></div></div>
        </section>
        <section class="panel" data-panel="expected"><div class="code-label"><span>Expectativas declaradas</span></div><pre id="expected-code"></pre></section>
        <section class="panel" data-panel="response"><div id="response-view"></div></section>
        <section class="panel" data-panel="code"><div class="code-label"><span id="source-path"></span><span>JSON</span></div><pre id="case-code"></pre></section>
      </article>
    </main>
  </div>
  <script>
    const report = JSON.parse(new TextDecoder().decode(Uint8Array.from(atob('${encoded}'), c => c.charCodeAt(0))));
    let selected = 0;
    let filter = 'all';
    const $ = selector => document.querySelector(selector);
    const results = report.results || [];
    const formatJSON = value => JSON.stringify(value ?? null, null, 2);
    const totalDuration = results.reduce((sum, item) => sum + (item.duration_ms || 0), 0);

    $('#total').textContent = report.summary?.total ?? results.length;
    $('#passed').textContent = report.summary?.passed ?? results.filter(item => item.passed).length;
    $('#failed').textContent = report.summary?.failed ?? results.filter(item => !item.passed).length;
    $('#duration').textContent = totalDuration >= 1000 ? (totalDuration / 1000).toFixed(1) + ' s' : totalDuration + ' ms';
    $('#run-time').textContent = 'Ejecución: ' + new Date(report.started_at).toLocaleString('es-AR');
    $('#api-url').textContent = report.api_url || 'Reporte local';
    const passCount = results.filter(item => item.passed).length;
    const failCount = results.length - passCount;
    const passFilter = document.querySelector('[data-filter="pass"]');
    const failFilter = document.querySelector('[data-filter="fail"]');
    passFilter.textContent = 'OK (' + passCount + ')';
    failFilter.textContent = 'Fallidos (' + failCount + ')';
    passFilter.disabled = passCount === 0;
    failFilter.disabled = failCount === 0;

    function renderList() {
      const list = $('#test-list');
      list.replaceChildren();
      results.forEach((result, index) => {
        if (filter !== 'all' && (result.passed ? 'pass' : 'fail') !== filter) return;
        const button = document.createElement('button');
        button.className = 'test-row';
        button.type = 'button';
        button.setAttribute('aria-current', String(index === selected));
        const dot = document.createElement('span');
        dot.className = 'status-dot ' + (result.passed ? 'pass' : 'fail');
        const content = document.createElement('span');
        const id = document.createElement('span');
        id.className = 'test-id';
        id.textContent = result.id;
        const description = document.createElement('span');
        description.className = 'test-description';
        description.textContent = result.description || 'Sin descripción';
        const runtime = document.createElement('span');
        runtime.className = 'test-runtime';
        runtime.textContent = result.duration_ms + ' ms';
        content.append(id, description, runtime);
        button.append(dot, content);
        button.addEventListener('click', () => { selected = index; renderList(); renderDetail(); });
        list.append(button);
      });
    }

    function renderItems(items) {
      const root = $('#items-view');
      root.replaceChildren();
      if (!items.length) {
        const empty = document.createElement('div');
        empty.className = 'empty';
        empty.textContent = 'El mensaje no produjo materiales.';
        root.append(empty);
        return;
      }
      const table = document.createElement('table');
      table.className = 'items';
      const head = document.createElement('thead');
      head.innerHTML = '<tr><th>Descripción</th><th>Cantidad</th><th>Unidad</th><th>Matching</th><th>Confianza</th><th>Subtotal</th></tr>';
      const body = document.createElement('tbody');
      items.forEach(item => {
        const row = document.createElement('tr');
        [item.requested_description, item.quantity, item.unit ?? '—', item.match_status, item.confidence_score ?? '—', item.subtotal ?? 'Sin precio'].forEach(value => {
          const cell = document.createElement('td');
          cell.textContent = value;
          row.append(cell);
        });
        body.append(row);
      });
      table.append(head, body);
      root.append(table);
    }

    function responseBlock(title, response) {
      const block = document.createElement('div');
      block.className = 'http-block';
      const label = document.createElement('div');
      label.className = 'code-label';
      const name = document.createElement('span');
      name.textContent = title;
      const status = document.createElement('span');
      status.className = 'http-status';
      status.textContent = response ? 'HTTP ' + response.status : 'No ejecutado';
      label.append(name, status);
      const code = document.createElement('pre');
      code.textContent = formatJSON(response?.body ?? null);
      block.append(label, code);
      return block;
    }

    function renderDetail() {
      const result = results[selected];
      if (!result) return;
      const definition = result.definition || {};
      $('#case-id').textContent = result.id;
      $('#case-description').textContent = result.description || 'Sin descripción';
      const badge = $('#case-status');
      badge.textContent = result.passed ? 'APROBADO' : 'FALLÓ';
      badge.className = 'badge ' + (result.passed ? 'pass' : 'fail');
      $('#case-duration').textContent = result.duration_ms + ' ms';
      const items = result.pricing?.body?.items ?? result.draft?.body?.items ?? [];
      $('#case-items').textContent = String(items.length);
      $('#case-rfq').textContent = result.draft?.body?.rfq?.status ?? '—';
      $('#case-quote').textContent = result.pricing?.body?.quote?.current_status ?? result.draft?.body?.quote?.current_status ?? 'Sin cotización';
      $('#case-message').textContent = result.message;
      renderItems(items);

      const failureBox = $('#failure-box');
      const failureList = $('#failure-list');
      failureList.replaceChildren();
      failureBox.classList.toggle('visible', (result.failures || []).length > 0);
      (result.failures || []).forEach(failure => { const item = document.createElement('li'); item.textContent = failure; failureList.append(item); });

      $('#expected-code').textContent = formatJSON(definition.expected ?? null);
      $('#case-code').textContent = formatJSON(definition || { id: result.id, description: result.description, message: result.message });
      $('#source-path').textContent = report.cases_file || 'scripts/fixtures/rfq-eval-cases.json';
      const response = $('#response-view');
      response.replaceChildren(responseBlock('Creación del draft', result.draft), responseBlock('Aceptación de materiales y pricing', result.pricing));
    }

    document.querySelectorAll('.filter').forEach(button => button.addEventListener('click', () => {
      filter = button.dataset.filter;
      document.querySelectorAll('.filter').forEach(item => item.setAttribute('aria-pressed', String(item === button)));
      const visible = results.findIndex(result => filter === 'all' || (result.passed ? 'pass' : 'fail') === filter);
      if (visible >= 0) selected = visible;
      renderList();
      renderDetail();
    }));
    document.querySelectorAll('.tab').forEach(button => button.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach(item => item.setAttribute('aria-selected', String(item === button)));
      document.querySelectorAll('.panel').forEach(panel => panel.classList.toggle('active', panel.dataset.panel === button.dataset.tab));
    }));
    renderList();
    renderDetail();
  </script>
</body>
</html>`;
}

// writeRFQDashboard writes the report beside the machine-readable JSON artifact.
export function writeRFQDashboard(report, definitions, outputPath) {
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  fs.writeFileSync(outputPath, `${renderRFQDashboard(report, definitions)}\n`);
  return outputPath;
}
