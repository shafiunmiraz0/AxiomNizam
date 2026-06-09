// Authentication Module
const AUTH_CONFIG = (() => {
    // Prefer shared backend resolver to avoid stale localhost fallbacks.
    let apiURL = (typeof window.resolveBackendURL === 'function')
        ? window.resolveBackendURL()
        : String(window.BACKEND_URL || '').trim();

    if (!apiURL) {
        const host = String(window.location.hostname || '').toLowerCase();
        if (host && host !== 'localhost' && host !== '127.0.0.1' && host !== '0.0.0.0') {
            const protocol = window.location.protocol || 'https:';
            if (host.indexOf('axiomnizam.') === 0) {
                apiURL = protocol + '//axiomnizam-platform.' + host.substring('axiomnizam.'.length);
            } else {
                apiURL = protocol + '//' + host;
            }
        } else {
            apiURL = 'http://localhost:8000';
        }
    }

    if (apiURL.length > 1 && apiURL.endsWith('/')) {
        apiURL = apiURL.slice(0, -1);
    }
    return {
        apiURL: apiURL,
        loginEndpoint: '/auth/login',
        refreshEndpoint: '/auth/refresh'
    };
})();

console.log('🔐 Auth Config:', AUTH_CONFIG);

let authToken = null;
let refreshToken = null;
let userName = null;
let userRole = 'user';

function normalizeRole(role) {
    const value = String(role || '').toLowerCase().trim();
    if (!value) return 'user';
    if (value === 'sysadmin' || value === 'sys-admin' || value === 'system-admin' || value === 'system_admin' || value === 'system-manager' || value === 'system manager' || value === 'systemadministrator' || value === 'system-administrator' || value === 'system administrator') {
        return 'system-manager';
    }
    if (value === 'manager' || value === 'api-manager' || value === 'api_manager') {
        return 'manager';
    }
    if (value === 'admin' || value === 'administrator' || value === 'superadmin' || value === 'super-admin') {
        return 'admin';
    }
    if (value.indexOf('system') !== -1 && value.indexOf('admin') !== -1) {
        return 'system-manager';
    }
    if (value.indexOf('admin') !== -1 && value.indexOf('account') === -1) {
        return 'admin';
    }
    return value;
}

function roleFromAlias(value) {
    const raw = String(value || '').toLowerCase().trim();
    if (!raw) return '';

    const localPart = raw.indexOf('@') !== -1 ? raw.split('@')[0] : raw;
    const compact = localPart.replace(/[^a-z0-9]/g, '');

    if (compact === 'sysadmin' || compact === 'systemadmin' || compact === 'systemadministrator' || compact === 'systemmanager') {
        return 'system-manager';
    }
    if (compact === 'admin' || compact === 'administrator' || compact === 'superadmin') {
        return 'admin';
    }
    if (compact === 'manager' || compact === 'apimanager' || compact === 'mgr') {
        return 'manager';
    }
    return '';
}

function roleFromRoleList(roles) {
    if (!Array.isArray(roles)) {
        return normalizeRole(roles || '');
    }

    for (let i = 0; i < roles.length; i++) {
        const normalized = normalizeRole(roles[i]);
        if (normalized === 'system-manager') return 'system-manager';
    }
    for (let i = 0; i < roles.length; i++) {
        const normalized = normalizeRole(roles[i]);
        if (normalized === 'admin') return 'admin';
    }
    for (let i = 0; i < roles.length; i++) {
        const normalized = normalizeRole(roles[i]);
        if (normalized === 'manager') return 'manager';
    }
    return 'user';
}

function resolveBestRole(loginData, token, submittedUsername) {
    const candidates = [
        normalizeRole(loginData && loginData.role),
        roleFromRoleList(loginData && loginData.user && loginData.user.roles),
        extractUserRole(token),
        roleFromAlias(loginData && loginData.username),
        roleFromAlias(loginData && loginData.user && loginData.user.email),
        roleFromAlias(loginData && loginData.user && loginData.user.display_name),
        roleFromAlias(submittedUsername),
    ];

    for (let i = 0; i < candidates.length; i++) {
        const role = normalizeRole(candidates[i]);
        if (role && role !== 'user') {
            return role;
        }
    }

    return 'user';
}

