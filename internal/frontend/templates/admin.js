// Admin Dashboard JS — API Builder, File-to-Dashboard, Dashboard↔GIS Converter, File Scanner
const BACKEND_URL = (() => {
    function normalizeURL(raw) {
        var value = String(raw || '').trim();
        if (!value) return '';
        if (value.length > 1 && value.endsWith('/')) {
            value = value.slice(0, -1);
        }
        return value;
    }

    function parseHostname(rawURL) {
        try {
            return new URL(rawURL).hostname.toLowerCase();
        } catch (_) {
            return '';
        }
    }

    function isLocalOrInternalHost(hostname) {
        if (!hostname) return true;
        if (hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '0.0.0.0' || hostname === '::1') return true;
        if (hostname === 'axiomnizam' || hostname.endsWith('.local')) return true;
        return hostname.indexOf('.') === -1;
    }

    function inferPublicBackendURL() {
        var browserHost = String(window.location.hostname || '').toLowerCase();
        if (!browserHost || browserHost === 'localhost' || browserHost === '127.0.0.1' || browserHost === '0.0.0.0') {
            return '';
        }
        var protocol = window.location.protocol || 'https:';
        if (browserHost.indexOf('axiomnizam.') === 0) {
            return protocol + '//axiomnizam-platform.' + browserHost.substring('axiomnizam.'.length);
        }
        return protocol + '//' + browserHost;
    }

    if (typeof window.resolveBackendURL === 'function') {
        var resolved = normalizeURL(window.resolveBackendURL());
        if (resolved) return resolved;
    }

    var value = normalizeURL(window.BACKEND_URL || '');
    if (value) {
        var browserHost = String(window.location.hostname || '').toLowerCase();
        var browserIsPublic = browserHost && browserHost !== 'localhost' && browserHost !== '127.0.0.1' && browserHost !== '0.0.0.0';
        var backendHost = parseHostname(value);
        if (browserIsPublic && isLocalOrInternalHost(backendHost)) {
            return inferPublicBackendURL() || value;
        }
        return value;
    }

    return inferPublicBackendURL() || 'http://localhost:8000';
})();

let filteredMethod = 'ALL';
let currentCSVUploadId = null;
let currentDashMappings = [];
let builderDataServers = [];
let customAPIById = {};
let graphQLAPIById = {};
let existingRESTEndpoints = [];
let existingGraphQLEndpoints = [];
let scanHistoryById = {};
let selectedScanReportId = '';
let graphQLFormMode = 'create';
let editingGraphQLApiId = '';
let latestAdminAPIScanReport = null;
let latestSQLAISuggestions = [];

const DEFAULT_BUILDER_DATABASES = ['mysql', 'postgres', 'mariadb', 'percona', 'oracle'];

console.log('Admin - Backend URL:', BACKEND_URL);

function getAuthHeaders() {
    const token = localStorage.getItem('authToken');
    const headers = { 'Content-Type': 'application/json' };
    if (token) headers['Authorization'] = 'Bearer ' + token;
    return headers;
}

function fetchJSON(path) {
    return fetch(BACKEND_URL + path, { headers: getAuthHeaders() }).then(function(r) { return r.json(); });
}
function postJSON(path, body) {
    return fetch(BACKEND_URL + path, { method: 'POST', headers: getAuthHeaders(), body: JSON.stringify(body) }).then(function(r) { return r.json(); });
}
function putJSON(path, body) {
    return fetch(BACKEND_URL + path, { method: 'PUT', headers: getAuthHeaders(), body: JSON.stringify(body) }).then(function(r) { return r.json(); });
}
function deleteJSON(path) {
    return fetch(BACKEND_URL + path, { method: 'DELETE', headers: getAuthHeaders() }).then(function(r) { return r.json(); });
}

// ===================================================================
// Tab switching & init
// ===================================================================
window.addEventListener('DOMContentLoaded', function() {
    var userName = localStorage.getItem('userName');
    if (userName) {
        var el = document.getElementById('adminUserName');
        if (el) el.textContent = userName;
    }
    loadBuilderSummary();
    loadCustomAPIs();
    loadRESTEndpointCatalog();
    loadGraphQLBuilderSummary();
    loadGraphQLCustomAPIs();
    loadGraphQLEndpointCatalog();
    bindRESTEndpointValidation();
    bindGraphQLEndpointValidation();
    loadBuilderDataSources();
    loadCSVHistory();
    loadAPIs();
    setupCSVDropZone();
    setupScanDropZone();
    loadScannerHealth();
    toggleAdminApiScanFields();
    initSQLAssistantPanel();
    applyRoleRestrictions();
    initAdminCertificateActions();
});

function switchTab(tabName) {
    var tabs = document.querySelectorAll('.tab-content');
    tabs.forEach(function(t) { t.classList.remove('active'); });
    var btns = document.querySelectorAll('.tab-btn');
    btns.forEach(function(b) { b.classList.remove('active'); });
    var sel = document.getElementById(tabName);
    if (sel) sel.classList.add('active');
    if (event && event.currentTarget) event.currentTarget.classList.add('active');

    if (tabName === 'api-builder') { loadBuilderSummary(); loadCustomAPIs(); initSQLAssistantPanel(); }
    if (tabName === 'graphql-api-builder') { loadGraphQLBuilderSummary(); loadGraphQLCustomAPIs(); }
    if (tabName === 'csv-upload') { loadCSVHistory(); }
    if (tabName === 'converter') { loadConverterDropdowns(); loadConversionHistory(); }
    if (tabName === 'file-scanner') { loadScanHistory(); loadScannerHealth(); }
    if (tabName === 'api-testing') { loadAPIs(); loadAdminApiScanReports(); toggleAdminApiScanFields(); }
    if (tabName === 'graphql-studio') { loadAdminGraphQLSchemaInfo(); }
    if (tabName === 'control-plane') { refreshAdminControlPlaneData(); }
    if (tabName === 'settings') { loadAdminCertificatePanel(); }
}

// ===================================================================
// API Builder
// ===================================================================
function loadBuilderSummary() {
    fetchJSON('/api/v1/builder/summary?api_type=rest').then(function(d) {
        var el = document.getElementById('builderSummary');
        if (!el) return;
        el.innerHTML =
            '<div class="summary-card"><div class="sc-value">' + (d.total_apis || 0) + '</div><div class="sc-label">Total APIs</div></div>' +
            '<div class="summary-card"><div class="sc-value">' + (d.active || 0) + '</div><div class="sc-label">Active</div></div>' +
            '<div class="summary-card"><div class="sc-value">' + (d.draft || 0) + '</div><div class="sc-label">Draft</div></div>' +
            '<div class="summary-card"><div class="sc-value">' + (d.total_hits || 0) + '</div><div class="sc-label">Total Hits</div></div>' +
            '<div class="summary-card"><div class="sc-value">' + (d.total_csv_uploads || 0) + '</div><div class="sc-label">CSV Uploads</div></div>' +
            '<div class="summary-card"><div class="sc-value">' + (d.total_conversions || 0) + '</div><div class="sc-label">Conversions</div></div>';
    }).catch(function() {});
}

function initSQLAssistantPanel() {
    var chatEl = document.getElementById('sqlAssistantChat');
    if (!chatEl) return;
    if (chatEl.dataset.initialized === '1') return;
    chatEl.dataset.initialized = '1';
    clearSQLAssistantPanel();
}

function appendSQLAssistantChat(role, text, meta) {
    var chatEl = document.getElementById('sqlAssistantChat');
    if (!chatEl) return;

    if (chatEl.querySelector('.empty-state')) {
        chatEl.innerHTML = '';
    }

    var roleBadgeClass = role === 'assistant' ? 'status-active' : 'status-draft';
    var roleTitle = role === 'assistant' ? 'Assistant' : 'You';
    var metaLine = meta ? '<div style="margin-top:4px;color:#64748b;font-size:12px;">' + escapeHtml(meta) + '</div>' : '';
    var block = '<div class="cp-card" style="margin:0 0 8px 0;padding:10px;">' +
        '<div><span class="status-badge ' + roleBadgeClass + '">' + roleTitle + '</span></div>' +
        '<div style="margin-top:6px;white-space:pre-wrap;">' + escapeHtml(text || '') + '</div>' +
        metaLine +
        '</div>';

    chatEl.insertAdjacentHTML('beforeend', block);
    chatEl.scrollTop = chatEl.scrollHeight;
}

function renderSQLAssistantSuggestions() {
    var el = document.getElementById('sqlAssistantSuggestions');
    if (!el) return;

    if (!Array.isArray(latestSQLAISuggestions) || latestSQLAISuggestions.length === 0) {
        el.innerHTML = '<div class="empty-state" style="margin:0; padding:20px;">No suggestions yet.</div>';
        return;
    }

    var html = '<table class="admin-table compact" style="margin:0; border:none;"><thead><tr style="background:rgba(0,0,0,0.2);"><th>Title</th><th>SQL</th><th>Notes</th><th>Actions</th></tr></thead><tbody>';
    latestSQLAISuggestions.forEach(function(item, idx) {
        html += '<tr>' +
            '<td style="font-weight:600; color:var(--primary-light);">' + escapeHtml(item.title || 'SQL suggestion') + '</td>' +
            '<td><pre style="margin:0; white-space:pre-wrap; background:rgba(0,0,0,0.1); padding:8px; border-radius:4px; border:1px solid rgba(255,255,255,0.05); font-family:var(--font-mono);">' + escapeHtml(item.sql || '') + '</pre></td>' +
            '<td><small style="color:var(--text-muted);">' + escapeHtml(item.notes || '-') + '</small></td>' +
            '<td style="white-space:nowrap;">' +
            '<div style="display:flex; gap:6px;">' +
            '<button class="btn-sm btn-secondary" onclick="copySQLAISuggestion(' + idx + ')" title="Copy SQL"><svg style="width:14px;height:14px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg></button> ' +
            '<button class="btn-sm btn-test" onclick="applySQLAISuggestion(' + idx + ')" title="Use SQL"><svg style="width:14px;height:14px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"></polyline></svg> Use</button>' +
            '</div>' +
            '</td>' +
            '</tr>';
    });
    html += '</tbody></table>';
    el.innerHTML = html;
}

function normalizeSQLAISuggestion(item) {
    if (!item || typeof item !== 'object') return null;
    var title = String(item.title || item.Title || 'SQL suggestion').trim();
    var sql = String(item.sql || item.SQL || '').trim();
    var notes = String(item.notes || item.Notes || '').trim();
    if (!sql) return null;
    return { title: title, sql: sql, notes: notes };
}

function askSQLAssistant() {
    var promptEl = document.getElementById('sqlAssistantPrompt');
    if (!promptEl) return;
    var message = String(promptEl.value || '').trim();
    if (!message) {
        alert('Please write what SQL you want to build.');
        return;
    }

    var askBtn = document.getElementById('sqlAssistantAskBtn');
    if (askBtn) askBtn.disabled = true;

    var payload = {
        message: message,
        dialect: (document.getElementById('sqlAssistantDialect') || {}).value || 'postgres',
        schema_hint: (document.getElementById('sqlAssistantSchemaHint') || {}).value || '',
        context: 'AxiomNizam API Builder SQL template generation'
    };

    appendSQLAssistantChat('user', message);

    fetch(BACKEND_URL + '/api/v1/builder/sql-assistant/chat', {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify(payload)
    }).then(function(response) {
        return response.json().then(function(data) {
            if (!response.ok) {
                var errMessage = (data && (data.error || data.message)) || ('SQL assistant failed with status ' + response.status);
                throw new Error(errMessage);
            }
            return data;
        });
    }).then(function(data) {
        var assistant = (data && data.assistant) || {};
        var provider = String(assistant.provider || 'fallback');
        var reply = String(assistant.reply || 'Generated SQL suggestions.');
        var warning = String(assistant.warning || '').trim();

        latestSQLAISuggestions = ((assistant.suggestions || []).map(normalizeSQLAISuggestion).filter(function(item) { return !!item; }));
        renderSQLAssistantSuggestions();

        var meta = 'provider: ' + provider + (warning ? ' | ' + warning : '');
        appendSQLAssistantChat('assistant', reply, meta);
        if (warning) addLog('SQL assistant warning: ' + warning, 'warn');
    }).catch(function(err) {
        appendSQLAssistantChat('assistant', 'Failed to generate SQL suggestions.', err.message || 'Unknown error');
        addLog('SQL assistant error: ' + (err.message || 'unknown'), 'error');
    }).finally(function() {
        if (askBtn) askBtn.disabled = false;
    });
}

function copySQLAISuggestion(index) {
    if (!Array.isArray(latestSQLAISuggestions) || !latestSQLAISuggestions[index]) return;
    var sql = String(latestSQLAISuggestions[index].sql || '');
    if (!sql) return;

    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(sql).then(function() {
            addLog('Copied SQL suggestion to clipboard', 'info');
        }).catch(function() {
            alert('Failed to copy SQL suggestion.');
        });
        return;
    }

    var input = document.createElement('textarea');
    input.value = sql;
    document.body.appendChild(input);
    input.select();
    try {
        document.execCommand('copy');
        addLog('Copied SQL suggestion to clipboard', 'info');
    } catch (e) {
        alert('Failed to copy SQL suggestion.');
    }
    document.body.removeChild(input);
}

function applySQLAISuggestion(index) {
    if (!Array.isArray(latestSQLAISuggestions) || !latestSQLAISuggestions[index]) return;
    var sql = String(latestSQLAISuggestions[index].sql || '').trim();
    if (!sql) return;

    var editModal = document.getElementById('editAPIModal');
    var createModal = document.getElementById('createAPIModal');
    var applied = false;

    if (editModal && editModal.style.display === 'flex') {
        var editInput = document.getElementById('editApiSQLTemplateInput');
        if (editInput) {
            editInput.value = sql;
            applied = true;
        }
    }

    if (!applied) {
        var createInput = document.getElementById('apiSQLTemplateInput');
        if (createInput) {
            createInput.value = sql;
            applied = true;
        }
        if (createModal && createModal.style.display !== 'flex' && canModify()) {
            openCreateAPIModal();
        }
    }

    if (applied) {
        addLog('Applied SQL suggestion to API form', 'info');
    } else {
        copySQLAISuggestion(index);
    }
}

function applyTopSQLAISuggestion() {
    if (!Array.isArray(latestSQLAISuggestions) || latestSQLAISuggestions.length === 0) {
        alert('No SQL suggestions available yet. Ask SQL AI first.');
        return;
    }
    applySQLAISuggestion(0);
}

function clearSQLAssistantPanel() {
    latestSQLAISuggestions = [];
    var promptEl = document.getElementById('sqlAssistantPrompt');
    var chatEl = document.getElementById('sqlAssistantChat');

    if (promptEl) promptEl.value = '';
    if (chatEl) chatEl.innerHTML = '<div class="empty-state" style="margin:0;">Ask a SQL question to start the conversation.</div>';
    renderSQLAssistantSuggestions();
}

function formatBuilderAPITimestamp(raw) {
    if (!raw) return '--';
    var date = new Date(raw);
    if (Number.isNaN(date.getTime())) return '--';
    return date.toLocaleString();
}

function formatBuilderAPIRateLimit(rateLimit) {
    var parsed = parseInt(rateLimit, 10);
    if (!parsed || parsed <= 0) return 'Unlimited';
    return parsed + ' / min';
}

function renderAPIHoverMetricItem(label, value) {
    return '<div class="api-hover-metric-item">' +
        '<div class="api-hover-metric-label">' + escapeHtml(label) + '</div>' +
        '<div class="api-hover-metric-value">' + escapeHtml(value) + '</div>' +
        '</div>';
}

function renderAPIHoverMetrics(api) {
    var queryParamCount = Array.isArray(api.query_params) ? api.query_params.length : 0;
    var cacheTTL = parseInt(api.cache_ttl, 10);
    if (!cacheTTL || cacheTTL <= 0) cacheTTL = 300;
    var cacheLabel = api.cache_enabled ? ('On (' + cacheTTL + 's)') : 'Off';
    var authLabel = api.auth_required ? 'Required' : 'Optional';
    var sqlMode = api.sql_template ? 'SQL Template' : (api.graphql_query ? 'GraphQL Query' : 'Mock/Static');

    return '<div class="api-hover-panel" role="tooltip">' +
        '<div class="api-hover-title">Short API Metrics</div>' +
        '<div class="api-hover-grid">' +
            renderAPIHoverMetricItem('Hits', String(api.hit_count || 0)) +
            renderAPIHoverMetricItem('Rate Limit', formatBuilderAPIRateLimit(api.rate_limit)) +
            renderAPIHoverMetricItem('Auth', authLabel) +
            renderAPIHoverMetricItem('Cache', cacheLabel) +
            renderAPIHoverMetricItem('Query Params', String(queryParamCount)) +
            renderAPIHoverMetricItem('Mode', sqlMode) +
        '</div>' +
        '<div class="api-hover-footnote">Updated: ' + escapeHtml(formatBuilderAPITimestamp(api.updated_at)) + '</div>' +
        '</div>';
}

function renderAPINameCell(api) {
    return '<div class="api-name-with-hover">' +
        '<span class="api-name-text">' + escapeHtml(api.name) + '</span>' +
        '<span class="api-hover-trigger" tabindex="0">Metrics</span>' +
        renderAPIHoverMetrics(api) +
        '</div>';
}

function normalizeRESTEndpointMethod(method) {
    return String(method || 'GET').trim().toUpperCase();
}

function normalizeRESTEndpointPath(path) {
    var raw = String(path || '').trim();
    if (!raw) return '/';

    var normalized = '/' + raw.replace(/^\/+/, '');
    normalized = normalized.replace(/\/{2,}/g, '/');
    if (normalized.length > 1 && normalized.endsWith('/')) {
        normalized = normalized.slice(0, -1);
    }

    if (normalized === '/api/custom') {
        return '/';
    }
    if (normalized.indexOf('/api/custom/') === 0) {
        var trimmed = normalized.substring('/api/custom'.length);
        if (!trimmed) return '/';
        return '/' + String(trimmed).replace(/^\/+/, '').replace(/\/{2,}/g, '/');
    }

    return normalized;
}

