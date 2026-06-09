// =====================================================
// Analytics Dashboard JS
// Chart rendering, Widget editor, CSV/XLSX export
// =====================================================

const BACKEND = (typeof window.resolveBackendURL === 'function') ? window.resolveBackendURL() : (window.__backendURL || window.BACKEND_URL || 'http://localhost:8000');
let currentDashboardId = '';
let currentDashboard = null;
let editMode = false;
let widgetTypes = [];
let editingWidgetId = null;
let chartInstances = {};

function readAuthCookie(name) {
    const prefix = name + '=';
    const parts = document.cookie.split(';');
    for (let i = 0; i < parts.length; i++) {
        const item = parts[i].trim();
        if (item.startsWith(prefix)) {
            return decodeURIComponent(item.substring(prefix.length));
        }
    }
    return '';
}

function buildAuthHeaders(includeJSONContentType) {
    if (typeof getAuthHeaders === 'function') {
        const headers = getAuthHeaders();
        if (!includeJSONContentType) {
            delete headers['Content-Type'];
        }
        return headers;
    }

    const headers = {};
    const token = localStorage.getItem('authToken') || readAuthCookie('authToken');
    if (token) {
        headers.Authorization = token.startsWith('Bearer ') ? token : 'Bearer ' + token;
    }
    if (includeJSONContentType) {
        headers['Content-Type'] = 'application/json';
    }
    return headers;
}

// =====================================================
// Initialization
// =====================================================
document.addEventListener('DOMContentLoaded', () => {
    loadDashboardList();
    loadWidgetTypes();
});

async function loadDashboardList() {
    try {
        const res = await fetch(`${BACKEND}/api/v1/analytics/dashboards`, {
            headers: buildAuthHeaders(false)
        });
        const data = await res.json();
        const sel = document.getElementById('dashboardSelector');
        sel.innerHTML = '<option value="">Select Dashboard...</option>';
        data.forEach(d => {
            const opt = document.createElement('option');
            opt.value = d.id;
            opt.textContent = `${d.name} (${d.widgetCount} widgets)`;
            sel.appendChild(opt);
        });
    } catch (e) {
        console.error('Failed to load dashboards:', e);
    }
}

async function loadWidgetTypes() {
    try {
        const res = await fetch(`${BACKEND}/api/v1/analytics/widget-types`, {
            headers: buildAuthHeaders(false)
        });
        widgetTypes = await res.json();
    } catch (e) {
        console.error('Failed to load widget types:', e);
    }
}

// =====================================================
// Dashboard Loading
// =====================================================
async function loadDashboard(id) {
    if (!id) {
        currentDashboard = null;
        currentDashboardId = '';
        document.getElementById('widgetGrid').innerHTML = `
            <div class="analytics-empty" id="emptyState">
                <div class="analytics-empty-icon">📊</div>
                <h3>Select a Dashboard</h3>
                <p>Choose a dashboard from the dropdown above to view analytics</p>
            </div>`;
        document.getElementById('filterBar').style.display = 'none';
        document.getElementById('dashboardDesc').textContent = '';
        return;
    }
    try {
        const res = await fetch(`${BACKEND}/api/v1/analytics/dashboards/${encodeURIComponent(id)}`, {
            headers: buildAuthHeaders(false)
        });
        if (!res.ok) throw new Error('Dashboard not found');
        currentDashboard = await res.json();
        currentDashboardId = id;
        document.getElementById('dashboardDesc').textContent = currentDashboard.description || '';
        renderFilters(currentDashboard.filters);
        renderWidgets(currentDashboard.widgets);
        buildExportWidgetList(currentDashboard.widgets);
    } catch (e) {
        console.error('Failed to load dashboard:', e);
        document.getElementById('widgetGrid').innerHTML = `<div class="analytics-empty"><div class="analytics-empty-icon">⚠️</div><h3>Error</h3><p>${e.message}</p></div>`;
    }
}

function refreshDashboard() {
    if (currentDashboardId) loadDashboard(currentDashboardId);
}