function readCookie(name) {
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

function setAuthCookies(token, role, name) {
    const maxAge = 60 * 60 * 12;
    document.cookie = 'authToken=' + encodeURIComponent(token || '') + '; path=/; max-age=' + maxAge + '; SameSite=Lax';
    document.cookie = 'userRole=' + encodeURIComponent(normalizeRole(role)) + '; path=/; max-age=' + maxAge + '; SameSite=Lax';
    document.cookie = 'userName=' + encodeURIComponent(name || '') + '; path=/; max-age=' + maxAge + '; SameSite=Lax';
}

function clearAuthCookies() {
    document.cookie = 'authToken=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT; SameSite=Lax';
    document.cookie = 'userRole=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT; SameSite=Lax';
    document.cookie = 'userName=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT; SameSite=Lax';
}

function defaultPathForRole(role) {
    const normalized = normalizeRole(role);
    if (normalized === 'system-manager') return '/system-manager';
    if (normalized === 'admin') return '/admin';
    if (normalized === 'manager') return '/manager';
    return '/';
}

function canAccessPath(path, role) {
    const normalized = normalizeRole(role);
    if (path === '/governance') return normalized === 'admin' || normalized === 'system-manager';
    if (path === '/operations-center') return normalized === 'admin' || normalized === 'system-manager' || normalized === 'manager';
    if (path === '/lineage-version') return normalized === 'admin' || normalized === 'system-manager';
    if (path === '/admin') return normalized === 'admin' || normalized === 'system-manager';
    if (path === '/system-manager') return normalized === 'system-manager';
    if (path === '/iam-admin') return normalized === 'system-manager';
    if (path === '/manager') return normalized === 'manager';
    return true;
}

function isProtectedPath(path) {
    return path === '/admin' ||
        path === '/system-manager' ||
        path === '/manager' ||
        path === '/governance' ||
        path === '/operations-center' ||
        path === '/lineage-version' ||
        path === '/iam-admin';
}

function consumeOAuthErrorQuery() {
    if (window.location.pathname !== '/login') return;
    const params = new URLSearchParams(window.location.search || '');
    const oauthError = params.get('oauth_error');
    if (!oauthError) return;

    alert('OAuth login failed: ' + oauthError);
    params.delete('oauth_error');

    if (window.history && window.history.replaceState) {
        const nextQuery = params.toString();
        const cleanedURL = window.location.pathname + (nextQuery ? ('?' + nextQuery) : '') + (window.location.hash || '');
        window.history.replaceState({}, document.title, cleanedURL);
    }
}

function completeOAuthLoginFromFragment() {
    const hash = String(window.location.hash || '').replace(/^#/, '');
    if (!hash || hash.indexOf('oauth_access_token=') === -1) {
        return false;
    }

    const params = new URLSearchParams(hash);
    const token = String(params.get('oauth_access_token') || '').trim();
    if (!token) {
        return false;
    }

    const oauthRefresh = String(params.get('oauth_refresh_token') || '').trim();
    const oauthUser = String(params.get('oauth_username') || '').trim();
    const oauthRole = normalizeRole(String(params.get('oauth_role') || '').trim() || extractUserRole(token) || 'user');
    const oauthReturnTo = String(params.get('oauth_return_to') || '').trim();

    authToken = token;
    refreshToken = oauthRefresh || null;
    userName = oauthUser || readCookie('userName') || 'OAuth User';
    userRole = oauthRole;

    localStorage.setItem('authToken', authToken);
    localStorage.setItem('userRole', userRole);
    localStorage.setItem('userName', userName);
    if (refreshToken) {
        localStorage.setItem('refreshToken', refreshToken);
    } else {
        localStorage.removeItem('refreshToken');
    }
    setAuthCookies(authToken, userRole, userName);

    if (window.history && window.history.replaceState) {
        const cleaned = window.location.pathname + (window.location.search || '');
        window.history.replaceState({}, document.title, cleaned);
    }

    if (oauthReturnTo && oauthReturnTo.charAt(0) === '/' && oauthReturnTo !== '/login' && canAccessPath(oauthReturnTo, userRole)) {
        window.location.href = oauthReturnTo;
    } else {
        window.location.href = defaultPathForRole(userRole);
    }
    return true;
}

// Initialize authentication on page load
window.addEventListener('DOMContentLoaded', function() {
    if (completeOAuthLoginFromFragment()) {
        return;
    }

    consumeOAuthErrorQuery();

    authToken = localStorage.getItem('authToken');
    refreshToken = localStorage.getItem('refreshToken');
    userName = localStorage.getItem('userName');
    userRole = normalizeRole(localStorage.getItem('userRole') || 'user');

    if (!authToken) {
        authToken = readCookie('authToken');
        userRole = normalizeRole(readCookie('userRole') || userRole);
        userName = readCookie('userName') || userName;

        if (authToken) {
            localStorage.setItem('authToken', authToken);
            localStorage.setItem('userRole', userRole);
            if (userName) {
                localStorage.setItem('userName', userName);
            }
        }
    } else {
        setAuthCookies(authToken, userRole, userName);
    }

    const path = window.location.pathname;

    // If on login page and already authenticated, redirect to returnTo or default
    if (authToken && (path === '/login' || path === '/signup')) {
        var returnTo = localStorage.getItem('returnTo');
        localStorage.removeItem('returnTo');
        if (returnTo && returnTo.charAt(0) === '/' && returnTo !== '/login' && returnTo !== '/signup') {
            window.location.href = returnTo;
        } else {
            window.location.href = defaultPathForRole(userRole);
        }
        return;
    }

    if (!authToken && isProtectedPath(path)) {
        window.location.href = '/login';
        return;
    }

    if (authToken && !canAccessPath(path, userRole)) {
        window.location.href = defaultPathForRole(userRole);
    }
});

function handleLogin(event) {
    event.preventDefault();
    const username = document.getElementById('username').value;
    const password = document.getElementById('password').value;

    const loginBtn = event.target.querySelector('button[type="submit"]');
    const originalText = loginBtn.textContent;
    loginBtn.disabled = true;
    loginBtn.textContent = 'Logging in...';

    const loginURL = AUTH_CONFIG.apiURL + AUTH_CONFIG.loginEndpoint;
    console.log('🔐 Attempting login to:', loginURL);

    // Send credentials to backend API (which proxies IAM auth securely)
    fetch(loginURL, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            username: username,
            password: password
        })
    })
    .then(function(response) {
        console.log('📡 Login response status:', response.status);
        return response.text().then(function(rawBody) {
            var data = {};
            if (rawBody) {
                try {
                    data = JSON.parse(rawBody);
                } catch (parseErr) {
                    var compactBody = rawBody.replace(/\s+/g, ' ').trim();
                    if (compactBody.length > 180) {
                        compactBody = compactBody.substring(0, 180) + '...';
                    }
                    data = {
                        error: 'Login endpoint returned non-JSON response (status ' + response.status + '): ' + compactBody
                    };
                }
            }

            if (!response.ok) {
                throw new Error(data.error || 'Login failed: ' + response.statusText);
            }

            if (!data || !data.access_token) {
                throw new Error(data.error || 'Login succeeded but no access token was returned');
            }

            return data;
        });
    })
    .then(function(data) {
        console.log('✅ Login successful:', data);
        authToken = data.access_token;
        refreshToken = data.refresh_token || null;
        userName = data.username;
        localStorage.setItem('authToken', authToken);
        if (refreshToken) {
            localStorage.setItem('refreshToken', refreshToken);
        }
        localStorage.setItem('userName', userName);
        
        const userRole = resolveBestRole(data, authToken, username);

        console.log('👤 User role:', userRole, '(server role:', normalizeRole(data.role || ''), ', user.roles:', (data.user && data.user.roles) || [], ')');
        localStorage.setItem('userRole', userRole);
        setAuthCookies(authToken, userRole, userName);

        // Check for return URL from landing page feature click
        var returnTo = localStorage.getItem('returnTo');
        localStorage.removeItem('returnTo');

        if (returnTo && returnTo.charAt(0) === '/' && returnTo !== '/login' && returnTo !== '/signup') {
            window.location.href = returnTo;
        } else if (userRole === 'system-manager') {
            window.location.href = '/system-manager';
        } else if (userRole === 'admin') {
            window.location.href = '/admin';
        } else if (userRole === 'manager') {
            window.location.href = '/manager';
        } else {
            window.location.href = '/';
        }
    })
    .catch(function(error) {
        console.error('❌ Login error:', error);
        loginBtn.disabled = false;
        loginBtn.textContent = originalText;
        alert('Login failed: ' + error.message);
    });
}

