const VERSION_LINEAGE_API_BASE = (typeof window.resolveBackendURL === 'function') ? window.resolveBackendURL() : (window.BACKEND_URL || 'http://localhost:8000');

function versionLineageURL(path) {
    if (!path) return VERSION_LINEAGE_API_BASE;
    if (/^https?:\/\//i.test(path)) return path;
    return VERSION_LINEAGE_API_BASE + path;
}

function versionLineageHeaders() {
    return (typeof getAuthHeaders === 'function') ? getAuthHeaders() : { 'Content-Type': 'application/json' };
}

function renderVersionLineage(data) {
    const el = document.getElementById('versionLineageResponse');
    if (el) {
        el.textContent = JSON.stringify(data, null, 2);
    }
}

async function vlFetch(path) {
    const response = await fetch(versionLineageURL(path), { headers: versionLineageHeaders() });
    const data = await response.json().catch(function() { return { error: 'Invalid JSON' }; });
    if (!response.ok) {
        throw new Error(data.error || data.message || response.statusText);
    }
    return data;
}

function bindVersionLineageForms() {
    const versionHistoryForm = document.getElementById('versionHistoryForm');
    const versionDiffForm = document.getElementById('versionDiffForm');
    const lineageGraphForm = document.getElementById('lineageGraphForm');
    const lineageImpactForm = document.getElementById('lineageImpactForm');
    const traceSearchForm = document.getElementById('traceSearchForm');

    if (versionHistoryForm) {
        versionHistoryForm.addEventListener('submit', async function(event) {
            event.preventDefault();
            try {
                const type = document.getElementById('vhType').value;
                const id = document.getElementById('vhId').value;
                const result = await vlFetch('/api/v1/versioning/history/' + encodeURIComponent(type) + '/' + encodeURIComponent(id));
                renderVersionLineage({ mode: 'history', result: result });
            } catch (err) {
                renderVersionLineage({ error: err.message });
            }
        });
    }

    if (versionDiffForm) {
        versionDiffForm.addEventListener('submit', async function(event) {
            event.preventDefault();
            try {
                const type = document.getElementById('vdType').value;
                const id = document.getElementById('vdId').value;
                const from = document.getElementById('vdFrom').value;
                const to = document.getElementById('vdTo').value;
                const path = '/api/v1/versioning/diff/' + encodeURIComponent(type) + '/' + encodeURIComponent(id) + '?from=' + encodeURIComponent(from) + '&to=' + encodeURIComponent(to);
                const result = await vlFetch(path);
                renderVersionLineage({ mode: 'diff', result: result });
            } catch (err) {
                renderVersionLineage({ error: err.message });
            }
        });
    }

    if (lineageGraphForm) {
        lineageGraphForm.addEventListener('submit', async function(event) {
            event.preventDefault();
            try {
                const type = document.getElementById('lgType').value;
                const id = document.getElementById('lgId').value;
                const result = await vlFetch('/api/v1/lineage/' + encodeURIComponent(type) + '/' + encodeURIComponent(id));
                renderVersionLineage({ mode: 'lineage-graph', result: result });
            } catch (err) {
                renderVersionLineage({ error: err.message });
            }
        });
    }

    if (lineageImpactForm) {
        lineageImpactForm.addEventListener('submit', async function(event) {
            event.preventDefault();
            try {
                const type = document.getElementById('liType').value;
                const id = document.getElementById('liId').value;
                const result = await vlFetch('/api/v1/lineage/impact/' + encodeURIComponent(type) + '/' + encodeURIComponent(id));
                renderVersionLineage({ mode: 'lineage-impact', result: result });
            } catch (err) {
                renderVersionLineage({ error: err.message });
            }
        });
    }

    if (traceSearchForm) {
        traceSearchForm.addEventListener('submit', async function(event) {
            event.preventDefault();
            try {
                const service = document.getElementById('tsService').value;
                const limit = document.getElementById('tsLimit').value || '20';
                const path = '/api/v1/tracing/traces/search?service=' + encodeURIComponent(service) + '&limit=' + encodeURIComponent(limit);
                const result = await vlFetch(path);
                renderVersionLineage({ mode: 'trace-search', result: result });
            } catch (err) {
                renderVersionLineage({ error: err.message });
            }
        });
    }
}

window.addEventListener('DOMContentLoaded', function() {
    if (window.location.pathname !== '/lineage-version') {
        return;
    }
    bindVersionLineageForms();
});