// =====================================================
// Filters
// =====================================================
function renderFilters(filters) {
    const bar = document.getElementById('filterBar');
    const container = document.getElementById('filterContainer');
    if (!filters || filters.length === 0) {
        bar.style.display = 'none';
        return;
    }
    bar.style.display = 'flex';
    container.innerHTML = '';
    filters.forEach(f => {
        const wrap = document.createElement('div');
        wrap.className = 'analytics-filter-item';
        if (f.type === 'select' || f.type === 'multi-select') {
            wrap.innerHTML = `
                <label>${f.label}</label>
                <select class="analytics-select analytics-select-sm" data-filter-key="${f.key}" ${f.type === 'multi-select' ? 'multiple' : ''}>
                    ${(f.options || []).map(o => `<option value="${o.value}" ${o.value === f.default ? 'selected' : ''}>${o.label}</option>`).join('')}
                </select>`;
        } else if (f.type === 'date-range') {
            wrap.innerHTML = `
                <label>${f.label}</label>
                <input type="date" class="analytics-input analytics-input-sm" data-filter-key="${f.key}-start" />
                <span>to</span>
                <input type="date" class="analytics-input analytics-input-sm" data-filter-key="${f.key}-end" />`;
        } else if (f.type === 'search') {
            wrap.innerHTML = `
                <label>${f.label}</label>
                <input type="text" class="analytics-input analytics-input-sm" data-filter-key="${f.key}" placeholder="Search..." />`;
        }
        container.appendChild(wrap);
    });
}

function applyDashFilters() {
    // Filters are client-side visual state — re-render with current dashboard
    if (currentDashboard) renderWidgets(currentDashboard.widgets);
}

function resetDashFilters() {
    document.querySelectorAll('#filterContainer select, #filterContainer input').forEach(el => {
        if (el.tagName === 'SELECT') el.selectedIndex = 0;
        else el.value = '';
    });
}

// =====================================================
// Widget Grid Rendering
// =====================================================
function renderWidgets(widgets) {
    // Destroy old charts
    Object.values(chartInstances).forEach(c => { try { c.destroy(); } catch (_) {} });
    chartInstances = {};

    const grid = document.getElementById('widgetGrid');
    if (!widgets || widgets.length === 0) {
        grid.innerHTML = '<div class="analytics-empty"><div class="analytics-empty-icon">📭</div><h3>No Widgets</h3><p>This dashboard has no widgets yet</p></div>';
        return;
    }

    const sorted = [...widgets].sort((a, b) => a.order - b.order);
    grid.innerHTML = '';

    sorted.forEach(w => {
        const card = document.createElement('div');
        card.className = `analytics-widget analytics-widget-${w.type}`;
        card.style.gridColumn = `span ${Math.min(w.width || 6, 12)}`;
        card.style.gridRow = `span ${Math.min(w.height || 1, 4)}`;
        card.dataset.widgetId = w.id;

        const editBtns = editMode ? `
            <div class="analytics-widget-actions">
                <button class="analytics-btn-xs" onclick="openWidgetEditor('${w.id}')" title="Edit">⚙️</button>
            </div>` : '';

        card.innerHTML = `
            <div class="analytics-widget-header">
                <span class="analytics-widget-title">${w.title}</span>
                <span class="analytics-widget-type-badge">${getWidgetIcon(w.type)} ${w.type.toUpperCase()}</span>
                ${editBtns}
            </div>
            <div class="analytics-widget-body" id="widget-body-${w.id}"></div>`;
        grid.appendChild(card);

        // Render each widget type
        requestAnimationFrame(() => renderWidgetContent(w));
    });
}

