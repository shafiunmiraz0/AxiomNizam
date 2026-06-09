// =====================================================
// CDC / ETL Dashboard — Enriched Frontend Logic
// =====================================================

(function() {
    'use strict';

    const API = (typeof window.resolveBackendURL === 'function') ? window.resolveBackendURL() : (window.BACKEND_URL || 'http://localhost:8000');
    const state = {
        currentTab: 'etl',
        createType: 'etl',
        currentEditingETLPipelineId: '',
        chartInstances: {},
        etlPipelines: [],
        etlRuns: [],
        cdcPipelines: [],
        etlConnectors: [],
        cdcSources: [],
        cdcSinks: [],
        blueprints: [],
        capabilities: []
    };

    const DB_CONNECTOR_IDS = ['mysql', 'postgres', 'mariadb', 'oracle', 'mongodb'];

    function buildHeaders(includeContentType) {
        const headers = {};
        if (includeContentType) {
            headers['Content-Type'] = 'application/json';
        }

        if (typeof window.getAuthHeaders === 'function') {
            return Object.assign(headers, window.getAuthHeaders());
        }

        const token = localStorage.getItem('authToken');
        if (token) {
            headers.Authorization = token.startsWith('Bearer ') ? token : 'Bearer ' + token;
        }
        return headers;
    }

    async function fetchJSON(path, options) {
        const opts = options || {};
        const method = opts.method || 'GET';
        const headers = buildHeaders(!!opts.body || method !== 'GET');
        try {
            const resp = await fetch(API + path, {
                method: method,
                headers: headers,
                body: opts.body ? JSON.stringify(opts.body) : undefined
            });
            const data = await resp.json().catch(function() { return {}; });
            if (!resp.ok) {
                const msg = data && (data.message || data.error) ? (data.message || data.error) : ('HTTP ' + resp.status);
                throw new Error(msg);
            }
            return data;
        } catch (err) {
            console.error('Request failed:', path, err);
            if (!opts.silent) {
                showToast('Request failed: ' + err.message, true);
            }
            return null;
        }
    }

    function showToast(message, isError) {
        if (!message) return;
        if (isError) {
            alert(message);
            return;
        }
        console.log(message);
    }

    function escapeHtml(input) {
        const div = document.createElement('div');
        div.textContent = input == null ? '' : String(input);
        return div.innerHTML;
    }

    function parseCSVList(raw) {
        if (!raw) return [];
        return raw
            .split(',')
            .map(function(item) { return item.trim(); })
            .filter(function(item) { return item.length > 0; });
    }

    function mergeUniqueLists() {
        const out = [];
        const seen = {};
        for (let i = 0; i < arguments.length; i++) {
            const list = arguments[i] || [];
            list.forEach(function(item) {
                const value = (item || '').trim();
                if (!value) return;
                const key = value.toLowerCase();
                if (seen[key]) return;
                seen[key] = true;
                out.push(value);
            });
        }
        return out;
    }

    function getInputValue(id) {
        const el = document.getElementById(id);
        if (!el) return '';
        return (el.value || '').trim();
    }

    function setInputValue(id, value) {
        const el = document.getElementById(id);
        if (!el) return;
        el.value = value || '';
    }

    function collectConnectorCredentialMetadata(prefix) {
        const keys = {
            dbHost: getInputValue(prefix + '-connector-db-host'),
            dbPort: getInputValue(prefix + '-connector-db-port'),
            dbUser: getInputValue(prefix + '-connector-db-user'),
            dbPassword: getInputValue(prefix + '-connector-db-password'),
            dbName: getInputValue(prefix + '-connector-db-name'),
            apiKey: getInputValue(prefix + '-connector-cred-api-key'),
            clientId: getInputValue(prefix + '-connector-cred-client-id'),
            clientSecret: getInputValue(prefix + '-connector-cred-client-secret'),
            accessKey: getInputValue(prefix + '-connector-cred-access-key'),
            secretKey: getInputValue(prefix + '-connector-cred-secret-key'),
            token: getInputValue(prefix + '-connector-cred-token'),
            saslUser: getInputValue(prefix + '-connector-cred-sasl-user'),
            saslPass: getInputValue(prefix + '-connector-cred-sasl-pass')
        };

        const configKeys = mergeUniqueLists([
            keys.dbHost,
            keys.dbPort,
            keys.dbUser,
            keys.dbPassword,
            keys.dbName,
            keys.apiKey,
            keys.clientId,
            keys.clientSecret,
            keys.accessKey,
            keys.secretKey,
            keys.token,
            keys.saslUser,
            keys.saslPass
        ]);

        const authModes = [];
        if (keys.dbUser || keys.dbPassword) authModes.push('password');
        if (keys.apiKey) authModes.push('apikey');
        if (keys.clientId || keys.clientSecret) authModes.push('oauth2');
        if (keys.accessKey || keys.secretKey) authModes.push('access_key');
        if (keys.saslUser || keys.saslPass) authModes.push('sasl');
        if (keys.token) authModes.push('bearer');

        return {
            configKeys: configKeys,
            authModes: authModes
        };
    }

    function findMatchingConnectorKey(configKeys, aliases) {
        if (!Array.isArray(configKeys) || !Array.isArray(aliases)) return '';
        for (let i = 0; i < aliases.length; i++) {
            const alias = aliases[i].toLowerCase();
            for (let j = 0; j < configKeys.length; j++) {
                const key = String(configKeys[j] || '');
                if (key.toLowerCase() === alias) {
                    return key;
                }
            }
        }
        return '';
    }

    function fillConnectorCredentialInputs(prefix, connector) {
        const configKeys = (connector && connector.config_keys) || [];
        setInputValue(prefix + '-connector-db-host', findMatchingConnectorKey(configKeys, ['host', 'hostname', 'server', 'endpoint']));
        setInputValue(prefix + '-connector-db-port', findMatchingConnectorKey(configKeys, ['port']));
        setInputValue(prefix + '-connector-db-user', findMatchingConnectorKey(configKeys, ['username', 'user', 'uid']));
        setInputValue(prefix + '-connector-db-password', findMatchingConnectorKey(configKeys, ['password', 'pass', 'pwd']));
        setInputValue(prefix + '-connector-db-name', findMatchingConnectorKey(configKeys, ['database', 'db', 'dbname', 'service_name']));
        setInputValue(prefix + '-connector-cred-api-key', findMatchingConnectorKey(configKeys, ['api_key', 'apikey']));
        setInputValue(prefix + '-connector-cred-client-id', findMatchingConnectorKey(configKeys, ['client_id']));
        setInputValue(prefix + '-connector-cred-client-secret', findMatchingConnectorKey(configKeys, ['client_secret']));
        setInputValue(prefix + '-connector-cred-access-key', findMatchingConnectorKey(configKeys, ['access_key']));
        setInputValue(prefix + '-connector-cred-secret-key', findMatchingConnectorKey(configKeys, ['secret_key']));
        setInputValue(prefix + '-connector-cred-token', findMatchingConnectorKey(configKeys, ['token', 'bearer_token', 'jwt']));
        setInputValue(prefix + '-connector-cred-sasl-user', findMatchingConnectorKey(configKeys, ['sasl_username', 'sasl_user']));
        setInputValue(prefix + '-connector-cred-sasl-pass', findMatchingConnectorKey(configKeys, ['sasl_password', 'sasl_pass']));
    }

    function resetConnectorCredentialInputs(prefix) {
        setInputValue(prefix + '-connector-db-host', '');
        setInputValue(prefix + '-connector-db-port', '');
        setInputValue(prefix + '-connector-db-user', '');
        setInputValue(prefix + '-connector-db-password', '');
        setInputValue(prefix + '-connector-db-name', '');
        setInputValue(prefix + '-connector-cred-api-key', '');
        setInputValue(prefix + '-connector-cred-client-id', '');
        setInputValue(prefix + '-connector-cred-client-secret', '');
        setInputValue(prefix + '-connector-cred-access-key', '');
        setInputValue(prefix + '-connector-cred-secret-key', '');
        setInputValue(prefix + '-connector-cred-token', '');
        setInputValue(prefix + '-connector-cred-sasl-user', '');
        setInputValue(prefix + '-connector-cred-sasl-pass', '');
    }

    function toMaybeInt(value) {
        const raw = (value || '').trim();
        if (!raw) return null;
        const num = parseInt(raw, 10);
        return isNaN(num) ? raw : num;
    }

    function isDatabaseLikeConnector(connectorID) {
        const id = (connectorID || '').trim().toLowerCase();
        if (!id) return false;
        if (DB_CONNECTOR_IDS.indexOf(id) >= 0) return true;
        const connector = state.etlConnectors.find(function(c) { return (c.id || '').toLowerCase() === id; });
        if (!connector) return false;
        const category = (connector.category || '').toLowerCase();
        return category === 'database' || category === 'warehouse' || category === 'lakehouse';
    }

    function collectRuntimeStepConfig(prefix) {
        const cfg = {};

        const host = getInputValue(prefix + '-db-host');
        const port = toMaybeInt(getInputValue(prefix + '-db-port'));
        const dbUser = getInputValue(prefix + '-db-user');
        const dbPassword = getInputValue(prefix + '-db-password');
        const dbName = getInputValue(prefix + '-db-name');
        const dbTable = getInputValue(prefix + '-db-table');
        const sql = getInputValue(prefix + '-sql');

        if (host) cfg.host = host;
        if (port !== null) cfg.port = port;
        if (dbUser) cfg.username = dbUser;
        if (dbPassword) cfg.password = dbPassword;
        if (dbName) cfg.database = dbName;
        if (dbTable) cfg.table = dbTable;
        if (sql) cfg.query = sql;

        const apiKey = getInputValue(prefix + '-cred-api-key');
        const token = getInputValue(prefix + '-cred-token');
        const clientID = getInputValue(prefix + '-cred-client-id');
        const clientSecret = getInputValue(prefix + '-cred-client-secret');
        const accessKey = getInputValue(prefix + '-cred-access-key');
        const secretKey = getInputValue(prefix + '-cred-secret-key');
        const username = getInputValue(prefix + '-cred-username');
        const password = getInputValue(prefix + '-cred-password');
        const saslUsername = getInputValue(prefix + '-cred-sasl-user');
        const saslPassword = getInputValue(prefix + '-cred-sasl-pass');

        if (apiKey) cfg.api_key = apiKey;
        if (token) cfg.token = token;
        if (clientID) cfg.client_id = clientID;
        if (clientSecret) cfg.client_secret = clientSecret;
        if (accessKey) cfg.access_key = accessKey;
        if (secretKey) cfg.secret_key = secretKey;
        if (username) cfg.username = username;
        if (password) cfg.password = password;
        if (saslUsername) cfg.sasl_username = saslUsername;
        if (saslPassword) cfg.sasl_password = saslPassword;

        return cfg;
    }

    function fillRuntimeStepConfig(prefix, config) {
        const cfg = config || {};
        setInputValue(prefix + '-db-host', cfg.host || cfg.hostname || cfg.server || '');
        setInputValue(prefix + '-db-port', cfg.port != null ? String(cfg.port) : '');
        setInputValue(prefix + '-db-user', cfg.username || cfg.user || '');
        setInputValue(prefix + '-db-password', cfg.password || cfg.pass || '');
        setInputValue(prefix + '-db-name', cfg.database || cfg.db || cfg.dbname || cfg.service_name || '');
        setInputValue(prefix + '-db-table', cfg.table || '');
        setInputValue(prefix + '-sql', cfg.query || '');
        setInputValue(prefix + '-cred-api-key', cfg.api_key || cfg.apikey || '');
        setInputValue(prefix + '-cred-token', cfg.token || cfg.bearer_token || cfg.jwt || '');
        setInputValue(prefix + '-cred-client-id', cfg.client_id || '');
        setInputValue(prefix + '-cred-client-secret', cfg.client_secret || '');
        setInputValue(prefix + '-cred-access-key', cfg.access_key || '');
        setInputValue(prefix + '-cred-secret-key', cfg.secret_key || '');
        setInputValue(prefix + '-cred-username', cfg.username || cfg.user || '');
        setInputValue(prefix + '-cred-password', cfg.password || cfg.pass || '');
        setInputValue(prefix + '-cred-sasl-user', cfg.sasl_username || cfg.sasl_user || '');
        setInputValue(prefix + '-cred-sasl-pass', cfg.sasl_password || cfg.sasl_pass || '');
    }

    function resetRuntimeStepConfig(prefix) {
        setInputValue(prefix + '-db-host', '');
        setInputValue(prefix + '-db-port', '');
        setInputValue(prefix + '-db-user', '');
        setInputValue(prefix + '-db-password', '');
        setInputValue(prefix + '-db-name', '');
        setInputValue(prefix + '-db-table', '');
        setInputValue(prefix + '-sql', '');
        setInputValue(prefix + '-cred-api-key', '');
        setInputValue(prefix + '-cred-token', '');
        setInputValue(prefix + '-cred-client-id', '');
        setInputValue(prefix + '-cred-client-secret', '');
        setInputValue(prefix + '-cred-access-key', '');
        setInputValue(prefix + '-cred-secret-key', '');
        setInputValue(prefix + '-cred-username', '');
        setInputValue(prefix + '-cred-password', '');
        setInputValue(prefix + '-cred-sasl-user', '');
        setInputValue(prefix + '-cred-sasl-pass', '');
    }

    function updateCredentialOptionVisibility(connectorID, dbOptionsID, otherCredOptionsID) {
        const dbOpts = document.getElementById(dbOptionsID);
        const otherCredOpts = document.getElementById(otherCredOptionsID);
        const isDB = isDatabaseLikeConnector(connectorID);
        if (dbOpts) dbOpts.style.display = isDB ? 'block' : 'none';
        if (otherCredOpts) otherCredOpts.style.display = isDB ? 'none' : 'block';
    }

    function populateEditConnectorDropdowns() {
        const extractEl = document.getElementById('edit-etl-extract-connector');
        const loadEl = document.getElementById('edit-etl-load-connector');
        if (extractEl) {
            const extractOpts = state.etlConnectors.filter(function(c) {
                return (c.supported_as || []).indexOf('extract') >= 0 || (c.supported_as || []).indexOf('both') >= 0;
            });
            extractEl.innerHTML = extractOpts.map(function(c) {
                return '<option value="' + escapeHtml(c.id) + '">' + escapeHtml(c.name) + '</option>';
            }).join('');
        }
        if (loadEl) {
            const loadOpts = state.etlConnectors.filter(function(c) {
                return (c.supported_as || []).indexOf('load') >= 0 || (c.supported_as || []).indexOf('both') >= 0;
            });
            loadEl.innerHTML = loadOpts.map(function(c) {
                return '<option value="' + escapeHtml(c.id) + '">' + escapeHtml(c.name) + '</option>';
            }).join('');
        }
    }

    function buildUpdatedETLStepsForEdit(pipeline) {
        const src = pipeline || {};
        const steps = JSON.parse(JSON.stringify(src.steps || []));
        const extractConnector = (document.getElementById('edit-etl-extract-connector').value || '').trim();
        const loadConnector = (document.getElementById('edit-etl-load-connector').value || '').trim();

        let extractIdx = -1;
        for (let i = 0; i < steps.length; i++) {
            if ((steps[i].type || '').toLowerCase() === 'extract') {
                extractIdx = i;
                break;
            }
        }
        if (extractIdx === -1) {
            steps.unshift({ id: 'step-extract', name: 'Extract', type: 'extract', connector: extractConnector, config: {}, order: 1 });
            extractIdx = 0;
        }

        let loadIdx = -1;
        for (let i = steps.length - 1; i >= 0; i--) {
            if ((steps[i].type || '').toLowerCase() === 'load') {
                loadIdx = i;
                break;
            }
        }
        if (loadIdx === -1) {
            steps.push({ id: 'step-load', name: 'Load', type: 'load', connector: loadConnector, config: {}, order: steps.length + 1 });
            loadIdx = steps.length - 1;
        }

        steps[extractIdx].connector = extractConnector;
        steps[extractIdx].config = collectRuntimeStepConfig('edit-extract');
        steps[loadIdx].connector = loadConnector;
        steps[loadIdx].config = collectRuntimeStepConfig('edit-load');

        steps.forEach(function(step, idx) {
            if (!step.id) step.id = 'step-' + (idx + 1);
            if (!step.name) step.name = 'Step ' + (idx + 1);
            step.order = idx + 1;
            if (!step.config) step.config = {};
        });

        return steps;
    }

    function fmtNum(n) {
        if (n == null) return '0';
        if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
        if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
        return String(n);
    }

    function fmtTime(ts) {
        if (!ts) return '—';
        const d = new Date(ts);
        if (isNaN(d.getTime())) return '—';
        return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    }

    function statusBadge(status) {
        const s = (status || 'unknown').toLowerCase();
        return '<span class="status-badge ' + escapeHtml(s) + '"><span class="status-dot"></span> ' + escapeHtml(s) + '</span>';
    }

    function tagList(tags) {
        if (!tags || !tags.length) return '—';
        return '<div class="tag-list">' + tags.map(function(tag) {
            return '<span class="tag">' + escapeHtml(tag) + '</span>';
        }).join('') + '</div>';
    }

    function kpiCard(label, value, sub, cls) {
        return '<div class="kpi-card ' + (cls ? ('kpi-' + cls) : '') + '">' +
            '<div class="kpi-label">' + escapeHtml(label) + '</div>' +
            '<div class="kpi-value">' + escapeHtml(value) + '</div>' +
            (sub ? ('<div class="kpi-sub">' + escapeHtml(sub) + '</div>') : '') +
            '</div>';
    }

    function currentTabId(tab) {
        if (tab === 'observability') return 'obs';
        if (tab === 'connectors') return 'conn';
        if (tab === 'orchestration') return 'orch';
        return tab;
    }

    window.switchTab = function(tab) {
        state.currentTab = tab;
        document.querySelectorAll('.cdc-etl-tab').forEach(function(btn) { btn.classList.remove('active'); });
        document.querySelectorAll('.panel-tab-content').forEach(function(panel) { panel.classList.remove('active'); });
        const tabBtn = document.getElementById('tab-' + currentTabId(tab));
        const tabPanel = document.getElementById('panel-' + tab);
        if (tabBtn) tabBtn.classList.add('active');
        if (tabPanel) tabPanel.classList.add('active');

        if (tab === 'connectors') {
            refreshConnectorView();
        }
        if (tab === 'orchestration') {
            renderOrchestrationHealth();
        }
    };

    async function loadETL() {
        const responses = await Promise.all([
            fetchJSON('/api/v1/etl/pipelines', { silent: true }),
            fetchJSON('/api/v1/etl/runs', { silent: true }),
            fetchJSON('/api/v1/etl/observability', { silent: true })
        ]);
        const pData = responses[0] || {};
        const rData = responses[1] || {};
        const oData = responses[2] || {};

        state.etlPipelines = pData.pipelines || [];
        state.etlRuns = rData.runs || [];

        renderETLPipelines(state.etlPipelines);
        renderETLRuns(state.etlRuns);
        renderETLKPIs(oData.observability || {});

        const countEl = document.getElementById('etl-count');
        if (countEl) countEl.textContent = String(state.etlPipelines.length);
    }

    function renderETLKPIs(obs) {
        const el = document.getElementById('etl-kpis');
        if (!el) return;
        el.innerHTML = [
            kpiCard('Pipelines', obs.pipelines_total || 0, '', ''),
            kpiCard('Total Runs', obs.runs_total || 0, '', 'info'),
            kpiCard('Successful', obs.runs_success || 0, '', 'success'),
            kpiCard('Failed', obs.runs_failed || 0, '', 'danger'),
            kpiCard('Running', obs.runs_running || 0, '', 'warning'),
            kpiCard('Rows Read', fmtNum(obs.total_rows_read || 0), '', 'info'),
            kpiCard('Rows Written', fmtNum(obs.total_rows_written || 0), '', 'success'),
            kpiCard('Avg Duration', obs.avg_duration_seconds ? (obs.avg_duration_seconds.toFixed(1) + 's') : '—', '', '')
        ].join('');
    }

    function renderETLPipelines(pipelines) {
        const tbody = document.getElementById('etl-pipeline-body');
        if (!tbody) return;
        if (!pipelines.length) {
            tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;padding:30px;color:#888">No ETL pipelines found</td></tr>';
            return;
        }

        tbody.innerHTML = pipelines.map(function(p) {
            const orch = p.orchestration || {};
            const schedLabel = p.schedule || (orch.catchup ? 'catchup-enabled' : 'manual/event');
            return '<tr>' +
                '<td><strong>' + escapeHtml(p.name) + '</strong><br><small style="color:#888">' + escapeHtml(p.id) + '</small></td>' +
                '<td>' + statusBadge(p.status) + '</td>' +
                '<td>' + ((p.steps || []).length) + ' steps</td>' +
                '<td>' + escapeHtml(schedLabel) + '</td>' +
                '<td>' + (p.run_count || 0) + '</td>' +
                '<td>' + fmtTime(p.last_run_at) + '</td>' +
                '<td>' + tagList(p.tags) + '</td>' +
                '<td class="action-btns">' +
                    '<button class="action-btn" onclick="editETLPipeline(\'' + escapeHtml(p.id) + '\')" title="Edit">✏️</button>' +
                    '<button class="action-btn" onclick="runETLPipeline(\'' + escapeHtml(p.id) + '\')" title="Run">▶</button>' +
                    '<button class="action-btn" onclick="openCreateModal(\'etl\', \'' + escapeHtml(p.id) + '\')" title="Clone">⧉</button>' +
                    '<button class="action-btn danger" onclick="deleteETLPipeline(\'' + escapeHtml(p.id) + '\')" title="Delete">🗑</button>' +
                '</td>' +
            '</tr>';
        }).join('');
    }

    function renderETLRuns(runs) {
        const tbody = document.getElementById('etl-runs-body');
        if (!tbody) return;
        if (!runs.length) {
            tbody.innerHTML = '<tr><td colspan="8" style="text-align:center;padding:30px;color:#888">No runs yet</td></tr>';
            return;
        }

        const sorted = runs.slice().sort(function(a, b) {
            return new Date(b.started_at).getTime() - new Date(a.started_at).getTime();
        });

        tbody.innerHTML = sorted.slice(0, 20).map(function(r) {
            return '<tr>' +
                '<td><small>' + escapeHtml(r.id) + '</small></td>' +
                '<td>' + escapeHtml(r.pipeline_id) + '</td>' +
                '<td>' + statusBadge(r.status) + '</td>' +
                '<td>' + escapeHtml(r.trigger || '—') + '</td>' +
                '<td>' + fmtNum(r.rows_read) + '</td>' +
                '<td>' + fmtNum(r.rows_written) + '</td>' +
                '<td>' + escapeHtml(r.duration || '—') + '</td>' +
                '<td>' + fmtTime(r.started_at) + '</td>' +
            '</tr>';
        }).join('');
    }

    window.runETLPipeline = async function(id) {
        const resp = await fetchJSON('/api/v1/etl/pipelines/' + encodeURIComponent(id) + '/run', { method: 'POST' });
        if (resp) {
            showToast('Pipeline run triggered: ' + id, false);
            setTimeout(function() { loadETL(); loadObservability(); renderOrchestrationHealth(); }, 500);
        }
    };

    window.deleteETLPipeline = async function(id) {
        if (!confirm('Delete pipeline ' + id + '?')) return;
        const resp = await fetchJSON('/api/v1/etl/pipelines/' + encodeURIComponent(id), { method: 'DELETE' });
        if (resp) {
            loadETL();
            loadObservability();
            renderOrchestrationHealth();
        }
    };

    async function loadCDC() {
        const responses = await Promise.all([
            fetchJSON('/api/v1/cdc/pipelines', { silent: true }),
            fetchJSON('/api/v1/cdc/observability', { silent: true })
        ]);

        const pData = responses[0] || {};
        const oData = responses[1] || {};
        state.cdcPipelines = pData.pipelines || [];

        renderCDCPipelines(state.cdcPipelines);
        renderCDCKPIs(oData.observability || {});

        const countEl = document.getElementById('cdc-count');
        if (countEl) countEl.textContent = String(state.cdcPipelines.length);
    }

    function renderCDCKPIs(obs) {
        const el = document.getElementById('cdc-kpis');
        if (!el) return;
        el.innerHTML = [
            kpiCard('Total Pipelines', obs.pipelines_total || 0, '', ''),
            kpiCard('Active', obs.pipelines_active || 0, '', 'success'),
            kpiCard('Paused', obs.pipelines_paused || 0, '', 'warning'),
            kpiCard('Failed', obs.pipelines_failed || 0, '', 'danger'),
            kpiCard('Total Events', fmtNum(obs.total_events || 0), '', 'info'),
            kpiCard('Events/sec', obs.events_per_second ? obs.events_per_second.toFixed(1) : '0', '', 'success'),
            kpiCard('Errors', fmtNum(obs.total_errors || 0), '', 'danger'),
            kpiCard('Avg Lag', obs.avg_lag_ms ? obs.avg_lag_ms.toFixed(0) + 'ms' : '—', '', '')
        ].join('');
    }

    function renderCDCPipelines(pipelines) {
        const tbody = document.getElementById('cdc-pipeline-body');
        if (!tbody) return;
        if (!pipelines.length) {
            tbody.innerHTML = '<tr><td colspan="9" style="text-align:center;padding:30px;color:#888">No CDC pipelines found</td></tr>';
            return;
        }
        tbody.innerHTML = pipelines.map(function(p) {
            return '<tr>' +
                '<td><strong>' + escapeHtml(p.name) + '</strong><br><small style="color:#888">' + escapeHtml(p.id) + '</small></td>' +
                '<td>' + statusBadge(p.status) + '</td>' +
                '<td>' + escapeHtml((p.source && p.source.type) || '—') + '</td>' +
                '<td>' + escapeHtml((p.sink && p.sink.type) || '—') + '</td>' +
                '<td>' + fmtNum(p.event_count || 0) + '</td>' +
                '<td>' + fmtNum(p.error_count || 0) + '</td>' +
                '<td>' + escapeHtml(p.lag || '—') + '</td>' +
                '<td>' + tagList(p.tags) + '</td>' +
                '<td class="action-btns">' +
                    '<button class="action-btn" onclick="editCDCPipeline(\'' + escapeHtml(p.id) + '\')" title="Edit">✏️</button>' +
                    (p.status !== 'active' ? '<button class="action-btn" onclick="cdcAction(\'' + escapeHtml(p.id) + '\',\'start\')" title="Start">▶</button>' : '') +
                    (p.status === 'active' ? '<button class="action-btn" onclick="cdcAction(\'' + escapeHtml(p.id) + '\',\'pause\')" title="Pause">⏸</button>' : '') +
                    (p.status !== 'stopped' ? '<button class="action-btn" onclick="cdcAction(\'' + escapeHtml(p.id) + '\',\'stop\')" title="Stop">⏹</button>' : '') +
                    '<button class="action-btn danger" onclick="deleteCDCPipeline(\'' + escapeHtml(p.id) + '\')" title="Delete">🗑</button>' +
                '</td>' +
            '</tr>';
        }).join('');
    }

    window.cdcAction = async function(id, action) {
        const resp = await fetchJSON('/api/v1/cdc/pipelines/' + encodeURIComponent(id) + '/' + action, { method: 'POST' });
        if (resp) {
            loadCDC();
            loadObservability();
        }
    };

    window.deleteCDCPipeline = async function(id) {
        if (!confirm('Delete CDC pipeline ' + id + '?')) return;
        const resp = await fetchJSON('/api/v1/cdc/pipelines/' + encodeURIComponent(id), { method: 'DELETE' });
        if (resp) {
            loadCDC();
            loadObservability();
        }
    };

    function destroyChart(id) {
        if (state.chartInstances[id]) {
            state.chartInstances[id].destroy();
            delete state.chartInstances[id];
        }
    }

    function renderErrorList(elId, errors) {
        const el = document.getElementById(elId);
        if (!el) return;
        const entries = Object.entries(errors || {});
        if (!entries.length) {
            el.innerHTML = '<li style="color:#888">No errors recorded</li>';
            return;
        }
        el.innerHTML = entries.map(function(entry) {
            return '<li><span class="error-type">' + escapeHtml(entry[0]) + '</span><span class="error-count">' + escapeHtml(entry[1]) + '</span></li>';
        }).join('');
    }

    function renderETLRunsChart(obs) {
        destroyChart('chart-etl-runs');
        const canvas = document.getElementById('chart-etl-runs');
        if (!canvas || typeof Chart === 'undefined') return;

        const log = obs.throughput_log || [];
        const labels = log.map(function(point) {
            const d = new Date(point.timestamp);
            return d.getHours() + ':' + String(d.getMinutes()).padStart(2, '0');
        });

        state.chartInstances['chart-etl-runs'] = new window.__axc(canvas, {
            type: 'line',
            data: {
                labels: labels.length ? labels : ['No data'],
                datasets: [{
                    label: 'Rows/sec',
                    data: log.map(function(point) { return point.rows_per_sec || 0; }),
                    borderColor: '#4f46e5',
                    backgroundColor: 'rgba(79,70,229,0.1)',
                    fill: true,
                    tension: 0.3
                }]
            },
            options: { responsive: true, plugins: { legend: { display: false } }, scales: { y: { beginAtZero: true } } }
        });
    }

    function renderCDCThroughputChart(obs) {
        destroyChart('chart-cdc-throughput');
        const canvas = document.getElementById('chart-cdc-throughput');
        if (!canvas || typeof Chart === 'undefined') return;

        const log = obs.throughput_log || [];
        const labels = log.map(function(point) {
            const d = new Date(point.timestamp);
            return d.getHours() + ':' + String(d.getMinutes()).padStart(2, '0');
        });

        state.chartInstances['chart-cdc-throughput'] = new window.__axc(canvas, {
            type: 'line',
            data: {
                labels: labels.length ? labels : ['No data'],
                datasets: [{
                    label: 'Events/sec',
                    data: log.map(function(point) { return point.events_per_sec || 0; }),
                    borderColor: '#10b981',
                    backgroundColor: 'rgba(16,185,129,0.1)',
                    fill: true,
                    tension: 0.3
                }, {
                    label: 'Lag (ms)',
                    data: log.map(function(point) { return point.lag_ms || 0; }),
                    borderColor: '#f59e0b',
                    fill: false,
                    tension: 0.3,
                    yAxisID: 'y1'
                }]
            },
            options: {
                responsive: true,
                scales: {
                    y: { beginAtZero: true, position: 'left', title: { display: true, text: 'Events/sec' } },
                    y1: { beginAtZero: true, position: 'right', title: { display: true, text: 'Lag (ms)' }, grid: { drawOnChartArea: false } }
                }
            }
        });
    }

    function renderETLStepsChart(obs) {
        destroyChart('chart-etl-steps');
        const canvas = document.getElementById('chart-etl-steps');
        if (!canvas || typeof Chart === 'undefined') return;

        const stats = obs.step_type_stats || {};
        const labels = Object.keys(stats);
        const values = Object.values(stats);
        state.chartInstances['chart-etl-steps'] = new window.__axc(canvas, {
            type: 'doughnut',
            data: {
                labels: labels.length ? labels : ['No data'],
                datasets: [{
                    data: values.length ? values : [1],
                    backgroundColor: ['#4f46e5', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4', '#ec4899', '#14b8a6']
                }]
            },
            options: { responsive: true, plugins: { legend: { position: 'right' } } }
        });
    }

    function renderCDCOpsChart(obs) {
        destroyChart('chart-cdc-ops');
        const canvas = document.getElementById('chart-cdc-ops');
        if (!canvas || typeof Chart === 'undefined') return;

        const byOp = obs.events_by_operation || {};
        state.chartInstances['chart-cdc-ops'] = new window.__axc(canvas, {
            type: 'bar',
            data: {
                labels: Object.keys(byOp).length ? Object.keys(byOp) : ['No data'],
                datasets: [{
                    label: 'Events',
                    data: Object.values(byOp).length ? Object.values(byOp) : [0],
                    backgroundColor: ['#10b981', '#3b82f6', '#ef4444', '#f59e0b']
                }]
            },
            options: { responsive: true, plugins: { legend: { display: false } }, scales: { y: { beginAtZero: true } } }
        });
    }

    async function loadObservability() {
        const responses = await Promise.all([
            fetchJSON('/api/v1/etl/observability', { silent: true }),
            fetchJSON('/api/v1/cdc/observability', { silent: true })
        ]);

        const eObs = (responses[0] && responses[0].observability) || {};
        const cObs = (responses[1] && responses[1].observability) || {};

        const kpiEl = document.getElementById('obs-kpis');
        if (kpiEl) {
            kpiEl.innerHTML = [
                kpiCard('ETL Pipelines', eObs.pipelines_total || 0, '', ''),
                kpiCard('CDC Pipelines', cObs.pipelines_total || 0, '', ''),
                kpiCard('ETL Runs', eObs.runs_total || 0, 'Success: ' + (eObs.runs_success || 0), 'info'),
                kpiCard('CDC Events', fmtNum(cObs.total_events || 0), fmtNum(cObs.events_per_second || 0) + ' evt/s', 'success'),
                kpiCard('ETL Rows', fmtNum((eObs.total_rows_read || 0) + (eObs.total_rows_written || 0)), '', 'info'),
                kpiCard('CDC Errors', fmtNum(cObs.total_errors || 0), ((cObs.error_rate_percent || 0).toFixed(2)) + '% rate', 'danger')
            ].join('');
        }

        renderETLRunsChart(eObs);
        renderCDCThroughputChart(cObs);
        renderETLStepsChart(eObs);
        renderCDCOpsChart(cObs);
        renderErrorList('etl-error-list', eObs.errors_by_type || {});
        renderErrorList('cdc-error-list', cObs.errors_by_type || {});
    }

    function renderConnectorCards(elId, list) {
        const el = document.getElementById(elId);
        if (!el) return;
        if (!list || !list.length) {
            el.innerHTML = '<p style="color:#888">No connectors available</p>';
            return;
        }

        el.innerHTML = list.map(function(c) {
            const flags = [];
            if (c.supports_incremental) flags.push('incremental');
            if (c.schema_discovery) flags.push('schema');
            if (c.supports_cdc) flags.push('cdc');

            return '<div class="connector-card">' +
                '<div class="connector-icon">' + (c.icon || '') + '</div>' +
                '<div class="connector-name">' + escapeHtml(c.name || c.id) + '</div>' +
                '<div class="connector-type">' + escapeHtml(c.category || c.method || c.id) + '</div>' +
                (c.version ? '<div class="connector-meta">v' + escapeHtml(c.version) + '</div>' : '') +
                (c.description ? '<div class="connector-desc">' + escapeHtml(c.description) + '</div>' : '') +
                (flags.length ? '<div class="connector-flag-list">' + flags.map(function(flag) { return '<span class="tag">' + escapeHtml(flag) + '</span>'; }).join('') + '</div>' : '') +
                '<div class="connector-actions" style="margin-top:8px;display:flex;gap:6px;">' +
                    '<button class="action-btn" onclick="editConnector(\'' + escapeHtml(c.id) + '\')" title="Edit" style="font-size:12px;">✏️ Edit</button>' +
                    '<button class="action-btn danger" onclick="deleteConnector(\'' + escapeHtml(c.id) + '\')" title="Delete" style="font-size:12px;">🗑</button>' +
                '</div>' +
            '</div>';
        }).join('');
    }

    window.refreshConnectorView = function() {
        const qEl = document.getElementById('connector-search');
        const catEl = document.getElementById('connector-category');
        const query = qEl ? qEl.value.trim().toLowerCase() : '';
        const category = catEl ? catEl.value.trim().toLowerCase() : '';

        const filtered = state.etlConnectors.filter(function(c) {
            if (category && (c.category || '').toLowerCase() !== category) {
                return false;
            }
            if (!query) {
                return true;
            }
            const haystack = [c.id, c.name, c.description, c.category].join(' ').toLowerCase();
            return haystack.indexOf(query) >= 0;
        });

        renderConnectorCards('etl-connectors', filtered);
        renderConnectorCards('cdc-sources', state.cdcSources);
        renderConnectorCards('cdc-sinks', state.cdcSinks);
    };

    function populateConnectorDropdowns() {
        const extractEl = document.getElementById('modal-etl-extract-connector');
        const loadEl = document.getElementById('modal-etl-load-connector');
        if (extractEl) {
            const extractOpts = state.etlConnectors.filter(function(c) {
                return (c.supported_as || []).indexOf('extract') >= 0 || (c.supported_as || []).indexOf('both') >= 0;
            });
            extractEl.innerHTML = extractOpts.map(function(c) {
                return '<option value="' + escapeHtml(c.id) + '">' + escapeHtml(c.name) + '</option>';
            }).join('');
        }
        if (loadEl) {
            const loadOpts = state.etlConnectors.filter(function(c) {
                return (c.supported_as || []).indexOf('load') >= 0 || (c.supported_as || []).indexOf('both') >= 0;
            });
            loadEl.innerHTML = loadOpts.map(function(c) {
                return '<option value="' + escapeHtml(c.id) + '">' + escapeHtml(c.name) + '</option>';
            }).join('');
        }
    }

    function populateCDCTypeDropdowns() {
        const srcEl = document.getElementById('modal-source-type');
        const sinkEl = document.getElementById('modal-sink-type');
        if (srcEl) {
            srcEl.innerHTML = state.cdcSources.map(function(s) {
                return '<option value="' + escapeHtml(s.id) + '">' + escapeHtml(s.name) + '</option>';
            }).join('');
        }
        if (sinkEl) {
            sinkEl.innerHTML = state.cdcSinks.map(function(s) {
                return '<option value="' + escapeHtml(s.id) + '">' + escapeHtml(s.name) + '</option>';
            }).join('');
        }
    }

    function populateBlueprintDropdown() {
        const el = document.getElementById('modal-blueprint');
        if (!el) return;
        const base = '<option value="">Custom Pipeline</option>';
        const opts = state.blueprints.map(function(bp) {
            return '<option value="' + escapeHtml(bp.id) + '">' + escapeHtml(bp.name) + '</option>';
        }).join('');
        el.innerHTML = base + opts;
    }

    async function loadConnectors() {
        const responses = await Promise.all([
            fetchJSON('/api/v1/etl/connectors/catalog', { silent: true }),
            fetchJSON('/api/v1/cdc/sources', { silent: true }),
            fetchJSON('/api/v1/cdc/sinks', { silent: true })
        ]);

        const etlData = responses[0] || {};
        state.etlConnectors = etlData.connectors || [];
        state.cdcSources = (responses[1] && responses[1].sources) || [];
        state.cdcSinks = (responses[2] && responses[2].sinks) || [];

        populateConnectorDropdowns();
        populateCDCTypeDropdowns();
        refreshConnectorView();
    }

    window.submitNewConnector = async function() {
        const credentialMeta = collectConnectorCredentialMetadata('new');
        const payload = {
            id: (document.getElementById('new-connector-id').value || '').trim().toLowerCase(),
            name: (document.getElementById('new-connector-name').value || '').trim(),
            category: (document.getElementById('new-connector-category').value || '').trim().toLowerCase(),
            icon: (document.getElementById('new-connector-icon').value || '').trim() || '🔌',
            version: (document.getElementById('new-connector-version').value || '').trim() || '1.0',
            supported_as: parseCSVList(document.getElementById('new-connector-supported-as').value || 'extract,load'),
            auth_modes: mergeUniqueLists(parseCSVList(document.getElementById('new-connector-auth').value || ''), credentialMeta.authModes),
            config_keys: mergeUniqueLists(parseCSVList(document.getElementById('new-connector-config').value || ''), credentialMeta.configKeys),
            description: (document.getElementById('new-connector-description').value || '').trim(),
            supports_incremental: document.getElementById('new-connector-incremental').checked,
            schema_discovery: document.getElementById('new-connector-schema').checked,
            supports_cdc: document.getElementById('new-connector-cdc').checked
        };

        if (!payload.id || !payload.name || !payload.category) {
            showToast('Connector id, name and category are required.', true);
            return;
        }

        const resp = await fetchJSON('/api/v1/etl/connectors', { method: 'POST', body: payload });
        if (!resp) return;

        showToast('Connector added: ' + payload.name, false);
        document.getElementById('new-connector-id').value = '';
        document.getElementById('new-connector-name').value = '';
        document.getElementById('new-connector-icon').value = '';
        document.getElementById('new-connector-version').value = '1.0';
        document.getElementById('new-connector-supported-as').value = 'extract,load';
        document.getElementById('new-connector-auth').value = '';
        document.getElementById('new-connector-config').value = '';
        document.getElementById('new-connector-description').value = '';
        document.getElementById('new-connector-incremental').checked = false;
        document.getElementById('new-connector-schema').checked = false;
        document.getElementById('new-connector-cdc').checked = false;
        resetConnectorCredentialInputs('new');

        await loadConnectors();
    };

    function renderCapabilities() {
        const grid = document.getElementById('orchestration-feature-grid');
        if (!grid) return;
        if (!state.capabilities.length) {
            grid.innerHTML = '<p style="color:#888">No capabilities metadata available</p>';
            return;
        }
        grid.innerHTML = state.capabilities.map(function(cap) {
            return '<div class="feature-card">' +
                '<div class="feature-category">' + escapeHtml(cap.category || 'feature') + '</div>' +
                '<h4>' + escapeHtml(cap.name) + '</h4>' +
                '<p>' + escapeHtml(cap.description || '') + '</p>' +
            '</div>';
        }).join('');
    }

    function renderBlueprints() {
        const grid = document.getElementById('etl-blueprints');
        if (!grid) return;
        if (!state.blueprints.length) {
            grid.innerHTML = '<p style="color:#888">No ETL blueprints available</p>';
            return;
        }
        grid.innerHTML = state.blueprints.map(function(bp) {
            return '<div class="blueprint-card">' +
                '<div class="feature-category">' + escapeHtml(bp.category || 'template') + '</div>' +
                '<h4>' + escapeHtml(bp.name) + '</h4>' +
                '<p>' + escapeHtml(bp.description || '') + '</p>' +
                '<div class="blueprint-meta">' +
                    '<span>' + ((bp.steps || []).length) + ' steps</span>' +
                    '<span>' + escapeHtml(bp.default_schedule || 'event/manual') + '</span>' +
                '</div>' +
                '<div class="tag-list">' + (bp.tags || []).map(function(tag) { return '<span class="tag">' + escapeHtml(tag) + '</span>'; }).join('') + '</div>' +
                '<button class="btn-create" onclick="createFromBlueprint(\'' + escapeHtml(bp.id) + '\')">Use Blueprint</button>' +
            '</div>';
        }).join('');
    }

    function renderOrchestrationHealth() {
        const el = document.getElementById('etl-orchestration-health');
        if (!el) return;

        const total = state.etlPipelines.length;
        const catchupEnabled = state.etlPipelines.filter(function(p) { return p.orchestration && p.orchestration.catchup; }).length;
        const dependsOnPast = state.etlPipelines.filter(function(p) { return p.orchestration && p.orchestration.depends_on_past; }).length;
        const highRetry = state.etlPipelines.filter(function(p) { return p.orchestration && (p.orchestration.retries || 0) >= 3; }).length;
        const running = state.etlRuns.filter(function(r) { return (r.status || '').toLowerCase() === 'running'; }).length;

        el.innerHTML = '<div class="kpi-grid">' +
            kpiCard('Managed Pipelines', total, '', '') +
            kpiCard('Catchup Enabled', catchupEnabled, '', 'info') +
            kpiCard('Depends On Past', dependsOnPast, '', 'warning') +
            kpiCard('High Retry Pipelines', highRetry, '', 'danger') +
            kpiCard('Currently Running', running, '', 'success') +
            '</div>';
    }

    async function loadOrchestration() {
        const responses = await Promise.all([
            fetchJSON('/api/v1/etl/orchestration/capabilities', { silent: true }),
            fetchJSON('/api/v1/etl/blueprints', { silent: true })
        ]);
        state.capabilities = (responses[0] && responses[0].capabilities) || [];
        state.blueprints = (responses[1] && responses[1].blueprints) || [];
        populateBlueprintDropdown();
        renderCapabilities();
        renderBlueprints();
        renderOrchestrationHealth();
    }

    function toInt(value, fallback) {
        const parsed = parseInt(value, 10);
        return isNaN(parsed) ? fallback : parsed;
    }

    function findBlueprint(id) {
        if (!id) return null;
        for (let i = 0; i < state.blueprints.length; i++) {
            if (state.blueprints[i].id === id) {
                return state.blueprints[i];
            }
        }
        return null;
    }

    window.previewBlueprintSelection = function() {
        const bpSel = document.getElementById('modal-blueprint');
        const bp = findBlueprint(bpSel ? bpSel.value : '');
        if (!bp) return;

        const nameEl = document.getElementById('modal-name');
        if (nameEl && !nameEl.value.trim()) {
            nameEl.value = bp.name + ' Pipeline';
        }

        const scheduleEl = document.getElementById('modal-schedule');
        if (scheduleEl && bp.default_schedule) {
            scheduleEl.value = bp.default_schedule;
        }

        const tagsEl = document.getElementById('modal-tags');
        if (tagsEl && bp.tags && bp.tags.length) {
            tagsEl.value = bp.tags.join(',');
        }

        const extractEl = document.getElementById('modal-etl-extract-connector');
        const loadEl = document.getElementById('modal-etl-load-connector');
        if (extractEl) {
            const extractStep = (bp.steps || []).find(function(s) { return s.type === 'extract' && s.connector; });
            if (extractStep) extractEl.value = extractStep.connector;
        }
        if (loadEl) {
            const loadStep = (bp.steps || []).slice().reverse().find(function(s) { return s.type === 'load' && s.connector; });
            if (loadStep) loadEl.value = loadStep.connector;
        }

        const qualityFlagEl = document.getElementById('modal-etl-quality-gate');
        if (qualityFlagEl) {
            const hasValidate = (bp.steps || []).some(function(s) { return s.type === 'validate' || s.type === 'deduplicate'; });
            qualityFlagEl.checked = hasValidate;
        }

        window.onExtractConnectorChange();
        window.onLoadConnectorChange();
    };

    function buildETLSteps() {
        const bpSel = document.getElementById('modal-blueprint');
        const bp = findBlueprint(bpSel ? bpSel.value : '');
        if (bp && bp.steps && bp.steps.length) {
            return bp.steps.map(function(step, index) {
                return {
                    id: 'step-' + (index + 1),
                    name: step.name,
                    type: step.type,
                    connector: step.connector || '',
                    config: {},
                    order: index + 1
                };
            });
        }

        const extractConnector = (document.getElementById('modal-etl-extract-connector').value || '').trim();
        const loadConnector = (document.getElementById('modal-etl-load-connector').value || '').trim();
        const qualityGate = document.getElementById('modal-etl-quality-gate').checked;

        const extractConfig = collectRuntimeStepConfig('modal-extract');
        const loadConfig = collectRuntimeStepConfig('modal-load');

        const steps = [
            { id: 'step-1', name: 'Extract', type: 'extract', connector: extractConnector, config: extractConfig, order: 1 },
            { id: 'step-2', name: 'Transform', type: 'transform', connector: '', config: {}, order: 2 }
        ];
        if (qualityGate) {
            steps.push({ id: 'step-3', name: 'Validate', type: 'validate', connector: '', config: {}, order: 3 });
            steps.push({ id: 'step-4', name: 'Deduplicate', type: 'deduplicate', connector: '', config: {}, order: 4 });
            steps.push({ id: 'step-5', name: 'Load', type: 'load', connector: loadConnector, config: loadConfig, order: 5 });
        } else {
            steps.push({ id: 'step-3', name: 'Load', type: 'load', connector: loadConnector, config: loadConfig, order: 3 });
        }
        return steps;
    }

    window.openCreateModal = function(type) {
        state.createType = type;
        state.currentEditingETLPipelineId = '';
        document.getElementById('modal-title').textContent = type === 'etl' ? 'Create ETL Pipeline' : 'Create CDC Pipeline';

        document.getElementById('modal-schedule-group').style.display = type === 'etl' ? 'block' : 'none';
        document.getElementById('modal-blueprint-group').style.display = type === 'etl' ? 'block' : 'none';
        document.getElementById('modal-etl-connector-group').style.display = type === 'etl' ? 'block' : 'none';
        document.getElementById('modal-orchestration-group').style.display = type === 'etl' ? 'grid' : 'none';
        document.getElementById('modal-orchestration-flags').style.display = type === 'etl' ? 'flex' : 'none';
        document.getElementById('modal-source-group').style.display = type === 'cdc' ? 'block' : 'none';
        document.getElementById('modal-sink-group').style.display = type === 'cdc' ? 'block' : 'none';

        document.getElementById('modal-name').value = '';
        document.getElementById('modal-desc').value = '';
        document.getElementById('modal-schedule').value = '';
        document.getElementById('modal-owner').value = 'data-platform';
        document.getElementById('modal-queue').value = 'default';
        document.getElementById('modal-retries').value = '2';
        document.getElementById('modal-retry-delay').value = '60';
        document.getElementById('modal-timeout').value = '1800';
        document.getElementById('modal-sla-seconds').value = '3600';
        document.getElementById('modal-max-active-runs').value = '1';
        document.getElementById('modal-concurrency').value = '4';
        document.getElementById('modal-priority').value = '5';
        document.getElementById('modal-alert-channels').value = 'slack';
        document.getElementById('modal-catchup').checked = false;
        document.getElementById('modal-depends-past').checked = false;
        document.getElementById('modal-tags').value = '';
        document.getElementById('modal-etl-quality-gate').checked = true;
        resetRuntimeStepConfig('modal-extract');
        resetRuntimeStepConfig('modal-load');

        const bpSel = document.getElementById('modal-blueprint');
        if (bpSel) bpSel.value = '';

        populateConnectorDropdowns();
        populateCDCTypeDropdowns();
        populateBlueprintDropdown();
        // Show/hide DB options based on the default selected connector
        window.onExtractConnectorChange();
        window.onLoadConnectorChange();
        document.getElementById('createModal').classList.add('visible');
    };

    window.closeCreateModal = function() {
        const modal = document.getElementById('createModal');
        if (modal) modal.classList.remove('visible');
    };

    window.createFromBlueprint = function(blueprintId) {
        window.openCreateModal('etl');
        const sel = document.getElementById('modal-blueprint');
        if (sel) {
            sel.value = blueprintId;
            window.previewBlueprintSelection();
        }
    };

    window.submitCreate = async function() {
        const name = (document.getElementById('modal-name').value || '').trim();
        const desc = (document.getElementById('modal-desc').value || '').trim();
        if (!name) {
            showToast('Pipeline name is required.', true);
            return;
        }

        let payload;
        let url;

        if (state.createType === 'etl') {
            payload = {
                name: name,
                description: desc,
                schedule: (document.getElementById('modal-schedule').value || '').trim(),
                steps: buildETLSteps(),
                tags: parseCSVList(document.getElementById('modal-tags').value || ''),
                config: {},
                orchestration: {
                    owner: (document.getElementById('modal-owner').value || '').trim(),
                    queue: (document.getElementById('modal-queue').value || '').trim(),
                    retries: toInt(document.getElementById('modal-retries').value, 2),
                    retry_delay_sec: toInt(document.getElementById('modal-retry-delay').value, 60),
                    timeout_sec: toInt(document.getElementById('modal-timeout').value, 1800),
                    sla_seconds: toInt(document.getElementById('modal-sla-seconds').value, 3600),
                    max_active_runs: toInt(document.getElementById('modal-max-active-runs').value, 1),
                    concurrency: toInt(document.getElementById('modal-concurrency').value, 4),
                    priority_weight: toInt(document.getElementById('modal-priority').value, 5),
                    catchup: document.getElementById('modal-catchup').checked,
                    depends_on_past: document.getElementById('modal-depends-past').checked,
                    alert_channels: parseCSVList(document.getElementById('modal-alert-channels').value || 'slack')
                }
            };
            url = '/api/v1/etl/pipelines';
        } else {
            const srcType = document.getElementById('modal-source-type').value;
            const sinkType = document.getElementById('modal-sink-type').value;
            payload = {
                name: name,
                description: desc,
                source: { type: srcType, connector: srcType, config: {} },
                sink: { type: sinkType, connector: sinkType, config: {} },
                filters: {},
                tags: parseCSVList(document.getElementById('modal-tags').value || '')
            };
            url = '/api/v1/cdc/pipelines';
        }

        const resp = await fetchJSON(url, { method: 'POST', body: payload });
        if (!resp) return;

        showToast('Pipeline created: ' + name, false);
        closeCreateModal();
        if (state.createType === 'etl') {
            await loadETL();
            await loadObservability();
            renderOrchestrationHealth();
        } else {
            await loadCDC();
            await loadObservability();
        }
    };

    async function initData() {
        await Promise.all([loadETL(), loadCDC(), loadObservability(), loadConnectors(), loadOrchestration()]);
    }

    // ---- DB/API Runtime Credential Option Toggle ----
    window.onExtractConnectorChange = function() {
        const sel = document.getElementById('modal-etl-extract-connector');
        if (!sel) return;
        updateCredentialOptionVisibility(sel.value, 'modal-extract-db-options', 'modal-extract-other-cred-options');
    };

    window.onLoadConnectorChange = function() {
        const sel = document.getElementById('modal-etl-load-connector');
        if (!sel) return;
        updateCredentialOptionVisibility(sel.value, 'modal-load-db-options', 'modal-load-other-cred-options');
    };

    window.onEditExtractConnectorChange = function() {
        const sel = document.getElementById('edit-etl-extract-connector');
        if (!sel) return;
        updateCredentialOptionVisibility(sel.value, 'edit-extract-db-options', 'edit-extract-other-cred-options');
    };

    window.onEditLoadConnectorChange = function() {
        const sel = document.getElementById('edit-etl-load-connector');
        if (!sel) return;
        updateCredentialOptionVisibility(sel.value, 'edit-load-db-options', 'edit-load-other-cred-options');
    };

    // ---- Edit ETL Pipeline ----
    window.editETLPipeline = function(id) {
        const p = state.etlPipelines.find(function(x) { return x.id === id; });
        if (!p) return;
        state.currentEditingETLPipelineId = p.id;
        document.getElementById('edit-pipeline-id').value = p.id;
        document.getElementById('edit-pipeline-type').value = 'etl';
        document.getElementById('edit-pipeline-title').textContent = 'Edit ETL Pipeline';
        document.getElementById('edit-pipeline-name').value = p.name || '';
        document.getElementById('edit-pipeline-desc').value = p.description || '';
        document.getElementById('edit-pipeline-schedule').value = p.schedule || '';
        document.getElementById('edit-pipeline-schedule-group').style.display = 'block';
        document.getElementById('edit-pipeline-tags').value = (p.tags || []).join(', ');

        const etlGroup = document.getElementById('edit-pipeline-etl-connector-group');
        if (etlGroup) etlGroup.style.display = 'block';
        populateEditConnectorDropdowns();

        const steps = p.steps || [];
        const extractStep = steps.find(function(s) { return (s.type || '').toLowerCase() === 'extract'; });
        const loadStep = steps.slice().reverse().find(function(s) { return (s.type || '').toLowerCase() === 'load'; });

        const extractSel = document.getElementById('edit-etl-extract-connector');
        const loadSel = document.getElementById('edit-etl-load-connector');
        if (extractSel && extractStep && extractStep.connector) extractSel.value = extractStep.connector;
        if (loadSel && loadStep && loadStep.connector) loadSel.value = loadStep.connector;

        fillRuntimeStepConfig('edit-extract', extractStep ? extractStep.config : {});
        fillRuntimeStepConfig('edit-load', loadStep ? loadStep.config : {});
        window.onEditExtractConnectorChange();
        window.onEditLoadConnectorChange();
        document.getElementById('editPipelineModal').classList.add('visible');
    };

    // ---- Edit CDC Pipeline ----
    window.editCDCPipeline = function(id) {
        const p = state.cdcPipelines.find(function(x) { return x.id === id; });
        if (!p) return;
        state.currentEditingETLPipelineId = '';
        document.getElementById('edit-pipeline-id').value = p.id;
        document.getElementById('edit-pipeline-type').value = 'cdc';
        document.getElementById('edit-pipeline-title').textContent = 'Edit CDC Pipeline';
        document.getElementById('edit-pipeline-name').value = p.name || '';
        document.getElementById('edit-pipeline-desc').value = p.description || '';
        document.getElementById('edit-pipeline-schedule-group').style.display = 'none';
        document.getElementById('edit-pipeline-tags').value = (p.tags || []).join(', ');
        const etlGroup = document.getElementById('edit-pipeline-etl-connector-group');
        if (etlGroup) etlGroup.style.display = 'none';
        document.getElementById('editPipelineModal').classList.add('visible');
    };

    window.closeEditPipelineModal = function() {
        state.currentEditingETLPipelineId = '';
        document.getElementById('editPipelineModal').classList.remove('visible');
    };

    window.submitEditPipeline = async function() {
        const id = document.getElementById('edit-pipeline-id').value;
        const type = document.getElementById('edit-pipeline-type').value;
        const payload = {
            name: (document.getElementById('edit-pipeline-name').value || '').trim(),
            description: (document.getElementById('edit-pipeline-desc').value || '').trim(),
            tags: parseCSVList(document.getElementById('edit-pipeline-tags').value || '')
        };
        if (type === 'etl') {
            payload.schedule = (document.getElementById('edit-pipeline-schedule').value || '').trim();
            const current = state.etlPipelines.find(function(p) { return p.id === id; });
            payload.steps = buildUpdatedETLStepsForEdit(current || {});
        }
        const url = type === 'etl'
            ? '/api/v1/etl/pipelines/' + encodeURIComponent(id)
            : '/api/v1/cdc/pipelines/' + encodeURIComponent(id);
        const resp = await fetchJSON(url, { method: 'PUT', body: payload });
        if (!resp) return;
        showToast('Pipeline updated', false);
        closeEditPipelineModal();
        if (type === 'etl') { await loadETL(); } else { await loadCDC(); }
        await loadObservability();
    };

    // ---- Edit Connector ----
    window.editConnector = function(id) {
        const c = state.etlConnectors.find(function(x) { return x.id === id; });
        if (!c) return;
        document.getElementById('edit-connector-id').value = c.id;
        document.getElementById('edit-connector-name').value = c.name || '';
        document.getElementById('edit-connector-category').value = c.category || 'database';
        document.getElementById('edit-connector-icon').value = c.icon || '';
        document.getElementById('edit-connector-version').value = c.version || '';
        document.getElementById('edit-connector-supported-as').value = (c.supported_as || []).join(', ');
        document.getElementById('edit-connector-auth').value = (c.auth_modes || []).join(', ');
        document.getElementById('edit-connector-config').value = (c.config_keys || []).join(', ');
        document.getElementById('edit-connector-description').value = c.description || '';
        document.getElementById('edit-connector-incremental').checked = !!c.supports_incremental;
        document.getElementById('edit-connector-schema').checked = !!c.schema_discovery;
        document.getElementById('edit-connector-cdc').checked = !!c.supports_cdc;
        fillConnectorCredentialInputs('edit', c);
        document.getElementById('editConnectorModal').classList.add('visible');
    };

    window.closeEditConnectorModal = function() {
        document.getElementById('editConnectorModal').classList.remove('visible');
    };

    window.submitEditConnector = async function() {
        const id = document.getElementById('edit-connector-id').value;
        const credentialMeta = collectConnectorCredentialMetadata('edit');
        const payload = {
            name: (document.getElementById('edit-connector-name').value || '').trim(),
            category: (document.getElementById('edit-connector-category').value || '').trim(),
            icon: (document.getElementById('edit-connector-icon').value || '').trim(),
            version: (document.getElementById('edit-connector-version').value || '').trim(),
            supported_as: parseCSVList(document.getElementById('edit-connector-supported-as').value || ''),
            auth_modes: mergeUniqueLists(parseCSVList(document.getElementById('edit-connector-auth').value || ''), credentialMeta.authModes),
            config_keys: mergeUniqueLists(parseCSVList(document.getElementById('edit-connector-config').value || ''), credentialMeta.configKeys),
            description: (document.getElementById('edit-connector-description').value || '').trim(),
            supports_incremental: document.getElementById('edit-connector-incremental').checked,
            schema_discovery: document.getElementById('edit-connector-schema').checked,
            supports_cdc: document.getElementById('edit-connector-cdc').checked
        };
        const resp = await fetchJSON('/api/v1/etl/connectors/' + encodeURIComponent(id), { method: 'PUT', body: payload });
        if (!resp) return;
        showToast('Connector updated', false);
        closeEditConnectorModal();
        await loadConnectors();
    };

    window.deleteConnector = async function(id) {
        if (!confirm('Delete connector ' + id + '?')) return;
        const resp = await fetchJSON('/api/v1/etl/connectors/' + encodeURIComponent(id), { method: 'DELETE' });
        if (resp) {
            await loadConnectors();
        }
    };

    function bindLocalEvents() {
        const modal = document.getElementById('createModal');
        if (modal) {
            modal.addEventListener('click', function(evt) {
                if (evt.target === modal) {
                    closeCreateModal();
                }
            });
        }
        const editPModal = document.getElementById('editPipelineModal');
        if (editPModal) {
            editPModal.addEventListener('click', function(evt) {
                if (evt.target === editPModal) closeEditPipelineModal();
            });
        }
        const editCModal = document.getElementById('editConnectorModal');
        if (editCModal) {
            editCModal.addEventListener('click', function(evt) {
                if (evt.target === editCModal) closeEditConnectorModal();
            });
        }
    }

    async function init() {
        bindLocalEvents();
        await initData();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
