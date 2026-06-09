let authToken = null;
let userName = null;
const IAM_API_BASE = (() => {
    if (typeof window.resolveBackendURL === 'function') {
        return window.resolveBackendURL();
    }

    const fromWindow = String(window.BACKEND_URL || '').trim();
    if (fromWindow) {
        return fromWindow.endsWith('/') ? fromWindow.slice(0, -1) : fromWindow;
    }

    const host = String(window.location.hostname || '').toLowerCase();
    if (host && host !== 'localhost' && host !== '127.0.0.1' && host !== '0.0.0.0') {
        const protocol = window.location.protocol || 'https:';
        if (host.indexOf('axiomnizam.') === 0) {
            return protocol + '//axiomnizam-platform.' + host.substring('axiomnizam.'.length);
        }
        return protocol + '//' + host;
    }

    return 'http://localhost:8000';
})();

// Check if user is already logged in
window.addEventListener('DOMContentLoaded', function() {
    const token = localStorage.getItem('authToken');
    const user = localStorage.getItem('userName');
    
    if (token && user) {
        authToken = token;
        userName = user;
        showDashboard();
        loadStatusData();
        loadAPIs();
        setInterval(loadStatusData, 30000); // Refresh every 30 seconds
    } else {
        showLogin();
    }
});

function showLogin() {
    document.getElementById('loginScreen').style.display = 'flex';
    document.getElementById('dashboard').style.display = 'none';
}

function showDashboard() {
    document.getElementById('loginScreen').style.display = 'none';
    document.getElementById('dashboard').style.display = 'block';
    document.getElementById('userName').textContent = userName || 'Admin';
}

function initiateIAMLogin() {
    // For demo, show a simple username/password prompt
    // In production, you would use a dedicated IAM login form
    const username = prompt('Enter username (admin):');
    if (username) {
        const password = prompt('Enter password (admin):');
        if (password) {
            loginWithIAM(username, password);
        }
    }
}

function loginWithIAM(username, password) {
    const body = {
        email: username,
        password: password
    };

    fetch(IAM_API_BASE + '/iam/auth/login', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(body)
    })
    .then(function(response) {
        if (!response.ok) throw new Error('Login failed');
        return response.json();
    })
    .then(function(data) {
        authToken = data.access_token;
        userName = (data.user && data.user.display_name) || (data.user && data.user.email) || username;
        localStorage.setItem('authToken', authToken);
        localStorage.setItem('userName', userName);
        showDashboard();
        loadStatusData();
        loadAPIs();
        setInterval(loadStatusData, 30000);
    })
    .catch(function(error) {
        alert('Login failed: ' + error.message);
    });
}

function logout() {
    authToken = null;
    userName = null;
    localStorage.removeItem('authToken');
    localStorage.removeItem('userName');
    showLogin();
}

function showTab(tabName) {
    // Hide all tabs
    var tabs = document.getElementsByClassName('tab-content');
    for (var i = 0; i < tabs.length; i++) {
        tabs[i].classList.remove('active');
    }
    
    // Remove active from all buttons
    var buttons = document.getElementsByClassName('tab-btn');
    for (var i = 0; i < buttons.length; i++) {
        buttons[i].classList.remove('active');
    }
    
    // Show selected tab
    document.getElementById(tabName).classList.add('active');
    
    // Add active to clicked button
    event.target.classList.add('active');
}

function loadStatusData() {
    fetch(IAM_API_BASE + '/api/health')
        .then(function(response) { return response.json(); })
        .then(function(data) {
            document.getElementById('overallStatus').textContent = data.status ? data.status.toUpperCase() : 'UNKNOWN';
            document.getElementById('overallStatus').className = 'status-value ' + (data.status === 'ok' ? 'ok' : 'error');
            document.getElementById('lastUpdated').textContent = new Date().toLocaleTimeString();
        })
        .catch(function(error) {
            document.getElementById('overallStatus').textContent = 'Error';
            document.getElementById('overallStatus').className = 'status-value error';
        });

    fetch(IAM_API_BASE + '/api/status')
        .then(function(response) { return response.json(); })
        .then(function(data) {
            const databases = data.data || data.databases || {};
            let html = '';
            
            const sortedDbs = Object.entries(databases).sort(function(a, b) {
                return a[0].localeCompare(b[0]);
            });
            
            for (let i = 0; i < sortedDbs.length; i++) {
                const dbName = sortedDbs[i][0];
                const dbStatus = sortedDbs[i][1];
                const isConnected = dbStatus.toLowerCase().includes('connected');
                
                html += '<div class="status-item">' +
                    '<div class="status-label">' + capitalizeFirstLetter(dbName) + '</div>' +
                    '<div class="status-value ' + (isConnected ? 'ok' : 'error') + '">' + dbStatus + '</div>' +
                    '</div>';
            }
            
            document.getElementById('dbStatus').innerHTML = html;
        })
        .catch(function(error) {
            document.getElementById('dbStatus').innerHTML = '<div class="error-message">Failed to load database status</div>';
        });
}

