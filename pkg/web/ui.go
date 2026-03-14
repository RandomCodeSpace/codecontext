package web

// uiHTML is the complete single-page web application served at /.
// Visualises the code graph as a zoomable icicle chart (horizontal flame chart)
// with a dependency + entity detail panel. Handles millions of files with O(visible)
// rendering — no force simulation, no SVG DOM nodes.
const uiHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>codecontext</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#080c14;--surface:#0d1520;--surface2:#111927;--border:#1e2d42;
  --accent:#6366f1;--accent2:#818cf8;--accent-glow:rgba(99,102,241,.25);
  --text:#e2e8f0;--text2:#94a3b8;--text3:#475569;
  --highlight:#f59e0b;--danger:#ef4444;
}
html,body{height:100%;overflow:hidden;background:var(--bg);color:var(--text);font-family:'SF Mono',ui-monospace,monospace;font-size:13px}

/* ── Header ── */
header{
  display:flex;align-items:center;gap:14px;padding:0 18px;height:48px;
  background:var(--surface);border-bottom:1px solid var(--border);
  position:relative;z-index:10;box-shadow:0 1px 20px rgba(0,0,0,.5);
}
.logo{display:flex;align-items:center;gap:8px;color:var(--accent);font-weight:700;font-size:15px;letter-spacing:-.3px}
.search-wrap{position:relative;flex:0 0 240px}
.search-wrap svg{position:absolute;left:9px;top:50%;transform:translateY(-50%);opacity:.4;pointer-events:none}
#search{
  width:100%;background:var(--bg);color:var(--text);
  border:1px solid var(--border);border-radius:6px;
  padding:5px 10px 5px 30px;font-family:inherit;font-size:12px;outline:none;
  transition:border-color .2s,box-shadow .2s;
}
#search:focus{border-color:var(--accent);box-shadow:0 0 0 3px var(--accent-glow)}
#search::placeholder{color:var(--text3)}
.stats{margin-left:auto;display:flex;gap:18px;align-items:center}
.stat{display:flex;flex-direction:column;align-items:flex-end}
.stat-val{font-size:14px;font-weight:600;color:var(--text);line-height:1}
.stat-lbl{font-size:10px;color:var(--text3);margin-top:2px;text-transform:uppercase;letter-spacing:.06em}
.lang-pills{display:flex;gap:5px;align-items:center;margin-left:8px;padding-left:14px;border-left:1px solid var(--border)}
.lang-pills-label{font-size:9px;font-weight:600;color:var(--text3);text-transform:uppercase;letter-spacing:.08em;white-space:nowrap;margin-right:2px}
.lang-pill{
  padding:3px 9px;border-radius:12px;font-size:10px;font-weight:700;
  cursor:pointer;border:1.5px solid currentColor;user-select:none;
  transition:opacity .15s,background .15s,border-style .15s,text-decoration .15s;
}
/* active = language IS visible — filled tinted background */
.lang-pill.active{opacity:1;border-style:solid}
/* hidden = language is filtered out — dim, dashed border, strikethrough */
.lang-pill.hidden{opacity:.32;border-style:dashed;background:transparent!important;text-decoration:line-through}
.lang-pill:hover{opacity:.8}

/* ── Breadcrumb ── */
#breadcrumb{
  display:flex;align-items:center;gap:2px;padding:0 18px;height:32px;
  background:var(--surface2);border-bottom:1px solid var(--border);
  font-size:11px;color:var(--text3);overflow-x:auto;white-space:nowrap;
}
#breadcrumb::-webkit-scrollbar{height:2px}
#breadcrumb::-webkit-scrollbar-thumb{background:var(--border)}
.crumb{color:var(--accent2);cursor:pointer;padding:1px 4px;border-radius:3px;transition:background .15s,color .15s}
.crumb:hover{background:var(--accent-glow);color:var(--accent)}
.crumb-sep{color:var(--text3);margin:0 1px}
.crumb-cur{color:var(--text2)}

/* ── Main layout ── */
#layout{display:flex;height:calc(100vh - 104px);overflow:hidden;position:relative}

/* ── Chart area ── */
#chart-wrap{flex:1;position:relative;overflow:hidden;background:var(--bg);cursor:crosshair}
#icicle{display:block;width:100%;height:100%}
#tooltip{
  position:absolute;pointer-events:none;display:none;
  background:var(--surface);border:1px solid var(--border);
  border-radius:6px;padding:6px 10px;font-size:11px;color:var(--text);
  box-shadow:0 4px 20px rgba(0,0,0,.6);z-index:20;max-width:280px;line-height:1.6;
}
.tooltip-name{font-weight:600;color:var(--accent2);margin-bottom:2px}
.tooltip-sub{color:var(--text3)}
#empty-hint{
  position:absolute;inset:0;display:flex;flex-direction:column;
  align-items:center;justify-content:center;color:var(--text3);gap:10px;pointer-events:none;
}
#empty-hint svg{opacity:.3}
#empty-hint p{font-size:12px;text-align:center;line-height:1.7}
#filter-hint{
  position:absolute;inset:0;display:flex;flex-direction:column;
  align-items:center;justify-content:center;color:var(--text3);gap:10px;pointer-events:none;
}
#filter-hint svg{opacity:.3}
#filter-hint p{font-size:12px;text-align:center;line-height:1.7}
code{background:var(--surface2);padding:1px 5px;border-radius:3px;color:var(--accent2)}

