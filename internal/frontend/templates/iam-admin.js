// ─────────────────────────────────────────────────────────────────────────────
// IAM Admin Console — JavaScript
// ─────────────────────────────────────────────────────────────────────────────

const IAM_API = (() => {
    let base = (typeof window.resolveBackendURL === 'function') ? window.resolveBackendURL() : String(window.BACKEND_URL || '').trim();
    if (!base) {
        const browserHost = String(window.location.hostname || '').toLowerCase();
        if (browserHost && browserHost !== 'localhost' && browserHost !== '127.0.0.1' && browserHost !== '0.0.0.0') {
            const protocol = window.location.protocol || 'https:';
            if (browserHost.indexOf('axiomnizam.') === 0) {
                base = protocol + '//axiomnizam-platform.' + browserHost.substring('axiomnizam.'.length);
            } else {
                base = protocol + '//' + browserHost;
            }
        } else {
            base = 'http://localhost:8000';
        }
    }
    if (base.endsWith('/')) base = base.slice(0, -1);
    return base;
})();

function readIAMCookie(name) {
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

function resolvePlatformAuthToken() {
    return localStorage.getItem('authToken') || readIAMCookie('authToken') || '';
}

function resolvePlatformRefreshToken() {
    return localStorage.getItem('refreshToken') || '';
}

// IAM-specific auth token (separate from platform auth)
let iamToken = localStorage.getItem('iamToken') || resolvePlatformAuthToken();
let iamRefreshToken = localStorage.getItem('iamRefreshToken') || resolvePlatformRefreshToken();
let iamServiceAccessInfo = null;
let iamLastGeneratedTokenResponse = null;
let iamSelectedRealm = '';
let iamPermissionHintShown = false;

function clearIAMSession() {
    iamToken = '';
    iamRefreshToken = '';
    localStorage.removeItem('iamToken');
    localStorage.removeItem('iamRefreshToken');
    iamPermissionHintShown = false;
    updateIAMPermissionBanner(false, '');
    updateIAMLoginBanner();
}

// ── Helpers ──────────────────────────────────────────────────────────────────

function iamHeaders() {
    const h = { 'Content-Type': 'application/json' };

    if (!iamToken) {
        iamToken = localStorage.getItem('iamToken') || resolvePlatformAuthToken();
    }

    if (iamToken) h['Authorization'] = 'Bearer ' + iamToken;
    return h;
}

async function iamFetch(path, opts) {
    opts = opts || {};
    opts.headers = Object.assign(iamHeaders(), opts.headers || {});
    let resp = await fetch(IAM_API + path, opts);
    if (resp.status === 401 && iamRefreshToken) {
        const refreshed = await tryIAMRefresh();
        if (refreshed) {
            opts.headers['Authorization'] = 'Bearer ' + iamToken;
            resp = await fetch(IAM_API + path, opts);
        }
    }

    if (resp.status === 401) {
        clearIAMSession();
    }

    return resp;
}

async function tryIAMRefresh() {
    try {
        const resp = await fetch(IAM_API + '/iam/auth/refresh', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ refresh_token: iamRefreshToken })
        });
        if (!resp.ok) {
            clearIAMSession();
            return false;
        }
        const data = await resp.json();
        iamToken = data.access_token || '';
        if (!iamToken) {
            clearIAMSession();
            return false;
        }
        iamRefreshToken = data.refresh_token || iamRefreshToken;
        localStorage.setItem('iamToken', iamToken);
        localStorage.setItem('iamRefreshToken', iamRefreshToken);
        return true;
    } catch (_) {
        clearIAMSession();
        return false;
    }
}

function showIAMToast(message, isError) {
    const el = document.createElement('div');
    el.textContent = message;
    el.style.cssText = 'position:fixed;top:20px;right:20px;z-index:100000;padding:12px 24px;border-radius:8px;color:#fff;font-weight:600;font-size:.9rem;box-shadow:0 4px 12px rgba(0,0,0,.3);transition:opacity .3s;' +
        (isError ? 'background:#e74c3c;' : 'background:#27ae60;');
    document.body.appendChild(el);
    setTimeout(() => { el.style.opacity = '0'; setTimeout(() => el.remove(), 300); }, 3000);
}

async function copyToClipboard(text) {
    const value = (text || '').trim();
    if (!value) return false;
    try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
            await navigator.clipboard.writeText(value);
            return true;
        }
    } catch (_) {}

    const ta = document.createElement('textarea');
    ta.value = value;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    return ok;
}

async function copyTextFromElement(id) {
    const el = document.getElementById(id);
    if (!el) {
        showIAMToast('Copy source not found', true);
        return;
    }
    const value = typeof el.value === 'string' ? el.value : el.textContent;
    const ok = await copyToClipboard(value || '');
    showIAMToast(ok ? 'Copied' : 'Copy failed', !ok);
}

function setFieldText(id, value) {
    const el = document.getElementById(id);
    if (!el) return;
    if (typeof el.value === 'string') {
        el.value = value || '';
    } else {
        el.textContent = value || '–';
    }
}

function getServiceTokenEndpoint() {
    const endpoints = iamServiceAccessInfo && iamServiceAccessInfo.endpoints ? iamServiceAccessInfo.endpoints : {};
    return endpoints.keycloak_token || endpoints.iam_token || (IAM_API + '/oauth/token');
}

function buildClientCredentialsSnippet(clientID, clientSecret, scope) {
    let cmd = "curl --request POST '" + getServiceTokenEndpoint() + "'" +
        " --header 'Content-Type: application/x-www-form-urlencoded'" +
        " --data-urlencode 'grant_type=client_credentials'" +
        " --data-urlencode 'client_id=" + (clientID || '') + "'" +
        " --data-urlencode 'client_secret=" + (clientSecret || '') + "'";
    if (scope) {
        cmd += " --data-urlencode 'scope=" + scope + "'";
    }
    return cmd;
}

function applyServiceRealm() {
    const input = document.getElementById('iamServiceRealmInput');
    const requestedRealm = input ? (input.value || '').trim() : '';
    loadServiceAccessInfo(requestedRealm);
}

function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str || '';
    return div.innerHTML;
}

function formatDate(iso) {
    if (!iso) return '–';
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleString();
}

// ── Tab Switching ────────────────────────────────────────────────────────────

function switchIAMTab(tabId) {
    document.querySelectorAll('.admin-container .tab-content').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.admin-container .tab-btn').forEach(b => b.classList.remove('active'));

    const target = document.getElementById(tabId);
    if (target) target.classList.add('active');

    const tabs = document.querySelectorAll('.admin-container .tab-btn');
    tabs.forEach(b => {
        if (b.getAttribute('onclick') && b.getAttribute('onclick').includes(tabId)) {
            b.classList.add('active');
        }
    });

    // Lazy-load data on tab switch
    if (tabId === 'iam-users') loadIAMUsers();
    if (tabId === 'iam-clients') {
        loadIAMClients();
        loadServiceAccessInfo();
    }
    if (tabId === 'iam-roles') loadIAMRoles();
    if (tabId === 'iam-bindings') loadIAMBindings();
    if (tabId === 'iam-oidc') loadOIDCInfo();
    if (tabId === 'iam-dashboard') loadIAMDashboard();
    if (tabId === 'iam-realms') loadRealms();
    if (tabId === 'iam-groups') { populateRealmDropdowns(); loadGroups(); }
    if (tabId === 'iam-idps') { populateRealmDropdowns(); loadIdentityProviders(); }
    if (tabId === 'iam-scopes') { populateRealmDropdowns(); loadClientScopes(); }
    if (tabId === 'iam-events') { populateRealmDropdowns(); loadEvents(); }
}

// ── Modal helpers ────────────────────────────────────────────────────────────

function openIAMModal(id) {
    const m = document.getElementById(id);
    if (m) m.style.display = 'flex';
}

function closeIAMModal(id) {
    const m = document.getElementById(id);
    if (m) m.style.display = 'none';
}

// Close modals on background click
document.addEventListener('click', function(e) {
    if (e.target.classList.contains('modal') && e.target.id && e.target.id.startsWith('iam')) {
        e.target.style.display = 'none';
    }
});

// ── IAM Login ────────────────────────────────────────────────────────────────

async function iamLogin() {
    const identifier = document.getElementById('iamLoginEmail').value.trim();
    const password = document.getElementById('iamLoginPassword').value;
    if (!identifier || !password) { showIAMToast('Username/email and password required', true); return; }

    try {
        const resp = await fetch(IAM_API + '/iam/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email: identifier, password: password })
        });
        const data = await resp.json();
        if (!resp.ok) {
            showIAMToast(data.error || 'Login failed', true);
            return;
        }
        iamToken = data.access_token || '';
        iamRefreshToken = data.refresh_token || '';
        localStorage.setItem('iamToken', iamToken);
        localStorage.setItem('iamRefreshToken', iamRefreshToken);
        showIAMToast('IAM login successful');
        updateIAMLoginBanner();
        loadIAMDashboard();
    } catch (err) {
        showIAMToast('Login error: ' + err.message, true);
    }
}

function iamLogout() {
    iamToken = '';
    iamRefreshToken = '';
    localStorage.removeItem('iamToken');
    localStorage.removeItem('iamRefreshToken');
    updateIAMLoginBanner();
    showIAMToast('IAM session ended');
}

function updateIAMLoginBanner() {
    const banner = document.getElementById('iamLoginBanner');
    const latestToken = iamToken || localStorage.getItem('iamToken') || resolvePlatformAuthToken();
    if (banner) banner.style.display = latestToken ? 'none' : 'block';
}

function updateIAMPermissionBanner(show, message) {
    const banner = document.getElementById('iamPermissionBanner');
    const text = document.getElementById('iamPermissionBannerMessage');
    if (text && message) {
        text.textContent = message;
    }
    if (banner) {
        banner.style.display = show ? 'block' : 'none';
    }
}

function setIAMSystemStatus(text, color) {
    const el = document.getElementById('iamStatSystem');
    if (!el) return;
    el.textContent = text;
    if (color) el.style.color = color;
}

// ── Dashboard ────────────────────────────────────────────────────────────────

