// Manager Portal JavaScript
// View + Edit only — NO create, NO delete

const BACKEND_URL = (typeof window.resolveBackendURL === 'function') ? window.resolveBackendURL() : (window.BACKEND_URL || 'http://localhost:8000');

function mgrBuildEmbedURL(path) {
    const sep = path.indexOf('?') >= 0 ? '&' : '?';
    return path + sep + 'embed=1&from=manager&v=3';
}

function mgrNormalizeEmbeddedFrame(frame) {
    if (!frame) return;
    try {
        const doc = frame.contentDocument || (frame.contentWindow && frame.contentWindow.document);
        if (!doc) return;

        const nav = doc.querySelector('.navbar');
        if (nav) nav.style.display = 'none';

        const footer = doc.querySelector('.footer');
        if (footer) footer.style.display = 'none';

        const main = doc.querySelector('.main-content');
        if (main) {
            main.style.maxWidth = '100%';
            main.style.margin = '0';
            main.style.padding = '0';
        }
    } catch (err) {
        console.warn('Unable to normalize embedded frame:', err);
    }
}

function mgrInitEmbeddedFrames() {
    const frames = [
        { id: 'mgrGisFrame', path: '/gis' },
        { id: 'mgrAnalyticsFrame', path: '/analytics' },
        { id: 'mgrNetintelFrame', path: '/netintel' }
    ];

    frames.forEach(function(item) {
        const frame = document.getElementById(item.id);
        if (!frame) return;

        if (!frame.dataset.embedLoadHooked) {
            frame.addEventListener('load', function() {
                mgrNormalizeEmbeddedFrame(frame);
            });
            frame.dataset.embedLoadHooked = '1';
        }

        const target = mgrBuildEmbedURL(item.path);
        if (frame.getAttribute('src') !== target) {
            frame.setAttribute('src', target);
        } else {
            mgrNormalizeEmbeddedFrame(frame);
        }
    });
}

// ===================== TAB SWITCHING =====================
function mgr_switchTab(tabId) {
    document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));

    const tab = document.getElementById(tabId);
    if (tab) tab.classList.add('active');

    // Find the button that triggered this tab and mark it active
    document.querySelectorAll('.tab-btn').forEach(btn => {
        if (btn.getAttribute('onclick') && btn.getAttribute('onclick').includes(tabId)) {
            btn.classList.add('active');
        }
    });

    // Lazy-load content when tab becomes visible
    if (tabId === 'mgr-apis') mgrLoadAPIs();
    if (tabId === 'mgr-dashboards') mgrLoadDashboards();
    if (tabId === 'mgr-gis' || tabId === 'mgr-analytics' || tabId === 'mgr-netintel') {
        mgrInitEmbeddedFrames();
    }
}

// ===================== INIT =====================
window.addEventListener('DOMContentLoaded', function () {
    const userNameEl = document.getElementById('managerUserName');
    if (userNameEl) {
        userNameEl.textContent = localStorage.getItem('userName') || 'Manager';
    }
    mgrInitEmbeddedFrames();
    mgrLoadAPIs();
});