function getWidgetIcon(type) {
    const icons = {
        bar: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><rect x="3" y="12" width="4" height="9" rx="1"/><rect x="10" y="7" width="4" height="14" rx="1"/><rect x="17" y="3" width="4" height="18" rx="1"/></svg>',
        line: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><polyline points="3 17 9 11 13 15 21 7"/><polyline points="17 7 21 7 21 11"/></svg>',
        area: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><path d="M3 20l6-8 4 4 8-10v14H3z" fill="currentColor" opacity="0.15"/><polyline points="3 12 9 4 13 8 21 2"/></svg>',
        pie: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><path d="M12 2a10 10 0 1 0 10 10h-10V2z"/><path d="M20 12A8 8 0 0 0 12 4v8h8z"/></svg>',
        doughnut: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="4"/><path d="M12 3v4"/><path d="M21 12h-4"/></svg>',
        radar: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><polygon points="12 2 20 8 18 18 6 18 4 8"/><polygon points="12 6 16 9 15 15 9 15 8 9" opacity="0.4"/><circle cx="12" cy="12" r="1" fill="currentColor"/></svg>',
        scatter: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><path d="M3 3v18h18"/><circle cx="8" cy="14" r="1.5" fill="currentColor"/><circle cx="12" cy="9" r="1.5" fill="currentColor"/><circle cx="16" cy="12" r="1.5" fill="currentColor"/><circle cx="14" cy="6" r="1.5" fill="currentColor"/><circle cx="18" cy="8" r="1.5" fill="currentColor"/></svg>',
        heatmap: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><rect x="3" y="3" width="5" height="5" rx="1" fill="currentColor" opacity="0.8"/><rect x="10" y="3" width="5" height="5" rx="1" fill="currentColor" opacity="0.4"/><rect x="17" y="3" width="5" height="5" rx="1" fill="currentColor" opacity="0.6"/><rect x="3" y="10" width="5" height="5" rx="1" fill="currentColor" opacity="0.3"/><rect x="10" y="10" width="5" height="5" rx="1" fill="currentColor" opacity="0.9"/><rect x="17" y="10" width="5" height="5" rx="1" fill="currentColor" opacity="0.5"/><rect x="3" y="17" width="5" height="5" rx="1" fill="currentColor" opacity="0.6"/><rect x="10" y="17" width="5" height="5" rx="1" fill="currentColor" opacity="0.2"/><rect x="17" y="17" width="5" height="5" rx="1" fill="currentColor" opacity="0.7"/></svg>',
        funnel: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><path d="M3 4h18l-6 7v6l-4 3V11L3 4z"/></svg>',
        gauge: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><path d="M12 21a9 9 0 1 1 0-18 9 9 0 0 1 0 18z"/><path d="M12 12l4-4"/><circle cx="12" cy="12" r="1.5" fill="currentColor"/><path d="M5 17h14" opacity="0.3"/></svg>',
        kpi: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><rect x="3" y="3" width="18" height="18" rx="3"/><path d="M8 12h8"/><path d="M12 8v8"/><circle cx="12" cy="12" r="3" opacity="0.3"/></svg>',
        table: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18"/><path d="M3 15h18"/><path d="M9 3v18"/></svg>',
        log: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" width="18" height="18"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M7 8h10"/><path d="M7 12h7"/><path d="M7 16h10"/></svg>'
    };
    return icons[type] || icons.bar;
}

// =====================================================
// Widget Content Renderers
// =====================================================
function renderWidgetContent(widget) {
    const container = document.getElementById(`widget-body-${widget.id}`);
    if (!container) return;

    switch (widget.type) {
        case 'kpi':     renderKPI(container, widget); break;
        case 'gauge':   renderGauge(container, widget); break;
        case 'table':   renderTable(container, widget); break;
        case 'log':     renderLog(container, widget); break;
        case 'heatmap': renderHeatmap(container, widget); break;
        case 'funnel':  renderFunnel(container, widget); break;
        default:        renderChart(container, widget); break;
    }
}

// --- KPI Card ---
function renderKPI(container, widget) {
    const color = (widget.config.colors && widget.config.colors[0]) || '#3b82f6';
    const subtitle = (widget.data.labels && widget.data.labels[0]) || '';
    container.innerHTML = `
        <div class="kpi-card" style="--kpi-color: ${color}">
            <div class="kpi-value">${widget.data.value || '—'}</div>
            <div class="kpi-subtitle">${subtitle}</div>
        </div>`;
}