function toRESTEndpointCatalogEntry(api) {
    if (!api || typeof api !== 'object') return null;

    var apiType = String(api.api_type || 'rest').trim().toLowerCase();
    if (apiType && apiType !== 'rest') return null;

    var rawPath = String(api.path || '').trim();
    if (!rawPath) return null;

    return {
        id: String(api.id || '').trim(),
        name: String(api.name || '').trim(),
        method: normalizeRESTEndpointMethod(api.method),
        rawPath: rawPath,
        normalizedPath: normalizeRESTEndpointPath(rawPath)
    };
}

function setRESTEndpointCatalog(list) {
    var entries = Array.isArray(list) ? list.map(toRESTEndpointCatalogEntry).filter(function(item) { return !!item; }) : [];
    var seen = {};

    existingRESTEndpoints = entries.filter(function(item) {
        var key = item.id + '|' + item.method + '|' + item.normalizedPath;
        if (seen[key]) return false;
        seen[key] = true;
        return true;
    });

    renderRESTEndpointSuggestions();
}

function renderRESTEndpointSuggestions() {
    var datalist = document.getElementById('restEndpointSuggestions');
    if (!datalist) return;

    var rows = existingRESTEndpoints.slice().sort(function(a, b) {
        var pathCmp = a.normalizedPath.localeCompare(b.normalizedPath);
        if (pathCmp !== 0) return pathCmp;
        return a.method.localeCompare(b.method);
    });

    var optionSeen = {};
    var html = '';
    rows.forEach(function(entry) {
        var optionKey = entry.method + '|' + entry.rawPath;
        if (optionSeen[optionKey]) return;
        optionSeen[optionKey] = true;

        var label = entry.method + ' ' + entry.normalizedPath;
        if (entry.name) {
            label += ' - ' + entry.name;
        }

        html += '<option value="' + escapeHtml(entry.rawPath) + '" label="' + escapeHtml(label) + '"></option>';
    });

    datalist.innerHTML = html;
}

function loadRESTEndpointCatalog() {
    return fetchJSON('/api/v1/builder/apis?api_type=rest').then(function(d) {
        setRESTEndpointCatalog(d.apis || []);
        validateCreateRESTEndpoint(false);
        validateEditRESTEndpoint(false);
    }).catch(function() {
        var fallback = [];
        Object.keys(customAPIById || {}).forEach(function(id) {
            fallback.push(customAPIById[id]);
        });
        if (fallback.length > 0) {
            setRESTEndpointCatalog(fallback);
            validateCreateRESTEndpoint(false);
            validateEditRESTEndpoint(false);
        }
    });
}

function findDuplicateRESTEndpoint(method, path, excludeID) {
    var targetMethod = normalizeRESTEndpointMethod(method);
    var targetPath = normalizeRESTEndpointPath(path);
    var skipID = String(excludeID || '').trim();

    for (var i = 0; i < existingRESTEndpoints.length; i++) {
        var entry = existingRESTEndpoints[i];
        if (skipID && entry.id === skipID) continue;
        if (entry.method === targetMethod && entry.normalizedPath === targetPath) {
            return entry;
        }
    }

    return null;
}

function setRESTEndpointDuplicateWarning(elementID, message) {
    var warningEl = document.getElementById(elementID);
    if (!warningEl) return;
    if (message) {
        warningEl.textContent = message;
        warningEl.style.display = 'block';
        return;
    }

    warningEl.textContent = '';
    warningEl.style.display = 'none';
}

function validateCreateRESTEndpoint(showAlert) {
    var methodEl = document.getElementById('apiMethodInput');
    var pathEl = document.getElementById('apiPathInput');
    if (!methodEl || !pathEl) return true;

    var path = String(pathEl.value || '').trim();
    if (!path) {
        pathEl.setCustomValidity('');
        setRESTEndpointDuplicateWarning('apiPathDuplicateWarning', '');
        return true;
    }

    var duplicate = findDuplicateRESTEndpoint(methodEl.value, path, '');
    if (!duplicate) {
        pathEl.setCustomValidity('');
        setRESTEndpointDuplicateWarning('apiPathDuplicateWarning', '');
        return true;
    }

    var msg = 'Endpoint already exists: ' + duplicate.method + ' ' + duplicate.normalizedPath + (duplicate.name ? ' (' + duplicate.name + ')' : '');
    pathEl.setCustomValidity(msg);
    setRESTEndpointDuplicateWarning('apiPathDuplicateWarning', msg);
    if (showAlert) {
        alert(msg);
    }
    return false;
}

function validateEditRESTEndpoint(showAlert) {
    var methodEl = document.getElementById('editApiMethodInput');
    var pathEl = document.getElementById('editApiPathInput');
    var idEl = document.getElementById('editApiIdInput');
    if (!methodEl || !pathEl || !idEl) return true;

    var path = String(pathEl.value || '').trim();
    if (!path) {
        pathEl.setCustomValidity('');
        setRESTEndpointDuplicateWarning('editApiPathDuplicateWarning', '');
        return true;
    }

    var duplicate = findDuplicateRESTEndpoint(methodEl.value, path, idEl.value || '');
    if (!duplicate) {
        pathEl.setCustomValidity('');
        setRESTEndpointDuplicateWarning('editApiPathDuplicateWarning', '');
        return true;
    }

    var msg = 'Endpoint already exists: ' + duplicate.method + ' ' + duplicate.normalizedPath + (duplicate.name ? ' (' + duplicate.name + ')' : '');
    pathEl.setCustomValidity(msg);
    setRESTEndpointDuplicateWarning('editApiPathDuplicateWarning', msg);
    if (showAlert) {
        alert(msg);
    }
    return false;
}

function bindRESTEndpointValidation() {
    var createMethodEl = document.getElementById('apiMethodInput');
    var createPathEl = document.getElementById('apiPathInput');
    var editMethodEl = document.getElementById('editApiMethodInput');
    var editPathEl = document.getElementById('editApiPathInput');

    if (createMethodEl && createMethodEl.dataset.endpointValidationBound !== '1') {
        createMethodEl.addEventListener('change', function() { validateCreateRESTEndpoint(false); });
        createMethodEl.dataset.endpointValidationBound = '1';
    }
    if (createPathEl && createPathEl.dataset.endpointValidationBound !== '1') {
        createPathEl.addEventListener('input', function() { validateCreateRESTEndpoint(false); });
        createPathEl.dataset.endpointValidationBound = '1';
    }
    if (editMethodEl && editMethodEl.dataset.endpointValidationBound !== '1') {
        editMethodEl.addEventListener('change', function() { validateEditRESTEndpoint(false); });
        editMethodEl.dataset.endpointValidationBound = '1';
    }
    if (editPathEl && editPathEl.dataset.endpointValidationBound !== '1') {
        editPathEl.addEventListener('input', function() { validateEditRESTEndpoint(false); });
        editPathEl.dataset.endpointValidationBound = '1';
    }
}

function normalizeGraphQLEndpointMethod(method) {
    var normalized = String(method || '').trim().toUpperCase();
    return normalized || 'POST';
}

function normalizeGraphQLEndpointPath(path) {
    var raw = String(path || '').trim();
    if (!raw) {
        raw = '/api/graphql';
    }
    return normalizeRESTEndpointPath(raw);
}

function toGraphQLEndpointCatalogEntry(api) {
    if (!api || typeof api !== 'object') return null;

    var apiType = String(api.api_type || 'rest').trim().toLowerCase();
    if (apiType !== 'graphql') return null;

    return {
        id: String(api.id || '').trim(),
        name: String(api.name || '').trim(),
        operation: String(api.graphql_operation_name || '').trim(),
        method: normalizeGraphQLEndpointMethod(api.method),
        rawPath: String(api.path || '/api/graphql').trim() || '/api/graphql',
        normalizedPath: normalizeGraphQLEndpointPath(api.path)
    };
}

function setGraphQLEndpointCatalog(list) {
    var entries = Array.isArray(list) ? list.map(toGraphQLEndpointCatalogEntry).filter(function(item) { return !!item; }) : [];
    var seen = {};

    existingGraphQLEndpoints = entries.filter(function(item) {
        var key = item.id + '|' + item.method + '|' + item.normalizedPath;
        if (seen[key]) return false;
        seen[key] = true;
        return true;
    });

    renderGraphQLEndpointSuggestions();
}

function renderGraphQLEndpointSuggestions() {
    var datalist = document.getElementById('graphqlEndpointSuggestions');
    if (!datalist) return;

    var rows = existingGraphQLEndpoints.slice().sort(function(a, b) {
        var pathCmp = a.normalizedPath.localeCompare(b.normalizedPath);
        if (pathCmp !== 0) return pathCmp;
        return a.method.localeCompare(b.method);
    });

    var optionSeen = {};
    var html = '';
    rows.forEach(function(entry) {
        var optionKey = entry.method + '|' + entry.rawPath;
        if (optionSeen[optionKey]) return;
        optionSeen[optionKey] = true;

        var label = entry.method + ' ' + entry.normalizedPath;
        if (entry.operation) {
            label += ' - ' + entry.operation;
        } else if (entry.name) {
            label += ' - ' + entry.name;
        }

        html += '<option value="' + escapeHtml(entry.rawPath) + '" label="' + escapeHtml(label) + '"></option>';
    });

    datalist.innerHTML = html;
}

function loadGraphQLEndpointCatalog() {
    return fetchJSON('/api/v1/builder/apis?api_type=graphql').then(function(d) {
        setGraphQLEndpointCatalog(d.apis || []);
        validateGraphQLEndpoint(false);
    }).catch(function() {
        var fallback = [];
        Object.keys(graphQLAPIById || {}).forEach(function(id) {
            fallback.push(graphQLAPIById[id]);
        });
        if (fallback.length > 0) {
            setGraphQLEndpointCatalog(fallback);
            validateGraphQLEndpoint(false);
        }
    });
}

function findDuplicateGraphQLEndpoint(method, path, excludeID) {
    var targetMethod = normalizeGraphQLEndpointMethod(method);
    var targetPath = normalizeGraphQLEndpointPath(path);
    var skipID = String(excludeID || '').trim();

    for (var i = 0; i < existingGraphQLEndpoints.length; i++) {
        var entry = existingGraphQLEndpoints[i];
        if (skipID && entry.id === skipID) continue;
        if (entry.method === targetMethod && entry.normalizedPath === targetPath) {
            return entry;
        }
    }

    return null;
}

function setGraphQLEndpointDuplicateWarning(message) {
    var warningEl = document.getElementById('gqlPathDuplicateWarning');
    if (!warningEl) return;
    if (message) {
        warningEl.textContent = message;
        warningEl.style.display = 'block';
        return;
    }

    warningEl.textContent = '';
    warningEl.style.display = 'none';
}

function validateGraphQLEndpoint(showAlert) {
    var pathEl = document.getElementById('gqlPathInput');
    if (!pathEl) return true;

    var effectivePath = String(pathEl.value || '').trim() || '/api/graphql';
    var excludeID = (graphQLFormMode === 'edit' && editingGraphQLApiId) ? editingGraphQLApiId : '';
    var duplicate = findDuplicateGraphQLEndpoint('POST', effectivePath, excludeID);
    if (!duplicate) {
        pathEl.setCustomValidity('');
        setGraphQLEndpointDuplicateWarning('');
        return true;
    }

    var msg = 'GraphQL endpoint already exists: ' + duplicate.method + ' ' + duplicate.normalizedPath + (duplicate.operation ? ' (' + duplicate.operation + ')' : (duplicate.name ? ' (' + duplicate.name + ')' : ''));
    pathEl.setCustomValidity(msg);
    setGraphQLEndpointDuplicateWarning(msg);
    if (showAlert) {
        alert(msg);
    }
    return false;
}

function bindGraphQLEndpointValidation() {
    var pathEl = document.getElementById('gqlPathInput');
    if (pathEl && pathEl.dataset.graphqlEndpointValidationBound !== '1') {
        pathEl.addEventListener('input', function() { validateGraphQLEndpoint(false); });
        pathEl.dataset.graphqlEndpointValidationBound = '1';
    }
}

