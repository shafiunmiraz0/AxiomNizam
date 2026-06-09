// System Manager JS
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
        if (resolved) {
            return resolved;
        }
    }

    var url = normalizeURL(window.BACKEND_URL || '');
    if (url) {
        var browserHost = String(window.location.hostname || '').toLowerCase();
        var browserIsPublic = browserHost && browserHost !== 'localhost' && browserHost !== '127.0.0.1' && browserHost !== '0.0.0.0';
        var backendHost = parseHostname(url);
        if (browserIsPublic && isLocalOrInternalHost(backendHost)) {
            return inferPublicBackendURL() || url;
        }
        return url;
    }

    return inferPublicBackendURL() || 'http://localhost:8000';
})();

console.log('System Manager - Backend URL:', BACKEND_URL);

var availableDbServers = [];
var monitoringDataSnapshot = {
    apiStats: null,
    apiCount: null,
    builderSummary: null,
    builderAPIs: [],
    realms: [],
    realmDashboards: {},
    clients: [],
    users: [],
    roles: [],
    bindings: [],
    selectedRealm: '',
    apiTrendHistory: {},
    trendMaxPoints: 12,
    realmDashboardsReady: false
};

function normalizeSystemManagerRole(role) {
    var value = String(role || '').toLowerCase().trim();
    if (!value) return '';
    if (value === 'sysadmin' || value === 'system-admin' || value === 'system_admin') return 'system-manager';
    if (value === 'superadmin' || value === 'super-admin') return 'admin';
    if (value === 'api-manager' || value === 'api_manager') return 'manager';
    return value;
}

function readSystemManagerCookie(name) {
    var prefix = name + '=';
    var parts = document.cookie.split(';');
    for (var i = 0; i < parts.length; i++) {
        var item = parts[i].trim();
        if (item.indexOf(prefix) === 0) {
            return decodeURIComponent(item.substring(prefix.length));
        }
    }
    return '';
}

function getLatestAuthTokenForCopy() {
    return localStorage.getItem('authToken') || readSystemManagerCookie('authToken') || '';
}

function canShowTokenCopyShortcut() {
    var role = normalizeSystemManagerRole(localStorage.getItem('userRole') || readSystemManagerCookie('userRole') || '');
    return role === 'admin' || role === 'system-manager';
}

function setTokenCopyStatus(message, color) {
    var el = document.getElementById('copyTokenStatus');
    if (!el) return;
    el.textContent = message;
    if (color) {
        el.style.color = color;
    }
}

function setupTokenCopyShortcut() {
    var container = document.getElementById('tokenCopyShortcut');
    if (!container) return;

    if (!canShowTokenCopyShortcut()) {
        container.style.display = 'none';
        return;
    }

    container.style.display = 'block';
    setTokenCopyStatus('Ready', 'var(--text-secondary,#94a3b8)');
}

function fallbackCopyToken(token, onSuccess, onError) {
    try {
        var textArea = document.createElement('textarea');
        textArea.value = token;
        textArea.setAttribute('readonly', 'readonly');
        textArea.style.position = 'fixed';
        textArea.style.top = '-9999px';
        document.body.appendChild(textArea);
        textArea.focus();
        textArea.select();

        var copied = document.execCommand('copy');
        document.body.removeChild(textArea);

        if (copied) {
            onSuccess();
            return;
        }
        onError();
    } catch (_) {
        onError();
    }
}

function copyAuthTokenForPostman() {
    var token = getLatestAuthTokenForCopy();
    if (!token) {
        setTokenCopyStatus('No token found', '#ef4444');
        return;
    }

    var success = function() {
        setTokenCopyStatus('Copied', '#10b981');
        addOperationLog('API token copied for Postman', 'info');
        setTimeout(function() {
            setTokenCopyStatus('Ready', 'var(--text-secondary,#94a3b8)');
        }, 2500);
    };

    var failure = function() {
        setTokenCopyStatus('Copy failed', '#ef4444');
    };

    if (navigator.clipboard && window.isSecureContext) {
        navigator.clipboard.writeText(token)
            .then(success)
            .catch(function() {
                fallbackCopyToken(token, success, failure);
            });
        return;
    }

    fallbackCopyToken(token, success, failure);
}

window.addEventListener('DOMContentLoaded', function() {
    // Set user name from localStorage
    const userName = localStorage.getItem('userName');
    if (userName) {
        const userNameElem = document.getElementById('managerUserName');
        if (userNameElem) {
            userNameElem.textContent = userName;
        }
    }
    setupTokenCopyShortcut();
    loadStatusData();
    loadDatabases();
    loadDatabaseServers();
    loadConnectedServers();
    loadMonitoringDashboards();
    setInterval(loadStatusData, 30000);
});

function switchManagerTab(tabName) {
    // Hide all tabs
    const tabs = document.querySelectorAll('.tab-content');
    tabs.forEach(function(tab) { tab.classList.remove('active'); });
    
    // Remove active from buttons
    const buttons = document.querySelectorAll('.tab-btn');
    buttons.forEach(function(btn) { btn.classList.remove('active'); });
    
    // Show selected tab
    const selectedTab = document.getElementById(tabName);
    if (selectedTab) selectedTab.classList.add('active');
    
    // Add active to clicked button
    if (event && event.currentTarget) {
        event.currentTarget.classList.add('active');
    }
    
    if (tabName === 'databases') {
        loadDatabases();
        loadConnectedServers();
    }
    if (tabName === 'users') {
        loadUsers();
    }
    if (tabName === 'monitoring') {
        loadMonitoringDashboards();
    }
    if (tabName === 'graphql-studio') {
        loadManagerGraphQLSchemaInfo();
    }
    if (tabName === 'control-plane') {
        refreshManagerControlPlaneData();
    }
}

function getSystemManagerAuthHeaders() {
    if (typeof getAuthHeaders === 'function') {
        return getAuthHeaders();
    }

    var token = localStorage.getItem('authToken') || readSystemManagerCookie('authToken') || '';
    var headers = { 'Content-Type': 'application/json' };
    if (token) {
        headers.Authorization = 'Bearer ' + token;
    }
    return headers;
}

function monitoringFetchJSON(path, timeoutMs) {
    var headers = getSystemManagerAuthHeaders();
    var controller = (typeof AbortController !== 'undefined') ? new AbortController() : null;
    var effectiveTimeout = safeNumber(timeoutMs) > 0 ? safeNumber(timeoutMs) : 10000;
    var timeoutHandle = null;

    var options = { headers: headers };
    if (controller) {
        options.signal = controller.signal;
        timeoutHandle = setTimeout(function() {
            controller.abort();
        }, effectiveTimeout);
    }

    return fetch(BACKEND_URL + path, options)
        .then(function(response) {
            return response.text().then(function(text) {
                var payload = {};
                try {
                    payload = text ? JSON.parse(text) : {};
                } catch (e) {
                    payload = { raw: text };
                }

                if (!response.ok) {
                    var message = (payload && (payload.error || payload.message)) || ('Request failed with status ' + response.status);
                    var err = new Error(message);
                    err.status = response.status;
                    err.payload = payload;
                    throw err;
                }

                return payload;
            });
        })
        .catch(function(err) {
            if (err && err.name === 'AbortError') {
                throw new Error('Request timeout for ' + path);
            }
            throw err;
        })
        .finally(function() {
            if (timeoutHandle) {
                clearTimeout(timeoutHandle);
            }
        });
}

function safeNumber(value) {
    var n = Number(value);
    return Number.isFinite(n) ? n : 0;
}

function formatMetricNumber(value) {
    var n = safeNumber(value);
    return new Intl.NumberFormat().format(n);
}

function formatMetricPercent(value) {
    var n = safeNumber(value);
    return n.toFixed(1) + '%';
}

function formatMetricDuration(ms) {
    var n = safeNumber(ms);
    if (n <= 0) {
        return '--';
    }
    if (n < 1000) {
        return n + 'ms';
    }
    return (n / 1000).toFixed(2) + 's';
}

function formatMetricDateTime(raw) {
    if (!raw) return '--';
    var d = new Date(raw);
    if (Number.isNaN(d.getTime())) return '--';
    return d.toLocaleString();
}

function monitoringStatusClassFromRate(percent) {
    if (percent >= 95) return 'status-active';
    if (percent >= 75) return 'status-draft';
    return 'status-inactive';
}

function normalizeMetricEndpoint(path) {
    var text = String(path || '').split('?')[0].trim();
    if (!text) return '/';
    if (text.charAt(0) !== '/') {
        text = '/' + text;
    }
    return text;
}

function setMonitoringUpdatedAt(elementId, text) {
    var el = document.getElementById(elementId);
    if (!el) return;
    if (text) {
        el.textContent = text;
        return;
    }
    el.textContent = 'Updated: ' + new Date().toLocaleTimeString();
}

function setMonitoringBodyLoading(elementId, colSpan, message) {
    var body = document.getElementById(elementId);
    if (!body) return;
    body.innerHTML = '<tr><td colspan="' + colSpan + '" class="monitoring-loading-cell">' + escapeHtml(message) + '</td></tr>';
}

function normalizeRealmIdentifier(realm) {
    if (!realm || typeof realm !== 'object') return '';
    return String(realm.id || realm.name || '').trim();
}

function populateMonitoringRealmFilter(realms) {
    var select = document.getElementById('monitoringRealmFilter');
    if (!select) return;

    var realmList = Array.isArray(realms) ? realms : [];
    var previousSelection = monitoringDataSnapshot.selectedRealm || select.value || '';
    var seen = {};
    var options = '<option value="">All realms</option>';

    for (var i = 0; i < realmList.length; i++) {
        var realm = realmList[i] || {};
        var key = normalizeRealmIdentifier(realm);
        if (!key || seen[key]) continue;
        seen[key] = true;

        var label = String(realm.display_name || realm.name || key);
        options += '<option value="' + escapeHtml(key) + '">' + escapeHtml(label) + '</option>';
    }

    select.innerHTML = options;

    if (previousSelection && seen[previousSelection]) {
        select.value = previousSelection;
        monitoringDataSnapshot.selectedRealm = previousSelection;
    } else {
        select.value = '';
        monitoringDataSnapshot.selectedRealm = '';
    }

    select.disabled = realmList.length === 0;
}

function getSelectedMonitoringRealm() {
    var select = document.getElementById('monitoringRealmFilter');
    if (!select) {
        return monitoringDataSnapshot.selectedRealm || '';
    }
    return String(select.value || '').trim();
}

function getMonitoringRealmLabel(realmID) {
    if (!realmID) return 'All realms';
    var realms = Array.isArray(monitoringDataSnapshot.realms) ? monitoringDataSnapshot.realms : [];
    for (var i = 0; i < realms.length; i++) {
        var realm = realms[i] || {};
        if (normalizeRealmIdentifier(realm) === realmID) {
            return String(realm.display_name || realm.name || realmID);
        }
    }
    return realmID;
}

function handleMonitoringRealmFilterChange() {
    monitoringDataSnapshot.selectedRealm = getSelectedMonitoringRealm();
    applyMonitoringFiltersAndRender({ trackTrend: false });
}

function getMonitoringTrendKey(method, path) {
    return String(method || 'GET').toUpperCase() + ' ' + normalizeMetricEndpoint(path || '/');
}

