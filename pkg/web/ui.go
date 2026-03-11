package web

// uiHTML is the complete single-page web application served at /.
// It renders a force-directed graph of the code graph using pure SVG + JS
// (no external dependencies required).
const uiHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>CodeContext Graph</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
       background: #0f1117; color: #e2e8f0; display: flex; height: 100vh; overflow: hidden; }

/* ---- Sidebar ---- */
#sidebar {
  width: 280px; min-width: 220px; max-width: 360px;
  background: #1a1d27; border-right: 1px solid #2d3148;
  display: flex; flex-direction: column; overflow: hidden;
}
#sidebar-header { padding: 16px; border-bottom: 1px solid #2d3148; }
#sidebar-header h1 { font-size: 1.1rem; font-weight: 700; color: #818cf8; }
#sidebar-header p  { font-size: 0.75rem; color: #64748b; margin-top: 2px; }

#stats { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; padding: 12px 16px;
         border-bottom: 1px solid #2d3148; }
.stat-card { background: #0f1117; border-radius: 8px; padding: 8px 10px; }
.stat-card .val { font-size: 1.4rem; font-weight: 700; color: #818cf8; }
.stat-card .lbl { font-size: 0.7rem; color: #64748b; margin-top: 1px; }

#filters { padding: 10px 16px; border-bottom: 1px solid #2d3148; }
#filters label { font-size: 0.75rem; color: #94a3b8; display: block; margin-bottom: 4px; }
#filter-input { width: 100%; background: #0f1117; border: 1px solid #2d3148; border-radius: 6px;
                color: #e2e8f0; padding: 6px 10px; font-size: 0.82rem; outline: none; }
#filter-input:focus { border-color: #818cf8; }
#type-filters { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 8px; }
.type-pill { font-size: 0.7rem; padding: 2px 8px; border-radius: 20px; cursor: pointer;
             border: 1px solid; user-select: none; transition: opacity .15s; }
.type-pill.off { opacity: 0.35; }

#detail-panel { padding: 12px 16px; border-bottom: 1px solid #2d3148; min-height: 120px; }
#detail-panel h3 { font-size: 0.78rem; color: #64748b; text-transform: uppercase; letter-spacing: .05em; margin-bottom: 8px; }
#detail-content { font-size: 0.8rem; color: #cbd5e1; line-height: 1.5; }
#detail-content .dk { color: #64748b; }
#detail-content .dv { color: #e2e8f0; }
#detail-content code { font-family: monospace; background: #0f1117; border-radius: 4px;
                       padding: 1px 5px; font-size: 0.78rem; color: #a5b4fc; }

#legend { padding: 10px 16px; }
#legend h3 { font-size: 0.78rem; color: #64748b; text-transform: uppercase; letter-spacing: .05em; margin-bottom: 8px; }
.legend-item { display: flex; align-items: center; gap: 8px; font-size: 0.78rem;
               color: #94a3b8; margin-bottom: 4px; }
.legend-dot { width: 10px; height: 10px; border-radius: 50%; flex-shrink: 0; }

/* ---- Graph area ---- */
#graph-area { flex: 1; position: relative; overflow: hidden; }
#graph-svg { width: 100%; height: 100%; cursor: grab; }
#graph-svg:active { cursor: grabbing; }

.node circle { stroke-width: 2; cursor: pointer; transition: r .15s; }
.node circle:hover { stroke-width: 3; }
.node text { pointer-events: none; user-select: none; }
.link { stroke-opacity: 0.45; stroke-width: 1; fill: none; }
.link.defines   { stroke: #818cf8; }
.link.imports   { stroke: #34d399; stroke-dasharray: 4 3; }
.link.contains  { stroke: #475569; stroke-opacity: 0.2; }

#loading { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center;
           background: #0f1117; font-size: 1rem; color: #64748b; }
#error-msg { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center;
             color: #f87171; display: none; }

#controls { position: absolute; bottom: 16px; right: 16px; display: flex; flex-direction: column; gap: 6px; }
.ctrl-btn { width: 34px; height: 34px; border-radius: 8px; background: #1a1d27;
            border: 1px solid #2d3148; color: #94a3b8; font-size: 1rem;
            cursor: pointer; display: flex; align-items: center; justify-content: center;
            transition: background .15s; }
.ctrl-btn:hover { background: #2d3148; color: #e2e8f0; }
</style>
</head>
<body>

<!-- Sidebar -->
<div id="sidebar">
  <div id="sidebar-header">
    <h1>⚡ CodeContext</h1>
    <p>Interactive code graph</p>
  </div>
  <div id="stats">
    <div class="stat-card"><div class="val" id="s-files">–</div><div class="lbl">📄 Files</div></div>
    <div class="stat-card"><div class="val" id="s-ent">–</div><div class="lbl">🧩 Entities</div></div>
    <div class="stat-card"><div class="val" id="s-rel">–</div><div class="lbl">🔗 Relations</div></div>
    <div class="stat-card"><div class="val" id="s-dep">–</div><div class="lbl">📦 Deps</div></div>
  </div>
  <div id="filters">
    <label>Search nodes</label>
    <input id="filter-input" type="text" placeholder="Filter by name…">
    <div id="type-filters"></div>
  </div>
  <div id="detail-panel">
    <h3>Selected node</h3>
    <div id="detail-content"><span style="color:#475569">Click a node for details</span></div>
  </div>
  <div id="legend">
    <h3>Legend</h3>
    <div class="legend-item"><div class="legend-dot" style="background:#818cf8"></div> defines relation</div>
    <div class="legend-item"><div class="legend-dot" style="background:#34d399"></div> imports relation</div>
    <div class="legend-item"><div class="legend-dot" style="background:#475569"></div> contains (file→entity)</div>
  </div>
</div>

<!-- Graph -->
<div id="graph-area">
  <div id="loading">⏳ Loading graph…</div>
  <div id="error-msg"></div>
  <svg id="graph-svg">
    <defs>
      <marker id="arrow-defines" markerWidth="6" markerHeight="6" refX="5" refY="3" orient="auto">
        <path d="M0,0 L6,3 L0,6 Z" fill="#818cf8" opacity="0.7"/>
      </marker>
      <marker id="arrow-imports" markerWidth="6" markerHeight="6" refX="5" refY="3" orient="auto">
        <path d="M0,0 L6,3 L0,6 Z" fill="#34d399" opacity="0.7"/>
      </marker>
    </defs>
    <g id="zoom-layer">
      <g id="links-layer"></g>
      <g id="nodes-layer"></g>
    </g>
  </svg>
  <div id="controls">
    <button class="ctrl-btn" id="btn-zoom-in"  title="Zoom in">+</button>
    <button class="ctrl-btn" id="btn-zoom-out" title="Zoom out">−</button>
    <button class="ctrl-btn" id="btn-reset"    title="Reset view">⌂</button>
    <button class="ctrl-btn" id="btn-toggle-labels" title="Toggle labels">Aa</button>
  </div>
</div>

<script>
// ============================================================
// Colour palette per entity type
// ============================================================
const COLORS = {
  file:         '#475569',
  go:           '#00ADD8',
  python:       '#3B82F6',
  javascript:   '#F59E0B',
  typescript:   '#3B82F6',
  java:         '#F97316',
  function:     '#A78BFA',
  method:       '#818CF8',
  class:        '#34D399',
  interface:    '#6EE7B7',
  struct:       '#2DD4BF',
  type:         '#67E8F9',
  constant:     '#FCA5A5',
  variable:     '#FDA4AF',
  enum:         '#FBBF24',
  annotation:   '#E879F9',
  field:        '#94A3B8',
  _default:     '#64748b',
};
function colorFor(n) {
  if (n.group === 'file') return COLORS[n.type] || COLORS.file;
  return COLORS[n.type] || COLORS._default;
}
function radiusFor(n) {
  if (n.group === 'file') return 14;
  if (n.type === 'class' || n.type === 'interface') return 11;
  return 8;
}

// ============================================================
// State
// ============================================================
let allNodes = [], allEdges = [];
let simNodes = [], simEdges = [];
let selectedId = null;
let showLabels = true;
let transform = { x: 0, y: 0, k: 1 };
let hiddenTypes = new Set();
let filterText = '';

// ============================================================
// Fetch data
// ============================================================
async function fetchData() {
  const [graphResp, statsResp] = await Promise.all([
    fetch('/api/graph'), fetch('/api/stats')
  ]);
  if (!graphResp.ok) throw new Error('Failed to load graph');
  const graph = await graphResp.json();
  const stats = await statsResp.json();
  return { graph, stats };
}

// ============================================================
// Force simulation (pure JS, no dependencies)
// ============================================================
const REPULSION  = 3500;
const LINK_DIST  = 90;
const LINK_STR   = 0.06;
const CENTER_STR = 0.005;
const DAMPING    = 0.82;
const ALPHA_MIN  = 0.005;
let alpha = 1.0;
let animFrame = null;

function initSimulation() {
  const cx = svgWidth() / 2, cy = svgHeight() / 2;
  simNodes.forEach((n, i) => {
    const angle = (i / simNodes.length) * 2 * Math.PI;
    const r = 200 + Math.random() * 100;
    n.x  = cx + r * Math.cos(angle);
    n.y  = cy + r * Math.sin(angle);
    n.vx = 0; n.vy = 0;
    n.pinned = false;
  });
  alpha = 1.0;
  if (animFrame) cancelAnimationFrame(animFrame);
  tick();
}

function tick() {
  if (alpha < ALPHA_MIN) { renderGraph(); return; }

  // Reset forces
  simNodes.forEach(n => { n.fx = 0; n.fy = 0; });

  // Repulsion (O(n²) — acceptable for typical graph sizes < 2000 nodes)
  for (let i = 0; i < simNodes.length; i++) {
    for (let j = i + 1; j < simNodes.length; j++) {
      const a = simNodes[i], b = simNodes[j];
      let dx = b.x - a.x || 0.01, dy = b.y - a.y || 0.01;
      const d2 = dx*dx + dy*dy + 1;
      const f = REPULSION / d2;
      const fx = f * dx / Math.sqrt(d2), fy = f * dy / Math.sqrt(d2);
      a.fx -= fx; a.fy -= fy;
      b.fx += fx; b.fy += fy;
    }
  }

  // Link attraction
  simEdges.forEach(e => {
    const s = e._sNode, t = e._tNode;
    if (!s || !t) return;
    const dx = t.x - s.x, dy = t.y - s.y;
    const dist = Math.sqrt(dx*dx + dy*dy) + 0.001;
    const diff = (dist - LINK_DIST) * LINK_STR;
    const fx = diff * dx / dist, fy = diff * dy / dist;
    if (!s.pinned) { s.fx += fx; s.fy += fy; }
    if (!t.pinned) { t.fx -= fx; t.fy -= fy; }
  });

  // Centre gravity
  const cx = svgWidth()/2, cy = svgHeight()/2;
  simNodes.forEach(n => {
    n.fx += (cx - n.x) * CENTER_STR;
    n.fy += (cy - n.y) * CENTER_STR;
  });

  // Integrate
  simNodes.forEach(n => {
    if (n.pinned) return;
    n.vx = (n.vx + n.fx) * DAMPING;
    n.vy = (n.vy + n.fy) * DAMPING;
    n.x += n.vx; n.y += n.vy;
  });

  alpha *= 0.992;
  renderGraph();
  animFrame = requestAnimationFrame(tick);
}

// ============================================================
// Build visible node/edge sets from allNodes/allEdges
// ============================================================
function buildSimData() {
  const lc = filterText.toLowerCase();
  simNodes = allNodes.filter(n =>
    !hiddenTypes.has(n.type) &&
    (lc === '' || n.label.toLowerCase().includes(lc))
  );
  const visIds = new Set(simNodes.map(n => n.id));
  simEdges = allEdges.filter(e => visIds.has(e.source) && visIds.has(e.target));

  // Pre-link node references
  const nodeById = {};
  simNodes.forEach(n => nodeById[n.id] = n);
  simEdges.forEach(e => {
    e._sNode = nodeById[e.source];
    e._tNode = nodeById[e.target];
  });
}

// ============================================================
// SVG rendering
// ============================================================
const svg     = document.getElementById('graph-svg');
const zoomG   = document.getElementById('zoom-layer');
const linksG  = document.getElementById('links-layer');
const nodesG  = document.getElementById('nodes-layer');

function svgWidth()  { return svg.clientWidth  || 800; }
function svgHeight() { return svg.clientHeight || 600; }

function applyTransform() {
  zoomG.setAttribute('transform', 'translate('+transform.x+','+transform.y+') scale('+transform.k+')');
}

function renderGraph() {
  // Render edges
  linksG.innerHTML = '';
  simEdges.forEach(e => {
    if (!e._sNode || !e._tNode) return;
    if (e.type === 'contains') return; // hide clutter
    const line = document.createElementNS('http://www.w3.org/2000/svg','line');
    line.setAttribute('x1', e._sNode.x); line.setAttribute('y1', e._sNode.y);
    line.setAttribute('x2', e._tNode.x); line.setAttribute('y2', e._tNode.y);
    line.setAttribute('class', 'link ' + e.type);
    if (e.type === 'defines' || e.type === 'imports') {
      line.setAttribute('marker-end', 'url(#arrow-' + e.type + ')');
    }
    linksG.appendChild(line);
  });

  // Render nodes
  nodesG.innerHTML = '';
  simNodes.forEach(n => {
    const g = document.createElementNS('http://www.w3.org/2000/svg','g');
    g.setAttribute('class', 'node');
    g.setAttribute('data-id', n.id);

    const circle = document.createElementNS('http://www.w3.org/2000/svg','circle');
    const r = radiusFor(n);
    circle.setAttribute('r', r);
    circle.setAttribute('cx', n.x); circle.setAttribute('cy', n.y);
    circle.setAttribute('fill', colorFor(n));
    circle.setAttribute('stroke', n.id === selectedId ? '#fff' : colorFor(n));
    circle.setAttribute('stroke-opacity', n.id === selectedId ? '1' : '0.6');
    circle.setAttribute('fill-opacity', '0.85');
    g.appendChild(circle);

    if (showLabels) {
      const text = document.createElementNS('http://www.w3.org/2000/svg','text');
      text.setAttribute('x', n.x + r + 4);
      text.setAttribute('y', n.y + 4);
      text.setAttribute('font-size', n.group === 'file' ? '11' : '10');
      text.setAttribute('fill', '#cbd5e1');
      text.setAttribute('opacity', '0.85');
      text.textContent = n.label.length > 24 ? n.label.slice(0,22)+'…' : n.label;
      g.appendChild(text);
    }

    // Click to select
    g.addEventListener('click', (ev) => {
      ev.stopPropagation();
      selectNode(n);
    });

    // Drag
    let dragging = false, dragStart = null;
    g.addEventListener('mousedown', (ev) => {
      dragging = true;
      dragStart = { mx: ev.clientX, my: ev.clientY, nx: n.x, ny: n.y };
      n.pinned = true;
      ev.stopPropagation();
    });
    window.addEventListener('mousemove', (ev) => {
      if (!dragging) return;
      const dx = (ev.clientX - dragStart.mx) / transform.k;
      const dy = (ev.clientY - dragStart.my) / transform.k;
      n.x = dragStart.nx + dx;
      n.y = dragStart.ny + dy;
      n.vx = 0; n.vy = 0;
      alpha = Math.max(alpha, 0.3);
      if (!animFrame || alpha < ALPHA_MIN) tick();
    });
    window.addEventListener('mouseup', () => { dragging = false; });

    nodesG.appendChild(g);
  });
}

// ============================================================
// Selection / details
// ============================================================
function selectNode(n) {
  selectedId = n.id;
  renderGraph();
  const dc = document.getElementById('detail-content');
  let html = '';
  const row = (k,v) => '<div><span class="dk">'+k+':</span> <span class="dv">'+v+'</span></div>';
  html += row('Name', '<code>'+esc(n.label)+'</code>');
  html += row('Type', n.type);
  html += row('Group', n.group);
  if (n.filePath) html += row('File', esc(shortPath(n.filePath)));
  if (n.parent) html += row('Parent', esc(n.parent));
  if (n.line) html += row('Line', n.line);

  // Count edges
  const edgesFrom = allEdges.filter(e => e.source === n.id).length;
  const edgesTo   = allEdges.filter(e => e.target === n.id).length;
  html += row('Outgoing edges', edgesFrom);
  html += row('Incoming edges', edgesTo);

  dc.innerHTML = html;
}

function esc(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}
function shortPath(p) {
  const parts = p.replace(/\\/g,'/').split('/');
  return parts.length <= 2 ? p : parts.slice(-2).join('/');
}

svg.addEventListener('click', () => {
  selectedId = null;
  renderGraph();
  document.getElementById('detail-content').innerHTML = '<span style="color:#475569">Click a node for details</span>';
});

// ============================================================
// Pan & zoom
// ============================================================
let isPanning = false, panStart = null;
svg.addEventListener('mousedown', (ev) => {
  if (ev.target === svg || ev.target === zoomG) {
    isPanning = true;
    panStart = { mx: ev.clientX, my: ev.clientY, tx: transform.x, ty: transform.y };
  }
});
window.addEventListener('mousemove', (ev) => {
  if (!isPanning) return;
  transform.x = panStart.tx + ev.clientX - panStart.mx;
  transform.y = panStart.ty + ev.clientY - panStart.my;
  applyTransform();
});
window.addEventListener('mouseup', () => { isPanning = false; });
svg.addEventListener('wheel', (ev) => {
  ev.preventDefault();
  const factor = ev.deltaY < 0 ? 1.1 : 0.9;
  const rect = svg.getBoundingClientRect();
  const mx = ev.clientX - rect.left, my = ev.clientY - rect.top;
  transform.x = mx + (transform.x - mx) * factor;
  transform.y = my + (transform.y - my) * factor;
  transform.k *= factor;
  applyTransform();
}, { passive: false });

document.getElementById('btn-zoom-in').onclick  = () => { transform.k *= 1.2; applyTransform(); };
document.getElementById('btn-zoom-out').onclick = () => { transform.k /= 1.2; applyTransform(); };
document.getElementById('btn-reset').onclick    = () => { transform = {x:0,y:0,k:1}; applyTransform(); };
document.getElementById('btn-toggle-labels').onclick = () => {
  showLabels = !showLabels; renderGraph();
};

// ============================================================
// Type filters
// ============================================================
function buildTypeFilters() {
  const types = [...new Set(allNodes.map(n => n.type))].sort();
  const container = document.getElementById('type-filters');
  container.innerHTML = '';
  types.forEach(t => {
    const pill = document.createElement('span');
    pill.className = 'type-pill';
    pill.textContent = t;
    pill.style.color = colorFor({type:t, group:'entity'});
    pill.style.borderColor = colorFor({type:t, group:'entity'});
    pill.title = 'Toggle ' + t;
    pill.addEventListener('click', () => {
      if (hiddenTypes.has(t)) { hiddenTypes.delete(t); pill.classList.remove('off'); }
      else { hiddenTypes.add(t); pill.classList.add('on','off'); }
      buildSimData();
      initSimulation();
    });
    container.appendChild(pill);
  });
}

// ============================================================
// Search filter
// ============================================================
document.getElementById('filter-input').addEventListener('input', (ev) => {
  filterText = ev.target.value.trim();
  buildSimData();
  initSimulation();
});

// ============================================================
// Bootstrap
// ============================================================
(async () => {
  try {
    const { graph, stats } = await fetchData();
    document.getElementById('loading').style.display = 'none';

    allNodes = graph.nodes || [];
    allEdges = graph.edges || [];

    document.getElementById('s-files').textContent = stats.files ?? 0;
    document.getElementById('s-ent').textContent   = stats.entities ?? 0;
    document.getElementById('s-rel').textContent   = stats.relations ?? 0;
    document.getElementById('s-dep').textContent   = stats.dependencies ?? 0;

    buildTypeFilters();
    buildSimData();
    initSimulation();
  } catch (err) {
    document.getElementById('loading').style.display = 'none';
    const em = document.getElementById('error-msg');
    em.style.display = 'flex';
    em.textContent = '❌ ' + err.message;
  }
})();
</script>
</body>
</html>`