function loadCustomAPIs() {
    var catEl = document.getElementById('apiCategoryFilter');
    var statEl = document.getElementById('apiStatusFilter');
    var cat = catEl ? catEl.value : '';
    var status = statEl ? statEl.value : '';
    var q = '/api/v1/builder/apis?api_type=rest&';
    if (cat) q += 'category=' + encodeURIComponent(cat) + '&';
    if (status) q += 'status=' + encodeURIComponent(status);

    fetchJSON(q).then(function(d) {
        var list = d.apis || [];
        customAPIById = {};
        if (!cat && !status) {
            setRESTEndpointCatalog(list);
        }
        var el = document.getElementById('apiBuilderList');
        if (!el) return;
        if (list.length === 0) {
            el.innerHTML = '<div class="empty-state">No custom APIs found. Click <strong>+ Create API</strong> to get started.</div>';
            return;
        }
        var html = '<table class="admin-table"><thead><tr>' +
            '<th>Method</th><th>Name</th><th>Path</th><th>Category</th><th>Source DB</th><th>Source Server</th><th>Status</th><th>Hits</th><th>Actions</th>' +
            '</tr></thead><tbody>';
        list.forEach(function(api) {
            customAPIById[api.id] = api;
            var safeId = api.id.replace(/'/g, "\\'");
            var actionsHtml = '<button class="btn-sm btn-test" onclick="testCustomAPI(\'' + safeId + '\')">Test</button> ';
            if (canModify()) {
                var isActive = (api.status || '').toLowerCase() === 'active';
                actionsHtml += '<button class="btn-sm btn-edit" onclick="openEditCustomAPI(\'' + safeId + '\')">Edit</button> ';
                actionsHtml += '<button class="btn-sm btn-toggle" onclick="toggleCustomAPIStatus(\'' + safeId + '\')">' + (isActive ? 'Deactivate' : 'Activate') + '</button> ';
                actionsHtml += '<button class="btn-sm btn-del" onclick="deleteCustomAPI(\'' + safeId + '\')">Del</button>';
            }
            html += '<tr>' +
                '<td><span class="method-badge method-' + api.method.toLowerCase() + '">' + api.method + '</span></td>' +
                '<td>' + renderAPINameCell(api) + '</td>' +
                '<td><code>' + escapeHtml(api.path) + '</code></td>' +
                '<td>' + escapeHtml(api.category || '-') + '</td>' +
                '<td>' + escapeHtml(api.source_database || '-') + '</td>' +
                '<td>' + escapeHtml(api.source_server || 'default') + '</td>' +
                '<td><span class="status-badge status-' + api.status + '">' + api.status + '</span></td>' +
                '<td>' + (api.hit_count || 0) + '</td>' +
                '<td>' + actionsHtml + '</td></tr>';
        });
        html += '</tbody></table>';
        el.innerHTML = html;
    }).catch(function() {
        var el = document.getElementById('apiBuilderList');
        if (el) el.innerHTML = '<div class="error-state">Failed to load APIs</div>';
    });
}

function escapeHtml(str) {
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function closeCreateAPIModal() {
    document.getElementById('createAPIModal').style.display = 'none';
    document.getElementById('createAPIForm').reset();
    var pathInput = document.getElementById('apiPathInput');
    if (pathInput) pathInput.setCustomValidity('');
    setRESTEndpointDuplicateWarning('apiPathDuplicateWarning', '');
    var ttlGroup = document.getElementById('cacheTTLGroup');
    if (ttlGroup) ttlGroup.style.display = 'none';
    updateBuilderSourceServers();
}

function toggleCacheTTL() {
    var checked = document.getElementById('apiCacheInput').checked;
    var group = document.getElementById('cacheTTLGroup');
    if (group) group.style.display = checked ? 'block' : 'none';
}

function countSQLPlaceholders(query) {
    var m = String(query || '').match(/\?/g);
    return m ? m.length : 0;
}

function getSQLTemplateFromAPI(api) {
    if (api && api.sql_template) return String(api.sql_template).trim();
    if (api && api.mock_response && typeof api.mock_response === 'object' && api.mock_response.query) {
        return String(api.mock_response.query).trim();
    }
    return '';
}

function submitCreateAPI(e) {
    e.preventDefault();
    if (!validateCreateRESTEndpoint(true)) {
        return;
    }

    var mockRaw = document.getElementById('apiMockResponseInput').value.trim();
    var mockResp = null;
    if (mockRaw) {
        try { mockResp = JSON.parse(mockRaw); } catch(err) { alert('Invalid JSON in mock response'); return; }
    }
    var qpRaw = document.getElementById('apiQueryParamsInput').value.trim();
    var queryParams = [];
    if (qpRaw) {
        qpRaw.split('\n').forEach(function(line) {
            var parts = line.trim().split(':');
            if (parts.length >= 2) {
                queryParams.push({ name: parts[0].trim(), type: parts[1].trim(), required: parts.length > 2 && parts[2].trim() === 'true' });
            }
        });
    }

    var sqlTemplate = (document.getElementById('apiSQLTemplateInput').value || '').trim();
    var sqlPlaceholderCount = countSQLPlaceholders(sqlTemplate);

    var body = {
        api_type: 'rest',
        name: document.getElementById('apiNameInput').value,
        method: document.getElementById('apiMethodInput').value,
        path: document.getElementById('apiPathInput').value,
        description: document.getElementById('apiDescInput').value,
        category: document.getElementById('apiCategoryInput').value,
        source_database: (document.getElementById('apiSourceDatabaseInput').value || '').trim(),
        source_server: (document.getElementById('apiSourceServerInput').value || '').trim(),
        auth_required: document.getElementById('apiAuthInput').checked,
        rate_limit: parseInt(document.getElementById('apiRateLimitInput').value) || 0,
        cache_enabled: document.getElementById('apiCacheInput').checked,
        cache_ttl: parseInt(document.getElementById('apiCacheTTLInput').value) || 300,
        sql_template: sqlTemplate,
        mock_response: mockResp,
        query_params: queryParams
    };

    if (body.source_database && !body.sql_template) {
        alert('SQL Template is required when Source Database is selected.');
        return;
    }
    if (sqlPlaceholderCount > 0 && queryParams.length === 0) {
        alert('Define Query Parameters for each SQL placeholder (?); one parameter per placeholder in order.');
        return;
    }

    postJSON('/api/v1/builder/apis', body).then(function(d) {
        if (d.status === 'success') {
            closeCreateAPIModal();
            loadBuilderSummary();
            loadCustomAPIs();
            loadRESTEndpointCatalog();
            addLog('Created API: ' + body.name, 'info');
        } else {
            alert(d.error || 'Failed to create API');
        }
    });
}

function loadBuilderDataSources(onDone) {
    fetchJSON('/api/admin/database/servers').then(function(d) {
        builderDataServers = d.servers || [];

        var dbSelect = document.getElementById('apiSourceDatabaseInput');
        var gqlDbSelect = document.getElementById('gqlSourceDatabaseInput');
        var editDbSelect = document.getElementById('editApiSourceDatabaseInput');
        if (!dbSelect && !gqlDbSelect && !editDbSelect) {
            if (typeof onDone === 'function') onDone();
            return;
        }

        var existing = {};
        DEFAULT_BUILDER_DATABASES.forEach(function(db) { existing[db] = true; });
        (builderDataServers || []).forEach(function(s) {
            if (s && s.db_type) existing[s.db_type] = true;
        });

        function populateDatabaseSelect(selectEl) {
            if (!selectEl) return;
            var selectedDB = selectEl.value;
            selectEl.innerHTML = '<option value="">Select database...</option>';
            Object.keys(existing).sort().forEach(function(dbType) {
                selectEl.innerHTML += '<option value="' + dbType + '">' + dbType.toUpperCase() + '</option>';
            });
            if (selectedDB && existing[selectedDB]) {
                selectEl.value = selectedDB;
            }
        }

        populateDatabaseSelect(dbSelect);
        populateDatabaseSelect(gqlDbSelect);
        populateDatabaseSelect(editDbSelect);

        updateBuilderSourceServers();
        updateGraphQLBuilderSourceServers();
        updateEditBuilderSourceServers();
        if (typeof onDone === 'function') onDone();
    }).catch(function() {
        var dbSelect = document.getElementById('apiSourceDatabaseInput');
        var gqlDbSelect = document.getElementById('gqlSourceDatabaseInput');
        var editDbSelect = document.getElementById('editApiSourceDatabaseInput');
        if (!dbSelect && !gqlDbSelect && !editDbSelect) {
            if (typeof onDone === 'function') onDone();
            return;
        }

        function populateDefault(selectEl) {
            if (!selectEl) return;
            var selectedDB = selectEl.value;
            selectEl.innerHTML = '<option value="">Select database...</option>';
            DEFAULT_BUILDER_DATABASES.forEach(function(dbType) {
                selectEl.innerHTML += '<option value="' + dbType + '">' + dbType.toUpperCase() + '</option>';
            });
            if (selectedDB) {
                selectEl.value = selectedDB;
            }
        }

        populateDefault(dbSelect);
        populateDefault(gqlDbSelect);
        populateDefault(editDbSelect);

        updateBuilderSourceServers();
        updateGraphQLBuilderSourceServers();
        updateEditBuilderSourceServers();
        if (typeof onDone === 'function') onDone();
    });
}

function updateBuilderSourceServers() {
    var dbTypeEl = document.getElementById('apiSourceDatabaseInput');
    var serverEl = document.getElementById('apiSourceServerInput');
    if (!dbTypeEl || !serverEl) return;

    var selectedDB = (dbTypeEl.value || '').trim();
    var previouslySelected = serverEl.value;
    var filtered = (builderDataServers || []).filter(function(s) {
        return !selectedDB || s.db_type === selectedDB;
    });

    serverEl.innerHTML = '<option value="">Default server for selected database</option>';
    filtered.forEach(function(s) {
        var name = (s.name || s.key || 'server') + (s.connected ? '' : ' (disconnected)');
        serverEl.innerHTML += '<option value="' + escapeHtml(s.key || '') + '">' + escapeHtml(name) + '</option>';
    });

    if (previouslySelected && filtered.some(function(s) { return s.key === previouslySelected; })) {
        serverEl.value = previouslySelected;
    }
}

function updateEditBuilderSourceServers() {
    var dbTypeEl = document.getElementById('editApiSourceDatabaseInput');
    var serverEl = document.getElementById('editApiSourceServerInput');
    if (!dbTypeEl || !serverEl) return;

    var selectedDB = (dbTypeEl.value || '').trim();
    var previouslySelected = serverEl.value;
    var filtered = (builderDataServers || []).filter(function(s) {
        return !selectedDB || s.db_type === selectedDB;
    });

    serverEl.innerHTML = '<option value="">Default server for selected database</option>';
    filtered.forEach(function(s) {
        var name = (s.name || s.key || 'server') + (s.connected ? '' : ' (disconnected)');
        serverEl.innerHTML += '<option value="' + escapeHtml(s.key || '') + '">' + escapeHtml(name) + '</option>';
    });

    if (previouslySelected && filtered.some(function(s) { return s.key === previouslySelected; })) {
        serverEl.value = previouslySelected;
    }
}

function loadGraphQLBuilderSummary() {
    fetchJSON('/api/v1/builder/summary?api_type=graphql').then(function(d) {
        var el = document.getElementById('graphqlBuilderSummary');
        if (!el) return;
        el.innerHTML =
            '<div class="summary-card"><div class="sc-value">' + (d.total_apis || 0) + '</div><div class="sc-label">GraphQL APIs</div></div>' +
            '<div class="summary-card"><div class="sc-value">' + (d.active || 0) + '</div><div class="sc-label">Active</div></div>' +
            '<div class="summary-card"><div class="sc-value">' + (d.draft || 0) + '</div><div class="sc-label">Draft</div></div>' +
            '<div class="summary-card"><div class="sc-value">' + (d.total_hits || 0) + '</div><div class="sc-label">Total Hits</div></div>';
    }).catch(function() {});
}

function loadGraphQLCustomAPIs() {
    var catEl = document.getElementById('graphqlApiCategoryFilter');
    var statEl = document.getElementById('graphqlApiStatusFilter');
    var cat = catEl ? catEl.value : '';
    var status = statEl ? statEl.value : '';
    var q = '/api/v1/builder/apis?api_type=graphql&';
    if (cat) q += 'category=' + encodeURIComponent(cat) + '&';
    if (status) q += 'status=' + encodeURIComponent(status);

    fetchJSON(q).then(function(d) {
        var list = d.apis || [];
        graphQLAPIById = {};
        if (!cat && !status) {
            setGraphQLEndpointCatalog(list);
        }
        var el = document.getElementById('graphqlApiBuilderList');
        if (!el) return;
        if (list.length === 0) {
            el.innerHTML = '<div class="empty-state">No GraphQL APIs found. Click <strong>+ Create GraphQL API</strong> to get started.</div>';
            return;
        }

        var html = '<table class="admin-table"><thead><tr>' +
            '<th>Name</th><th>Operation</th><th>Endpoint</th><th>Category</th><th>Source DB</th><th>Source Server</th><th>Status</th><th>Hits</th><th>Actions</th>' +
            '</tr></thead><tbody>';

        list.forEach(function(api) {
            graphQLAPIById[api.id] = api;
            var safeId = api.id.replace(/'/g, "\\'");
            var actionsHtml = '<button class="btn-sm btn-test" onclick="testGraphQLCustomAPI(\'' + safeId + '\')">Test</button> ';
            if (canModify()) {
                var isActive = (api.status || '').toLowerCase() === 'active';
                actionsHtml += '<button class="btn-sm btn-edit" onclick="openEditGraphQLCustomAPI(\'' + safeId + '\')">Edit</button> ';
                actionsHtml += '<button class="btn-sm btn-toggle" onclick="toggleGraphQLCustomAPIStatus(\'' + safeId + '\')">' + (isActive ? 'Deactivate' : 'Activate') + '</button> ';
                actionsHtml += '<button class="btn-sm btn-del" onclick="deleteGraphQLCustomAPI(\'' + safeId + '\')">Del</button>';
            }
            html += '<tr>' +
                '<td>' + escapeHtml(api.name) + '</td>' +
                '<td>' + escapeHtml(api.graphql_operation_name || '-') + '</td>' +
                '<td><code>' + escapeHtml(api.path || '/api/graphql') + '</code></td>' +
                '<td>' + escapeHtml(api.category || '-') + '</td>' +
                '<td>' + escapeHtml(api.source_database || '-') + '</td>' +
                '<td>' + escapeHtml(api.source_server || 'default') + '</td>' +
                '<td><span class="status-badge status-' + api.status + '">' + api.status + '</span></td>' +
                '<td>' + (api.hit_count || 0) + '</td>' +
                '<td>' + actionsHtml + '</td>' +
                '</tr>';
        });

        html += '</tbody></table>';
        el.innerHTML = html;
    }).catch(function() {
        var el = document.getElementById('graphqlApiBuilderList');
        if (el) el.innerHTML = '<div class="error-state">Failed to load GraphQL APIs</div>';
    });
}

function resetGraphQLFormMode() {
    graphQLFormMode = 'create';
    editingGraphQLApiId = '';

    var titleEl = document.querySelector('#createGraphQLAPIModal .modal-header h2');
    if (titleEl) titleEl.textContent = '🧩 Create GraphQL API';

    var submitBtn = document.querySelector('#createGraphQLAPIForm button[type="submit"]');
    if (submitBtn) submitBtn.textContent = 'Create GraphQL API';
}

function updateGraphQLBuilderSourceServers() {
    var dbTypeEl = document.getElementById('gqlSourceDatabaseInput');
    var serverEl = document.getElementById('gqlSourceServerInput');
    if (!dbTypeEl || !serverEl) return;

    var selectedDB = (dbTypeEl.value || '').trim();
    var previouslySelected = serverEl.value;
    var filtered = (builderDataServers || []).filter(function(s) {
        return !selectedDB || s.db_type === selectedDB;
    });

    serverEl.innerHTML = '<option value="">Default server for selected database</option>';
    filtered.forEach(function(s) {
        var name = (s.name || s.key || 'server') + (s.connected ? '' : ' (disconnected)');
        serverEl.innerHTML += '<option value="' + escapeHtml(s.key || '') + '">' + escapeHtml(name) + '</option>';
    });

    if (previouslySelected && filtered.some(function(s) { return s.key === previouslySelected; })) {
        serverEl.value = previouslySelected;
    }
}

function openCreateGraphQLAPIModal() {
    if (!canModify()) { alert('You do not have permission to create APIs. Contact an admin, manager, or system-manager.'); return; }
    resetGraphQLFormMode();
    bindGraphQLEndpointValidation();
    loadGraphQLEndpointCatalog();
    setGraphQLEndpointDuplicateWarning('');
    var pathEl = document.getElementById('gqlPathInput');
    if (pathEl) {
        pathEl.setCustomValidity('');
        if (!pathEl.value) {
            pathEl.value = '/api/graphql';
        }
    }
    loadBuilderDataSources();
    document.getElementById('createGraphQLAPIModal').style.display = 'flex';
}

function closeCreateGraphQLAPIModal() {
    resetGraphQLFormMode();
    document.getElementById('createGraphQLAPIModal').style.display = 'none';
    document.getElementById('createGraphQLAPIForm').reset();
    var pathEl = document.getElementById('gqlPathInput');
    if (pathEl) pathEl.setCustomValidity('');
    setGraphQLEndpointDuplicateWarning('');
    var ttlGroup = document.getElementById('gqlCacheTTLGroup');
    if (ttlGroup) ttlGroup.style.display = 'none';
    if (pathEl && !pathEl.value) {
        pathEl.value = '/api/graphql';
    }
    updateGraphQLBuilderSourceServers();
}

function toggleGraphQLCacheTTL() {
    var checked = document.getElementById('gqlCacheInput').checked;
    var group = document.getElementById('gqlCacheTTLGroup');
    if (group) group.style.display = checked ? 'block' : 'none';
}

function parseBuilderParams(raw) {
    var params = [];
    if (!raw) return params;
    raw.split('\n').forEach(function(line) {
        var parts = line.trim().split(':');
        if (parts.length >= 2) {
            params.push({ name: parts[0].trim(), type: parts[1].trim(), required: parts.length > 2 && parts[2].trim() === 'true' });
        }
    });
    return params;
}

function submitCreateGraphQLAPI(e) {
    e.preventDefault();
    if (!validateGraphQLEndpoint(true)) {
        return;
    }

    var mockRaw = document.getElementById('gqlMockResponseInput').value.trim();
    var mockResp = null;
    if (mockRaw) {
        try { mockResp = JSON.parse(mockRaw); } catch (err) { alert('Invalid JSON in mock response'); return; }
    }

    var body = {
        name: document.getElementById('gqlApiNameInput').value,
        method: 'POST',
        path: (document.getElementById('gqlPathInput').value || '/api/graphql').trim(),
        graphql_query: document.getElementById('gqlQueryInput').value,
        graphql_operation_name: (document.getElementById('gqlOperationNameInput').value || '').trim(),
        description: document.getElementById('gqlDescInput').value,
        category: document.getElementById('gqlApiCategoryInput').value,
        source_database: (document.getElementById('gqlSourceDatabaseInput').value || '').trim(),
        source_server: (document.getElementById('gqlSourceServerInput').value || '').trim(),
        auth_required: document.getElementById('gqlAuthInput').checked,
        rate_limit: parseInt(document.getElementById('gqlRateLimitInput').value, 10) || 0,
        cache_enabled: document.getElementById('gqlCacheInput').checked,
        cache_ttl: parseInt(document.getElementById('gqlCacheTTLInput').value, 10) || 300,
        mock_response: mockResp,
        query_params: parseBuilderParams(document.getElementById('gqlQueryParamsInput').value.trim())
    };

    var isEditMode = graphQLFormMode === 'edit' && !!editingGraphQLApiId;
    var req;
    if (isEditMode) {
        body.api_type = 'graphql';
        req = putJSON('/api/v1/builder/apis/' + encodeURIComponent(editingGraphQLApiId), body);
    } else {
        body.api_type = 'graphql';
        req = postJSON('/api/v1/builder/apis', body);
    }

    req.then(function(d) {
        if (d.status === 'success') {
            closeCreateGraphQLAPIModal();
            loadGraphQLBuilderSummary();
            loadGraphQLCustomAPIs();
            loadGraphQLEndpointCatalog();
            if (isEditMode) {
                addLog('Updated GraphQL API: ' + body.name, 'info');
            } else {
                addLog('Created GraphQL API: ' + body.name, 'info');
            }
        } else {
            alert(d.error || 'Failed to save GraphQL API');
        }
    });
}

function testGraphQLCustomAPI(id) {
    postJSON('/api/v1/builder/apis/' + id + '/test', {}).then(function(d) {
        showResponse('GraphQL API Test Result', 200, d, 'POST ' + (d.path || '/api/graphql'));
        addLog('Tested GraphQL API: ' + id, 'info');
    });
}

function deleteGraphQLCustomAPI(id) {
    if (!canModify()) { alert('You do not have permission to delete APIs. Contact an admin, manager, or system-manager.'); return; }
    var api = graphQLAPIById[id] || {};
    var apiName = api.name || id;
    if (!confirmDeleteAPI('GraphQL API', apiName)) return;
    deleteJSON('/api/v1/builder/apis/' + id).then(function(d) {
        if (d.status === 'success' || d.status === 'ok') {
            addLog('Deleted GraphQL API: ' + id, 'warn');
            loadGraphQLBuilderSummary();
            loadGraphQLCustomAPIs();
            loadGraphQLEndpointCatalog();
        } else {
            alert(d.error || 'Failed to delete GraphQL API');
        }
    });
}

function openEditGraphQLCustomAPI(id) {
    if (!canModify()) { alert('You do not have permission to edit APIs. Contact an admin, manager, or system-manager.'); return; }
    var api = graphQLAPIById[id];
    if (!api) { alert('GraphQL API details not found. Please refresh and try again.'); return; }

    graphQLFormMode = 'edit';
    editingGraphQLApiId = id;

    var titleEl = document.querySelector('#createGraphQLAPIModal .modal-header h2');
    if (titleEl) titleEl.textContent = '✏️ Edit GraphQL API';
    var submitBtn = document.querySelector('#createGraphQLAPIForm button[type="submit"]');
    if (submitBtn) submitBtn.textContent = 'Save Changes';

    loadBuilderDataSources(function() {
        loadGraphQLEndpointCatalog();
        document.getElementById('gqlApiNameInput').value = api.name || '';
        document.getElementById('gqlApiCategoryInput').value = api.category || 'custom';
        document.getElementById('gqlOperationNameInput').value = api.graphql_operation_name || '';
        document.getElementById('gqlPathInput').value = (api.path || '/api/graphql').trim();
        document.getElementById('gqlSourceDatabaseInput').value = api.source_database || '';
        updateGraphQLBuilderSourceServers();
        document.getElementById('gqlSourceServerInput').value = api.source_server || '';
        document.getElementById('gqlDescInput').value = api.description || '';
        document.getElementById('gqlQueryInput').value = api.graphql_query || '';
        document.getElementById('gqlAuthInput').checked = !!api.auth_required;
        document.getElementById('gqlRateLimitInput').value = api.rate_limit || 0;
        document.getElementById('gqlCacheInput').checked = !!api.cache_enabled;
        document.getElementById('gqlCacheTTLInput').value = api.cache_ttl || 300;
        toggleGraphQLCacheTTL();
        document.getElementById('gqlMockResponseInput').value = api.mock_response ? JSON.stringify(api.mock_response, null, 2) : '';

        var qp = Array.isArray(api.query_params) ? api.query_params : [];
        var qpLines = qp.map(function(p) {
            var pName = (p && p.name) ? p.name : '';
            var pType = (p && p.type) ? p.type : 'string';
            var pReq = (p && p.required) ? 'true' : 'false';
            return pName + ':' + pType + ':' + pReq;
        });
        document.getElementById('gqlQueryParamsInput').value = qpLines.join('\n');

        document.getElementById('createGraphQLAPIModal').style.display = 'flex';
        validateGraphQLEndpoint(false);
    });
}

function toggleGraphQLCustomAPIStatus(id) {
    if (!canModify()) { alert('You do not have permission to update API status. Contact an admin, manager, or system-manager.'); return; }

    var api = graphQLAPIById[id];
    if (!api) { alert('GraphQL API details not found. Please refresh and try again.'); return; }

    var currentStatus = String(api.status || '').toLowerCase();
    var nextStatus = currentStatus === 'active' ? 'inactive' : 'active';

    putJSON('/api/v1/builder/apis/' + encodeURIComponent(id), { status: nextStatus }).then(function(d) {
        if (d.status === 'success' || d.status === 'ok') {
            addLog('Updated GraphQL API status: ' + id + ' -> ' + nextStatus, 'info');
            loadGraphQLBuilderSummary();
            loadGraphQLCustomAPIs();
        } else {
            alert(d.error || 'Failed to update GraphQL API status');
        }
    }).catch(function() {
        alert('Failed to update GraphQL API status');
    });
}

function testCustomAPI(id) {
    postJSON('/api/v1/builder/apis/' + id + '/test', {}).then(function(d) {
        showResponse('API Test Result', 200, d, (d.method || 'GET') + ' ' + (d.path || ''));
        addLog('Tested API: ' + id, 'info');
    });
}

function openEditCustomAPI(id) {
    if (!canModify()) { alert('You do not have permission to edit APIs. Contact an admin, manager, or system-manager.'); return; }
    var api = customAPIById[id];
    if (!api) { alert('API details not found. Please refresh and try again.'); return; }

    loadBuilderDataSources(function() {
        loadRESTEndpointCatalog();
        document.getElementById('editApiIdInput').value = api.id || '';
        document.getElementById('editApiNameInput').value = api.name || '';
        document.getElementById('editApiMethodInput').value = api.method || 'GET';
        document.getElementById('editApiPathInput').value = api.path || '';
        document.getElementById('editApiCategoryInput').value = api.category || 'custom';
        document.getElementById('editApiSourceDatabaseInput').value = api.source_database || '';
        updateEditBuilderSourceServers();
        document.getElementById('editApiSourceServerInput').value = api.source_server || '';
        document.getElementById('editApiDescInput').value = api.description || '';
        document.getElementById('editApiSQLTemplateInput').value = getSQLTemplateFromAPI(api);
        document.getElementById('editApiAuthInput').checked = !!api.auth_required;
        document.getElementById('editApiCacheInput').checked = !!api.cache_enabled;
        document.getElementById('editApiCacheTTLInput').value = api.cache_ttl || 300;
        toggleEditCacheTTL();
        document.getElementById('editApiRateLimitInput').value = api.rate_limit || 0;
        document.getElementById('editApiStatusInput').value = api.status || 'active';
        document.getElementById('editAPIModal').style.display = 'flex';
        validateEditRESTEndpoint(false);
    });
}

function closeEditAPIModal() {
    document.getElementById('editAPIModal').style.display = 'none';
    document.getElementById('editAPIForm').reset();
    var pathInput = document.getElementById('editApiPathInput');
    if (pathInput) pathInput.setCustomValidity('');
    setRESTEndpointDuplicateWarning('editApiPathDuplicateWarning', '');
    var ttlGroup = document.getElementById('editCacheTTLGroup');
    if (ttlGroup) ttlGroup.style.display = 'none';
}

function toggleEditCacheTTL() {
    var checked = document.getElementById('editApiCacheInput').checked;
    var group = document.getElementById('editCacheTTLGroup');
    if (group) group.style.display = checked ? 'block' : 'none';
}

function submitEditAPI(e) {
    e.preventDefault();
    if (!canModify()) { alert('You do not have permission to edit APIs. Contact an admin, manager, or system-manager.'); return; }
    if (!validateEditRESTEndpoint(true)) {
        return;
    }

    var id = document.getElementById('editApiIdInput').value;
    if (!id) {
        alert('Invalid API ID');
        return;
    }

    var body = {
        name: document.getElementById('editApiNameInput').value,
        method: document.getElementById('editApiMethodInput').value,
        path: document.getElementById('editApiPathInput').value,
        description: document.getElementById('editApiDescInput').value,
        sql_template: (document.getElementById('editApiSQLTemplateInput').value || '').trim(),
        category: document.getElementById('editApiCategoryInput').value,
        source_database: (document.getElementById('editApiSourceDatabaseInput').value || '').trim(),
        source_server: (document.getElementById('editApiSourceServerInput').value || '').trim(),
        auth_required: document.getElementById('editApiAuthInput').checked,
        cache_enabled: document.getElementById('editApiCacheInput').checked,
        cache_ttl: parseInt(document.getElementById('editApiCacheTTLInput').value, 10) || 300,
        rate_limit: parseInt(document.getElementById('editApiRateLimitInput').value, 10) || 0,
        status: document.getElementById('editApiStatusInput').value
    };

    if (body.source_database && !body.sql_template) {
        alert('SQL Template is required when Source Database is selected.');
        return;
    }

    putJSON('/api/v1/builder/apis/' + encodeURIComponent(id), body).then(function(d) {
        if (d.status === 'success' || d.status === 'ok') {
            closeEditAPIModal();
            loadBuilderSummary();
            loadCustomAPIs();
            loadRESTEndpointCatalog();
            addLog('Updated API: ' + id, 'info');
        } else {
            alert(d.error || 'Failed to update API');
        }
    }).catch(function() {
        alert('Failed to update API');
    });
}

function toggleCustomAPIStatus(id) {
    if (!canModify()) { alert('You do not have permission to update API status. Contact an admin, manager, or system-manager.'); return; }

    var api = customAPIById[id];
    if (!api) { alert('API details not found. Please refresh and try again.'); return; }

    var currentStatus = String(api.status || '').toLowerCase();
    var nextStatus = currentStatus === 'active' ? 'inactive' : 'active';

    putJSON('/api/v1/builder/apis/' + encodeURIComponent(id), { status: nextStatus }).then(function(d) {
        if (d.status === 'success' || d.status === 'ok') {
            addLog('Updated API status: ' + id + ' -> ' + nextStatus, 'info');
            loadBuilderSummary();
            loadCustomAPIs();
        } else {
            alert(d.error || 'Failed to update API status');
        }
    }).catch(function() {
        alert('Failed to update API status');
    });
}

// ===================================================================
// CSV Upload & Dashboard Generation
// ===================================================================
function setupCSVDropZone() {
    var dz = document.getElementById('csvDropZone');
    if (!dz) return;
    dz.addEventListener('dragover', function(e) { e.preventDefault(); dz.classList.add('drag-over'); });
    dz.addEventListener('dragleave', function() { dz.classList.remove('drag-over'); });
    dz.addEventListener('drop', function(e) {
        e.preventDefault();
        dz.classList.remove('drag-over');
        if (e.dataTransfer.files.length > 0) {
            var fi = document.getElementById('csvFileInput');
            fi.files = e.dataTransfer.files;
            handleFileUpload();
        }
    });
}

function handleFileUpload() {
    var fileInput = document.getElementById('csvFileInput');
    if (!fileInput.files || !fileInput.files[0]) return;
    var file = fileInput.files[0];

    var formData = new FormData();
    formData.append('file', file);

    var token = localStorage.getItem('authToken');
    var headers = {};
    if (token) headers['Authorization'] = 'Bearer ' + token;

    fetch(BACKEND_URL + '/api/v1/builder/csv/upload', {
        method: 'POST', headers: headers, body: formData
    }).then(function(r) { return r.json(); }).then(function(d) {
        if (d.status === 'success') {
            currentCSVUploadId = d.upload.id;
            showCSVAnalysis(d.upload, d.can_convert_gis);
            var scanMsg = d.scan_safe ? ' (Security scan: PASSED)' : '';
            addLog('Uploaded file: ' + d.upload.filename + scanMsg, 'info');
            loadCSVHistory();
        } else {
            // Show scan failure details
            if (d.findings && d.findings.length > 0) {
                var msgs = d.findings.map(function(f) { return f.severity.toUpperCase() + ': ' + f.description; });
                alert('File rejected by security scanner:\\n\\n' + msgs.join('\\n'));
            } else {
                alert(d.error || 'Upload failed');
            }
        }
    }).catch(function(err) {
        alert('Upload error: ' + err.message);
    });
}

function showCSVAnalysis(upload, canGIS) {
    document.getElementById('csvAnalysisResult').style.display = 'block';
    document.getElementById('csvFilenameDisplay').textContent = upload.filename;

    var badges = '<span class="badge">' + upload.rows + ' rows</span> ' +
        '<span class="badge">' + upload.columns + ' cols</span> ' +
        '<span class="badge badge-' + (upload.has_geo_data ? 'success' : 'default') + '">' +
        (upload.has_geo_data ? 'Geo Data Detected' : 'No Geo Data') + '</span>';
    document.getElementById('csvAnalysisBadges').innerHTML = badges;

    // Column analysis
    var colHtml = '<div class="col-grid">';
    for (var i = 0; i < upload.column_names.length; i++) {
        var ct = (upload.column_types && upload.column_types[i]) || 'string';
        colHtml += '<div class="col-chip col-type-' + ct.replace(/[^a-z_]/g, '') + '">' +
            '<strong>' + escapeHtml(upload.column_names[i]) + '</strong><br><small>' + ct + '</small></div>';
    }
    colHtml += '</div>';
    document.getElementById('csvColumnAnalysis').innerHTML = colHtml;

    // Sample data table
    if (upload.sample_data && upload.sample_data.length > 0) {
        var tHtml = '<table class="admin-table compact"><thead><tr>';
        upload.column_names.forEach(function(c) { tHtml += '<th>' + escapeHtml(c) + '</th>'; });
        tHtml += '</tr></thead><tbody>';
        upload.sample_data.forEach(function(row) {
            tHtml += '<tr>';
            upload.column_names.forEach(function(c) { tHtml += '<td>' + escapeHtml(String(row[c] || '')) + '</td>'; });
            tHtml += '</tr>';
        });
        tHtml += '</tbody></table>';
        document.getElementById('csvSampleTable').innerHTML = tHtml;
    }

    var gisBtn = document.getElementById('btnGenerateGIS');
    if (gisBtn) gisBtn.style.display = canGIS ? 'inline-block' : 'none';
}

function generateCSVDashboard() {
    if (!currentCSVUploadId) return;
    postJSON('/api/v1/builder/csv/uploads/' + currentCSVUploadId + '/generate-dashboard', {}).then(function(d) {
        if (d.status === 'success') {
            addLog('Generated dashboard from CSV: ' + d.dashboard_id, 'info');
            alert('Dashboard created! Go to Analytics page to view it.');
            loadCSVHistory();
        } else {
            alert(d.error || 'Dashboard generation failed');
        }
    });
}

function generateCSVGIS() {
    if (!currentCSVUploadId) return;
    postJSON('/api/v1/builder/csv/uploads/' + currentCSVUploadId + '/generate-gis', {}).then(function(d) {
        if (d.status === 'success') {
            addLog('Generated GIS dataset from CSV: ' + d.dataset_id + ' (' + d.markers_created + ' markers)', 'info');
            alert('GIS dataset created with ' + d.markers_created + ' markers! Go to GIS page to view.');
            loadCSVHistory();
        } else {
            alert(d.error || 'GIS generation failed');
        }
    });
}

function resolveUploadDashboardId(upload) {
    if (!upload) return '';
    return upload.dashboard_id || upload.dashboardId || upload.generated_dashboard_id || '';
}

function confirmDeleteCSVAndDashboard(filename, dashboardId) {
    var fileLabel = String(filename || 'this CSV upload').trim();
    var dashLabel = String(dashboardId || '').trim();
    var message = 'Delete CSV "' + fileLabel + '" and its dashboard' + (dashLabel ? ' (' + dashLabel + ')' : '') + '? This action cannot be undone.';
    if (!confirm(message)) {
        return false;
    }

    var ack = prompt('Type DELETE BOTH to confirm combined deletion for "' + fileLabel + '"');
    if (ack === null) {
        return false;
    }
    if (String(ack).trim().toUpperCase() !== 'DELETE BOTH') {
        alert('Delete cancelled: confirmation text did not match DELETE BOTH.');
        return false;
    }
    return true;
}

function loadCSVHistory() {
    fetchJSON('/api/v1/builder/csv/uploads').then(function(d) {
        var list = d.uploads || [];
        var el = document.getElementById('csvUploadHistory');
        if (!el) return;
        if (list.length === 0) {
            el.innerHTML = '<div class="empty-state">No CSV uploads yet</div>';
            return;
        }
        var html = '<table class="admin-table compact"><thead><tr><th>File</th><th>Type</th><th>Rows</th><th>Cols</th><th>Geo</th><th>Status</th><th>Dashboard</th><th>Actions</th></tr></thead><tbody>';
        list.forEach(function(u) {
            var safeId = u.id.replace(/'/g, "\\'");
            var safeFilename = String(u.filename || 'upload').replace(/'/g, "\\'");
            var dashboardId = resolveUploadDashboardId(u);
            var dashActions = '';
            var rowActions = '<button class="btn-sm btn-del" onclick="deleteCSVUpload(\'' + safeId + '\')">Delete CSV</button>';

            if (dashboardId) {
                var safeDashId = dashboardId.replace(/'/g, "\\'");
                dashActions = '<span>' + escapeHtml(dashboardId) + '</span>';
                rowActions = '<button class="btn-sm btn-del" onclick="deleteCSVAndDashboard(\'' + safeId + '\', \'' + safeDashId + '\', \'' + safeFilename + '\')">Delete CSV + Dashboard</button> ' +
                    '<button class="btn-sm btn-del" onclick="deleteDashboard(\'' + safeDashId + '\')">Delete Dashboard</button> ' + rowActions;
            } else if (u.gis_dashboard_id) {
                dashActions = escapeHtml(u.gis_dashboard_id);
            } else {
                dashActions = '-';
            }
            html += '<tr><td>' + escapeHtml(u.filename) + '</td><td>' + (u.file_type || 'csv').toUpperCase() + '</td><td>' + u.rows + '</td><td>' + u.columns + '</td>' +
                '<td>' + (u.has_geo_data ? 'Yes' : '-') + '</td>' +
                '<td><span class="status-badge status-' + u.status + '">' + u.status + '</span></td>' +
                '<td>' + dashActions + '</td>' +
                '<td>' + rowActions + '</td></tr>';
        });
        html += '</tbody></table>';
        el.innerHTML = html;
    }).catch(function() {});
}

function deleteCSVUpload(id) {
    if (!confirm('Delete this CSV upload?')) return;
    deleteJSON('/api/v1/builder/csv/uploads/' + id).then(function() { loadCSVHistory(); });
}

function deleteCSVAndDashboard(uploadId, dashboardId, filename) {
    if (!canModify()) { alert('You do not have permission to delete dashboards. Contact an admin, manager, or system-manager.'); return; }
    if (!dashboardId) {
        alert('No dashboard is linked to this upload.');
        return;
    }
    if (!confirmDeleteCSVAndDashboard(filename, dashboardId)) return;

    deleteJSON('/api/v1/builder/dashboards/' + dashboardId).then(function(dashResp) {
        if (!(dashResp && dashResp.status === 'success')) {
            alert((dashResp && dashResp.error) || 'Failed to delete dashboard. CSV was not deleted.');
            return;
        }
        deleteJSON('/api/v1/builder/csv/uploads/' + uploadId).then(function(csvResp) {
            if (csvResp && csvResp.status === 'success') {
                addLog('Deleted CSV and dashboard: ' + uploadId + ' / ' + dashboardId, 'warn');
                loadCSVHistory();
            } else {
                alert((csvResp && csvResp.error) || 'Dashboard deleted, but CSV delete failed.');
                loadCSVHistory();
            }
        });
    });
}

// ===================================================================
// Dashboard <-> GIS Converter
// ===================================================================
function loadConverterDropdowns() {
    // Load analytics dashboards
    fetchJSON('/api/v1/analytics/dashboards').then(function(list) {
        var sel = document.getElementById('dashboardSelect');
        if (!sel) return;
        sel.innerHTML = '<option value="">Select a dashboard...</option>';
        (Array.isArray(list) ? list : []).forEach(function(d) {
            sel.innerHTML += '<option value="' + d.id + '">' + escapeHtml(d.name) + ' (' + (d.widgetCount || 0) + ' widgets)</option>';
        });
    }).catch(function() {});

    // Load GIS datasets
    fetchJSON('/api/v1/gis/datasets').then(function(d) {
        var datasets = d.datasets || d || [];
        var sel = document.getElementById('gisDatasetSelect');
        if (!sel) return;
        sel.innerHTML = '<option value="">Select a GIS dataset...</option>';
        (Array.isArray(datasets) ? datasets : []).forEach(function(ds) {
            sel.innerHTML += '<option value="' + ds.id + '">' + escapeHtml(ds.name || ds.id) + '</option>';
        });
    }).catch(function() {});
}

function analyzeDashToGIS() {
    var id = document.getElementById('dashboardSelect').value;
    var el = document.getElementById('dashToGISAnalysis');
    if (!id) { if (el) el.style.display = 'none'; return; }

    postJSON('/api/v1/builder/convert/analyze', { source_type: 'dashboard', source_id: id }).then(function(d) {
        el.style.display = 'block';
        currentDashMappings = d.field_mappings || [];
        var conf = Math.round((d.confidence || 0) * 100);
        var barColor = conf >= 50 ? '#10b981' : (conf >= 30 ? '#f59e0b' : '#ef4444');
        el.innerHTML =
            '<div class="conv-confidence"><div class="conf-bar" style="width:' + conf + '%;background:' + barColor + '"></div></div>' +
            '<div class="conv-text">Confidence: <strong>' + conf + '%</strong> &mdash; ' + escapeHtml(d.suggestion || '') + '</div>' +
            '<div class="conv-fields">Geo fields found: ' + (d.geo_fields_found || []).map(escapeHtml).join(', ') + '</div>';
        var btn = document.getElementById('btnConvertToGIS');
        if (btn) btn.style.display = d.can_convert ? 'inline-block' : 'none';
    }).catch(function() { el.style.display = 'none'; });
}

function convertDashboardToGIS() {
    var id = document.getElementById('dashboardSelect').value;
    if (!id) return;
    postJSON('/api/v1/builder/convert/dashboard-to-gis', { dashboard_id: id, field_mappings: currentDashMappings }).then(function(d) {
        if (d.status === 'success') {
            addLog('Converted dashboard to GIS: ' + d.dataset_id + ' (' + d.markers_created + ' markers)', 'info');
            alert(d.message || 'Conversion successful!');
            loadConversionHistory();
        } else { alert(d.error || 'Conversion failed'); }
    });
}

function analyzeGISToDash() {
    var id = document.getElementById('gisDatasetSelect').value;
    var el = document.getElementById('gisToDashAnalysis');
    if (!id) { if (el) el.style.display = 'none'; return; }

    postJSON('/api/v1/builder/convert/analyze', { source_type: 'gis', source_id: id }).then(function(d) {
        el.style.display = 'block';
        var conf = Math.round((d.confidence || 0) * 100);
        el.innerHTML =
            '<div class="conv-confidence"><div class="conf-bar" style="width:' + conf + '%;background:#10b981"></div></div>' +
            '<div class="conv-text">Confidence: <strong>' + conf + '%</strong> &mdash; ' + escapeHtml(d.suggestion || '') + '</div>';
        var btn = document.getElementById('btnConvertToDash');
        if (btn) btn.style.display = d.can_convert ? 'inline-block' : 'none';
    }).catch(function() { el.style.display = 'none'; });
}

function convertGISToDashboard() {
    var id = document.getElementById('gisDatasetSelect').value;
    if (!id) return;
    postJSON('/api/v1/builder/convert/gis-to-dashboard', { dataset_id: id }).then(function(d) {
        if (d.status === 'success') {
            addLog('Converted GIS to dashboard: ' + d.dashboard_id + ' (' + d.widget_count + ' widgets)', 'info');
            alert(d.message || 'Conversion successful!');
            loadConversionHistory();
        } else { alert(d.error || 'Conversion failed'); }
    });
}

function loadConversionHistory() {
    fetchJSON('/api/v1/builder/conversions').then(function(d) {
        var list = d.conversions || [];
        var el = document.getElementById('conversionHistoryList');
        if (!el) return;
        if (list.length === 0) {
            el.innerHTML = '<div class="empty-state">No conversions yet</div>';
            return;
        }
        var html = '<table class="admin-table compact"><thead><tr><th>Direction</th><th>Source</th><th>Target</th><th>Confidence</th><th>Status</th><th>Date</th></tr></thead><tbody>';
        list.forEach(function(c) {
            var dir = c.source_type === 'dashboard' ? 'Dashboard &rarr; GIS' : 'GIS &rarr; Dashboard';
            html += '<tr><td>' + dir + '</td><td>' + escapeHtml(c.source_id) + '</td><td>' + escapeHtml(c.target_id) + '</td>' +
                '<td>' + Math.round((c.confidence || 0) * 100) + '%</td>' +
                '<td><span class="status-badge status-' + c.status + '">' + c.status + '</span></td>' +
                '<td>' + new Date(c.created_at).toLocaleDateString() + '</td></tr>';
        });
        html += '</tbody></table>';
        el.innerHTML = html;
    }).catch(function() {});
}

// ===================================================================
// API Testing (original functionality)
// ===================================================================
function loadAPIs() {
    var el = document.getElementById('apiCategories');
    if (el) el.innerHTML = '<div class="loading">Loading custom runtime endpoints...</div>';

    Promise.all([
        fetchJSON('/api/v1/builder/apis?api_type=rest&status=active').catch(function() { return { apis: [] }; }),
        fetchJSON('/api/v1/builder/apis?api_type=graphql&status=active').catch(function() { return { apis: [] }; })
    ]).then(function(results) {
        var restAPIs = (results[0] && results[0].apis) ? results[0].apis : [];
        var graphqlAPIs = (results[1] && results[1].apis) ? results[1].apis : [];

        var categories = {
            'Core Platform': [
                { method: 'GET', path: '/health', url: BACKEND_URL + '/health', description: 'Health check', body: null },
                { method: 'GET', path: '/status', url: BACKEND_URL + '/status', description: 'Platform status', body: null }
            ],
            'Notifications': [
                {
                    method: 'POST',
                    path: '/api/v1/notifications/send',
                    url: BACKEND_URL + '/api/v1/notifications/send',
                    description: 'Send custom notification to Discord',
                    body: {
                        title: 'AxiomNizam Notification',
                        message: 'Notification API restored and working.',
                        type: 'info'
                    }
                },
                {
                    method: 'POST',
                    path: '/api/v1/notifications/health',
                    url: BACKEND_URL + '/api/v1/notifications/health',
                    description: 'Send health check notification',
                    body: null
                },
                {
                    method: 'POST',
                    path: '/api/v1/notifications/status',
                    url: BACKEND_URL + '/api/v1/notifications/status',
                    description: 'Send platform status notification',
                    body: null
                },
                {
                    method: 'GET',
                    path: '/api/v1/notifications/status',
                    url: BACKEND_URL + '/api/v1/notifications/status',
                    description: 'Get notification service status',
                    body: null
                }
            ],
            'Custom REST Runtime (API Builder)': restAPIs.map(function(api) {
                var method = String(api.method || 'GET').toUpperCase();
                var runtimePath = normalizeRuntimePath(api.path || '/');
                var url = BACKEND_URL + '/api/custom' + (runtimePath === '/' ? '' : runtimePath);
                var body = null;
                if (method !== 'GET') {
                    var paramsBody = {};
                    (api.query_params || []).forEach(function(p) {
                        paramsBody[p.name] = sampleParamValue(p);
                    });
                    body = { params: paramsBody };
                } else if ((api.query_params || []).length > 0) {
                    var queryPairs = [];
                    api.query_params.forEach(function(p) {
                        queryPairs.push(encodeURIComponent(p.name) + '=' + encodeURIComponent(sampleParamValue(p)));
                    });
                    if (queryPairs.length > 0) {
                        url += (url.indexOf('?') >= 0 ? '&' : '?') + queryPairs.join('&');
                    }
                }

                return {
                    method: method,
                    path: '/api/custom' + (runtimePath === '/' ? '' : runtimePath),
                    url: url,
                    description: api.name || 'Custom runtime API',
                    body: body
                };
            }),
            'Custom GraphQL Runtime (API Builder)': graphqlAPIs.map(function(api) {
                return {
                    method: String(api.method || 'POST').toUpperCase(),
                    path: api.path || '/api/graphql',
                    url: BACKEND_URL + normalizeRuntimePath(api.path || '/api/graphql'),
                    description: api.name || 'Custom GraphQL API',
                    body: {
                        query: api.graphql_query || 'query { __typename }',
                        operationName: api.graphql_operation_name || '',
                        variables: {}
                    }
                };
            })
        };

        var html = '';
        for (var category in categories) {
            var apis = categories[category] || [];
            if (apis.length === 0) continue;
            html += '<div class="api-category"><div class="category-header">' + escapeHtml(category) + '</div><div class="api-items">';
            for (var i = 0; i < apis.length; i++) {
                var api = apis[i];
                html += '<button class="api-test-btn api-method-' + String(api.method || 'GET').toLowerCase() + '" ' +
                    'onclick="testAPI(\'' + api.method + '\', \'' + api.url + '\', ' +
                    (api.body ? JSON.stringify(api.body).replace(/'/g, '&#39;') : 'null') + ', \'' + escapeHtml(api.description).replace(/'/g, '&#39;') + '\')">' +
                    '<span class="api-method ' + String(api.method || 'GET').toLowerCase() + '">' + escapeHtml(String(api.method || 'GET')) + '</span>' +
                    '<span style="font-weight:500">' + escapeHtml(api.path) + '</span>' +
                    '<span style="font-size:0.85em;color:#999">' + escapeHtml(api.description) + '</span>' +
                    '</button>';
            }
            html += '</div></div>';
        }

        if (!html) {
            html = '<div class="empty-state">No custom APIs available yet. Create APIs in API Builder and return here to test runtime endpoints.</div>';
        }

        if (el) el.innerHTML = html;
        filterAPIs();
    }).catch(function() {
        if (el) el.innerHTML = '<div class="error-state">Failed to load runtime endpoints</div>';
    });
}

function normalizeRuntimePath(path) {
    var normalized = '/' + String(path || '').trim().replace(/^\/+|\/+$/g, '');
    if (normalized === '/api/custom') return '/';
    if (normalized.indexOf('/api/custom/') === 0) {
        normalized = normalized.replace('/api/custom', '');
        normalized = '/' + normalized.trim().replace(/^\/+|\/+$/g, '');
    }
    if (normalized === '/api/graphql') return '/api/graphql';
    return normalized;
}

function sampleParamValue(param) {
    if (!param) return 'sample';
    if (param.default && String(param.default).trim() !== '') return param.default;
    var t = String(param.type || '').toLowerCase();
    if (t === 'int' || t === 'integer' || t === 'number' || t === 'float' || t === 'decimal') return 1;
    if (t === 'bool' || t === 'boolean') return true;
    return 'sample';
}

function filterAPIs() {
    var searchTerm = document.getElementById('apiSearch').value.toLowerCase();
    var buttons = document.querySelectorAll('.api-test-btn');

    buttons.forEach(function(btn) {
        var text = btn.textContent.toLowerCase();
        var matches = text.indexOf(searchTerm) >= 0;
        var methodMatches = filteredMethod === 'ALL' || btn.classList.contains('api-method-' + filteredMethod.toLowerCase());
        btn.style.display = (matches && methodMatches) ? 'flex' : 'none';
    });
}

function filterByMethod(method) {
    filteredMethod = method;
    var filterBtns = document.querySelectorAll('.filter-btn');
    filterBtns.forEach(function(btn) { btn.classList.remove('active'); });
    if (event && event.target) event.target.classList.add('active');
    filterAPIs();
}

function testAPI(method, url, body, description) {
    var options = {
        method: method,
        headers: getAuthHeaders()
    };

    if (body && (method === 'POST' || method === 'PUT' || method === 'PATCH')) {
        options.body = typeof body === 'string' ? body : JSON.stringify(body);
    }

    showSpinner(description);

    fetch(url, options)
        .then(function(response) {
            var status = response.status;
            return response.json().catch(function() { return null; }).then(function(data) {
                return { status: status, data: data };
            });
        })
        .then(function(result) {
            showResponse(description, result.status, result.data, method + ' ' + url);
            addLog('API Call: ' + method + ' ' + url, 'info');
        })
        .catch(function(error) {
            showResponse(description, 'ERROR', {error: error.message}, method + ' ' + url);
            addLog('API Error: ' + error.message, 'error');
        });
}

// ===================================================================
// API Scanner Reports (Admin UI)
// ===================================================================
function toggleAdminApiScanFields() {
    var typeEl = document.getElementById('adminApiScanType');
    if (!typeEl) return;

    var scanType = typeEl.value;
    var runtimeOnly = document.querySelectorAll('.scan-runtime-only');
    var discoveryOnly = document.querySelectorAll('.scan-discovery-only');
    var discoverAPIOnly = document.querySelectorAll('.scan-discover-api-only');
    var discoverDomainOnly = document.querySelectorAll('.scan-discover-domain-only');

    runtimeOnly.forEach(function(el) { el.style.display = (scanType === 'runtime') ? '' : 'none'; });
    discoveryOnly.forEach(function(el) { el.style.display = (scanType === 'runtime') ? 'none' : ''; });
    discoverAPIOnly.forEach(function(el) { el.style.display = (scanType === 'discover-api') ? '' : 'none'; });
    discoverDomainOnly.forEach(function(el) { el.style.display = (scanType === 'discover-domain') ? '' : 'none'; });

    // Update the custom icon based on selection
    var iconEl = document.getElementById('adminApiScanTypeIcon');
    if (iconEl) {
        if (scanType === 'runtime') {
            iconEl.innerHTML = '<svg style="width:20px;height:20px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon></svg>';
        } else if (scanType === 'discover-api') {
            iconEl.innerHTML = '<svg style="width:20px;height:20px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"></circle><line x1="21" y1="21" x2="16.65" y2="16.65"></line></svg>';
        } else if (scanType === 'discover-domain') {
            iconEl.innerHTML = '<svg style="width:20px;height:20px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><line x1="2" y1="12" x2="22" y2="12"></line><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"></path></svg>';
        }
    }
}

function parseAdminTextList(raw) {
    var parts = String(raw || '').split(/[\n,]+/);
    return parts.map(function(p) { return p.trim(); }).filter(function(p) { return p.length > 0; });
}

function parseAdminHeaderMap(raw) {
    var map = {};
    String(raw || '').split(/\n+/).forEach(function(line) {
        var item = line.trim();
        if (!item) return;
        var idx = item.indexOf(':');
        if (idx < 1) return;
        var key = item.slice(0, idx).trim();
        var value = item.slice(idx + 1).trim();
        if (key) map[key] = value;
    });
    return map;
}

function buildAdminApiScanPayload() {
    var scanType = (document.getElementById('adminApiScanType').value || 'runtime').trim();
    var target = (document.getElementById('adminApiScanTarget').value || '').trim();
    var timeoutSeconds = parseInt(document.getElementById('adminApiScanTimeout').value, 10) || 30;
    var headers = parseAdminHeaderMap(document.getElementById('adminApiScanHeaders').value);

    var payload = {
        scan_type: scanType,
        target: target,
        headers: headers,
        timeout_seconds: timeoutSeconds,
        include_scan_ids: parseAdminTextList(document.getElementById('adminApiScanIncludeIDs').value),
        exclude_scan_ids: parseAdminTextList(document.getElementById('adminApiScanExcludeIDs').value)
    };

    if (scanType === 'runtime') {
        payload.method = (document.getElementById('adminApiScanMethod').value || 'GET').trim();
        payload.body = document.getElementById('adminApiScanBody').value || '';
        payload.retry_count = 1;
        payload.retry_backoff_ms = 1000;
    }

    if (scanType === 'discover-api') {
        payload.max_paths = parseInt(document.getElementById('adminApiScanMaxPaths').value, 10) || 64;
        payload.insecure_skip_verify = !!document.getElementById('adminApiScanInsecure').checked;
    }

    if (scanType === 'discover-domain') {
        payload.max_subdomains = parseInt(document.getElementById('adminApiScanMaxSubdomains').value, 10) || 32;
        payload.max_hints = parseInt(document.getElementById('adminApiScanMaxHints').value, 10) || 48;
        payload.schemes = parseAdminTextList(document.getElementById('adminApiScanSchemes').value);
    }

    return payload;
}

function runAdminApiScan() {
    var target = (document.getElementById('adminApiScanTarget').value || '').trim();
    if (!target) {
        alert('Please enter a scan target URL/domain.');
        return;
    }

    var runBtn = document.getElementById('adminRunApiScanBtn');
    var output = document.getElementById('adminApiScanReportOutput');
    var summaryEl = document.getElementById('adminApiScanSummary');
    var structuredEl = document.getElementById('adminApiScanStructured');
    if (runBtn) runBtn.disabled = true;
    if (output) output.textContent = 'Running scan...';
    if (summaryEl) summaryEl.innerHTML = '<span class="badge">Running scan...</span>';
    if (structuredEl) structuredEl.innerHTML = '<div class="loading">Collecting report details...</div>';

    fetch(BACKEND_URL + '/api/v1/builder/api-scanner/scan', {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify(buildAdminApiScanPayload())
    }).then(function(response) {
        return response.json().then(function(data) {
            if (!response.ok) {
                var errMessage = (data && (data.error || data.details)) || ('Scan request failed with status ' + response.status);
                throw new Error(errMessage);
            }
            return data;
        });
    }).then(function(data) {
        latestAdminAPIScanReport = data.report;
        renderAdminApiScanReport(data.report);
        loadAdminApiScanReports();
        var downloadBtn = document.getElementById('adminDownloadApiScanBtn');
        if (downloadBtn) downloadBtn.disabled = false;
        addLog('API scan report generated: ' + data.report.id, 'info');
    }).catch(function(err) {
        if (output) output.textContent = 'Scan failed\n\n' + err.message;
        if (summaryEl) summaryEl.innerHTML = '<span class="badge badge-danger">Scan Failed</span>';
        if (structuredEl) structuredEl.innerHTML = '<div class="error-state">' + escapeHtml(err.message || 'Scan failed') + '</div>';
        addLog('API scan failed: ' + err.message, 'error');
    }).finally(function() {
        if (runBtn) runBtn.disabled = false;
    });
}

function adminScanPickValue(obj, keys, fallback) {
    if (!obj || typeof obj !== 'object') return fallback;
    for (var i = 0; i < keys.length; i++) {
        if (Object.prototype.hasOwnProperty.call(obj, keys[i]) && obj[keys[i]] !== undefined && obj[keys[i]] !== null) {
            return obj[keys[i]];
        }
    }
    return fallback;
}

function adminScanToInt(value) {
    var n = Number(value);
    if (!isFinite(n)) return 0;
    return Math.floor(n);
}

function adminScanGetResult(report) {
    if (!report || typeof report !== 'object') return {};
    if (!report.result || typeof report.result !== 'object') return {};
    return report.result;
}

function adminScanGetSummary(report) {
    var fromReport = adminScanPickValue(report, ['summary'], null);
    if (fromReport && typeof fromReport === 'object') return fromReport;
    var result = adminScanGetResult(report);
    var fromResult = adminScanPickValue(result, ['summary'], null);
    if (fromResult && typeof fromResult === 'object') return fromResult;
    return {};
}

function adminScanBadge(label, value, variant) {
    var cls = 'badge';
    if (variant === 'success') cls += ' badge-success';
    if (variant === 'danger') cls += ' badge-danger';
    if (variant === 'default') cls += ' badge-default';
    var fullText = escapeHtml(label) + ': ' + escapeHtml(String(value));
    return '<span class="' + cls + '" title="' + fullText + '">' + fullText + '</span>';
}

function adminScanSeverityClass(severity) {
    var s = String(severity || '').toLowerCase();
    if (s === 'critical' || s === 'high') return 'status-inactive';
    if (s === 'medium') return 'status-draft';
    return 'status-active';
}

function adminScanFormatWhen(value) {
    if (!value) return '-';
    var d = new Date(value);
    if (isNaN(d.getTime())) return String(value);
    return d.toLocaleString();
}

function adminScanRenderOverview(report, result) {
    var scanner = adminScanPickValue(result, ['scanner'], '-');
    var method = adminScanPickValue(report, ['method'], '');
    var scannedAt = adminScanPickValue(result, ['scannedAt', 'scanned_at'], report.created_at || report.createdAt || '');
    var html = '<div class="table-responsive"><table class="admin-table compact" style="margin:0;"><tbody>';
    html += '<tr><th style="width:160px; background:rgba(0,0,0,0.02);">Report ID</th><td><code style="background:transparent; padding:0; font-weight:600;">' + escapeHtml(report.id || '-') + '</code></td></tr>';
    html += '<tr><th style="background:rgba(0,0,0,0.02);">Type & Target</th><td>' + escapeHtml(report.scan_type || report.scanType || '-') + ' <span style="color:var(--text-muted);">&rarr;</span> <code>' + escapeHtml(report.target || '-') + '</code></td></tr>';
    html += '<tr><th style="background:rgba(0,0,0,0.02);">Scanner</th><td>' + escapeHtml(scanner || '-') + '</td></tr>';
    html += '<tr><th style="background:rgba(0,0,0,0.02);">Scanned At</th><td>' + escapeHtml(adminScanFormatWhen(scannedAt)) + '</td></tr>';
    html += '</tbody></table></div>';
    return '<h5 style="margin:0 0 12px 0; color:var(--text-secondary);">Overview</h5>' + html;
}

function adminScanRenderRuntimeFindings(result) {
    var findings = adminScanPickValue(result, ['findings'], []);
    if (!Array.isArray(findings) || findings.length === 0) {
        return '<h5 style="margin:20px 0 12px 0; color:var(--text-secondary); border-top:1px dashed var(--border-color); padding-top:16px;">Vulnerability Findings</h5><div class="empty-state" style="background:rgba(var(--primary-rgb),0.05); color:var(--success-color); border:1px solid rgba(var(--primary-rgb),0.2);">No findings were returned for this runtime scan. Excellent!</div>';
    }

    var html = '<h5 style="margin:20px 0 12px 0; color:var(--danger-color); border-top:1px dashed var(--border-color); padding-top:16px;">Vulnerability Findings (' + findings.length + ')</h5>';
    html += '<div class="table-responsive"><table class="admin-table compact" style="margin:0;"><thead><tr><th>Severity</th><th>Type</th><th>Title</th><th>Endpoint</th><th>Evidence</th></tr></thead><tbody>';
    findings.forEach(function(finding) {
        var sev = String(adminScanPickValue(finding, ['severity'], 'info')).toUpperCase();
        var endpoint = adminScanPickValue(finding, ['endpoint'], '-');
        var evidence = adminScanPickValue(finding, ['evidence', 'description'], '');
        if (typeof evidence === 'string' && evidence.length > 140) {
            evidence = evidence.slice(0, 140) + '...';
        }
        var trStyle = (sev === 'CRITICAL' || sev === 'HIGH') ? 'style="background:rgba(244,42,65,0.05);"' : '';
        html += '<tr ' + trStyle + '>' +
            '<td><span class="badge ' + adminScanSeverityClass(sev) + '">' + escapeHtml(sev) + '</span></td>' +
            '<td style="font-weight:600;">' + escapeHtml(String(adminScanPickValue(finding, ['type'], '-'))) + '</td>' +
            '<td>' + escapeHtml(String(adminScanPickValue(finding, ['title'], '-'))) + '</td>' +
            '<td><code>' + escapeHtml(String(endpoint)) + '</code></td>' +
            '<td><small style="color:var(--text-muted);">' + escapeHtml(String(evidence || '-')) + '</small></td>' +
            '</tr>';
    });
    html += '</tbody></table></div>';
    return html;
}

function adminScanRenderChecks(result) {
    var checks = adminScanPickValue(result, ['checks'], []);
    if (!Array.isArray(checks) || checks.length === 0) {
        return '';
    }

    var html = '<h5 style="margin:0 0 12px 0; color:var(--text-secondary);">Check Coverage</h5>';
    html += '<div class="table-responsive"><table class="admin-table compact" style="margin:0;"><thead><tr><th style="width:50%;">Check</th><th>Executed</th><th>Findings</th></tr></thead><tbody>';
    checks.forEach(function(check) {
        var executed = !!adminScanPickValue(check, ['executed'], false);
        var findings = adminScanToInt(adminScanPickValue(check, ['findings'], 0));
        var statusClass = executed ? (findings > 0 ? 'badge-warning' : 'badge-success') : 'badge-danger';
        var findingStyle = findings > 0 ? 'color:var(--danger-color); font-weight:bold;' : 'color:var(--text-secondary);';
        
        html += '<tr style="background:rgba(0,0,0,0.01);">' +
            '<td style="font-weight:500;">' + escapeHtml(String(adminScanPickValue(check, ['name', 'id'], '-'))) + '</td>' +
            '<td><span class="badge ' + statusClass + '">' + (executed ? 'Yes' : 'Skipped') + '</span></td>' +
            '<td style="' + findingStyle + '">' + findings + '</td>' +
            '</tr>';
    });
    html += '</tbody></table></div>';
    return html;
}

function adminScanRenderDiscoveryRows(discovered, title) {
    if (!Array.isArray(discovered) || discovered.length === 0) {
        return '<h5 style="margin:0 0 8px 0;">' + escapeHtml(title) + '</h5><div class="empty-state">No records found.</div>';
    }

    var html = '<h5 style="margin:0 0 8px 0;">' + escapeHtml(title) + '</h5>';
    html += '<table class="admin-table compact"><thead><tr><th>Check ID</th><th>Name</th><th>Category</th><th>URL</th><th>Status</th></tr></thead><tbody>';
    discovered.forEach(function(item) {
        var statusCode = adminScanPickValue(item, ['statusCode', 'status_code'], '-');
        html += '<tr>' +
            '<td>' + escapeHtml(String(adminScanPickValue(item, ['checkId', 'check_id'], '-'))) + '</td>' +
            '<td>' + escapeHtml(String(adminScanPickValue(item, ['name'], '-'))) + '</td>' +
            '<td>' + escapeHtml(String(adminScanPickValue(item, ['category'], '-'))) + '</td>' +
            '<td><code>' + escapeHtml(String(adminScanPickValue(item, ['url'], '-'))) + '</code></td>' +
            '<td>' + escapeHtml(String(statusCode)) + '</td>' +
            '</tr>';
    });
    html += '</tbody></table>';
    return html;
}

function adminScanRenderFingerprints(result) {
    var fps = adminScanPickValue(result, ['fingerprints'], []);
    if (!Array.isArray(fps) || fps.length === 0) {
        return '<h5 style="margin:0 0 8px 0;">Fingerprints</h5><div class="empty-state">No fingerprint data found.</div>';
    }

    var html = '<h5 style="margin:0 0 8px 0;">Fingerprints</h5>';
    html += '<table class="admin-table compact"><thead><tr><th>Type</th><th>Value</th><th>Source Header</th></tr></thead><tbody>';
    fps.forEach(function(fp) {
        html += '<tr>' +
            '<td>' + escapeHtml(String(adminScanPickValue(fp, ['type'], '-'))) + '</td>' +
            '<td><code>' + escapeHtml(String(adminScanPickValue(fp, ['value'], '-'))) + '</code></td>' +
            '<td>' + escapeHtml(String(adminScanPickValue(fp, ['sourceHeader', 'source_header'], '-'))) + '</td>' +
            '</tr>';
    });
    html += '</tbody></table>';
    return html;
}

function adminScanRenderResolvedSubdomains(result) {
    var resolved = adminScanPickValue(result, ['resolved'], []);
    if (!Array.isArray(resolved) || resolved.length === 0) {
        return '<h5 style="margin:0 0 8px 0;">Resolved Subdomains</h5><div class="empty-state">No subdomains resolved.</div>';
    }

    var html = '<h5 style="margin:0 0 8px 0;">Resolved Subdomains</h5>';
    html += '<table class="admin-table compact"><thead><tr><th>Name</th><th>Host</th><th>Addresses</th></tr></thead><tbody>';
    resolved.forEach(function(item) {
        var addresses = adminScanPickValue(item, ['addresses'], []);
        html += '<tr>' +
            '<td>' + escapeHtml(String(adminScanPickValue(item, ['name'], '-'))) + '</td>' +
            '<td><code>' + escapeHtml(String(adminScanPickValue(item, ['host'], '-'))) + '</code></td>' +
            '<td>' + escapeHtml(Array.isArray(addresses) ? addresses.join(', ') : '-') + '</td>' +
            '</tr>';
    });
    html += '</tbody></table>';
    return html;
}

function adminScanRenderSummaryBadges(report) {
    var summary = adminScanGetSummary(report);
    var scanType = String(report.scan_type || report.scanType || 'runtime').toLowerCase();
    var badges = [];

    badges.push(adminScanBadge('Type', report.scan_type || report.scanType || '-', 'default'));
    badges.push(adminScanBadge('Target', report.target || '-', 'default'));
    badges.push(adminScanBadge('Created', adminScanFormatWhen(report.created_at || report.createdAt), 'default'));

    if (scanType === 'runtime' || scanType === 'scan-api' || scanType === 'api') {
        var total = adminScanToInt(adminScanPickValue(summary, ['total', 'Total'], 0));
        var critical = adminScanToInt(adminScanPickValue(summary, ['critical', 'Critical'], 0));
        var high = adminScanToInt(adminScanPickValue(summary, ['high', 'High'], 0));
        var medium = adminScanToInt(adminScanPickValue(summary, ['medium', 'Medium'], 0));
        badges.push(adminScanBadge('Findings', total, total > 0 ? 'danger' : 'success'));
        badges.push(adminScanBadge('Critical', critical, critical > 0 ? 'danger' : 'default'));
        badges.push(adminScanBadge('High', high, high > 0 ? 'danger' : 'default'));
        badges.push(adminScanBadge('Medium', medium, medium > 0 ? 'default' : 'success'));
    } else if (scanType === 'discover-api' || scanType === 'api-discovery') {
        var totalPaths = adminScanToInt(adminScanPickValue(summary, ['totalPaths', 'total_paths'], 0));
        var wellKnown = adminScanToInt(adminScanPickValue(summary, ['wellKnown', 'well_known'], 0));
        var exposed = adminScanToInt(adminScanPickValue(summary, ['exposedFiles', 'exposed_files'], 0));
        var fingerprints = adminScanToInt(adminScanPickValue(summary, ['fingerprints'], 0));
        badges.push(adminScanBadge('Discovered Paths', totalPaths, totalPaths > 0 ? 'success' : 'default'));
        badges.push(adminScanBadge('Well-Known', wellKnown, 'default'));
        badges.push(adminScanBadge('Exposed Files', exposed, exposed > 0 ? 'danger' : 'success'));
        badges.push(adminScanBadge('Fingerprints', fingerprints, 'default'));
    } else {
        var candidates = adminScanToInt(adminScanPickValue(summary, ['candidates'], 0));
        var resolved = adminScanToInt(adminScanPickValue(summary, ['resolved'], 0));
        var hints = adminScanToInt(adminScanPickValue(summary, ['apiHints', 'api_hints'], 0));
        var fps = adminScanToInt(adminScanPickValue(summary, ['fingerprints'], 0));
        badges.push(adminScanBadge('Candidates', candidates, 'default'));
        badges.push(adminScanBadge('Resolved', resolved, resolved > 0 ? 'success' : 'default'));
        badges.push(adminScanBadge('API Hints', hints, hints > 0 ? 'success' : 'default'));
        badges.push(adminScanBadge('Fingerprints', fps, 'default'));
    }

    return badges.join(' ');
}

function adminScanRenderStructured(report) {
    var result = adminScanGetResult(report);
    var scanType = String(report.scan_type || report.scanType || 'runtime').toLowerCase();
    var blocks = [];

    blocks.push('<div style="margin-bottom:10px;">' + adminScanRenderOverview(report, result) + '</div>');
    blocks.push('<div style="margin-bottom:10px;">' + adminScanRenderChecks(result) + '</div>');

    if (scanType === 'runtime' || scanType === 'scan-api' || scanType === 'api') {
        blocks.push('<div style="margin-bottom:10px;">' + adminScanRenderRuntimeFindings(result) + '</div>');
    } else if (scanType === 'discover-api' || scanType === 'api-discovery') {
        blocks.push('<div style="margin-bottom:10px;">' + adminScanRenderDiscoveryRows(adminScanPickValue(result, ['discovered'], []), 'Discovered Endpoints') + '</div>');
        blocks.push('<div style="margin-bottom:10px;">' + adminScanRenderFingerprints(result) + '</div>');
    } else {
        blocks.push('<div style="margin-bottom:10px;">' + adminScanRenderResolvedSubdomains(result) + '</div>');
        blocks.push('<div style="margin-bottom:10px;">' + adminScanRenderDiscoveryRows(adminScanPickValue(result, ['apiHints', 'api_hints'], []), 'API Hints') + '</div>');
        blocks.push('<div style="margin-bottom:10px;">' + adminScanRenderFingerprints(result) + '</div>');
    }

    return blocks.join('');
}

function renderAdminApiScanReport(report) {
    var output = document.getElementById('adminApiScanReportOutput');
    var summaryEl = document.getElementById('adminApiScanSummary');
    var structuredEl = document.getElementById('adminApiScanStructured');

    if (!report || typeof report !== 'object') {
        if (summaryEl) summaryEl.innerHTML = '<span class="badge">No report selected</span>';
        if (structuredEl) structuredEl.innerHTML = '<div class="empty-state">Run a scan or open a report to view details.</div>';
        if (output) output.textContent = 'Run a scan to generate a report.';
        return;
    }

    if (summaryEl) summaryEl.innerHTML = adminScanRenderSummaryBadges(report);
    if (structuredEl) structuredEl.innerHTML = adminScanRenderStructured(report);
    if (output) output.textContent = JSON.stringify(report, null, 2);
}

function loadAdminApiScanReports() {
    var el = document.getElementById('adminApiScanReportsList');
    if (!el) return;

    fetchJSON('/api/v1/builder/api-scanner/reports').then(function(data) {
        if (data && data.error) throw new Error(data.error);
        var reports = data.reports || [];
        if (reports.length === 0) {
            el.innerHTML = '<div class="empty-state">No API scan reports yet.</div>';
            updateAdminApiScanSelectedCount();
            return;
        }

        var html = '<table class="admin-table compact" style="margin:0;"><thead><tr><th style="width:40px;"><input type="checkbox" id="adminApiScanSelectAll" onchange="toggleAllAdminApiScanReportsSelection(this.checked)" aria-label="Select all reports"></th><th>ID</th><th>Type</th><th>Target</th><th>Summary</th><th>Created</th><th>Actions</th></tr></thead><tbody>';
        reports.forEach(function(report) {
            var summary = adminScanGetSummary(report);
            var scanType = String(report.scan_type || report.scanType || '').toLowerCase();
            var summaryLabel = '-';

            if (scanType === 'runtime' || scanType === 'scan-api' || scanType === 'api') {
                summaryLabel = 'Findings: ' + adminScanToInt(adminScanPickValue(summary, ['total', 'Total'], 0));
            } else if (scanType === 'discover-api' || scanType === 'api-discovery') {
                summaryLabel = 'Paths: ' + adminScanToInt(adminScanPickValue(summary, ['totalPaths', 'total_paths'], 0));
            } else {
                summaryLabel = 'Hints: ' + adminScanToInt(adminScanPickValue(summary, ['apiHints', 'api_hints'], 0));
            }

            html += '<tr>' +
                '<td><input type="checkbox" class="admin-api-scan-select" value="' + escapeHtml(report.id) + '" onchange="updateAdminApiScanSelectedCount()" aria-label="Select report ' + escapeHtml(report.id) + '"></td>' +
                '<td><strong>' + escapeHtml(report.id.substring(0,14)) + '...</strong></td>' +
                '<td><span class="status-badge" style="background:rgba(var(--primary-rgb),0.1); color:var(--primary-light);">' + escapeHtml(report.scan_type) + '</span></td>' +
                '<td><code style="background:rgba(0,0,0,0.1); padding:2px 6px; border-radius:4px;">' + escapeHtml(report.target || '-') + '</code></td>' +
                '<td><span style="font-weight:500; font-size:0.85rem;">' + escapeHtml(summaryLabel) + '</span></td>' +
                '<td style="color:var(--text-muted); font-size:0.85rem;">' + new Date(report.created_at).toLocaleString() + '</td>' +
                '<td>' +
                '<div style="display:flex; gap:6px;">' +
                '<button class="btn-sm btn-test" onclick="viewAdminApiScanReport(\'' + report.id + '\')" title="View"><svg style="width:14px;height:14px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path><circle cx="12" cy="12" r="3"></circle></svg></button> ' +
                '<button class="btn-sm btn-secondary" onclick="downloadAdminApiScanReportById(\'' + report.id + '\')" title="Download"><svg style="width:14px;height:14px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg></button> ' +
                '<button class="btn-sm btn-del" onclick="deleteAdminApiScanReport(\'' + report.id + '\')" title="Delete"><svg style="width:14px;height:14px;" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path></svg></button>' +
                '</div>' +
                '</td>' +
                '</tr>';
        });
        html += '</tbody></table>';
        el.innerHTML = html;
        updateAdminApiScanSelectedCount();
    }).catch(function(err) {
        el.innerHTML = '<div class="error-state">Failed to load API scan reports: ' + escapeHtml(err.message || '') + '</div>';
        updateAdminApiScanSelectedCount();
    });
}

function getSelectedAdminApiScanReportIds() {
    var selected = document.querySelectorAll('.admin-api-scan-select:checked');
    var ids = [];
    selected.forEach(function(checkbox) {
        var value = String(checkbox.value || '').trim();
        if (value) ids.push(value);
    });
    return ids;
}

function updateAdminApiScanSelectedCount() {
    var selectedCount = getSelectedAdminApiScanReportIds().length;
    var totalCount = document.querySelectorAll('.admin-api-scan-select').length;
    var countEl = document.getElementById('adminApiScanSelectedCount');
    if (countEl) countEl.textContent = selectedCount + ' selected';

    var selectAll = document.getElementById('adminApiScanSelectAll');
    if (selectAll) {
        selectAll.checked = totalCount > 0 && selectedCount === totalCount;
        selectAll.indeterminate = selectedCount > 0 && selectedCount < totalCount;
    }
}

function toggleAllAdminApiScanReportsSelection(checked) {
    var checkboxes = document.querySelectorAll('.admin-api-scan-select');
    checkboxes.forEach(function(checkbox) {
        checkbox.checked = !!checked;
    });
    updateAdminApiScanSelectedCount();
}

function performAdminApiScanBulkDelete(payload, doneMessage) {
    fetch(BACKEND_URL + '/api/v1/builder/api-scanner/reports/bulk-delete', {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify(payload)
    }).then(function(response) {
        return response.json().then(function(data) {
            if (!response.ok) {
                var errMessage = (data && (data.error || data.message)) || ('Bulk delete failed with status ' + response.status);
                throw new Error(errMessage);
            }
            return data;
        });
    }).then(function(data) {
        var deletedCount = Number((data && data.deleted_count) || 0);
        if (deletedCount > 0 && latestAdminAPIScanReport) {
            var deletedIDs = (data && data.deleted_ids) || [];
            if (Array.isArray(deletedIDs) && deletedIDs.indexOf(latestAdminAPIScanReport.id) >= 0) {
                clearAdminApiScanView();
            }
        }
        loadAdminApiScanReports();
        addLog(doneMessage + ' (' + deletedCount + ' report(s))', 'info');
    }).catch(function(err) {
        alert('Bulk delete failed: ' + (err.message || 'Unknown error'));
    });
}

function deleteSelectedAdminApiScanReports() {
    var ids = getSelectedAdminApiScanReportIds();
    if (ids.length === 0) {
        alert('Select one or more reports first.');
        return;
    }
    if (!confirm('Delete ' + ids.length + ' selected API scan report(s)?')) return;
    performAdminApiScanBulkDelete({ ids: ids, all: false }, 'Deleted selected API scan reports');
}

function deleteAllAdminApiScanReports() {
    var totalCount = document.querySelectorAll('.admin-api-scan-select').length;
    if (totalCount === 0) {
        alert('No reports to delete.');
        return;
    }
    if (!confirm('Delete ALL API scan reports (' + totalCount + ')? This cannot be undone.')) return;
    performAdminApiScanBulkDelete({ all: true }, 'Deleted all API scan reports');
}

function viewAdminApiScanReport(id) {
    fetchJSON('/api/v1/builder/api-scanner/reports/' + encodeURIComponent(id)).then(function(data) {
        if (data && data.error) throw new Error(data.error);
        latestAdminAPIScanReport = data.report;
        renderAdminApiScanReport(data.report);
        var downloadBtn = document.getElementById('adminDownloadApiScanBtn');
        if (downloadBtn) downloadBtn.disabled = false;
    }).catch(function(err) {
        var output = document.getElementById('adminApiScanReportOutput');
        var summaryEl = document.getElementById('adminApiScanSummary');
        var structuredEl = document.getElementById('adminApiScanStructured');
        if (output) output.textContent = 'Failed to load report\n\n' + (err.message || 'Unknown error');
        if (summaryEl) summaryEl.innerHTML = '<span class="badge badge-danger">Load Failed</span>';
        if (structuredEl) structuredEl.innerHTML = '<div class="error-state">' + escapeHtml(err.message || 'Unknown error') + '</div>';
    });
}

function downloadLatestAdminApiScanReport() {
    if (!latestAdminAPIScanReport) {
        alert('No report loaded. Run or view a report first.');
        return;
    }
    downloadAdminApiScanReportObject(latestAdminAPIScanReport);
}

function downloadAdminApiScanReportById(id) {
    if (latestAdminAPIScanReport && latestAdminAPIScanReport.id === id) {
        downloadAdminApiScanReportObject(latestAdminAPIScanReport);
        return;
    }

    fetchJSON('/api/v1/builder/api-scanner/reports/' + encodeURIComponent(id)).then(function(data) {
        downloadAdminApiScanReportObject(data.report);
    }).catch(function(err) {
        alert('Failed to download report JSON: ' + (err.message || 'Unknown error'));
    });
}

function deleteAdminApiScanReport(id) {
    if (!id) return;
    if (!confirm('Delete API scan report ' + id + '?')) return;

    fetch(BACKEND_URL + '/api/v1/builder/api-scanner/reports/' + encodeURIComponent(id), {
        method: 'DELETE',
        headers: getAuthHeaders()
    }).then(function(response) {
        return response.json().then(function(data) {
            if (!response.ok) {
                var errMessage = (data && (data.error || data.message)) || ('Delete failed with status ' + response.status);
                throw new Error(errMessage);
            }
            return data;
        });
    }).then(function() {
        if (latestAdminAPIScanReport && latestAdminAPIScanReport.id === id) {
            clearAdminApiScanView();
        }
        loadAdminApiScanReports();
        addLog('Deleted API scan report: ' + id, 'info');
    }).catch(function(err) {
        alert('Failed to delete report: ' + (err.message || 'Unknown error'));
    });
}

function copyLatestAdminApiScanReportId() {
    if (!latestAdminAPIScanReport || !latestAdminAPIScanReport.id) {
        alert('No report loaded. Run or view a report first.');
        return;
    }

    var id = String(latestAdminAPIScanReport.id);
    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(id).then(function() {
            addLog('Copied API scan report ID: ' + id, 'info');
        }).catch(function() {
            alert('Failed to copy report ID to clipboard.');
        });
        return;
    }

    var input = document.createElement('input');
    input.value = id;
    document.body.appendChild(input);
    input.select();
    try {
        document.execCommand('copy');
        addLog('Copied API scan report ID: ' + id, 'info');
    } catch (e) {
        alert('Failed to copy report ID to clipboard.');
    }
    document.body.removeChild(input);
}

function clearAdminApiScanView() {
    latestAdminAPIScanReport = null;
    var summaryEl = document.getElementById('adminApiScanSummary');
    var structuredEl = document.getElementById('adminApiScanStructured');
    var output = document.getElementById('adminApiScanReportOutput');
    var downloadBtn = document.getElementById('adminDownloadApiScanBtn');

    if (summaryEl) summaryEl.innerHTML = '<span class="badge">No report selected</span>';
    if (structuredEl) structuredEl.innerHTML = '<div class="empty-state">Run a scan or open a report to view details.</div>';
    if (output) output.textContent = 'Run a scan to generate a report.';
    if (downloadBtn) downloadBtn.disabled = true;
}

function downloadAdminApiScanReportObject(report) {
    var payload = JSON.stringify(report, null, 2);
    var blob = new Blob([payload], { type: 'application/json;charset=utf-8' });
    var fileName = 'api-scan-report-' + String(report.id || 'latest') + '.json';
    var url = URL.createObjectURL(blob);
    var anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = fileName;
    document.body.appendChild(anchor);
    anchor.click();
    document.body.removeChild(anchor);
    URL.revokeObjectURL(url);
}

// ===================================================================
// Modals & Logs
// ===================================================================
function showSpinner(title) {
    var modal = document.getElementById('responseModal');
    var body = document.getElementById('modalBody');
    document.getElementById('modalTitle').textContent = title;
    body.innerHTML = '<div class="spinner"></div><p class="loading">Loading...</p>';
    modal.style.display = 'flex';
}

function showResponse(title, status, data, endpoint) {
    var modal = document.getElementById('responseModal');
    var body = document.getElementById('modalBody');
    document.getElementById('modalTitle').textContent = title;

    var statusClass = 'success-message';
    if (typeof status === 'number' && status >= 400) {
        statusClass = 'error-message';
    } else if (typeof status === 'string' && status === 'ERROR') {
        statusClass = 'error-message';
    }

    var html = '<div class="' + statusClass + '"><strong>Status:</strong> ' + status + '</div>' +
        '<div style="margin-top:15px"><strong>Endpoint:</strong> <code>' + escapeHtml(String(endpoint)) + '</code></div>' +
        '<div style="margin-top:15px"><strong>Response:</strong></div>' +
        '<pre>' + escapeHtml(JSON.stringify(data, null, 2)) + '</pre>';

    body.innerHTML = html;
    modal.style.display = 'flex';
}

function closeResponseModal() {
    document.getElementById('responseModal').style.display = 'none';
}

function clearLogs() {
    document.getElementById('logsViewer').innerHTML = '';
}

function addLog(message, type) {
    var logViewer = document.getElementById('logsViewer');
    if (!logViewer) return;
    var timestamp = new Date().toLocaleTimeString();
    var entry = document.createElement('div');
    entry.className = 'log-entry ' + type;
    entry.innerHTML = '<span class="log-time">[' + timestamp + ']</span>' +
        '<span class="log-level">' + type.toUpperCase() + '</span>' +
        '<span class="log-message">' + escapeHtml(message) + '</span>';
    logViewer.insertBefore(entry, logViewer.firstChild);
}

// ===================================================================
// GraphQL Studio + Control Plane (Admin)
// ===================================================================
function adminApiCall(method, path, body) {
    var options = {
        method: method,
        headers: getAuthHeaders()
    };
    if (body !== undefined && body !== null) {
        options.body = JSON.stringify(body);
    }

    return fetch(BACKEND_URL + path, options).then(function(response) {
        return response.text().then(function(text) {
            var parsed;
            try {
                parsed = text ? JSON.parse(text) : {};
            } catch (e) {
                parsed = { raw: text };
            }
            if (!response.ok) {
                var msg = (parsed && (parsed.error || parsed.message)) || ('Request failed with status ' + response.status);
                var err = new Error(msg);
                err.status = response.status;
                err.response = parsed;
                throw err;
            }
            return { status: response.status, data: parsed };
        });
    });
}

function parseJSONInput(elementId, fallback) {
    var el = document.getElementById(elementId);
    if (!el) return fallback;
    var raw = (el.value || '').trim();
    if (!raw) return fallback;
    return JSON.parse(raw);
}

function setAdminControlPlaneOutput(title, payload) {
    var el = document.getElementById('adminControlPlaneOutput');
    if (!el) return;
    el.textContent = title + '\n\n' + JSON.stringify(payload, null, 2);
}

function getAdminControlPlaneInput() {
    var namespace = (document.getElementById('adminCpNamespace').value || 'default').trim() || 'default';
    var kind = (document.getElementById('adminCpKind').value || 'apis').trim().toLowerCase();
    var name = (document.getElementById('adminCpName').value || '').trim();
    return { namespace: namespace, kind: kind, name: name };
}

function canControlPlaneWrite() {
    var role = (localStorage.getItem('userRole') || 'user').toLowerCase();
    return role === 'admin' || role === 'system-manager';
}

function ensureControlPlaneWrite(actionLabel) {
    if (canControlPlaneWrite()) return true;
    alert('RBAC: only admin or system-manager can ' + actionLabel + '.');
    return false;
}

function runAdminGraphQLQuery() {
    var queryEl = document.getElementById('adminGraphQLQuery');
    var opEl = document.getElementById('adminGraphQLOperation');
    var resultEl = document.getElementById('adminGraphQLResult');
    if (!queryEl || !resultEl) return;

    var query = (queryEl.value || '').trim();
    if (!query) {
        alert('GraphQL query is required.');
        return;
    }

    if (/^\s*mutation\b/i.test(query)) {
        var mutationOk = confirm('This GraphQL request appears to be a mutation and may change data. Continue?');
        if (!mutationOk) {
            return;
        }
        var mutationAck = prompt('Type RUN MUTATION to confirm execution');
        if (mutationAck === null || String(mutationAck).trim().toUpperCase() !== 'RUN MUTATION') {
            alert('Execution cancelled: confirmation text did not match RUN MUTATION.');
            return;
        }
    }

    var variables;
    try {
        variables = parseJSONInput('adminGraphQLVariables', {});
    } catch (e) {
        alert('Invalid JSON in GraphQL variables: ' + e.message);
        return;
    }

    resultEl.textContent = 'Running query...';
    adminApiCall('POST', '/api/graphql', {
        query: query,
        variables: variables,
        operationName: (opEl && opEl.value ? opEl.value.trim() : '') || undefined
    }).then(function(result) {
        resultEl.textContent = JSON.stringify(result.data, null, 2);
        addLog('GraphQL query executed', 'info');
    }).catch(function(err) {
        resultEl.textContent = JSON.stringify({
            error: err.message,
            status: err.status || 'n/a',
            details: err.response || {}
        }, null, 2);
        addLog('GraphQL query failed: ' + err.message, 'error');
    });
}

function loadAdminGraphQLSchemaInfo() {
    var resultEl = document.getElementById('adminGraphQLResult');
    if (!resultEl) return;
    adminApiCall('GET', '/api/graphql/schema').then(function(result) {
        resultEl.textContent = JSON.stringify(result.data, null, 2);
    }).catch(function(err) {
        resultEl.textContent = JSON.stringify({
            error: err.message,
            details: err.response || {}
        }, null, 2);
    });
}

function adminApplyResource() {
    if (!ensureControlPlaneWrite('apply resources')) return;
    var meta = getAdminControlPlaneInput();
    var body;
    try {
        body = parseJSONInput('adminCpBody', {});
    } catch (e) {
        alert('Invalid JSON in resource body: ' + e.message);
        return;
    }

    if (!body.metadata) body.metadata = {};
    if (!body.metadata.name && meta.name) {
        body.metadata.name = meta.name;
    }

    adminApiCall('POST', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind), body)
        .then(function(result) {
            setAdminControlPlaneOutput('Resource applied', result.data);
            addLog('Applied resource ' + (body.metadata.name || ''), 'info');
        })
        .catch(function(err) {
            setAdminControlPlaneOutput('Apply failed', { error: err.message, details: err.response || {} });
        });
}

function adminListResources() {
    var meta = getAdminControlPlaneInput();
    adminApiCall('GET', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind))
        .then(function(result) {
            setAdminControlPlaneOutput('Resource list', result.data);
        })
        .catch(function(err) {
            setAdminControlPlaneOutput('List failed', { error: err.message, details: err.response || {} });
        });
}

function adminGetResource() {
    var meta = getAdminControlPlaneInput();
    if (!meta.name) { alert('Resource name is required.'); return; }
    adminApiCall('GET', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind) + '/' + encodeURIComponent(meta.name))
        .then(function(result) {
            setAdminControlPlaneOutput('Resource detail', result.data);
        })
        .catch(function(err) {
            setAdminControlPlaneOutput('Get failed', { error: err.message, details: err.response || {} });
        });
}

function adminGetResourceStatus() {
    var meta = getAdminControlPlaneInput();
    if (!meta.name) { alert('Resource name is required.'); return; }
    adminApiCall('GET', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind) + '/' + encodeURIComponent(meta.name) + '/status')
        .then(function(result) {
            setAdminControlPlaneOutput('Resource status', result.data);
        })
        .catch(function(err) {
            setAdminControlPlaneOutput('Status failed', { error: err.message, details: err.response || {} });
        });
}

function adminGetResourceEvents() {
    var meta = getAdminControlPlaneInput();
    if (!meta.name) { alert('Resource name is required.'); return; }
    adminApiCall('GET', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind) + '/' + encodeURIComponent(meta.name) + '/events')
        .then(function(result) {
            setAdminControlPlaneOutput('Resource events', result.data);
        })
        .catch(function(err) {
            setAdminControlPlaneOutput('Events failed', { error: err.message, details: err.response || {} });
        });
}

function adminDeleteResource() {
    if (!ensureControlPlaneWrite('delete resources')) return;
    var meta = getAdminControlPlaneInput();
    if (!meta.name) { alert('Resource name is required.'); return; }
    adminApiCall('DELETE', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind) + '/' + encodeURIComponent(meta.name))
        .then(function(result) {
            setAdminControlPlaneOutput('Resource deleted', result.data);
            addLog('Deleted resource ' + meta.name, 'warn');
        })
        .catch(function(err) {
            setAdminControlPlaneOutput('Delete failed', { error: err.message, details: err.response || {} });
        });
}

function adminRunWorkflow() {
    if (!ensureControlPlaneWrite('run workflows')) return;
    var name = (document.getElementById('adminWorkflowName').value || '').trim();
    if (!name) { alert('Workflow name is required.'); return; }
    adminApiCall('POST', '/api/v1/workflows/' + encodeURIComponent(name) + '/run', {})
        .then(function(result) {
            setAdminControlPlaneOutput('Workflow run requested', result.data);
        })
        .catch(function(err) {
            setAdminControlPlaneOutput('Workflow run failed', { error: err.message, details: err.response || {} });
        });
}

function adminListDatasources() {
    adminApiCall('GET', '/api/v1/datasources').then(function(result) {
        setAdminControlPlaneOutput('Datasources', result.data);
    }).catch(function(err) {
        setAdminControlPlaneOutput('List datasources failed', { error: err.message, details: err.response || {} });
    });
}

function adminGetDatasource() {
    var name = (document.getElementById('adminDatasourceName').value || '').trim();
    if (!name) { alert('Datasource name is required.'); return; }
    adminApiCall('GET', '/api/v1/datasources/' + encodeURIComponent(name)).then(function(result) {
        setAdminControlPlaneOutput('Datasource detail', result.data);
    }).catch(function(err) {
        setAdminControlPlaneOutput('Get datasource failed', { error: err.message, details: err.response || {} });
    });
}

function adminTestDatasource() {
    if (!ensureControlPlaneWrite('test datasources')) return;
    var name = (document.getElementById('adminDatasourceName').value || '').trim();
    if (!name) { alert('Datasource name is required.'); return; }
    adminApiCall('POST', '/api/v1/datasources/' + encodeURIComponent(name) + '/test', {}).then(function(result) {
        setAdminControlPlaneOutput('Datasource test result', result.data);
    }).catch(function(err) {
        setAdminControlPlaneOutput('Test datasource failed', { error: err.message, details: err.response || {} });
    });
}

function adminListJobs() {
    adminApiCall('GET', '/api/v1/jobs').then(function(result) {
        setAdminControlPlaneOutput('Jobs', result.data);
    }).catch(function(err) {
        setAdminControlPlaneOutput('List jobs failed', { error: err.message, details: err.response || {} });
    });
}

function adminGetJob() {
    var id = (document.getElementById('adminJobId').value || '').trim();
    if (!id) { alert('Job ID/name is required.'); return; }
    adminApiCall('GET', '/api/v1/jobs/' + encodeURIComponent(id)).then(function(result) {
        setAdminControlPlaneOutput('Job detail', result.data);
    }).catch(function(err) {
        setAdminControlPlaneOutput('Get job failed', { error: err.message, details: err.response || {} });
    });
}

function adminRunJob() {
    if (!ensureControlPlaneWrite('run jobs')) return;
    var id = (document.getElementById('adminJobId').value || '').trim();
    if (!id) { alert('Job ID/name is required.'); return; }
    adminApiCall('POST', '/api/v1/jobs/' + encodeURIComponent(id) + '/run', {}).then(function(result) {
        setAdminControlPlaneOutput('Job run requested', result.data);
    }).catch(function(err) {
        setAdminControlPlaneOutput('Run job failed', { error: err.message, details: err.response || {} });
    });
}

function adminGetJobLogs() {
    var id = (document.getElementById('adminJobId').value || '').trim();
    if (!id) { alert('Job ID/name is required.'); return; }
    adminApiCall('GET', '/api/v1/jobs/' + encodeURIComponent(id) + '/logs').then(function(result) {
        setAdminControlPlaneOutput('Job logs', result.data);
    }).catch(function(err) {
        setAdminControlPlaneOutput('Get job logs failed', { error: err.message, details: err.response || {} });
    });
}

function adminCancelJob() {
    if (!ensureControlPlaneWrite('cancel jobs')) return;
    var id = (document.getElementById('adminJobId').value || '').trim();
    if (!id) { alert('Job ID/name is required.'); return; }
    adminApiCall('POST', '/api/v1/jobs/' + encodeURIComponent(id) + '/cancel', {}).then(function(result) {
        setAdminControlPlaneOutput('Job cancel requested', result.data);
    }).catch(function(err) {
        setAdminControlPlaneOutput('Cancel job failed', { error: err.message, details: err.response || {} });
    });
}

function refreshAdminControlPlaneData() {
    var meta = getAdminControlPlaneInput();
    Promise.all([
        adminApiCall('GET', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind)),
        adminApiCall('GET', '/api/v1/datasources'),
        adminApiCall('GET', '/api/v1/jobs')
    ]).then(function(results) {
        setAdminControlPlaneOutput('Control plane snapshot', {
            resources: results[0].data,
            datasources: results[1].data,
            jobs: results[2].data
        });
    }).catch(function(err) {
        setAdminControlPlaneOutput('Refresh failed', { error: err.message, details: err.response || {} });
    });
}