function logout() {
    authToken = null;
    userName = null;
    localStorage.removeItem('authToken');
    localStorage.removeItem('userName');
    localStorage.removeItem('userRole');
    localStorage.removeItem('refreshToken');
    clearAuthCookies();
    window.location.href = '/';
}

// Decode JWT token and extract user role
function extractUserRole(token) {
    try {
        // JWT format: header.payload.signature
        const parts = token.split('.');
        if (parts.length !== 3) {
            console.warn('⚠️ Invalid token format');
            return 'user';
        }
        
        // Decode the payload (second part)
        const payload = parts[1].replace(/-/g, '+').replace(/_/g, '/');
        // Add padding if needed
        const padded = payload + '='.repeat((4 - payload.length % 4) % 4);
        const decoded = JSON.parse(atob(padded));
        
        console.log('🔐 Full token payload:', JSON.stringify(decoded, null, 2));
        
        // Check for IAM top-level roles first.
        if (Array.isArray(decoded.roles)) {
            const directRoles = decoded.roles;
            console.log('📋 IAM roles found:', directRoles);
            for (let i = 0; i < directRoles.length; i++) {
                const role = String(directRoles[i] || '').toLowerCase();
                if (role === 'system-manager' || role === 'system_manager' || role === 'system-admin' || role === 'sysadmin') {
                    return 'system-manager';
                }
                if (role.includes('admin') && !role.includes('account')) {
                    return 'admin';
                }
                if (role === 'manager' || role === 'api-manager' || role === 'api_manager') {
                    return 'manager';
                }
            }
        }

        // Check realm roles for legacy compatibility.
        if (decoded.realm_access && decoded.realm_access.roles) {
            const roles = decoded.realm_access.roles;
            console.log('📋 Realm roles found:', roles);
            
            // Check each role and determine user type
            for (let i = 0; i < roles.length; i++) {
                const role = roles[i].toLowerCase();
                console.log(`  - Checking role: "${role}"`);
                
                // Check for admin role
                // Check for system-manager role (various formats)
                if (role === 'system-manager' || role === 'system_manager' || role === 'system-admin' || role === 'sysadmin') {
                    console.log('✅ Detected as SYSTEM-MANAGER role');
                    return 'system-manager';
                }

                if (role.includes('admin') && !role.includes('account')) {
                    console.log('✅ Detected as ADMIN role');
                    return 'admin';
                }

                // Check for manager role (must come AFTER system-manager check)
                if (role === 'manager' || role === 'api-manager' || role === 'api_manager') {
                    console.log('✅ Detected as MANAGER role');
                    return 'manager';
                }
            }
        }
        
        // Check for client roles
        if (decoded.resource_access) {
            const clientRoles = decoded.resource_access['axiomnizam-backend'];
            if (clientRoles && clientRoles.roles) {
                console.log('📋 Client roles found:', clientRoles.roles);
                
                for (let i = 0; i < clientRoles.roles.length; i++) {
                    const role = clientRoles.roles[i].toLowerCase();
                    if (role === 'system-manager' || role === 'system_manager' || role === 'sysadmin') return 'system-manager';
                    if (role.includes('admin')) return 'admin';
                }
            }
        }
        
        console.log('ℹ️ No admin/manager roles found, defaulting to user');
        return 'user';
    } catch (error) {
        console.error('❌ Error decoding token:', error);
        return 'user';
    }
}

