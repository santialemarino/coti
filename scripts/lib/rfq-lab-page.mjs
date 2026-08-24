// renderRFQLab returns the local QA workspace. Runtime data arrives from same-origin endpoints.
export function renderRFQLab() {
  return `<!doctype html>
<html lang="es-AR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Coti QA Lab</title>
  <style>
    :root {
      color-scheme: light;
      --page: #f4f6f8;
      --surface: #ffffff;
      --surface-soft: #f8fafb;
      --ink: #17212b;
      --muted: #5b6875;
      --line: #d8dee5;
      --accent: #155d88;
      --accent-soft: #e5f1f8;
      --pass: #167449;
      --pass-soft: #e7f5ed;
      --fail: #b42318;
      --fail-soft: #fdecea;
      --warning: #8a5a00;
      --warning-soft: #fff3d6;
      --code: #101820;
      --code-text: #dce6ee;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    * { box-sizing: border-box; }
    [hidden] { display: none !important; }
    body { margin: 0; background: var(--page); color: var(--ink); }
    button, input, select, textarea { font: inherit; }
    button:focus-visible, input:focus-visible, select:focus-visible, textarea:focus-visible, a:focus-visible { outline: 3px solid color-mix(in srgb, var(--accent) 30%, transparent); outline-offset: 2px; }
    button { cursor: pointer; }
    button:disabled { cursor: not-allowed; opacity: .5; }
    .topbar { display: flex; min-height: 68px; align-items: center; justify-content: space-between; padding: 12px 22px; gap: 18px; border-bottom: 1px solid var(--line); background: var(--surface); }
    .brand h1 { margin: 0; font-size: 19px; letter-spacing: 0; }
    .brand p { margin: 3px 0 0; color: var(--muted); font-size: 12px; }
    .top-actions { display: flex; align-items: center; gap: 10px; }
    .api-state { display: flex; align-items: center; gap: 7px; color: var(--muted); font-size: 12px; }
    .api-dot { width: 9px; height: 9px; border-radius: 50%; background: var(--fail); }
    .api-dot.online { background: var(--pass); }
    .button { display: inline-flex; min-height: 38px; align-items: center; justify-content: center; padding: 0 14px; border: 1px solid var(--line); border-radius: 6px; background: var(--surface); color: var(--ink); text-decoration: none; font-size: 13px; font-weight: 700; }
    .button:hover:not(:disabled) { border-color: var(--accent); color: var(--accent); }
    .button.primary { border-color: var(--accent); background: var(--accent); color: white; }
    .button.primary:hover:not(:disabled) { background: #104d72; color: white; }
    .workspace { display: grid; min-height: calc(100vh - 68px); grid-template-columns: 300px minmax(0, 1fr); }
    .sidebar { border-right: 1px solid var(--line); background: var(--surface); }
    .sidebar-head { padding: 18px 18px 10px; }
    .sidebar-head h2 { margin: 0; font-size: 13px; }
    .sidebar-head p { margin: 5px 0 0; color: var(--muted); font-size: 11px; line-height: 1.4; }
    .type-list { display: grid; padding: 8px 10px 18px; gap: 4px; }
    .type-button { display: grid; width: 100%; min-height: 62px; grid-template-columns: minmax(0, 1fr) auto; align-items: center; padding: 10px 12px; gap: 10px; border: 1px solid transparent; border-radius: 6px; background: transparent; color: inherit; text-align: left; }
    .type-button:hover { border-color: var(--line); background: var(--surface-soft); }
    .type-button[aria-current="true"] { border-color: color-mix(in srgb, var(--accent) 42%, var(--line)); background: var(--accent-soft); }
    .type-name { display: block; font-size: 13px; font-weight: 750; }
    .type-component { display: block; margin-top: 4px; color: var(--muted); font-size: 11px; }
    .ai-mark { padding: 3px 5px; border-radius: 3px; background: var(--warning-soft); color: var(--warning); font-size: 9px; font-weight: 850; }
    .blocked-mark { padding: 3px 5px; border-radius: 3px; background: var(--fail-soft); color: var(--fail); font-size: 9px; font-weight: 850; }
    .content { min-width: 0; }
    .selection { padding: 24px clamp(18px, 4vw, 46px); border-bottom: 1px solid var(--line); background: var(--surface); }
    .selection-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; }
    .selection h2 { margin: 0; font-size: 24px; letter-spacing: 0; }
    .selection-description { max-width: 760px; margin: 8px 0 0; color: var(--muted); font-size: 14px; line-height: 1.55; }
    .badges { display: flex; flex-wrap: wrap; margin-top: 16px; gap: 7px; }
    .badge { padding: 5px 7px; border: 1px solid var(--line); border-radius: 4px; background: var(--surface-soft); color: var(--muted); font-size: 10px; font-weight: 800; }
    .badge.ai { border-color: color-mix(in srgb, var(--warning) 30%, var(--line)); background: var(--warning-soft); color: var(--warning); }
    .warning { display: none; margin-top: 18px; padding: 12px 14px; border-left: 4px solid var(--warning); background: var(--warning-soft); color: var(--warning); font-size: 12px; line-height: 1.5; }
    .warning.visible { display: block; }
    .source-list { margin: 14px 0 0; color: var(--muted); font: 11px/1.6 "Cascadia Code", Consolas, monospace; }
    .body { padding: 24px clamp(18px, 4vw, 46px) 48px; }
    .section { margin-bottom: 28px; }
    .section-title { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 12px; gap: 16px; }
    .section-title h3 { margin: 0; font-size: 14px; }
    .section-title span { color: var(--muted); font-size: 11px; }
    .preflight-head { display: flex; align-items: center; gap: 9px; }
    .preflight-state { padding: 4px 7px; border-radius: 4px; font-size: 10px; font-weight: 850; }
    .preflight-state.READY { background: var(--pass-soft); color: var(--pass); }
    .preflight-state.BLOCKED { background: var(--fail-soft); color: var(--fail); }
    .preflight-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); border-top: 1px solid var(--line); }
    .preflight-check { display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: start; padding: 12px 10px; gap: 9px; border-bottom: 1px solid var(--line); }
    .check-dot { width: 9px; height: 9px; margin-top: 4px; border-radius: 50%; background: var(--fail); }
    .check-dot.READY { background: var(--pass); }
    .check-label { display: block; font-size: 12px; font-weight: 750; }
    .check-detail { display: block; margin-top: 3px; color: var(--muted); font-size: 11px; line-height: 1.45; }
    .preflight-actions { display: flex; justify-content: flex-end; margin-top: 12px; }
    .case-builder { display: none; }
    .case-builder.visible { display: block; }
    .form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px 18px; }
    .field { display: grid; gap: 6px; }
    .field.full { grid-column: 1 / -1; }
    .field label { color: var(--muted); font-size: 11px; font-weight: 750; }
    .field input, .field select, .field textarea { width: 100%; border: 1px solid var(--line); border-radius: 5px; background: var(--surface); color: var(--ink); }
    .field input, .field select { min-height: 40px; padding: 0 10px; }
    .field textarea { min-height: 100px; padding: 10px; resize: vertical; line-height: 1.5; }
    .field input:hover, .field select:hover, .field textarea:hover { border-color: color-mix(in srgb, var(--accent) 45%, var(--line)); }
    .check-field { display: flex; min-height: 40px; align-items: center; gap: 9px; }
    .check-field input { width: 16px; height: 16px; accent-color: var(--accent); }
    .form-actions { display: flex; justify-content: flex-end; margin-top: 16px; gap: 10px; }
    .run-controls { display: grid; grid-template-columns: minmax(220px, 1fr) auto; align-items: end; gap: 14px; }
    .saved-case { display: none; }
    .saved-case.visible { display: grid; }
    .run-status { display: none; align-items: center; justify-content: space-between; margin-bottom: 10px; gap: 14px; }
    .run-status.visible { display: flex; }
    .status-label { font-size: 12px; font-weight: 800; }
    .status-label.PASSED { color: var(--pass); }
    .status-label.FAILED { color: var(--fail); }
    .status-label.RUNNING { color: var(--accent); }
    .test-summary { display: flex; flex-wrap: wrap; margin: 14px 0 10px; gap: 8px; color: var(--muted); font-size: 11px; }
    .summary-count { padding: 5px 7px; border: 1px solid var(--line); border-radius: 4px; background: var(--surface); font-weight: 750; }
    .test-results { display: grid; margin-bottom: 14px; border-top: 1px solid var(--line); }
    .test-result { border-bottom: 1px solid var(--line); background: var(--surface); }
    .test-result summary { display: grid; min-height: 52px; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; padding: 8px 10px; gap: 10px; cursor: pointer; list-style: none; }
    .test-result summary::-webkit-details-marker { display: none; }
    .test-result summary:hover { background: var(--surface-soft); }
    .test-status { min-width: 72px; padding: 4px 6px; border-radius: 4px; text-align: center; font-size: 9px; font-weight: 850; }
    .test-status.PASSED { background: var(--pass-soft); color: var(--pass); }
    .test-status.FAILED { background: var(--fail-soft); color: var(--fail); }
    .test-status.RUNNING { background: var(--accent-soft); color: var(--accent); }
    .test-status.SKIPPED { background: var(--surface-soft); color: var(--muted); }
    .test-name { min-width: 0; }
    .test-name strong { display: block; overflow-wrap: anywhere; font-size: 12px; }
    .test-name span { display: block; margin-top: 3px; color: var(--muted); font-size: 11px; }
    .test-duration { color: var(--muted); font: 11px "Cascadia Code", Consolas, monospace; }
    .test-detail { padding: 0 12px 14px; }
    .test-meta { margin: 0 0 10px; color: var(--muted); font: 11px/1.5 "Cascadia Code", Consolas, monospace; overflow-wrap: anywhere; }
    .test-detail h4 { margin: 12px 0 6px; font-size: 11px; }
    .code-block { max-height: 360px; overflow: auto; margin: 0; padding: 13px; border-radius: 5px; background: var(--code); color: var(--code-text); font: 11px/1.55 "Cascadia Code", Consolas, monospace; white-space: pre; }
    .terminal { min-height: 280px; max-height: 520px; overflow: auto; margin: 0; padding: 16px; border-radius: 6px; background: var(--code); color: var(--code-text); font: 12px/1.55 "Cascadia Code", Consolas, monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
    .terminal .stderr { color: #ffb4aa; }
    .empty { padding: 34px 20px; border: 1px dashed var(--line); border-radius: 6px; color: var(--muted); text-align: center; font-size: 13px; }
    .reports { display: grid; border-top: 1px solid var(--line); }
    .report-row { display: grid; grid-template-columns: minmax(0, 1fr) repeat(3, 80px) auto; align-items: center; min-height: 54px; padding: 8px 10px; gap: 12px; border-bottom: 1px solid var(--line); background: var(--surface); font-size: 12px; }
    .report-date { color: var(--muted); }
    .report-pass { color: var(--pass); font-weight: 800; }
    .report-fail { color: var(--fail); font-weight: 800; }
    dialog { width: min(520px, calc(100vw - 32px)); padding: 0; border: 1px solid var(--line); border-radius: 7px; background: var(--surface); color: var(--ink); box-shadow: 0 18px 60px rgba(23, 33, 43, .24); }
    dialog::backdrop { background: rgba(23, 33, 43, .42); }
    .dialog-body { padding: 22px; }
    .dialog-body h2 { margin: 0; font-size: 18px; }
    .dialog-body p { margin: 10px 0 0; color: var(--muted); font-size: 13px; line-height: 1.55; }
    .dialog-cost { margin-top: 16px; padding: 12px; border-left: 4px solid var(--warning); background: var(--warning-soft); color: var(--warning); font-size: 12px; line-height: 1.5; }
    .dialog-actions { display: flex; justify-content: flex-end; padding: 14px 22px; gap: 10px; border-top: 1px solid var(--line); background: var(--surface-soft); }
    .toast { position: fixed; right: 18px; bottom: 18px; z-index: 5; max-width: min(420px, calc(100vw - 36px)); padding: 11px 14px; border-radius: 5px; background: var(--ink); color: white; font-size: 12px; opacity: 0; transform: translateY(8px); pointer-events: none; transition: opacity 150ms ease, transform 150ms ease; }
    .toast.visible { opacity: 1; transform: translateY(0); }
    @media (max-width: 840px) {
      .workspace { grid-template-columns: 1fr; }
      .sidebar { border-right: 0; border-bottom: 1px solid var(--line); }
      .type-list { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      .selection-head { flex-direction: column; }
    }
    @media (max-width: 620px) {
      .topbar { align-items: flex-start; padding: 12px 16px; flex-direction: column; }
      .top-actions { width: 100%; justify-content: space-between; }
      .type-list, .form-grid { grid-template-columns: 1fr; }
      .preflight-grid { grid-template-columns: 1fr; }
      .field.full { grid-column: auto; }
      .run-controls { grid-template-columns: 1fr; }
      .run-controls .button { width: 100%; }
      .report-row { grid-template-columns: minmax(0, 1fr) auto; }
      .report-row > :nth-child(2), .report-row > :nth-child(3) { display: none; }
      .test-result summary { grid-template-columns: auto minmax(0, 1fr); }
      .test-duration { grid-column: 2; }
    }
    @media (prefers-reduced-motion: reduce) { .toast { transition: none; } }
  </style>
</head>
<body>
  <header class="topbar">
    <div class="brand"><h1>Coti QA Lab</h1><p>Motor RFQ · pruebas locales y evaluaciones con proveedores</p></div>
    <div class="top-actions">
      <div class="api-state"><span class="api-dot" id="api-dot"></span><span id="api-label">Consultando API</span></div>
      <a class="button" id="latest-report" href="/latest">Último reporte</a>
    </div>
  </header>
  <main class="workspace">
    <aside class="sidebar">
      <div class="sidebar-head"><h2>Superficie a probar</h2><p>Cada opción ejecuta un comando cerrado del repositorio.</p></div>
      <div class="type-list" id="type-list"></div>
    </aside>
    <div class="content">
      <section class="selection">
        <div class="selection-head">
          <div><h2 id="type-title"></h2><p class="selection-description" id="type-description"></p></div>
          <button class="button primary" id="run-top" type="button">Ejecutar test</button>
        </div>
        <div class="badges" id="badges"></div>
        <div class="warning" id="ai-warning"></div>
        <div class="source-list" id="source-list"></div>
      </section>
      <div class="body">
        <section class="section" id="preflight-section">
          <div class="section-title">
            <div class="preflight-head"><h3>Preparación</h3><span class="preflight-state" id="preflight-state"></span></div>
            <span id="preflight-time"></span>
          </div>
          <div class="preflight-grid" id="preflight-checks"></div>
          <div class="preflight-actions"><button class="button" id="refresh-preflight" type="button">Volver a verificar</button></div>
        </section>
        <section class="section case-builder" id="case-builder">
          <div class="section-title"><h3>Crear caso WhatsApp</h3><span>Se guarda localmente en .artifacts</span></div>
          <form id="case-form">
            <div class="form-grid">
              <div class="field"><label for="case-name">Nombre del test</label><input id="case-name" name="name" maxlength="80" required></div>
              <div class="field"><label for="case-description">Descripción</label><input id="case-description" name="description" maxlength="240"></div>
              <div class="field full"><label for="case-message">Mensaje mockeado de WhatsApp</label><textarea id="case-message" name="message" maxlength="4000" required placeholder="Ej.: Necesito 12 bolsas de cemento y 2 m3 de arena fina"></textarea></div>
              <div class="field"><label for="item-count">Cantidad esperada de ítems</label><input id="item-count" name="item_count" type="number" min="0" max="50" value="1" required></div>
              <div class="field"><label for="rfq-status">Estado esperado del RFQ</label><select id="rfq-status" name="rfq_status"><option>GENERATED</option><option>RECEIVED</option></select></div>
              <div class="field"><label for="quote-status">Estado esperado de la cotización</label><select id="quote-status" name="quote_status"><option>DRAFT</option><option>NONE</option></select></div>
              <div class="field"><label for="first-description">Primer ítem contiene</label><input id="first-description" name="first_description_contains" maxlength="160" placeholder="cemento"></div>
              <div class="field"><label for="first-quantity">Cantidad del primer ítem</label><input id="first-quantity" name="first_quantity" inputmode="decimal" placeholder="12.00"></div>
              <div class="field"><label for="first-unit">Unidad del primer ítem contiene</label><input id="first-unit" name="first_unit_contains" maxlength="64" placeholder="bolsa"></div>
              <div class="field"><label for="first-match">Matching del primer ítem</label><select id="first-match" name="first_match_status"><option value="">No validar</option><option>MATCHED</option><option>AMBIGUOUS</option><option>NO_MATCH</option></select></div>
              <div class="field full"><label class="check-field"><input name="price_after_draft" type="checkbox">Aceptar materiales y validar pricing después del draft</label></div>
            </div>
            <div class="form-actions"><button class="button" type="submit">Guardar caso</button></div>
          </form>
        </section>
        <section class="section">
          <div class="section-title"><h3>Ejecución</h3><span id="run-help"></span></div>
          <div class="run-controls">
            <div class="field saved-case" id="saved-case-field"><label for="saved-case">Caso guardado</label><select id="saved-case"></select></div>
            <button class="button primary" id="run-main" type="button">Ejecutar test</button>
          </div>
        </section>
        <section class="section">
          <div class="run-status" id="run-status"><span class="status-label" id="status-label"></span><a class="button" id="run-report" href="#" hidden>Abrir reporte</a></div>
          <div class="test-summary" id="test-summary" hidden></div>
          <div class="test-results" id="test-results" hidden></div>
          <pre class="terminal" id="terminal" aria-live="polite">Seleccioná una superficie y ejecutá el test para ver la salida.</pre>
        </section>
        <section class="section">
          <div class="section-title"><h3>Reportes recientes</h3><span>Las evaluaciones con IA incluyen trazabilidad por etapa</span></div>
          <div class="reports" id="reports"></div>
        </section>
      </div>
    </div>
  </main>
  <dialog id="ai-dialog">
    <div class="dialog-body"><h2>Esta ejecución consume APIs de IA</h2><p id="dialog-description"></p><div class="dialog-cost" id="dialog-cost"></div></div>
    <div class="dialog-actions"><button class="button" id="cancel-ai" type="button">Cancelar</button><button class="button primary" id="confirm-ai" type="button">Confirmar y ejecutar</button></div>
  </dialog>
  <div class="toast" id="toast" role="status"></div>
  <script>
    let state = { types: [], cases: [], reports: [], api_online: false, preflights: {} };
    let selectedType = null;
    let activeRun = null;
    let pollTimer = null;
    const $ = selector => document.querySelector(selector);

    async function request(url, options) {
      const response = await fetch(url, options);
      const body = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(body.error || 'La operación no pudo completarse');
      return body;
    }

    async function loadState(force = false) {
      state = await request(force ? '/api/state?refresh=1' : '/api/state');
      selectedType = state.types.find(type => type.id === selectedType?.id) || state.types[0];
      render();
    }

    async function refreshPreflight() {
      const button = $('#refresh-preflight');
      button.disabled = true;
      button.textContent = 'Verificando…';
      try {
        await loadState(true);
      } catch (error) {
        showToast(error.message);
      } finally {
        button.disabled = false;
        button.textContent = 'Volver a verificar';
      }
    }

    function render() {
      renderTypes();
      renderSelection();
      renderPreflight();
      renderCases();
      renderReports();
      $('#api-dot').classList.toggle('online', state.api_online);
      $('#api-label').textContent = state.api_online ? 'API RFQ disponible' : 'API RFQ sin conexión';
      $('#latest-report').hidden = state.reports.length === 0;
    }

    function renderTypes() {
      const root = $('#type-list');
      root.replaceChildren();
      state.types.forEach(type => {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'type-button';
        button.setAttribute('aria-current', String(type.id === selectedType.id));
        const text = document.createElement('span');
        const name = document.createElement('span');
        name.className = 'type-name';
        name.textContent = type.label;
        const component = document.createElement('span');
        component.className = 'type-component';
        component.textContent = type.component;
        text.append(name, component);
        button.append(text);
        const preflight = state.preflights[type.id];
        if (preflight && !preflight.ready) {
          const mark = document.createElement('span');
          mark.className = 'blocked-mark';
          mark.textContent = 'BLOQUEADO';
          button.append(mark);
        } else if (type.uses_ai) {
          const mark = document.createElement('span');
          mark.className = 'ai-mark';
          mark.textContent = 'USA IA';
          button.append(mark);
        }
        button.addEventListener('click', () => { selectedType = type; render(); });
        root.append(button);
      });
    }

    function renderSelection() {
      $('#type-title').textContent = selectedType.label;
      $('#type-description').textContent = selectedType.description;
      const badges = $('#badges');
      badges.replaceChildren(
        badge(selectedType.uses_ai ? 'IA: sí' : 'IA: no', selectedType.uses_ai),
        badge(selectedType.requires_database ? 'Base de datos: sí' : 'Base de datos: no'),
        badge(selectedType.requires_api ? 'API local: requerida' : 'API local: no requerida'),
      );
      const warning = $('#ai-warning');
      warning.classList.toggle('visible', selectedType.uses_ai);
      warning.textContent = selectedType.uses_ai
        ? 'Esta prueba realiza llamadas reales a ' + selectedType.providers.join(' y ') + '. Antes de ejecutarla se pedirá confirmación.'
        : 'Esta prueba usa dobles determinísticos y no consume Anthropic ni OpenAI.';
      const source = $('#source-list');
      source.replaceChildren();
      selectedType.source_files.forEach(file => {
        const line = document.createElement('div');
        line.textContent = file;
        source.append(line);
      });
      $('#case-builder').classList.toggle('visible', selectedType.accepts_case);
      $('#saved-case-field').classList.toggle('visible', selectedType.accepts_case);
      const preflight = selectedPreflight();
      $('#run-help').textContent = preflight && !preflight.ready
        ? 'Hay requisitos pendientes'
        : selectedType.uses_ai ? 'Requiere confirmación de consumo' : 'Sin consumo de IA';
      setRunning(activeRun?.status === 'RUNNING');
    }

    function selectedPreflight() {
      return selectedType ? state.preflights[selectedType.id] : null;
    }

    function renderPreflight() {
      const preflight = selectedPreflight();
      const root = $('#preflight-checks');
      root.replaceChildren();
      if (!preflight) {
        $('#preflight-state').className = 'preflight-state BLOCKED';
        $('#preflight-state').textContent = 'SIN DATOS';
        $('#preflight-time').textContent = '';
        setRunning(activeRun?.status === 'RUNNING');
        return;
      }
      $('#preflight-state').className = 'preflight-state ' + (preflight.ready ? 'READY' : 'BLOCKED');
      $('#preflight-state').textContent = preflight.ready ? 'LISTO' : 'BLOQUEADO';
      $('#preflight-time').textContent = 'Verificado ' + new Date(preflight.checked_at).toLocaleTimeString('es-AR');
      preflight.checks.forEach(check => {
        const row = document.createElement('div');
        row.className = 'preflight-check';
        const dot = document.createElement('span');
        dot.className = 'check-dot ' + check.status;
        const content = document.createElement('span');
        const label = document.createElement('span');
        label.className = 'check-label';
        label.textContent = check.label;
        const detail = document.createElement('span');
        detail.className = 'check-detail';
        detail.textContent = check.detail;
        content.append(label, detail);
        row.append(dot, content);
        root.append(row);
      });
      setRunning(activeRun?.status === 'RUNNING');
    }

    function badge(text, ai = false) {
      const item = document.createElement('span');
      item.className = 'badge' + (ai ? ' ai' : '');
      item.textContent = text;
      return item;
    }

    function renderCases() {
      const select = $('#saved-case');
      const current = select.value;
      select.replaceChildren();
      if (state.cases.length === 0) {
        const option = document.createElement('option');
        option.value = '';
        option.textContent = 'Creá y guardá un caso primero';
        select.append(option);
        return;
      }
      state.cases.forEach(testCase => {
        const option = document.createElement('option');
        option.value = testCase.id;
        option.textContent = testCase.description;
        select.append(option);
      });
      if (state.cases.some(testCase => testCase.id === current)) select.value = current;
    }

    function renderReports() {
      const root = $('#reports');
      root.replaceChildren();
      if (state.reports.length === 0) {
        const empty = document.createElement('div');
        empty.className = 'empty';
        empty.textContent = 'Todavía no hay evaluaciones guardadas.';
        root.append(empty);
        return;
      }
      state.reports.slice(0, 10).forEach(report => {
        const row = document.createElement('div');
        row.className = 'report-row';
        const date = document.createElement('span');
        date.className = 'report-date';
        date.textContent = new Date(report.started_at).toLocaleString('es-AR');
        const total = document.createElement('span');
        total.textContent = report.summary.total + ' casos';
        const passed = document.createElement('span');
        passed.className = 'report-pass';
        passed.textContent = report.summary.passed + ' OK';
        const failed = document.createElement('span');
        failed.className = report.summary.failed ? 'report-fail' : 'report-pass';
        failed.textContent = report.summary.failed + ' fallidos';
        const link = document.createElement('a');
        link.className = 'button';
        link.href = report.dashboard_url;
        link.textContent = 'Abrir';
        row.append(date, total, passed, failed, link);
        root.append(row);
      });
    }

    async function saveCase(event) {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      const body = Object.fromEntries(form.entries());
      body.price_after_draft = form.has('price_after_draft');
      try {
        const saved = await request('/api/cases', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        });
        await loadState();
        $('#saved-case').value = saved.case.id;
        showToast('Caso guardado. Ya podés ejecutarlo.');
      } catch (error) {
        showToast(error.message);
      }
    }

    function requestRun() {
      if (activeRun?.status === 'RUNNING') return;
      if (!selectedPreflight()?.ready) {
        showToast('Resolvé los requisitos bloqueados antes de ejecutar.');
        return;
      }
      if (selectedType.accepts_case && !$('#saved-case').value) {
        showToast('Guardá y seleccioná un caso antes de ejecutar.');
        return;
      }
      if (selectedType.uses_ai) {
        $('#dialog-description').textContent = selectedType.label + ' realizará llamadas reales a ' + selectedType.providers.join(' y ') + '.';
        $('#dialog-cost').textContent = selectedType.id === 'live_suite'
          ? 'La suite tiene siete casos: normalmente consume una generación de Anthropic y un batch de embeddings de OpenAI por caso con materiales.'
          : 'Este caso normalmente consume una generación de Anthropic y un batch de embeddings de OpenAI si se extraen materiales.';
        $('#ai-dialog').showModal();
        return;
      }
      startRun(false);
    }

    async function startRun(confirmAI) {
      $('#ai-dialog').close();
      try {
        activeRun = await request('/api/runs', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            type_id: selectedType.id,
            case_id: selectedType.accepts_case ? $('#saved-case').value : null,
            confirm_ai: confirmAI,
          }),
        });
        setRunning(true);
        renderRun();
        pollTimer = setInterval(pollRun, 500);
      } catch (error) {
        showToast(error.message);
      }
    }

    async function pollRun() {
      try {
        activeRun = await request('/api/runs/' + activeRun.id);
        renderRun();
        if (activeRun.status !== 'RUNNING') {
          clearInterval(pollTimer);
          pollTimer = null;
          setRunning(false);
          await loadState();
          renderRun();
        }
      } catch (error) {
        clearInterval(pollTimer);
        pollTimer = null;
        setRunning(false);
        showToast(error.message);
      }
    }

    function renderRun() {
      if (!activeRun) return;
      $('#run-status').classList.add('visible');
      const label = $('#status-label');
      label.className = 'status-label ' + activeRun.status;
      label.textContent = activeRun.type_label + ' · ' + statusText(activeRun.status);
      const report = $('#run-report');
      report.hidden = !activeRun.report_url;
      if (activeRun.report_url) report.href = activeRun.report_url;
      const terminal = $('#terminal');
      terminal.replaceChildren();
      activeRun.logs.forEach(line => {
        const span = document.createElement('span');
        span.className = line.channel;
        span.textContent = line.text + '\\n';
        terminal.append(span);
      });
      terminal.scrollTop = terminal.scrollHeight;
      renderStructuredTests();
    }

    function renderStructuredTests() {
      const tests = activeRun?.tests || [];
      const summaryRoot = $('#test-summary');
      const resultsRoot = $('#test-results');
      const openTests = new Set(
        [...resultsRoot.querySelectorAll('details[open]')].map(item => item.dataset.testId),
      );
      summaryRoot.replaceChildren();
      resultsRoot.replaceChildren();
      summaryRoot.hidden = tests.length === 0;
      resultsRoot.hidden = tests.length === 0;
      if (tests.length === 0) return;

      const summary = activeRun.summary || {};
      [
        ['Total', summary.total || 0],
        ['OK', summary.passed || 0],
        ['Fallidos', summary.failed || 0],
        ['Omitidos', summary.skipped || 0],
        ['Ejecutando', summary.running || 0],
      ].forEach(([label, value]) => {
        const item = document.createElement('span');
        item.className = 'summary-count';
        item.textContent = label + ': ' + value;
        summaryRoot.append(item);
      });

      tests.forEach(test => {
        const details = document.createElement('details');
        details.className = 'test-result';
        details.dataset.testId = test.id;
        details.open = openTests.has(test.id) || test.status === 'FAILED';
        const summary = document.createElement('summary');
        const status = document.createElement('span');
        status.className = 'test-status ' + test.status;
        status.textContent = statusText(test.status);
        const name = document.createElement('span');
        name.className = 'test-name';
        const title = document.createElement('strong');
        title.textContent = test.name;
        const description = document.createElement('span');
        description.textContent = test.description;
        name.append(title, description);
        const duration = document.createElement('span');
        duration.className = 'test-duration';
        duration.textContent = test.duration_ms == null ? '—' : test.duration_ms + ' ms';
        summary.append(status, name, duration);

        const detail = document.createElement('div');
        detail.className = 'test-detail';
        const meta = document.createElement('p');
        meta.className = 'test-meta';
        meta.textContent = test.source
          ? test.source.path + ':' + test.source.line + ' · ' + test.package
          : test.package;
        detail.append(meta);
        if (test.error) {
          detail.append(sectionHeading('Error'), codeBlock(test.error));
        }
        if (test.source?.code) {
          detail.append(sectionHeading('Código relacionado'), codeBlock(test.source.code));
        }
        detail.append(
          sectionHeading('Respuesta del programa'),
          codeBlock((test.output || []).join('\\n') || 'Sin salida adicional.'),
        );
        details.append(summary, detail);
        resultsRoot.append(details);
      });
    }

    function sectionHeading(text) {
      const heading = document.createElement('h4');
      heading.textContent = text;
      return heading;
    }

    function codeBlock(text) {
      const block = document.createElement('pre');
      block.className = 'code-block';
      block.textContent = text;
      return block;
    }

    function statusText(status) {
      return {
        RUNNING: 'EJECUTANDO',
        PASSED: 'APROBADO',
        FAILED: 'FALLÓ',
        SKIPPED: 'OMITIDO',
      }[status] || status;
    }

    function setRunning(running) {
      const blocked = !selectedPreflight()?.ready;
      $('#run-main').disabled = running || blocked;
      $('#run-top').disabled = running || blocked;
      $('#run-main').textContent = running ? 'Ejecutando…' : 'Ejecutar test';
      $('#run-top').textContent = running ? 'Ejecutando…' : 'Ejecutar test';
    }

    let toastTimer;
    function showToast(message) {
      const toast = $('#toast');
      toast.textContent = message;
      toast.classList.add('visible');
      clearTimeout(toastTimer);
      toastTimer = setTimeout(() => toast.classList.remove('visible'), 3000);
    }

    $('#case-form').addEventListener('submit', saveCase);
    $('#run-main').addEventListener('click', requestRun);
    $('#run-top').addEventListener('click', requestRun);
    $('#refresh-preflight').addEventListener('click', refreshPreflight);
    $('#cancel-ai').addEventListener('click', () => $('#ai-dialog').close());
    $('#confirm-ai').addEventListener('click', () => startRun(true));
    loadState().catch(error => showToast(error.message));
  </script>
</body>
</html>`;
}