// --- Gauge ---
function renderGauge(container, widget) {
    const value = typeof widget.data.value === 'number' ? widget.data.value : 0;
    const max = widget.data.max || 100;
    const pct = Math.min(value / max, 1);
    const angle = pct * 180;
    const colors = widget.config.colors || ['#ef4444', '#f59e0b', '#10b981'];

    let gaugeColor = colors[0];
    if (pct > 0.66) gaugeColor = colors[2] || colors[0];
    else if (pct > 0.33) gaugeColor = colors[1] || colors[0];

    container.innerHTML = `
        <div class="gauge-container">
            <svg viewBox="0 0 200 120" class="gauge-svg">
                <path d="M 20 100 A 80 80 0 0 1 180 100" fill="none" stroke="var(--analytics-border)" stroke-width="16" stroke-linecap="round"/>
                <path d="M 20 100 A 80 80 0 0 1 180 100" fill="none" stroke="${gaugeColor}" stroke-width="16" stroke-linecap="round"
                    stroke-dasharray="${2.51 * angle} 999" class="gauge-fill"/>
                <text x="100" y="90" text-anchor="middle" class="gauge-value-text">${value}${max === 100 ? '%' : ''}</text>
                <text x="100" y="110" text-anchor="middle" class="gauge-label-text">${widget.data.labels ? widget.data.labels[Math.min(Math.floor(pct * (widget.data.labels.length)), widget.data.labels.length - 1)] : ''}</text>
            </svg>
        </div>`;
}

// --- Table ---
function renderTable(container, widget) {
    const cols = widget.data.columns || [];
    const rows = widget.data.rows || [];
    if (cols.length === 0) { container.innerHTML = '<p class="analytics-muted">No columns defined</p>'; return; }

    let sortCol = null, sortDir = 'asc';

    function render() {
        let sorted = [...rows];
        if (sortCol !== null) {
            sorted.sort((a, b) => {
                let va = a[sortCol], vb = b[sortCol];
                if (typeof va === 'number' && typeof vb === 'number') return sortDir === 'asc' ? va - vb : vb - va;
                return sortDir === 'asc' ? String(va).localeCompare(String(vb)) : String(vb).localeCompare(String(va));
            });
        }
        container.innerHTML = `
            <div class="analytics-table-wrap">
                <table class="analytics-table">
                    <thead><tr>${cols.map(c => `<th class="${c.sortable ? 'sortable' : ''}" data-col="${c.key}">${c.label} ${sortCol === c.key ? (sortDir === 'asc' ? '▲' : '▼') : ''}</th>`).join('')}</tr></thead>
                    <tbody>${sorted.map(r => `<tr>${cols.map(c => `<td class="col-${c.type}">${formatCellValue(r[c.key], c.type)}</td>`).join('')}</tr>`).join('')}</tbody>
                </table>
            </div>`;
        container.querySelectorAll('th.sortable').forEach(th => {
            th.addEventListener('click', () => {
                const col = th.dataset.col;
                if (sortCol === col) sortDir = sortDir === 'asc' ? 'desc' : 'asc';
                else { sortCol = col; sortDir = 'asc'; }
                render();
            });
        });
    }
    render();
}

function formatCellValue(val, type) {
    if (val == null) return '—';
    if (type === 'status') return `<span class="status-badge status-${val}">${val}</span>`;
    if (type === 'currency') return `<span class="currency-val">${val}</span>`;
    return val;
}

// --- Log Viewer ---
function renderLog(container, widget) {
    const entries = widget.data.entries || [];
    container.innerHTML = `
        <div class="log-viewer">
            ${entries.map(e => `
                <div class="log-entry log-${e.level}">
                    <span class="log-time">${e.timestamp}</span>
                    <span class="log-level">${e.level.toUpperCase()}</span>
                    <span class="log-source">${e.source}</span>
                    <span class="log-msg">${e.message}</span>
                </div>`).join('')}
        </div>`;
}