function getAuthHeaders() {
    const headers = {
        'Content-Type': 'application/json'
    };

    // Always resolve the latest token from storage/cookies to avoid stale in-memory state.
    const latestToken = localStorage.getItem('authToken') || readCookie('authToken') || authToken;
    if (latestToken) {
        authToken = latestToken;
        headers['Authorization'] = 'Bearer ' + latestToken;
    }

    return headers;
}

function isAuthenticated() {
    return authToken !== null && authToken !== undefined;
}

function isAdmin() {
    return userRole === 'admin' || userRole === 'system-manager';
}

// ═══════════════════════════════════════════════
// AUTOMATIC TOKEN REFRESH MECHANISM
// ═══════════════════════════════════════════════

// Extract the expiry timestamp (epoch seconds) from a JWT token.
function getTokenExpiry(token) {
    try {
        if (!token) return 0;
        const parts = token.split('.');
        if (parts.length !== 3) return 0;
        const payload = parts[1].replace(/-/g, '+').replace(/_/g, '/');
        const padded = payload + '='.repeat((4 - payload.length % 4) % 4);
        const decoded = JSON.parse(atob(padded));
        return decoded.exp || 0;
    } catch (e) {
        return 0;
    }
}

// Returns remaining seconds until the current access token expires.
function getTokenRemainingSeconds() {
    const token = localStorage.getItem('authToken') || authToken;
    if (!token) return 0;
    const exp = getTokenExpiry(token);
    if (!exp) return 0;
    return Math.max(0, exp - Math.floor(Date.now() / 1000));
}