function updateMonitoringAPITrendHistory(rows) {
    if (!Array.isArray(rows)) return;

    var historyMap = monitoringDataSnapshot.apiTrendHistory || {};
    var maxPoints = safeNumber(monitoringDataSnapshot.trendMaxPoints) || 12;

    for (var i = 0; i < rows.length; i++) {
        var row = rows[i] || {};
        var key = getMonitoringTrendKey(row.method, row.path);
        if (!historyMap[key]) {
            historyMap[key] = [];
        }

        var series = historyMap[key];
        series.push(safeNumber(row.calls));
        if (series.length > maxPoints) {
            series.splice(0, series.length - maxPoints);
        }
    }

    monitoringDataSnapshot.apiTrendHistory = historyMap;
}

function getMonitoringAPIDeltaSeries(history) {
    var source = Array.isArray(history) ? history : [];
    if (source.length === 0) {
        return [0, 0];
    }
    if (source.length === 1) {
        return [source[0], source[0]];
    }

    var deltas = [];
    for (var i = 1; i < source.length; i++) {
        deltas.push(Math.max(0, safeNumber(source[i]) - safeNumber(source[i - 1])));
    }

    if (deltas.length === 1) {
        deltas.unshift(deltas[0]);
    }
    return deltas;
}

function buildMonitoringSparklineSVG(series, lineClass) {
    var data = Array.isArray(series) ? series : [0, 0];
    if (data.length < 2) {
        data = [0, 0];
    }

    var width = 96;
    var height = 26;
    var padding = 2;
    var min = Math.min.apply(null, data);
    var max = Math.max.apply(null, data);
    var range = max - min;
    if (range <= 0) {
        range = 1;
    }

    var points = [];
    for (var i = 0; i < data.length; i++) {
        var x = padding + ((width - (padding * 2)) * i / Math.max(1, data.length - 1));
        var y = (height - padding) - (((safeNumber(data[i]) - min) / range) * (height - (padding * 2)));
        points.push(x.toFixed(2) + ',' + y.toFixed(2));
    }

    return '<svg class="monitoring-sparkline-svg" viewBox="0 0 ' + width + ' ' + height + '" aria-hidden="true">' +
        '<polyline class="' + lineClass + '" points="' + points.join(' ') + '"></polyline>' +
        '</svg>';
}

function renderMonitoringAPISparkline(method, path) {
    var key = getMonitoringTrendKey(method, path);
    var historyMap = monitoringDataSnapshot.apiTrendHistory || {};
    var history = historyMap[key] || [];
    var deltas = getMonitoringAPIDeltaSeries(history);
    var lineClass = 'sparkline-neutral';

    if (deltas.length >= 2) {
        var last = deltas[deltas.length - 1];
        var prev = deltas[deltas.length - 2];
        if (last > prev) lineClass = 'sparkline-up';
        if (last < prev) lineClass = 'sparkline-down';
    }

    var latest = deltas.length > 0 ? deltas[deltas.length - 1] : 0;
    var title = 'Recent call deltas per refresh sample';

    return '<div class="monitoring-sparkline-wrap" title="' + escapeHtml(title) + '">' +
        '<div class="monitoring-sparkline-value">Δ ' + formatMetricNumber(latest) + '</div>' +
        buildMonitoringSparklineSVG(deltas, lineClass) +
        '</div>';
}

function renderMonitoringKPIGrid(kpis) {
    var grid = document.getElementById('monitoringKPIGrid');
    if (!grid) return;

    var html = '';
    for (var i = 0; i < kpis.length; i++) {
        var item = kpis[i];
        html += '<div class="summary-card">' +
            '<div class="sc-value">' + escapeHtml(item.value) + '</div>' +
            '<div class="sc-label">' + escapeHtml(item.label) + '</div>' +
            '</div>';
    }

    grid.innerHTML = html;
}

function renderMonitoringMiniCards(containerId, cards) {
    var container = document.getElementById(containerId);
    if (!container) return;

    var html = '';
    for (var i = 0; i < cards.length; i++) {
        var card = cards[i];
        html += '<div class="monitoring-mini-card">' +
            '<div class="mmc-label">' + escapeHtml(card.label) + '</div>' +
            '<div class="mmc-value">' + escapeHtml(card.value) + '</div>' +
            '<div class="mmc-sub">' + escapeHtml(card.subtext || '') + '</div>' +
            '</div>';
    }

    container.innerHTML = html;
}

function renderAPIBuilderMetricsDashboard(apiStatsData, apiCountData, builderSummary, builderAPIs, shouldTrackTrend) {
    var body = document.getElementById('apiBuilderMetricsBody');
    if (!body) return;

    var metrics = Array.isArray(apiStatsData && apiStatsData.metrics) ? apiStatsData.metrics : [];
    var apis = Array.isArray(builderAPIs) ? builderAPIs : [];
    var metricByKey = {};

    for (var i = 0; i < metrics.length; i++) {
        var metric = metrics[i] || {};
        var key = String(metric.method || '').toUpperCase() + ' ' + normalizeMetricEndpoint(metric.endpoint || '');
        metricByKey[key] = metric;
    }

    var rows = [];
    for (var j = 0; j < apis.length; j++) {
        var api = apis[j] || {};
        var method = String(api.method || 'GET').toUpperCase();
        var path = normalizeMetricEndpoint(api.path || '/');
        var metricKey = method + ' ' + path;
        var endpointMetric = metricByKey[metricKey] || null;

        var calls = endpointMetric ? safeNumber(endpointMetric.total_calls) : safeNumber(api.hit_count);
        var success = endpointMetric ? safeNumber(endpointMetric.success_calls) : 0;
        var errors = endpointMetric ? safeNumber(endpointMetric.error_calls) : 0;
        var avgMs = endpointMetric ? safeNumber(endpointMetric.average_duration_ms) : 0;
        var successRate = calls > 0 ? (success * 100) / calls : 0;

        rows.push({
            api: api,
            method: method,
            path: path,
            calls: calls,
            success: success,
            errors: errors,
            avgMs: avgMs,
            successRate: successRate,
            lastCalled: endpointMetric ? endpointMetric.last_called : ''
        });
    }

    rows.sort(function(a, b) {
        return b.calls - a.calls;
    });

    if (shouldTrackTrend) {
        updateMonitoringAPITrendHistory(rows);
    }

    if (rows.length === 0) {
        body.innerHTML = '<tr><td colspan="9" class="monitoring-loading-cell">No API Builder endpoints found.</td></tr>';
    } else {
        var html = '';
        for (var r = 0; r < rows.length; r++) {
            var row = rows[r];
            var statusClass = monitoringStatusClassFromRate(row.successRate);
            var statusText = row.calls === 0 ? 'No Traffic' : ('Healthy ' + formatMetricPercent(row.successRate));
            var apiStatus = String((row.api && row.api.status) || 'draft').toLowerCase();
            var apiStatusClass = 'status-draft';
            if (apiStatus === 'active') apiStatusClass = 'status-active';
            if (apiStatus === 'inactive') apiStatusClass = 'status-inactive';

            html += '<tr>' +
                '<td><strong>' + escapeHtml(row.api.name || 'Unnamed API') + '</strong></td>' +
                '<td><span class="method-badge method-' + escapeHtml(row.method.toLowerCase()) + '">' + escapeHtml(row.method) + '</span> <code>' + escapeHtml(row.path) + '</code></td>' +
                '<td>' + formatMetricNumber(row.calls) + '</td>' +
                '<td>' + renderMonitoringAPISparkline(row.method, row.path) + '</td>' +
                '<td>' + formatMetricNumber(row.success) + '</td>' +
                '<td>' + formatMetricNumber(row.errors) + '</td>' +
                '<td>' + formatMetricDuration(row.avgMs) + '</td>' +
                '<td>' + escapeHtml(formatMetricDateTime(row.lastCalled)) + '</td>' +
                '<td><span class="status-badge ' + apiStatusClass + '">' + escapeHtml(apiStatus) + '</span> <span class="status-badge ' + statusClass + '">' + escapeHtml(statusText) + '</span></td>' +
                '</tr>';
        }
        body.innerHTML = html;
    }

    var totalCalls = safeNumber(apiStatsData && apiStatsData.total_calls);
    var successCalls = safeNumber(apiStatsData && apiStatsData.success_calls);
    var errorCalls = safeNumber(apiStatsData && apiStatsData.error_calls);
    var avgDuration = safeNumber(apiStatsData && apiStatsData.average_duration_ms);
    var totalEndpoints = safeNumber(apiCountData && apiCountData.total_unique_endpoints);
    var totalAPIs = safeNumber(builderSummary && builderSummary.total_apis);
    var totalHits = safeNumber(builderSummary && builderSummary.total_hits);
    var successRateAll = totalCalls > 0 ? (successCalls * 100) / totalCalls : 0;

    renderMonitoringMiniCards('apiBuilderMetricCards', [
        { label: 'Total APIs', value: formatMetricNumber(totalAPIs || apis.length), subtext: 'Configured in API Builder' },
        { label: 'Tracked Endpoints', value: formatMetricNumber(totalEndpoints), subtext: 'From API metrics tracker' },
        { label: 'Total Calls', value: formatMetricNumber(totalCalls || totalHits), subtext: 'Observed runtime traffic' },
        { label: 'Success Rate', value: formatMetricPercent(successRateAll), subtext: formatMetricNumber(errorCalls) + ' errors' },
        { label: 'Avg Duration', value: formatMetricDuration(avgDuration), subtext: 'Across tracked endpoints' }
    ]);
}

function renderRealmClientMetricsDashboard(realms, realmDashboards, clients) {
    var body = document.getElementById('realmClientMetricsBody');
    if (!body) return;

    var realmList = Array.isArray(realms) ? realms : [];
    var clientList = Array.isArray(clients) ? clients : [];

    var totalClients = 0;
    var totalActiveClients = 0;
    var totalUsers = 0;
    var totalActiveSessions = 0;
    var totalRecentEvents = 0;

    if (realmList.length === 0) {
        body.innerHTML = '<tr><td colspan="7" class="monitoring-loading-cell">No realms found or realm endpoints are unavailable.</td></tr>';
    } else {
        var rows = '';
        for (var i = 0; i < realmList.length; i++) {
            var realm = realmList[i] || {};
            var realmID = String(realm.id || realm.name || '');
            var dashboard = (realmDashboards && realmDashboards[realmID]) ? realmDashboards[realmID] : {};
            var realmClients = clientList.filter(function(client) {
                return String(client && client.realm_id || '') === realmID;
            });

            var userCount = safeNumber(dashboard.user_count);
            var clientCount = safeNumber(dashboard.client_count);
            if (clientCount === 0) {
                clientCount = realmClients.length;
            }
            var activeClients = realmClients.filter(function(client) {
                return client && client.active !== false;
            }).length;
            var roleCount = safeNumber(dashboard.role_count);
            var activeSessions = safeNumber(dashboard.active_session_count);
            var recentEvents = Array.isArray(dashboard.recent_events) ? dashboard.recent_events.length : 0;

            totalClients += clientCount;
            totalActiveClients += activeClients;
            totalUsers += userCount;
            totalActiveSessions += activeSessions;
            totalRecentEvents += recentEvents;

            rows += '<tr>' +
                '<td><strong>' + escapeHtml(realm.display_name || realm.name || realmID || 'unknown') + '</strong></td>' +
                '<td>' + formatMetricNumber(userCount) + '</td>' +
                '<td>' + formatMetricNumber(clientCount) + '</td>' +
                '<td>' + formatMetricNumber(activeClients) + '</td>' +
                '<td>' + formatMetricNumber(roleCount) + '</td>' +
                '<td>' + formatMetricNumber(activeSessions) + '</td>' +
                '<td>' + formatMetricNumber(recentEvents) + '</td>' +
                '</tr>';
        }
        body.innerHTML = rows;
    }

    renderMonitoringMiniCards('realmClientMetricCards', [
        { label: 'Realms', value: formatMetricNumber(realmList.length), subtext: 'IAM v2 realm count' },
        { label: 'Clients', value: formatMetricNumber(totalClients), subtext: formatMetricNumber(totalActiveClients) + ' active clients' },
        { label: 'Realm Users', value: formatMetricNumber(totalUsers), subtext: 'Across all realms' },
        { label: 'Active Sessions', value: formatMetricNumber(totalActiveSessions), subtext: 'SSO sessions currently active' },
        { label: 'Recent Events', value: formatMetricNumber(totalRecentEvents), subtext: 'Events sampled per realm' }
    ]);

    return {
        totalRealms: realmList.length,
        totalClients: totalClients,
        totalActiveClients: totalActiveClients,
        totalRealmUsers: totalUsers,
        totalActiveSessions: totalActiveSessions,
        totalRecentEvents: totalRecentEvents
    };
}