async function loadIAMDashboard() {
    if (!iamToken) {
        iamToken = localStorage.getItem('iamToken') || resolvePlatformAuthToken();
    }
    if (!iamRefreshToken) {
        iamRefreshToken = localStorage.getItem('iamRefreshToken') || resolvePlatformRefreshToken();
    }

    if (!iamToken) {
        updateIAMPermissionBanner(false, '');
        updateIAMLoginBanner();
        setIAMSystemStatus('Auth Required', 'var(--warning-color)');
        return;
    }

    updateIAMPermissionBanner(false, '');
    setIAMSystemStatus('Checking', 'var(--text-muted)');

    // WhoAmI
    try {
        const resp = await iamFetch('/iam/auth/whoami');
        const whoami = document.getElementById('iamWhoAmI');
        if (resp.ok) {
            const data = await resp.json();
            if (whoami) whoami.textContent = JSON.stringify(data, null, 2);
        } else {
            if (whoami) whoami.textContent = 'Error: ' + resp.status;
            if (resp.status === 401) {
                setIAMSystemStatus('Auth Required', 'var(--warning-color)');
                updateIAMLoginBanner();
                return;
            }
            if (resp.status === 403) {
                setIAMSystemStatus('Access Denied', 'var(--danger-color)');
                updateIAMPermissionBanner(true, 'Your IAM token is valid but not authorized for this operation.');
                return;
            }
            setIAMSystemStatus('WhoAmI Error', 'var(--danger-color)');
            return;
        }
    } catch (err) {
        const whoami = document.getElementById('iamWhoAmI');
        if (whoami) whoami.textContent = 'Connection error: ' + err.message;
        setIAMSystemStatus('Connection Error', 'var(--danger-color)');
        if (!iamPermissionHintShown) {
            showIAMToast('Cannot reach IAM backend. Check backend URL/network.', true);
            iamPermissionHintShown = true;
        }
        return;
    }

    // Counts
    try {
        const [uResp, cResp, rResp] = await Promise.all([
            iamFetch('/iam/admin/users'),
            iamFetch('/iam/admin/clients'),
            iamFetch('/iam/admin/roles')
        ]);

        if (uResp.status === 401 || cResp.status === 401 || rResp.status === 401) {
            setIAMSystemStatus('Auth Required', 'var(--warning-color)');
            updateIAMLoginBanner();
            updateIAMPermissionBanner(false, '');
            return;
        }

        if (uResp.status === 403 || cResp.status === 403 || rResp.status === 403) {
            setIAMSystemStatus('Access Denied', 'var(--danger-color)');
            updateIAMPermissionBanner(true, 'Your IAM token is valid but not sysadmin. Login with IAM sysadmin to view admin data.');
            if (!iamPermissionHintShown) {
                showIAMToast('IAM session is not sysadmin. Admin data is restricted.', true);
                iamPermissionHintShown = true;
            }
            return;
        }

        if (!uResp.ok || !cResp.ok || !rResp.ok) {
            setIAMSystemStatus('API Error', 'var(--danger-color)');
            if (!iamPermissionHintShown) {
                showIAMToast('IAM admin endpoints returned an error. Check backend logs.', true);
                iamPermissionHintShown = true;
            }
            return;
        }

        if (uResp.ok) {
            const users = await uResp.json();
            const count = Array.isArray(users) ? users.length : (users.users ? users.users.length : 0);
            setTextById('dashUserCount', count);
            setTextById('iamStatUsers', count);
        }
        if (cResp.ok) {
            const clients = await cResp.json();
            const count = Array.isArray(clients) ? clients.length : (clients.clients ? clients.clients.length : 0);
            setTextById('dashClientCount', count);
            setTextById('iamStatClients', count);
        }
        if (rResp.ok) {
            const roles = await rResp.json();
            const count = Array.isArray(roles) ? roles.length : (roles.roles ? roles.roles.length : 0);
            setTextById('dashRoleCount', count);
            setTextById('iamStatRoles', count);
        }

        setIAMSystemStatus('Active', 'var(--primary-color)');
        iamPermissionHintShown = false;
    } catch (err) {
        setIAMSystemStatus('Connection Error', 'var(--danger-color)');
        if (!iamPermissionHintShown) {
            showIAMToast('Failed to load IAM dashboard data: ' + err.message, true);
            iamPermissionHintShown = true;
        }
    }
}

function setTextById(id, val) {
    const el = document.getElementById(id);
    if (el) el.textContent = val;
}

// ── Users CRUD ───────────────────────────────────────────────────────────────

let iamUsersCache = [];
let iamClientsCache = [];

async function loadIAMUsers() {
    if (!iamToken) return;
    try {
        const resp = await iamFetch('/iam/admin/users');
        if (!resp.ok) {
            if (resp.status === 403) {
                updateIAMPermissionBanner(true, 'Your IAM session does not have sysadmin privileges to list users.');
            }
            showIAMToast('Failed to load users: ' + resp.status, true);
            return;
        }
        const data = await resp.json();
        iamUsersCache = Array.isArray(data) ? data : (data.users || []);
        renderIAMUsers(iamUsersCache);
    } catch (err) {
        showIAMToast('Error loading users: ' + err.message, true);
    }
}

function renderIAMUsers(users) {
    const tbody = document.getElementById('iamUsersBody');
    if (!tbody) return;
    if (!users.length) {
        tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:var(--text-muted);">No users found</td></tr>';
        return;
    }
    tbody.innerHTML = users.map(u => {
        const active = u.active !== false;
        const verified = u.email_verified === true;
        return '<tr>' +
            '<td>' + escapeHtml(u.email) + '</td>' +
            '<td>' + escapeHtml(u.display_name || '–') + '</td>' +
            '<td><span class="status-badge ' + (active ? 'status-active' : 'status-inactive') + '">' + (active ? 'Active' : 'Inactive') + '</span></td>' +
            '<td><span class="status-badge ' + (verified ? 'status-active' : 'status-inactive') + '">' + (verified ? 'Yes' : 'No') + '</span></td>' +
            '<td>' + formatDate(u.created_at) + '</td>' +
            '<td style="white-space:nowrap;">' +
                '<button class="btn-sm btn-primary" onclick="viewUser(\'' + escapeHtml(u.id) + '\')">View</button> ' +
                '<button class="btn-sm btn-secondary" onclick="editUser(\'' + escapeHtml(u.id) + '\')">Edit</button> ' +
                '<button class="btn-sm" style="background:var(--danger-color);color:#fff;" onclick="deleteUser(\'' + escapeHtml(u.id) + '\')">Delete</button>' +
            '</td></tr>';
    }).join('');
}

function filterIAMUsers() {
    const q = (document.getElementById('iamUserSearch').value || '').toLowerCase();
    if (!q) { renderIAMUsers(iamUsersCache); return; }
    renderIAMUsers(iamUsersCache.filter(u =>
        (u.email || '').toLowerCase().includes(q) ||
        (u.display_name || '').toLowerCase().includes(q)
    ));
}

function showCreateUserModal() {
    document.getElementById('iamUserEditID').value = '';
    document.getElementById('iamUserEmail').value = '';
    document.getElementById('iamUserDisplayName').value = '';
    document.getElementById('iamUserPassword').value = '';
    document.getElementById('iamUserActive').checked = true;
    document.getElementById('iamUserEmailVerified').checked = false;
    document.getElementById('iamUserPasswordGroup').style.display = '';
    document.getElementById('iamUserModalTitle').textContent = 'Create User';
    document.getElementById('iamUserSubmitBtn').textContent = 'Create';
    openIAMModal('iamUserModal');
}