// ===================== HELPERS =====================
function mgrAuthHeaders() {
    const token = localStorage.getItem('authToken');
    return token ? { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' } : { 'Content-Type': 'application/json' };
}

// ===================== API LIST =====================
async function mgrLoadAPIs() {
    const listEl = document.getElementById('mgrApiList');
    const summaryEl = document.getElementById('mgrApiSummary');
    const category = document.getElementById('mgrApiCategoryFilter')?.value || '';
    const status = document.getElementById('mgrApiStatusFilter')?.value || '';

    if (listEl) listEl.innerHTML = '<div class="loading">Loading APIs...</div>';

    try {
        let url = BACKEND_URL + '/api/v1/builder/apis?limit=100';
        if (category) url += '&category=' + encodeURIComponent(category);
        if (status) url += '&status=' + encodeURIComponent(status);

        const resp = await fetch(url, { headers: mgrAuthHeaders() });
        const data = await resp.json();

        const apis = data.data || data.apis || data || [];

        if (summaryEl) {
            summaryEl.innerHTML = `
                <div class="summary-card"><span class="summary-value">${apis.length}</span><span class="summary-label">Total APIs</span></div>
                <div class="summary-card"><span class="summary-value">${apis.filter(a => a.status === 'active').length}</span><span class="summary-label">Active</span></div>
                <div class="summary-card"><span class="summary-value">${apis.filter(a => a.status === 'inactive').length}</span><span class="summary-label">Inactive</span></div>
            `;
        }

        if (!apis.length) {
            listEl.innerHTML = '<div class="empty-state"><p>No APIs found.</p></div>';
            return;
        }

        listEl.innerHTML = apis.map(api => `
            <div class="api-card" id="api-card-${api.id}">
                <div class="api-card-header">
                    <div class="api-card-title">
                        <span class="method-badge method-${(api.method || 'GET').toLowerCase()}">${api.method || 'GET'}</span>
                        <strong>${escapeHtml(api.name || '')}</strong>
                        <code class="api-endpoint">${escapeHtml(api.path || api.endpoint || '')}</code>
                    </div>
                    <div class="api-card-actions">
                        <span class="status-badge status-${api.status || 'draft'}">${api.status || 'draft'}</span>
                        <button class="btn-sm btn-secondary" onclick="mgrOpenTestModal('${escapeAttr(api.id)}', '${escapeAttr(api.method || 'GET')}', '${escapeAttr(api.path || api.endpoint || '')}')">🧪 Test</button>
                        <button class="btn-sm btn-primary" onclick="mgrOpenEditAPIModal('${escapeAttr(api.id)}')">✏️ Edit</button>
                    </div>
                </div>
                ${api.description ? `<p class="api-description">${escapeHtml(api.description)}</p>` : ''}
                <div class="api-meta">
                    <span>📁 ${escapeHtml(api.category || 'custom')}</span>
                    <span>🗄️ ${escapeHtml(api.source_database || 'n/a')}</span>
                    <span>🖧 ${escapeHtml(api.source_server || 'default')}</span>
                    ${api.created_at ? `<span>🕐 ${new Date(api.created_at).toLocaleDateString()}</span>` : ''}
                </div>
            </div>
        `).join('');

    } catch (err) {
        console.error('Failed to load APIs:', err);
        if (listEl) listEl.innerHTML = '<div class="error-state"><p>⚠️ Failed to load APIs.</p></div>';
    }
}

// ===================== EDIT API =====================
let _mgrEditingAPI = null;

async function mgrOpenEditAPIModal(apiId) {
    try {
        const resp = await fetch(`${BACKEND_URL}/api/v1/builder/apis/${apiId}`, { headers: mgrAuthHeaders() });
        const data = await resp.json();
        const api = data.data || data;

        _mgrEditingAPI = api;

        document.getElementById('mgrEditAPIId').value = api.id || apiId;
        document.getElementById('mgrEditAPIName').value = api.name || '';
        document.getElementById('mgrEditAPIDescription').value = api.description || '';
        document.getElementById('mgrEditAPIEndpoint').value = api.path || api.endpoint || '';
        document.getElementById('mgrEditAPIMethod').value = api.method || 'GET';
        document.getElementById('mgrEditAPICategory').value = api.category || 'custom';
        document.getElementById('mgrEditAPIStatus').value = api.status || 'active';

        document.getElementById('mgrEditAPIModal').style.display = 'flex';
    } catch (err) {
        console.error('Failed to fetch API:', err);
        alert('Failed to load API details.');
    }
}

function mgrCloseEditAPIModal() {
    document.getElementById('mgrEditAPIModal').style.display = 'none';
    _mgrEditingAPI = null;
}

async function mgrSaveAPIEdit() {
    const id = document.getElementById('mgrEditAPIId').value;
    if (!id) return;

    const payload = {
        name: document.getElementById('mgrEditAPIName').value,
        description: document.getElementById('mgrEditAPIDescription').value,
        path: document.getElementById('mgrEditAPIEndpoint').value,
        method: document.getElementById('mgrEditAPIMethod').value,
        category: document.getElementById('mgrEditAPICategory').value,
        status: document.getElementById('mgrEditAPIStatus').value,
    };

    try {
        const resp = await fetch(`${BACKEND_URL}/api/v1/builder/apis/${id}`, {
            method: 'PUT',
            headers: mgrAuthHeaders(),
            body: JSON.stringify(payload),
        });

        if (resp.ok) {
            mgrCloseEditAPIModal();
            mgrLoadAPIs();
        } else {
            const err = await resp.json().catch(() => ({}));
            alert('Failed to save: ' + (err.message || resp.statusText));
        }
    } catch (err) {
        console.error('Save API error:', err);
        alert('Network error saving API.');
    }
}

// ===================== TEST API =====================
let _mgrTestAPIId = null;

function mgrOpenTestModal(apiId, method, endpoint) {
    _mgrTestAPIId = apiId;
    document.getElementById('mgrTestMethod').textContent = method;
    document.getElementById('mgrTestEndpoint').value = BACKEND_URL + endpoint;
    document.getElementById('mgrTestBody').value = '';
    document.getElementById('mgrTestResult').style.display = 'none';
    document.getElementById('mgrTestAPIModal').style.display = 'flex';
}

function mgrCloseTestAPIModal() {
    document.getElementById('mgrTestAPIModal').style.display = 'none';
    _mgrTestAPIId = null;
}

async function mgrRunAPITest() {
    const method = document.getElementById('mgrTestMethod').textContent;
    const endpoint = document.getElementById('mgrTestEndpoint').value;
    const bodyText = document.getElementById('mgrTestBody').value.trim();
    const resultEl = document.getElementById('mgrTestResult');
    const resultBodyEl = document.getElementById('mgrTestResultBody');

    resultBodyEl.textContent = 'Running...';
    resultEl.style.display = 'block';

    try {
        const opts = { method, headers: mgrAuthHeaders() };
        if (bodyText && ['POST', 'PUT', 'PATCH'].includes(method)) {
            opts.body = bodyText;
        }
        const resp = await fetch(endpoint, opts);
        const text = await resp.text();
        let display = text;
        try { display = JSON.stringify(JSON.parse(text), null, 2); } catch (_) {}
        resultBodyEl.textContent = `HTTP ${resp.status} ${resp.statusText}\n\n${display}`;
    } catch (err) {
        resultBodyEl.textContent = 'Error: ' + err.message;
    }
}

// ===================== DASHBOARDS =====================
async function mgrLoadDashboards() {
    const listEl = document.getElementById('mgrDashboardList');
    if (!listEl) return;
    listEl.innerHTML = '<div class="loading">Loading dashboards...</div>';

    try {
        const resp = await fetch(BACKEND_URL + '/api/v1/builder/csv-uploads?limit=100', { headers: mgrAuthHeaders() });
        const data = await resp.json();
        const dashboards = data.data || data.uploads || data || [];

        if (!dashboards.length) {
            listEl.innerHTML = '<div class="empty-state"><p>No dashboards found.</p></div>';
            return;
        }

        listEl.innerHTML = `<div class="api-builder-list">` + dashboards.map(db => `
            <div class="api-card">
                <div class="api-card-header">
                    <div class="api-card-title">
                        <span class="status-badge status-active">📊</span>
                        <strong>${escapeHtml(db.original_name || db.filename || db.name || 'Dashboard')}</strong>
                    </div>
                    <div class="api-card-actions">
                        ${db.dashboard_url || db.id ? `<a class="btn-sm btn-secondary" href="${escapeAttr(db.dashboard_url || '/admin-dashboard?id=' + db.id)}" target="_blank">👁 View</a>` : ''}
                    </div>
                </div>
                <div class="api-meta">
                    ${db.created_at ? `<span>🕐 ${new Date(db.created_at).toLocaleDateString()}</span>` : ''}
                    ${db.row_count ? `<span>📋 ${db.row_count} rows</span>` : ''}
                    ${db.status ? `<span class="status-badge status-${db.status}">${db.status}</span>` : ''}
                </div>
            </div>
        `).join('') + `</div>`;

    } catch (err) {
        console.error('Failed to load dashboards:', err);
        if (listEl) listEl.innerHTML = '<div class="error-state"><p>⚠️ Failed to load dashboards.</p></div>';
    }
}

// ===================== UTILS =====================
function escapeHtml(str) {
    if (!str) return '';
    return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function escapeAttr(str) {
    if (!str) return '';
    return String(str).replace(/'/g, '&#39;').replace(/"/g, '&quot;');
}

// Close modals when clicking outside
window.addEventListener('click', function (e) {
    const modals = ['mgrEditAPIModal', 'mgrTestAPIModal'];
    modals.forEach(id => {
        const modal = document.getElementById(id);
        if (modal && e.target === modal) modal.style.display = 'none';
    });
});