// Guard to prevent concurrent refresh calls.
let _refreshInFlight = null;

// Refresh the access token using the stored refresh token.
// Returns a promise that resolves to the new access token or null on failure.
function refreshAccessToken() {
    // If a refresh is already in-flight, piggyback on it.
    if (_refreshInFlight) return _refreshInFlight;

    const currentRefreshToken = localStorage.getItem('refreshToken') || refreshToken;
    if (!currentRefreshToken) {
        console.warn('🔄 No refresh token available — cannot refresh session');
        return Promise.resolve(null);
    }

    const refreshURL = AUTH_CONFIG.apiURL + AUTH_CONFIG.refreshEndpoint;
    console.log('🔄 Refreshing access token via:', refreshURL);

    _refreshInFlight = fetch(refreshURL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: currentRefreshToken })
    })
    .then(function(response) {
        if (!response.ok) {
            return response.text().then(function(body) {
                var detail = '';
                try { detail = JSON.parse(body).error || body; } catch(e) { detail = body; }
                throw new Error('Refresh failed (' + response.status + '): ' + detail);
            });
        }
        return response.json();
    })
    .then(function(data) {
        var newAccess = data.access_token || '';
        var newRefresh = data.refresh_token || '';

        if (!newAccess) {
            throw new Error('Refresh response missing access_token');
        }

        // Update in-memory state.
        authToken = newAccess;
        refreshToken = newRefresh || currentRefreshToken;

        // Persist to storage.
        localStorage.setItem('authToken', authToken);
        if (newRefresh) {
            localStorage.setItem('refreshToken', newRefresh);
        }

        // Use role and username from refresh response if available,
        // otherwise fall back to existing values or extract from token.
        var resolvedRole = normalizeRole(data.role || '') || userRole || normalizeRole(localStorage.getItem('userRole') || extractUserRole(authToken));
        var resolvedName = (data.username || '').trim() || userName || localStorage.getItem('userName') || '';

        // Persist updated role/username.
        userRole = resolvedRole;
        userName = resolvedName;
        localStorage.setItem('userRole', resolvedRole);
        if (resolvedName) {
            localStorage.setItem('userName', resolvedName);
        }
        setAuthCookies(authToken, resolvedRole, resolvedName);

        console.log('✅ Token refreshed successfully, new expiry in', getTokenRemainingSeconds(), 'seconds');

        // Re-schedule the next proactive refresh.
        scheduleTokenRefresh();

        return authToken;
    })
    .catch(function(err) {
        console.error('❌ Token refresh failed:', err.message);
        // If refresh fails, the session is truly expired — force re-login.
        handleSessionExpired();
        return null;
    })
    .finally(function() {
        _refreshInFlight = null;
    });

    return _refreshInFlight;
}