function getAdminCertName() {
    var el = document.getElementById('adminCertName');
    if (!el) return '';
    return (el.value || '').trim();
}

function setAdminCertificateOutput(title, payload) {
    var el = document.getElementById('adminCertificateOutput');
    if (!el) return;
    el.textContent = title + '\n\n' + JSON.stringify(payload, null, 2);
}

function loadAdminCertificatePanel() {
    var el = document.getElementById('adminCertificateOutput');
    if (el && !el.textContent.trim()) {
        el.textContent = 'Kubernetes certificate status will appear here.';
    }
}

function initAdminCertificateActions() {
    var checkBtn = document.getElementById('adminCertCheckBtn');
    var dryRunBtn = document.getElementById('adminCertDryRunBtn');
    var renewBtn = document.getElementById('adminCertRenewBtn');

    if (checkBtn && !checkBtn.dataset.bound) {
        checkBtn.addEventListener('click', function() { adminCheckCertificateExpiry(); });
        checkBtn.dataset.bound = '1';
    }
    if (dryRunBtn && !dryRunBtn.dataset.bound) {
        dryRunBtn.addEventListener('click', function() { adminRenewCertificate(true); });
        dryRunBtn.dataset.bound = '1';
    }
    if (renewBtn && !renewBtn.dataset.bound) {
        renewBtn.addEventListener('click', function() { adminRenewCertificate(false); });
        renewBtn.dataset.bound = '1';
    }
}