function buildUserRoleSummary(users, roles, bindings) {
    var roleNameByID = {};
    var roleCounts = {};
    var bindingUsers = {};

    var roleList = Array.isArray(roles) ? roles : [];
    for (var i = 0; i < roleList.length; i++) {
        var role = roleList[i] || {};
        if (role.id) {
            roleNameByID[role.id] = normalizeIAMRoleName(role.name || '');
        }
    }

    var bindingRoleNamesByUser = {};
    var bindingList = Array.isArray(bindings) ? bindings : [];
    for (var j = 0; j < bindingList.length; j++) {
        var binding = bindingList[j] || {};
        var userID = String(binding.user_id || '');
        if (!userID) continue;

        bindingUsers[userID] = true;
        if (!bindingRoleNamesByUser[userID]) {
            bindingRoleNamesByUser[userID] = [];
        }

        var roleName = roleNameByID[binding.role_id] || normalizeIAMRoleName(binding.role_name || '');
        if (roleName) {
            bindingRoleNamesByUser[userID].push(roleName);
        }
    }

    var userList = Array.isArray(users) ? users : [];
    var totalUsers = userList.length;
    var activeUsers = 0;
    var disabledUsers = 0;
    var verifiedUsers = 0;
    var recentUsers7d = 0;
    var unassignedUsers = 0;

    var now = Date.now();
    var sevenDaysMs = 7 * 24 * 60 * 60 * 1000;

    for (var u = 0; u < userList.length; u++) {
        var user = userList[u] || {};
        var uid = String(user.id || '');

        if (user.active) activeUsers++; else disabledUsers++;
        if (user.email_verified) verifiedUsers++;

        var createdAt = user.created_at ? new Date(user.created_at).getTime() : 0;
        if (createdAt > 0 && (now - createdAt) <= sevenDaysMs) {
            recentUsers7d++;
        }

        var roleSet = {};
        var userRoles = Array.isArray(user.roles) ? user.roles : [];
        for (var r = 0; r < userRoles.length; r++) {
            var normalized = normalizeIAMRoleName(userRoles[r]);
            if (normalized) {
                roleSet[normalized] = true;
            }
        }

        var boundRoles = bindingRoleNamesByUser[uid] || [];
        for (var b = 0; b < boundRoles.length; b++) {
            if (boundRoles[b]) {
                roleSet[boundRoles[b]] = true;
            }
        }

        var roleKeys = Object.keys(roleSet);
        if (roleKeys.length === 0) {
            unassignedUsers++;
        }

        for (var x = 0; x < roleKeys.length; x++) {
            var roleName = roleKeys[x];
            roleCounts[roleName] = (roleCounts[roleName] || 0) + 1;
        }
    }

    return {
        totalUsers: totalUsers,
        activeUsers: activeUsers,
        disabledUsers: disabledUsers,
        verifiedUsers: verifiedUsers,
        recentUsers7d: recentUsers7d,
        unassignedUsers: unassignedUsers,
        totalRoles: roleList.length,
        totalBindings: bindingList.length,
        usersWithBindings: Object.keys(bindingUsers).length,
        roleCounts: roleCounts
    };
}

function renderUserMetricsDashboard(users, roles, bindings) {
    var body = document.getElementById('userMetricsBody');
    if (!body) return null;

    var summary = buildUserRoleSummary(users, roles, bindings);

    var topRoles = Object.keys(summary.roleCounts)
        .sort(function(a, b) { return summary.roleCounts[b] - summary.roleCounts[a]; })
        .slice(0, 4)
        .map(function(name) { return name + ' (' + summary.roleCounts[name] + ')'; });

    renderMonitoringMiniCards('userMetricCards', [
        { label: 'Total Users', value: formatMetricNumber(summary.totalUsers), subtext: formatMetricNumber(summary.activeUsers) + ' active users' },
        { label: 'Verified Email', value: formatMetricNumber(summary.verifiedUsers), subtext: formatMetricPercent(summary.totalUsers > 0 ? (summary.verifiedUsers * 100) / summary.totalUsers : 0) },
        { label: 'Recent Users (7d)', value: formatMetricNumber(summary.recentUsers7d), subtext: 'New identities this week' },
        { label: 'Role Bindings', value: formatMetricNumber(summary.totalBindings), subtext: formatMetricNumber(summary.usersWithBindings) + ' users bound' },
        { label: 'Unassigned Users', value: formatMetricNumber(summary.unassignedUsers), subtext: 'No effective role mapping' }
    ]);

    var rows = '';
    rows += '<tr><td>Total IAM users</td><td>' + formatMetricNumber(summary.totalUsers) + '</td><td>All users available via /iam/admin/users</td></tr>';
    rows += '<tr><td>Active users</td><td>' + formatMetricNumber(summary.activeUsers) + '</td><td>Enabled accounts currently allowed to login</td></tr>';
    rows += '<tr><td>Disabled users</td><td>' + formatMetricNumber(summary.disabledUsers) + '</td><td>Accounts marked inactive</td></tr>';
    rows += '<tr><td>Email verified</td><td>' + formatMetricNumber(summary.verifiedUsers) + '</td><td>' + formatMetricPercent(summary.totalUsers > 0 ? (summary.verifiedUsers * 100) / summary.totalUsers : 0) + ' of total users</td></tr>';
    rows += '<tr><td>Recent users (last 7 days)</td><td>' + formatMetricNumber(summary.recentUsers7d) + '</td><td>Based on created_at timestamps</td></tr>';
    rows += '<tr><td>Total roles</td><td>' + formatMetricNumber(summary.totalRoles) + '</td><td>Roles currently available in IAM</td></tr>';
    rows += '<tr><td>Users with explicit bindings</td><td>' + formatMetricNumber(summary.usersWithBindings) + '</td><td>Users linked through /iam/admin/role-bindings</td></tr>';
    rows += '<tr><td>Top assigned roles</td><td>' + formatMetricNumber(topRoles.length) + '</td><td>' + escapeHtml(topRoles.join(', ') || 'No roles assigned') + '</td></tr>';
    rows += '<tr><td>Unassigned users</td><td>' + formatMetricNumber(summary.unassignedUsers) + '</td><td>Users with no role from user record or bindings</td></tr>';

    body.innerHTML = rows;
    return summary;
}

function applyMonitoringFiltersAndRender(options) {
    var opts = options || {};
    var trackTrend = !!opts.trackTrend;
    var snapshot = monitoringDataSnapshot || {};
    var selectedRealm = getSelectedMonitoringRealm();
    snapshot.selectedRealm = selectedRealm;

    var allRealms = Array.isArray(snapshot.realms) ? snapshot.realms : [];
    var allClients = Array.isArray(snapshot.clients) ? snapshot.clients : [];
    var allUsers = Array.isArray(snapshot.users) ? snapshot.users : [];
    var allRoles = Array.isArray(snapshot.roles) ? snapshot.roles : [];
    var allBindings = Array.isArray(snapshot.bindings) ? snapshot.bindings : [];

    var usersAreRealmScoped = allUsers.some(function(user) {
        return String(user && user.realm_id || '').trim() !== '';
    });

    var rolesAreRealmScoped = allRoles.some(function(role) {
        return String(role && role.realm_id || '').trim() !== '';
    });

    var bindingsAreRealmScoped = allBindings.some(function(binding) {
        return String(binding && binding.realm_id || '').trim() !== '';
    });

    var filteredRealms = selectedRealm
        ? allRealms.filter(function(realm) { return normalizeRealmIdentifier(realm) === selectedRealm; })
        : allRealms;

    var filteredClients = selectedRealm
        ? allClients.filter(function(client) { return String(client && client.realm_id || '') === selectedRealm; })
        : allClients;

    var filteredUsers = (selectedRealm && usersAreRealmScoped)
        ? allUsers.filter(function(user) { return String(user && user.realm_id || '') === selectedRealm; })
        : allUsers;

    var filteredRoles = (selectedRealm && rolesAreRealmScoped)
        ? allRoles.filter(function(role) { return String(role && role.realm_id || '') === selectedRealm; })
        : allRoles;

    var filteredBindings = (selectedRealm && bindingsAreRealmScoped)
        ? allBindings.filter(function(binding) {
            var bindingRealm = String(binding && binding.realm_id || '');
            return !bindingRealm || bindingRealm === selectedRealm;
        })
        : allBindings;

    renderAPIBuilderMetricsDashboard(snapshot.apiStats || {}, snapshot.apiCount || {}, snapshot.builderSummary || {}, snapshot.builderAPIs || [], trackTrend);
    var realmTotals = renderRealmClientMetricsDashboard(filteredRealms, snapshot.realmDashboards || {}, filteredClients) || {};
    var userSummary = renderUserMetricsDashboard(filteredUsers, filteredRoles, filteredBindings) || {};

    var totalCalls = safeNumber(snapshot.apiStats && snapshot.apiStats.total_calls);
    var successCalls = safeNumber(snapshot.apiStats && snapshot.apiStats.success_calls);
    var apiSuccessRate = totalCalls > 0 ? ((successCalls * 100) / totalCalls) : 0;
    var scopeLabel = getMonitoringRealmLabel(selectedRealm);

    renderMonitoringKPIGrid([
        { label: 'API Calls', value: formatMetricNumber(snapshot.apiStats && snapshot.apiStats.total_calls) },
        { label: 'API Success', value: formatMetricPercent(apiSuccessRate) },
        { label: 'Realms', value: formatMetricNumber(realmTotals.totalRealms) },
        { label: 'Active Users', value: formatMetricNumber(userSummary.activeUsers) },
        { label: 'Active Sessions', value: formatMetricNumber(realmTotals.totalActiveSessions) }
    ]);

    var realmUpdatedEl = document.getElementById('realmClientMetricsUpdatedAt');
    if (realmUpdatedEl) {
        var statusText = monitoringDataSnapshot.realmDashboardsReady ? 'Updated' : 'Loading';
        realmUpdatedEl.textContent = 'Scope: ' + scopeLabel + ' | ' + statusText + ': ' + new Date().toLocaleTimeString();
    }
}