// Handle a fully expired session (refresh token invalid/expired).
function handleSessionExpired() {
    console.warn('⏰ Session expired — redirecting to login');
    authToken = null;
    refreshToken = null;
    localStorage.removeItem('authToken');
    localStorage.removeItem('refreshToken');
    localStorage.removeItem('userName');
    localStorage.removeItem('userRole');
    clearAuthCookies();

    // Only redirect if we are on a protected page.
    var path = window.location.pathname;
    if (isProtectedPath(path) || path === '/') {
        window.location.href = '/login';
    }
}

// ── Proactive refresh timer ──
// Refreshes the token automatically ~2 minutes before it expires.
var _refreshTimerId = null;
var REFRESH_MARGIN_SECONDS = 120; // Refresh 2 minutes before expiry.

function scheduleTokenRefresh() {
    // Clear any existing timer.
    if (_refreshTimerId) {
        clearTimeout(_refreshTimerId);
        _refreshTimerId = null;
    }

    var remaining = getTokenRemainingSeconds();
    if (remaining <= 0) return; // Already expired, nothing to schedule.

    // If remaining time is less than our margin, refresh immediately.
    var delay = Math.max(0, remaining - REFRESH_MARGIN_SECONDS) * 1000;
    if (delay <= 0) {
        // Token is about to expire — refresh now.
        refreshAccessToken();
        return;
    }

    console.log('⏱️ Token refresh scheduled in', Math.round(delay / 1000), 'seconds (token expires in', remaining, 's)');

    _refreshTimerId = setTimeout(function() {
        _refreshTimerId = null;
        var hasRefresh = localStorage.getItem('refreshToken') || refreshToken;
        if (!hasRefresh) return;

        refreshAccessToken();
    }, delay);
}

// ── Fetch interceptor ──
// Wraps window.fetch to automatically retry on 401 with a refreshed token.
(function installFetchInterceptor() {
    var _originalFetch = window.fetch;

    window.fetch = function(input, init) {
        return _originalFetch.call(this, input, init).then(function(response) {
            // If the request returned 401 and we have a refresh token, try once.
            if (response.status === 401) {
                var hasRefresh = localStorage.getItem('refreshToken') || refreshToken;
                if (!hasRefresh) return response;

                // Check if this request had an Authorization header (skip public endpoints).
                var hadAuth = false;
                if (init && init.headers) {
                    if (typeof init.headers === 'object' && init.headers['Authorization']) hadAuth = true;
                    if (typeof init.headers.get === 'function' && init.headers.get('Authorization')) hadAuth = true;
                }
                if (!hadAuth) return response;

                console.log('🔄 Got 401 — attempting token refresh before retry');
                return refreshAccessToken().then(function(newToken) {
                    if (!newToken) return response; // Refresh failed, return original 401.

                    // Clone the request with the new token.
                    var newInit = Object.assign({}, init || {});
                    if (!newInit.headers || typeof newInit.headers !== 'object') {
                        newInit.headers = {};
                    }
                    // Handle both plain objects and Headers instances.
                    if (typeof newInit.headers.set === 'function') {
                        newInit.headers.set('Authorization', 'Bearer ' + newToken);
                    } else {
                        newInit.headers = Object.assign({}, newInit.headers);
                        newInit.headers['Authorization'] = 'Bearer ' + newToken;
                    }

                    console.log('🔄 Retrying request with refreshed token');
                    return _originalFetch.call(window, input, newInit);
                });
            }
            return response;
        });
    };
})();

// ── Bootstrap ──
// Start the proactive refresh timer on page load.
window.addEventListener('DOMContentLoaded', function() {
    // Small delay to let the main DOMContentLoaded handler in auth.js run first.
    setTimeout(function() {
        if (localStorage.getItem('authToken') && localStorage.getItem('refreshToken')) {
            scheduleTokenRefresh();
        }
    }, 500);
});