function adminCheckCertificateExpiry() {
    if (!ensureControlPlaneWrite('view certificate status')) return;

    var cert = getAdminCertName();
    var path = '/api/admin/certificates/status';
    if (cert) {
        path += '?cert=' + encodeURIComponent(cert);
    }

    adminApiCall('GET', path).then(function(result) {
        setAdminCertificateOutput('Certificate status', result.data);
        addLog('Checked certificate expiry' + (cert ? ' for ' + cert : ''), 'info');
    }).catch(function(err) {
        setAdminCertificateOutput('Certificate status failed', { error: err.message, details: err.response || {} });
    });
}

function adminRenewCertificate(dryRun) {
    if (!ensureControlPlaneWrite('renew certificates')) return;

    var cert = getAdminCertName();
    var body = { dry_run: !!dryRun };
    if (cert) body.cert = cert;

    adminApiCall('POST', '/api/admin/certificates/renew', body).then(function(result) {
        setAdminCertificateOutput(dryRun ? 'Certificate renew dry run' : 'Certificate renew result', result.data);
        addLog((dryRun ? 'Prepared' : 'Triggered') + ' certificate renewal' + (cert ? ' for ' + cert : ''), dryRun ? 'info' : 'warn');
    }).catch(function(err) {
        setAdminCertificateOutput('Certificate renew failed', { error: err.message, details: err.response || {} });
    });
}