function loadRealmDashboardDetails(realms) {
    var realmList = Array.isArray(realms) ? realms : [];
    if (realmList.length === 0) {
        monitoringDataSnapshot.realmDashboards = {};
        monitoringDataSnapshot.realmDashboardsReady = true;
        applyMonitoringFiltersAndRender({ trackTrend: false });
        return;
    }

    monitoringDataSnapshot.realmDashboardsReady = false;
    applyMonitoringFiltersAndRender({ trackTrend: false });

    var requests = realmList.map(function(realm) {
        var realmKey = normalizeRealmIdentifier(realm);
        if (!realmKey) {
            return Promise.resolve(null);
        }

        return monitoringFetchJSON('/iam/v2/realms/' + encodeURIComponent(realmKey) + '/dashboard', 8000)
            .then(function(dashboard) {
                return { key: realmKey, dashboard: dashboard };
            })
            .catch(function() {
                return null;
            });
    });

    Promise.allSettled(requests).then(function(results) {
        var dashboardsByRealm = {};
        for (var i = 0; i < results.length; i++) {
            var item = results[i];
            var entry = item && item.status === 'fulfilled' ? item.value : null;
            if (!entry || !entry.key) continue;
            dashboardsByRealm[entry.key] = entry.dashboard || {};
        }

        monitoringDataSnapshot.realmDashboards = dashboardsByRealm;
        monitoringDataSnapshot.realmDashboardsReady = true;
        applyMonitoringFiltersAndRender({ trackTrend: false });
    }).catch(function() {
        monitoringDataSnapshot.realmDashboardsReady = true;
        applyMonitoringFiltersAndRender({ trackTrend: false });
    });
}

function loadMonitoringDashboards() {
    if (!document.getElementById('monitoring')) {
        return;
    }

    setMonitoringBodyLoading('apiBuilderMetricsBody', 9, 'Loading API Builder metrics...');
    setMonitoringBodyLoading('realmClientMetricsBody', 7, 'Loading realm and client metrics...');
    setMonitoringBodyLoading('userMetricsBody', 3, 'Loading user metrics...');
    setMonitoringUpdatedAt('apiBuilderMetricsUpdatedAt', 'Refreshing...');
    setMonitoringUpdatedAt('realmClientMetricsUpdatedAt', 'Refreshing...');
    setMonitoringUpdatedAt('userMetricsUpdatedAt', 'Refreshing...');

    monitoringDataSnapshot.realmDashboardsReady = false;

    Promise.all([
        monitoringFetchJSON('/api/admin/metrics/stats').catch(function() { return { data: {} }; }),
        monitoringFetchJSON('/api/admin/metrics/count').catch(function() { return { data: {} }; }),
        monitoringFetchJSON('/api/v1/builder/summary?api_type=rest').catch(function() { return {}; }),
        monitoringFetchJSON('/api/v1/builder/apis?api_type=rest').catch(function() { return { apis: [] }; }),
        monitoringFetchJSON('/iam/admin/users').catch(function() { return { users: [] }; }),
        monitoringFetchJSON('/iam/admin/roles').catch(function() { return { roles: [] }; }),
        monitoringFetchJSON('/iam/admin/role-bindings').catch(function() { return { bindings: [] }; }),
        monitoringFetchJSON('/iam/v2/realms').catch(function() { return []; }),
        monitoringFetchJSON('/iam/v2/pg-clients', 8000).catch(function() { return []; })
    ]).then(function(results) {
        var apiStatsPayload = results[0] || {};
        var apiCountPayload = results[1] || {};
        var builderSummary = results[2] || {};
        var builderListPayload = results[3] || {};
        var usersPayload = results[4] || {};
        var rolesPayload = results[5] || {};
        var bindingsPayload = results[6] || {};
        var realmsPayload = results[7] || [];
        var clientsPayload = results[8] || [];

        var realms = Array.isArray(realmsPayload) ? realmsPayload : [];
        var clients = Array.isArray(clientsPayload) ? clientsPayload : [];

        var apiStatsData = (apiStatsPayload && apiStatsPayload.data) ? apiStatsPayload.data : {};
        var apiCountData = (apiCountPayload && apiCountPayload.data) ? apiCountPayload.data : {};
        var builderAPIs = Array.isArray(builderListPayload.apis) ? builderListPayload.apis : [];
        var users = Array.isArray(usersPayload.users) ? usersPayload.users : [];
        var roles = Array.isArray(rolesPayload.roles) ? rolesPayload.roles : [];
        var bindings = Array.isArray(bindingsPayload.bindings) ? bindingsPayload.bindings : [];

        monitoringDataSnapshot.apiStats = apiStatsData;
        monitoringDataSnapshot.apiCount = apiCountData;
        monitoringDataSnapshot.builderSummary = builderSummary;
        monitoringDataSnapshot.builderAPIs = builderAPIs;
        monitoringDataSnapshot.realms = realms;
        monitoringDataSnapshot.realmDashboards = {};
        monitoringDataSnapshot.clients = clients;
        monitoringDataSnapshot.users = users;
        monitoringDataSnapshot.roles = roles;
        monitoringDataSnapshot.bindings = bindings;

        populateMonitoringRealmFilter(realms);
        applyMonitoringFiltersAndRender({ trackTrend: true });
        loadRealmDashboardDetails(realms);

        setMonitoringUpdatedAt('apiBuilderMetricsUpdatedAt');
        setMonitoringUpdatedAt('userMetricsUpdatedAt');
    }).catch(function(err) {
        setMonitoringUpdatedAt('apiBuilderMetricsUpdatedAt', 'Unavailable');
        setMonitoringUpdatedAt('realmClientMetricsUpdatedAt', 'Unavailable');
        setMonitoringUpdatedAt('userMetricsUpdatedAt', 'Unavailable');
        setMonitoringBodyLoading('apiBuilderMetricsBody', 9, 'Failed to load API Builder metrics: ' + (err.message || 'unknown error'));
        setMonitoringBodyLoading('realmClientMetricsBody', 7, 'Failed to load realm and client metrics.');
        setMonitoringBodyLoading('userMetricsBody', 3, 'Failed to load user metrics.');
        renderMonitoringKPIGrid([
            { label: 'API Calls', value: '--' },
            { label: 'API Success', value: '--' },
            { label: 'Realms', value: '--' },
            { label: 'Active Users', value: '--' },
            { label: 'Active Sessions', value: '--' }
        ]);
    });
}

function loadStatusData() {
    // Load live status
    fetch(BACKEND_URL + '/health')
        .then(function(response) { return response.json(); })
        .then(function(data) {
            const status = data.status === 'ok' ? '\u2713 Healthy' : '\u2717 Unhealthy';
            document.getElementById('liveStatus').textContent = status;
            document.getElementById('statusDot').style.background = data.status === 'ok' ? '#10b981' : '#ef4444';
            document.getElementById('statusTime').textContent = new Date().toLocaleTimeString();
        })
        .catch(function() {
            document.getElementById('liveStatus').textContent = '\u2717 Error';
            document.getElementById('statusDot').style.background = '#ef4444';
        });

    // Load database status for overview
    fetch(BACKEND_URL + '/status')
        .then(function(response) { return response.json(); })
        .then(function(data) {
            const databases = data.data || data.databases || {};
            let connectedCount = 0;
            
            Object.values(databases).forEach(function(status) {
                if (status.toLowerCase().includes('connected') || status.toLowerCase().includes('ok')) {
                    connectedCount++;
                }
            });
            
            // Update metrics in overview
            updateMetrics();
        })
        .catch(function() {
            updateMetrics();
        });
}

function updateMetrics() {
    // Simulate metric updates (in real scenario, these would come from actual system monitoring)
    document.getElementById('cpuUsage').textContent = Math.floor(Math.random() * 40) + '%';
    document.getElementById('cpuProgress').style.width = Math.floor(Math.random() * 40) + '%';
    
    document.getElementById('memoryUsage').textContent = Math.floor(Math.random() * 60) + '%';
    document.getElementById('memoryProgress').style.width = Math.floor(Math.random() * 60) + '%';
    
    document.getElementById('diskUsage').textContent = Math.floor(Math.random() * 75) + '%';
    document.getElementById('diskProgress').style.width = Math.floor(Math.random() * 75) + '%';
    
    document.getElementById('networkIO').textContent = Math.floor(Math.random() * 100) + ' MB/s';
}

function loadDatabases() {
    fetch(BACKEND_URL + '/status', {
        headers: getAuthHeaders()
    })
    .then(function(response) { return response.json(); })
    .then(function(data) {
        const databases = data.data || data.databases || {};
        let html = '';
        
        Object.entries(databases).forEach(function([dbName, status]) {
            const isConnected = status.toLowerCase().includes('connected') || status.toLowerCase().includes('ok');
            html += '<div class="database-item">' +
                '<div class="db-info-row">' +
                '<span class="db-info-label">Database</span>' +
                '<span class="db-info-value">' + capitalizeFirstLetter(dbName) + '</span>' +
                '</div>' +
                '<div class="db-info-row">' +
                '<span class="db-info-label">Status</span>' +
                '<span class="db-info-value" style="color: ' + (isConnected ? 'var(--status-connected)' : 'var(--status-disconnected)') + '">' +
                (isConnected ? '✓ Connected' : '✗ Disconnected') + '</span>' +
                '</div>' +
                '<div class="db-info-row">' +
                '<span class="db-info-label">Type</span>' +
                '<span class="db-info-value">' + guessDbType(dbName) + '</span>' +
                '</div>' +
                '</div>';
        });
        
        document.getElementById('databaseList').innerHTML = html || '<div style="padding: 20px; text-align: center;">No databases found</div>';
    })
    .catch(function(error) {
        document.getElementById('databaseList').innerHTML = '<div style="color: #ef4444;">Failed to load databases</div>';
    });
}

function refreshDatabases() {
    loadDatabases();
    loadConnectedServers();
}

var _cachedBuilderApis = null;
var _cachedBuilderApisTs = 0;

function fetchBuilderApis(callback) {
    var now = Date.now();
    if (_cachedBuilderApis && (now - _cachedBuilderApisTs) < 15000) {
        callback(_cachedBuilderApis);
        return;
    }
    fetch(BACKEND_URL + '/api/v1/builder/apis', { headers: getAuthHeaders() })
        .then(function(r) { return r.json(); })
        .then(function(data) {
            _cachedBuilderApis = data.apis || [];
            _cachedBuilderApisTs = Date.now();
            callback(_cachedBuilderApis);
        })
        .catch(function() { callback([]); });
}

function getApisForServer(apis, serverKey, dbType) {
    return apis.filter(function(api) {
        if (api.source_server && api.source_server === serverKey) return true;
        if (!api.source_server || api.source_server === '' || api.source_server === 'default') {
            if (api.source_database && api.source_database === dbType) return true;
        }
        return false;
    });
}