/* ── Detail panel ── */
#panel{width:300px;flex:0 0 300px;background:var(--surface);border-left:1px solid var(--border);overflow-y:auto;display:flex;flex-direction:column}
#panel::-webkit-scrollbar{width:4px}
#panel::-webkit-scrollbar-thumb{background:var(--border)}
#panel-header{padding:14px 16px 10px;border-bottom:1px solid var(--border);position:sticky;top:0;background:var(--surface);z-index:5}
.panel-title{font-size:13px;font-weight:700;color:var(--text);line-height:1.3;word-break:break-all}
.panel-meta{font-size:11px;color:var(--text3);margin-top:5px;display:flex;gap:6px;flex-wrap:wrap}
.meta-chip{padding:2px 7px;background:var(--surface2);border-radius:10px;border:1px solid var(--border);color:var(--text2);font-size:10px}
#panel-body{flex:1;padding:12px 16px}
.panel-hint{display:flex;flex-direction:column;align-items:center;justify-content:center;height:100%;color:var(--text3);gap:10px;text-align:center;padding:20px}
.panel-hint svg{opacity:.3}
.panel-hint p{font-size:12px;line-height:1.6}
.section{margin-bottom:18px}
.section-title{font-size:10px;font-weight:600;color:var(--text3);text-transform:uppercase;letter-spacing:.08em;margin-bottom:8px;display:flex;align-items:center;gap:6px}
.section-title::after{content:'';flex:1;height:1px;background:var(--border)}
.item-list{display:flex;flex-direction:column;gap:2px}
.item{display:flex;align-items:center;gap:7px;padding:4px 6px;border-radius:4px;font-size:11px;color:var(--text2);transition:background .12s;cursor:default}
.item.link{cursor:pointer}
.item.link:hover{background:var(--surface2);color:var(--accent2)}
.item-icon{opacity:.5;flex-shrink:0;width:14px}
.item-name{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;flex:1}
.badge{flex-shrink:0;padding:1px 5px;border-radius:8px;font-size:9px;font-weight:600;text-transform:uppercase;letter-spacing:.04em;background:var(--surface2);border:1px solid var(--border);color:var(--text3)}
.badge.class{background:#064e3b;border-color:#10b981;color:#6ee7b7}
.badge.interface{background:#064e3b;border-color:#34d399;color:#a7f3d0}
.badge.function,.badge.method{background:#1e1b4b;border-color:#6366f1;color:#a5b4fc}
.badge.type,.badge.struct{background:#0c4a6e;border-color:#38bdf8;color:#7dd3fc}
.badge.enum{background:#451a03;border-color:#f59e0b;color:#fcd34d}
.badge.annotation{background:#4a044e;border-color:#d946ef;color:#f0abfc}
::-webkit-scrollbar{width:6px;height:6px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:var(--border);border-radius:3px}

/* ── Search clear button ── */
.search-clear{
  position:absolute;right:6px;top:50%;transform:translateY(-50%);
  width:18px;height:18px;border-radius:50%;border:none;cursor:pointer;
  background:var(--surface2);color:var(--text3);font-size:11px;line-height:18px;text-align:center;
  display:none;transition:background .15s,color .15s;padding:0;
}
.search-clear:hover{background:var(--border);color:var(--text)}

/* ── Help button ── */
.help-btn{
  width:26px;height:26px;border-radius:50%;border:1px solid var(--border);
  background:transparent;color:var(--text3);font-size:12px;font-weight:700;
  cursor:pointer;transition:border-color .15s,color .15s;flex-shrink:0;
}
.help-btn:hover{border-color:var(--accent);color:var(--accent2)}

/* ── Help overlay ── */
#help-overlay{
  position:fixed;inset:0;z-index:100;display:none;
  background:rgba(0,0,0,.7);backdrop-filter:blur(4px);
  align-items:center;justify-content:center;
}
#help-overlay.visible{display:flex}
#help-modal{
  background:var(--surface);border:1px solid var(--border);border-radius:10px;
  padding:24px 28px;max-width:480px;width:90%;box-shadow:0 8px 40px rgba(0,0,0,.6);
}
.help-title{font-size:15px;font-weight:700;color:var(--text);margin-bottom:16px;display:flex;align-items:center;justify-content:space-between}
.help-close{background:none;border:none;color:var(--text3);cursor:pointer;font-size:18px;padding:2px 6px}
.help-close:hover{color:var(--text)}
.help-section{margin-bottom:14px}
.help-section-title{font-size:10px;font-weight:600;color:var(--text3);text-transform:uppercase;letter-spacing:.08em;margin-bottom:6px}
.help-row{display:flex;align-items:center;gap:10px;padding:3px 0;font-size:11px;color:var(--text2)}
.help-key{
  display:inline-block;min-width:22px;padding:2px 6px;text-align:center;
  background:var(--surface2);border:1px solid var(--border);border-radius:4px;
  font-size:10px;font-weight:600;color:var(--text);font-family:inherit;
}
.help-swatch{width:12px;height:12px;border-radius:3px;flex-shrink:0;border:1px solid rgba(255,255,255,.1)}

/* ── Panel toggle ── */
.panel-toggle{
  position:absolute;top:50%;right:300px;transform:translateY(-50%);z-index:8;
  width:16px;height:40px;border-radius:4px 0 0 4px;cursor:pointer;
  background:var(--surface2);border:1px solid var(--border);border-right:none;
  color:var(--text3);display:flex;align-items:center;justify-content:center;
  font-size:10px;transition:color .15s,background .15s;
}
.panel-toggle:hover{background:var(--border);color:var(--text)}
#panel.collapsed{width:0;flex:0 0 0;overflow:hidden;border-left:none}
#layout.panel-collapsed .panel-toggle{right:0;border-radius:4px}

/* ── Status bar (keyboard hints) ── */
#status-bar{
  height:24px;display:flex;align-items:center;gap:14px;padding:0 18px;
  background:var(--surface);border-top:1px solid var(--border);
  font-size:10px;color:var(--text3);
}
.status-item{display:flex;align-items:center;gap:4px}
.status-key{
  padding:0 4px;background:var(--surface2);border:1px solid var(--border);
  border-radius:3px;font-size:9px;font-weight:600;color:var(--text2);
}
</style>
</head>
<body>

<header>
  <div class="logo">
    <svg width="20" height="20" viewBox="0 0 20 20" fill="none">
      <path d="M3 4h14M3 10h9M3 16h6" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
      <circle cx="15" cy="13" r="3.5" stroke="currentColor" stroke-width="1.5"/>
      <path d="M17.5 15.5L19 17" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
    </svg>
    codecontext
  </div>

  <div class="search-wrap">
    <svg width="13" height="13" viewBox="0 0 16 16" fill="none">
      <circle cx="7" cy="7" r="5" stroke="currentColor" stroke-width="1.6"/>
      <path d="M11 11l3 3" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/>
    </svg>
    <input id="search" placeholder="Search paths, files&#x2026;" autocomplete="off" spellcheck="false">
    <button class="search-clear" id="search-clear" title="Clear search">&times;</button>
  </div>

  <div class="lang-pills" id="lang-pills"></div>

  <button class="help-btn" id="help-btn" title="Keyboard shortcuts &amp; legend (H)">?</button>

  <div class="stats">
    <div class="stat"><span class="stat-val" id="s-files">&#x2014;</span><span class="stat-lbl">Files</span></div>
    <div class="stat"><span class="stat-val" id="s-entities">&#x2014;</span><span class="stat-lbl">Entities</span></div>
    <div class="stat"><span class="stat-val" id="s-deps">&#x2014;</span><span class="stat-lbl">Deps</span></div>
  </div>
</header>

<div id="breadcrumb"><span class="crumb-cur">.</span></div>

<div id="layout">
  <div id="chart-wrap">
    <canvas id="icicle"></canvas>
    <div id="tooltip"></div>
    <div id="empty-hint" style="display:none">
      <svg width="48" height="48" viewBox="0 0 48 48" fill="none">
        <rect x="8" y="8" width="32" height="32" rx="4" stroke="currentColor" stroke-width="2"/>
        <path d="M16 20h16M16 28h10" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
      </svg>
      <p>No data yet.<br>Run <code>codecontext index .</code> to index your project.</p>
    </div>
    <div id="filter-hint" style="display:none">
      <svg width="48" height="48" viewBox="0 0 48 48" fill="none">
        <circle cx="24" cy="24" r="18" stroke="currentColor" stroke-width="2" stroke-dasharray="4 3"/>
        <path d="M18 22h12M18 26h8" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
      </svg>
      <p>All files hidden by the active filter.<br>Click a <b style="color:var(--accent2)">language pill</b> to show files,<br>or press <code>reset</code>.</p>
    </div>
  </div>

  <div class="panel-toggle" id="panel-toggle" title="Toggle panel (P)">&#x25B6;</div>

  <div id="panel">
    <div id="panel-header" style="display:none">
      <div class="panel-title" id="panel-title"></div>
      <div class="panel-meta" id="panel-meta"></div>
    </div>
    <div id="panel-body">
      <div class="panel-hint" id="panel-hint">
        <svg width="40" height="40" viewBox="0 0 40 40" fill="none">
          <path d="M8 12h24M8 20h16M8 28h10" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>
          <circle cx="30" cy="27" r="5" stroke="currentColor" stroke-width="1.5"/>
          <path d="M34 31l3 3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
        </svg>
        <p>Click any segment to explore<br>dependencies &amp; entities.</p>
      </div>
      <div id="panel-content" style="display:none"></div>
    </div>
  </div>
</div>

<div id="status-bar">
  <span class="status-item"><span class="status-key">Click</span> select</span>
  <span class="status-item"><span class="status-key">Click&#xD7;2</span> zoom in</span>
  <span class="status-item"><span class="status-key">Esc</span> zoom out</span>
  <span class="status-item"><span class="status-key">/</span> search</span>
  <span class="status-item"><span class="status-key">P</span> panel</span>
  <span class="status-item"><span class="status-key">H</span> help</span>
</div>

<div id="help-overlay">
  <div id="help-modal">
    <div class="help-title">
      Keyboard Shortcuts &amp; Legend
      <button class="help-close" id="help-close">&times;</button>
    </div>
    <div class="help-section">
      <div class="help-section-title">Navigation</div>
      <div class="help-row"><span class="help-key">Click</span> Select a segment &amp; show details</div>
      <div class="help-row"><span class="help-key">Click again</span> Zoom into selected directory</div>
      <div class="help-row"><span class="help-key">Esc</span> Zoom out one level (or deselect)</div>
      <div class="help-row"><span class="help-key">Backspace</span> Zoom out one level</div>
      <div class="help-row"><span class="help-key">/</span> Focus search box</div>
      <div class="help-row"><span class="help-key">P</span> Toggle detail panel</div>
      <div class="help-row"><span class="help-key">H</span> Toggle this help overlay</div>
    </div>
    <div class="help-section">
      <div class="help-section-title">Chart Colors</div>
      <div class="help-row"><span class="help-swatch" style="background:#f97316"></span> <b>Bright</b> = source file (colored by language)</div>
      <div class="help-row"><span class="help-swatch" style="background:#131c2a;border-left:2px solid #f97316"></span> <b>Dark + stripe</b> = directory (stripe = dominant language)</div>
      <div class="help-row"><span class="help-swatch" style="background:#4338ca"></span> <b>Indigo</b> = currently selected</div>
      <div class="help-row"><span class="help-swatch" style="background:#f59e0b"></span> <b>Amber</b> = search match</div>
    </div>
    <div class="help-section">
      <div class="help-section-title">Language Colors</div>
      <div id="help-langs"></div>
    </div>
  </div>
</div>

<script>
'use strict';

const LANG_COLOR={java:'#f97316',go:'#00ADD8',python:'#3b82f6',javascript:'#f59e0b',typescript:'#60a5fa',c:'#a8b9cc',cpp:'#f34b7d',_dir:'#131c2a',_dirAlt:'#172131',_default:'#334155'};

let treeRoot=null,zoomedPath='',hoveredNode=null,selectedNode=null,searchText='',langFilter=new Set();
let layoutCache=[];
const dpr=window.devicePixelRatio||1;

const canvas=document.getElementById('icicle');
const ctx=canvas.getContext('2d');
const tooltip=document.getElementById('tooltip');
const crumbBar=document.getElementById('breadcrumb');
const panelHeader=document.getElementById('panel-header');
const panelTitle=document.getElementById('panel-title');
const panelMeta=document.getElementById('panel-meta');
const panelBody=document.getElementById('panel-body');
const panelHint=document.getElementById('panel-hint');
const panelContent=document.getElementById('panel-content');
const emptyHint=document.getElementById('empty-hint');
const filterHint=document.getElementById('filter-hint');

function fmt(n){return n>=1e6?(n/1e6).toFixed(1)+'M':n>=1e3?(n/1e3).toFixed(1)+'K':String(n)}

// Blend a hex color toward black by a factor (0=black, 1=full color).
function dimColor(hex,factor){
  factor=factor||0.3;
  var n=parseInt(hex.replace('#',''),16);
  var r=Math.round(((n>>16)&255)*factor);
  var g=Math.round(((n>>8)&255)*factor);
  var b=Math.round((n&255)*factor);
  return '#'+((1<<24)+(r<<16)+(g<<8)+b).toString(16).slice(1);
}

function nodeColor(node,depth){
  // Files: bright saturated language color
  if(isFile(node)){
    return (node.lang&&LANG_COLOR[node.lang])?LANG_COLOR[node.lang]:LANG_COLOR._default;
  }
  // Directories: neutral dark tone — alternates by depth for visual layering
  return (depth||0)%2===0?LANG_COLOR._dir:LANG_COLOR._dirAlt;
}

// Returns the accent color for a directory's left stripe based on
// the dominant *visible* language (respects the active filter).
function dirAccentColor(node){
  // No filter — use the node's dominant language
  if(langFilter.size===0){
    return (node.lang&&LANG_COLOR[node.lang])?LANG_COLOR[node.lang]:LANG_COLOR._default;
  }
  // If the node's dominant lang is still visible, use it
  if(node.lang&&!langFilter.has(node.lang)&&LANG_COLOR[node.lang])return LANG_COLOR[node.lang];
  // Otherwise scan immediate children for the first visible language
  if(node.children){
    for(var i=0;i<node.children.length;i++){
      var c=node.children[i];
      if(c.lang&&!langFilter.has(c.lang)&&LANG_COLOR[c.lang])return LANG_COLOR[c.lang];
    }
  }
  return LANG_COLOR._default;
}

// Count files per language in the tree (used for pill labels).
function countByLang(node){
  var acc={};
  function walk(n){
    if(isFile(n)&&n.lang){acc[n.lang]=(acc[n.lang]||0)+1;}
    if(n.children)for(var i=0;i<n.children.length;i++)walk(n.children[i]);
  }
  walk(node);
  return acc;
}

function lighten(hex,amt){
  amt=amt||35;
  let n=parseInt(hex.replace('#',''),16);
  let r=Math.min(255,((n>>16)&255)+amt);
  let g=Math.min(255,((n>>8)&255)+amt);
  let b=Math.min(255,(n&255)+amt);
  return '#'+(((r<<16)|(g<<8)|b)).toString(16).padStart(6,'0');
}

function findNode(node,path){
  if(!path||path==='.')return node;
  if(node.path===path)return node;
  if(node.children)for(var i=0;i<node.children.length;i++){var f=findNode(node.children[i],path);if(f)return f;}
  return null;
}

function maxDepth(node){
  if(!node.children||!node.children.length)return 0;
  var m=0;for(var i=0;i<node.children.length;i++)m=Math.max(m,1+maxDepth(node.children[i]));
  return m;
}

function collectLangs(node,acc){
  acc=acc||new Set();
  if(node.lang)acc.add(node.lang);
  if(node.children)for(var i=0;i<node.children.length;i++)collectLangs(node.children[i],acc);
  return acc;
}

function resizeCanvas(){
  var wrap=document.getElementById('chart-wrap');
  var W=wrap.clientWidth,H=wrap.clientHeight;
  canvas.width=W*dpr;canvas.height=H*dpr;
  canvas.style.width=W+'px';canvas.style.height=H+'px';
  ctx.setTransform(dpr,0,0,dpr,0,0);
  draw();
}
new ResizeObserver(resizeCanvas).observe(document.getElementById('chart-wrap'));

var GAP=1;

function draw(){
  var W=canvas.width/dpr,H=canvas.height/dpr;
  ctx.clearRect(0,0,W,H);
  layoutCache=[];
  if(!treeRoot){emptyHint.style.display='flex';filterHint.style.display='none';return;}
  emptyHint.style.display='none';
  filterHint.style.display='none';
  var zr=findNode(treeRoot,zoomedPath)||treeRoot;
  var depth=maxDepth(zr);
  var rowH=Math.max(18,Math.min(44,Math.floor(H/Math.max(depth+1,1))));
  drawNode(zr,0,0,W,rowH,0);
  // Show filter-empty message when all files are hidden
  if(langFilter.size>0&&layoutCache.length<=1){filterHint.style.display='flex';}
  // Hover highlight
  if(hoveredNode){
    var entry=null;
    for(var i=layoutCache.length-1;i>=0;i--){if(layoutCache[i].node===hoveredNode){entry=layoutCache[i];break;}}
    if(entry){
      ctx.save();ctx.strokeStyle='rgba(255,255,255,.35)';ctx.lineWidth=1.5;
      ctx.strokeRect(entry.x+.75,entry.y+.75,entry.w-1.5,entry.h-1.5);ctx.restore();
    }
  }
}

// A leaf node is a file (no children); directories always have children.
function isFile(n){return !n.children||!n.children.length;}

// Returns true if this node (or any descendant) has visible (non-hidden) files.
function nodeHasVisible(node){
  if(isFile(node))return !node.lang||!langFilter.has(node.lang);
  for(var i=0;i<node.children.length;i++)if(nodeHasVisible(node.children[i]))return true;
  return false;
}

// Returns the count of only visible files in a subtree (for width proportioning).
function visibleCount(node){
  if(isFile(node))return (node.lang&&langFilter.has(node.lang))?0:node.count;
  var s=0;for(var i=0;i<node.children.length;i++)s+=visibleCount(node.children[i]);
  return s;
}

function drawNode(node,depth,x,w,rowH,offsetDepth){
  if(w<1)return;
  // Skip file nodes whose language is hidden
  if(isFile(node)&&node.lang&&langFilter.has(node.lang))return;
  // Skip directory nodes whose entire subtree is hidden
  if(!isFile(node)&&langFilter.size>0&&!nodeHasVisible(node))return;
  var y=(depth-offsetDepth)*rowH;
  if(y>canvas.height/dpr)return;
  var h=rowH-GAP;
  var color=nodeColor(node,depth);
  var lc=searchText.toLowerCase();
  var matched=lc&&node.path.toLowerCase().indexOf(lc)>=0;
  if(matched)color='#f59e0b';
  else if(node===selectedNode)color='#4338ca';
  // Shadow glow for selected
  if(node===selectedNode){ctx.shadowColor='rgba(99,102,241,.6)';ctx.shadowBlur=10;}
  ctx.fillStyle=color;
  ctx.fillRect(x,y,w-GAP,h);
  ctx.shadowBlur=0;ctx.shadowColor='transparent';
  // Left accent stripe for directories — use dominant *visible* language color
  if(!isFile(node)){
    ctx.fillStyle=dirAccentColor(node);
    ctx.fillRect(x,y,2,h);
  }
  // Bottom edge glow for files — subtle language tint
  if(isFile(node)&&w>4){
    ctx.fillStyle=hexAlpha((node.lang&&LANG_COLOR[node.lang])?LANG_COLOR[node.lang]:LANG_COLOR._default,0.3);
    ctx.fillRect(x,y+h-1,w-GAP,1);
  }
  // Label — show visible count when filtering, full count otherwise
  if(w>36){
    var fs=Math.max(9,Math.min(12,rowH-6));
    ctx.font=fs+'px \'SF Mono\',ui-monospace,monospace';
    ctx.fillStyle='rgba(255,255,255,.85)';
    ctx.save();
    ctx.beginPath();ctx.rect(x+4,y+1,w-10,h-2);ctx.clip();
    var dispCount=!isFile(node)&&langFilter.size>0?visibleCount(node):node.count;
    var label=node.name+(!isFile(node)&&w>90?'  '+fmt(dispCount):'');
    ctx.fillText(label,x+6,y+h/2+fs*0.36);
    ctx.restore();
  }
  layoutCache.push({node:node,x:x,y:y,w:w-GAP,h:h});
  // Children — filter recursively and re-proportion widths by visible count
  if(node.children&&node.children.length){
    var visible=langFilter.size>0
      ?node.children.filter(nodeHasVisible)
      :node.children;
    var total=0;for(var i=0;i<visible.length;i++)total+=langFilter.size>0?visibleCount(visible[i]):visible[i].count;
    if(total===0)total=1;
    var cx=x;
    for(var i=0;i<visible.length;i++){
      var vc=langFilter.size>0?visibleCount(visible[i]):visible[i].count;
      var cw=Math.floor((vc/total)*w);
      drawNode(visible[i],depth+1,cx,cw,rowH,offsetDepth);
      cx+=cw;
    }
  }
}

function hitTest(px,py){
  for(var i=layoutCache.length-1;i>=0;i--){
    var e=layoutCache[i];
    if(px>=e.x&&px<e.x+e.w&&py>=e.y&&py<e.y+e.h)return e.node;
  }
  return null;
}

function updateBreadcrumb(){
  crumbBar.innerHTML='';
  var zr=findNode(treeRoot,zoomedPath)||treeRoot;
  var parts=zr.path?zr.path.split('/'):[];
  var rootEl=document.createElement('span');
  rootEl.className='crumb';rootEl.textContent='.';
  rootEl.onclick=function(){zoomedPath='';draw();updateBreadcrumb();};
  crumbBar.appendChild(rootEl);
  var cumPath='';
  for(var i=0;i<parts.length;i++){
    cumPath=cumPath?cumPath+'/'+parts[i]:parts[i];
    var sep=document.createElement('span');sep.className='crumb-sep';sep.textContent='/';
    crumbBar.appendChild(sep);
    var el=document.createElement('span');
    var isLast=i===parts.length-1;
    el.className=isLast?'crumb-cur':'crumb';
    el.textContent=parts[i];
    if(!isLast){(function(cp){el.onclick=function(){zoomedPath=cp;draw();updateBreadcrumb();};})(cumPath);}
    crumbBar.appendChild(el);
  }
}

function showTooltip(node,px,py){
  tooltip.style.display='block';
  var isDir=!isFile(node);
  var typeLabel=isDir?'<span style="color:var(--text3)">directory</span>':'<span style="color:var(--text3)">file</span>';
  var hint='';
  if(node===selectedNode&&isDir)hint='<div style="color:var(--accent);font-size:10px;margin-top:3px">Click to zoom in</div>';
  else if(node!==selectedNode)hint='<div style="color:var(--text3);font-size:10px;margin-top:3px">Click to select</div>';
  tooltip.innerHTML='<div class="tooltip-name">'+esc(node.name)+'</div>'
    +'<div class="tooltip-sub">'
    +typeLabel+' &middot; '
    +(node.lang?'<b>'+node.lang+'</b> &middot; ':'')+fmt(node.count)+' file'+(node.count!==1?'s':'')
    +(node.path?' &middot; '+esc(node.path):'')+'</div>'
    +hint;
  var wrap=document.getElementById('chart-wrap');
  var W=wrap.clientWidth,H=wrap.clientHeight;
  var tx=px+14,ty=py+14;
  if(tx+290>W)tx=px-300;
  if(ty+60>H)ty=py-70;
  tooltip.style.left=tx+'px';tooltip.style.top=ty+'px';
}
function hideTooltip(){tooltip.style.display='none';}

function esc(s){return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');}
function eattr(s){return String(s).replace(/"/g,'&quot;');}
function shortDir(p){var pts=p.split('/');return pts.length>2?'&hellip;/'+pts.slice(-2).join('/'):esc(p);}

function dirIcon(){return '<svg class="item-icon" width="14" height="14" viewBox="0 0 16 16" fill="none"><path d="M1.5 6A1.5 1.5 0 013 4.5h3.586a1 1 0 01.707.293l1 1A1 1 0 009 6.086V12A1.5 1.5 0 017.5 13.5h-5A1.5 1.5 0 011 12V7.5A1.5 1.5 0 012.5 6H1.5z" stroke="currentColor" stroke-width="1.2" fill="none"/><path d="M8.5 8h5A1.5 1.5 0 0115 9.5v3A1.5 1.5 0 0113.5 14H8A1.5 1.5 0 016.5 12.5v-3A1.5 1.5 0 018 8" stroke="currentColor" stroke-width="1.2" fill="none"/></svg>';}
function fileIcon(){return '<svg class="item-icon" width="14" height="14" viewBox="0 0 16 16" fill="none"><path d="M3 2h7l3 3v9a1 1 0 01-1 1H3a1 1 0 01-1-1V3a1 1 0 011-1z" stroke="currentColor" stroke-width="1.2"/><path d="M10 2v3h3" stroke="currentColor" stroke-width="1.2"/></svg>';}
function entityIcon(t){var c=t==='class'||t==='interface'?'#34d399':t==='function'||t==='method'?'#818cf8':'#67e8f9';return '<svg class="item-icon" width="14" height="14" viewBox="0 0 16 16" fill="none"><circle cx="8" cy="8" r="5" stroke="'+c+'" stroke-width="1.3"/><path d="M8 5v3l2 2" stroke="'+c+'" stroke-width="1.2" stroke-linecap="round"/></svg>';}

function zoomTo(path){
  zoomedPath=path;draw();updateBreadcrumb();
  var node=findNode(treeRoot,path);
  if(node){selectedNode=node;fetchDirDetail(path);}
}
window.zoomTo=zoomTo;

function hexAlpha(hex,a){
  var r=parseInt(hex.slice(1,3),16),g=parseInt(hex.slice(3,5),16),b=parseInt(hex.slice(5,7),16);
  return 'rgba('+r+','+g+','+b+','+a+')';
}

function buildLangPills(){
  if(!treeRoot)return;
  var langs=Array.from(collectLangs(treeRoot)).sort();
  var container=document.getElementById('lang-pills');
  container.innerHTML='';
  if(!langs.length)return;

  var langCounts=countByLang(treeRoot);

  // Label
  var lbl=document.createElement('span');
  lbl.className='lang-pills-label';lbl.textContent='langs:';
  container.appendChild(lbl);

  var pillEls={};

  langs.forEach(function(lang){
    var c=LANG_COLOR[lang]||LANG_COLOR._default;
    var cnt=langCounts[lang]||0;
    var pill=document.createElement('div');
    pill.className='lang-pill active';
    pill.style.color=c;
    pill.style.borderColor=c;
    pill.style.background=hexAlpha(c,0.18);
    pill.textContent=lang+' '+fmt(cnt);
    pill.title='Click to hide '+lang+' files ('+fmt(cnt)+' files)';
    pillEls[lang]=pill;

    pill.onclick=function(){
      if(langFilter.has(lang)){
        // un-hide: remove from exclusion set → active
        langFilter.delete(lang);
        pill.classList.remove('hidden');pill.classList.add('active');
        pill.style.background=hexAlpha(c,0.18);
        pill.title='Click to hide '+lang+' files';
      } else {
        // hide: add to exclusion set → hidden
        langFilter.add(lang);
        pill.classList.remove('active');pill.classList.add('hidden');
        pill.style.background='transparent';
        pill.title='Click to show '+lang+' files';
      }
      draw();
      updateResetBtn();
    };
    container.appendChild(pill);
  });

  // Reset button — only visible when a filter is active
  var reset=document.createElement('div');
  reset.id='lang-reset';
  reset.style.cssText='display:none;padding:2px 7px;border-radius:10px;font-size:9px;font-weight:600;cursor:pointer;border:1px solid var(--border);color:var(--text3);margin-left:2px;transition:color .15s';
  reset.textContent='reset';
  reset.title='Show all languages';
  reset.onmouseenter=function(){reset.style.color='var(--accent2)';};
  reset.onmouseleave=function(){reset.style.color='var(--text3)';};
  reset.onclick=function(){
    langFilter.clear();
    langs.forEach(function(lang){
      var c=LANG_COLOR[lang]||LANG_COLOR._default;
      var p=pillEls[lang];
      p.classList.remove('hidden');p.classList.add('active');
      p.style.background=hexAlpha(c,0.18);
      p.title='Click to hide '+lang+' files';
    });
    draw();updateResetBtn();
  };
  container.appendChild(reset);

  function updateResetBtn(){
    reset.style.display=langFilter.size>0?'block':'none';
  }
  updateResetBtn();
}

function showPanelLoading(node){
  panelHeader.style.display='block';
  panelTitle.textContent=node.path||'.';
  panelMeta.textContent='Loading...';
  panelHint.style.display='none';
  panelContent.style.display='block';
  panelContent.innerHTML='<div style="color:var(--text3);font-size:11px;padding:20px 0;text-align:center">Fetching details&#x2026;</div>';
}

function populatePanel(detail){
  panelHeader.style.display='block';
  panelHint.style.display='none';
  panelContent.style.display='block';
  var name=detail.path||'.';
  panelTitle.textContent=name;
  panelMeta.innerHTML='<span class="meta-chip">'+fmt(detail.fileCount)+' files</span>'
    +(detail.topEntities&&detail.topEntities.length?'<span class="meta-chip">'+detail.topEntities.length+'+ entities</span>':'');
  var html='';
  if(detail.importsFrom&&detail.importsFrom.length){
    html+='<div class="section"><div class="section-title">Imports From</div><div class="item-list">';
    detail.importsFrom.forEach(function(d){
      html+='<div class="item link" onclick="zoomTo(\''+eattr(d)+'\')">'+dirIcon()+'<span class="item-name" title="'+esc(d)+'">'+shortDir(d)+'</span></div>';
    });
    html+='</div></div>';
  }
  if(detail.importedBy&&detail.importedBy.length){
    html+='<div class="section"><div class="section-title">Imported By</div><div class="item-list">';
    detail.importedBy.forEach(function(d){
      html+='<div class="item link" onclick="zoomTo(\''+eattr(d)+'\')">'+dirIcon()+'<span class="item-name" title="'+esc(d)+'">'+shortDir(d)+'</span></div>';
    });
    html+='</div></div>';
  }
  if(detail.topFiles&&detail.topFiles.length){
    html+='<div class="section"><div class="section-title">Files</div><div class="item-list">';
    detail.topFiles.forEach(function(f){
      html+='<div class="item">'+fileIcon()+'<span class="item-name" title="'+esc(f)+'">'+esc(f)+'</span></div>';
    });
    html+='</div></div>';
  }
  if(detail.topEntities&&detail.topEntities.length){
    html+='<div class="section"><div class="section-title">Entities</div><div class="item-list">';
    detail.topEntities.forEach(function(e){
      var t=(e.type||'').toLowerCase();
      html+='<div class="item">'+entityIcon(t)+'<span class="item-name" title="'+esc(e.name)+' ('+esc(e.file)+')">'+esc(e.name)+'</span><span class="badge '+eattr(t)+'">'+esc(e.type)+'</span></div>';
    });
    html+='</div></div>';
  }
  if(!html)html='<div style="color:var(--text3);font-size:11px;padding:16px 0">No dependency data for this path.</div>';
  panelContent.innerHTML=html;
}

canvas.addEventListener('mousemove',function(e){
  var r=canvas.getBoundingClientRect();
  var px=e.clientX-r.left,py=e.clientY-r.top;
  var node=hitTest(px,py);
  if(node!==hoveredNode){hoveredNode=node;draw();}
  if(node)showTooltip(node,px,py);else hideTooltip();
});
canvas.addEventListener('mouseleave',function(){hoveredNode=null;hideTooltip();draw();});
canvas.addEventListener('click',function(e){
  var r=canvas.getBoundingClientRect();
  var node=hitTest(e.clientX-r.left,e.clientY-r.top);
  if(!node)return;
  if(node===selectedNode&&node.children&&node.children.length){
    zoomedPath=node.path;updateBreadcrumb();draw();
  } else {
    selectedNode=node;draw();
    if(node.path)fetchDirDetail(node.path);
  }
});

// ── Search with clear button ──
var searchInput=document.getElementById('search');
var searchClear=document.getElementById('search-clear');
searchInput.addEventListener('input',function(e){
  searchText=e.target.value.trim();
  searchClear.style.display=searchText?'block':'none';
  draw();
});
searchClear.addEventListener('click',function(){
  searchInput.value='';searchText='';searchClear.style.display='none';
  searchInput.focus();draw();
});

// ── Panel toggle ──
var panelEl=document.getElementById('panel');
var layoutEl=document.getElementById('layout');
var panelToggle=document.getElementById('panel-toggle');
function togglePanel(){
  var collapsed=panelEl.classList.toggle('collapsed');
  layoutEl.classList.toggle('panel-collapsed',collapsed);
  panelToggle.innerHTML=collapsed?'&#x25C0;':'&#x25B6;';
  panelToggle.title=collapsed?'Show panel (P)':'Hide panel (P)';
  setTimeout(resizeCanvas,50);
}
panelToggle.addEventListener('click',togglePanel);

// ── Help overlay ──
var helpOverlay=document.getElementById('help-overlay');
function toggleHelp(){helpOverlay.classList.toggle('visible');}
document.getElementById('help-btn').addEventListener('click',toggleHelp);
document.getElementById('help-close').addEventListener('click',toggleHelp);
helpOverlay.addEventListener('click',function(e){if(e.target===helpOverlay)toggleHelp();});
// Populate language colors in help
(function(){
  var el=document.getElementById('help-langs');
  var langs={java:'#f97316',go:'#00ADD8',python:'#3b82f6',javascript:'#f59e0b',typescript:'#60a5fa',c:'#a8b9cc',cpp:'#f34b7d'};
  var html='';
  for(var l in langs)html+='<div class="help-row"><span class="help-swatch" style="background:'+langs[l]+'"></span> '+l+'</div>';
  el.innerHTML=html;
})();

// ── Keyboard shortcuts ──
document.addEventListener('keydown',function(e){
  // Ignore when typing in search
  if(e.target===searchInput&&e.key!=='Escape')return;

  switch(e.key){
    case 'Escape':
      // If help is open, close it first
      if(helpOverlay.classList.contains('visible')){toggleHelp();return;}
      // If search is active, clear it
      if(searchText){searchInput.value='';searchText='';searchClear.style.display='none';draw();return;}
      // Zoom out one level
      if(zoomedPath){
        var parts=zoomedPath.split('/');parts.pop();
        zoomedPath=parts.join('/');
        selectedNode=findNode(treeRoot,zoomedPath)||null;
        draw();updateBreadcrumb();
        if(zoomedPath)fetchDirDetail(zoomedPath);
        return;
      }
      // Deselect
      if(selectedNode){selectedNode=null;draw();panelHeader.style.display='none';panelHint.style.display='flex';panelContent.style.display='none';}
      break;
    case 'Backspace':
      if(e.target.tagName==='INPUT')return;
      if(zoomedPath){
        var parts=zoomedPath.split('/');parts.pop();
        zoomedPath=parts.join('/');
        draw();updateBreadcrumb();
      }
      break;
    case '/':
      if(e.target.tagName==='INPUT')return;
      e.preventDefault();searchInput.focus();
      break;
    case 'p': case 'P':
      if(e.target.tagName==='INPUT')return;
      togglePanel();break;
    case 'h': case 'H': case '?':
      if(e.target.tagName==='INPUT')return;
      toggleHelp();break;
  }
});

async function fetchTree(){
  try{
    var res=await fetch('/api/tree');
    if(!res.ok)throw new Error('HTTP '+res.status);
    treeRoot=await res.json();
    draw();updateBreadcrumb();buildLangPills();
  }catch(err){
    emptyHint.style.display='flex';console.error('fetchTree:',err);
  }
}

async function fetchDirDetail(path){
  var node=findNode(treeRoot,path)||{path:path};
  showPanelLoading(node);
  try{
    var res=await fetch('/api/dir?path='+encodeURIComponent(path));
    if(!res.ok)throw new Error('HTTP '+res.status);
    populatePanel(await res.json());
  }catch(err){
    panelMeta.innerHTML='<span class="meta-chip" style="border-color:var(--danger);color:var(--danger)">Error</span>';
    panelContent.style.display='block';
    panelContent.innerHTML='<div style="color:var(--danger);font-size:11px;padding:10px">Error loading details.<br><span style="color:var(--text3)">'+esc(String(err))+'</span></div>';
    console.error('fetchDirDetail:',err);
  }
}

async function fetchStats(){
  try{
    var res=await fetch('/api/stats');
    var d=await res.json();
    document.getElementById('s-files').textContent=fmt(d.files||0);
    document.getElementById('s-entities').textContent=fmt(d.entities||0);
    document.getElementById('s-deps').textContent=fmt(d.dependencies||0);
  }catch(e){}
}

resizeCanvas();
fetchStats();
fetchTree();
</script>
</body>
</html>`