window.adminCheckCertificateExpiry = adminCheckCertificateExpiry;
window.adminRenewCertificate = adminRenewCertificate;

// ===================================================================
// File Scanner (SafeGate Pipeline)
// ===================================================================
function setupScanDropZone() {
    var dz = document.getElementById('scanDropZone');
    if (!dz) return;
    dz.addEventListener('dragover', function(e) { e.preventDefault(); dz.classList.add('drag-over'); });
    dz.addEventListener('dragleave', function() { dz.classList.remove('drag-over'); });
    dz.addEventListener('drop', function(e) {
        e.preventDefault();
        dz.classList.remove('drag-over');
        if (e.dataTransfer.files.length > 0) {
            var fi = document.getElementById('scanFileInput');
            fi.files = e.dataTransfer.files;
            handleFileScan();
        }
    });
}

function handleFileScan() {
    var fileInput = document.getElementById('scanFileInput');
    if (!fileInput.files || !fileInput.files[0]) return;
    var file = fileInput.files[0];

    var formData = new FormData();
    formData.append('file', file);

    var token = localStorage.getItem('authToken');
    var headers = {};
    if (token) headers['Authorization'] = 'Bearer ' + token;

    document.getElementById('scanResult').style.display = 'block';
    document.getElementById('scanFilenameDisplay').textContent = 'Scanning ' + file.name + '...';
    document.getElementById('scanBadges').innerHTML = '<span class="badge">Scanning...</span>';
    document.getElementById('scanFindings').innerHTML = '<div class="loading">Running SafeGate pipeline...</div>';

    fetch(BACKEND_URL + '/api/v1/builder/scanner/scan', {
        method: 'POST', headers: headers, body: formData
    }).then(function(r) { return r.json(); }).then(function(d) {
        if (d.status === 'success') {
            showScanResult(d.scan);
            if (d.scan && d.scan.id) {
                selectedScanReportId = d.scan.id;
            }
            addLog('Scanned file: ' + d.scan.filename + ' — ' + (d.safe ? 'SAFE' : 'THREATS FOUND'), d.safe ? 'info' : 'error');
            loadScanHistory();
        } else {
            document.getElementById('scanFindings').innerHTML = '<div class="error-state">' + escapeHtml(d.error || 'Scan failed') + '</div>';
        }
    }).catch(function(err) {
        document.getElementById('scanFindings').innerHTML = '<div class="error-state">Scan error: ' + escapeHtml(err.message) + '</div>';
    });
}