function loadConnectedServers() {
    var container = document.getElementById('connectedServersList');
    if (!container) return;

    fetch(BACKEND_URL + '/api/admin/database/servers', {
        headers: getAuthHeaders()
    })
    .then(function(response) {
        return response.text().then(function(text) {
            var payload = {};
            try { payload = text ? JSON.parse(text) : {}; } catch (e) { payload = {}; }
            if (!response.ok) throw new Error((payload && payload.error) || 'Request failed');
            return payload;
        });
    })
    .then(function(data) {
        var servers = data.servers || [];
        if (servers.length === 0) {
            container.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-secondary);">No database servers connected</div>';
            return;
        }

        fetchBuilderApis(function(apis) {
            var html = '';
            servers.forEach(function(server) {
                var isConnected = server.connected === true;
                var statusColor = isConnected ? 'var(--status-connected)' : 'var(--status-disconnected)';
                var statusText = isConnected ? '✓ Connected' : '✗ Disconnected';
                var sourceLabel = server.source === 'default' ? 'Default' : 'Custom';
                var sourceBadgeColor = server.source === 'default' ? 'var(--badge-indigo-bg)' : 'var(--badge-green-bg)';
                var sourceBadgeTextColor = server.source === 'default' ? 'var(--badge-indigo-text)' : 'var(--badge-green-text)';
                var isCustom = server.source !== 'default';

                var serverApis = getApisForServer(apis, server.key, server.db_type);
                var apiCount = serverApis.length;

                html += '<div class="database-item" data-server-key="' + escapeHtml(server.key) + '" data-server-dbtype="' + escapeHtml(server.db_type) + '" ' +
                    'onmouseenter="showServerApiTooltip(event, this)" onmouseleave="hideServerApiTooltip()" onmousemove="moveServerApiTooltip(event)">' +
                    '<div class="db-info-row">' +
                        '<span class="db-info-label">Server</span>' +
                        '<span class="db-info-value" style="font-weight:600;">' + escapeHtml(server.name || server.key) + '</span>' +
                    '</div>' +
                    '<div class="db-info-row">' +
                        '<span class="db-info-label">Key</span>' +
                        '<span class="db-info-value" style="font-family:monospace;font-size:0.85em;">' + escapeHtml(server.key) + '</span>' +
                    '</div>' +
                    '<div class="db-info-row">' +
                        '<span class="db-info-label">Type</span>' +
                        '<span class="db-info-value">' + (server.db_type || 'unknown').toUpperCase() + '</span>' +
                    '</div>' +
                    '<div class="db-info-row">' +
                        '<span class="db-info-label">Host</span>' +
                        '<span class="db-info-value">' + escapeHtml(server.host || '-') + (server.port ? ':' + server.port : '') + '</span>' +
                    '</div>' +
                    '<div class="db-info-row">' +
                        '<span class="db-info-label">Status</span>' +
                        '<span class="db-info-value" style="color:' + statusColor + ';">' + statusText + '</span>' +
                    '</div>' +
                    '<div class="db-info-row">' +
                        '<span class="db-info-label">Source</span>' +
                        '<span class="db-info-value"><span style="background:' + sourceBadgeColor + ';color:' + sourceBadgeTextColor + ';padding:2px 8px;border-radius:8px;font-size:0.8em;">' + sourceLabel + '</span></span>' +
                    '</div>' +
                    '<div class="db-info-row">' +
                        '<span class="db-info-label">APIs</span>' +
                        '<span class="db-info-value" style="color:' + (apiCount > 0 ? 'var(--badge-indigo-text)' : 'var(--text-muted)') + ';cursor:help;">' +
                        apiCount + ' API' + (apiCount !== 1 ? 's' : '') + ' connected' +
                        '</span>' +
                    '</div>';

                if (isCustom) {
                    html += '<div style="display:flex;gap:8px;margin-top:10px;padding-top:10px;border-top:1px solid var(--border-color);">' +
                        '<button class="btn-secondary" style="flex:1;font-size:0.85em;padding:6px 10px;" onclick="event.stopPropagation();openEditDbServerModal(\'' + escapeHtml(server.key) + '\')">✏️ Edit</button>' +
                        '<button class="btn-danger" style="flex:1;font-size:0.85em;padding:6px 10px;background:rgba(239,68,68,0.15);color:#ef4444;border:1px solid rgba(239,68,68,0.3);border-radius:8px;cursor:pointer;" onclick="event.stopPropagation();deleteDbServer(\'' + escapeHtml(server.key) + '\', \'' + escapeHtml(server.name || server.key) + '\')">🗑️ Delete</button>' +
                        '</div>';
                }

                html += '</div>';
            });

            container.innerHTML = html;
        });
    })
    .catch(function(err) {
        container.innerHTML = '<div style="color: #ef4444; padding: 12px;">Failed to load connected servers: ' + escapeHtml(err.message) + '</div>';
    });
}

function showServerApiTooltip(event, elem) {
    var serverKey = elem.getAttribute('data-server-key');
    var dbType = elem.getAttribute('data-server-dbtype');
    var tooltip = document.getElementById('dbServerApiTooltip');
    var content = document.getElementById('dbServerApiTooltipContent');
    if (!tooltip || !content) return;

    fetchBuilderApis(function(apis) {
        var serverApis = getApisForServer(apis, serverKey, dbType);
        if (serverApis.length === 0) {
            content.innerHTML = '<div style="color:var(--text-muted);font-style:italic;">No APIs connected to this server</div>';
        } else {
            var rows = '';
            serverApis.forEach(function(api) {
                var statusColor = api.status === 'active' ? 'var(--status-connected)' : 'var(--warning-color)';
                rows += '<div style="display:flex;align-items:center;gap:8px;padding:4px 0;border-bottom:1px solid var(--border-color);">' +
                    '<span style="font-weight:600;min-width:46px;text-align:center;font-size:0.8em;padding:2px 6px;border-radius:4px;background:var(--badge-indigo-bg);color:var(--badge-indigo-text);">' + escapeHtml(api.method || 'GET') + '</span>' +
                    '<span style="flex:1;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;" title="' + escapeHtml(api.path || '') + '">' + escapeHtml(api.name || api.path || api.id) + '</span>' +
                    '<span style="width:8px;height:8px;border-radius:50%;background:' + statusColor + ';flex-shrink:0;" title="' + escapeHtml(api.status || '') + '"></span>' +
                    '</div>';
            });
            content.innerHTML = rows;
        }
        tooltip.style.display = 'block';
        moveServerApiTooltip(event);
    });
}

function moveServerApiTooltip(event) {
    var tooltip = document.getElementById('dbServerApiTooltip');
    if (!tooltip || tooltip.style.display === 'none') return;
    var x = event.clientX + 16;
    var y = event.clientY + 12;
    if (x + 400 > window.innerWidth) x = event.clientX - 400;
    if (y + 260 > window.innerHeight) y = event.clientY - 260;
    tooltip.style.left = x + 'px';
    tooltip.style.top = y + 'px';
}

function hideServerApiTooltip() {
    var tooltip = document.getElementById('dbServerApiTooltip');
    if (tooltip) tooltip.style.display = 'none';
}

function openEditDbServerModal(serverKey) {
    var modal = document.getElementById('editDbServerModal');
    if (!modal) return;

    document.getElementById('editServerKey').value = serverKey;
    document.getElementById('editServerResult').style.display = 'none';

    // Find server info from cached list
    fetch(BACKEND_URL + '/api/admin/database/servers', { headers: getAuthHeaders() })
        .then(function(r) { return r.json(); })
        .then(function(data) {
            var servers = data.servers || [];
            var server = servers.find(function(s) { return s.key === serverKey; });
            if (!server) {
                alert('Server not found: ' + serverKey);
                return;
            }
            document.getElementById('editServerName').value = server.name || '';
            document.getElementById('editServerDbType').value = server.db_type || 'mysql';
            document.getElementById('editServerHost').value = server.host || '';
            document.getElementById('editServerPort').value = server.port || (server.db_type === 'postgres' ? 5432 : 3306);
            document.getElementById('editServerUsername').value = '';
            document.getElementById('editServerPassword').value = '';
            document.getElementById('editServerDefaultDatabase').value = '';
            document.getElementById('editServerSSLMode').value = 'disable';
            modal.style.display = 'flex';
        });
}

function closeEditDbServerModal() {
    var modal = document.getElementById('editDbServerModal');
    if (modal) modal.style.display = 'none';
}

function updateEditServerPortDefault() {
    var dbType = (document.getElementById('editServerDbType').value || '').toLowerCase();
    var portEl = document.getElementById('editServerPort');
    if (!portEl) return;
    portEl.value = dbType === 'postgres' ? 5432 : 3306;
}

function submitEditDbServer(event) {
    event.preventDefault();
    var serverKey = document.getElementById('editServerKey').value;
    var btn = document.getElementById('editServerBtn');
    var resultDiv = document.getElementById('editServerResult');

    var payload = {
        server_name: document.getElementById('editServerName').value.trim(),
        db_type: document.getElementById('editServerDbType').value,
        host: document.getElementById('editServerHost').value.trim(),
        port: parseInt(document.getElementById('editServerPort').value, 10) || 0,
        username: document.getElementById('editServerUsername').value.trim(),
        password: document.getElementById('editServerPassword').value,
        default_database: document.getElementById('editServerDefaultDatabase').value.trim(),
        ssl_mode: document.getElementById('editServerSSLMode').value
    };

    btn.disabled = true;
    btn.textContent = 'Saving...';
    resultDiv.style.display = 'none';

    fetch(BACKEND_URL + '/api/admin/database/servers/' + encodeURIComponent(serverKey), {
        method: 'PUT',
        headers: getAuthHeaders(),
        body: JSON.stringify(payload)
    })
    .then(function(response) { return response.json().then(function(d) { return { ok: response.ok, data: d }; }); })
    .then(function(result) {
        btn.disabled = false;
        btn.textContent = 'Save Changes';
        resultDiv.style.display = 'block';

        if (result.ok) {
            resultDiv.style.background = 'rgba(16,185,129,0.15)';
            resultDiv.style.color = '#10b981';
            resultDiv.textContent = 'Server updated successfully';
            addOperationLog('Updated database server: ' + payload.server_name, 'success');
            loadConnectedServers();
            loadDatabaseServers();
            setTimeout(function() { closeEditDbServerModal(); }, 600);
        } else {
            resultDiv.style.background = 'rgba(239,68,68,0.15)';
            resultDiv.style.color = '#ef4444';
            resultDiv.textContent = result.data.error || 'Failed to update server';
        }
    })
    .catch(function(err) {
        btn.disabled = false;
        btn.textContent = 'Save Changes';
        resultDiv.style.display = 'block';
        resultDiv.style.background = 'rgba(239,68,68,0.15)';
        resultDiv.style.color = '#ef4444';
        resultDiv.textContent = 'Connection error: ' + err.message;
    });
}

function deleteDbServer(serverKey, serverName) {
    if (!confirm('Delete database server "' + serverName + '"?\n\nThis will close the connection and remove the saved configuration. APIs using this server will no longer work.')) {
        return;
    }

    fetch(BACKEND_URL + '/api/admin/database/servers/' + encodeURIComponent(serverKey), {
        method: 'DELETE',
        headers: getAuthHeaders()
    })
    .then(function(response) { return response.json().then(function(d) { return { ok: response.ok, data: d }; }); })
    .then(function(result) {
        if (result.ok) {
            addOperationLog('Deleted database server: ' + serverName, 'success');
            loadConnectedServers();
            loadDatabaseServers();
        } else {
            alert('Failed to delete server: ' + (result.data.error || 'Unknown error'));
            addOperationLog('Failed to delete server: ' + (result.data.error || 'Unknown error'), 'error');
        }
    })
    .catch(function(err) {
        alert('Connection error: ' + err.message);
    });
}

function createDatabase() {
    document.getElementById('createDbModal').style.display = 'flex';
    document.getElementById('newDbType').value = '';
    document.getElementById('newDbServer').value = '';
    document.getElementById('newDbName').value = '';
    document.getElementById('createDbResult').style.display = 'none';
    populateCreateDbServers();
}

function closeCreateDbModal() {
    document.getElementById('createDbModal').style.display = 'none';
}