// --- Heatmap ---
function renderHeatmap(container, widget) {
    const labels = widget.data.labels || [];
    const datasets = widget.data.datasets || [];
    const colors = widget.config.colors || ['#eff6ff', '#3b82f6', '#1e3a5f'];

    // Find min/max
    let allVals = [];
    datasets.forEach(ds => ds.data.forEach(v => allVals.push(v)));
    const minV = Math.min(...allVals), maxV = Math.max(...allVals);

    function getColor(val) {
        const t = maxV > minV ? (val - minV) / (maxV - minV) : 0;
        // Interpolate between colors[0], [1], [2]
        if (colors.length < 3) return colors[0] || '#3b82f6';
        const c1 = hexToRgb(t < 0.5 ? colors[0] : colors[1]);
        const c2 = hexToRgb(t < 0.5 ? colors[1] : colors[2]);
        const localT = t < 0.5 ? t * 2 : (t - 0.5) * 2;
        const r = Math.round(c1.r + (c2.r - c1.r) * localT);
        const g = Math.round(c1.g + (c2.g - c1.g) * localT);
        const b = Math.round(c1.b + (c2.b - c1.b) * localT);
        return `rgb(${r},${g},${b})`;
    }

    container.innerHTML = `
        <div class="heatmap-container">
            <div class="heatmap-grid" style="grid-template-columns: auto repeat(${labels.length}, 1fr)">
                <div class="heatmap-corner"></div>
                ${labels.map(l => `<div class="heatmap-col-label">${l}</div>`).join('')}
                ${datasets.map(ds => `
                    <div class="heatmap-row-label">${ds.label}</div>
                    ${ds.data.map(v => `<div class="heatmap-cell" style="background:${getColor(v)}" title="${ds.label} @ ${labels[ds.data.indexOf(v)]}: ${v}">${v}</div>`).join('')}
                `).join('')}
            </div>
        </div>`;
}

function hexToRgb(hex) {
    hex = hex.replace('#', '');
    if (hex.length === 3) hex = hex.split('').map(c => c + c).join('');
    return { r: parseInt(hex.substring(0, 2), 16), g: parseInt(hex.substring(2, 4), 16), b: parseInt(hex.substring(4, 6), 16) };
}

// --- Funnel ---
function renderFunnel(container, widget) {
    const labels = widget.data.labels || [];
    const vals = (widget.data.datasets && widget.data.datasets[0]) ? widget.data.datasets[0].data : [];
    const colors = widget.config.colors || ['#1e3a5f', '#2563eb', '#3b82f6', '#60a5fa', '#93c5fd'];
    const maxVal = Math.max(...vals, 1);

    container.innerHTML = `
        <div class="funnel-container">
            ${labels.map((l, i) => {
                const pct = (vals[i] || 0) / maxVal * 100;
                const color = colors[i % colors.length];
                return `<div class="funnel-step">
                    <div class="funnel-bar" style="width:${pct}%; background:${color}">
                        <span class="funnel-label">${l}</span>
                        <span class="funnel-val">${(vals[i] || 0).toLocaleString()}</span>
                    </div>
                </div>`;
            }).join('')}
        </div>`;
}