function loadAPIs() {
    const apiCategories = {
        'Health & Status': [
            { method: 'GET', path: '/health', url: IAM_API_BASE + '/health', description: 'Health check', auth: false },
            { method: 'GET', path: '/status', url: IAM_API_BASE + '/status', description: 'Check all connections', auth: false },
        ],
        'Notifications': [
            { method: 'POST', path: '/api/notifications/send', url: IAM_API_BASE + '/api/notifications/send', description: 'Send custom notification', auth: true, body: {title: 'Test', message: 'Test notification', type: 'info'} },
            { method: 'POST', path: '/api/notifications/health', url: IAM_API_BASE + '/api/notifications/health', description: 'Send health notification', auth: true },
            { method: 'POST', path: '/api/notifications/status', url: IAM_API_BASE + '/api/notifications/status', description: 'Send status notification', auth: true },
        ],
        'Admin - Database': [
            { method: 'GET', path: '/api/admin/database/list', url: IAM_API_BASE + '/api/admin/database/list?db_type=mysql', description: 'List databases', auth: true },
            { method: 'POST', path: '/api/admin/database/create', url: IAM_API_BASE + '/api/admin/database/create', description: 'Create database', auth: true, body: {database_name: 'test_db', db_type: 'mysql'} },
        ],
        'Admin - Tables': [
            { method: 'GET', path: '/api/admin/table/list', url: IAM_API_BASE + '/api/admin/table/list?db_type=mysql', description: 'List tables', auth: true },
            { method: 'POST', path: '/api/admin/table/create', url: IAM_API_BASE + '/api/admin/table/create', description: 'Create table', auth: true, body: {table_name: 'test_table', db_type: 'mysql', columns: [{name: 'id', type: 'INT', nullable: false, primary: true}]} },
        ],
    };

    let html = '';
    for (const [category, apis] of Object.entries(apiCategories)) {
        html += '<div class="api-category"><div class="api-category-title">' + category + '</div><div class="api-buttons-grid">';
        
        for (let i = 0; i < apis.length; i++) {
            const api = apis[i];
            const btnId = 'api-' + i;
            html += '<button class="api-test-btn" onclick="testAPI(\'' + api.method + '\', \'' + api.url + '\', ' + (api.body ? JSON.stringify(api.body).replace(/'/g, '&#39;') : 'null') + ', \'' + api.description + '\')">' +
                '<span class="api-method ' + api.method + '">' + api.method + '</span>' +
                '<span style="font-weight: 500;">' + api.path.split('/').pop() + '</span>' +
                '<span style="font-size: 0.8em; color: #999;">' + api.description + '</span>' +
                '</button>';
        }
        
        html += '</div></div>';
    }
    
    document.getElementById('apiCategories').innerHTML = html;
}

function testAPI(method, url, body, description) {
    const options = {
        method: method,
        headers: {
            'Content-Type': 'application/json'
        }
    };

    if (authToken) {
        options.headers['Authorization'] = 'Bearer ' + authToken;
    }

    if (body && (method === 'POST' || method === 'PUT')) {
        options.body = typeof body === 'string' ? body : JSON.stringify(body);
    }

    showSpinner(description);

    fetch(url, options)
        .then(function(response) {
            const status = response.status;
            return response.json().then(function(data) {
                return { status: status, data: data };
            });
        })
        .then(function(result) {
            showResponse(description, result.status, result.data, method + ' ' + url);
        })
        .catch(function(error) {
            showResponse(description, 'ERROR', {error: error.message}, method + ' ' + url);
        });
}

function showSpinner(title) {
    const modal = document.getElementById('responseModal');
    const body = document.getElementById('modalBody');
    document.getElementById('modalTitle').textContent = title;
    body.innerHTML = '<div class="spinner"></div><p class="loading">Loading...</p>';
    modal.classList.add('show');
}

function showResponse(title, status, data, endpoint) {
    const modal = document.getElementById('responseModal');
    const body = document.getElementById('modalBody');
    document.getElementById('modalTitle').textContent = title;
    
    let statusClass = 'success-message';
    if (typeof status === 'number' && status >= 400) {
        statusClass = 'error-message';
    } else if (typeof status === 'string' && status === 'ERROR') {
        statusClass = 'error-message';
    }
    
    let html = '<div class="' + statusClass + '"><strong>Status:</strong> ' + status + '</div>' +
        '<div style="margin-top: 15px;"><strong>Endpoint:</strong> <code>' + endpoint + '</code></div>' +
        '<div style="margin-top: 15px;"><strong>Response:</strong></div>' +
        '<pre style="background: #f8f9fa; padding: 15px; border-radius: 5px; overflow-x: auto;">' + 
        JSON.stringify(data, null, 2) + '</pre>';
    
    body.innerHTML = html;
    modal.classList.add('show');
}

function closeModal() {
    document.getElementById('responseModal').classList.remove('show');
}

function capitalizeFirstLetter(string) {
    return string.charAt(0).toUpperCase() + string.slice(1);
}

// Close modal when clicking outside
window.onclick = function(event) {
    const modal = document.getElementById('responseModal');
    if (event.target === modal) {
        modal.classList.remove('show');
    }
};