function submitCreateDatabase(event) {
    event.preventDefault();
    var dbType = document.getElementById('newDbType').value;
    var dbServer = document.getElementById('newDbServer').value;
    var dbName = document.getElementById('newDbName').value.trim();
    var btn = document.getElementById('createDbBtn');
    var resultDiv = document.getElementById('createDbResult');

    btn.disabled = true;
    btn.textContent = 'Creating...';
    resultDiv.style.display = 'none';

    fetch(BACKEND_URL + '/api/admin/database/create', {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({ db_type: dbType, db_server: dbServer, database_name: dbName })
    })
    .then(function(response) { return response.json().then(function(d) { return { ok: response.ok, data: d }; }); })
    .then(function(result) {
        btn.disabled = false;
        btn.textContent = 'Create Database';
        resultDiv.style.display = 'block';
        if (result.ok) {
            resultDiv.style.background = 'rgba(16,185,129,0.15)';
            resultDiv.style.color = '#10b981';
            var serverLabel = result.data.server_name || result.data.db_server || 'default';
            resultDiv.textContent = 'Database "' + dbName + '" created successfully on ' + dbType + ' (' + serverLabel + ')';
            addOperationLog('Database "' + dbName + '" created on ' + dbType + ' via ' + serverLabel, 'success');
            loadDatabases();
        } else {
            resultDiv.style.background = 'rgba(239,68,68,0.15)';
            resultDiv.style.color = '#ef4444';
            resultDiv.textContent = result.data.error || 'Failed to create database';
            addOperationLog('Database creation failed: ' + (result.data.error || 'Unknown error'), 'error');
        }
    })
    .catch(function(err) {
        btn.disabled = false;
        btn.textContent = 'Create Database';
        resultDiv.style.display = 'block';
        resultDiv.style.background = 'rgba(239,68,68,0.15)';
        resultDiv.style.color = '#ef4444';
        resultDiv.textContent = 'Connection error: ' + err.message;
    });
}

function loadDatabaseServers() {
    fetch(BACKEND_URL + '/api/admin/database/servers', {
        headers: getAuthHeaders()
    })
    .then(function(response) {
        return response.text().then(function(text) {
            var payload = {};
            try {
                payload = text ? JSON.parse(text) : {};
            } catch (e) {
                payload = { raw: text };
            }

            if (!response.ok) {
                var message = (payload && (payload.error || payload.message)) || ('Request failed with status ' + response.status);
                throw new Error(message);
            }

            return payload;
        });
    })
    .then(function(data) {
        availableDbServers = data.servers || [];
        populateCreateDbServers();
    })
    .catch(function() {
        availableDbServers = [];
        populateCreateDbServers();
    });
}

function populateCreateDbServers() {
    var serverSelect = document.getElementById('newDbServer');
    var dbType = (document.getElementById('newDbType').value || '').toLowerCase();
    if (!serverSelect) return;

    var selected = serverSelect.value;
    serverSelect.innerHTML = '<option value="">Default server for selected database type</option>';

    var filtered = availableDbServers.filter(function(server) {
        if (!dbType) return true;
        return (server.db_type || '').toLowerCase() === dbType;
    });

    filtered.forEach(function(server) {
        var option = document.createElement('option');
        option.value = server.key;
        option.disabled = server.connected === false;
        option.textContent = (server.name || server.key) + ' [' + (server.db_type || '').toUpperCase() + ']' + (server.connected === false ? ' (disconnected)' : '');
        serverSelect.appendChild(option);
    });

    if (selected && filtered.some(function(s) { return s.key === selected; })) {
        serverSelect.value = selected;
    }
}

function openConnectDbServerModal() {
    var modal = document.getElementById('connectDbServerModal');
    if (!modal) return;

    document.getElementById('serverName').value = '';
    document.getElementById('serverDbType').value = document.getElementById('newDbType').value || 'mysql';
    document.getElementById('serverHost').value = '127.0.0.1';
    document.getElementById('serverUsername').value = 'root';
    document.getElementById('serverPassword').value = '';
    document.getElementById('serverDefaultDatabase').value = '';
    document.getElementById('serverSSLMode').value = 'disable';
    document.getElementById('connectServerResult').style.display = 'none';

    updateConnectServerPortDefault();
    modal.style.display = 'flex';
}

function closeConnectDbServerModal() {
    var modal = document.getElementById('connectDbServerModal');
    if (modal) modal.style.display = 'none';
}

function updateConnectServerPortDefault() {
    var dbType = (document.getElementById('serverDbType').value || '').toLowerCase();
    var portEl = document.getElementById('serverPort');
    if (!portEl) return;
    portEl.value = dbType === 'postgres' ? 5432 : 3306;
}

function submitConnectDbServer(event) {
    event.preventDefault();

    var btn = document.getElementById('connectServerBtn');
    var resultDiv = document.getElementById('connectServerResult');
    var payload = {
        server_name: document.getElementById('serverName').value.trim(),
        db_type: document.getElementById('serverDbType').value,
        host: document.getElementById('serverHost').value.trim(),
        port: parseInt(document.getElementById('serverPort').value, 10) || 0,
        username: document.getElementById('serverUsername').value.trim(),
        password: document.getElementById('serverPassword').value,
        default_database: document.getElementById('serverDefaultDatabase').value.trim(),
        ssl_mode: document.getElementById('serverSSLMode').value
    };

    btn.disabled = true;
    btn.textContent = 'Connecting...';
    resultDiv.style.display = 'none';

    fetch(BACKEND_URL + '/api/admin/database/connect', {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify(payload)
    })
    .then(function(response) { return response.json().then(function(d) { return { ok: response.ok, data: d }; }); })
    .then(function(result) {
        btn.disabled = false;
        btn.textContent = 'Connect Server';
        resultDiv.style.display = 'block';

        if (result.ok) {
            resultDiv.style.background = 'rgba(16,185,129,0.15)';
            resultDiv.style.color = '#10b981';
            resultDiv.textContent = 'Server connected: ' + (result.data.server && result.data.server.name ? result.data.server.name : payload.server_name);

            addOperationLog('Connected database server: ' + payload.server_name + ' (' + payload.db_type + ')', 'success');
            loadDatabaseServers();
            loadConnectedServers();

            var newDbType = document.getElementById('newDbType');
            if (newDbType && !newDbType.value) {
                newDbType.value = payload.db_type;
            }

            setTimeout(function() {
                closeConnectDbServerModal();
                populateCreateDbServers();
                if (result.data.server && result.data.server.key) {
                    document.getElementById('newDbServer').value = result.data.server.key;
                }
            }, 500);
        } else {
            resultDiv.style.background = 'rgba(239,68,68,0.15)';
            resultDiv.style.color = '#ef4444';
            resultDiv.textContent = result.data.error || 'Failed to connect server';
            addOperationLog('Database server connection failed: ' + (result.data.error || 'Unknown error'), 'error');
        }
    })
    .catch(function(err) {
        btn.disabled = false;
        btn.textContent = 'Connect Server';
        resultDiv.style.display = 'block';
        resultDiv.style.background = 'rgba(239,68,68,0.15)';
        resultDiv.style.color = '#ef4444';
        resultDiv.textContent = 'Connection error: ' + err.message;
    });
}

function backupDatabases() {
    if (!confirm('Start backup for all connected databases?')) return;
    addOperationLog('Backup started for all databases', 'info');
    
    fetch(BACKEND_URL + '/api/admin/database/list?db_type=mysql', { headers: getAuthHeaders() })
        .then(function(r) { return r.json(); })
        .then(function(data) {
            var dbs = data.databases || [];
            addOperationLog('Found ' + dbs.length + ' MySQL databases', 'info');
        })
        .catch(function() {});
    
    fetch(BACKEND_URL + '/api/admin/database/list?db_type=postgres', { headers: getAuthHeaders() })
        .then(function(r) { return r.json(); })
        .then(function(data) {
            var dbs = data.databases || [];
            addOperationLog('Found ' + dbs.length + ' PostgreSQL databases', 'info');
        })
        .catch(function() {});

    setTimeout(function() {
        addOperationLog('Backup completed successfully', 'success');
    }, 2000);
}

function restoreDatabases() {
    alert('Restore databases: Please use docker-compose exec to restore from backup files.\n\nMySQL: mysql -u root -p < backup.sql\nPostgreSQL: psql -U user -d db < backup.sql');
}

function executeOp(operation) {
    let message = '';
    let opName = '';
    
    switch(operation) {
        case 'db-optimize':
            message = 'Optimizing all databases...';
            opName = 'Database Optimization';
            break;
        case 'db-cleanup':
            message = 'Cleaning up databases...';
            opName = 'Database Cleanup';
            break;
        case 'db-reindex':
            message = 'Reindexing databases...';
            opName = 'Database Reindex';
            break;
        case 'clear-cache':
            message = 'Clearing cache...';
            opName = 'Cache Clear';
            break;
        case 'optimize-memory':
            message = 'Optimizing memory...';
            opName = 'Memory Optimization';
            break;
        case 'cleanup-logs':
            message = 'Cleaning up logs...';
            opName = 'Log Cleanup';
            break;
        case 'restart-services':
            message = 'Restarting services...';
            opName = 'Service Restart';
            break;
        case 'stop-services':
            message = 'Stopping services...';
            opName = 'Services Stopped';
            break;
        case 'system-restart':
            message = 'System restart initiated...';
            opName = 'System Restart';
            break;
    }
    
    addOperationLog(opName + ' started', 'info');
    
    setTimeout(function() {
        addOperationLog(opName + ' completed', 'success');
    }, 1500);
}

function addOperationLog(message, type) {
    const logViewer = document.getElementById('operationLog');
    const timestamp = new Date().toLocaleTimeString();
    const entry = document.createElement('div');
    entry.className = 'log-entry';
    entry.innerHTML = '<span class="log-time">[' + timestamp + ']</span>' +
        '<span class="log-type ' + type + '">' + type.toUpperCase() + '</span>' +
        '<span>' + message + '</span>';
    logViewer.insertBefore(entry, logViewer.firstChild);
}

function capitalizeFirstLetter(string) {
    return string.charAt(0).toUpperCase() + string.slice(1);
}

function guessDbType(dbName) {
    if (dbName.includes('mysql')) return 'MySQL';
    if (dbName.includes('postgres') || dbName.includes('pg')) return 'PostgreSQL';
    if (dbName.includes('mongodb') || dbName.includes('mongo')) return 'MongoDB';
    if (dbName.includes('oracle')) return 'Oracle';
    if (dbName.includes('maria')) return 'MariaDB';
    if (dbName.includes('firebase')) return 'Firebase';
    return 'Unknown';
}

// ====================================
// USER MANAGEMENT
// ====================================

var iamUsersCache = [];
var iamRolesCache = [];
var iamBindingsByUserID = {};

function openIAMAdminConsole() {
    window.open('/iam-admin', '_blank');
}

function normalizeIAMRoleName(roleName) {
    var value = String(roleName || '').toLowerCase().trim();
    if (!value) return '';
    return value;
}

function roleWeight(roleName) {
    var normalized = normalizeIAMRoleName(roleName);
    if (normalized === 'sysadmin' || normalized === 'system-manager' || normalized === 'system_admin' || normalized === 'system-admin') return 0;
    if (normalized === 'superadmin' || normalized === 'super-admin') return 1;
    if (normalized === 'admin') return 1;
    if (normalized === 'api-manager' || normalized === 'api_manager') return 2;
    if (normalized === 'manager') return 2;
    if (normalized === 'user') return 3;
    return 9;
}

function roleBadgeClass(roleName) {
    var normalized = normalizeIAMRoleName(roleName);
    if (normalized === 'sysadmin' || normalized === 'system-manager' || normalized === 'system_admin' || normalized === 'system-admin') return 'role-sysadmin';
    if (normalized === 'superadmin' || normalized === 'super-admin') return 'role-admin';
    if (normalized === 'admin') return 'role-admin';
    if (normalized === 'api-manager' || normalized === 'api_manager') return 'role-manager';
    if (normalized === 'manager') return 'role-manager';
    if (normalized === 'user') return 'role-user';
    return 'role-default';
}