// --- Charts (bar, line, area, pie, doughnut, radar, scatter) ---
function renderChart(container, widget) {
    const canvas = document.createElement('canvas');
    canvas.id = `chart-${widget.id}`;
    container.innerHTML = '';
    container.appendChild(canvas);

    const cfg = widget.config || {};
    const data = widget.data || {};
    const type = widget.type === 'area' ? 'line' : widget.type;

    const datasets = (data.datasets || []).map((ds, idx) => {
        const d = { ...ds };
        if (widget.type === 'area') {
            d.fill = true;
            if (!d.tension) d.tension = 0.4;
        }
        // For pie/doughnut ensure backgroundColor is array matching data length
        if ((type === 'pie' || type === 'doughnut') && d.backgroundColor && d.backgroundColor.length > 0) {
            // Already set
        } else if (type !== 'pie' && type !== 'doughnut' && type !== 'radar') {
            if (d.backgroundColor && d.backgroundColor.length === 1) {
                d.backgroundColor = d.backgroundColor[0];
            }
        }
        if (!d.borderWidth) d.borderWidth = type === 'bar' ? 1 : 2;
        return d;
    });

    const chartConfig = {
        type: type,
        data: {
            labels: data.labels || [],
            datasets: datasets
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: cfg.animation !== false ? { duration: 800 } : false,
            plugins: {
                legend: {
                    display: cfg.showLegend !== false,
                    position: 'top',
                    labels: { color: getComputedStyle(document.documentElement).getPropertyValue('--analytics-text').trim() || '#e0e0e0', font: { size: 11 } }
                },
                tooltip: { enabled: true }
            }
        }
    };

    // Axis config for cartesian charts
    if (['bar', 'line'].includes(type) || widget.type === 'area') {
        chartConfig.options.scales = {
            x: {
                display: true,
                stacked: cfg.stacked || false,
                title: { display: !!cfg.xAxis, text: cfg.xAxis || '' },
                grid: { display: cfg.showGrid !== false, color: 'rgba(255,255,255,0.06)' },
                ticks: { color: getComputedStyle(document.documentElement).getPropertyValue('--analytics-text-muted').trim() || '#999' }
            },
            y: {
                display: true,
                stacked: cfg.stacked || false,
                title: { display: !!cfg.yAxis, text: cfg.yAxis || '' },
                grid: { display: cfg.showGrid !== false, color: 'rgba(255,255,255,0.06)' },
                ticks: { color: getComputedStyle(document.documentElement).getPropertyValue('--analytics-text-muted').trim() || '#999' }
            }
        };
    }

    // Radar chart scales
    if (type === 'radar') {
        chartConfig.options.scales = {
            r: {
                angleLines: { color: 'rgba(255,255,255,0.1)' },
                grid: { color: 'rgba(255,255,255,0.1)' },
                pointLabels: { color: getComputedStyle(document.documentElement).getPropertyValue('--analytics-text').trim() || '#e0e0e0', font: { size: 11 } },
                ticks: { display: false }
            }
        };
    }

    try {
        chartInstances[widget.id] = new window.__axc(canvas, chartConfig);
    } catch (e) {
        container.innerHTML = `<p class="analytics-muted">Chart rendering failed: ${e.message}</p>`;
    }
}

// =====================================================
// Edit Mode
// =====================================================
function toggleEditMode() {
    editMode = !editMode;
    document.getElementById('editModeIcon').textContent = editMode ? '✅' : '✏️';
    document.getElementById('editModeLabel').textContent = editMode ? 'Done' : 'Edit';
    document.querySelector('.analytics-container').classList.toggle('edit-mode', editMode);
    if (currentDashboard) renderWidgets(currentDashboard.widgets);
}

function openWidgetEditor(widgetId) {
    const widget = currentDashboard.widgets.find(w => w.id === widgetId);
    if (!widget) return;
    editingWidgetId = widgetId;

    document.getElementById('editorTitle').value = widget.title;
    document.getElementById('editorWidth').value = widget.width || 6;
    document.getElementById('editorWidthVal').textContent = widget.width || 6;
    document.getElementById('editorHeight').value = widget.height || 2;
    document.getElementById('editorHeightVal').textContent = widget.height || 2;
    document.getElementById('editorShowLegend').checked = widget.config.showLegend !== false;
    document.getElementById('editorShowGrid').checked = widget.config.showGrid !== false;
    document.getElementById('editorColors').value = (widget.config.colors || []).join(', ');

    // Render type grid
    const grid = document.getElementById('editorTypeGrid');
    grid.innerHTML = widgetTypes.map(t =>
        `<button class="analytics-type-btn ${t.type === widget.type ? 'active' : ''}" data-type="${t.type}" onclick="selectEditorType(this, '${t.type}')">
            <span class="type-icon">${t.icon}</span>
            <span class="type-label">${t.label}</span>
        </button>`
    ).join('');

    document.getElementById('widgetEditorOverlay').style.display = 'flex';
}