function normalizeScanFindings(scan) {
    if (!scan || !Array.isArray(scan.findings)) return [];
    return scan.findings;
}

function getFindingSeverityWeight(severity) {
    var sev = String(severity || '').toLowerCase();
    if (sev === 'critical') return 5;
    if (sev === 'high') return 4;
    if (sev === 'medium') return 3;
    if (sev === 'low') return 2;
    if (sev === 'info') return 1;
    return 0;
}

function getHighestFindingSeverity(findings) {
    var highest = 'unknown';
    var highestWeight = -1;
    (findings || []).forEach(function(f) {
        var candidate = String((f && f.severity) || 'unknown').toLowerCase();
        var weight = getFindingSeverityWeight(candidate);
        if (weight > highestWeight) {
            highestWeight = weight;
            highest = candidate;
        }
    });
    return highest;
}

function renderFindingSeverityBadge(severity) {
    var sev = String(severity || 'unknown').toLowerCase();
    var cls = 'status-active';
    if (sev === 'critical' || sev === 'high') cls = 'status-inactive';
    else if (sev === 'medium') cls = 'status-draft';
    return '<span class="status-badge ' + cls + '">' + escapeHtml(sev.toUpperCase()) + '</span>';
}

function showScanResult(scan) {
    if (!scan) return;
    var findings = normalizeScanFindings(scan);
    var resultBox = document.getElementById('scanResult');
    if (resultBox) resultBox.style.display = 'block';

    if (scan.id) {
        selectedScanReportId = scan.id;
    }

    document.getElementById('scanFilenameDisplay').textContent = scan.filename;
    var safeBadge = scan.safe
        ? '<span class="badge badge-success">✅ SAFE</span>'
        : '<span class="badge badge-danger">⚠️ THREATS FOUND</span>';
    document.getElementById('scanBadges').innerHTML =
        safeBadge + ' ' +
        '<span class="badge">' + formatFileSize(scan.file_size) + '</span> ' +
        '<span class="badge">' + findings.length + ' finding(s)</span> ' +
        (scan.id ? '<span class="badge">Report: ' + escapeHtml(scan.id) + '</span> ' : '') +
        (scan.scanned_at ? '<span class="badge">' + escapeHtml(new Date(scan.scanned_at).toLocaleString()) + '</span>' : '');

    var html = '';
    if (findings.length === 0) {
        html = '<div class="empty-state" style="padding:10px">No security issues found.</div>';
    } else {
        html = '<table class="admin-table compact"><thead><tr><th>Severity</th><th>Scanner</th><th>Description</th><th>Details</th></tr></thead><tbody>';
        findings.forEach(function(f) {
            var sevClass = f.severity === 'critical' || f.severity === 'high' ? 'status-inactive' :
                           f.severity === 'medium' ? 'status-draft' : 'status-active';
            html += '<tr><td><span class="status-badge ' + sevClass + '">' + f.severity.toUpperCase() + '</span></td>' +
                '<td>' + escapeHtml(f.scanner) + '</td>' +
                '<td>' + escapeHtml(f.description) + '</td>' +
                '<td><small>' + escapeHtml(f.details || '') + '</small></td></tr>';
        });
        html += '</tbody></table>';
    }
    document.getElementById('scanFindings').innerHTML = html;
}