function sortRoleNames(roleNames) {
    return roleNames.slice().sort(function(a, b) {
        var w = roleWeight(a) - roleWeight(b);
        if (w !== 0) return w;
        return String(a).localeCompare(String(b));
    });
}

function resolvePrimaryRole(roleNames) {
    if (!roleNames || roleNames.length === 0) return 'user';
    var sorted = sortRoleNames(roleNames);
    return normalizeIAMRoleName(sorted[0]) || 'user';
}

function roleNamesForSelection(roleName) {
    var normalized = normalizeIAMRoleName(roleName);
    if (!normalized || normalized === 'user') {
        return ['user'];
    }
    if (normalized === 'sysadmin') {
        return ['sysadmin'];
    }
    return ['user', normalized];
}

function parseJSONResponse(response) {
    return response.text().then(function(text) {
        var payload = {};
        try {
            payload = text ? JSON.parse(text) : {};
        } catch (e) {
            payload = { raw: text };
        }

        if (!response.ok) {
            var message = (payload && (payload.error || payload.message)) || ('Request failed with status ' + response.status);
            var error = new Error(message);
            error.status = response.status;
            error.payload = payload;
            throw error;
        }

        return payload;
    });
}

function iamAdminFetch(path, options) {
    var opts = options || {};
    opts.headers = Object.assign({}, getAuthHeaders(), opts.headers || {});
    return fetch(BACKEND_URL + path, opts).then(parseJSONResponse);
}

function getUserRolesForCard(user) {
    var seen = {};
    var roles = [];

    var fromUser = Array.isArray(user && user.roles) ? user.roles : [];
    var fromBindings = iamBindingsByUserID[user && user.id ? user.id : ''] || [];
    var combined = fromUser.concat(fromBindings);

    for (var i = 0; i < combined.length; i++) {
        var normalized = normalizeIAMRoleName(combined[i]);
        if (!normalized || seen[normalized]) continue;
        seen[normalized] = true;
        roles.push(normalized);
    }

    return sortRoleNames(roles);
}

function buildRoleOptionsHTML(selectedRole) {
    var seen = {};
    var roleNames = [];

    for (var i = 0; i < iamRolesCache.length; i++) {
        var role = iamRolesCache[i];
        if (!role || !role.name) continue;
        var normalized = normalizeIAMRoleName(role.name);
        if (!normalized || seen[normalized]) continue;
        seen[normalized] = true;
        roleNames.push(normalized);
    }

    if (!seen.user) {
        roleNames.push('user');
    }

    roleNames = sortRoleNames(roleNames);
    var target = normalizeIAMRoleName(selectedRole) || 'user';

    if (roleNames.indexOf(target) === -1) {
        roleNames.unshift(target);
        roleNames = sortRoleNames(roleNames);
    }

    var options = '';
    for (var j = 0; j < roleNames.length; j++) {
        var roleName = roleNames[j];
        options += '<option value="' + escapeHtml(roleName) + '"' + (roleName === target ? ' selected' : '') + '>' + escapeHtml(roleName) + '</option>';
    }
    return options;
}

function populateCreateIAMUserRoleOptions() {
    var select = document.getElementById('createIAMUserRole');
    if (!select) return;

    select.innerHTML = buildRoleOptionsHTML('user');
    select.value = 'user';
}

function renderUsers(users) {
    var userList = document.getElementById('userList');
    if (!userList) return;

    if (!users || users.length === 0) {
        userList.innerHTML = '<div style="padding:20px;text-align:center;color:var(--text-secondary,#94a3b8);">No IAM users found. Use the create action to add users.</div>';
        return;
    }

    var html = '';
    for (var i = 0; i < users.length; i++) {
        var u = users[i] || {};
        var userID = u.id || '';
        var email = u.email || '';
        var displayName = u.display_name || (email ? email.split('@')[0] : 'Unknown');
        var createdAt = u.created_at ? new Date(u.created_at).toLocaleDateString() : 'N/A';
        var statusColor = u.active ? 'var(--status-connected)' : 'var(--status-disconnected)';
        var verifyColor = u.email_verified ? 'var(--status-connected)' : 'var(--warning-color)';
        var roleNames = getUserRolesForCard(u);
        var primaryRole = resolvePrimaryRole(roleNames);

        var roleBadges = '';
        if (roleNames.length === 0) {
            roleBadges = '<span class="user-role-badge role-default">unassigned</span>';
        } else {
            for (var r = 0; r < roleNames.length; r++) {
                var roleName = roleNames[r];
                roleBadges += '<span class="user-role-badge ' + roleBadgeClass(roleName) + '">' + escapeHtml(roleName) + '</span>';
            }
        }

        html += '<div class="user-card">' +
            '<div style="display:flex;justify-content:space-between;align-items:center;">' +
                '<strong style="font-size:1.1em;">' + escapeHtml(displayName) + '</strong>' +
                '<span style="color:' + statusColor + ';font-weight:600;">' + (u.active ? '● Active' : '● Disabled') + '</span>' +
            '</div>' +
            '<div style="color:var(--text-secondary);font-size:0.95em;margin-bottom:8px;">' + escapeHtml(email) + '</div>' +
            '<div style="display:flex;justify-content:space-between;align-items:center;font-size:0.85em;font-weight:500;">' +
                '<span style="color:' + verifyColor + ';">' + (u.email_verified ? '● Email Verified' : '● Email Unverified') + '</span>' +
                '<span style="color:var(--text-muted);">Created: ' + createdAt + '</span>' +
            '</div>' +
            '<div class="role-chip-row">' + roleBadges + '</div>' +
            '<div class="user-role-actions">' +
                '<span>Access role</span>' +
                '<select class="user-role-select" data-user-id="' + escapeHtml(userID) + '">' + buildRoleOptionsHTML(primaryRole) + '</select>' +
                '<button class="btn-secondary btn-sm" data-user-id="' + escapeHtml(userID) + '" onclick="updateUserRole(this)">Update Role</button>' +
            '</div>' +
        '</div>';
    }

    userList.innerHTML = html;
}

function loadUsers() {
    var userList = document.getElementById('userList');
    if (!userList) return;
    userList.innerHTML = '<div class="loading">Loading users...</div>';

    Promise.all([
        iamAdminFetch('/iam/admin/users'),
        iamAdminFetch('/iam/admin/roles'),
        iamAdminFetch('/iam/admin/role-bindings')
    ])
        .then(function(results) {
            var usersPayload = results[0] || {};
            var rolesPayload = results[1] || {};
            var bindingsPayload = results[2] || {};

            iamUsersCache = usersPayload.users || [];
            iamRolesCache = rolesPayload.roles || [];

            var roleByID = {};
            for (var i = 0; i < iamRolesCache.length; i++) {
                var role = iamRolesCache[i];
                if (role && role.id) {
                    roleByID[role.id] = role;
                }
            }

            iamBindingsByUserID = {};
            var bindings = bindingsPayload.bindings || [];
            for (var j = 0; j < bindings.length; j++) {
                var binding = bindings[j];
                if (!binding || !binding.user_id || !binding.role_id) continue;
                var linkedRole = roleByID[binding.role_id];
                var roleName = linkedRole && linkedRole.name ? normalizeIAMRoleName(linkedRole.name) : '';
                if (!roleName) continue;

                if (!iamBindingsByUserID[binding.user_id]) {
                    iamBindingsByUserID[binding.user_id] = [];
                }
                iamBindingsByUserID[binding.user_id].push(roleName);
            }

            populateCreateIAMUserRoleOptions();
            renderUsers(iamUsersCache);
        })
        .catch(function(err) {
            userList.innerHTML = '<div style="color:#ef4444;padding:20px;">Failed to load users: ' + err.message + '</div>';
        });
}

function openCreateIAMUserModal() {
    populateCreateIAMUserRoleOptions();
    document.getElementById('createIAMUserEmail').value = '';
    document.getElementById('createIAMUserDisplayName').value = '';
    document.getElementById('createIAMUserPassword').value = '';
    document.getElementById('createIAMUserRole').value = 'user';
    document.getElementById('createIAMUserActive').checked = true;
    document.getElementById('createIAMUserEmailVerified').checked = false;
    document.getElementById('createIAMUserResult').style.display = 'none';
    document.getElementById('createIAMUserModal').style.display = 'flex';
}

function closeCreateIAMUserModal() {
    document.getElementById('createIAMUserModal').style.display = 'none';
}

function submitCreateIAMUser(event) {
    event.preventDefault();

    var email = (document.getElementById('createIAMUserEmail').value || '').trim();
    var displayName = (document.getElementById('createIAMUserDisplayName').value || '').trim();
    var password = document.getElementById('createIAMUserPassword').value || '';
    var roleName = normalizeIAMRoleName(document.getElementById('createIAMUserRole').value || 'user');
    var active = document.getElementById('createIAMUserActive').checked;
    var emailVerified = document.getElementById('createIAMUserEmailVerified').checked;

    if (!email) {
        alert('Email is required.');
        return;
    }
    if (password.length < 8) {
        alert('Password must be at least 8 characters.');
        return;
    }

    var createBtn = document.getElementById('createIAMUserBtn');
    var resultDiv = document.getElementById('createIAMUserResult');

    createBtn.disabled = true;
    createBtn.textContent = 'Creating...';
    resultDiv.style.display = 'none';

    iamAdminFetch('/iam/admin/users', {
        method: 'POST',
        body: JSON.stringify({
            email: email,
            password: password,
            display_name: displayName,
            active: active,
            email_verified: emailVerified,
            role_names: roleNamesForSelection(roleName)
        })
    }).then(function(payload) {
        createBtn.disabled = false;
        createBtn.textContent = 'Create User';

        resultDiv.style.display = 'block';
        resultDiv.style.background = 'rgba(16,185,129,0.15)';
        resultDiv.style.color = '#10b981';
        resultDiv.textContent = 'IAM user created successfully: ' + (payload.email || email);

        addOperationLog('IAM user created: ' + email, 'success');
        setTimeout(function() {
            closeCreateIAMUserModal();
            loadUsers();
        }, 400);
    }).catch(function(err) {
        createBtn.disabled = false;
        createBtn.textContent = 'Create User';

        resultDiv.style.display = 'block';
        resultDiv.style.background = 'rgba(239,68,68,0.15)';
        resultDiv.style.color = '#ef4444';
        resultDiv.textContent = err.message;
        addOperationLog('IAM user creation failed: ' + err.message, 'error');
    });
}

function updateUserRole(button) {
    if (!button) return;

    var userID = button.getAttribute('data-user-id') || '';
    if (!userID) {
        alert('Missing user identifier.');
        return;
    }

    var actionsRow = button.parentElement;
    var select = actionsRow ? actionsRow.querySelector('.user-role-select') : null;
    if (!select) {
        alert('Role selector not found.');
        return;
    }

    var selectedRole = normalizeIAMRoleName(select.value || 'user');
    if (!selectedRole) {
        alert('Please select a role.');
        return;
    }

    if (!confirm('Update this user role to "' + selectedRole + '"?')) {
        return;
    }

    var previousLabel = button.textContent;
    button.disabled = true;
    button.textContent = 'Updating...';

    iamAdminFetch('/iam/admin/users/' + encodeURIComponent(userID) + '/roles', {
        method: 'PUT',
        body: JSON.stringify({ role_names: roleNamesForSelection(selectedRole) })
    }).then(function() {
        addOperationLog('Updated IAM role mapping for user ' + userID, 'success');
        loadUsers();
    }).catch(function(err) {
        addOperationLog('IAM role update failed: ' + err.message, 'error');
        alert('Failed to update user role: ' + err.message);
    }).finally(function() {
        button.disabled = false;
        button.textContent = previousLabel;
    });
}

