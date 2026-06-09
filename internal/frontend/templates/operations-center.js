const OPERATIONS_API_BASE = (typeof window.resolveBackendURL === 'function') ? window.resolveBackendURL() : (window.BACKEND_URL || 'http://localhost:8000');

function operationsURL(path) {
    if (!path) return OPERATIONS_API_BASE;
    if (/^https?:\/\//i.test(path)) return path;
    return OPERATIONS_API_BASE + path;
}

function operationsHeaders() {
    return (typeof getAuthHeaders === 'function') ? getAuthHeaders() : { 'Content-Type': 'application/json' };
}

function renderOpsJSON(data) {
    const el = document.getElementById('operationsResponse');
    if (el) {
        el.textContent = JSON.stringify(data, null, 2);
    }
}

function toArray(payload, key) {
    if (Array.isArray(payload)) return payload;
    if (payload && Array.isArray(payload[key])) return payload[key];
    return [];
}

async function opsFetch(path, method, body) {
    const response = await fetch(operationsURL(path), {
        method: method || 'GET',
        headers: operationsHeaders(),
        body: body ? JSON.stringify(body) : undefined
    });
    const data = await response.json().catch(function() { return { error: 'Invalid JSON' }; });
    if (!response.ok) {
        throw new Error(data.error || data.message || response.statusText);
    }
    return data;
}

function renderList(targetId, items, titleField, metaBuilder) {
    const container = document.getElementById(targetId);
    if (!container) return;
    container.innerHTML = '';
    if (!items.length) {
        container.innerHTML = '<div class="platform-item">No data.</div>';
        return;
    }

    items.forEach(function(item) {
        const node = document.createElement('div');
        node.className = 'platform-item';
        const title = item[titleField] || item.id || 'item';
        node.innerHTML = '<strong>' + title + '</strong><div class="meta">' + metaBuilder(item) + '</div>';
        container.appendChild(node);
    });
}

async function loadOperationsData() {
    try {
        const [notif, alertsData, anomaliesData, webhooksData, streamsData, exportsData, bulkData] = await Promise.all([
            opsFetch('/api/notifications/status').catch(function(err) { return { error: err.message }; }),
            opsFetch('/api/v1/netintel/alerts').catch(function() { return { alerts: [] }; }),
            opsFetch('/api/v1/netintel/anomalies').catch(function() { return { anomalies: [] }; }),
            opsFetch('/api/v1/webhooks').catch(function() { return { webhooks: [] }; }),
            opsFetch('/api/v1/streams').catch(function() { return { streams: [] }; }),
            opsFetch('/api/v1/exports').catch(function() { return { exports: [] }; }),
            opsFetch('/api/v1/bulk/operations').catch(function() { return { operations: [] }; })
        ]);

        const alerts = toArray(alertsData, 'alerts');
        const anomalies = toArray(anomaliesData, 'anomalies');
        const webhooks = toArray(webhooksData, 'webhooks');
        const streams = toArray(streamsData, 'streams');
        const exportsList = toArray(exportsData, 'exports');
        const bulkOps = toArray(bulkData, 'operations');

        const alertsMetric = document.getElementById('metricAlerts');
        const anomaliesMetric = document.getElementById('metricAnomalies');
        const webhooksMetric = document.getElementById('metricWebhooks');
        const streamsMetric = document.getElementById('metricStreams');

        if (alertsMetric) alertsMetric.textContent = String(alerts.length);
        if (anomaliesMetric) anomaliesMetric.textContent = String(anomalies.length);
        if (webhooksMetric) webhooksMetric.textContent = String(webhooks.length);
        if (streamsMetric) streamsMetric.textContent = String(streams.length);

        renderList('opsWebhookList', webhooks, 'name', function(item) {
            return 'id: ' + (item.id || '-') + ' | active: ' + String(item.active !== false);
        });

        renderList('opsStreamList', streams, 'id', function(item) {
            return 'active: ' + String(item.active !== false) + ' | created: ' + (item.createdAt || '-');
        });

        renderList('opsExportList', exportsList, 'name', function(item) {
            return 'status: ' + (item.status || '-') + ' | progress: ' + (item.progress || 0) + '%';
        });

        renderList('opsBulkList', bulkOps, 'id', function(item) {
            return 'status: ' + (item.status || '-') + ' | total: ' + (item.totalItems || 0);
        });

        renderOpsJSON({
            notifications: notif,
            alerts: alerts,
            anomalies: anomalies
        });
    } catch (err) {
        renderOpsJSON({ error: err.message });
    }
}

function initOperationsForms() {
    const webhookForm = document.getElementById('webhookCreateForm');
    const streamForm = document.getElementById('streamCreateForm');

    if (webhookForm) {
        webhookForm.addEventListener('submit', async function(event) {
            event.preventDefault();
            try {
                const payload = {
                    name: document.getElementById('webhookName').value,
                    url: document.getElementById('webhookURL').value,
                    events: ['etl.completed', 'etl.failed'],
                    secret: 'ops-center'
                };
                const result = await opsFetch('/api/v1/webhooks', 'POST', payload);
                renderOpsJSON(result);
                loadOperationsData();
            } catch (err) {
                renderOpsJSON({ error: err.message });
            }
        });
    }

    if (streamForm) {
        streamForm.addEventListener('submit', async function(event) {
            event.preventDefault();
            try {
                const payload = {
                    tenantId: document.getElementById('streamTenantId').value,
                    topic: document.getElementById('streamTopic').value
                };
                const result = await opsFetch('/api/v1/streams', 'POST', payload);
                renderOpsJSON(result);
                loadOperationsData();
            } catch (err) {
                renderOpsJSON({ error: err.message });
            }
        });
    }
}

window.addEventListener('DOMContentLoaded', function() {
    if (window.location.pathname !== '/operations-center') {
        return;
    }
    initOperationsForms();
    loadOperationsData();
});