function viewScanReport(scanID) {
    var id = String(scanID || '').trim();
    if (!id) return;

    var cached = scanHistoryById[id];
    if (cached) {
        showScanResult(cached);
        return;
    }

    fetchJSON('/api/v1/builder/scanner/scans/' + encodeURIComponent(id)).then(function(d) {
        if (d.status !== 'success' || !d.scan) {
            alert(d.error || 'Scan report not found');
            return;
        }
        scanHistoryById[id] = d.scan;
        showScanResult(d.scan);
    }).catch(function(err) {
        alert('Failed to load scan report: ' + (err && err.message ? err.message : 'unknown error'));
    });
}

function renderSuspiciousScanList(list) {
    var el = document.getElementById('scanSuspiciousFiles');
    if (!el) return;

    var suspicious = (list || []).filter(function(s) { return !s.safe; });
    if (suspicious.length === 0) {
        el.innerHTML = '<div class="empty-state">No suspicious files found yet.</div>';
        return;
    }

    var html = '<table class="admin-table compact"><thead><tr><th>File</th><th>Highest Severity</th><th>Findings</th><th>Date</th><th>Action</th></tr></thead><tbody>';
    suspicious.forEach(function(s) {
        var findings = normalizeScanFindings(s);
        var severity = getHighestFindingSeverity(findings);
        var safeID = String(s.id || '').replace(/'/g, "\\'");
        html += '<tr>' +
            '<td>' + escapeHtml(s.filename || '-') + '</td>' +
            '<td>' + renderFindingSeverityBadge(severity) + '</td>' +
            '<td>' + findings.length + '</td>' +
            '<td>' + (s.scanned_at ? new Date(s.scanned_at).toLocaleString() : '-') + '</td>' +
            '<td><button class="btn-sm btn-secondary" onclick="viewScanReport(\'' + safeID + '\')">View Report</button></td>' +
            '</tr>';
    });
    html += '</tbody></table>';
    el.innerHTML = html;
}

function formatFileSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}

function loadScannerHealth() {
    fetchJSON('/api/v1/builder/scanner/health').then(function(d) {
        var el = document.getElementById('scannerHealth');
        if (!el) return;
        var html = '<div class="col-grid">';
        (d.scanners || []).forEach(function(name) {
            html += '<div class="col-chip col-type-string"><strong>' + escapeHtml(name) + '</strong><br><small>Active</small></div>';
        });
        html += '</div><p style="margin-top:8px;color:#888">Total scans performed: ' + (d.total_scans || 0) + '</p>';
        el.innerHTML = html;
    }).catch(function() {
        var el = document.getElementById('scannerHealth');
        if (el) el.innerHTML = '<div class="empty-state">Could not load scanner status</div>';
    });
}

function loadScanHistory() {
    fetchJSON('/api/v1/builder/scanner/scans').then(function(d) {
        var list = d.scans || [];
        scanHistoryById = {};
        list.forEach(function(s) {
            if (s && s.id) scanHistoryById[s.id] = s;
        });

        var el = document.getElementById('scanHistory');
        if (!el) return;
        if (list.length === 0) {
            el.innerHTML = '<div class="empty-state">No scans yet</div>';
            renderSuspiciousScanList([]);
            return;
        }
        var html = '<table class="admin-table compact"><thead><tr><th>File</th><th>Size</th><th>Result</th><th>Findings</th><th>Date</th><th>Action</th></tr></thead><tbody>';
        list.forEach(function(s) {
            var findings = normalizeScanFindings(s);
            var resultBadge = s.safe
                ? '<span class="status-badge status-active">Safe</span>'
                : '<span class="status-badge status-inactive">Unsafe</span>';
            var safeID = String(s.id || '').replace(/'/g, "\\'");
            html += '<tr><td>' + escapeHtml(s.filename) + '</td>' +
                '<td>' + formatFileSize(s.file_size) + '</td>' +
                '<td>' + resultBadge + '</td>' +
                '<td>' + findings.length + '</td>' +
                '<td>' + new Date(s.scanned_at).toLocaleString() + '</td>' +
                '<td><button class="btn-sm btn-secondary" onclick="viewScanReport(\'' + safeID + '\')">View Report</button></td></tr>';
        });
        html += '</tbody></table>';
        el.innerHTML = html;

        renderSuspiciousScanList(list);

        if (selectedScanReportId && scanHistoryById[selectedScanReportId]) {
            showScanResult(scanHistoryById[selectedScanReportId]);
        }
    }).catch(function() {});
}

// Close modals on outside click
window.onclick = function(event) {
    var responseModal = document.getElementById('responseModal');
    if (event.target === responseModal) responseModal.style.display = 'none';
    var createModal = document.getElementById('createAPIModal');
    if (event.target === createModal) createModal.style.display = 'none';
    var editModal = document.getElementById('editAPIModal');
    if (event.target === editModal) editModal.style.display = 'none';
    var createGraphQLModal = document.getElementById('createGraphQLAPIModal');
    if (event.target === createGraphQLModal) closeCreateGraphQLAPIModal();
};

// ===================================================================
// Role-Based Access Control
// ===================================================================
function canModify() {
    var role = localStorage.getItem('userRole') || 'user';
    return role === 'admin' || role === 'manager' || role === 'system-manager';
}

function applyRoleRestrictions() {
    var role = localStorage.getItem('userRole') || 'user';
    if (role === 'admin' || role === 'manager' || role === 'system-manager') return; // full access

    // Normal users: hide create/delete/modify controls
    var hideSelectors = [
        '.btn-create-api',         // Create API button
        '#createAPIBtn',           // Create API button by id
        '.btn-del',                // Delete buttons in API table
        '.btn-danger',             // Danger buttons
    ];

    // Hide "Create API" button 
    var createBtns = document.querySelectorAll('button');
    createBtns.forEach(function(btn) {
        var text = btn.textContent.trim().toLowerCase();
        if (text.includes('create api') || text.includes('+ create') || text.includes('delete')) {
            btn.style.display = 'none';
        }
    });

    // Add a read-only banner
    var container = document.querySelector('.admin-container') || document.querySelector('.manager-header');
    if (container) {
        var banner = document.createElement('div');
        banner.style.cssText = 'background:rgba(59,130,246,0.15);color:#3b82f6;padding:10px 20px;border-radius:8px;margin-bottom:16px;text-align:center;font-weight:600;';
        banner.textContent = '👁️ View-Only Mode — Your role (' + role + ') allows viewing and calling APIs only';
        container.insertBefore(banner, container.firstChild);
    }
}

function confirmDeleteAPI(kindLabel, apiName) {
    var label = kindLabel || 'API';
    var name = String(apiName || '').trim() || 'unnamed-api';
    if (!confirm('Delete ' + label + ' "' + name + '"? This action cannot be undone.')) {
        return false;
    }

    var ack = prompt('Type DELETE to confirm deleting "' + name + '"');
    if (ack === null) {
        return false;
    }
    if (String(ack).trim().toUpperCase() !== 'DELETE') {
        alert('Delete cancelled: confirmation text did not match DELETE.');
        return false;
    }
    return true;
}

// Override delete/create functions for role checks
function deleteCustomAPI(id) {
    if (!canModify()) { alert('You do not have permission to delete APIs. Contact an admin or manager.'); return; }
    var api = customAPIById[id] || {};
    var apiName = api.name || id;
    if (!confirmDeleteAPI('REST API', apiName)) return;
    deleteJSON('/api/v1/builder/apis/' + id).then(function(d) {
        if (d.status === 'success' || d.status === 'ok') { addLog('Deleted API: ' + id, 'warn'); loadBuilderSummary(); loadCustomAPIs(); loadRESTEndpointCatalog(); }
        else { alert(d.error || 'Failed to delete API'); }
    });
}

function deleteDashboard(dashId) {
    if (!canModify()) { alert('You do not have permission to delete dashboards. Contact an admin or manager.'); return; }
    if (!confirm('Delete this dashboard? This cannot be undone.')) return;
    deleteJSON('/api/v1/builder/dashboards/' + dashId).then(function(d) {
        if (d.status === 'success') { addLog('Deleted dashboard: ' + dashId, 'warn'); loadCSVHistory(); }
        else { alert(d.error || 'Failed to delete dashboard'); }
    });
}

function openCreateAPIModal() {
    if (!canModify()) { alert('You do not have permission to create APIs. Contact an admin, manager, or system-manager.'); return; }
    bindRESTEndpointValidation();
    loadRESTEndpointCatalog();
    setRESTEndpointDuplicateWarning('apiPathDuplicateWarning', '');
    var pathInput = document.getElementById('apiPathInput');
    if (pathInput) pathInput.setCustomValidity('');
    loadBuilderDataSources();
    document.getElementById('createAPIModal').style.display = 'flex';
}
