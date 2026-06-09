/* ===================================================
   Network Intelligence Dashboard — Logic
   =================================================== */
(function() {
    'use strict';
    const API = (typeof window.resolveBackendURL === 'function') ? window.resolveBackendURL() : (window.BACKEND_URL || 'http://localhost:8000');
    const BASE = API + '/api/v1/netintel';

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

    // State
    let charts = {};
    let heatmapInstance = null;
    let heatmapLayer = null;
    let topoData = null;

    async function fetchJSON(url) {
        try {
            const res = await fetch(url, {
                headers: buildAuthHeaders(false)
            });
            if (!res.ok) throw new Error(res.statusText);
            return await res.json();
        } catch (e) {
            console.error('Fetch error:', url, e);
            return null;
        }
    }

    async function postJSON(url, body) {
        try {
            const res = await fetch(url, {
                method: 'POST',
                headers: buildAuthHeaders(true),
                body: JSON.stringify(body)
            });
            return await res.json();
        } catch (e) {
            console.error('Post error:', url, e);
            return null;
        }
    }

    // ==================
    // Tab Switching
    // ==================
    function switchTab(tab) {
        document.querySelectorAll('.ni-tab').forEach(t => t.classList.remove('active'));
        document.querySelectorAll('.ni-panel').forEach(p => p.classList.remove('active'));
        const tabBtn = document.querySelector(`.ni-tab[data-tab="${tab}"]`);
        const panel = document.getElementById('panel-' + tab);
        if (tabBtn) tabBtn.classList.add('active');
        if (panel) panel.classList.add('active');

        // Load data on tab switch
        switch (tab) {
            case 'overview': loadOverview(); break;
            case 'logs': loadLogs(); break;
            case 'topology': loadTopology(); break;
            case 'heatmap': loadHeatmap(); break;
            case 'trends': loadTrends(); loadForecasts(); break;
            case 'alerts': loadAlerts(); loadAnomalies(); break;
            case 'predictions': loadPredictions(); loadTracks(); break;
            case 'parsers': loadParsers(); loadLogTypes(); break;
        }
    }

    // ==================
    // Overview
    // ==================
    async function loadOverview() {
        const data = await fetchJSON(BASE + '/summary');
        if (!data || data.status !== 'success') return;
        const s = data.summary;

        setText('kpi-devices', s.active_devices);
        setText('kpi-entries', formatNum(s.total_entries));
        setText('kpi-alerts', s.active_alerts);
        setText('kpi-anomalies', s.active_anomalies);
        setText('kpi-accuracy', (s.prediction_accuracy * 100).toFixed(0) + '%');
        setText('kpi-signal', s.metrics.avg_signal_dbm + ' dBm');

        // Alert badge
        const badge = document.getElementById('alertBadge');
        if (badge) {
            badge.textContent = s.active_alerts;
            badge.style.display = s.active_alerts > 0 ? 'inline-block' : 'none';
        }

        // Log distribution chart
        const stats = await fetchJSON(BASE + '/logs/stats');
        if (stats && stats.status === 'success') {
            renderDoughnut('chartLogTypes', stats.stats.by_type);
        }

        // Severity chart
        if (s.severity_breakdown) {
            renderBarChart('chartSeverity', s.severity_breakdown,
                { critical: '#ef4444', high: '#f59e0b', medium: '#eab308', low: '#22c55e' });
        }

        // Traffic trend
        const trend = await fetchJSON(BASE + '/trends?metric=traffic&hours=24');
        if (trend && trend.status === 'success') {
            renderLineChart('chartTrafficTrend', trend.trend, 'Traffic (bytes)');
        }

        // Top devices
        renderTopDevices(s.top_devices || []);

        // Zone activity
        renderZoneBars(s.zone_activity || {});
    }

    function renderTopDevices(devices) {
        const body = document.getElementById('topDevicesBody');
        if (!body) return;
        body.innerHTML = devices.map(d => {
            const riskPct = (d.risk_score * 100).toFixed(0);
            const riskClass = d.risk_score < 0.3 ? 'low' : d.risk_score < 0.7 ? 'med' : 'high';
            return `<tr>
                <td><code>${d.mac}</code></td>
                <td>${d.name || '—'}</td>
                <td>${d.entries}</td>
                <td>${d.avg_signal_dbm} dBm</td>
                <td>${d.current_zone || '—'}</td>
                <td><div class="ni-risk"><div class="ni-risk-fill ni-risk-${riskClass}" style="width:${riskPct}%"></div></div> ${riskPct}%</td>
            </tr>`;
        }).join('');
    }

    function renderZoneBars(zones) {
        const el = document.getElementById('zoneBars');
        if (!el) return;
        const max = Math.max(1, ...Object.values(zones));
        el.innerHTML = Object.entries(zones)
            .sort((a, b) => b[1] - a[1])
            .map(([zone, count]) => {
                const h = Math.max(8, (count / max) * 80);
                return `<div class="ni-zone-bar">
                    <div class="ni-zone-bar-value">${count}</div>
                    <div class="ni-zone-bar-fill" style="height:${h}px"></div>
                    <div class="ni-zone-bar-label">${zone}</div>
                </div>`;
            }).join('');
    }

    // ==================
    // Log Explorer
    // ==================
    async function loadLogs() {
        const type = val('logTypeFilter');
        const severity = val('logSeverityFilter');
        const limit = val('logLimitFilter') || '200';
        let url = `${BASE}/logs?limit=${limit}`;
        if (type) url += `&type=${type}`;
        if (severity) url += `&severity=${severity}`;

        const data = await fetchJSON(url);
        if (!data || data.status !== 'success') return;

        const statsEl = document.getElementById('logStats');
        if (statsEl) {
            statsEl.innerHTML = `<span>Showing <strong>${data.total}</strong> entries</span>`;
        }

        const body = document.getElementById('logTableBody');
        if (!body) return;
        body.innerHTML = data.entries.map(e => `<tr>
            <td>${fmtTime(e.timestamp)}</td>
            <td><span class="ni-logtype">${e.log_type}</span></td>
            <td><span class="ni-sev ni-sev-${e.severity || 'info'}">${e.severity || 'info'}</span></td>
            <td>${e.source}</td>
            <td title="${esc(e.message)}">${truncate(e.message, 80)}</td>
            <td><code>${e.device_mac || e.src_ip || '—'}</code></td>
        </tr>`).join('');
    }

    function exportLogs() {
        const table = document.querySelector('#panel-logs .ni-table');
        if (!table) return;
        const rows = table.querySelectorAll('tr');
        let csv = '';
        rows.forEach(r => {
            const cells = r.querySelectorAll('th, td');
            csv += Array.from(cells).map(c => '"' + c.textContent.replace(/"/g, '""') + '"').join(',') + '\n';
        });
        const blob = new Blob([csv], { type: 'text/csv' });
        const a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = 'netintel-logs.csv';
        a.click();
    }

    // ==================
    // Topology
    // ==================
    async function loadTopology() {
        const data = await fetchJSON(BASE + '/topology');
        if (!data || data.status !== 'success') return;
        topoData = data.topology;

        const stats = topoData.stats;
        const statsEl = document.getElementById('topoStats');
        if (statsEl) {
            statsEl.innerHTML = `<span>Nodes: <strong>${stats.total_nodes}</strong></span>
                <span>Edges: <strong>${stats.total_edges}</strong></span>
                <span>Online: <strong>${stats.online_nodes}</strong></span>
                <span>Offline: <strong>${stats.offline_nodes}</strong></span>
                <span>Degraded Edges: <strong>${stats.degraded_edges}</strong></span>`;
        }

        drawTopology();
    }

    const nodeColors = {
        router: '#3b82f6', switch: '#10b981', access_point: '#f59e0b',
        firewall: '#ef4444', server: '#8b5cf6', sensor: '#06b6d4',
        gateway: '#ec4899', device: '#64748b', cloud: '#a855f7'
    };

    function drawTopology() {
        const canvas = document.getElementById('topoCanvas');
        if (!canvas || !topoData) return;
        const ctx = canvas.getContext('2d');
        const W = canvas.width;
        const H = canvas.height;
        ctx.clearRect(0, 0, W, H);

        const nodeMap = {};
        topoData.nodes.forEach(n => { nodeMap[n.id] = n; });

        // Draw edges
        topoData.edges.forEach(e => {
            const src = nodeMap[e.source];
            const tgt = nodeMap[e.target];
            if (!src || !tgt) return;
            ctx.beginPath();
            ctx.moveTo(src.x, src.y);
            ctx.lineTo(tgt.x, tgt.y);
            ctx.strokeStyle = e.status === 'down' ? '#ef4444' : e.status === 'degraded' ? '#f59e0b' : '#475569';
            ctx.lineWidth = e.status === 'down' ? 1 : Math.max(1, Math.min(3, e.traffic_mbps / 100));
            if (e.status === 'down') ctx.setLineDash([4, 4]); else ctx.setLineDash([]);
            ctx.stroke();

            // Edge label
            if (e.label) {
                const mx = (src.x + tgt.x) / 2;
                const my = (src.y + tgt.y) / 2;
                ctx.font = '9px sans-serif';
                ctx.fillStyle = '#64748b';
                ctx.textAlign = 'center';
                ctx.fillText(e.label, mx, my - 4);
            }
        });

        // Draw nodes
        topoData.nodes.forEach(n => {
            const r = n.type === 'device' ? 10 : n.type === 'sensor' ? 11 : 14;
            const color = nodeColors[n.type] || '#64748b';

            // Glow
            ctx.beginPath();
            ctx.arc(n.x, n.y, r + 4, 0, Math.PI * 2);
            ctx.fillStyle = n.status === 'offline' ? 'rgba(239,68,68,0.15)' :
                            n.status === 'degraded' ? 'rgba(245,158,11,0.15)' :
                            color.replace(')', ',0.12)').replace('rgb', 'rgba');
            ctx.fill();

            // Node circle
            ctx.beginPath();
            ctx.arc(n.x, n.y, r, 0, Math.PI * 2);
            ctx.fillStyle = n.status === 'offline' ? '#374151' : color;
            ctx.fill();
            ctx.strokeStyle = n.status === 'offline' ? '#ef4444' : '#0f172a';
            ctx.lineWidth = 2;
            ctx.stroke();

            // Label
            ctx.font = '10px sans-serif';
            ctx.fillStyle = '#e2e8f0';
            ctx.textAlign = 'center';
            ctx.fillText(n.label, n.x, n.y + r + 14);
        });
    }

    // ==================
    // Heatmap
    // ==================
    async function loadHeatmap() {
        const category = val('heatmapCategory') || 'wifi_signal';
        const data = await fetchJSON(BASE + '/heatmap?category=' + category);
        if (!data || data.status !== 'success') return;

        const mapEl = document.getElementById('heatmapMap');
        if (!mapEl) return;

        if (!heatmapInstance) {
            heatmapInstance = window.__axm.map('heatmapMap').setView([23.8103, 90.4125], 16);
            window.__axm.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
                attribution: '© OpenStreetMap contributors',
                maxZoom: 20
            }).addTo(heatmapInstance);
        }

        if (heatmapLayer) {
            heatmapInstance.removeLayer(heatmapLayer);
        }

        // Render as circle markers with gradient
        const pts = data.heatmap.points || [];
        if (pts.length === 0) return;

        const group = [];
        pts.forEach(p => {
            const color = intensityColor(p.intensity);
            const circle = window.__axm.circleMarker([p.lat, p.lng], {
                radius: 6 + p.intensity * 8,
                fillColor: color,
                color: color,
                weight: 0.5,
                opacity: 0.7,
                fillOpacity: 0.5
            });
            let popup = `<b>Intensity:</b> ${(p.intensity * 100).toFixed(0)}%`;
            if (p.signal_dbm) popup += `<br><b>Signal:</b> ${p.signal_dbm} dBm`;
            if (p.device_mac) popup += `<br><b>Device:</b> ${p.device_mac}`;
            if (p.label) popup += `<br>${p.label}`;
            circle.bindPopup(popup);
            group.push(circle);
        });

        heatmapLayer = L.layerGroup(group).addTo(heatmapInstance);

        // Fit bounds
        if (pts.length > 0) {
            const bounds = pts.map(p => [p.lat, p.lng]);
            heatmapInstance.fitBounds(bounds, { padding: [30, 30] });
        }

        // Legend
        const legend = document.getElementById('heatmapLegend');
        if (legend) {
            legend.innerHTML = `<span>Points: <strong>${pts.length}</strong></span>
                <span>Category: <strong>${category.replace(/_/g, ' ')}</strong></span>
                <span style="display:flex;align-items:center;gap:4px">
                    Low <span style="display:inline-block;width:60px;height:8px;border-radius:4px;background:linear-gradient(to right,#22c55e,#eab308,#ef4444)"></span> High
                </span>`;
        }
    }

    function intensityColor(v) {
        if (v < 0.33) return '#22c55e';
        if (v < 0.66) return '#eab308';
        return '#ef4444';
    }

    // ==================
    // Trends & Forecasts
    // ==================
    async function loadTrends() {
        const metric = val('trendMetric') || 'traffic';
        const hours = val('trendHours') || '24';
        setText('trendTitle', metric.charAt(0).toUpperCase() + metric.slice(1) + ' Trend');

        const data = await fetchJSON(`${BASE}/trends?metric=${metric}&hours=${hours}`);
        if (!data || data.status !== 'success') return;
        renderLineChart('chartTrend', data.trend, metric);
    }

    async function loadForecasts() {
        const data = await fetchJSON(BASE + '/forecasts');
        if (!data || data.status !== 'success') return;

        const grid = document.getElementById('forecastGrid');
        if (!grid) return;

        grid.innerHTML = Object.entries(data.forecasts).map(([key, f]) => {
            const trendIcon = f.trend === 'increasing' ? '📈' : f.trend === 'decreasing' ? '📉' : f.trend === 'cyclic' ? '🔄' : '➡️';
            return `<div class="ni-forecast-card">
                <h4>${trendIcon} ${f.metric.replace(/_/g, ' ')}</h4>
                <div class="ni-forecast-meta">
                    <span>Trend: <strong>${f.trend}</strong></span>
                    <span>Confidence: <strong>${(f.confidence * 100).toFixed(0)}%</strong></span>
                    <span>Period: <strong>${f.period}</strong></span>
                </div>
            </div>`;
        }).join('');
    }

    // ==================
    // Alerts & Anomalies
    // ==================
    async function loadAlerts() {
        const status = val('alertStatusFilter');
        let url = BASE + '/alerts';
        if (status) url += '?status=' + status;
        const data = await fetchJSON(url);
        if (!data || data.status !== 'success') return;

        const list = document.getElementById('alertList');
        if (!list) return;

        list.innerHTML = data.alerts.map(a => `<div class="ni-alert-item ${a.severity}">
            <div class="ni-alert-body">
                <div class="ni-alert-title">
                    <span class="ni-sev ni-sev-${a.severity}">${a.severity}</span>
                    ${esc(a.title)}
                </div>
                <div class="ni-alert-msg">${esc(a.message)}</div>
                <div class="ni-alert-meta">
                    <span>Type: ${a.type}</span>
                    <span>Source: ${a.source}</span>
                    <span>${fmtTime(a.timestamp)}</span>
                    <span class="ni-status ni-status-${a.status}">${a.status}</span>
                    ${a.tags ? a.tags.map(t => `<span class="ni-logtype">${t}</span>`).join('') : ''}
                </div>
            </div>
            <div class="ni-alert-actions">
                ${a.status === 'active' ? `<button class="ni-btn ni-btn-sm" onclick="NI.ackAlert('${a.id}')">Ack</button>` : ''}
                ${a.status !== 'resolved' ? `<button class="ni-btn ni-btn-sm" onclick="NI.resolveAlert('${a.id}')">Resolve</button>` : ''}
            </div>
        </div>`).join('') || '<p style="color:var(--text-secondary)">No alerts found.</p>';
    }

    async function loadAnomalies() {
        const status = val('anomalyStatusFilter');
        const severity = val('anomalySeverityFilter');
        let url = BASE + '/anomalies?';
        if (status) url += 'status=' + status + '&';
        if (severity) url += 'severity=' + severity;
        const data = await fetchJSON(url);
        if (!data || data.status !== 'success') return;

        const list = document.getElementById('anomalyList');
        if (!list) return;

        list.innerHTML = data.anomalies.map(a => `<div class="ni-anomaly-item ${a.severity}">
            <div class="ni-anomaly-body">
                <div class="ni-anomaly-title">
                    <span class="ni-sev ni-sev-${a.severity}">${a.severity}</span>
                    ${a.type.replace(/_/g, ' ')} — Score: ${(a.score * 100).toFixed(0)}%
                </div>
                <div class="ni-anomaly-msg">${esc(a.description)}</div>
                <div class="ni-anomaly-meta">
                    <span>Source: ${a.source}</span>
                    <span>${fmtTime(a.timestamp)}</span>
                    <span class="ni-status ni-status-${a.status}">${a.status}</span>
                    ${a.device_mac ? `<span>Device: ${a.device_mac}</span>` : ''}
                    ${a.src_ip ? `<span>IP: ${a.src_ip}</span>` : ''}
                </div>
            </div>
            <div class="ni-anomaly-actions">
                ${a.status === 'active' ? `<button class="ni-btn ni-btn-sm" onclick="NI.ackAnomaly('${a.id}')">Ack</button>` : ''}
                ${a.status !== 'resolved' ? `<button class="ni-btn ni-btn-sm" onclick="NI.resolveAnomaly('${a.id}')">Resolve</button>` : ''}
            </div>
        </div>`).join('') || '<p style="color:var(--text-secondary)">No anomalies found.</p>';
    }

    async function ackAlert(id) {
        await postJSON(`${BASE}/alerts/${id}/acknowledge`, {});
        loadAlerts();
    }
    async function resolveAlert(id) {
        await postJSON(`${BASE}/alerts/${id}/resolve`, {});
        loadAlerts();
    }
    async function ackAnomaly(id) {
        await postJSON(`${BASE}/anomalies/${id}/acknowledge`, {});
        loadAnomalies();
    }
    async function resolveAnomaly(id) {
        await postJSON(`${BASE}/anomalies/${id}/resolve`, {});
        loadAnomalies();
    }

    // ==================
    // Predictions & Tracks
    // ==================
    async function loadPredictions() {
        const data = await fetchJSON(BASE + '/predictions');
        if (!data || data.status !== 'success') return;

        const grid = document.getElementById('predictionGrid');
        if (!grid) return;

        grid.innerHTML = data.predictions.map(p => {
            const confPct = (p.confidence * 100).toFixed(0);
            const confColor = p.confidence > 0.8 ? '#22c55e' : p.confidence > 0.6 ? '#eab308' : '#ef4444';
            return `<div class="ni-pred-card">
                <div class="ni-pred-header">
                    <span class="ni-pred-mac">${p.device_mac}</span>
                    <span class="ni-pred-conf" style="color:${confColor}">${confPct}% conf</span>
                </div>
                <div class="ni-pred-detail">
                    <strong>Zone:</strong> ${p.current_zone || '—'} <span class="ni-pred-arrow">→</span> ${p.predicted_zone || '—'}<br>
                    <strong>Direction:</strong> ${p.direction} &nbsp; <strong>Speed:</strong> ${p.speed_mps.toFixed(2)} m/s<br>
                    <strong>ETA:</strong> ${p.eta} &nbsp; <strong>Path points:</strong> ${(p.predicted_path || []).length}
                </div>
            </div>`;
        }).join('') || '<p style="color:var(--text-secondary)">No predictions available.</p>';
    }

    async function loadTracks() {
        const data = await fetchJSON(BASE + '/tracks');
        if (!data || data.status !== 'success') return;

        const body = document.getElementById('trackTableBody');
        if (!body) return;

        body.innerHTML = data.tracks.map(t => `<tr>
            <td><code>${t.device_mac}</code></td>
            <td>${t.device_name || '—'}</td>
            <td>${(t.zones_visited || []).join(', ')}</td>
            <td>${t.total_distance_m.toFixed(1)} m</td>
            <td>${t.avg_speed_mps.toFixed(3)} m/s</td>
            <td>${(t.points || []).length}</td>
            <td>${fmtTime(t.last_seen)}</td>
        </tr>`).join('');
    }

    // ==================
    // Parsers
    // ==================
    async function loadParsers() {
        const data = await fetchJSON(BASE + '/parsers');
        if (!data || data.status !== 'success') return;

        const body = document.getElementById('parserTableBody');
        if (!body) return;

        body.innerHTML = data.parsers.map(p => `<tr>
            <td><strong>${esc(p.name)}</strong></td>
            <td><span class="ni-logtype">${p.log_type}</span></td>
            <td>${p.source.type}: ${p.source.endpoint || '—'}</td>
            <td><span class="ni-status ni-status-${p.status}">${p.status}</span></td>
            <td>${formatNum(p.parsed_count)}</td>
            <td>${p.error_count || 0}</td>
            <td>
                ${p.status === 'active' ?
                    `<button class="ni-btn ni-btn-sm" onclick="NI.updateParser('${p.id}','paused')">Pause</button>` :
                    `<button class="ni-btn ni-btn-sm" onclick="NI.updateParser('${p.id}','active')">Resume</button>`}
                <button class="ni-btn ni-btn-sm" onclick="NI.deleteParser('${p.id}')">Delete</button>
            </td>
        </tr>`).join('');
    }

    async function loadLogTypes() {
        const data = await fetchJSON(BASE + '/log-types');
        if (!data || data.status !== 'success') return;

        const grid = document.getElementById('logTypesGrid');
        if (grid) {
            grid.innerHTML = data.log_types.map(t => `<div class="ni-logtype-card">
                <div class="ni-logtype-icon">${t.icon}</div>
                <div class="ni-logtype-info">
                    <h4>${esc(t.name)}</h4>
                    <p>${esc(t.description)}</p>
                    <span class="ni-logtype-badge">${t.category}</span>
                </div>
            </div>`).join('');
        }

        // Populate filter dropdowns
        const select = document.getElementById('logTypeFilter');
        const parserSelect = document.getElementById('parserLogType');
        if (select && select.options.length <= 1) {
            data.log_types.forEach(t => {
                select.add(new Option(t.name, t.id));
            });
        }
        if (parserSelect && parserSelect.options.length === 0) {
            data.log_types.forEach(t => {
                parserSelect.add(new Option(t.name, t.id));
            });
        }
    }

    function showCreateParser() {
        document.getElementById('parserModal').classList.add('show');
    }
    function closeParserModal() {
        document.getElementById('parserModal').classList.remove('show');
    }

    async function createParser() {
        const body = {
            name: val('parserName'),
            log_type: val('parserLogType'),
            description: val('parserDesc'),
            source: {
                type: val('parserSourceType'),
                endpoint: val('parserEndpoint')
            }
        };
        if (!body.name) { alert('Name is required'); return; }
        const res = await postJSON(BASE + '/parsers', body);
        if (res && res.status === 'success') {
            closeParserModal();
            loadParsers();
        }
    }

    async function updateParser(id, status) {
        await fetch(`${BASE}/parsers/${id}`, {
            method: 'PUT',
            headers: buildAuthHeaders(true),
            body: JSON.stringify({ status })
        });
        loadParsers();
    }

    async function deleteParser(id) {
        if (!confirm('Delete this parser?')) return;
        await fetch(`${BASE}/parsers/${id}`, {
            method: 'DELETE',
            headers: buildAuthHeaders(false)
        });
        loadParsers();
    }

    // ==================
    // Chart Helpers
    // ==================
    function renderDoughnut(canvasId, dataMap) {
        const canvas = document.getElementById(canvasId);
        if (!canvas) return;
        if (charts[canvasId]) charts[canvasId].destroy();

        const labels = Object.keys(dataMap);
        const values = Object.values(dataMap);
        const palette = ['#3b82f6','#10b981','#f59e0b','#ef4444','#8b5cf6','#06b6d4','#ec4899','#64748b','#a855f7','#22d3ee','#f97316','#84cc16'];

        charts[canvasId] = new window.__axc(canvas, {
            type: 'doughnut',
            data: {
                labels,
                datasets: [{ data: values, backgroundColor: palette.slice(0, labels.length), borderWidth: 0 }]
            },
            options: {
                responsive: true,
                plugins: {
                    legend: { position: 'right', labels: { color: '#94a3b8', font: { size: 11 } } }
                }
            }
        });
    }

    function renderBarChart(canvasId, dataMap, colorMap) {
        const canvas = document.getElementById(canvasId);
        if (!canvas) return;
        if (charts[canvasId]) charts[canvasId].destroy();

        const labels = Object.keys(dataMap);
        const values = Object.values(dataMap);
        const colors = labels.map(l => (colorMap && colorMap[l]) || '#3b82f6');

        charts[canvasId] = new window.__axc(canvas, {
            type: 'bar',
            data: {
                labels,
                datasets: [{ data: values, backgroundColor: colors, borderRadius: 4 }]
            },
            options: {
                responsive: true,
                plugins: { legend: { display: false } },
                scales: {
                    x: { ticks: { color: '#94a3b8' }, grid: { display: false } },
                    y: { ticks: { color: '#94a3b8' }, grid: { color: 'rgba(148,163,184,0.1)' } }
                }
            }
        });
    }

    function renderLineChart(canvasId, points, label) {
        const canvas = document.getElementById(canvasId);
        if (!canvas) return;
        if (charts[canvasId]) charts[canvasId].destroy();

        const labels = points.map(p => {
            const d = new Date(p.timestamp);
            return d.getHours() + ':00';
        });
        const values = points.map(p => p.value);

        charts[canvasId] = new window.__axc(canvas, {
            type: 'line',
            data: {
                labels,
                datasets: [{
                    label: label || 'Value',
                    data: values,
                    borderColor: '#3b82f6',
                    backgroundColor: 'rgba(59,130,246,0.1)',
                    fill: true,
                    tension: 0.3,
                    pointRadius: 2
                }]
            },
            options: {
                responsive: true,
                plugins: { legend: { labels: { color: '#94a3b8' } } },
                scales: {
                    x: { ticks: { color: '#94a3b8', maxTicksLimit: 12 }, grid: { color: 'rgba(148,163,184,0.08)' } },
                    y: { ticks: { color: '#94a3b8' }, grid: { color: 'rgba(148,163,184,0.08)' } }
                }
            }
        });
    }

    // ==================
    // Utility
    // ==================
    function setText(id, v) {
        const el = document.getElementById(id);
        if (el) el.textContent = v;
    }
    function val(id) {
        const el = document.getElementById(id);
        return el ? el.value : '';
    }
    function formatNum(n) {
        if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
        if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
        return String(n);
    }
    function fmtTime(ts) {
        if (!ts) return '—';
        const d = new Date(ts);
        return d.toLocaleString();
    }
    function truncate(s, n) {
        if (!s) return '';
        return s.length > n ? s.substring(0, n) + '…' : s;
    }
    function esc(s) {
        if (!s) return '';
        const d = document.createElement('div');
        d.textContent = s;
        return d.innerHTML;
    }

    // ==================
    // Init & Export
    // ==================
    window.NI = {
        switchTab, loadLogs, exportLogs,
        loadTopology, loadHeatmap, loadTrends,
        loadAlerts, loadAnomalies, loadPredictions,
        loadParsers, showCreateParser, closeParserModal,
        createParser, updateParser, deleteParser,
        ackAlert, resolveAlert, ackAnomaly, resolveAnomaly
    };

    // Auto-load overview on page load
    document.addEventListener('DOMContentLoaded', function() {
        if (document.querySelector('.netintel-container')) {
            loadOverview();
            loadLogTypes(); // populate dropdowns
        }
    });
})();
