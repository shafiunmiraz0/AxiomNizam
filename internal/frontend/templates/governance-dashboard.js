const GOVERNANCE_API_BASE = (typeof window.resolveBackendURL === 'function') ? window.resolveBackendURL() : (window.BACKEND_URL || 'http://localhost:8000');

function governanceURL(path) {
    if (!path) return GOVERNANCE_API_BASE;
    if (/^https?:\/\//i.test(path)) return path;
    return GOVERNANCE_API_BASE + path;
}

function governanceHeaders() {
    return (typeof getAuthHeaders === 'function') ? getAuthHeaders() : { 'Content-Type': 'application/json' };
}

function renderGovernanceJSON(targetId, data) {
    const el = document.getElementById(targetId);
    if (el) {
        el.textContent = JSON.stringify(data, null, 2);
    }
}

function safeArrayPayload(data, key) {
    if (Array.isArray(data)) return data;
    if (data && Array.isArray(data[key])) return data[key];
    return [];
}

function renderTenantList(items) {
    const container = document.getElementById('tenantList');
    if (!container) return;
    container.innerHTML = '';
    if (!items.length) {
        container.innerHTML = '<div class="platform-item">No tenants found.</div>';
        return;
    }

    items.forEach(function(item) {
        const node = document.createElement('div');
        node.className = 'platform-item';
        node.innerHTML = '<strong>' + (item.name || item.id || 'tenant') + '</strong>' +
            '<div class="meta">id: ' + (item.id || '-') + ' | owner: ' + (item.owner || '-') + '</div>';
        container.appendChild(node);
    });
}

function renderRoleList(items) {
    const container = document.getElementById('roleList');
    if (!container) return;
    container.innerHTML = '';
    if (!items.length) {
        container.innerHTML = '<div class="platform-item">No roles found.</div>';
        return;
    }

    items.forEach(function(item) {
        const node = document.createElement('div');
        node.className = 'platform-item';
        node.innerHTML = '<strong>' + (item.name || item.id || 'role') + '</strong>' +
            '<div class="meta">tenant: ' + (item.tenantId || '-') + ' | active: ' + String(item.isActive !== false) + '</div>';
        container.appendChild(node);
    });
}

async function governanceFetch(path, method, body) {
    const response = await fetch(governanceURL(path), {
        method: method || 'GET',
        headers: governanceHeaders(),
        body: body ? JSON.stringify(body) : undefined
    });

    const data = await response.json().catch(function() {
        return { error: 'Invalid JSON response' };
    });

    if (!response.ok) {
        throw new Error(data.error || data.message || response.statusText);
    }

    return data;
}

async function loadGovernanceData() {
    try {
        const tenants = await governanceFetch('/api/v1/tenants');
        const roles = await governanceFetch('/api/v1/rbac/roles');
        renderTenantList(safeArrayPayload(tenants, 'tenants'));
        renderRoleList(safeArrayPayload(roles, 'roles'));
    } catch (err) {
        renderGovernanceJSON('governanceResponse', { error: err.message });
    }
}

function initGovernanceForms() {
    const tenantForm = document.getElementById('tenantCreateForm');
    const roleForm = document.getElementById('roleCreateForm');
    const checkForm = document.getElementById('permissionCheckForm');

    if (tenantForm) {
        tenantForm.addEventListener('submit', async function(event) {
            event.preventDefault();
            try {
                const payload = {
                    name: document.getElementById('tenantName').value,
                    owner: document.getElementById('tenantOwner').value,
                    tier: document.getElementById('tenantTier').value,
                    isolationLevel: document.getElementById('tenantIsolation').value
                };
                const result = await governanceFetch('/api/v1/tenants', 'POST', payload);
                renderGovernanceJSON('governanceResponse', result);
                loadGovernanceData();
            } catch (err) {
                renderGovernanceJSON('governanceResponse', { error: err.message });
            }
        });
    }

    if (roleForm) {
        roleForm.addEventListener('submit', async function(event) {
            event.preventDefault();
            try {
                const payload = {
                    tenantId: document.getElementById('roleTenantId').value,
                    name: document.getElementById('roleName').value,
                    description: document.getElementById('roleDescription').value
                };
                const result = await governanceFetch('/api/v1/rbac/roles', 'POST', payload);
                renderGovernanceJSON('governanceResponse', result);
                loadGovernanceData();
            } catch (err) {
                renderGovernanceJSON('governanceResponse', { error: err.message });
            }
        });
    }

    if (checkForm) {
        checkForm.addEventListener('submit', async function(event) {
            event.preventDefault();
            try {
                const payload = {
                    tenantId: document.getElementById('checkTenantId').value,
                    principalId: document.getElementById('checkPrincipalId').value,
                    resource: document.getElementById('checkResource').value,
                    action: document.getElementById('checkAction').value
                };
                const result = await governanceFetch('/api/v1/rbac/permissions/check', 'POST', payload);
                renderGovernanceJSON('governanceResponse', result);
            } catch (err) {
                renderGovernanceJSON('governanceResponse', { error: err.message });
            }
        });
    }
}

window.addEventListener('DOMContentLoaded', function() {
    if (window.location.pathname !== '/governance') {
        return;
    }
    initGovernanceForms();
    loadGovernanceData();
});