async function viewUser(id) {
    try {
        const resp = await iamFetch('/iam/admin/users/' + encodeURIComponent(id));
        if (!resp.ok) { showIAMToast('Failed to load user', true); return; }
        const data = await resp.json();
        document.getElementById('iamUserDetailContent').textContent = JSON.stringify(data, null, 2);
        openIAMModal('iamUserDetailModal');
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

async function editUser(id) {
    try {
        const resp = await iamFetch('/iam/admin/users/' + encodeURIComponent(id));
        if (!resp.ok) { showIAMToast('Failed to load user', true); return; }
        const u = await resp.json();
        document.getElementById('iamUserEditID').value = u.id;
        document.getElementById('iamUserEmail').value = u.email || '';
        document.getElementById('iamUserDisplayName').value = u.display_name || '';
        document.getElementById('iamUserPassword').value = '';
        document.getElementById('iamUserActive').checked = u.active !== false;
        document.getElementById('iamUserEmailVerified').checked = u.email_verified === true;
        document.getElementById('iamUserPasswordGroup').style.display = 'none';
        document.getElementById('iamUserModalTitle').textContent = 'Edit User';
        document.getElementById('iamUserSubmitBtn').textContent = 'Update';
        openIAMModal('iamUserModal');
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

async function submitUser() {
    const editID = document.getElementById('iamUserEditID').value;
    const body = {
        email: document.getElementById('iamUserEmail').value.trim(),
        display_name: document.getElementById('iamUserDisplayName').value.trim(),
        active: document.getElementById('iamUserActive').checked,
        email_verified: document.getElementById('iamUserEmailVerified').checked
    };
    if (!editID) {
        body.password = document.getElementById('iamUserPassword').value;
        if (!body.email || !body.password) { showIAMToast('Email and password are required', true); return; }
    }

    try {
        const url = editID ? '/iam/admin/users/' + encodeURIComponent(editID) : '/iam/admin/users';
        const method = editID ? 'PUT' : 'POST';
        const resp = await iamFetch(url, { method: method, body: JSON.stringify(body) });
        const data = await resp.json();
        if (!resp.ok) { showIAMToast(data.error || 'Operation failed', true); return; }
        showIAMToast(editID ? 'User updated' : 'User created');
        closeIAMModal('iamUserModal');
        loadIAMUsers();
        loadIAMDashboard();
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

async function deleteUser(id) {
    if (!confirm('Delete this user permanently?')) return;
    try {
        const resp = await iamFetch('/iam/admin/users/' + encodeURIComponent(id), { method: 'DELETE' });
        if (!resp.ok) {
            const data = await resp.json().catch(() => ({}));
            showIAMToast(data.error || 'Delete failed', true);
            return;
        }
        showIAMToast('User deleted');
        loadIAMUsers();
        loadIAMDashboard();
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

// ── Clients CRUD ─────────────────────────────────────────────────────────────

async function loadIAMClients() {
    if (!iamToken) return;
    try {
        const resp = await iamFetch('/iam/admin/clients');
        if (!resp.ok) {
            if (resp.status === 403) {
                updateIAMPermissionBanner(true, 'Your IAM session does not have sysadmin privileges to list OAuth clients.');
            }
            showIAMToast('Failed to load clients: ' + resp.status, true);
            return;
        }
        const data = await resp.json();
        const clients = Array.isArray(data) ? data : (data.clients || []);
        iamClientsCache = clients;
        renderIAMClients(clients);
    } catch (err) {
        showIAMToast('Error loading clients: ' + err.message, true);
    }
}

function renderIAMClients(clients) {
    const tbody = document.getElementById('iamClientsBody');
    if (!tbody) return;
    if (!clients.length) {
        tbody.innerHTML = '<tr><td colspan="9" style="text-align:center;color:var(--text-muted);">No clients registered</td></tr>';
        return;
    }
    tbody.innerHTML = clients.map(c => {
        const grants = (c.grant_types || []).join(', ') || '–';
        const uris = (c.redirect_uris || []).map(u => escapeHtml(u)).join('<br>') || '–';
        const scopes = (c.scopes || []).join(', ') || '–';
        const serviceRoles = (c.service_roles || []).join(', ') || '–';
        const rateLimit = c.rate_limit_max_calls || 500;
        const tokenValidityMinutes = c.token_validity_minutes || 15;
        const mode = c.public ? 'Public' : 'Confidential';
        const hasClientCredentials = Array.isArray(c.grant_types) && c.grant_types.includes('client_credentials');
        const canGenerateToken = hasClientCredentials && !c.public;
        const canRegenerateSecret = !c.public;
        return '<tr>' +
            '<td style="font-family:var(--font-mono);font-size:.85rem;">' + escapeHtml(c.id) + '</td>' +
            '<td>' + escapeHtml(c.name) + '<div style="font-size:.75rem;color:var(--text-muted);margin-top:4px;">' + escapeHtml(mode) + '</div></td>' +
            '<td>' + escapeHtml(grants) + '</td>' +
            '<td style="font-size:.8rem;">' + uris + '</td>' +
            '<td>' + escapeHtml(scopes) + '</td>' +
            '<td>' + escapeHtml(serviceRoles) + '</td>' +
            '<td>' + escapeHtml(String(rateLimit)) + '</td>' +
            '<td>' + escapeHtml(String(tokenValidityMinutes) + ' min') + '</td>' +
            '<td style="white-space:nowrap;">' +
                (canGenerateToken ? '<button class="btn-sm btn-primary" onclick="showGenerateTokenModal(\'' + escapeHtml(c.id) + '\')">Generate Token</button> ' : '') +
                (canRegenerateSecret ? '<button class="btn-sm btn-secondary" onclick="regenerateClientSecret(\'' + escapeHtml(c.id) + '\')">Regen Secret</button> ' : '') +
                '<button class="btn-sm btn-secondary" onclick="showClientIDChangeModal(\'' + escapeHtml(c.id) + '\')">Change ID</button> ' +
                '<button class="btn-sm btn-secondary" onclick="editClient(\'' + escapeHtml(c.id) + '\')">Edit</button> ' +
                '<button class="btn-sm" style="background:var(--danger-color);color:#fff;" onclick="deleteClient(\'' + escapeHtml(c.id) + '\')">Delete</button>' +
            '</td></tr>';
    }).join('');
}

function showCreateClientModal() {
    document.getElementById('iamClientEditID').value = '';
    document.getElementById('iamClientName').value = '';
    document.getElementById('iamClientRedirectURIs').value = '';
    document.getElementById('iamClientGrantTypes').value = 'authorization_code,refresh_token,client_credentials';
    document.getElementById('iamClientScopes').value = 'openid,profile,email';
    document.getElementById('iamClientServiceRoles').value = '';
    document.getElementById('iamClientRateLimitMaxCalls').value = '500';
    document.getElementById('iamClientTokenValidityMinutes').value = '15';
    document.getElementById('iamClientPublic').checked = false;
    document.getElementById('iamClientModalTitle').textContent = 'Register OAuth Client';
    document.getElementById('iamClientSubmitBtn').textContent = 'Register';
    openIAMModal('iamClientModal');
}

async function editClient(id) {
    try {
        const resp = await iamFetch('/iam/admin/clients/' + encodeURIComponent(id));
        if (!resp.ok) { showIAMToast('Failed to load client', true); return; }
        const c = await resp.json();
        document.getElementById('iamClientEditID').value = c.id;
        document.getElementById('iamClientName').value = c.name || '';
        document.getElementById('iamClientRedirectURIs').value = (c.redirect_uris || []).join('\n');
        document.getElementById('iamClientGrantTypes').value = (c.grant_types || []).join(',');
        document.getElementById('iamClientScopes').value = (c.scopes || []).join(',');
        document.getElementById('iamClientServiceRoles').value = (c.service_roles || []).join(',');
        document.getElementById('iamClientRateLimitMaxCalls').value = String(c.rate_limit_max_calls || 500);
        document.getElementById('iamClientTokenValidityMinutes').value = String(c.token_validity_minutes || 15);
        document.getElementById('iamClientPublic').checked = c.public === true;
        document.getElementById('iamClientModalTitle').textContent = 'Edit Client';
        document.getElementById('iamClientSubmitBtn').textContent = 'Update';
        openIAMModal('iamClientModal');
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

async function submitClient() {
    const editID = document.getElementById('iamClientEditID').value;
    const name = document.getElementById('iamClientName').value.trim();
    if (!name) { showIAMToast('Client name is required', true); return; }

    const redirectURIs = document.getElementById('iamClientRedirectURIs').value
        .split('\n').map(s => s.trim()).filter(Boolean);
    const grantTypes = document.getElementById('iamClientGrantTypes').value
        .split(',').map(s => s.trim()).filter(Boolean);
    const scopes = document.getElementById('iamClientScopes').value
        .split(',').map(s => s.trim()).filter(Boolean);
    const serviceRoles = document.getElementById('iamClientServiceRoles').value
        .split(',').map(s => s.trim()).filter(Boolean);
    const publicClient = document.getElementById('iamClientPublic').checked;
    const rateLimitMaxCalls = parseInt(document.getElementById('iamClientRateLimitMaxCalls').value, 10);
    const tokenValidityMinutes = parseInt(document.getElementById('iamClientTokenValidityMinutes').value, 10);

    if (!Number.isFinite(rateLimitMaxCalls) || rateLimitMaxCalls < 1) {
        showIAMToast('Rate limit max calls must be at least 1', true);
        return;
    }
    if (!Number.isFinite(tokenValidityMinutes) || tokenValidityMinutes < 1) {
        showIAMToast('Token validity minutes must be at least 1', true);
        return;
    }

    if (publicClient && grantTypes.includes('client_credentials')) {
        showIAMToast('Public clients cannot use client_credentials grant', true);
        return;
    }

    const body = {
        name: name,
        redirect_uris: redirectURIs,
        grant_types: grantTypes,
        scopes: scopes,
        service_roles: serviceRoles,
        rate_limit_max_calls: rateLimitMaxCalls,
        token_validity_minutes: tokenValidityMinutes,
        public: publicClient
    };

    try {
        const url = editID ? '/iam/admin/clients/' + encodeURIComponent(editID) : '/iam/admin/clients';
        const method = editID ? 'PUT' : 'POST';
        const resp = await iamFetch(url, { method: method, body: JSON.stringify(body) });
        const data = await resp.json();
        if (!resp.ok) { showIAMToast(data.error || 'Operation failed', true); return; }

        closeIAMModal('iamClientModal');

        // Show secret for new clients
        if (!editID && data.client_secret) {
            const clientID = data.id || data.client_id || '';
            const clientSecret = data.client_secret || '';
            const defaultScope = (data.scopes || []).join(' ');
            showClientSecretModal(clientID, clientSecret, defaultScope, '🔑 Client Registered');
        } else {
            showIAMToast(editID ? 'Client updated' : 'Client registered');
        }
        loadIAMClients();
        loadIAMDashboard();
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

function showClientSecretModal(clientID, clientSecret, scope, title) {
    setFieldText('iamClientSecretModalTitle', title || '🔑 Client Secret');
    setFieldText('iamNewClientID', clientID || '');
    setFieldText('iamNewClientSecret', clientSecret || '');
    setFieldText('iamClientTokenEndpoint', getServiceTokenEndpoint());
    setFieldText('iamClientCredentialsSnippet', buildClientCredentialsSnippet(clientID || '', clientSecret || '', scope || ''));
    openIAMModal('iamClientSecretModal');
}

async function copyClientSecret() {
    const secret = document.getElementById('iamNewClientSecret').value;
    const ok = await copyToClipboard(secret);
    showIAMToast(ok ? 'Secret copied' : 'Copy failed', !ok);
}

async function copyClientCredentialsSnippet() {
    const snippet = document.getElementById('iamClientCredentialsSnippet').textContent;
    const ok = await copyToClipboard(snippet || '');
    showIAMToast(ok ? 'Request copied' : 'Copy failed', !ok);
}

function getClientByID(clientID) {
    return iamClientsCache.find(c => c.id === clientID) || null;
}

function showClientIDChangeModal(clientID) {
    setFieldText('iamCurrentClientID', clientID || '');
    setFieldText('iamNewClientIDInput', clientID || '');
    openIAMModal('iamClientIDChangeModal');
}

async function submitClientIDChange() {
    const currentID = (document.getElementById('iamCurrentClientID').value || '').trim();
    const newClientID = (document.getElementById('iamNewClientIDInput').value || '').trim();

    if (!currentID || !newClientID) {
        showIAMToast('Both current and new client IDs are required.', true);
        return;
    }

    try {
        const resp = await iamFetch('/iam/admin/clients/' + encodeURIComponent(currentID) + '/client-id', {
            method: 'PUT',
            body: JSON.stringify({ new_client_id: newClientID })
        });
        const data = await resp.json();
        if (!resp.ok) {
            showIAMToast(data.error || 'Client ID update failed', true);
            return;
        }

        showIAMToast('Client ID updated');
        closeIAMModal('iamClientIDChangeModal');
        loadIAMClients();
        loadIAMDashboard();
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

async function regenerateClientSecret(clientID) {
    if (!confirm('Regenerate secret for this client? Existing secret integrations will stop working.')) return;

    const client = getClientByID(clientID);
    if (!client) {
        showIAMToast('Client not found. Refresh and try again.', true);
        return;
    }
    if (client.public) {
        showIAMToast('Public clients do not have secrets.', true);
        return;
    }

    try {
        const resp = await iamFetch('/iam/admin/clients/' + encodeURIComponent(clientID) + '/regenerate-secret', {
            method: 'POST'
        });
        const data = await resp.json();
        if (!resp.ok) {
            showIAMToast(data.error || 'Secret regeneration failed', true);
            return;
        }

        const scope = (data.scopes || client.scopes || []).join(' ');
        showClientSecretModal(data.client_id || clientID, data.client_secret || '', scope, '🔁 Client Secret Regenerated');
        showIAMToast('Client secret regenerated');
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

function showGenerateTokenModal(clientID) {
    const client = getClientByID(clientID);
    if (!client) {
        showIAMToast('Client not found. Refresh and try again.', true);
        return;
    }

    if (client.public) {
        showIAMToast('Public clients do not support client_credentials token generation.', true);
        return;
    }

    if (!Array.isArray(client.grant_types) || !client.grant_types.includes('client_credentials')) {
        showIAMToast('This client does not allow client_credentials grant.', true);
        return;
    }

    setFieldText('iamGenerateTokenClientID', client.id || '');
    setFieldText('iamGenerateTokenClientSecret', '');
    setFieldText('iamGenerateTokenScope', (client.scopes || []).join(' '));
    setFieldText('iamGenerateTokenEndpoint', getServiceTokenEndpoint());
    setFieldText('iamGenerateTokenResponse', 'No token generated yet.');
    iamLastGeneratedTokenResponse = null;

    openIAMModal('iamGenerateTokenModal');
}

async function generateClientTestToken() {
    const clientID = (document.getElementById('iamGenerateTokenClientID').value || '').trim();
    const clientSecret = (document.getElementById('iamGenerateTokenClientSecret').value || '').trim();
    const scope = (document.getElementById('iamGenerateTokenScope').value || '').trim();
    const endpoint = (document.getElementById('iamGenerateTokenEndpoint').value || '').trim();

    if (!clientID || !clientSecret || !endpoint) {
        showIAMToast('Client ID, secret, and endpoint are required.', true);
        return;
    }

    const form = new URLSearchParams();
    form.set('grant_type', 'client_credentials');
    form.set('client_id', clientID);
    form.set('client_secret', clientSecret);
    if (scope) form.set('scope', scope);

    try {
        const resp = await fetch(endpoint, {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: form.toString()
        });

        const raw = await resp.text();
        let parsed = null;
        try {
            parsed = raw ? JSON.parse(raw) : {};
        } catch (_) {
            parsed = { raw_response: raw };
        }

        iamLastGeneratedTokenResponse = parsed;
        setFieldText('iamGenerateTokenResponse', JSON.stringify(parsed, null, 2));

        if (!resp.ok) {
            const message = parsed && parsed.error ? parsed.error : ('Token generation failed: ' + resp.status);
            showIAMToast(message, true);
            return;
        }

        showIAMToast('Test token generated');
    } catch (err) {
        setFieldText('iamGenerateTokenResponse', 'Connection error: ' + err.message);
        showIAMToast('Token request failed: ' + err.message, true);
    }
}

async function copyGeneratedAccessToken() {
    const token = iamLastGeneratedTokenResponse && iamLastGeneratedTokenResponse.access_token ? iamLastGeneratedTokenResponse.access_token : '';
    if (!token) {
        showIAMToast('No generated access token to copy.', true);
        return;
    }
    const ok = await copyToClipboard(token);
    showIAMToast(ok ? 'Access token copied' : 'Copy failed', !ok);
}

async function deleteClient(id) {
    if (!confirm('Delete this OAuth client?')) return;
    try {
        const resp = await iamFetch('/iam/admin/clients/' + encodeURIComponent(id), { method: 'DELETE' });
        if (!resp.ok) {
            const data = await resp.json().catch(() => ({}));
            showIAMToast(data.error || 'Delete failed', true);
            return;
        }
        showIAMToast('Client deleted');
        loadIAMClients();
        loadIAMDashboard();
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

// ── Roles CRUD ───────────────────────────────────────────────────────────────

async function loadIAMRoles() {
    if (!iamToken) return;
    try {
        const resp = await iamFetch('/iam/admin/roles');
        if (!resp.ok) { showIAMToast('Failed to load roles: ' + resp.status, true); return; }
        const data = await resp.json();
        const roles = Array.isArray(data) ? data : (data.roles || []);
        renderIAMRoles(roles);
    } catch (err) {
        showIAMToast('Error loading roles: ' + err.message, true);
    }
}

function renderIAMRoles(roles) {
    const tbody = document.getElementById('iamRolesBody');
    if (!tbody) return;
    if (!roles.length) {
        tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;color:var(--text-muted);">No roles defined</td></tr>';
        return;
    }
    tbody.innerHTML = roles.map(r => {
        const isSystem = r.system === true;
        const perms = (r.permissions || []).map(p =>
            '<span class="status-badge" style="font-size:.75rem;margin:2px;">' + escapeHtml(p.resource + ':' + p.action) + '</span>'
        ).join(' ') || '–';
        return '<tr>' +
            '<td style="font-weight:600;">' + escapeHtml(r.name) + '</td>' +
            '<td>' + escapeHtml(r.description || '–') + '</td>' +
            '<td><span class="status-badge ' + (isSystem ? 'status-active' : '') + '">' + (isSystem ? 'System' : 'Custom') + '</span></td>' +
            '<td>' + perms + '</td>' +
            '<td style="white-space:nowrap;">' +
                (isSystem ? '<span style="color:var(--text-muted);font-size:.8rem;">Protected</span>' :
                    '<button class="btn-sm btn-secondary" onclick="editRole(\'' + escapeHtml(r.id) + '\')">Edit</button> ' +
                    '<button class="btn-sm" style="background:var(--danger-color);color:#fff;" onclick="deleteRole(\'' + escapeHtml(r.id) + '\')">Delete</button>'
                ) +
            '</td></tr>';
    }).join('');
}

function showCreateRoleModal() {
    document.getElementById('iamRoleEditID').value = '';
    document.getElementById('iamRoleName').value = '';
    document.getElementById('iamRoleDescription').value = '';
    document.getElementById('iamRolePermissions').value = '';
    document.getElementById('iamRoleModalTitle').textContent = 'Create Role';
    document.getElementById('iamRoleSubmitBtn').textContent = 'Create';
    openIAMModal('iamRoleModal');
}

async function editRole(id) {
    try {
        const resp = await iamFetch('/iam/admin/roles/' + encodeURIComponent(id));
        if (!resp.ok) { showIAMToast('Failed to load role', true); return; }
        const r = await resp.json();
        document.getElementById('iamRoleEditID').value = r.id;
        document.getElementById('iamRoleName').value = r.name || '';
        document.getElementById('iamRoleDescription').value = r.description || '';
        document.getElementById('iamRolePermissions').value = (r.permissions || [])
            .map(p => p.resource + ':' + p.action).join('\n');
        document.getElementById('iamRoleModalTitle').textContent = 'Edit Role';
        document.getElementById('iamRoleSubmitBtn').textContent = 'Update';
        openIAMModal('iamRoleModal');
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

async function submitRole() {
    const editID = document.getElementById('iamRoleEditID').value;
    const name = document.getElementById('iamRoleName').value.trim();
    if (!name) { showIAMToast('Role name is required', true); return; }

    const permissions = document.getElementById('iamRolePermissions').value
        .split('\n').map(s => s.trim()).filter(Boolean)
        .map(line => {
            const parts = line.split(':');
            return { resource: parts[0] || '*', action: parts[1] || '*' };
        });

    const body = {
        name: name,
        description: document.getElementById('iamRoleDescription').value.trim(),
        permissions: permissions
    };

    try {
        const url = editID ? '/iam/admin/roles/' + encodeURIComponent(editID) : '/iam/admin/roles';
        const method = editID ? 'PUT' : 'POST';
        const resp = await iamFetch(url, { method: method, body: JSON.stringify(body) });
        const data = await resp.json();
        if (!resp.ok) { showIAMToast(data.error || 'Operation failed', true); return; }
        showIAMToast(editID ? 'Role updated' : 'Role created');
        closeIAMModal('iamRoleModal');
        loadIAMRoles();
        loadIAMDashboard();
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

async function deleteRole(id) {
    if (!confirm('Delete this role?')) return;
    try {
        const resp = await iamFetch('/iam/admin/roles/' + encodeURIComponent(id), { method: 'DELETE' });
        if (!resp.ok) {
            const data = await resp.json().catch(() => ({}));
            showIAMToast(data.error || 'Delete failed', true);
            return;
        }
        showIAMToast('Role deleted');
        loadIAMRoles();
        loadIAMDashboard();
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

// ── Role Bindings ────────────────────────────────────────────────────────────

async function loadIAMBindings() {
    if (!iamToken) return;
    try {
        const resp = await iamFetch('/iam/admin/role-bindings');
        if (!resp.ok) { showIAMToast('Failed to load bindings: ' + resp.status, true); return; }
        const data = await resp.json();
        const bindings = Array.isArray(data) ? data : (data.bindings || []);
        renderIAMBindings(bindings);
    } catch (err) {
        showIAMToast('Error loading bindings: ' + err.message, true);
    }
}

function renderIAMBindings(bindings) {
    const tbody = document.getElementById('iamBindingsBody');
    if (!tbody) return;
    if (!bindings.length) {
        tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;color:var(--text-muted);">No role bindings</td></tr>';
        return;
    }
    tbody.innerHTML = bindings.map(b => {
        return '<tr>' +
            '<td style="font-family:var(--font-mono);font-size:.8rem;">' + escapeHtml(b.id) + '</td>' +
            '<td style="font-family:var(--font-mono);font-size:.8rem;">' + escapeHtml(b.user_id) + '</td>' +
            '<td style="font-family:var(--font-mono);font-size:.8rem;">' + escapeHtml(b.role_id) + '</td>' +
            '<td>' + formatDate(b.assigned_at) + '</td>' +
            '<td><button class="btn-sm" style="background:var(--danger-color);color:#fff;" onclick="revokeBinding(\'' + escapeHtml(b.id) + '\')">Revoke</button></td>' +
            '</tr>';
    }).join('');
}

async function showAssignRoleModal() {
    // Populate user / role dropdowns
    const userSel = document.getElementById('iamBindingUserID');
    const roleSel = document.getElementById('iamBindingRoleID');
    userSel.innerHTML = '<option value="">Loading...</option>';
    roleSel.innerHTML = '<option value="">Loading...</option>';
    openIAMModal('iamBindingModal');

    try {
        const [uResp, rResp] = await Promise.all([
            iamFetch('/iam/admin/users'),
            iamFetch('/iam/admin/roles')
        ]);
        if (uResp.ok) {
            const data = await uResp.json();
            const users = Array.isArray(data) ? data : (data.users || []);
            userSel.innerHTML = '<option value="">— Select user —</option>' +
                users.map(u => '<option value="' + escapeHtml(u.id) + '">' + escapeHtml(u.email) + '</option>').join('');
        }
        if (rResp.ok) {
            const data = await rResp.json();
            const roles = Array.isArray(data) ? data : (data.roles || []);
            roleSel.innerHTML = '<option value="">— Select role —</option>' +
                roles.map(r => '<option value="' + escapeHtml(r.id) + '">' + escapeHtml(r.name) + '</option>').join('');
        }
    } catch (_) {
        showIAMToast('Failed to populate dropdowns', true);
    }
}

async function submitBinding() {
    const userID = document.getElementById('iamBindingUserID').value;
    const roleID = document.getElementById('iamBindingRoleID').value;
    if (!userID || !roleID) { showIAMToast('Select both user and role', true); return; }

    try {
        const resp = await iamFetch('/iam/admin/role-bindings', {
            method: 'POST',
            body: JSON.stringify({ user_id: userID, role_id: roleID })
        });
        const data = await resp.json();
        if (!resp.ok) { showIAMToast(data.error || 'Assignment failed', true); return; }
        showIAMToast('Role assigned');
        closeIAMModal('iamBindingModal');
        loadIAMBindings();
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

async function revokeBinding(id) {
    if (!confirm('Revoke this role assignment?')) return;
    try {
        const resp = await iamFetch('/iam/admin/role-bindings/' + encodeURIComponent(id), { method: 'DELETE' });
        if (!resp.ok) {
            const data = await resp.json().catch(() => ({}));
            showIAMToast(data.error || 'Revoke failed', true);
            return;
        }
        showIAMToast('Role binding revoked');
        loadIAMBindings();
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

// ── Token Management ─────────────────────────────────────────────────────────

async function revokeToken() {
    const jti = document.getElementById('revokeTokenJTI').value.trim();
    if (!jti) { showIAMToast('Enter a token JTI', true); return; }
    try {
        const resp = await iamFetch('/iam/admin/tokens/revoke', {
            method: 'POST',
            body: JSON.stringify({ jti: jti })
        });
        if (!resp.ok) {
            const data = await resp.json().catch(() => ({}));
            showIAMToast(data.error || 'Revoke failed', true);
            return;
        }
        showIAMToast('Token revoked');
        document.getElementById('revokeTokenJTI').value = '';
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

async function revokeUserTokens() {
    const uid = document.getElementById('revokeUserID').value.trim();
    if (!uid) { showIAMToast('Enter a user ID', true); return; }
    if (!confirm('Revoke ALL tokens for this user?')) return;
    try {
        const resp = await iamFetch('/iam/admin/users/' + encodeURIComponent(uid) + '/revoke-tokens', {
            method: 'POST'
        });
        if (!resp.ok) {
            const data = await resp.json().catch(() => ({}));
            showIAMToast(data.error || 'Revoke failed', true);
            return;
        }
        showIAMToast('All user tokens revoked');
        document.getElementById('revokeUserID').value = '';
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
    }
}

// ── OIDC / JWKS ──────────────────────────────────────────────────────────────

async function loadServiceAccessInfo(realmOverride) {
    const rawEl = document.getElementById('iamServiceAccessJSON');
    const realmInput = document.getElementById('iamServiceRealmInput');
    const requestedRealm = (realmOverride !== undefined && realmOverride !== null)
        ? String(realmOverride).trim().toLowerCase()
        : (iamSelectedRealm || (realmInput ? (realmInput.value || '').trim().toLowerCase() : ''));

    if (!iamToken) {
        setFieldText('iamServiceRealm', 'IAM login required');
        setFieldText('iamServiceIssuer', 'IAM login required');
        setFieldText('iamServiceTokenEndpoint', 'IAM login required');
        setFieldText('iamServiceDiscoveryEndpoint', 'IAM login required');
        setFieldText('iamServiceCertsEndpoint', 'IAM login required');
        setFieldText('iamServiceGrantTypes', 'IAM login required');
        if (realmInput) realmInput.value = '';
        if (rawEl) rawEl.textContent = 'IAM login required';
        updateIAMPermissionBanner(false, '');
        return;
    }

    try {
        const path = requestedRealm
            ? '/iam/admin/service-access-info?realm=' + encodeURIComponent(requestedRealm)
            : '/iam/admin/service-access-info';
        const resp = await iamFetch(path);
        if (!resp.ok) {
            if (resp.status === 403) {
                updateIAMPermissionBanner(true, 'Your IAM session does not have sysadmin privileges to view service access metadata.');
            }
            const text = 'Failed to load service access info: ' + resp.status;
            if (rawEl) rawEl.textContent = text;
            setFieldText('iamServiceTokenEndpoint', text);
            showIAMToast('Unable to load realm details', true);
            return;
        }

        const info = await resp.json();
        iamServiceAccessInfo = info;
        iamSelectedRealm = info.realm || requestedRealm || '';
        const endpoints = info.endpoints || {};

        setFieldText('iamServiceRealm', info.realm || 'master');
        if (realmInput) realmInput.value = iamSelectedRealm;
        setFieldText('iamServiceIssuer', info.issuer || '–');
        setFieldText('iamServiceTokenEndpoint', endpoints.keycloak_token || endpoints.iam_token || '–');
        setFieldText('iamServiceDiscoveryEndpoint', endpoints.keycloak_openid_configuration || endpoints.iam_openid_configuration || '–');
        setFieldText('iamServiceCertsEndpoint', endpoints.keycloak_certs || endpoints.iam_jwks || '–');
        setFieldText('iamServiceGrantTypes', (info.grant_types_supported || []).join(', ') || '–');

        if (rawEl) rawEl.textContent = JSON.stringify(info, null, 2);
    } catch (err) {
        const text = 'Connection error: ' + err.message;
        if (rawEl) rawEl.textContent = text;
        setFieldText('iamServiceTokenEndpoint', text);
    }
}

async function loadOIDCInfo() {
    try {
        const [oidcResp, jwksResp] = await Promise.all([
            fetch(IAM_API + '/.well-known/openid-configuration'),
            fetch(IAM_API + '/.well-known/jwks.json')
        ]);
        const oidcEl = document.getElementById('iamOIDCConfig');
        const jwksEl = document.getElementById('iamJWKS');
        if (oidcResp.ok) {
            const data = await oidcResp.json();
            if (oidcEl) oidcEl.textContent = JSON.stringify(data, null, 2);
        } else {
            if (oidcEl) oidcEl.textContent = 'Error: ' + oidcResp.status;
        }
        if (jwksResp.ok) {
            const data = await jwksResp.json();
            if (jwksEl) jwksEl.textContent = JSON.stringify(data, null, 2);
        } else {
            if (jwksEl) jwksEl.textContent = 'Error: ' + jwksResp.status;
        }
    } catch (err) {
        const oidcEl = document.getElementById('iamOIDCConfig');
        if (oidcEl) oidcEl.textContent = 'Connection error: ' + err.message;
    }

    loadServiceAccessInfo();
}

// ── V2 Realm Dropdown Helper ─────────────────────────────────────────────────

let v2RealmsCache = [];

async function populateRealmDropdowns() {
    if (!iamToken) return;
    try {
        const resp = await iamFetch('/iam/v2/realms');
        if (!resp.ok) return;
        const data = await resp.json();
        v2RealmsCache = Array.isArray(data) ? data : (data.realms || []);
    } catch (_) { return; }

    const selectors = ['groupsRealmFilter', 'idpRealmFilter', 'scopeRealmFilter', 'eventRealmFilter',
                       'iamGroupRealmID', 'iamIdPRealmID', 'iamScopeRealmID'];
    selectors.forEach(id => {
        const sel = document.getElementById(id);
        if (!sel) return;
        const isFilter = id.endsWith('Filter');
        const current = sel.value;
        sel.innerHTML = (isFilter ? '<option value="">All Realms</option>' : '') +
            v2RealmsCache.map(r => '<option value="' + escapeHtml(r.ID || r.id) + '"' +
                (r.name === 'master' && !isFilter ? ' selected' : '') +
                '>' + escapeHtml(r.name || r.display_name) + '</option>').join('');
        if (current) sel.value = current;
    });
}

// ── Realms CRUD ──────────────────────────────────────────────────────────────

async function loadRealms() {
    if (!iamToken) return;
    try {
        const resp = await iamFetch('/iam/v2/realms');
        if (!resp.ok) { showIAMToast('Failed to load realms: ' + resp.status, true); return; }
        const data = await resp.json();
        v2RealmsCache = Array.isArray(data) ? data : (data.realms || []);
        renderRealms(v2RealmsCache);
    } catch (err) { showIAMToast('Error loading realms: ' + err.message, true); }
}

function renderRealms(realms) {
    const tbody = document.getElementById('realmsBody');
    if (!tbody) return;
    if (!realms.length) {
        tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;color:var(--text-muted);">No realms</td></tr>';
        return;
    }
    tbody.innerHTML = realms.map(r => {
        const id = r.ID || r.id;
        return '<tr>' +
            '<td style="font-weight:600;">' + escapeHtml(r.name) + '</td>' +
            '<td>' + escapeHtml(r.display_name || '–') + '</td>' +
            '<td><span class="status-badge ' + (r.enabled !== false ? 'status-active' : 'status-inactive') + '">' + (r.enabled !== false ? 'Yes' : 'No') + '</span></td>' +
            '<td>' + (r.registration_allowed ? 'Yes' : 'No') + '</td>' +
            '<td>' + (r.access_token_lifespan || 900) + 's</td>' +
            '<td>' + (r.sso_session_idle_timeout || 1800) + 's</td>' +
            '<td><span class="status-badge ' + (r.brute_force_protected ? 'status-active' : '') + '">' + (r.brute_force_protected ? 'On' : 'Off') + '</span></td>' +
            '<td style="white-space:nowrap;">' +
                '<button class="btn-sm btn-primary" onclick="viewRealmDashboard(\'' + escapeHtml(id) + '\')">Dashboard</button> ' +
                '<button class="btn-sm btn-secondary" onclick="editRealm(\'' + escapeHtml(id) + '\')">Edit</button> ' +
                (r.name !== 'master' ? '<button class="btn-sm" style="background:var(--danger-color);color:#fff;" onclick="deleteRealm(\'' + escapeHtml(id) + '\')">Delete</button>' : '') +
            '</td></tr>';
    }).join('');
}

function showCreateRealmModal() {
    document.getElementById('iamRealmEditID').value = '';
    document.getElementById('iamRealmName').value = '';
    document.getElementById('iamRealmDisplayName').value = '';
    document.getElementById('iamRealmAccessTTL').value = '900';
    document.getElementById('iamRealmRefreshTTL').value = '604800';
    document.getElementById('iamRealmSSOIdle').value = '1800';
    document.getElementById('iamRealmSSOMax').value = '36000';
    document.getElementById('iamRealmPassMinLen').value = '8';
    document.getElementById('iamRealmMaxFailures').value = '30';
    document.getElementById('iamRealmEnabled').checked = true;
    document.getElementById('iamRealmRegAllowed').checked = false;
    document.getElementById('iamRealmResetPw').checked = true;
    document.getElementById('iamRealmRememberMe').checked = true;
    document.getElementById('iamRealmVerifyEmail').checked = false;
    document.getElementById('iamRealmLoginEmail').checked = true;
    document.getElementById('iamRealmBruteForce').checked = true;
    document.getElementById('iamRealmPassUpper').checked = true;
    document.getElementById('iamRealmPassDigit').checked = true;
    document.getElementById('iamRealmPassSpecial').checked = false;
    document.getElementById('iamRealmModalTitle').textContent = 'Create Realm';
    document.getElementById('iamRealmSubmitBtn').textContent = 'Create';
    openIAMModal('iamRealmModal');
}

async function editRealm(id) {
    try {
        const resp = await iamFetch('/iam/v2/realms/' + encodeURIComponent(id));
        if (!resp.ok) { showIAMToast('Failed to load realm', true); return; }
        const r = await resp.json();
        document.getElementById('iamRealmEditID').value = r.ID || r.id || id;
        document.getElementById('iamRealmName').value = r.name || '';
        document.getElementById('iamRealmDisplayName').value = r.display_name || '';
        document.getElementById('iamRealmAccessTTL').value = String(r.access_token_lifespan || 900);
        document.getElementById('iamRealmRefreshTTL').value = String(r.refresh_token_lifespan || 604800);
        document.getElementById('iamRealmSSOIdle').value = String(r.sso_session_idle_timeout || 1800);
        document.getElementById('iamRealmSSOMax').value = String(r.sso_session_max_lifespan || 36000);
        document.getElementById('iamRealmPassMinLen').value = String(r.password_min_length || 8);
        document.getElementById('iamRealmMaxFailures').value = String(r.max_login_failures || 30);
        document.getElementById('iamRealmEnabled').checked = r.enabled !== false;
        document.getElementById('iamRealmRegAllowed').checked = r.registration_allowed === true;
        document.getElementById('iamRealmResetPw').checked = r.reset_password_allowed !== false;
        document.getElementById('iamRealmRememberMe').checked = r.remember_me !== false;
        document.getElementById('iamRealmVerifyEmail').checked = r.verify_email === true;
        document.getElementById('iamRealmLoginEmail').checked = r.login_with_email_allowed !== false;
        document.getElementById('iamRealmBruteForce').checked = r.brute_force_protected !== false;
        document.getElementById('iamRealmPassUpper').checked = r.password_require_uppercase !== false;
        document.getElementById('iamRealmPassDigit').checked = r.password_require_digit !== false;
        document.getElementById('iamRealmPassSpecial').checked = r.password_require_special_char === true;
        document.getElementById('iamRealmModalTitle').textContent = 'Edit Realm';
        document.getElementById('iamRealmSubmitBtn').textContent = 'Update';
        openIAMModal('iamRealmModal');
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

async function submitRealm() {
    const editID = document.getElementById('iamRealmEditID').value;
    const name = document.getElementById('iamRealmName').value.trim();
    if (!name) { showIAMToast('Realm name is required', true); return; }

    const body = {
        name: name,
        display_name: document.getElementById('iamRealmDisplayName').value.trim(),
        enabled: document.getElementById('iamRealmEnabled').checked,
        registration_allowed: document.getElementById('iamRealmRegAllowed').checked,
        reset_password_allowed: document.getElementById('iamRealmResetPw').checked,
        remember_me: document.getElementById('iamRealmRememberMe').checked,
        verify_email: document.getElementById('iamRealmVerifyEmail').checked,
        login_with_email_allowed: document.getElementById('iamRealmLoginEmail').checked,
        brute_force_protected: document.getElementById('iamRealmBruteForce').checked,
        access_token_lifespan: parseInt(document.getElementById('iamRealmAccessTTL').value, 10) || 900,
        refresh_token_lifespan: parseInt(document.getElementById('iamRealmRefreshTTL').value, 10) || 604800,
        sso_session_idle_timeout: parseInt(document.getElementById('iamRealmSSOIdle').value, 10) || 1800,
        sso_session_max_lifespan: parseInt(document.getElementById('iamRealmSSOMax').value, 10) || 36000,
        password_min_length: parseInt(document.getElementById('iamRealmPassMinLen').value, 10) || 8,
        max_login_failures: parseInt(document.getElementById('iamRealmMaxFailures').value, 10) || 30,
        password_require_uppercase: document.getElementById('iamRealmPassUpper').checked,
        password_require_digit: document.getElementById('iamRealmPassDigit').checked,
        password_require_special_char: document.getElementById('iamRealmPassSpecial').checked
    };

    try {
        const url = editID ? '/iam/v2/realms/' + encodeURIComponent(editID) : '/iam/v2/realms';
        const method = editID ? 'PUT' : 'POST';
        const resp = await iamFetch(url, { method, body: JSON.stringify(body) });
        const data = await resp.json();
        if (!resp.ok) { showIAMToast(data.error || 'Operation failed', true); return; }
        showIAMToast(editID ? 'Realm updated' : 'Realm created');
        closeIAMModal('iamRealmModal');
        loadRealms();
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

async function deleteRealm(id) {
    if (!confirm('Delete this realm and all associated data?')) return;
    try {
        const resp = await iamFetch('/iam/v2/realms/' + encodeURIComponent(id), { method: 'DELETE' });
        if (!resp.ok) { const d = await resp.json().catch(() => ({})); showIAMToast(d.error || 'Delete failed', true); return; }
        showIAMToast('Realm deleted');
        loadRealms();
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

async function viewRealmDashboard(id) {
    const panel = document.getElementById('realmDetailPanel');
    if (!panel) return;
    panel.style.display = 'block';
    document.getElementById('realmDetailTitle').textContent = 'Loading…';
    document.getElementById('realmDashCards').innerHTML = '';
    document.getElementById('realmRecentEvents').textContent = 'Loading…';

    try {
        const resp = await iamFetch('/iam/v2/realms/' + encodeURIComponent(id) + '/dashboard');
        if (!resp.ok) { showIAMToast('Failed to load realm dashboard', true); panel.style.display = 'none'; return; }
        const d = await resp.json();
        document.getElementById('realmDetailTitle').textContent = 'Realm: ' + (d.realm_name || id);
        document.getElementById('realmDashCards').innerHTML =
            '<div class="cp-card"><h4>' + (d.user_count || 0) + '</h4><p>Users</p></div>' +
            '<div class="cp-card"><h4>' + (d.client_count || 0) + '</h4><p>Clients</p></div>' +
            '<div class="cp-card"><h4>' + (d.role_count || 0) + '</h4><p>Roles</p></div>' +
            '<div class="cp-card"><h4>' + (d.group_count || 0) + '</h4><p>Groups</p></div>' +
            '<div class="cp-card"><h4>' + (d.idp_count || 0) + '</h4><p>Identity Providers</p></div>' +
            '<div class="cp-card"><h4>' + (d.active_session_count || 0) + '</h4><p>Active Sessions</p></div>';

        // Recent events
        const events = d.recent_events || [];
        if (events.length) {
            document.getElementById('realmRecentEvents').innerHTML = events.map(e =>
                '<div style="padding:4px 0;border-bottom:1px solid var(--border-color);">' +
                    '<span style="font-weight:600;">' + escapeHtml(e.type || '') + '</span> — ' +
                    escapeHtml(e.user_id || '') + ' — ' + formatDate(e.CreatedAt || e.created_at) +
                '</div>'
            ).join('');
        } else {
            document.getElementById('realmRecentEvents').textContent = 'No recent events';
        }
    } catch (err) {
        showIAMToast('Error: ' + err.message, true);
        panel.style.display = 'none';
    }
}

// ── Groups CRUD ──────────────────────────────────────────────────────────────

let v2GroupsCache = [];

async function loadGroups() {
    if (!iamToken) return;
    const realmFilter = (document.getElementById('groupsRealmFilter') || {}).value || '';
    const qs = realmFilter ? '?realm_id=' + encodeURIComponent(realmFilter) : '';
    try {
        const resp = await iamFetch('/iam/v2/groups' + qs);
        if (!resp.ok) { showIAMToast('Failed to load groups: ' + resp.status, true); return; }
        const data = await resp.json();
        v2GroupsCache = Array.isArray(data) ? data : (data.groups || []);
        renderGroups(v2GroupsCache);
    } catch (err) { showIAMToast('Error loading groups: ' + err.message, true); }
}

function renderGroups(groups) {
    const tbody = document.getElementById('groupsBody');
    if (!tbody) return;
    if (!groups.length) {
        tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;color:var(--text-muted);">No groups</td></tr>';
        return;
    }
    tbody.innerHTML = groups.map(g => {
        const id = g.ID || g.id;
        const realmName = v2RealmsCache.find(r => (r.ID || r.id) === g.realm_id);
        return '<tr>' +
            '<td style="font-weight:600;">' + escapeHtml(g.name) + '</td>' +
            '<td style="font-family:var(--font-mono);font-size:.85rem;">' + escapeHtml(g.path || '/') + '</td>' +
            '<td>' + escapeHtml(realmName ? realmName.name : (g.realm_id || '–')) + '</td>' +
            '<td>' + escapeHtml(g.parent_id || '–') + '</td>' +
            '<td style="white-space:nowrap;">' +
                '<button class="btn-sm btn-secondary" onclick="editGroup(\'' + escapeHtml(id) + '\')">Edit</button> ' +
                '<button class="btn-sm" style="background:var(--danger-color);color:#fff;" onclick="deleteGroup(\'' + escapeHtml(id) + '\')">Delete</button>' +
            '</td></tr>';
    }).join('');
}

function showCreateGroupModal() {
    document.getElementById('iamGroupEditID').value = '';
    document.getElementById('iamGroupName').value = '';
    populateGroupParentDropdown('');
    document.getElementById('iamGroupModalTitle').textContent = 'Create Group';
    document.getElementById('iamGroupSubmitBtn').textContent = 'Create';
    openIAMModal('iamGroupModal');
}

function populateGroupParentDropdown(excludeId) {
    const sel = document.getElementById('iamGroupParentID');
    if (!sel) return;
    sel.innerHTML = '<option value="">— Top-level —</option>' +
        v2GroupsCache.filter(g => (g.ID || g.id) !== excludeId).map(g =>
            '<option value="' + escapeHtml(g.ID || g.id) + '">' + escapeHtml(g.path || g.name) + '</option>'
        ).join('');
}

async function editGroup(id) {
    try {
        const resp = await iamFetch('/iam/v2/groups/' + encodeURIComponent(id));
        if (!resp.ok) { showIAMToast('Failed to load group', true); return; }
        const g = await resp.json();
        document.getElementById('iamGroupEditID').value = g.ID || g.id || id;
        document.getElementById('iamGroupName').value = g.name || '';
        const realmSel = document.getElementById('iamGroupRealmID');
        if (realmSel) realmSel.value = g.realm_id || '';
        populateGroupParentDropdown(g.ID || g.id || id);
        const parentSel = document.getElementById('iamGroupParentID');
        if (parentSel) parentSel.value = g.parent_id || '';
        document.getElementById('iamGroupModalTitle').textContent = 'Edit Group';
        document.getElementById('iamGroupSubmitBtn').textContent = 'Update';
        openIAMModal('iamGroupModal');
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

async function submitGroup() {
    const editID = document.getElementById('iamGroupEditID').value;
    const name = document.getElementById('iamGroupName').value.trim();
    const realmID = document.getElementById('iamGroupRealmID').value;
    if (!name) { showIAMToast('Group name is required', true); return; }
    if (!realmID) { showIAMToast('Realm is required', true); return; }

    const body = { name, realm_id: realmID, parent_id: document.getElementById('iamGroupParentID').value || '' };

    try {
        const url = editID ? '/iam/v2/groups/' + encodeURIComponent(editID) : '/iam/v2/groups';
        const method = editID ? 'PUT' : 'POST';
        const resp = await iamFetch(url, { method, body: JSON.stringify(body) });
        const data = await resp.json();
        if (!resp.ok) { showIAMToast(data.error || 'Operation failed', true); return; }
        showIAMToast(editID ? 'Group updated' : 'Group created');
        closeIAMModal('iamGroupModal');
        loadGroups();
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

async function deleteGroup(id) {
    if (!confirm('Delete this group?')) return;
    try {
        const resp = await iamFetch('/iam/v2/groups/' + encodeURIComponent(id), { method: 'DELETE' });
        if (!resp.ok) { const d = await resp.json().catch(() => ({})); showIAMToast(d.error || 'Delete failed', true); return; }
        showIAMToast('Group deleted');
        loadGroups();
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

// ── Identity Providers CRUD ──────────────────────────────────────────────────

async function loadIdentityProviders() {
    if (!iamToken) return;
    const realmFilter = (document.getElementById('idpRealmFilter') || {}).value || '';
    const qs = realmFilter ? '?realm_id=' + encodeURIComponent(realmFilter) : '';
    try {
        const resp = await iamFetch('/iam/v2/identity-providers' + qs);
        if (!resp.ok) { showIAMToast('Failed to load identity providers: ' + resp.status, true); return; }
        const data = await resp.json();
        const idps = Array.isArray(data) ? data : (data.identity_providers || []);
        renderIdentityProviders(idps);
    } catch (err) { showIAMToast('Error loading IdPs: ' + err.message, true); }
}

function renderIdentityProviders(idps) {
    const tbody = document.getElementById('idpsBody');
    if (!tbody) return;
    if (!idps.length) {
        tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:var(--text-muted);">No identity providers configured</td></tr>';
        return;
    }
    tbody.innerHTML = idps.map(p => {
        const id = p.ID || p.id;
        const realmName = v2RealmsCache.find(r => (r.ID || r.id) === p.realm_id);
        const typeBadgeColor = { oidc: '#3498db', saml: '#e67e22', github: '#333', google: '#ea4335', microsoft: '#00a4ef', ldap: '#9b59b6', gitlab: '#fc6d26', facebook: '#1877f2' }[p.provider_type] || 'var(--text-muted)';
        return '<tr>' +
            '<td style="font-weight:600;">' + escapeHtml(p.alias) + '</td>' +
            '<td>' + escapeHtml(p.display_name || '–') + '</td>' +
            '<td><span class="status-badge" style="background:' + typeBadgeColor + ';color:#fff;">' + escapeHtml(p.provider_type || '–') + '</span></td>' +
            '<td><span class="status-badge ' + (p.enabled !== false ? 'status-active' : 'status-inactive') + '">' + (p.enabled !== false ? 'Yes' : 'No') + '</span></td>' +
            '<td>' + (p.trust_email ? 'Yes' : 'No') + '</td>' +
            '<td>' + escapeHtml(realmName ? realmName.name : (p.realm_id || '–')) + '</td>' +
            '<td style="white-space:nowrap;">' +
                '<button class="btn-sm btn-secondary" onclick="editIdentityProvider(\'' + escapeHtml(id) + '\')">Edit</button> ' +
                '<button class="btn-sm" style="background:var(--danger-color);color:#fff;" onclick="deleteIdentityProvider(\'' + escapeHtml(id) + '\')">Delete</button>' +
            '</td></tr>';
    }).join('');
}

function toggleIdPFields() {
    const type = (document.getElementById('iamIdPType') || {}).value || '';
    const oidcFields = document.getElementById('idpOidcFields');
    if (oidcFields) oidcFields.style.display = (type === 'oidc' || type === 'saml') ? '' : 'none';
}

function showCreateIdPModal() {
    document.getElementById('iamIdPEditID').value = '';
    document.getElementById('iamIdPAlias').value = '';
    document.getElementById('iamIdPDisplayName').value = '';
    document.getElementById('iamIdPType').value = 'oidc';
    document.getElementById('iamIdPClientID').value = '';
    document.getElementById('iamIdPClientSecret').value = '';
    document.getElementById('iamIdPAuthURL').value = '';
    document.getElementById('iamIdPTokenURL').value = '';
    document.getElementById('iamIdPUserInfoURL').value = '';
    document.getElementById('iamIdPIssuer').value = '';
    document.getElementById('iamIdPScopes').value = 'openid profile email';
    document.getElementById('iamIdPEnabled').checked = true;
    document.getElementById('iamIdPTrustEmail').checked = false;
    document.getElementById('iamIdPStoreToken').checked = false;
    toggleIdPFields();
    document.getElementById('iamIdPModalTitle').textContent = 'Add Identity Provider';
    document.getElementById('iamIdPSubmitBtn').textContent = 'Add';
    openIAMModal('iamIdPModal');
}

async function editIdentityProvider(id) {
    try {
        const resp = await iamFetch('/iam/v2/identity-providers/' + encodeURIComponent(id));
        if (!resp.ok) { showIAMToast('Failed to load identity provider', true); return; }
        const p = await resp.json();
        document.getElementById('iamIdPEditID').value = p.ID || p.id || id;
        document.getElementById('iamIdPAlias').value = p.alias || '';
        document.getElementById('iamIdPDisplayName').value = p.display_name || '';
        document.getElementById('iamIdPType').value = p.provider_type || 'oidc';
        document.getElementById('iamIdPClientID').value = p.client_id || '';
        document.getElementById('iamIdPClientSecret').value = p.client_secret || '';
        document.getElementById('iamIdPAuthURL').value = p.authorization_url || '';
        document.getElementById('iamIdPTokenURL').value = p.token_url || '';
        document.getElementById('iamIdPUserInfoURL').value = p.userinfo_url || '';
        document.getElementById('iamIdPIssuer').value = p.issuer || '';
        document.getElementById('iamIdPScopes').value = p.default_scopes || 'openid profile email';
        document.getElementById('iamIdPEnabled').checked = p.enabled !== false;
        document.getElementById('iamIdPTrustEmail').checked = p.trust_email === true;
        document.getElementById('iamIdPStoreToken').checked = p.store_token === true;
        const realmSel = document.getElementById('iamIdPRealmID');
        if (realmSel) realmSel.value = p.realm_id || '';
        toggleIdPFields();
        document.getElementById('iamIdPModalTitle').textContent = 'Edit Identity Provider';
        document.getElementById('iamIdPSubmitBtn').textContent = 'Update';
        openIAMModal('iamIdPModal');
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

async function submitIdentityProvider() {
    const editID = document.getElementById('iamIdPEditID').value;
    const alias = document.getElementById('iamIdPAlias').value.trim();
    const realmID = document.getElementById('iamIdPRealmID').value;
    if (!alias) { showIAMToast('Alias is required', true); return; }
    if (!realmID) { showIAMToast('Realm is required', true); return; }

    const body = {
        alias, realm_id: realmID,
        display_name: document.getElementById('iamIdPDisplayName').value.trim(),
        provider_type: document.getElementById('iamIdPType').value,
        client_id: document.getElementById('iamIdPClientID').value.trim(),
        client_secret: document.getElementById('iamIdPClientSecret').value.trim(),
        authorization_url: document.getElementById('iamIdPAuthURL').value.trim(),
        token_url: document.getElementById('iamIdPTokenURL').value.trim(),
        userinfo_url: document.getElementById('iamIdPUserInfoURL').value.trim(),
        issuer: document.getElementById('iamIdPIssuer').value.trim(),
        default_scopes: document.getElementById('iamIdPScopes').value.trim(),
        enabled: document.getElementById('iamIdPEnabled').checked,
        trust_email: document.getElementById('iamIdPTrustEmail').checked,
        store_token: document.getElementById('iamIdPStoreToken').checked
    };

    try {
        const url = editID ? '/iam/v2/identity-providers/' + encodeURIComponent(editID) : '/iam/v2/identity-providers';
        const method = editID ? 'PUT' : 'POST';
        const resp = await iamFetch(url, { method, body: JSON.stringify(body) });
        const data = await resp.json();
        if (!resp.ok) { showIAMToast(data.error || 'Operation failed', true); return; }
        showIAMToast(editID ? 'Identity provider updated' : 'Identity provider added');
        closeIAMModal('iamIdPModal');
        loadIdentityProviders();
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

async function deleteIdentityProvider(id) {
    if (!confirm('Delete this identity provider?')) return;
    try {
        const resp = await iamFetch('/iam/v2/identity-providers/' + encodeURIComponent(id), { method: 'DELETE' });
        if (!resp.ok) { const d = await resp.json().catch(() => ({})); showIAMToast(d.error || 'Delete failed', true); return; }
        showIAMToast('Identity provider deleted');
        loadIdentityProviders();
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

// ── Client Scopes CRUD ──────────────────────────────────────────────────────

async function loadClientScopes() {
    if (!iamToken) return;
    const realmFilter = (document.getElementById('scopeRealmFilter') || {}).value || '';
    const qs = realmFilter ? '?realm_id=' + encodeURIComponent(realmFilter) : '';
    try {
        const resp = await iamFetch('/iam/v2/client-scopes' + qs);
        if (!resp.ok) { showIAMToast('Failed to load client scopes: ' + resp.status, true); return; }
        const data = await resp.json();
        const scopes = Array.isArray(data) ? data : (data.scopes || []);
        renderClientScopes(scopes);
    } catch (err) { showIAMToast('Error loading scopes: ' + err.message, true); }
}

function renderClientScopes(scopes) {
    const tbody = document.getElementById('scopesBody');
    if (!tbody) return;
    if (!scopes.length) {
        tbody.innerHTML = '<tr><td colspan="9" style="text-align:center;color:var(--text-muted);">No client scopes</td></tr>';
        return;
    }
    tbody.innerHTML = scopes.map(s => {
        const id = s.ID || s.id;
        return '<tr>' +
            '<td style="font-weight:600;">' + escapeHtml(s.name) + '</td>' +
            '<td>' + escapeHtml(s.description || '–') + '</td>' +
            '<td>' + escapeHtml(s.protocol || 'openid-connect') + '</td>' +
            '<td style="font-family:var(--font-mono);font-size:.85rem;">' + escapeHtml(s.claim_name || '–') + '</td>' +
            '<td>' + (s.include_in_id_token ? 'Yes' : 'No') + '</td>' +
            '<td>' + (s.include_in_access_token ? 'Yes' : 'No') + '</td>' +
            '<td>' + (s.include_in_userinfo ? 'Yes' : 'No') + '</td>' +
            '<td><span class="status-badge ' + (s.built_in ? 'status-active' : '') + '">' + (s.built_in ? 'Yes' : 'No') + '</span></td>' +
            '<td style="white-space:nowrap;">' +
                (s.built_in ? '<span style="color:var(--text-muted);font-size:.8rem;">Protected</span>' :
                    '<button class="btn-sm btn-secondary" onclick="editClientScope(\'' + escapeHtml(id) + '\')">Edit</button> ' +
                    '<button class="btn-sm" style="background:var(--danger-color);color:#fff;" onclick="deleteClientScope(\'' + escapeHtml(id) + '\')">Delete</button>') +
            '</td></tr>';
    }).join('');
}

function showCreateScopeModal() {
    document.getElementById('iamScopeEditID').value = '';
    document.getElementById('iamScopeName').value = '';
    document.getElementById('iamScopeDescription').value = '';
    document.getElementById('iamScopeClaimName').value = '';
    document.getElementById('iamScopeClaimType').value = '';
    document.getElementById('iamScopeIDToken').checked = true;
    document.getElementById('iamScopeAccessToken').checked = true;
    document.getElementById('iamScopeUserInfo').checked = true;
    document.getElementById('iamScopeModalTitle').textContent = 'Create Client Scope';
    document.getElementById('iamScopeSubmitBtn').textContent = 'Create';
    openIAMModal('iamScopeModal');
}

async function editClientScope(id) {
    try {
        const resp = await iamFetch('/iam/v2/client-scopes/' + encodeURIComponent(id));
        if (!resp.ok) { showIAMToast('Failed to load scope', true); return; }
        const s = await resp.json();
        document.getElementById('iamScopeEditID').value = s.ID || s.id || id;
        document.getElementById('iamScopeName').value = s.name || '';
        document.getElementById('iamScopeDescription').value = s.description || '';
        document.getElementById('iamScopeClaimName').value = s.claim_name || '';
        document.getElementById('iamScopeClaimType').value = s.claim_type || '';
        document.getElementById('iamScopeIDToken').checked = s.include_in_id_token !== false;
        document.getElementById('iamScopeAccessToken').checked = s.include_in_access_token !== false;
        document.getElementById('iamScopeUserInfo').checked = s.include_in_userinfo !== false;
        const realmSel = document.getElementById('iamScopeRealmID');
        if (realmSel) realmSel.value = s.realm_id || '';
        document.getElementById('iamScopeModalTitle').textContent = 'Edit Client Scope';
        document.getElementById('iamScopeSubmitBtn').textContent = 'Update';
        openIAMModal('iamScopeModal');
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

async function submitClientScope() {
    const editID = document.getElementById('iamScopeEditID').value;
    const name = document.getElementById('iamScopeName').value.trim();
    const realmID = document.getElementById('iamScopeRealmID').value;
    if (!name) { showIAMToast('Scope name is required', true); return; }
    if (!realmID) { showIAMToast('Realm is required', true); return; }

    const body = {
        name, realm_id: realmID,
        description: document.getElementById('iamScopeDescription').value.trim(),
        claim_name: document.getElementById('iamScopeClaimName').value.trim(),
        claim_type: document.getElementById('iamScopeClaimType').value,
        include_in_id_token: document.getElementById('iamScopeIDToken').checked,
        include_in_access_token: document.getElementById('iamScopeAccessToken').checked,
        include_in_userinfo: document.getElementById('iamScopeUserInfo').checked
    };

    try {
        const url = editID ? '/iam/v2/client-scopes/' + encodeURIComponent(editID) : '/iam/v2/client-scopes';
        const method = editID ? 'PUT' : 'POST';
        const resp = await iamFetch(url, { method, body: JSON.stringify(body) });
        const data = await resp.json();
        if (!resp.ok) { showIAMToast(data.error || 'Operation failed', true); return; }
        showIAMToast(editID ? 'Client scope updated' : 'Client scope created');
        closeIAMModal('iamScopeModal');
        loadClientScopes();
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

async function deleteClientScope(id) {
    if (!confirm('Delete this client scope?')) return;
    try {
        const resp = await iamFetch('/iam/v2/client-scopes/' + encodeURIComponent(id), { method: 'DELETE' });
        if (!resp.ok) { const d = await resp.json().catch(() => ({})); showIAMToast(d.error || 'Delete failed', true); return; }
        showIAMToast('Client scope deleted');
        loadClientScopes();
    } catch (err) { showIAMToast('Error: ' + err.message, true); }
}

// ── Events (Audit Log) ──────────────────────────────────────────────────────

async function loadEvents() {
    if (!iamToken) return;
    const typeFilter = (document.getElementById('eventTypeFilter') || {}).value || '';
    const realmFilter = (document.getElementById('eventRealmFilter') || {}).value || '';
    const limit = parseInt((document.getElementById('eventLimitInput') || {}).value, 10) || 100;

    const params = new URLSearchParams();
    if (typeFilter) params.set('type', typeFilter);
    if (realmFilter) params.set('realm_id', realmFilter);
    params.set('limit', String(limit));
    const qs = '?' + params.toString();

    try {
        const resp = await iamFetch('/iam/v2/events' + qs);
        if (!resp.ok) { showIAMToast('Failed to load events: ' + resp.status, true); return; }
        const data = await resp.json();
        const events = Array.isArray(data) ? data : (data.events || []);
        renderEvents(events);
    } catch (err) { showIAMToast('Error loading events: ' + err.message, true); }
}

function renderEvents(events) {
    const tbody = document.getElementById('eventsBody');
    if (!tbody) return;
    if (!events.length) {
        tbody.innerHTML = '<tr><td colspan="7" style="text-align:center;color:var(--text-muted);">No events found</td></tr>';
        return;
    }
    tbody.innerHTML = events.map(e => {
        const typeColors = { LOGIN: '#27ae60', LOGIN_ERROR: '#e74c3c', LOGOUT: '#f39c12', REGISTER: '#3498db', TOKEN: '#9b59b6', ADMIN: '#2c3e50' };
        const color = typeColors[e.type] || 'var(--text-muted)';
        return '<tr>' +
            '<td style="font-size:.85rem;">' + formatDate(e.CreatedAt || e.created_at) + '</td>' +
            '<td><span class="status-badge" style="background:' + color + ';color:#fff;">' + escapeHtml(e.type || '–') + '</span></td>' +
            '<td style="font-family:var(--font-mono);font-size:.8rem;">' + escapeHtml(e.user_id || '–') + '</td>' +
            '<td style="font-family:var(--font-mono);font-size:.8rem;">' + escapeHtml(e.client_id || '–') + '</td>' +
            '<td>' + escapeHtml(e.ip_address || '–') + '</td>' +
            '<td style="font-size:.85rem;max-width:200px;overflow:hidden;text-overflow:ellipsis;">' + escapeHtml(e.details || '–') + '</td>' +
            '<td style="color:var(--danger-color);font-size:.85rem;">' + escapeHtml(e.error || '') + '</td>' +
            '</tr>';
    }).join('');
}

// ── Initialisation ───────────────────────────────────────────────────────────

function initIAMAdminPage() {
    // Only run if we're on the IAM admin page
    if (!document.getElementById('iam-dashboard')) return;

    // Reuse platform session token if IAM-specific token is not set yet.
    if (!iamToken) {
        const sharedToken = localStorage.getItem('authToken') || '';
        if (sharedToken) {
            iamToken = sharedToken;
            localStorage.setItem('iamToken', iamToken);
        }
    }
    if (!iamRefreshToken) {
        const sharedRefresh = localStorage.getItem('refreshToken') || '';
        if (sharedRefresh) {
            iamRefreshToken = sharedRefresh;
            localStorage.setItem('iamRefreshToken', iamRefreshToken);
        }
    }

    updateIAMLoginBanner();
    loadIAMDashboard();
    loadServiceAccessInfo();
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initIAMAdminPage);
} else {
    initIAMAdminPage();
}