function selectEditorType(btn, type) {
    document.querySelectorAll('#editorTypeGrid .analytics-type-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
}

function closeWidgetEditor(event) {
    if (event && event.target !== document.getElementById('widgetEditorOverlay')) return;
    document.getElementById('widgetEditorOverlay').style.display = 'none';
    editingWidgetId = null;
}

async function saveWidgetEdits() {
    if (!editingWidgetId || !currentDashboardId) return;

    const selectedType = document.querySelector('#editorTypeGrid .analytics-type-btn.active');
    const body = {
        title: document.getElementById('editorTitle').value,
        type: selectedType ? selectedType.dataset.type : undefined,
        width: parseInt(document.getElementById('editorWidth').value),
        height: parseInt(document.getElementById('editorHeight').value),
        config: {
            showLegend: document.getElementById('editorShowLegend').checked,
            showGrid: document.getElementById('editorShowGrid').checked,
            colors: document.getElementById('editorColors').value.split(',').map(c => c.trim()).filter(c => c),
            animation: true
        }
    };

    try {
        const res = await fetch(`${BACKEND}/api/v1/analytics/dashboards/${encodeURIComponent(currentDashboardId)}/widgets/${encodeURIComponent(editingWidgetId)}`, {
            method: 'PUT',
            headers: buildAuthHeaders(true),
            body: JSON.stringify(body)
        });
        if (!res.ok) throw new Error('Save failed');
        // Reload dashboard to reflect changes
        await loadDashboard(currentDashboardId);
        document.getElementById('widgetEditorOverlay').style.display = 'none';
        editingWidgetId = null;
    } catch (e) {
        alert('Failed to save widget: ' + e.message);
    }
}

// =====================================================
// Export
// =====================================================
function toggleExportMenu() {
    const menu = document.getElementById('exportMenu');
    menu.classList.toggle('show');
    // Close on outside click
    setTimeout(() => {
        document.addEventListener('click', function handler(e) {
            if (!document.getElementById('exportDropdown').contains(e.target)) {
                menu.classList.remove('show');
                document.removeEventListener('click', handler);
            }
        });
    }, 0);
}

function buildExportWidgetList(widgets) {
    const list = document.getElementById('exportWidgetList');
    if (!list) return;
    list.innerHTML = (widgets || [])
        .filter(w => w.type === 'table' || w.type === 'log' || w.data.datasets)
        .map(w => `<div class="analytics-dropdown-item" onclick="exportWidgetCSV('${w.id}')">${getWidgetIcon(w.type)} ${w.title}</div>`)
        .join('');
}

function exportWidgetCSV(widgetId) {
    if (!currentDashboardId) return;
    window.open(`${BACKEND}/api/v1/analytics/dashboards/${encodeURIComponent(currentDashboardId)}/widgets/${encodeURIComponent(widgetId)}/export`, '_blank');
    document.getElementById('exportMenu').classList.remove('show');
}

function exportAllCSV() {
    if (!currentDashboard) return;
    // Export each widget sequentially
    currentDashboard.widgets.forEach(w => {
        if (w.type !== 'kpi' && w.type !== 'gauge') {
            exportWidgetCSV(w.id);
        }
    });
    document.getElementById('exportMenu').classList.remove('show');
}

function exportAllXLSX() {
    if (!currentDashboard) return;
    // Build a combined CSV with all widget data as XLSX placeholder
    // (Full XLSX generation would require a library; we provide CSV with .xlsx-like formatting)
    let csvContent = '';
    currentDashboard.widgets.forEach(w => {
        if (w.type === 'kpi' || w.type === 'gauge') return;
        csvContent += `\n=== ${w.title} (${w.type}) ===\n`;
        if (w.type === 'table' && w.data.columns) {
            csvContent += w.data.columns.map(c => c.label).join(',') + '\n';
            (w.data.rows || []).forEach(r => {
                csvContent += w.data.columns.map(c => `"${r[c.key] || ''}"`).join(',') + '\n';
            });
        } else if (w.type === 'log' && w.data.entries) {
            csvContent += 'Timestamp,Level,Source,Message\n';
            w.data.entries.forEach(e => {
                csvContent += `"${e.timestamp}","${e.level}","${e.source}","${e.message}"\n`;
            });
        } else if (w.data.labels && w.data.datasets) {
            const header = ['Label', ...(w.data.datasets || []).map(ds => ds.label)];
            csvContent += header.join(',') + '\n';
            (w.data.labels || []).forEach((l, i) => {
                const row = [`"${l}"`, ...(w.data.datasets || []).map(ds => ds.data[i] !== undefined ? ds.data[i] : '')];
                csvContent += row.join(',') + '\n';
            });
        }
    });

    const blob = new Blob([csvContent], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${currentDashboard.name.replace(/\s+/g, '_')}_export.csv`;
    a.click();
    URL.revokeObjectURL(url);
    document.getElementById('exportMenu').classList.remove('show');
}