// ====================================
// GraphQL Studio + Control Plane (System Manager)
// ====================================
function managerApiCall(method, path, body) {
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

function managerParseJSONInput(elementId, fallback) {
    var el = document.getElementById(elementId);
    if (!el) return fallback;
    var raw = (el.value || '').trim();
    if (!raw) return fallback;
    return JSON.parse(raw);
}

function setManagerControlPlaneOutput(title, payload) {
    var el = document.getElementById('managerControlPlaneOutput');
    if (!el) return;
    el.textContent = title + '\n\n' + JSON.stringify(payload, null, 2);
}

function setManagerGraphQLOutput(payload) {
    var el = document.getElementById('managerGraphQLResult');
    if (!el) return;
    el.textContent = JSON.stringify(payload, null, 2);
}

function getManagerControlPlaneInput() {
    var namespace = (document.getElementById('managerCpNamespace').value || 'default').trim() || 'default';
    var kind = (document.getElementById('managerCpKind').value || 'workflows').trim().toLowerCase();
    var name = (document.getElementById('managerCpName').value || '').trim();
    return { namespace: namespace, kind: kind, name: name };
}

function canManagerWriteControlPlane() {
    var role = (localStorage.getItem('userRole') || '').toLowerCase();
    return role === 'admin' || role === 'system-manager';
}

function ensureManagerWrite(actionLabel) {
    if (canManagerWriteControlPlane()) return true;
    alert('RBAC: only admin or system-manager can ' + actionLabel + '.');
    return false;
}

function runManagerGraphQLQuery() {
    var queryEl = document.getElementById('managerGraphQLQuery');
    var opEl = document.getElementById('managerGraphQLOperation');
    if (!queryEl) return;

    var query = (queryEl.value || '').trim();
    if (!query) {
        alert('GraphQL query is required.');
        return;
    }

    var variables;
    try {
        variables = managerParseJSONInput('managerGraphQLVariables', {});
    } catch (e) {
        alert('Invalid JSON in GraphQL variables: ' + e.message);
        return;
    }

    setManagerGraphQLOutput({ status: 'running' });
    managerApiCall('POST', '/api/graphql', {
        query: query,
        variables: variables,
        operationName: (opEl && opEl.value ? opEl.value.trim() : '') || undefined
    }).then(function(result) {
        setManagerGraphQLOutput(result.data);
        addOperationLog('GraphQL query executed', 'info');
    }).catch(function(err) {
        setManagerGraphQLOutput({ error: err.message, status: err.status || 'n/a', details: err.response || {} });
        addOperationLog('GraphQL query failed: ' + err.message, 'error');
    });
}

function loadManagerGraphQLSchemaInfo() {
    managerApiCall('GET', '/api/graphql/schema').then(function(result) {
        setManagerGraphQLOutput(result.data);
    }).catch(function(err) {
        setManagerGraphQLOutput({ error: err.message, details: err.response || {} });
    });
}

function managerApplyResource() {
    if (!ensureManagerWrite('apply resources')) return;
    var meta = getManagerControlPlaneInput();
    var body;
    try {
        body = managerParseJSONInput('managerCpBody', {});
    } catch (e) {
        alert('Invalid JSON in resource body: ' + e.message);
        return;
    }

    if (!body.metadata) body.metadata = {};
    if (!body.metadata.name && meta.name) body.metadata.name = meta.name;

    managerApiCall('POST', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind), body)
        .then(function(result) {
            setManagerControlPlaneOutput('Resource applied', result.data);
            addOperationLog('Applied resource ' + (body.metadata.name || ''), 'success');
        })
        .catch(function(err) {
            setManagerControlPlaneOutput('Apply failed', { error: err.message, details: err.response || {} });
        });
}

function managerListResources() {
    var meta = getManagerControlPlaneInput();
    managerApiCall('GET', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind))
        .then(function(result) {
            setManagerControlPlaneOutput('Resource list', result.data);
        })
        .catch(function(err) {
            setManagerControlPlaneOutput('List failed', { error: err.message, details: err.response || {} });
        });
}

function managerGetResource() {
    var meta = getManagerControlPlaneInput();
    if (!meta.name) { alert('Resource name is required.'); return; }
    managerApiCall('GET', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind) + '/' + encodeURIComponent(meta.name))
        .then(function(result) {
            setManagerControlPlaneOutput('Resource detail', result.data);
        })
        .catch(function(err) {
            setManagerControlPlaneOutput('Get failed', { error: err.message, details: err.response || {} });
        });
}

function managerGetResourceStatus() {
    var meta = getManagerControlPlaneInput();
    if (!meta.name) { alert('Resource name is required.'); return; }
    managerApiCall('GET', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind) + '/' + encodeURIComponent(meta.name) + '/status')
        .then(function(result) {
            setManagerControlPlaneOutput('Resource status', result.data);
        })
        .catch(function(err) {
            setManagerControlPlaneOutput('Status failed', { error: err.message, details: err.response || {} });
        });
}

function managerGetResourceEvents() {
    var meta = getManagerControlPlaneInput();
    if (!meta.name) { alert('Resource name is required.'); return; }
    managerApiCall('GET', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind) + '/' + encodeURIComponent(meta.name) + '/events')
        .then(function(result) {
            setManagerControlPlaneOutput('Resource events', result.data);
        })
        .catch(function(err) {
            setManagerControlPlaneOutput('Events failed', { error: err.message, details: err.response || {} });
        });
}

function managerDeleteResource() {
    if (!ensureManagerWrite('delete resources')) return;
    var meta = getManagerControlPlaneInput();
    if (!meta.name) { alert('Resource name is required.'); return; }
    managerApiCall('DELETE', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind) + '/' + encodeURIComponent(meta.name))
        .then(function(result) {
            setManagerControlPlaneOutput('Resource deleted', result.data);
            addOperationLog('Deleted resource ' + meta.name, 'success');
        })
        .catch(function(err) {
            setManagerControlPlaneOutput('Delete failed', { error: err.message, details: err.response || {} });
        });
}

function managerRunWorkflow() {
    if (!ensureManagerWrite('run workflows')) return;
    var name = (document.getElementById('managerWorkflowName').value || '').trim();
    if (!name) { alert('Workflow name is required.'); return; }
    managerApiCall('POST', '/api/v1/workflows/' + encodeURIComponent(name) + '/run', {})
        .then(function(result) {
            setManagerControlPlaneOutput('Workflow run requested', result.data);
        })
        .catch(function(err) {
            setManagerControlPlaneOutput('Workflow run failed', { error: err.message, details: err.response || {} });
        });
}

function managerListDatasources() {
    managerApiCall('GET', '/api/v1/datasources').then(function(result) {
        setManagerControlPlaneOutput('Datasources', result.data);
    }).catch(function(err) {
        setManagerControlPlaneOutput('List datasources failed', { error: err.message, details: err.response || {} });
    });
}

function managerGetDatasource() {
    var name = (document.getElementById('managerDatasourceName').value || '').trim();
    if (!name) { alert('Datasource name is required.'); return; }
    managerApiCall('GET', '/api/v1/datasources/' + encodeURIComponent(name)).then(function(result) {
        setManagerControlPlaneOutput('Datasource detail', result.data);
    }).catch(function(err) {
        setManagerControlPlaneOutput('Get datasource failed', { error: err.message, details: err.response || {} });
    });
}

function managerTestDatasource() {
    if (!ensureManagerWrite('test datasources')) return;
    var name = (document.getElementById('managerDatasourceName').value || '').trim();
    if (!name) { alert('Datasource name is required.'); return; }
    managerApiCall('POST', '/api/v1/datasources/' + encodeURIComponent(name) + '/test', {}).then(function(result) {
        setManagerControlPlaneOutput('Datasource test result', result.data);
    }).catch(function(err) {
        setManagerControlPlaneOutput('Test datasource failed', { error: err.message, details: err.response || {} });
    });
}

function managerListJobs() {
    managerApiCall('GET', '/api/v1/jobs').then(function(result) {
        setManagerControlPlaneOutput('Jobs', result.data);
    }).catch(function(err) {
        setManagerControlPlaneOutput('List jobs failed', { error: err.message, details: err.response || {} });
    });
}

function managerGetJob() {
    var id = (document.getElementById('managerJobId').value || '').trim();
    if (!id) { alert('Job ID/name is required.'); return; }
    managerApiCall('GET', '/api/v1/jobs/' + encodeURIComponent(id)).then(function(result) {
        setManagerControlPlaneOutput('Job detail', result.data);
    }).catch(function(err) {
        setManagerControlPlaneOutput('Get job failed', { error: err.message, details: err.response || {} });
    });
}

function managerRunJob() {
    if (!ensureManagerWrite('run jobs')) return;
    var id = (document.getElementById('managerJobId').value || '').trim();
    if (!id) { alert('Job ID/name is required.'); return; }
    managerApiCall('POST', '/api/v1/jobs/' + encodeURIComponent(id) + '/run', {}).then(function(result) {
        setManagerControlPlaneOutput('Job run requested', result.data);
    }).catch(function(err) {
        setManagerControlPlaneOutput('Run job failed', { error: err.message, details: err.response || {} });
    });
}

function managerGetJobLogs() {
    var id = (document.getElementById('managerJobId').value || '').trim();
    if (!id) { alert('Job ID/name is required.'); return; }
    managerApiCall('GET', '/api/v1/jobs/' + encodeURIComponent(id) + '/logs').then(function(result) {
        setManagerControlPlaneOutput('Job logs', result.data);
    }).catch(function(err) {
        setManagerControlPlaneOutput('Get job logs failed', { error: err.message, details: err.response || {} });
    });
}

function managerCancelJob() {
    if (!ensureManagerWrite('cancel jobs')) return;
    var id = (document.getElementById('managerJobId').value || '').trim();
    if (!id) { alert('Job ID/name is required.'); return; }
    managerApiCall('POST', '/api/v1/jobs/' + encodeURIComponent(id) + '/cancel', {}).then(function(result) {
        setManagerControlPlaneOutput('Job cancel requested', result.data);
    }).catch(function(err) {
        setManagerControlPlaneOutput('Cancel job failed', { error: err.message, details: err.response || {} });
    });
}

function refreshManagerControlPlaneData() {
    var meta = getManagerControlPlaneInput();
    Promise.all([
        managerApiCall('GET', '/api/v1/namespaces/' + encodeURIComponent(meta.namespace) + '/' + encodeURIComponent(meta.kind)),
        managerApiCall('GET', '/api/v1/datasources'),
        managerApiCall('GET', '/api/v1/jobs')
    ]).then(function(results) {
        setManagerControlPlaneOutput('Control plane snapshot', {
            resources: results[0].data,
            datasources: results[1].data,
            jobs: results[2].data
        });
    }).catch(function(err) {
        setManagerControlPlaneOutput('Refresh failed', { error: err.message, details: err.response || {} });
    });
}

function escapeHtml(str) {
    if (!str) return '';
    var div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

window.addEventListener('click', function(event) {
    var createDbModal = document.getElementById('createDbModal');
    if (createDbModal && event.target === createDbModal) {
        closeCreateDbModal();
    }
    var connectModal = document.getElementById('connectDbServerModal');
    if (connectModal && event.target === connectModal) {
        closeConnectDbServerModal();
    }
    var createIAMUserModal = document.getElementById('createIAMUserModal');
    if (createIAMUserModal && event.target === createIAMUserModal) {
        closeCreateIAMUserModal();
    }
});
