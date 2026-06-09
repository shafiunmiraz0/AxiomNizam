(function() {
    'use strict';

    var API = window.resolveBackendURL() + '/api/v1/conductor';
    var wsURL = '';
    var streamWS = null;
    var streamConnected = false;
    var cachedProducers = [];
    var cachedConsumers = [];
    var cachedConnections = [];
    var streamTrendPoints = [];
    var STREAM_TREND_LIMIT = 30;

    // ---- Toast Notification ----
    function showToast(message, type) {
        type = type || 'info';
        var container = document.getElementById('conductorToast');
        if (!container) return;
        var item = document.createElement('div');
        item.className = 'conductor-toast-item ' + type;
        item.textContent = message;
        container.appendChild(item);
        setTimeout(function() { item.style.opacity = '0'; item.style.transition = 'opacity 0.3s'; }, 3500);
        setTimeout(function() { if (item.parentNode) item.parentNode.removeChild(item); }, 4000);
    }

    function readCookie(name) {
        try {
            var key = name + '=';
            var cookies = (document.cookie || '').split(';');
            for (var i = 0; i < cookies.length; i++) {
                var c = cookies[i].trim();
                if (c.indexOf(key) === 0) {
                    return decodeURIComponent(c.substring(key.length));
                }
            }
        } catch (e) {}
        return '';
    }

    function resolveAuthToken() {
        var token = '';
        try {
            token = localStorage.getItem('authToken') || localStorage.getItem('auth_token') || '';
        } catch (e) {}
        if (token) return token;
        return readCookie('authToken') || readCookie('auth_token') || '';
    }

    // ---- Auth & API helpers with error handling ----
    function authHeaders() {
        var token = resolveAuthToken();
        var headers = { 'Content-Type': 'application/json' };
        if (token) headers['Authorization'] = 'Bearer ' + token;
        return headers;
    }

    function handleResponse(resp) {
        if (!resp.ok) {
            return resp.json().catch(function() { return { error: 'HTTP ' + resp.status + ' ' + resp.statusText }; }).then(function(body) {
                var msg = body.error || body.message || ('HTTP ' + resp.status);
                throw new Error(msg);
            });
        }
        return resp.json();
    }

    function apiGet(path) {
        return fetch(API + path, { headers: authHeaders() }).then(handleResponse);
    }
    function apiPost(path, body) {
        return fetch(API + path, { method: 'POST', headers: authHeaders(), body: JSON.stringify(body) }).then(handleResponse);
    }
    function apiPatch(path, body) {
        return fetch(API + path, { method: 'PATCH', headers: authHeaders(), body: JSON.stringify(body) }).then(handleResponse);
    }
    function apiDelete(path) {
        return fetch(API + path, { method: 'DELETE', headers: authHeaders() }).then(handleResponse);
    }

    // ---- Formatting helpers ----
    function esc(str) {
        if (!str) return '';
        var d = document.createElement('div'); d.textContent = str; return d.innerHTML;
    }
    function statusBadge(status) {
        return '<span class="status-badge status-' + esc(status || 'unknown') + '">' + esc(status || 'unknown') + '</span>';
    }
    function backendBadge(backend) {
        return '<span class="backend-badge backend-' + esc(backend || 'memory') + '">' + esc(backend || 'memory') + '</span>';
    }
    function formatTime(ts) {
        if (!ts) return '-';
        var d = new Date(ts);
        if (isNaN(d.getTime())) return '-';
        return d.toLocaleString();
    }
    function jsonPreview(obj) {
        if (!obj) return '-';
        var s = JSON.stringify(obj);
        if (s.length > 80) s = s.substring(0, 80) + '...';
        return '<span class="json-preview" title="' + esc(s) + '">' + esc(s) + '</span>';
    }

    // ---- API-builder style hover panel renderers ----
    function hoverMetricItem(label, value) {
        return '<div class="conductor-hover-metric-item"><div class="conductor-hover-metric-label">' + esc(label) + '</div><div class="conductor-hover-metric-value">' + esc(String(value)) + '</div></div>';
    }

    function producerHoverPanel(p) {
        return '<div class="conductor-hover-panel" role="tooltip">' +
            '<div class="conductor-hover-title">Producer Metrics</div>' +
            '<div class="conductor-hover-grid">' +
                hoverMetricItem('Status', p.status || '-') +
                hoverMetricItem('Backend', p.backend || '-') +
                hoverMetricItem('Messages Sent', p.messagesSent || 0) +
                hoverMetricItem('Content Type', p.contentType || '-') +
                hoverMetricItem('Last Sent', formatTime(p.lastSentAt)) +
                hoverMetricItem('Created', formatTime(p.createdAt)) +
            '</div>' +
            (p.routingKey ? '<div class="conductor-hover-footnote">Routing Key: ' + esc(p.routingKey) + '</div>' : '') +
        '</div>';
    }

    function consumerHoverPanel(c) {
        return '<div class="conductor-hover-panel" role="tooltip">' +
            '<div class="conductor-hover-title">Consumer Metrics</div>' +
            '<div class="conductor-hover-grid">' +
                hoverMetricItem('Status', c.status || '-') +
                hoverMetricItem('Backend', c.backend || '-') +
                hoverMetricItem('Received', c.messagesReceived || 0) +
                hoverMetricItem('Acked', c.messagesAcked || 0) +
                hoverMetricItem('Failed', c.messagesFailed || 0) +
                hoverMetricItem('Last Received', formatTime(c.lastReceivedAt)) +
                hoverMetricItem('Prefetch', (c.config && c.config.prefetchCount) || '-') +
                hoverMetricItem('DLQ', (c.config && c.config.dlqEnabled) ? 'Enabled' : 'Disabled') +
            '</div>' +
        '</div>';
    }

    function nameWithHover(name, id, hoverPanel) {
        return '<div class="conductor-name-with-hover">' +
            '<span class="conductor-name-text">' + esc(name) + '</span>' +
            '<button type="button" class="conductor-hover-trigger" tabindex="0" aria-label="View metrics for ' + esc(name) + '"><span class="conductor-hover-trigger-dot" aria-hidden="true"></span>Details</button>' +
            hoverPanel +
        '</div>' +
        '<small class="conductor-name-id">' + esc(id) + '</small>';
    }

    var activeHoverHost = null;
    var activeHoverPanel = null;

    function closeHoverPanel() {
        if (!activeHoverHost || !activeHoverPanel) return;
        activeHoverHost.classList.remove('is-hover-open');
        activeHoverPanel.style.left = '-9999px';
        activeHoverPanel.style.top = '-9999px';
        activeHoverHost = null;
        activeHoverPanel = null;
    }

    function positionHoverPanel(panel, trigger) {
        if (!panel || !trigger) return;
        panel.style.left = '0px';
        panel.style.top = '0px';

        var triggerRect = trigger.getBoundingClientRect();
        var panelRect = panel.getBoundingClientRect();
        var gutter = 12;

        var left = triggerRect.left;
        if (left + panelRect.width > window.innerWidth - gutter) {
            left = window.innerWidth - panelRect.width - gutter;
        }
        if (left < gutter) left = gutter;

        var top = triggerRect.bottom + 10;
        if (top + panelRect.height > window.innerHeight - gutter) {
            top = Math.max(gutter, triggerRect.top - panelRect.height - 10);
        }

        panel.style.left = Math.round(left) + 'px';
        panel.style.top = Math.round(top) + 'px';
    }

    function openHoverPanelFor(host) {
        if (!host) return;
        var trigger = host.querySelector('.conductor-hover-trigger');
        var panel = host.querySelector('.conductor-hover-panel');
        if (!trigger || !panel) return;

        if (activeHoverHost && activeHoverHost !== host) {
            activeHoverHost.classList.remove('is-hover-open');
        }

        host.classList.add('is-hover-open');
        positionHoverPanel(panel, trigger);
        activeHoverHost = host;
        activeHoverPanel = panel;
    }

    document.addEventListener('mouseover', function(e) {
        var host = e.target.closest('.conductor-name-with-hover');
        if (host) openHoverPanelFor(host);
    });

    document.addEventListener('mouseout', function(e) {
        if (!activeHoverHost) return;
        var host = e.target.closest('.conductor-name-with-hover');
        if (host !== activeHoverHost) return;
        if (!e.relatedTarget || !activeHoverHost.contains(e.relatedTarget)) {
            closeHoverPanel();
        }
    });

    document.addEventListener('focusin', function(e) {
        var host = e.target.closest('.conductor-name-with-hover');
        if (host) openHoverPanelFor(host);
    });

    document.addEventListener('click', function(e) {
        var trigger = e.target.closest('.conductor-hover-trigger');
        if (trigger) {
            var host = trigger.closest('.conductor-name-with-hover');
            if (host === activeHoverHost) {
                closeHoverPanel();
            } else {
                openHoverPanelFor(host);
            }
            e.preventDefault();
            return;
        }

        if (activeHoverHost && !activeHoverHost.contains(e.target)) {
            closeHoverPanel();
        }
    });

    document.addEventListener('keydown', function(e) {
        if (e.key === 'Escape') closeHoverPanel();
    });

    window.addEventListener('scroll', function() {
        if (!activeHoverHost || !activeHoverPanel) return;
        var trigger = activeHoverHost.querySelector('.conductor-hover-trigger');
        if (trigger) positionHoverPanel(activeHoverPanel, trigger);
    }, true);

    window.addEventListener('resize', function() {
        if (activeHoverHost && activeHoverPanel) {
            var trigger = activeHoverHost.querySelector('.conductor-hover-trigger');
            if (trigger) positionHoverPanel(activeHoverPanel, trigger);
        }
        drawStreamGraph();
    });

    // ---- Stats ----
    function loadStats() {
        apiGet('/stats').then(function(data) {
            document.getElementById('statProducers').textContent = data.producers || 0;
            document.getElementById('statConsumers').textContent = data.consumers || 0;
            document.getElementById('statSent').textContent = data.totalSent || 0;
            document.getElementById('statReceived').textContent = data.totalReceived || 0;
            document.getElementById('statAcked').textContent = data.totalAcked || 0;
            document.getElementById('statFailed').textContent = data.totalFailed || 0;
            document.getElementById('statDLQ').textContent = data.dlqSize || 0;
        }).catch(function(err) { showToast('Failed to load stats: ' + err.message, 'error'); });
    }

    // ---- Connections ----
    function loadConnections() {
        apiGet('/connections').then(function(data) {
            cachedConnections = data.connections || [];
            renderConnections(cachedConnections);
            populateBackendSelects();
        }).catch(function(err) { showToast('Failed to load connections: ' + err.message, 'error'); });
    }

    function renderConnections(conns) {
        var grid = document.getElementById('connectionsGrid');
        if (!grid) return;
        if (!conns.length) {
            grid.innerHTML = '<div style="padding:24px;text-align:center;color:var(--text-muted)">No backends configured. Click "+ Connect Backend" to add RabbitMQ or Kafka.</div>';
            return;
        }
        grid.innerHTML = conns.map(function(c) {
            var icon = c.type === 'rabbitmq' ? '🐰' : c.type === 'kafka' ? '📨' : '💾';
            var label = c.type === 'rabbitmq' ? 'RabbitMQ' : c.type === 'kafka' ? 'Apache Kafka' : c.type;
            var statusClass = c.status === 'connected' ? 'connected' : c.status === 'error' ? 'error' : 'disconnected';
            var actions = '';
            if (c.status === 'connected') {
                actions = '<button class="btn-secondary" onclick="window._conductorDisconnect(\'' + esc(c.type) + '\')">Disconnect</button>';
            } else {
                actions = '<button class="btn-primary" onclick="window._conductorReconnect(\'' + esc(c.type) + '\')">Reconnect</button>';
            }
            return '<div class="conn-card">' +
                '<div class="conn-card-header">' +
                    '<span class="conn-card-type">' + icon + ' ' + esc(label) + '</span>' +
                    '<span class="conn-card-status ' + statusClass + '">' + esc(c.status) + '</span>' +
                '</div>' +
                (c.url ? '<div class="conn-card-url">' + esc(c.url) + '</div>' : '') +
                (c.error ? '<div style="color:#ef4444;font-size:0.82rem;margin-bottom:8px">' + esc(c.error) + '</div>' : '') +
                '<div class="conn-card-stats">' +
                    '<div class="conn-card-stat"><div class="conn-card-stat-value">' + (c.producers || 0) + '</div><div class="conn-card-stat-label">Producers</div></div>' +
                    '<div class="conn-card-stat"><div class="conn-card-stat-value">' + (c.consumers || 0) + '</div><div class="conn-card-stat-label">Consumers</div></div>' +
                '</div>' +
                '<div class="conn-card-actions">' + actions + '</div>' +
            '</div>';
        }).join('');
    }

    function populateBackendSelects() {
        var options = '<option value="memory">Memory (in-process)</option>';
        (cachedConnections || []).forEach(function(c) {
            if (c.status === 'connected') {
                var label = c.type === 'rabbitmq' ? '🐰 RabbitMQ' : c.type === 'kafka' ? '📨 Kafka' : c.type;
                options += '<option value="' + esc(c.type) + '">' + label + '</option>';
            }
        });
        var prodSel = document.getElementById('prodBackend');
        var consSel = document.getElementById('consBackend');
        if (prodSel) prodSel.innerHTML = options;
        if (consSel) consSel.innerHTML = options;
    }

    window.showConnectModal = function() {
        document.getElementById('connectType').value = 'rabbitmq';
        document.getElementById('connectURL').value = '';
        document.getElementById('connectBrokers').value = '';
        onConnectTypeChange();
        document.getElementById('connectBackendModal').style.display = 'flex';
    };

    window.onConnectTypeChange = function() {
        var type = document.getElementById('connectType').value;
        document.getElementById('connectURLGroup').style.display = type === 'rabbitmq' ? '' : 'none';
        document.getElementById('connectBrokersGroup').style.display = type === 'kafka' ? '' : 'none';
    };

    window.connectBackend = function() {
        var type = document.getElementById('connectType').value;
        var req = { type: type };
        if (type === 'rabbitmq') {
            req.url = document.getElementById('connectURL').value.trim();
            if (!req.url) { showToast('AMQP URL is required', 'error'); return; }
        } else if (type === 'kafka') {
            var raw = document.getElementById('connectBrokers').value.trim();
            if (!raw) { showToast('Broker list is required', 'error'); return; }
            req.brokers = raw.split(',').map(function(b) { return b.trim(); }).filter(Boolean);
        }
        showToast('Connecting to ' + type + '...', 'info');
        apiPost('/connections', req).then(function() {
            closeModal('connectBackendModal');
            showToast(type + ' connected successfully', 'success');
            loadConnections();
        }).catch(function(err) { showToast('Connection failed: ' + err.message, 'error'); });
    };

    window._conductorDisconnect = function(type) {
        if (!confirm('Disconnect ' + type + '? Existing producers/consumers on this backend will stop working.')) return;
        apiDelete('/connections/' + encodeURIComponent(type)).then(function() {
            showToast(type + ' disconnected', 'success');
            loadConnections();
        }).catch(function(err) { showToast('Disconnect failed: ' + err.message, 'error'); });
    };

    window._conductorReconnect = function(type) {
        // Re-open the connect modal pre-filled
        document.getElementById('connectType').value = type;
        onConnectTypeChange();
        document.getElementById('connectBackendModal').style.display = 'flex';
    };

    // ---- Producer backend change hints ----
    window.onProducerBackendChange = function() {
        var backend = document.getElementById('prodBackend').value;
        var label = document.getElementById('prodExchangeLabel');
        var hint = document.getElementById('prodBackendHint');
        if (backend === 'kafka') {
            if (label) label.textContent = 'Topic';
            if (hint) hint.textContent = 'Kafka backend selected — messages will be sent to the specified topic.';
        } else if (backend === 'rabbitmq') {
            if (label) label.textContent = 'Exchange';
            if (hint) hint.textContent = 'RabbitMQ backend selected — messages will be published to the specified exchange.';
        } else {
            if (label) label.textContent = 'Exchange / Topic';
            if (hint) hint.textContent = 'Memory backend — messages stay in-process, no external broker needed.';
        }
    };

    window.onConsumerBackendChange = function() {
        var backend = document.getElementById('consBackend').value;
        var label = document.getElementById('consQueueLabel');
        var hint = document.getElementById('consBackendHint');
        if (backend === 'kafka') {
            if (label) label.textContent = 'Topic';
            if (hint) hint.textContent = 'Kafka backend selected — will consume from the specified topic.';
        } else if (backend === 'rabbitmq') {
            if (label) label.textContent = 'Queue';
            if (hint) hint.textContent = 'RabbitMQ backend selected — will consume from the specified queue.';
        } else {
            if (label) label.textContent = 'Queue / Topic';
            if (hint) hint.textContent = 'Memory backend — messages stay in-process.';
        }
    };

    // ---- Producers ----
    function loadProducers() {
        closeHoverPanel();
        apiGet('/producers').then(function(data) {
            var prods = data.producers || [];
            cachedProducers = prods;
            var tbody = document.getElementById('producersBody');
            if (!prods.length) {
                tbody.innerHTML = '<tr><td colspan="8" class="empty-row">No producers configured yet</td></tr>';
                return;
            }
            tbody.innerHTML = prods.map(function(p) {
                var target = p.backend === 'kafka' ? (p.topic || '-') : (p.exchange || '-');
                return '<tr>' +
                    '<td>' + nameWithHover(p.name, p.id, producerHoverPanel(p)) + '</td>' +
                    '<td>' + backendBadge(p.backend) + '</td>' +
                    '<td>' + esc(target) + '</td>' +
                    '<td>' + esc(p.routingKey || '-') + '</td>' +
                    '<td>' + statusBadge(p.status) + '</td>' +
                    '<td>' + (p.messagesSent || 0) + '</td>' +
                    '<td>' + formatTime(p.lastSentAt) + '</td>' +
                    '<td>' + producerActions(p) + '</td>' +
                    '</tr>';
            }).join('');
            updateProducerSelect(prods);
        }).catch(function(err) { showToast('Failed to load producers: ' + err.message, 'error'); });
    }

    function producerActions(p) {
        var html = '<button class="action-btn" onclick="window._conductorEditProducer(\'' + esc(p.id) + '\')" title="Edit">✏️</button>';
        if (p.status === 'active') {
            html += '<button class="action-btn" onclick="window._conductorPauseProducer(\'' + esc(p.id) + '\')">Pause</button>';
        } else if (p.status === 'paused') {
            html += '<button class="action-btn" onclick="window._conductorResumeProducer(\'' + esc(p.id) + '\')">Resume</button>';
        }
        html += '<button class="action-btn danger" onclick="window._conductorDeleteProducer(\'' + esc(p.id) + '\')">Delete</button>';
        return html;
    }

    function updateProducerSelect(prods) {
        var sel = document.getElementById('publishProducer');
        if (!sel) return;
        sel.innerHTML = prods.filter(function(p) { return p.status === 'active'; }).map(function(p) {
            return '<option value="' + esc(p.id) + '">' + esc(p.name) + ' (' + esc(p.backend) + ')</option>';
        }).join('');
    }

    window._conductorPauseProducer = function(id) {
        apiPost('/producers/' + encodeURIComponent(id) + '/pause', {}).then(function() {
            showToast('Producer paused', 'success'); loadProducers(); loadStats();
        }).catch(function(err) { showToast('Failed to pause producer: ' + err.message, 'error'); });
    };
    window._conductorResumeProducer = function(id) {
        apiPost('/producers/' + encodeURIComponent(id) + '/resume', {}).then(function() {
            showToast('Producer resumed', 'success'); loadProducers(); loadStats();
        }).catch(function(err) { showToast('Failed to resume producer: ' + err.message, 'error'); });
    };
    window._conductorDeleteProducer = function(id) {
        if (!confirm('Delete this producer?')) return;
        apiDelete('/producers/' + encodeURIComponent(id)).then(function() {
            showToast('Producer deleted', 'success'); loadProducers(); loadStats();
        }).catch(function(err) { showToast('Failed to delete producer: ' + err.message, 'error'); });
    };

    window._conductorEditProducer = function(id) {
        var p = cachedProducers.find(function(x) { return x.id === id; });
        if (!p) { showToast('Producer not found', 'error'); return; }
        document.getElementById('editProdId').value = p.id;
        document.getElementById('editProdName').value = p.name || '';
        document.getElementById('editProdExchange').value = (p.backend === 'kafka' ? p.topic : p.exchange) || '';
        document.getElementById('editProdRoutingKey').value = p.routingKey || '';
        document.getElementById('editProdContentType').value = p.contentType || 'application/json';
        document.getElementById('editProdPersistent').checked = p.config && p.config.persistent;
        document.getElementById('editProducerModal').style.display = 'flex';
    };

    window.saveEditProducer = function() {
        var id = document.getElementById('editProdId').value;
        var p = cachedProducers.find(function(x) { return x.id === id; });
        var exchTopic = document.getElementById('editProdExchange').value;
        var req = {
            name: document.getElementById('editProdName').value,
            backend: p ? p.backend : 'rabbitmq',
            contentType: document.getElementById('editProdContentType').value,
            config: { persistent: document.getElementById('editProdPersistent').checked }
        };
        if (p && p.backend === 'kafka') {
            req.topic = exchTopic;
        } else {
            req.exchange = exchTopic;
        }
        req.routingKey = document.getElementById('editProdRoutingKey').value;
        apiPatch('/producers/' + encodeURIComponent(id), req).then(function() {
            closeModal('editProducerModal');
            showToast('Producer updated', 'success');
            loadProducers(); loadStats();
        }).catch(function(err) { showToast('Failed to update producer: ' + err.message, 'error'); });
    };

    window.showCreateProducerModal = function() {
        document.getElementById('prodName').value = '';
        document.getElementById('prodExchange').value = '';
        document.getElementById('prodRoutingKey').value = '';
        document.getElementById('prodContentType').value = 'application/json';
        document.getElementById('prodPersistent').checked = true;
        populateBackendSelects();
        onProducerBackendChange();
        document.getElementById('createProducerModal').style.display = 'flex';
    };

    window.createProducer = function() {
        var name = document.getElementById('prodName').value.trim();
        if (!name) { showToast('Producer name is required', 'error'); return; }
        var backend = document.getElementById('prodBackend').value;
        var req = {
            name: name,
            backend: backend,
            contentType: document.getElementById('prodContentType').value || 'application/json',
            config: { persistent: document.getElementById('prodPersistent').checked }
        };
        var exchTopic = document.getElementById('prodExchange').value;
        if (backend === 'kafka') {
            req.topic = exchTopic;
        } else {
            req.exchange = exchTopic;
        }
        req.routingKey = document.getElementById('prodRoutingKey').value;
        apiPost('/producers', req).then(function() {
            closeModal('createProducerModal');
            showToast('Producer created', 'success');
            loadProducers(); loadStats();
        }).catch(function(err) { showToast('Failed to create producer: ' + err.message, 'error'); });
    };

    // ---- Consumers ----
    function loadConsumers() {
        closeHoverPanel();
        apiGet('/consumers').then(function(data) {
            var cons = data.consumers || [];
            cachedConsumers = cons;
            var tbody = document.getElementById('consumersBody');
            if (!cons.length) {
                tbody.innerHTML = '<tr><td colspan="9" class="empty-row">No consumers configured yet</td></tr>';
                return;
            }
            tbody.innerHTML = cons.map(function(c) {
                var target = c.backend === 'kafka' ? (c.topic || '-') : (c.queue || '-');
                return '<tr>' +
                    '<td>' + nameWithHover(c.name, c.id, consumerHoverPanel(c)) + '</td>' +
                    '<td>' + backendBadge(c.backend) + '</td>' +
                    '<td>' + esc(target) + '</td>' +
                    '<td>' + esc(c.consumerGroup || '-') + '</td>' +
                    '<td>' + statusBadge(c.status) + '</td>' +
                    '<td>' + (c.messagesReceived || 0) + '</td>' +
                    '<td>' + (c.messagesAcked || 0) + '</td>' +
                    '<td>' + (c.messagesFailed || 0) + '</td>' +
                    '<td>' + consumerActions(c) + '</td>' +
                    '</tr>';
            }).join('');
        }).catch(function(err) { showToast('Failed to load consumers: ' + err.message, 'error'); });
    }

    function consumerActions(c) {
        var html = '<button class="action-btn" onclick="window._conductorEditConsumer(\'' + esc(c.id) + '\')" title="Edit">✏️</button>';
        if (c.status === 'active') {
            html += '<button class="action-btn" onclick="window._conductorPauseConsumer(\'' + esc(c.id) + '\')">Pause</button>';
        } else if (c.status === 'paused') {
            html += '<button class="action-btn" onclick="window._conductorResumeConsumer(\'' + esc(c.id) + '\')">Resume</button>';
        }
        html += '<button class="action-btn danger" onclick="window._conductorDeleteConsumer(\'' + esc(c.id) + '\')">Delete</button>';
        return html;
    }

    window._conductorPauseConsumer = function(id) {
        apiPost('/consumers/' + encodeURIComponent(id) + '/pause', {}).then(function() {
            showToast('Consumer paused', 'success'); loadConsumers(); loadStats();
        }).catch(function(err) { showToast('Failed to pause consumer: ' + err.message, 'error'); });
    };
    window._conductorResumeConsumer = function(id) {
        apiPost('/consumers/' + encodeURIComponent(id) + '/resume', {}).then(function() {
            showToast('Consumer resumed', 'success'); loadConsumers(); loadStats();
        }).catch(function(err) { showToast('Failed to resume consumer: ' + err.message, 'error'); });
    };
    window._conductorDeleteConsumer = function(id) {
        if (!confirm('Delete this consumer?')) return;
        apiDelete('/consumers/' + encodeURIComponent(id)).then(function() {
            showToast('Consumer deleted', 'success'); loadConsumers(); loadStats();
        }).catch(function(err) { showToast('Failed to delete consumer: ' + err.message, 'error'); });
    };

    window._conductorEditConsumer = function(id) {
        var c = cachedConsumers.find(function(x) { return x.id === id; });
        if (!c) { showToast('Consumer not found', 'error'); return; }
        document.getElementById('editConsId').value = c.id;
        document.getElementById('editConsName').value = c.name || '';
        document.getElementById('editConsQueue').value = (c.backend === 'kafka' ? c.topic : c.queue) || '';
        document.getElementById('editConsExchange').value = c.exchange || '';
        document.getElementById('editConsRoutingKey').value = c.routingKey || '';
        document.getElementById('editConsGroup').value = c.consumerGroup || '';
        document.getElementById('editConsPrefetch').value = (c.config && c.config.prefetchCount) || 10;
        document.getElementById('editConsMaxRetries').value = (c.config && c.config.maxRetries) || 3;
        document.getElementById('editConsDLQ').checked = c.config && c.config.dlqEnabled;
        document.getElementById('editConsumerModal').style.display = 'flex';
    };

    window.saveEditConsumer = function() {
        var id = document.getElementById('editConsId').value;
        var c = cachedConsumers.find(function(x) { return x.id === id; });
        var queueTopic = document.getElementById('editConsQueue').value;
        var req = {
            name: document.getElementById('editConsName').value,
            backend: c ? c.backend : 'rabbitmq',
            consumerGroup: document.getElementById('editConsGroup').value,
            config: {
                prefetchCount: parseInt(document.getElementById('editConsPrefetch').value) || 10,
                maxRetries: parseInt(document.getElementById('editConsMaxRetries').value) || 3,
                dlqEnabled: document.getElementById('editConsDLQ').checked
            }
        };
        if (c && c.backend === 'kafka') {
            req.topic = queueTopic;
        } else {
            req.queue = queueTopic;
        }
        req.exchange = document.getElementById('editConsExchange').value;
        req.routingKey = document.getElementById('editConsRoutingKey').value;
        apiPatch('/consumers/' + encodeURIComponent(id), req).then(function() {
            closeModal('editConsumerModal');
            showToast('Consumer updated', 'success');
            loadConsumers(); loadStats();
        }).catch(function(err) { showToast('Failed to update consumer: ' + err.message, 'error'); });
    };

    window.showCreateConsumerModal = function() {
        document.getElementById('consName').value = '';
        document.getElementById('consQueue').value = '';
        document.getElementById('consExchange').value = '';
        document.getElementById('consRoutingKey').value = '';
        document.getElementById('consGroup').value = '';
        document.getElementById('consPrefetch').value = '10';
        document.getElementById('consMaxRetries').value = '3';
        document.getElementById('consDLQ').checked = true;
        populateBackendSelects();
        onConsumerBackendChange();
        document.getElementById('createConsumerModal').style.display = 'flex';
    };

    window.createConsumer = function() {
        var name = document.getElementById('consName').value.trim();
        if (!name) { showToast('Consumer name is required', 'error'); return; }
        var backend = document.getElementById('consBackend').value;
        var req = {
            name: name,
            backend: backend,
            consumerGroup: document.getElementById('consGroup').value,
            config: {
                prefetchCount: parseInt(document.getElementById('consPrefetch').value) || 10,
                maxRetries: parseInt(document.getElementById('consMaxRetries').value) || 3,
                dlqEnabled: document.getElementById('consDLQ').checked
            }
        };
        var queueTopic = document.getElementById('consQueue').value;
        if (backend === 'kafka') {
            req.topic = queueTopic;
        } else {
            req.queue = queueTopic;
        }
        req.exchange = document.getElementById('consExchange').value;
        req.routingKey = document.getElementById('consRoutingKey').value;
        apiPost('/consumers', req).then(function() {
            closeModal('createConsumerModal');
            showToast('Consumer created', 'success');
            loadConsumers(); loadStats();
        }).catch(function(err) { showToast('Failed to create consumer: ' + err.message, 'error'); });
    };

    // ---- Messages ----
    window.loadMessages = function() {
        apiGet('/messages?limit=100').then(function(data) {
            var msgs = data.messages || [];
            var tbody = document.getElementById('messagesBody');
            if (!msgs.length) {
                tbody.innerHTML = '<tr><td colspan="6" class="empty-row">No messages yet</td></tr>';
                return;
            }
            tbody.innerHTML = msgs.map(function(m) {
                return '<tr>' +
                    '<td><small>' + esc(m.id || '-') + '</small></td>' +
                    '<td>' + esc(m.producerId || '-') + '</td>' +
                    '<td>' + statusBadge(m.status) + '</td>' +
                    '<td>' + esc(m.contentType || '-') + '</td>' +
                    '<td>' + formatTime(m.timestamp) + '</td>' +
                    '<td>' + jsonPreview(m.body) + '</td>' +
                    '</tr>';
            }).join('');
        }).catch(function(err) { showToast('Failed to load messages: ' + err.message, 'error'); });
    };

    // ---- DLQ ----
    window.loadDLQ = function() {
        apiGet('/dlq').then(function(data) {
            var entries = data.dlq || [];
            var tbody = document.getElementById('dlqBody');
            if (!entries.length) {
                tbody.innerHTML = '<tr><td colspan="7" class="empty-row">No dead-lettered messages</td></tr>';
                return;
            }
            tbody.innerHTML = entries.map(function(e) {
                var replayBtn = e.replayed ? '<span style="color:var(--text-muted)">Replayed</span>' :
                    '<button class="action-btn" onclick="window._conductorReplayDLQ(\'' + esc(e.id) + '\')">Replay</button>';
                return '<tr>' +
                    '<td><small>' + esc(e.id) + '</small></td>' +
                    '<td>' + esc(e.consumerId || '-') + '</td>' +
                    '<td>' + esc(e.originalQueue || '-') + '</td>' +
                    '<td style="color:#ff4d4d">' + esc(e.errorMessage || '-') + '</td>' +
                    '<td>' + (e.retryCount || 0) + '</td>' +
                    '<td>' + formatTime(e.deadLetteredAt) + '</td>' +
                    '<td>' + replayBtn + '</td>' +
                    '</tr>';
            }).join('');
        }).catch(function(err) { showToast('Failed to load DLQ: ' + err.message, 'error'); });
    };

    window._conductorReplayDLQ = function(id) {
        apiPost('/dlq/' + encodeURIComponent(id) + '/replay', {}).then(function() {
            showToast('Message replayed', 'success'); loadDLQ(); loadStats();
        }).catch(function(err) { showToast('Replay failed: ' + err.message, 'error'); });
    };

    // ---- Publish ----
    window.publishMessage = function() {
        var prodId = document.getElementById('publishProducer').value;
        if (!prodId) { showToast('Select a producer first', 'error'); return; }
        var bodyText = document.getElementById('publishBody').value;
        var body;
        try { body = JSON.parse(bodyText); } catch(e) { showToast('Invalid JSON body: ' + e.message, 'error'); return; }
        var req = {
            producerId: prodId,
            body: body,
            routingKey: document.getElementById('publishRoutingKey').value,
            correlationId: document.getElementById('publishCorrelationId').value
        };
        var resultEl = document.getElementById('publishResult');
        apiPost('/publish', req).then(function(data) {
            resultEl.className = 'publish-result success';
            resultEl.textContent = 'Published! Message ID: ' + (data.id || 'unknown');
            showToast('Message published', 'success');
            loadStats();
        }).catch(function(err) {
            resultEl.className = 'publish-result error';
            resultEl.textContent = 'Error: ' + err.message;
            showToast('Publish failed: ' + err.message, 'error');
        });
    };

    // ---- Live Stream ----
    function classifyStreamStatus(status) {
        var s = String(status || '').toLowerCase();
        if (s === 'sent') return 'sent';
        if (s === 'acked' || s === 'delivered') return 'acked';
        if (s === 'failed' || s === 'dlq' || s === 'error') return 'failed';
        return '';
    }

    function summarizeStreamMessages(msgs) {
        var summary = { total: 0, sent: 0, acked: 0, failed: 0 };
        if (!Array.isArray(msgs)) return summary;
        summary.total = msgs.length;
        msgs.forEach(function(m) {
            var bucket = classifyStreamStatus(m && m.status);
            if (bucket === 'sent') summary.sent += 1;
            if (bucket === 'acked') summary.acked += 1;
            if (bucket === 'failed') summary.failed += 1;
        });
        return summary;
    }

    function drawLineSeries(ctx, points, key, color, xFor, yFor) {
        if (!points.length) return;
        ctx.beginPath();
        ctx.lineWidth = key === 'total' ? 2.4 : 1.8;
        ctx.strokeStyle = color;
        points.forEach(function(p, idx) {
            var x = xFor(idx);
            var y = yFor(p[key]);
            if (idx === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        });
        ctx.stroke();

        var last = points[points.length - 1];
        ctx.beginPath();
        ctx.fillStyle = color;
        ctx.arc(xFor(points.length - 1), yFor(last[key]), 2.5, 0, Math.PI * 2);
        ctx.fill();
    }

    function drawStreamGraph() {
        var canvas = document.getElementById('streamGraphCanvas');
        if (!canvas || !canvas.getContext) return;

        var width = Math.max(320, Math.floor((canvas.parentElement && canvas.parentElement.clientWidth) || 680));
        var height = 180;
        var dpr = window.devicePixelRatio || 1;
        canvas.width = Math.floor(width * dpr);
        canvas.height = Math.floor(height * dpr);
        canvas.style.width = width + 'px';
        canvas.style.height = height + 'px';

        var ctx = canvas.getContext('2d');
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        ctx.clearRect(0, 0, width, height);

        var pad = { left: 36, right: 12, top: 12, bottom: 24 };
        var plotW = Math.max(10, width - pad.left - pad.right);
        var plotH = Math.max(10, height - pad.top - pad.bottom);

        ctx.strokeStyle = 'rgba(255,255,255,0.08)';
        ctx.lineWidth = 1;
        for (var i = 0; i <= 4; i++) {
            var gy = pad.top + (plotH * i / 4);
            ctx.beginPath();
            ctx.moveTo(pad.left, gy);
            ctx.lineTo(width - pad.right, gy);
            ctx.stroke();
        }

        if (!streamTrendPoints.length) {
            ctx.fillStyle = 'rgba(138,164,146,0.9)';
            ctx.font = '12px sans-serif';
            ctx.textAlign = 'left';
            ctx.fillText('No stream data yet', pad.left + 8, pad.top + (plotH / 2));
            return;
        }

        var maxVal = 1;
        streamTrendPoints.forEach(function(p) {
            maxVal = Math.max(maxVal, p.total, p.sent, p.acked, p.failed);
        });

        function xFor(idx) {
            if (streamTrendPoints.length === 1) return pad.left + (plotW / 2);
            return pad.left + (plotW * idx / (streamTrendPoints.length - 1));
        }
        function yFor(val) {
            return pad.top + plotH - ((val / maxVal) * plotH);
        }

        drawLineSeries(ctx, streamTrendPoints, 'total', '#14b8a6', xFor, yFor);
        drawLineSeries(ctx, streamTrendPoints, 'sent', '#3b82f6', xFor, yFor);
        drawLineSeries(ctx, streamTrendPoints, 'acked', '#22c55e', xFor, yFor);
        drawLineSeries(ctx, streamTrendPoints, 'failed', '#ef4444', xFor, yFor);

        ctx.fillStyle = 'rgba(136,136,136,0.95)';
        ctx.font = '11px sans-serif';
        ctx.textAlign = 'right';
        ctx.fillText('0', pad.left - 6, pad.top + plotH + 4);
        ctx.fillText(String(maxVal), pad.left - 6, pad.top + 4);

        var first = streamTrendPoints[0];
        var last = streamTrendPoints[streamTrendPoints.length - 1];
        ctx.textAlign = 'left';
        ctx.fillText(first.label, pad.left, pad.top + plotH + 18);
        ctx.textAlign = 'right';
        ctx.fillText(last.label, pad.left + plotW, pad.top + plotH + 18);
    }

    function resetStreamTrendGraph() {
        streamTrendPoints = [];
        var summary = document.getElementById('streamGraphSummary');
        if (summary) summary.textContent = 'Waiting for stream data...';
        drawStreamGraph();
    }

    function updateStreamGraph(msgs) {
        var snapshot = summarizeStreamMessages(msgs);
        var label = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
        streamTrendPoints.push({
            label: label,
            total: snapshot.total,
            sent: snapshot.sent,
            acked: snapshot.acked,
            failed: snapshot.failed,
        });
        if (streamTrendPoints.length > STREAM_TREND_LIMIT) {
            streamTrendPoints = streamTrendPoints.slice(streamTrendPoints.length - STREAM_TREND_LIMIT);
        }

        var summary = document.getElementById('streamGraphSummary');
        if (summary) {
            summary.textContent = 'Buffer: ' + snapshot.total + ' | Sent: ' + snapshot.sent + ' | Acked: ' + snapshot.acked + ' | Failed: ' + snapshot.failed;
        }
        drawStreamGraph();
    }

    window.toggleStream = function() {
        if (streamConnected) {
            disconnectStream();
        } else {
            connectStream();
        }
    };

    function connectStream() {
        var base = window.resolveBackendURL();
        var token = resolveAuthToken();
        wsURL = base.replace(/^http/, 'ws') + '/ws/conductor' + (token ? '?token=' + encodeURIComponent(token) : '');
        try {
            streamWS = new WebSocket(wsURL);
        } catch(e) {
            connectSSE();
            return;
        }

        streamWS.onopen = function() {
            streamConnected = true;
            var indicator = document.getElementById('streamIndicator');
            indicator.textContent = '● Connected';
            indicator.className = 'stream-indicator connected';
            document.getElementById('streamToggle').textContent = 'Disconnect';
            document.getElementById('streamContainer').innerHTML = '';
            resetStreamTrendGraph();
            showToast('Live stream connected', 'success');
        };

        streamWS.onmessage = function(event) {
            try {
                var data = JSON.parse(event.data);
                var msgs = data.messages || [];
                renderStreamMessages(msgs);
                updateStreamGraph(msgs);
                if (data.stats) {
                    document.getElementById('statSent').textContent = data.stats.totalSent || 0;
                    document.getElementById('statReceived').textContent = data.stats.totalReceived || 0;
                    document.getElementById('statAcked').textContent = data.stats.totalAcked || 0;
                    document.getElementById('statFailed').textContent = data.stats.totalFailed || 0;
                }
            } catch(e) {}
        };

        streamWS.onclose = function() { disconnectStream(); };
        streamWS.onerror = function() { disconnectStream(); connectSSE(); };
    }

    function connectSSE() {
        streamConnected = true;
        var indicator = document.getElementById('streamIndicator');
        indicator.textContent = '● Connected (SSE)';
        indicator.className = 'stream-indicator connected';
        document.getElementById('streamToggle').textContent = 'Disconnect';
        document.getElementById('streamContainer').innerHTML = '';
        resetStreamTrendGraph();

        var token = resolveAuthToken();
        var es = new EventSource(API + '/stream' + (token ? '?token=' + encodeURIComponent(token) : ''));
        window._conductorSSE = es;
        es.onmessage = function(event) {
            try {
                var msgs = JSON.parse(event.data);
                renderStreamMessages(msgs);
                updateStreamGraph(msgs);
            } catch(e) {}
        };
        es.onerror = function() { disconnectStream(); showToast('Stream connection lost', 'error'); };
    }

    function disconnectStream() {
        streamConnected = false;
        if (streamWS) { try { streamWS.close(); } catch(e){} streamWS = null; }
        if (window._conductorSSE) { try { window._conductorSSE.close(); } catch(e){} window._conductorSSE = null; }
        var indicator = document.getElementById('streamIndicator');
        indicator.textContent = '● Disconnected';
        indicator.className = 'stream-indicator disconnected';
        document.getElementById('streamToggle').textContent = 'Connect';
    }

    function renderStreamMessages(msgs) {
        var container = document.getElementById('streamContainer');
        container.innerHTML = msgs.map(function(m) {
            var time = formatTime(m.timestamp);
            return '<div class="stream-msg">' +
                '<span class="stream-msg-time">' + time + '</span>' +
                '<span class="stream-msg-status">' + statusBadge(m.status) + '</span>' +
                '<span class="stream-msg-body">' + esc(JSON.stringify(m.body || {})) + '</span>' +
                '</div>';
        }).join('');
        container.scrollTop = container.scrollHeight;
    }

    // ---- Tabs ----
    window.switchTab = function(tab) {
        closeHoverPanel();
        document.querySelectorAll('.tab-content').forEach(function(el) { el.classList.remove('active'); });
        document.querySelectorAll('.tab-btn').forEach(function(el) { el.classList.remove('active'); });
        document.getElementById('tab-' + tab).classList.add('active');
        document.querySelector('.tab-btn[data-tab="' + tab + '"]').classList.add('active');

        if (tab === 'producers') loadProducers();
        if (tab === 'consumers') loadConsumers();
        if (tab === 'connections') loadConnections();
        if (tab === 'messages') window.loadMessages();
        if (tab === 'dlq') window.loadDLQ();
        if (tab === 'stream') setTimeout(drawStreamGraph, 0);
    };

    window.closeModal = function(id) {
        document.getElementById(id).style.display = 'none';
    };

    // Initial load
    loadStats();
    loadConnections();
    loadProducers();
    setTimeout(drawStreamGraph, 0);
    setInterval(loadStats, 5000);
})();
