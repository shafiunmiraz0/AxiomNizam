/* =============================================
   AxiomNizam — Reimagined Landing Page JS
   Particles, parallax, cursor glow, 3D tilt,
   counters, tabs, magnetic hover, spotlight
   ============================================= */
(function() {
    'use strict';

    // ---- Auth Helpers ----
    function readLandingCookie(name) {
        var prefix = name + '=';
        var parts = document.cookie.split(';');
        for (var i = 0; i < parts.length; i++) {
            var c = parts[i].trim();
            if (c.indexOf(prefix) === 0) {
                return decodeURIComponent(c.substring(prefix.length));
            }
        }
        return '';
    }

    function isAuthenticated() {
        return !!(localStorage.getItem('authToken') || readLandingCookie('authToken'));
    }

    window.requireAuth = function(targetPath) {
        if (isAuthenticated()) {
            window.location.href = targetPath;
        } else {
            // Save intended destination so login can redirect back
            localStorage.setItem('returnTo', targetPath);
            window.location.href = '/login';
        }
    };

    // ============================================================
    // COMMAND PALETTE SEARCH
    // ============================================================
    var searchOverlay = document.getElementById('searchOverlay');
    var searchBackdrop = document.getElementById('searchBackdrop');
    var searchModal = document.getElementById('searchModal');
    var searchInput = document.getElementById('searchInput');
    var searchResults = document.getElementById('searchResults');
    var searchEmpty = document.getElementById('searchEmpty');
    var searchTrigger = document.getElementById('searchTrigger');
    var activeIndex = -1;

    // SVG icon helpers for search results
    var svgIcon = function(path, vb) {
        return '<svg viewBox="' + (vb || '0 0 24 24') + '" fill="none" stroke="currentColor" stroke-width="1.5" style="width:18px;height:18px;flex-shrink:0">' + path + '</svg>';
    };
    var ICONS = {
        db: svgIcon('<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/>'),
        api: svgIcon('<path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>'),
        bolt: svgIcon('<polygon points="13 2 3 14 12 14 11 22 21 10 12 10"/>'),
        query: svgIcon('<rect x="4" y="4" width="16" height="16" rx="2"/><path d="M9 9h6"/><path d="M9 13h6"/><path d="M9 17h4"/>'),
        file: svgIcon('<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/>'),
        shuffle: svgIcon('<polyline points="16 3 21 3 21 8"/><line x1="4" y1="20" x2="21" y2="3"/><polyline points="21 16 21 21 16 21"/><line x1="15" y1="15" x2="21" y2="21"/><line x1="4" y1="4" x2="9" y2="9"/>'),
        layers: svgIcon('<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>'),
        wave: svgIcon('<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>'),
        eye: svgIcon('<path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/>'),
        link: svgIcon('<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>'),
        radio: svgIcon('<circle cx="12" cy="12" r="2"/><path d="M16.24 7.76a6 6 0 0 1 0 8.49"/><path d="M7.76 16.24a6 6 0 0 1 0-8.49"/><path d="M19.07 4.93a10 10 0 0 1 0 14.14"/><path d="M4.93 19.07a10 10 0 0 1 0-14.14"/>'),
        search: svgIcon('<circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>'),
        chart: svgIcon('<path d="M18 20V10"/><path d="M12 20V4"/><path d="M6 20v-6"/>'),
        globe: svgIcon('<circle cx="12" cy="12" r="10"/><path d="M2 12h20"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>'),
        zap: svgIcon('<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>'),
        pin: svgIcon('<polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/>'),
        shield: svgIcon('<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><polyline points="9 12 11 14 15 10"/>'),
        lock: svgIcon('<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>'),
        key: svgIcon('<path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/>'),
        building: svgIcon('<path d="M3 21h18"/><path d="M5 21V7l8-4 8 4v14"/><path d="M9 21v-6h6v6"/><path d="M9 9h1"/><path d="M14 9h1"/>'),
        checklist: svgIcon('<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><path d="M9 15l2 2 4-4"/>'),
        server: svgIcon('<rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/>'),
        clock: svgIcon('<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>'),
        users: svgIcon('<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>'),
        send: svgIcon('<path d="M22 2L11 13"/><path d="M22 2l-7 20-4-9-9-4z"/>'),
        shieldAlert: svgIcon('<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="M12 8v4"/><circle cx="12" cy="16" r="0.5" fill="currentColor"/>'),
        gear: svgIcon('<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>'),
        home: svgIcon('<path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/>'),
        userPlus: svgIcon('<path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="8.5" cy="7" r="4"/><line x1="20" y1="8" x2="20" y2="14"/><line x1="23" y1="11" x2="17" y2="11"/>'),
    };

    // Searchable items index
    var searchItems = [
        // Data Platform
        { title: 'API Builder', desc: 'Create REST APIs visually with auto-generated CRUD endpoints', icon: ICONS.api, url: '/admin', group: 'Data Platform', keywords: 'rest crud api mysql postgres mongodb oracle firebase openapi' },
        { title: 'Object Storage', desc: 'S3-compatible storage with buckets, encryption, and policies', icon: ICONS.server, url: '/object-storage', group: 'Data Platform', keywords: 's3 bucket upload download minio storage' },
        { title: 'Data Sources', desc: 'Connect external databases, APIs, and data streams', icon: ICONS.db, url: '/admin', group: 'Data Platform', keywords: 'database connection pool external api' },
        { title: 'Dynamic Queries', desc: 'SQL queries across connected databases with schema introspection', icon: ICONS.bolt, url: '/admin', group: 'Data Platform', keywords: 'sql query database schema batch' },
        { title: 'GraphQL Studio', desc: 'Interactive playground with schema introspection', icon: ICONS.layers, url: '/admin', group: 'Data Platform', keywords: 'graphql query mutation subscription' },
        { title: 'SQL AI Assistant', desc: 'Natural language to SQL with AI-powered suggestions', icon: ICONS.query, url: '/admin', group: 'Data Platform', keywords: 'ai sql assistant natural language nlp' },
        { title: 'CSV Upload & Transform', desc: 'Upload CSV files and auto-generate dashboards and APIs', icon: ICONS.file, url: '/admin', group: 'Data Platform', keywords: 'csv upload import transform data' },
        { title: 'Schema Registry', desc: 'Avro, Protobuf, and JSON schema management', icon: ICONS.server, url: '/admin', group: 'Data Platform', keywords: 'schema avro protobuf json registry compatibility' },
        { title: 'Feature Store', desc: 'Feature group management with online/offline serving', icon: ICONS.layers, url: '/admin', group: 'Data Platform', keywords: 'feature store ml ai machine learning serving' },

        // Real-time
        { title: 'CDC Pipelines', desc: 'Real-time Change Data Capture with streaming ingestion', icon: ICONS.radio, url: '/cdc-etl', group: 'Real-time', keywords: 'cdc change data capture streaming pipeline etl' },
        { title: 'ETL Pipelines', desc: 'Extract-Transform-Load with 20+ connector catalog', icon: ICONS.shuffle, url: '/cdc-etl', group: 'Real-time', keywords: 'etl extract transform load connector pipeline' },
        { title: 'Message Conductor', desc: 'RabbitMQ and Kafka broker management', icon: ICONS.wave, url: '/conductor', group: 'Real-time', keywords: 'kafka rabbitmq message broker producer consumer' },
        { title: 'WebSocket Streaming', desc: 'Real-time data streaming via WebSockets and SSE', icon: ICONS.eye, url: '/admin', group: 'Real-time', keywords: 'websocket sse streaming real-time event' },
        { title: 'Webhooks', desc: 'Event-driven integrations with delivery logging', icon: ICONS.link, url: '/admin', group: 'Real-time', keywords: 'webhook event callback integration' },
        { title: 'Event Bus', desc: 'Publish/subscribe event streaming with DLQ support', icon: ICONS.radio, url: '/admin', group: 'Real-time', keywords: 'event bus pubsub publish subscribe dlq' },
        { title: 'Distributed Tracing', desc: 'Trace ingestion, span correlation, service map', icon: ICONS.search, url: '/admin', group: 'Real-time', keywords: 'tracing trace span opentelemetry jaeger' },
        { title: 'OpenTelemetry', desc: 'OTLP trace propagation with structured access logging', icon: ICONS.layers, url: '/admin', group: 'Real-time', keywords: 'opentelemetry otlp observability' },

        // Analytics & Visualization
        { title: 'Analytics Dashboards', desc: 'Custom dashboards with charts, heatmaps, KPI widgets', icon: ICONS.chart, url: '/analytics', group: 'Analytics', keywords: 'analytics dashboard chart heatmap kpi widget' },
        { title: 'GIS Intelligence', desc: 'Multi-layer interactive maps with regions and markers', icon: ICONS.globe, url: '/gis', group: 'Analytics', keywords: 'gis map geography geo spatial geo layers' },
        { title: 'Network Intelligence', desc: 'Log parsing, topology mapping, anomaly detection', icon: ICONS.search, url: '/netintel', group: 'Analytics', keywords: 'network netintel topology heatmap anomaly mac' },
        { title: 'Data Lineage', desc: 'Column-level tracking with impact analysis', icon: ICONS.zap, url: '/admin', group: 'Analytics', keywords: 'lineage column tracking upstream downstream impact' },
        { title: 'Version Lineage', desc: 'Resource version tracking with diff visualization', icon: ICONS.pin, url: '/admin', group: 'Analytics', keywords: 'version lineage diff history rollback' },

        // Security & IAM
        { title: 'IAM & RBAC', desc: 'Keycloak integration with multi-realm and OAuth2/OIDC', icon: ICONS.lock, url: '/iam-admin', group: 'Security', keywords: 'iam rbac keycloak realm user role permission oauth oidc saml' },
        { title: '2FA / MFA (Gatekeeper)', desc: 'TOTP enrollment, backup codes, trusted devices', icon: ICONS.shield, url: '/two-factor', group: 'Security', keywords: '2fa mfa totp gatekeeper backup trusted device authenticator' },
        { title: 'CSRF Protection', desc: 'Double-submit cookie CSRF with token rotation', icon: ICONS.shieldAlert, url: '/admin', group: 'Security', keywords: 'csrf protection token cookie security' },
        { title: 'Security Headers', desc: 'HSTS, CSP, X-Frame-Options on every response', icon: ICONS.lock, url: '/admin', group: 'Security', keywords: 'security headers hsts csp x-frame' },
        { title: 'Encryption & Certs', desc: 'AES encryption, key management, certificate lifecycle', icon: ICONS.key, url: '/admin', group: 'Security', keywords: 'encryption aes key certificate tls ssl' },
        { title: 'Governance Console', desc: 'Policy engine with access, admission, compliance policies', icon: ICONS.building, url: '/governance', group: 'Security', keywords: 'governance policy compliance admission quota lifecycle' },
        { title: 'Audit & Compliance', desc: 'Complete audit trail with SOC2, HIPAA, PCI-DSS scoring', icon: ICONS.checklist, url: '/admin', group: 'Security', keywords: 'audit compliance soc2 hipaa pci risk assessment' },

        // Infrastructure
        { title: 'Operations Center', desc: 'Incident management, alerting, and job scheduling', icon: ICONS.server, url: '/admin', group: 'Infrastructure', keywords: 'operations center incident alert job schedule' },
        { title: 'Job Scheduler', desc: 'Cron-style scheduling with execution history', icon: ICONS.clock, url: '/admin', group: 'Infrastructure', keywords: 'job scheduler cron task execution' },
        { title: 'Multi-Tenancy', desc: 'Tenant CRUD, member management, quota management', icon: ICONS.users, url: '/admin', group: 'Infrastructure', keywords: 'tenant multi-tenancy isolation quota member' },
        { title: 'Deployment Controller', desc: 'Blue-green and canary deployments with rollback', icon: ICONS.send, url: '/admin', group: 'Infrastructure', keywords: 'deployment blue-green canary rollback health' },
        { title: 'Trivy Scanner', desc: 'Vulnerability scanning for containers and repos', icon: ICONS.shieldAlert, url: '/admin', group: 'Infrastructure', keywords: 'trivy vulnerability scan container image security' },
        { title: 'Autopilot', desc: 'Automated cluster health with dead server cleanup', icon: ICONS.gear, url: '/admin', group: 'Infrastructure', keywords: 'autopilot cluster health quorum cleanup' },
        { title: 'SafeGate Scanner', desc: 'File scanning pipeline with MIME, SVG, macro detection', icon: ICONS.shield, url: '/object-storage', group: 'Infrastructure', keywords: 'safegate scanner mime svg macro antivirus archive' },

        // Advanced
        { title: 'Vector Search', desc: 'Semantic search with vector embeddings and KNN', icon: ICONS.search, url: '/admin', group: 'Advanced', keywords: 'vector search embedding knn semantic ai' },
        { title: 'K8s Extensions', desc: 'Admission webhooks, CRD registry, scheduler', icon: ICONS.server, url: '/admin', group: 'Advanced', keywords: 'kubernetes k8s admission webhook crd scheduler' },
        { title: 'ML Pipeline', desc: 'Model deployment, feature extraction, training', icon: ICONS.gear, url: '/admin', group: 'Advanced', keywords: 'ml machine learning pipeline model training deploy' },
        { title: 'Prometheus Metrics', desc: 'Per-module metric collectors with /metrics endpoint', icon: ICONS.chart, url: '/admin', group: 'Advanced', keywords: 'prometheus metrics observability monitor' },

        // Pages
        { title: 'Admin Dashboard', desc: 'Full platform administration and management', icon: ICONS.home, url: '/admin', group: 'Pages', keywords: 'admin dashboard management' },
        { title: 'Sign Up', desc: 'Create a free account', icon: ICONS.userPlus, url: '/signup', group: 'Pages', keywords: 'signup register account create' },
        { title: 'Sign In', desc: 'Sign in to your account', icon: ICONS.lock, url: '/login', group: 'Pages', keywords: 'login signin sign-in auth' },
    ];

    // Fuzzy match
    function fuzzyMatch(text, query) {
        var ti = 0, qi = 0;
        var score = 0;
        var matchPositions = [];
        text = text.toLowerCase();
        query = query.toLowerCase();

        while (ti < text.length && qi < query.length) {
            if (text[ti] === query[qi]) {
                score += (ti === 0 || text[ti - 1] === ' ' || text[ti - 1] === '-') ? 3 : 1;
                matchPositions.push(ti);
                qi++;
            }
            ti++;
        }
        return qi === query.length ? { score: score, positions: matchPositions } : null;
    }

    function highlightMatch(text, positions) {
        if (!positions || positions.length === 0) return escapeHTML(text);
        var result = '';
        var posSet = {};
        for (var i = 0; i < positions.length; i++) posSet[positions[i]] = true;
        var inMark = false;
        for (var j = 0; j < text.length; j++) {
            if (posSet[j] && !inMark) { result += '<mark>'; inMark = true; }
            else if (!posSet[j] && inMark) { result += '</mark>'; inMark = false; }
            result += escapeHTML(text[j]);
        }
        if (inMark) result += '</mark>';
        return result;
    }

    function escapeHTML(str) {
        return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    }

    function doSearch(query) {
        if (!query || query.length < 1) return [];

        var scored = [];
        for (var i = 0; i < searchItems.length; i++) {
            var item = searchItems[i];
            var titleMatch = fuzzyMatch(item.title, query);
            var kwMatch = fuzzyMatch(item.keywords, query);
            var descMatch = fuzzyMatch(item.desc, query);

            var bestScore = 0;
            var bestPositions = null;
            var matchField = '';

            if (titleMatch && titleMatch.score * 2 > bestScore) {
                bestScore = titleMatch.score * 2;
                bestPositions = titleMatch.positions;
                matchField = 'title';
            }
            if (kwMatch && kwMatch.score > bestScore) {
                bestScore = kwMatch.score;
                bestPositions = kwMatch.positions;
                matchField = 'keywords';
            }
            if (descMatch && descMatch.score * 0.5 > bestScore) {
                bestScore = descMatch.score * 0.5;
                bestPositions = descMatch.positions;
                matchField = 'desc';
            }

            if (bestScore > 0) {
                scored.push({
                    item: item,
                    score: bestScore,
                    positions: matchField === 'title' ? bestPositions : null
                });
            }
        }

        scored.sort(function(a, b) { return b.score - a.score; });
        return scored.slice(0, 12);
    }

    function renderResults(query) {
        var results = doSearch(query);

        if (!query || query.length < 1) {
            searchResults.innerHTML = '';
            searchResults.appendChild(searchEmpty);
            searchEmpty.classList.remove('hidden');
            activeIndex = -1;
            return;
        }

        searchEmpty.classList.add('hidden');

        if (results.length === 0) {
            searchResults.innerHTML = '<div class="search-no-results"><span>No results found</span>Try a different search term</div>';
            activeIndex = -1;
            return;
        }

        // Group by category
        var groups = {};
        var groupOrder = [];
        for (var i = 0; i < results.length; i++) {
            var groupName = results[i].item.group;
            if (!groups[groupName]) {
                groups[groupName] = [];
                groupOrder.push(groupName);
            }
            groups[groupName].push(results[i]);
        }

        var html = '';
        var idx = 0;
        for (var g = 0; g < groupOrder.length; g++) {
            var gName = groupOrder[g];
            html += '<div class="search-group"><div class="search-group__label">' + escapeHTML(gName) + '</div>';
            for (var r = 0; r < groups[gName].length; r++) {
                var result = groups[gName][r];
                var titleHTML = result.positions ? highlightMatch(result.item.title, result.positions) : escapeHTML(result.item.title);
                html += '<a class="search-result' + (idx === 0 ? ' active' : '') + '" href="' + result.item.url + '" data-index="' + idx + '" style="animation-delay:' + (idx * 40) + 'ms">'
                    + '<div class="search-result__icon">' + result.item.icon + '</div>'
                    + '<div class="search-result__text">'
                    + '<div class="search-result__title">' + titleHTML + '</div>'
                    + '<div class="search-result__desc">' + escapeHTML(result.item.desc) + '</div>'
                    + '</div>'
                    + '<div class="search-result__arrow"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg></div>'
                    + '</a>';
                idx++;
            }
            html += '</div>';
        }

        searchResults.innerHTML = html;
        activeIndex = 0;
    }

    function setActiveResult(index) {
        var items = searchResults.querySelectorAll('.search-result');
        if (items.length === 0) return;
        if (index < 0) index = items.length - 1;
        if (index >= items.length) index = 0;
        for (var i = 0; i < items.length; i++) {
            items[i].classList.toggle('active', i === index);
        }
        activeIndex = index;
        // Scroll into view
        var activeEl = searchResults.querySelector('.search-result.active');
        if (activeEl) {
            activeEl.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
        }
    }

    function openSearch() {
        searchOverlay.classList.add('open');
        document.body.style.overflow = 'hidden';
        setTimeout(function() { searchInput.focus(); }, 50);
    }

    function closeSearch() {
        searchOverlay.classList.remove('open');
        document.body.style.overflow = '';
        searchInput.value = '';
        renderResults('');
        activeIndex = -1;
    }

    // Event: trigger button
    if (searchTrigger) {
        searchTrigger.addEventListener('click', openSearch);
    }

    // Event: backdrop click
    if (searchBackdrop) {
        searchBackdrop.addEventListener('click', closeSearch);
    }

    // Event: input
    if (searchInput) {
        searchInput.addEventListener('input', function() {
            renderResults(searchInput.value.trim());
        });
    }

    // Event: keyboard
    document.addEventListener('keydown', function(e) {
        // Open: / or Cmd+K / Ctrl+K
        if ((e.key === '/' || (e.key === 'k' && (e.metaKey || e.ctrlKey))) && !searchOverlay.classList.contains('open')) {
            var tag = (document.activeElement || {}).tagName;
            if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
            e.preventDefault();
            openSearch();
            return;
        }

        if (!searchOverlay.classList.contains('open')) return;

        // Close: Escape
        if (e.key === 'Escape') {
            e.preventDefault();
            closeSearch();
            return;
        }

        // Navigate: Arrow keys
        if (e.key === 'ArrowDown') {
            e.preventDefault();
            setActiveResult(activeIndex + 1);
            return;
        }
        if (e.key === 'ArrowUp') {
            e.preventDefault();
            setActiveResult(activeIndex - 1);
            return;
        }

        // Select: Enter
        if (e.key === 'Enter') {
            e.preventDefault();
            var active = searchResults.querySelector('.search-result.active');
            if (active) {
                var href = active.getAttribute('href');
                closeSearch();
                requireAuth(href);
            }
            return;
        }
    });

    // Event: result click (use requireAuth for proper redirect)
    if (searchResults) {
        searchResults.addEventListener('click', function(e) {
            var result = e.target.closest('.search-result');
            if (result) {
                e.preventDefault();
                var href = result.getAttribute('href');
                closeSearch();
                requireAuth(href);
            }
        });

        // Event: result hover
        searchResults.addEventListener('mouseover', function(e) {
            var result = e.target.closest('.search-result');
            if (result) {
                var idx = parseInt(result.getAttribute('data-index'), 10);
                if (!isNaN(idx)) setActiveResult(idx);
            }
        });
    }

    // ============================================================
    // DATA FLOW CANVAS ANIMATION — network graph
    // ============================================================
    var dfCanvas = document.getElementById('dataflowCanvas');
    if (dfCanvas) {
        var dfCtx = dfCanvas.getContext('2d');
        var dfPackets = [];
        var dfSpawnTimer = 0;

        function resizeDfCanvas() {
            dfCanvas.width = dfCanvas.offsetWidth;
            dfCanvas.height = dfCanvas.offsetHeight;
        }
        resizeDfCanvas();
        window.addEventListener('resize', resizeDfCanvas);

        function drawDf() {
            dfCtx.clearRect(0, 0, dfCanvas.width, dfCanvas.height);
            var w = dfCanvas.width, h = dfCanvas.height;
            var time = Date.now() * 0.001;

            // Pipeline nodes (match HTML positions)
            var dfNodes = [
                { x: w * 0.10, y: h * 0.22, label: 'PG', color: '#34d399' },
                { x: w * 0.26, y: h * 0.62, label: 'CDC', color: '#38bdf8' },
                { x: w * 0.44, y: h * 0.25, label: 'ETL', color: '#a78bfa' },
                { x: w * 0.62, y: h * 0.65, label: 'KAFKA', color: '#fbbf24' },
                { x: w * 0.79, y: h * 0.27, label: 'S3', color: '#34d399' },
                { x: w * 0.94, y: h * 0.63, label: 'API', color: '#38bdf8' },
            ];

            // Draw connections between consecutive nodes
            for (var n = 0; n < dfNodes.length - 1; n++) {
                var from = dfNodes[n];
                var to = dfNodes[n + 1];
                var cpx = (from.x + to.x) / 2 + Math.sin(n * 1.3) * 25;
                var cpy = (from.y + to.y) / 2 + Math.cos(n * 0.9) * 20;

                // Connection line (request path)
                dfCtx.beginPath();
                dfCtx.moveTo(from.x, from.y);
                dfCtx.quadraticCurveTo(cpx, cpy - 8, to.x, to.y);
                dfCtx.strokeStyle = 'rgba(52, 211, 153, 0.1)';
                dfCtx.lineWidth = 1.5;
                dfCtx.stroke();

                // Connection line (response path)
                dfCtx.beginPath();
                dfCtx.moveTo(to.x, to.y);
                dfCtx.quadraticCurveTo(cpx, cpy + 8, from.x, from.y);
                dfCtx.strokeStyle = 'rgba(56, 189, 248, 0.06)';
                dfCtx.lineWidth = 1;
                dfCtx.stroke();

                // Animated pulse along request path
                var pulseT = ((time * 0.3 + n * 0.18) % 1);
                var px = (1 - pulseT) * (1 - pulseT) * from.x + 2 * (1 - pulseT) * pulseT * cpx + pulseT * pulseT * to.x;
                var py = (1 - pulseT) * (1 - pulseT) * from.y + 2 * (1 - pulseT) * pulseT * (cpy - 8) + pulseT * pulseT * to.y;

                // Pulse glow
                dfCtx.beginPath();
                dfCtx.arc(px, py, 10, 0, Math.PI * 2);
                dfCtx.fillStyle = from.color + '10';
                dfCtx.fill();

                // Pulse dot
                dfCtx.beginPath();
                dfCtx.arc(px, py, 3, 0, Math.PI * 2);
                dfCtx.fillStyle = from.color + 'cc';
                dfCtx.fill();

                // Response dot going back
                var respT = ((time * 0.22 + n * 0.15 + 0.5) % 1);
                var rx = (1 - respT) * (1 - respT) * to.x + 2 * (1 - respT) * respT * cpx + respT * respT * from.x;
                var ry = (1 - respT) * (1 - respT) * to.y + 2 * (1 - respT) * respT * (cpy + 8) + respT * respT * from.y;

                dfCtx.beginPath();
                dfCtx.arc(rx, ry, 6, 0, Math.PI * 2);
                dfCtx.fillStyle = '#38bdf808';
                dfCtx.fill();

                dfCtx.beginPath();
                dfCtx.arc(rx, ry, 2, 0, Math.PI * 2);
                dfCtx.fillStyle = '#38bdf8aa';
                dfCtx.fill();
            }

            // Draw nodes
            dfNodes.forEach(function(node, i) {
                var pulse = 0.35 + Math.sin(time * 2.5 + i * 1.2) * 0.15;

                // Outer glow ring
                dfCtx.beginPath();
                dfCtx.arc(node.x, node.y, 20, 0, Math.PI * 2);
                dfCtx.fillStyle = node.color + '06';
                dfCtx.fill();

                // Middle ring
                dfCtx.beginPath();
                dfCtx.arc(node.x, node.y, 15, 0, Math.PI * 2);
                dfCtx.fillStyle = node.color + '0a';
                dfCtx.fill();

                // Node circle
                dfCtx.beginPath();
                dfCtx.arc(node.x, node.y, 11, 0, Math.PI * 2);
                dfCtx.fillStyle = 'rgba(15, 23, 42, 0.9)';
                dfCtx.fill();
                dfCtx.strokeStyle = node.color + Math.floor(pulse * 255).toString(16).padStart(2, '0');
                dfCtx.lineWidth = 1.5;
                dfCtx.stroke();

                // Label
                dfCtx.fillStyle = node.color;
                dfCtx.font = 'bold 10px monospace';
                dfCtx.textAlign = 'center';
                dfCtx.textBaseline = 'middle';
                dfCtx.fillText(node.label, node.x, node.y);
            });

            // Auto-spawn data packets (forward direction)
            dfSpawnTimer += 0.016;
            if (dfSpawnTimer > 0.25) {
                dfSpawnTimer = 0;
                var srcIdx = Math.floor(Math.random() * (dfNodes.length - 1));
                var colors = ['#34d399', '#38bdf8', '#a78bfa', '#fbbf24'];
                dfPackets.push({
                    x: dfNodes[srcIdx].x, y: dfNodes[srcIdx].y,
                    tx: dfNodes[srcIdx + 1].x, ty: dfNodes[srcIdx + 1].y,
                    cx: (dfNodes[srcIdx].x + dfNodes[srcIdx + 1].x) / 2 + (Math.random() - 0.5) * 30,
                    cy: (dfNodes[srcIdx].y + dfNodes[srcIdx + 1].y) / 2 + (Math.random() - 0.5) * 25 - 8,
                    t: 0, speed: 0.008 + Math.random() * 0.006,
                    color: dfNodes[srcIdx].color, size: Math.random() * 2 + 1.5,
                    reverse: false
                });

                // Also spawn some reverse packets (response)
                if (Math.random() < 0.4) {
                    dfPackets.push({
                        x: dfNodes[srcIdx + 1].x, y: dfNodes[srcIdx + 1].y,
                        tx: dfNodes[srcIdx].x, ty: dfNodes[srcIdx].y,
                        cx: (dfNodes[srcIdx].x + dfNodes[srcIdx + 1].x) / 2 + (Math.random() - 0.5) * 30,
                        cy: (dfNodes[srcIdx].y + dfNodes[srcIdx + 1].y) / 2 + (Math.random() - 0.5) * 25 + 8,
                        t: 0, speed: 0.006 + Math.random() * 0.005,
                        color: '#38bdf8', size: Math.random() * 1.5 + 1,
                        reverse: true
                    });
                }
            }

            // Update and draw packets
            for (var p = dfPackets.length - 1; p >= 0; p--) {
                var pkt = dfPackets[p];
                pkt.t += pkt.speed;
                if (pkt.t >= 1) { dfPackets.splice(p, 1); continue; }
                var tt = pkt.t;
                var ppx = (1 - tt) * (1 - tt) * pkt.x + 2 * (1 - tt) * tt * pkt.cx + tt * tt * pkt.tx;
                var ppy = (1 - tt) * (1 - tt) * pkt.y + 2 * (1 - tt) * tt * pkt.cy + tt * tt * pkt.ty;

                // Trail effect
                for (var trail = 1; trail <= 3; trail++) {
                    var trailT = Math.max(0, tt - trail * 0.06);
                    var trailPx = (1 - trailT) * (1 - trailT) * pkt.x + 2 * (1 - trailT) * trailT * pkt.cx + trailT * trailT * pkt.tx;
                    var trailPy = (1 - trailT) * (1 - trailT) * pkt.y + 2 * (1 - trailT) * trailT * pkt.cy + trailT * trailT * pkt.ty;
                    dfCtx.beginPath();
                    dfCtx.arc(trailPx, trailPy, pkt.size * (1 - trail * 0.2), 0, Math.PI * 2);
                    dfCtx.fillStyle = pkt.color + (30 - trail * 8).toString(16).padStart(2, '0');
                    dfCtx.fill();
                }

                // Glow
                dfCtx.beginPath();
                dfCtx.arc(ppx, ppy, pkt.size + 5, 0, Math.PI * 2);
                dfCtx.fillStyle = pkt.color + '15';
                dfCtx.fill();

                // Packet
                dfCtx.beginPath();
                dfCtx.arc(ppx, ppy, pkt.size, 0, Math.PI * 2);
                dfCtx.fillStyle = pkt.color + 'dd';
                dfCtx.fill();
            }

            // Stats in corner
            dfCtx.textAlign = 'right';
            dfCtx.textBaseline = 'top';
            dfCtx.fillStyle = '#64748b';
            dfCtx.font = 'bold 10px monospace';
            dfCtx.fillText('6 stages', w - 10, 12);
            dfCtx.fillText('34K evt/min', w - 10, 26);
            dfCtx.fillStyle = '#34d399';
            dfCtx.fillText('LIVE', w - 10, 40);

            requestAnimationFrame(drawDf);
        }

        // Start when visible
        var dfObserver = new IntersectionObserver(function(entries) {
            if (entries[0].isIntersecting) {
                drawDf();
                dfObserver.unobserve(dfCanvas);
            }
        }, { threshold: 0.2 });
        dfObserver.observe(dfCanvas);
    }

    // ============================================================
    // API LIFECYCLE ANIMATION
    // ============================================================
    var apiStage = document.getElementById('apiLifecycleStage');
    var apiCanvas = document.getElementById('apiCanvas');
    var apiCtx = apiCanvas ? apiCanvas.getContext('2d') : null;
    var liveReqCode = document.getElementById('liveReqCode');
    var liveResCode = document.getElementById('liveResCode');
    var liveTiming = document.getElementById('liveTiming');

    var apiEndpoints = [
        { method: 'GET', url: '/api/v1/users', status: 200, latency: 23, resBody: '{\n  "data": [\n    { "id": 1, "name": "John Doe" },\n    { "id": 2, "name": "Jane Smith" }\n  ],\n  "total": 2\n}' },
        { method: 'POST', url: '/api/v1/orders', status: 201, latency: 45, resBody: '{\n  "id": "ord_9f3a2b",\n  "status": "created",\n  "total": 149.99\n}' },
        { method: 'GET', url: '/api/v1/products?page=1', status: 200, latency: 18, resBody: '{\n  "data": [\n    { "id": 1, "name": "Widget A", "price": 29.99 },\n    { "id": 2, "name": "Widget B", "price": 49.99 }\n  ],\n  "page": 1\n}' },
        { method: 'PUT', url: '/api/v1/users/1', status: 200, latency: 31, resBody: '{\n  "id": 1,\n  "name": "John Updated",\n  "updated_at": "2026-05-28T10:30:00Z"\n}' },
        { method: 'DELETE', url: '/api/v1/sessions/abc', status: 204, latency: 12, resBody: '' },
        { method: 'GET', url: '/api/v1/analytics/summary', status: 200, latency: 67, resBody: '{\n  "requests_today": 12847,\n  "avg_latency_ms": 23,\n  "error_rate": 0.02\n}' },
    ];

    var apiNodeIds = ['nodeClient', 'nodeGateway', 'nodeBuilder', 'nodeDB', 'nodeResponse'];
    var apiCycleIdx = 0;
    var apiCycleTimer = null;
    var apiPackets = [];
    var apiAnimId = null;

    // Resize canvas
    function resizeApiCanvas() {
        if (!apiCanvas) return;
        apiCanvas.width = apiCanvas.offsetWidth;
        apiCanvas.height = apiCanvas.offsetHeight;
    }
    resizeApiCanvas();
    window.addEventListener('resize', resizeApiCanvas);

    // Node positions along the pipeline (normalized 0-1)
    var nodePositions = [
        { x: 0.05, y: 0.5 },  // Client
        { x: 0.25, y: 0.5 },  // Gateway
        { x: 0.5, y: 0.5 },   // Builder
        { x: 0.75, y: 0.5 },  // Database
        { x: 0.95, y: 0.5 },  // Response
    ];

    // Spawn a packet along the curve
    function spawnApiPacket(reverse) {
        apiPackets.push({
            t: 0,
            speed: 0.006 + Math.random() * 0.006,
            reverse: reverse,
            color: reverse ? '#38bdf8' : '#34d399',
            glowColor: reverse ? 'rgba(56, 189, 248, 0.5)' : 'rgba(52, 211, 153, 0.5)',
            size: 2.5 + Math.random() * 2,
        });
    }

    // Auto-spawn packets for continuous flow
    var apiSpawnTimer = 0;
    var apiResponseSpawnTimer = 0;

    // Draw the API lifecycle canvas
    function drawApiCanvas() {
        if (!apiCtx || !apiCanvas) return;
        apiCtx.clearRect(0, 0, apiCanvas.width, apiCanvas.height);
        var w = apiCanvas.width, h = apiCanvas.height;
        var time = Date.now() * 0.001;

        // Auto-spawn request packets (green, left to right)
        apiSpawnTimer += 0.016;
        if (apiSpawnTimer > 0.6) {
            apiSpawnTimer = 0;
            spawnApiPacket(false);
        }

        // Auto-spawn response packets (blue, right to left)
        apiResponseSpawnTimer += 0.016;
        if (apiResponseSpawnTimer > 0.9) {
            apiResponseSpawnTimer = 0;
            spawnApiPacket(true);
        }

        // Draw connections between nodes
        for (var i = 0; i < nodePositions.length - 1; i++) {
            var from = nodePositions[i];
            var to = nodePositions[i + 1];

            // Request path (top curve, green)
            var cpx = (from.x + to.x) / 2 * w;
            var cpyTop = (from.y + to.y) / 2 * h - 20 + Math.sin(time + i) * 3;

            apiCtx.beginPath();
            apiCtx.moveTo(from.x * w, from.y * h);
            apiCtx.quadraticCurveTo(cpx, cpyTop, to.x * w, to.y * h);
            apiCtx.strokeStyle = 'rgba(52, 211, 153, 0.1)';
            apiCtx.lineWidth = 1.5;
            apiCtx.stroke();

            // Response path (bottom curve, blue)
            var cpyBottom = (from.y + to.y) / 2 * h + 20 + Math.cos(time + i) * 3;
            apiCtx.beginPath();
            apiCtx.moveTo(to.x * w, to.y * h);
            apiCtx.quadraticCurveTo(cpx, cpyBottom, from.x * w, from.y * h);
            apiCtx.strokeStyle = 'rgba(56, 189, 248, 0.08)';
            apiCtx.lineWidth = 1.5;
            apiCtx.stroke();
        }

        // Draw node circles
        nodePositions.forEach(function(node, idx) {
            var pulse = Math.sin(time * 2 + idx) * 0.3 + 0.7;

            // Glow
            apiCtx.beginPath();
            apiCtx.arc(node.x * w, node.y * h, 12 + pulse * 4, 0, Math.PI * 2);
            apiCtx.fillStyle = 'rgba(52, 211, 153, 0.05)';
            apiCtx.fill();

            // Node
            apiCtx.beginPath();
            apiCtx.arc(node.x * w, node.y * h, 6, 0, Math.PI * 2);
            apiCtx.fillStyle = 'rgba(52, 211, 153, 0.15)';
            apiCtx.fill();
            apiCtx.strokeStyle = 'rgba(52, 211, 153, 0.3)';
            apiCtx.lineWidth = 1.5;
            apiCtx.stroke();
        });

        // Update and draw packets
        for (var p = apiPackets.length - 1; p >= 0; p--) {
            var pkt = apiPackets[p];
            pkt.t += pkt.speed;

            if (pkt.t >= 1) {
                apiPackets.splice(p, 1);
                continue;
            }

            var progress = pkt.reverse ? (1 - pkt.t) : pkt.t;
            var segCount = nodePositions.length - 1;
            var segProgress = progress * segCount;
            var segIdx = Math.min(Math.floor(segProgress), segCount - 1);
            var segT = segProgress - segIdx;

            var from = nodePositions[pkt.reverse ? segIdx + 1 : segIdx];
            var to = nodePositions[pkt.reverse ? segIdx : segIdx + 1];

            // Quadratic bezier interpolation
            var cpx = (from.x + to.x) / 2 * w;
            var cpyOffset = pkt.reverse ? 20 : -20;
            var cpy = (from.y + to.y) / 2 * h + cpyOffset + Math.sin(time + segIdx) * 3;

            var px = (1 - segT) * (1 - segT) * from.x * w + 2 * (1 - segT) * segT * cpx + segT * segT * to.x * w;
            var py = (1 - segT) * (1 - segT) * from.y * h + 2 * (1 - segT) * segT * cpy + segT * segT * to.y * h;

            // Glow
            apiCtx.beginPath();
            apiCtx.arc(px, py, pkt.size + 4, 0, Math.PI * 2);
            apiCtx.fillStyle = pkt.glowColor;
            apiCtx.fill();

            // Packet
            apiCtx.beginPath();
            apiCtx.arc(px, py, pkt.size, 0, Math.PI * 2);
            apiCtx.fillStyle = pkt.color;
            apiCtx.fill();

            // Trail
            for (var trail = 1; trail <= 3; trail++) {
                var trailT = Math.max(0, segT - trail * 0.08);
                var trailPx = (1 - trailT) * (1 - trailT) * from.x * w + 2 * (1 - trailT) * trailT * cpx + trailT * trailT * to.x * w;
                var trailPy = (1 - trailT) * (1 - trailT) * from.y * h + 2 * (1 - trailT) * trailT * cpy + trailT * trailT * to.y * h;
                apiCtx.beginPath();
                apiCtx.arc(trailPx, trailPy, pkt.size * (1 - trail * 0.2), 0, Math.PI * 2);
                apiCtx.fillStyle = pkt.color + (40 - trail * 10).toString(16).padStart(2, '0');
                apiCtx.fill();
            }
        }

        // Ambient floating particles
        for (var i = 0; i < 8; i++) {
            var ax = (Math.sin(time * 0.5 + i * 1.5) * 0.4 + 0.5) * w;
            var ay = (Math.cos(time * 0.3 + i * 2) * 0.3 + 0.5) * h;
            var aSize = Math.sin(time + i) * 0.5 + 1;
            apiCtx.beginPath();
            apiCtx.arc(ax, ay, aSize, 0, Math.PI * 2);
            apiCtx.fillStyle = 'rgba(148, 163, 184, 0.08)';
            apiCtx.fill();
        }

        apiAnimId = requestAnimationFrame(drawApiCanvas);
    }

    function activateNode(idx) {
        var nodes = apiStage ? apiStage.querySelectorAll('.api-lifecycle__node') : [];
        nodes.forEach(function(n, i) {
            n.classList.toggle('active', i <= idx);
        });
    }

    function runApiCycle() {
        var ep = apiEndpoints[apiCycleIdx % apiEndpoints.length];
        apiCycleIdx++;

        // Reset nodes
        activateNode(-1);
        if (liveReqCode) liveReqCode.innerHTML = '<span class="cursor-blink">|</span>';
        if (liveResCode) liveResCode.innerHTML = '';
        if (liveTiming) liveTiming.textContent = '';

        // Animate request flowing through nodes with packets
        var steps = [
            { delay: 200, node: 0, text: '<span class="req-method">' + ep.method + '</span> <span class="req-url">' + ep.url + '</span>\n<span class="req-key">Authorization:</span> <span class="req-val">Bearer eyJhbG...</span>\n<span class="req-key">Content-Type:</span> <span class="req-val">application/json</span>' },
            { delay: 600, node: 1, text: null },
            { delay: 1000, node: 2, text: null },
            { delay: 1400, node: 3, text: null },
        ];

        steps.forEach(function(step) {
            setTimeout(function() {
                activateNode(step.node);
                spawnApiPacket(false);
                if (step.text && liveReqCode) {
                    liveReqCode.innerHTML = step.text;
                }
            }, step.delay);
        });

        // Response flows back
        setTimeout(function() {
            activateNode(4);
            spawnApiPacket(true);
            if (liveResCode) {
                if (ep.status === 204) {
                    liveResCode.innerHTML = '<span class="res-status">' + ep.status + '</span> No Content';
                } else {
                    var highlighted = ep.resBody
                        .replace(/"([^"]+)":/g, '<span class="res-key">"$1"</span>:')
                        .replace(/: "([^"]+)"/g, ': <span class="res-str">"$1"</span>')
                        .replace(/: (\d+\.?\d*)/g, ': <span class="res-num">$1</span>');
                    liveResCode.innerHTML = '<span class="res-status">' + ep.status + ' ' + (ep.status === 200 ? 'OK' : ep.status === 201 ? 'Created' : 'OK') + '</span>\n' + highlighted;
                }
            }
            if (liveTiming) {
                liveTiming.textContent = ep.latency + 'ms';
            }
        }, 1800);
    }

    if (apiStage) {
        var apiObserver = new IntersectionObserver(function(entries) {
            if (entries[0].isIntersecting) {
                drawApiCanvas();
                runApiCycle();
                apiCycleTimer = setInterval(runApiCycle, 4000);
                apiObserver.unobserve(apiStage);
            } else {
                clearInterval(apiCycleTimer);
                if (apiAnimId) cancelAnimationFrame(apiAnimId);
            }
        }, { threshold: 0.3 });
        apiObserver.observe(apiStage);
    }

    // ---- Scroll-Linked Timeline for API Lifecycle ----
    var apiScrollBar = document.getElementById('apiScrollBar');
    var apiScrollStages = document.querySelectorAll('.api-lifecycle__scroll-stage');
    var apiLifecycleSection = document.getElementById('apiLifecycle');

    if (apiScrollBar && apiLifecycleSection) {
        var stageNames = ['client', 'gateway', 'builder', 'db', 'response'];

        function updateApiTimeline() {
            var rect = apiLifecycleSection.getBoundingClientRect();
            var viewportHeight = window.innerHeight;
            var sectionHeight = rect.height;

            // Calculate scroll progress through the section
            var scrollProgress = Math.max(0, Math.min(1,
                (viewportHeight - rect.top) / (viewportHeight + sectionHeight)
            ));

            // Update progress bar
            apiScrollBar.style.setProperty('--progress', (scrollProgress * 100) + '%');

            // Update active stages
            var activeIndex = Math.floor(scrollProgress * stageNames.length);
            apiScrollStages.forEach(function(stage, index) {
                if (index <= activeIndex) {
                    stage.classList.add('active');
                } else {
                    stage.classList.remove('active');
                }
            });
        }

        // Update on scroll
        window.addEventListener('scroll', function() {
            requestAnimationFrame(updateApiTimeline);
        });

        // Stage click to scroll
        apiScrollStages.forEach(function(stage) {
            stage.addEventListener('click', function() {
                var stageName = stage.getAttribute('data-stage');
                var node = document.querySelector('.api-lifecycle__node--' + stageName);
                if (node) {
                    node.scrollIntoView({ behavior: 'smooth', block: 'center' });
                    node.classList.add('active');
                    setTimeout(function() { node.classList.remove('active'); }, 2000);
                }
            });
        });
    }

    // ---- Magnetic Buttons ----
    var magneticButtons = document.querySelectorAll('.btn--primary, .btn--ghost, .hero__actions .btn');
    var magneticThreshold = 100; // pixels

    magneticButtons.forEach(function(btn) {
        btn.addEventListener('mousemove', function(e) {
            var rect = btn.getBoundingClientRect();
            var x = e.clientX - rect.left;
            var y = e.clientY - rect.top;
            var centerX = rect.width / 2;
            var centerY = rect.height / 2;

            var dx = x - centerX;
            var dy = y - centerY;
            var distance = Math.sqrt(dx * dx + dy * dy);

            if (distance < magneticThreshold) {
                var strength = 0.3; // How much the button follows the cursor
                var moveX = dx * strength;
                var moveY = dy * strength;

                btn.style.transform = 'translate(' + moveX + 'px, ' + moveY + 'px) scale(1.05)';
                btn.style.transition = 'transform 0.2s ease-out';

                // Move the inner content slightly more for parallax effect
                var inner = btn.querySelector('span, svg');
                if (inner) {
                    inner.style.transform = 'translate(' + (moveX * 0.5) + 'px, ' + (moveY * 0.5) + 'px)';
                    inner.style.transition = 'transform 0.1s ease-out';
                }
            }
        });

        btn.addEventListener('mouseleave', function() {
            btn.style.transform = '';
            btn.style.transition = 'transform 0.4s cubic-bezier(0.16, 1, 0.3, 1)';

            var inner = btn.querySelector('span, svg');
            if (inner) {
                inner.style.transform = '';
                inner.style.transition = 'transform 0.3s cubic-bezier(0.16, 1, 0.3, 1)';
            }
        });

        btn.addEventListener('mouseenter', function() {
            btn.style.transition = 'transform 0.1s ease-out';
        });
    });

    // ---- Node Hover Tilt Animation ----
    var lifecycleNodes = document.querySelectorAll('.api-lifecycle__node');

    lifecycleNodes.forEach(function(node) {
        node.addEventListener('mousemove', function(e) {
            var rect = node.getBoundingClientRect();
            var x = e.clientX - rect.left;
            var y = e.clientY - rect.top;
            var centerX = rect.width / 2;
            var centerY = rect.height / 2;

            var rotateX = ((y - centerY) / centerY) * -8;
            var rotateY = ((x - centerX) / centerX) * 8;

            node.style.transform = 'perspective(500px) rotateX(' + rotateX + 'deg) rotateY(' + rotateY + 'deg) translateY(-4px) scale(1.05)';

            // Move glow with mouse
            var glowX = (x / rect.width) * 100;
            var glowY = (y / rect.height) * 100;
            node.style.background = 'radial-gradient(circle at ' + glowX + '% ' + glowY + '%, rgba(52, 211, 153, 0.08), rgba(15, 23, 42, 0.5))';
        });

        node.addEventListener('mouseleave', function() {
            node.style.transform = '';
            node.style.background = '';
        });
    });

    // ---- Toast Notifications ----
    var toastContainer = document.getElementById('toastContainer');
    var toastQueue = [];
    var toastActive = false;

    function showToast(message, type, duration) {
        type = type || 'info';
        duration = duration || 4000;

        var icons = {
            success: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="toast__icon toast__icon--success"><polyline points="20 6 9 17 4 12"/></svg>',
            info: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="toast__icon toast__icon--info"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg>',
            warning: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="toast__icon toast__icon--warning"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
            error: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="toast__icon toast__icon--error"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>'
        };

        var toast = document.createElement('div');
        toast.className = 'toast';
        toast.innerHTML = (icons[type] || icons.info)
            + '<span class="toast__text">' + message + '</span>'
            + '<div class="toast__progress"></div>';

        toastContainer.appendChild(toast);

        // Trigger show animation
        requestAnimationFrame(function() {
            toast.classList.add('show');
        });

        // Auto-hide after duration
        setTimeout(function() {
            toast.classList.remove('show');
            toast.classList.add('hide');
            setTimeout(function() {
                toast.remove();
            }, 400);
        }, duration);
    }

    // Show demo toasts when sections come into view
    var toastDemoSections = {
        'bento': [
            { msg: '<strong>111 modules</strong> loaded successfully', type: 'success' },
            { msg: '<strong>API Builder</strong> — 350+ endpoints ready', type: 'info' }
        ],
        'apiLifecycle': [
            { msg: '<strong>Gateway</strong> — JWT validated', type: 'success' },
            { msg: '<strong>Database</strong> — Query executed in 12ms', type: 'info' }
        ]
    };

    Object.keys(toastDemoSections).forEach(function(sectionId) {
        var section = document.getElementById(sectionId);
        if (section) {
            var toastObserver = new IntersectionObserver(function(entries) {
                if (entries[0].isIntersecting) {
                    var toasts = toastDemoSections[sectionId];
                    toasts.forEach(function(t, i) {
                        setTimeout(function() {
                            showToast(t.msg, t.type);
                        }, i * 2000 + 500);
                    });
                    toastObserver.unobserve(section);
                }
            }, { threshold: 0.3 });
            toastObserver.observe(section);
        }
    });

    // ============================================================
    // INTERACTIVE CLI TERMINAL
    // ============================================================
    var termBody = document.getElementById('terminalBody');
    var termInput = document.getElementById('terminalInput');
    var termAutocomplete = document.getElementById('terminalAutocomplete');

    var cliCommands = [
        { cmd: 'help', desc: 'Show all available commands' },
        { cmd: 'api create', desc: 'Create a new REST API (api create <name> --db <type> --table <name>)' },
        { cmd: 'api list', desc: 'List all created APIs' },
        { cmd: 'api delete', desc: 'Delete an API (api delete <name>)' },
        { cmd: 'api describe', desc: 'Show API details (api describe <name>)' },
        { cmd: 'db connect', desc: 'Connect to a database (db connect <type> --host <h> --port <p>)' },
        { cmd: 'db list', desc: 'List connected databases' },
        { cmd: 'db query', desc: 'Execute SQL query (db query --sql "SELECT ...")' },
        { cmd: 'storage bucket create', desc: 'Create a storage bucket (storage bucket create <name>)' },
        { cmd: 'storage bucket list', desc: 'List all storage buckets' },
        { cmd: 'storage object upload', desc: 'Upload an object (storage object upload <bucket> <file>)' },
        { cmd: 'workflow run', desc: 'Run an ETL workflow (workflow run <name> --input <file>)' },
        { cmd: 'workflow list', desc: 'List available workflows' },
        { cmd: 'workflow status', desc: 'Check workflow status (workflow status <id>)' },
        { cmd: 'cdc pipeline create', desc: 'Create a CDC pipeline (cdc pipeline create --source <s> --sink <s>)' },
        { cmd: 'cdc pipeline list', desc: 'List CDC pipelines' },
        { cmd: 'policy apply', desc: 'Apply a policy (policy apply <file.yaml>)' },
        { cmd: 'policy list', desc: 'List active policies' },
        { cmd: 'user create', desc: 'Create a user (user create --name <n> --email <e>)' },
        { cmd: 'user list', desc: 'List users' },
        { cmd: 'role assign', desc: 'Assign role to user (role assign <user> <role>)' },
        { cmd: 'metrics show', desc: 'Show platform metrics' },
        { cmd: 'health check', desc: 'Run health check on all services' },
        { cmd: 'config get', desc: 'Get configuration value (config get <key>)' },
        { cmd: 'config set', desc: 'Set configuration value (config set <key> <value>)' },
        { cmd: 'version', desc: 'Show axiomnizamctl version' },
        { cmd: 'clear', desc: 'Clear terminal output' },
    ];

    var termHistory = [];
    var termHistoryIdx = -1;
    var autocompleteIdx = -1;

    function termPrint(html, cls) {
        var div = document.createElement('div');
        div.className = 'terminal__line' + (cls ? ' ' + cls : '');
        div.innerHTML = html;
        if (termBody) {
            termBody.appendChild(div);
            termBody.scrollTop = termBody.scrollHeight;
        }
    }

    // Terminal scroll indicator
    if (termBody) {
        termBody.addEventListener('scroll', function() {
            if (termBody.scrollTop > 10) {
                termBody.classList.add('scrolled');
            } else {
                termBody.classList.remove('scrolled');
            }
        });
    }

    function executeCommand(input) {
        var raw = input.trim();
        if (!raw) return;
        termHistory.unshift(raw);
        if (termHistory.length > 50) termHistory.pop();
        termHistoryIdx = -1;

        termPrint('<span class="terminal__prompt">$</span> ' + escapeHTML(raw));

        var parts = raw.toLowerCase().split(/\s+/);
        var cmd = parts[0];
        var sub = parts[1] || '';
        var fullCmd = cmd + (sub ? ' ' + sub : '');

        if (cmd === 'clear') {
            if (termBody) termBody.innerHTML = '';
            return;
        }

        if (cmd === 'help') {
            termPrint('<span class="terminal__output-accent">Available commands:</span>');
            for (var i = 0; i < cliCommands.length; i++) {
                termPrint('  <span class="terminal__output-accent">' + cliCommands[i].cmd.padEnd(24) + '</span> <span class="terminal__output-dim">' + cliCommands[i].desc + '</span>');
            }
            return;
        }

        if (cmd === 'version') {
            termPrint('<span class="terminal__output-accent">axiomnizamctl</span> <span class="terminal__output-dim">v0.2.0</span>');
            termPrint('<span class="terminal__output-accent">AxiomNizam Platform</span> <span class="terminal__output-dim">v0.5.0</span>');
            termPrint('<span class="terminal__output-dim">Built: 2026-05-28 | Commit: baa1fff | Go: 1.24</span>');
            return;
        }

        if (cmd === 'api') {
            if (sub === 'create') {
                var name = parts[2] || 'my-api';
                var db = parts.indexOf('--db') > -1 ? parts[parts.indexOf('--db') + 1] : 'postgres';
                var table = parts.indexOf('--table') > -1 ? parts[parts.indexOf('--table') + 1] : 'default';
                termPrint('<span class="terminal__output-success">api "' + name + '" created</span>');
                termPrint('<span class="terminal__output-dim">  Database: ' + db + ' | Table: ' + table + ' | Endpoints: 5 CRUD</span>');
                termPrint('<span class="terminal__output-dim">  GET /api/v1/' + name + '  |  POST /api/v1/' + name + '  |  GET /api/v1/' + name + '/:id</span>');
                termPrint('<span class="terminal__output-dim">  PUT /api/v1/' + name + '/:id  |  DELETE /api/v1/' + name + '/:id</span>');
            } else if (sub === 'list') {
                termPrint('<span class="terminal__output-table">NAME          ENDPOINTS  DATABASE    STATUS</span>');
                termPrint('<span class="terminal__output-table">user-api      5          postgres    active</span>');
                termPrint('<span class="terminal__output-table">product-api   5          mysql       active</span>');
                termPrint('<span class="terminal__output-table">order-api     8          mongodb     active</span>');
            } else if (sub === 'delete') {
                var delName = parts[2] || '';
                termPrint('<span class="terminal__output-success">api "' + delName + '" deleted</span>');
            } else if (sub === 'describe') {
                var descName = parts[2] || '';
                termPrint('<span class="terminal__output-accent">API: ' + descName + '</span>');
                termPrint('<span class="terminal__output-dim">  Status: active | Created: 2026-05-28</span>');
                termPrint('<span class="terminal__output-dim">  Database: postgres | Table: users</span>');
                termPrint('<span class="terminal__output-dim">  Endpoints: 5 | Rate Limit: 1000/min</span>');
            } else {
                termPrint('<span class="terminal__output-error">Unknown api subcommand: ' + escapeHTML(sub) + '</span>');
                termPrint('<span class="terminal__output-dim">Try: api create, api list, api delete, api describe</span>');
            }
            return;
        }

        if (cmd === 'db') {
            if (sub === 'list') {
                termPrint('<span class="terminal__output-table">NAME          TYPE        HOST           STATUS</span>');
                termPrint('<span class="terminal__output-table">primary       postgres    localhost:5432  connected</span>');
                termPrint('<span class="terminal__output-table">analytics     mysql       localhost:3306  connected</span>');
                termPrint('<span class="terminal__output-table">nosql         mongodb     localhost:27017 connected</span>');
            } else if (sub === 'connect') {
                termPrint('<span class="terminal__output-success">Connected to database successfully</span>');
            } else if (sub === 'query') {
                termPrint('<span class="terminal__output-table">id  | name       | email</span>');
                termPrint('<span class="terminal__output-table">----|------------|------------------</span>');
                termPrint('<span class="terminal__output-table">1   | John Doe   | john@example.com</span>');
                termPrint('<span class="terminal__output-table">2   | Jane Smith | jane@example.com</span>');
                termPrint('<span class="terminal__output-dim">2 rows returned in 12ms</span>');
            } else {
                termPrint('<span class="terminal__output-error">Unknown db subcommand: ' + escapeHTML(sub) + '</span>');
            }
            return;
        }

        if (cmd === 'storage') {
            if (sub === 'bucket' && parts[2] === 'list') {
                termPrint('<span class="terminal__output-table">NAME          OBJECTS  SIZE      ENCRYPTED</span>');
                termPrint('<span class="terminal__output-table">uploads       1,247    2.3 GB    yes</span>');
                termPrint('<span class="terminal__output-table">backups       89       456 MB    yes</span>');
                termPrint('<span class="terminal__output-table">media         3,456    12.1 GB   no</span>');
            } else if (sub === 'bucket' && parts[2] === 'create') {
                var bname = parts[3] || 'new-bucket';
                termPrint('<span class="terminal__output-success">bucket "' + bname + '" created</span>');
            } else {
                termPrint('<span class="terminal__output-dim">Usage: storage bucket create|list [name]</span>');
            }
            return;
        }

        if (cmd === 'workflow') {
            if (sub === 'run') {
                var wfName = parts[2] || 'pipeline';
                termPrint('<span class="terminal__output-success">workflow "' + wfName + '" started</span>');
                termPrint('<span class="terminal__output-dim">  ID: wf-' + Math.random().toString(36).substr(2, 8) + '</span>');
                termPrint('<span class="terminal__output-dim">  Records: 1,247 processed | Duration: 3.2s</span>');
            } else if (sub === 'list') {
                termPrint('<span class="terminal__output-table">NAME            STATUS    LAST RUN</span>');
                termPrint('<span class="terminal__output-table">etl-pipeline    running   2 min ago</span>');
                termPrint('<span class="terminal__output-table">data-sync       idle      1 hour ago</span>');
            } else if (sub === 'status') {
                termPrint('<span class="terminal__output-accent">Workflow Status: running</span>');
                termPrint('<span class="terminal__output-dim">  Progress: 78% | Records: 973/1,247</span>');
            } else {
                termPrint('<span class="terminal__output-error">Unknown workflow subcommand</span>');
            }
            return;
        }

        if (cmd === 'cdc') {
            termPrint('<span class="terminal__output-success">CDC pipeline operation completed</span>');
            return;
        }

        if (cmd === 'policy') {
            if (sub === 'list') {
                termPrint('<span class="terminal__output-table">NAME            TYPE        STATUS</span>');
                termPrint('<span class="terminal__output-table">compliance      access      active</span>');
                termPrint('<span class="terminal__output-table">rate-limit      quota       active</span>');
                termPrint('<span class="terminal__output-table">data-retention  lifecycle   active</span>');
            } else if (sub === 'apply') {
                termPrint('<span class="terminal__output-success">policy applied — 3 rules active</span>');
            } else {
                termPrint('<span class="terminal__output-error">Unknown policy subcommand</span>');
            }
            return;
        }

        if (cmd === 'user') {
            if (sub === 'list') {
                termPrint('<span class="terminal__output-table">NAME          EMAIL               ROLE</span>');
                termPrint('<span class="terminal__output-table">admin         admin@axiom.io      admin</span>');
                termPrint('<span class="terminal__output-table">developer     dev@axiom.io        manager</span>');
            } else {
                termPrint('<span class="terminal__output-success">User operation completed</span>');
            }
            return;
        }

        if (cmd === 'role') {
            termPrint('<span class="terminal__output-success">Role operation completed</span>');
            return;
        }

        if (cmd === 'metrics') {
            termPrint('<span class="terminal__output-accent">Platform Metrics:</span>');
            termPrint('<span class="terminal__output-dim">  API Requests:  12,847/min</span>');
            termPrint('<span class="terminal__output-dim">  Avg Latency:   23ms</span>');
            termPrint('<span class="terminal__output-dim">  Error Rate:    0.02%</span>');
            termPrint('<span class="terminal__output-dim">  Uptime:        99.97%</span>');
            return;
        }

        if (cmd === 'health') {
            termPrint('<span class="terminal__output-accent">Health Check:</span>');
            termPrint('  <span class="terminal__output-success">OK</span>  API Server');
            termPrint('  <span class="terminal__output-success">OK</span>  Database');
            termPrint('  <span class="terminal__output-success">OK</span>  Storage');
            termPrint('  <span class="terminal__output-success">OK</span>  Message Queue');
            termPrint('  <span class="terminal__output-success">OK</span>  Cache');
            termPrint('<span class="terminal__output-dim">All services healthy</span>');
            return;
        }

        if (cmd === 'config') {
            termPrint('<span class="terminal__output-success">Config operation completed</span>');
            return;
        }

        termPrint('<span class="terminal__output-error">Command not found: ' + escapeHTML(cmd) + '</span>');
        termPrint('<span class="terminal__output-dim">Type "help" to see available commands</span>');
    }

    function updateAutocomplete(value) {
        if (!termAutocomplete) return;
        if (!value || value.length < 1) {
            termAutocomplete.classList.remove('visible');
            termAutocomplete.innerHTML = '';
            autocompleteIdx = -1;
            return;
        }

        var matches = [];
        var lower = value.toLowerCase();
        for (var i = 0; i < cliCommands.length; i++) {
            if (cliCommands[i].cmd.toLowerCase().indexOf(lower) === 0 || cliCommands[i].cmd.toLowerCase().indexOf(lower) > -1) {
                matches.push(cliCommands[i]);
            }
        }

        if (matches.length === 0) {
            termAutocomplete.classList.remove('visible');
            termAutocomplete.innerHTML = '';
            autocompleteIdx = -1;
            return;
        }

        var html = '<div class="terminal__autocomplete-hint"><kbd>Tab</kbd> to complete &middot; <kbd>&uarr;&darr;</kbd> to navigate &middot; <kbd>Enter</kbd> to select</div>';
        for (var j = 0; j < matches.length; j++) {
            html += '<div class="terminal__autocomplete-item' + (j === 0 ? ' active' : '') + '" data-cmd="' + matches[j].cmd + '" data-index="' + j + '">'
                + '<span class="terminal__autocomplete-cmd">' + matches[j].cmd + '</span>'
                + '<span class="terminal__autocomplete-desc">' + matches[j].desc + '</span>'
                + '</div>';
        }
        termAutocomplete.innerHTML = html;
        termAutocomplete.classList.add('visible');
        autocompleteIdx = 0;
    }

    function selectAutocompleteItem(idx) {
        var items = termAutocomplete ? termAutocomplete.querySelectorAll('.terminal__autocomplete-item') : [];
        if (items.length === 0) return;
        if (idx < 0) idx = items.length - 1;
        if (idx >= items.length) idx = 0;
        for (var i = 0; i < items.length; i++) {
            items[i].classList.toggle('active', i === idx);
        }
        autocompleteIdx = idx;
    }

    if (termInput) {
        termInput.addEventListener('input', function() {
            updateAutocomplete(termInput.value.trim());
        });

        termInput.addEventListener('keydown', function(e) {
            var acVisible = termAutocomplete && termAutocomplete.classList.contains('visible');

            // Tab completion
            if (e.key === 'Tab') {
                e.preventDefault();
                if (acVisible) {
                    var activeItem = termAutocomplete.querySelector('.terminal__autocomplete-item.active');
                    if (activeItem) {
                        termInput.value = activeItem.getAttribute('data-cmd') + ' ';
                        termAutocomplete.classList.remove('visible');
                        termInput.focus();
                    }
                }
                return;
            }

            // Arrow keys
            if (e.key === 'ArrowDown') {
                e.preventDefault();
                if (acVisible) {
                    selectAutocompleteItem(autocompleteIdx + 1);
                } else if (termHistory.length > 0) {
                    termHistoryIdx = Math.max(0, termHistoryIdx - 1);
                    termInput.value = termHistory[termHistoryIdx] || '';
                }
                return;
            }

            if (e.key === 'ArrowUp') {
                e.preventDefault();
                if (acVisible) {
                    selectAutocompleteItem(autocompleteIdx - 1);
                } else if (termHistory.length > 0) {
                    termHistoryIdx = Math.min(termHistory.length - 1, termHistoryIdx + 1);
                    termInput.value = termHistory[termHistoryIdx] || '';
                }
                return;
            }

            // Enter
            if (e.key === 'Enter') {
                e.preventDefault();
                var cmd = termInput.value.trim();
                if (acVisible) {
                    var selected = termAutocomplete.querySelector('.terminal__autocomplete-item.active');
                    if (selected) {
                        cmd = selected.getAttribute('data-cmd');
                        termInput.value = cmd + ' ';
                        termAutocomplete.classList.remove('visible');
                        // Don't execute on autocomplete select, let user finish typing args
                        return;
                    }
                }
                termInput.value = '';
                termAutocomplete.classList.remove('visible');
                executeCommand(cmd);
                return;
            }

            // Escape
            if (e.key === 'Escape') {
                termAutocomplete.classList.remove('visible');
                return;
            }
        });

        // Click autocomplete item
        if (termAutocomplete) {
            termAutocomplete.addEventListener('click', function(e) {
                var item = e.target.closest('.terminal__autocomplete-item');
                if (item) {
                    termInput.value = item.getAttribute('data-cmd') + ' ';
                    termAutocomplete.classList.remove('visible');
                    termInput.focus();
                }
            });
        }
    }

    // ---- Terminal Auto-Play Typing Animation ----
    var cliSection = document.getElementById('cli');
    var cliAnimated = false;

    if (cliSection && termInput) {
        var cliObserver = new IntersectionObserver(function(entries) {
            entries.forEach(function(entry) {
                if (entry.isIntersecting && !cliAnimated) {
                    cliAnimated = true;

                    var demoCommands = ['health check', 'api list', 'metrics show'];
                    var cmdIndex = 0;

                    function typeCommand() {
                        if (cmdIndex >= demoCommands.length) return;
                        var cmd = demoCommands[cmdIndex];
                        var charIndex = 0;

                        // Focus terminal
                        termInput.focus();

                        // Type each character
                        var typeInterval = setInterval(function() {
                            if (charIndex <= cmd.length) {
                                termInput.value = cmd.substring(0, charIndex);
                                charIndex++;
                            } else {
                                clearInterval(typeInterval);
                                // Execute after a short delay
                                setTimeout(function() {
                                    executeCommand(cmd);
                                    termInput.value = '';
                                    cmdIndex++;
                                    // Type next command after a delay
                                    setTimeout(typeCommand, 1500);
                                }, 500);
                            }
                        }, 80);
                    }

                    // Start typing after a delay
                    setTimeout(typeCommand, 1000);
                    cliObserver.unobserve(entry.target);
                }
            });
        }, { threshold: 0.3 });

        cliObserver.observe(cliSection);
    }

    // ============================================================
    // BENTO CARD HOVER ANIMATIONS
    // ============================================================
    var hoverCanvases = document.querySelectorAll('.bento__hover-canvas');
    var hoverAnimations = {};

    function initHoverCanvas(canvas) {
        var ctx = canvas.getContext('2d');
        var type = canvas.getAttribute('data-canvas');
        var particles = [];
        var animId = null;
        var rect = canvas.getBoundingClientRect();

        function resize() {
            rect = canvas.parentElement.getBoundingClientRect();
            canvas.width = rect.width;
            canvas.height = rect.height;
        }

        // API Builder animation — network graph
        var apiPackets = [];
        var apiSpawnTimer = 0;

        function apiAnimation() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            var w = canvas.width, h = canvas.height;
            var time = Date.now() * 0.001;

            // Hub node (center)
            var hub = { x: w * 0.5, y: h * 0.5 };

            // API endpoint nodes around the hub
            var nodes = [
                { x: w * 0.12, y: h * 0.2, method: 'GET', color: '#34d399', path: '/users' },
                { x: w * 0.88, y: h * 0.18, method: 'POST', color: '#38bdf8', path: '/orders' },
                { x: w * 0.15, y: h * 0.8, method: 'PUT', color: '#fbbf24', path: '/users/:id' },
                { x: w * 0.85, y: h * 0.82, method: 'DEL', color: '#ef4444', path: '/sessions' },
                { x: w * 0.5, y: h * 0.08, method: 'GET', color: '#a78bfa', path: '/products' },
                { x: w * 0.5, y: h * 0.92, method: 'POST', color: '#34d399', path: '/auth/login' },
                { x: w * 0.08, y: h * 0.5, method: 'GET', color: '#38bdf8', path: '/analytics' },
                { x: w * 0.92, y: h * 0.5, method: 'PATCH', color: '#f472b6', path: '/settings' },
            ];

            // Draw connections with gradient
            nodes.forEach(function(node, i) {
                var grad = ctx.createLinearGradient(hub.x, hub.y, node.x, node.y);
                grad.addColorStop(0, 'rgba(52, 211, 153, 0.12)');
                grad.addColorStop(0.5, node.color + '18');
                grad.addColorStop(1, node.color + '30');

                // Curved bezier connection
                var cpx = (hub.x + node.x) / 2 + (Math.sin(i * 1.2) * 30);
                var cpy = (hub.y + node.y) / 2 + (Math.cos(i * 0.8) * 25);

                ctx.beginPath();
                ctx.moveTo(hub.x, hub.y);
                ctx.quadraticCurveTo(cpx, cpy, node.x, node.y);
                ctx.strokeStyle = grad;
                ctx.lineWidth = 1.5;
                ctx.stroke();

                // Animated pulse along the connection
                var pulseT = ((time * 0.3 + i * 0.12) % 1);
                var px = (1 - pulseT) * (1 - pulseT) * hub.x + 2 * (1 - pulseT) * pulseT * cpx + pulseT * pulseT * node.x;
                var py = (1 - pulseT) * (1 - pulseT) * hub.y + 2 * (1 - pulseT) * pulseT * cpy + pulseT * pulseT * node.y;

                ctx.beginPath();
                ctx.arc(px, py, 2.5, 0, Math.PI * 2);
                ctx.fillStyle = node.color + 'aa';
                ctx.fill();

                // Pulse glow
                ctx.beginPath();
                ctx.arc(px, py, 10, 0, Math.PI * 2);
                ctx.fillStyle = node.color + '15';
                ctx.fill();

                // Response packet going back
                var respT = ((time * 0.25 + i * 0.1 + 0.5) % 1);
                var rx = (1 - respT) * (1 - respT) * node.x + 2 * (1 - respT) * respT * cpx + respT * respT * hub.x;
                var ry = (1 - respT) * (1 - respT) * node.y + 2 * (1 - respT) * respT * cpy + respT * respT * hub.y;

                ctx.beginPath();
                ctx.arc(rx, ry, 2, 0, Math.PI * 2);
                ctx.fillStyle = '#38bdf8aa';
                ctx.fill();
            });

            // Draw hub node
            var hubGlow = 0.3 + Math.sin(time * 2) * 0.1;
            ctx.beginPath();
            ctx.arc(hub.x, hub.y, 22, 0, Math.PI * 2);
            ctx.fillStyle = 'rgba(52, 211, 153, ' + hubGlow + ')';
            ctx.fill();
            ctx.beginPath();
            ctx.arc(hub.x, hub.y, 16, 0, Math.PI * 2);
            ctx.fillStyle = 'rgba(15, 23, 42, 0.9)';
            ctx.fill();
            ctx.beginPath();
            ctx.arc(hub.x, hub.y, 12, 0, Math.PI * 2);
            ctx.fillStyle = 'rgba(52, 211, 153, 0.15)';
            ctx.fill();

            // Hub icon (wrench)
            ctx.fillStyle = '#34d399';
            ctx.font = 'bold 10px monospace';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText('API', hub.x, hub.y);

            // Draw endpoint nodes
            nodes.forEach(function(node, i) {
                var pulse = 0.5 + Math.sin(time * 3 + i * 1.5) * 0.2;

                // Outer glow
                ctx.beginPath();
                ctx.arc(node.x, node.y, 14, 0, Math.PI * 2);
                ctx.fillStyle = node.color + '12';
                ctx.fill();

                // Node circle
                ctx.beginPath();
                ctx.arc(node.x, node.y, 9, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(15, 23, 42, 0.85)';
                ctx.fill();
                ctx.strokeStyle = node.color + Math.floor(pulse * 255).toString(16).padStart(2, '0');
                ctx.lineWidth = 1.5;
                ctx.stroke();

                // Method label
                ctx.fillStyle = node.color;
                ctx.font = 'bold 10px monospace';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(node.method, node.x, node.y);
            });

            // Spawn floating data packets
            apiSpawnTimer += 0.016;
            if (apiSpawnTimer > 0.3) {
                apiSpawnTimer = 0;
                var srcNode = nodes[Math.floor(Math.random() * nodes.length)];
                apiPackets.push({
                    x: hub.x, y: hub.y,
                    tx: srcNode.x, ty: srcNode.y,
                    cx: (hub.x + srcNode.x) / 2 + (Math.random() - 0.5) * 40,
                    cy: (hub.y + srcNode.y) / 2 + (Math.random() - 0.5) * 30,
                    t: 0, speed: 0.012 + Math.random() * 0.008,
                    color: srcNode.color, size: Math.random() * 2 + 1.5
                });
            }

            // Draw & update floating packets
            for (var p = apiPackets.length - 1; p >= 0; p--) {
                var pkt = apiPackets[p];
                pkt.t += pkt.speed;
                if (pkt.t >= 1) {
                    apiPackets.splice(p, 1);
                    continue;
                }

                var tt = pkt.t;
                var ppx = (1 - tt) * (1 - tt) * pkt.x + 2 * (1 - tt) * tt * pkt.cx + tt * tt * pkt.tx;
                var ppy = (1 - tt) * (1 - tt) * pkt.y + 2 * (1 - tt) * tt * pkt.cy + tt * tt * pkt.ty;

                // Trail
                for (var tr = 0; tr < 4; tr++) {
                    var trT = Math.max(tt - tr * 0.03, 0);
                    var trx = (1 - trT) * (1 - trT) * pkt.x + 2 * (1 - trT) * trT * pkt.cx + trT * trT * pkt.tx;
                    var try_ = (1 - trT) * (1 - trT) * pkt.y + 2 * (1 - trT) * trT * pkt.cy + trT * trT * pkt.ty;
                    ctx.beginPath();
                    ctx.arc(trx, try_, pkt.size * (1 - tr * 0.2), 0, Math.PI * 2);
                    ctx.fillStyle = pkt.color + Math.floor((0.3 - tr * 0.07) * 255).toString(16).padStart(2, '0');
                    ctx.fill();
                }

                // Main packet
                ctx.beginPath();
                ctx.arc(ppx, ppy, pkt.size, 0, Math.PI * 2);
                ctx.fillStyle = pkt.color + 'dd';
                ctx.fill();

                ctx.beginPath();
                ctx.arc(ppx, ppy, pkt.size + 5, 0, Math.PI * 2);
                ctx.fillStyle = pkt.color + '18';
                ctx.fill();
            }

            // Draw "code" in corner
            var codeY = 14;
            var codeX = 12;
            ctx.textAlign = 'left';
            ctx.textBaseline = 'top';
            ctx.font = 'bold 11px monospace';

            var codeLines = [
                { text: 'GET /api/v1/users', color: '#34d399' },
                { text: '200 OK  23ms', color: '#64748b' },
                { text: '{ "total": 2 }', color: '#a78bfa' },
            ];

            codeLines.forEach(function(line, i) {
                var charCount = Math.floor(((time * 8 + i * 10) % 30));
                var displayText = line.text.substring(0, Math.min(charCount, line.text.length));
                ctx.fillStyle = line.color;
                ctx.fillText(displayText, codeX, codeY + i * 12);
                // Cursor blink on last active line
                if (i === Math.floor((time * 2) % 3) && Math.sin(time * 8) > 0) {
                    var textW = ctx.measureText(displayText).width;
                    ctx.fillStyle = '#34d399';
                    ctx.fillRect(codeX + textW + 1, codeY + i * 12, 4, 9);
                }
            });

            // Stats in opposite corner
            ctx.textAlign = 'right';
            ctx.fillStyle = '#64748b';
            ctx.font = 'bold 10px monospace';
            ctx.fillText(nodes.length + ' endpoints', w - 10, 12);
            ctx.fillText(Math.floor(time * 100) + ' req/s', w - 10, 26);
            ctx.fillStyle = '#34d399';
            ctx.fillText('99.9% uptime', w - 10, 40);

            animId = requestAnimationFrame(apiAnimation);
        }

        // Storage animation — S3 bucket data flow network
        var storageDropPackets = [];

        function storageAnimation() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            var w = canvas.width, h = canvas.height;
            var time = Date.now() * 0.001;

            // Cloud nodes (top)
            var clouds = [
                { x: w * 0.2, y: h * 0.12, label: 'uploads', color: '#34d399' },
                { x: w * 0.5, y: h * 0.08, label: 'backups', color: '#38bdf8' },
                { x: w * 0.8, y: h * 0.14, label: 'media', color: '#a78bfa' },
            ];

            // Storage center node
            var center = { x: w * 0.5, y: h * 0.5 };

            // Client nodes (bottom)
            var clients = [
                { x: w * 0.15, y: h * 0.88, label: 'PUT', color: '#34d399' },
                { x: w * 0.4, y: h * 0.92, label: 'GET', color: '#38bdf8' },
                { x: w * 0.65, y: h * 0.9, label: 'LIST', color: '#fbbf24' },
                { x: w * 0.88, y: h * 0.86, label: 'DEL', color: '#ef4444' },
            ];

            // Draw connections: clients -> center -> clouds
            clients.forEach(function(cl, i) {
                var cpx = (cl.x + center.x) / 2 + Math.sin(i * 2) * 20;
                var cpy = (cl.y + center.y) / 2;
                ctx.beginPath();
                ctx.moveTo(cl.x, cl.y);
                ctx.quadraticCurveTo(cpx, cpy, center.x, center.y);
                ctx.strokeStyle = cl.color + '15';
                ctx.lineWidth = 1.2;
                ctx.stroke();

                // Packet from client to center
                var t = ((time * 0.35 + i * 0.25) % 1);
                var px = (1 - t) * (1 - t) * cl.x + 2 * (1 - t) * t * cpx + t * t * center.x;
                var py = (1 - t) * (1 - t) * cl.y + 2 * (1 - t) * t * cpy + t * t * center.y;
                ctx.beginPath();
                ctx.arc(px, py, 2.5, 0, Math.PI * 2);
                ctx.fillStyle = cl.color + 'bb';
                ctx.fill();
                ctx.beginPath();
                ctx.arc(px, py, 8, 0, Math.PI * 2);
                ctx.fillStyle = cl.color + '12';
                ctx.fill();
            });

            clouds.forEach(function(cld, i) {
                var cpx = (center.x + cld.x) / 2 + Math.cos(i * 1.5) * 15;
                var cpy = (center.y + cld.y) / 2;
                ctx.beginPath();
                ctx.moveTo(center.x, center.y);
                ctx.quadraticCurveTo(cpx, cpy, cld.x, cld.y);
                ctx.strokeStyle = cld.color + '15';
                ctx.lineWidth = 1.2;
                ctx.stroke();

                // Packet from center to cloud
                var t = ((time * 0.3 + i * 0.3 + 0.15) % 1);
                var px = (1 - t) * (1 - t) * center.x + 2 * (1 - t) * t * cpx + t * t * cld.x;
                var py = (1 - t) * (1 - t) * center.y + 2 * (1 - t) * t * cpy + t * t * cld.y;
                ctx.beginPath();
                ctx.arc(px, py, 2.5, 0, Math.PI * 2);
                ctx.fillStyle = cld.color + 'bb';
                ctx.fill();
                ctx.beginPath();
                ctx.arc(px, py, 8, 0, Math.PI * 2);
                ctx.fillStyle = cld.color + '12';
                ctx.fill();
            });

            // Draw center S3 node
            var centerPulse = 0.25 + Math.sin(time * 2) * 0.08;
            ctx.beginPath();
            ctx.arc(center.x, center.y, 24, 0, Math.PI * 2);
            ctx.fillStyle = 'rgba(52, 211, 153, ' + centerPulse + ')';
            ctx.fill();
            ctx.beginPath();
            ctx.arc(center.x, center.y, 17, 0, Math.PI * 2);
            ctx.fillStyle = 'rgba(15, 23, 42, 0.9)';
            ctx.fill();
            ctx.fillStyle = '#34d399';
            ctx.font = 'bold 11px monospace';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText('S3', center.x, center.y);

            // Draw cloud bucket nodes
            clouds.forEach(function(cld, i) {
                var pulse = 0.4 + Math.sin(time * 2.5 + i) * 0.15;
                ctx.beginPath();
                ctx.arc(cld.x, cld.y, 14, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(15, 23, 42, 0.85)';
                ctx.fill();
                ctx.strokeStyle = cld.color + Math.floor(pulse * 255).toString(16).padStart(2, '0');
                ctx.lineWidth = 1.5;
                ctx.stroke();
                ctx.fillStyle = cld.color;
                ctx.font = 'bold 9px monospace';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(cld.label, cld.x, cld.y);
            });

            // Draw client nodes
            clients.forEach(function(cl, i) {
                var pulse = 0.4 + Math.sin(time * 3 + i * 1.2) * 0.15;
                ctx.beginPath();
                ctx.arc(cl.x, cl.y, 11, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(15, 23, 42, 0.85)';
                ctx.fill();
                ctx.strokeStyle = cl.color + Math.floor(pulse * 255).toString(16).padStart(2, '0');
                ctx.lineWidth = 1.2;
                ctx.stroke();
                ctx.fillStyle = cl.color;
                ctx.font = 'bold 9px monospace';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(cl.label, cl.x, cl.y);
            });

            // Floating file particles
            if (Math.random() < 0.06) {
                storageDropPackets.push({
                    x: Math.random() * w,
                    y: h + 10,
                    vy: -(0.8 + Math.random() * 1.5),
                    size: Math.random() * 2.5 + 1,
                    life: 1,
                    type: Math.random() > 0.4 ? 'up' : 'down'
                });
            }
            for (var p = storageDropPackets.length - 1; p >= 0; p--) {
                var pkt = storageDropPackets[p];
                pkt.y += pkt.vy * (pkt.type === 'up' ? 1 : -1);
                pkt.life -= 0.006;
                if (pkt.life <= 0) { storageDropPackets.splice(p, 1); continue; }
                ctx.beginPath();
                ctx.arc(pkt.x, pkt.y, pkt.size, 0, Math.PI * 2);
                ctx.fillStyle = pkt.type === 'up' ? 'rgba(52,211,153,' + (pkt.life * 0.4) + ')' : 'rgba(56,189,248,' + (pkt.life * 0.4) + ')';
                ctx.fill();
            }

            // Stats
            ctx.textAlign = 'right';
            ctx.textBaseline = 'top';
            ctx.fillStyle = '#64748b';
            ctx.font = 'bold 10px monospace';
            ctx.fillText('3 buckets', w - 10, 12);
            ctx.fillText('4,792 objects', w - 10, 26);
            ctx.fillStyle = '#34d399';
            ctx.fillText('14.8 GB', w - 10, 40);

            animId = requestAnimationFrame(storageAnimation);
        }

        // CDC animation — multi-stream pipeline network
        var cdcPackets = [];

        function cdcAnimation() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            var w = canvas.width, h = canvas.height;
            var time = Date.now() * 0.001;

            // Source databases (left)
            var sources = [
                { x: w * 0.08, y: h * 0.2, label: 'PG', color: '#34d399' },
                { x: w * 0.08, y: h * 0.5, label: 'MY', color: '#38bdf8' },
                { x: w * 0.08, y: h * 0.8, label: 'MO', color: '#a78bfa' },
            ];

            // Pipeline stages (center)
            var stages = [
                { x: w * 0.35, y: h * 0.3, label: 'CDC', color: '#34d399' },
                { x: w * 0.55, y: h * 0.6, label: 'ETL', color: '#fbbf24' },
                { x: w * 0.55, y: h * 0.3, label: 'QA', color: '#38bdf8' },
            ];

            // Sinks (right)
            var sinks = [
                { x: w * 0.85, y: h * 0.2, label: 'Kafka', color: '#34d399' },
                { x: w * 0.92, y: h * 0.5, label: 'S3', color: '#38bdf8' },
                { x: w * 0.85, y: h * 0.8, label: 'ES', color: '#a78bfa' },
            ];

            // Draw source -> stage connections
            sources.forEach(function(src, si) {
                stages.forEach(function(stg, sti) {
                    if ((si + sti) % 2 === 0) {
                        var cpx = (src.x + stg.x) / 2;
                        var cpy = (src.y + stg.y) / 2 + Math.sin(si + sti) * 15;
                        ctx.beginPath();
                        ctx.moveTo(src.x, src.y);
                        ctx.quadraticCurveTo(cpx, cpy, stg.x, stg.y);
                        ctx.strokeStyle = src.color + '10';
                        ctx.lineWidth = 1;
                        ctx.stroke();

                        var t = ((time * 0.3 + si * 0.2 + sti * 0.15) % 1);
                        var px = (1 - t) * (1 - t) * src.x + 2 * (1 - t) * t * cpx + t * t * stg.x;
                        var py = (1 - t) * (1 - t) * src.y + 2 * (1 - t) * t * cpy + t * t * stg.y;
                        ctx.beginPath();
                        ctx.arc(px, py, 2, 0, Math.PI * 2);
                        ctx.fillStyle = src.color + 'aa';
                        ctx.fill();
                    }
                });
            });

            // Draw stage -> sink connections
            stages.forEach(function(stg, sti) {
                sinks.forEach(function(snk, sni) {
                    if ((sti + sni) % 2 === 0) {
                        var cpx = (stg.x + snk.x) / 2;
                        var cpy = (stg.y + snk.y) / 2 + Math.cos(sti + sni) * 12;
                        ctx.beginPath();
                        ctx.moveTo(stg.x, stg.y);
                        ctx.quadraticCurveTo(cpx, cpy, snk.x, snk.y);
                        ctx.strokeStyle = stg.color + '10';
                        ctx.lineWidth = 1;
                        ctx.stroke();

                        var t = ((time * 0.25 + sti * 0.2 + sni * 0.18 + 0.3) % 1);
                        var px = (1 - t) * (1 - t) * stg.x + 2 * (1 - t) * t * cpx + t * t * snk.x;
                        var py = (1 - t) * (1 - t) * stg.y + 2 * (1 - t) * t * cpy + t * t * snk.y;
                        ctx.beginPath();
                        ctx.arc(px, py, 2, 0, Math.PI * 2);
                        ctx.fillStyle = stg.color + 'aa';
                        ctx.fill();
                    }
                });
            });

            // Draw nodes
            var allGroups = [
                { nodes: sources, size: 12 },
                { nodes: stages, size: 14 },
                { nodes: sinks, size: 12 },
            ];

            allGroups.forEach(function(group) {
                group.nodes.forEach(function(node, i) {
                    var pulse = 0.35 + Math.sin(time * 2.5 + i * 1.3) * 0.15;
                    ctx.beginPath();
                    ctx.arc(node.x, node.y, group.size + 3, 0, Math.PI * 2);
                    ctx.fillStyle = node.color + '0a';
                    ctx.fill();
                    ctx.beginPath();
                    ctx.arc(node.x, node.y, group.size, 0, Math.PI * 2);
                    ctx.fillStyle = 'rgba(15, 23, 42, 0.85)';
                    ctx.fill();
                    ctx.strokeStyle = node.color + Math.floor(pulse * 255).toString(16).padStart(2, '0');
                    ctx.lineWidth = 1.3;
                    ctx.stroke();
                    ctx.fillStyle = node.color;
                    ctx.font = 'bold 9px monospace';
                    ctx.textAlign = 'center';
                    ctx.textBaseline = 'middle';
                    ctx.fillText(node.label, node.x, node.y);
                });
            });

            // Floating event particles
            if (Math.random() < 0.08) {
                var srcPick = sources[Math.floor(Math.random() * sources.length)];
                cdcPackets.push({
                    x: srcPick.x, y: srcPick.y,
                    tx: sinks[Math.floor(Math.random() * sinks.length)].x,
                    ty: sinks[Math.floor(Math.random() * sinks.length)].y,
                    cx: w * (0.3 + Math.random() * 0.3),
                    cy: h * (0.2 + Math.random() * 0.6),
                    t: 0, speed: 0.008 + Math.random() * 0.006,
                    color: srcPick.color, size: Math.random() * 2 + 1
                });
            }
            for (var p = cdcPackets.length - 1; p >= 0; p--) {
                var pkt = cdcPackets[p];
                pkt.t += pkt.speed;
                if (pkt.t >= 1) { cdcPackets.splice(p, 1); continue; }
                var tt = pkt.t;
                var ppx = (1 - tt) * (1 - tt) * pkt.x + 2 * (1 - tt) * tt * pkt.cx + tt * tt * pkt.tx;
                var ppy = (1 - tt) * (1 - tt) * pkt.y + 2 * (1 - tt) * tt * pkt.cy + tt * tt * pkt.ty;
                ctx.beginPath();
                ctx.arc(ppx, ppy, pkt.size, 0, Math.PI * 2);
                ctx.fillStyle = pkt.color + 'cc';
                ctx.fill();
                ctx.beginPath();
                ctx.arc(ppx, ppy, pkt.size + 5, 0, Math.PI * 2);
                ctx.fillStyle = pkt.color + '15';
                ctx.fill();
            }

            // Stats
            ctx.textAlign = 'right';
            ctx.textBaseline = 'top';
            ctx.fillStyle = '#64748b';
            ctx.font = 'bold 10px monospace';
            ctx.fillText('3 pipelines', w - 10, 12);
            ctx.fillText('24,537 evt/min', w - 10, 26);
            ctx.fillStyle = '#34d399';
            ctx.fillText('99.97% uptime', w - 10, 40);

            animId = requestAnimationFrame(cdcAnimation);
        }

        // Scanner animation — 6-stage pipeline with scan beam
        var scanDetected = [];

        function scannerAnimation() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            var w = canvas.width, h = canvas.height;
            var time = Date.now() * 0.001;

            // 6 scanner pipeline nodes in a row
            var scanNodes = [
                { x: w * 0.08, y: h * 0.35, label: 'META', color: '#34d399', finds: 3 },
                { x: w * 0.24, y: h * 0.35, label: 'MIME', color: '#38bdf8', finds: 5 },
                { x: w * 0.40, y: h * 0.35, label: 'SVG', color: '#a78bfa', finds: 1 },
                { x: w * 0.56, y: h * 0.35, label: 'MACRO', color: '#fbbf24', finds: 2 },
                { x: w * 0.72, y: h * 0.35, label: 'ARCH', color: '#34d399', finds: 0 },
                { x: w * 0.88, y: h * 0.35, label: 'AV', color: '#ef4444', finds: 1 },
            ];

            // Input file node (left)
            var inputNode = { x: w * 0.06, y: h * 0.75, label: 'FILE', color: '#e2e8f0' };
            // Output node (right)
            var outputNode = { x: w * 0.94, y: h * 0.75, label: 'SAFE', color: '#34d399' };

            // Scanning beam position
            var beamProgress = ((time * 0.2) % 1);
            var beamIdx = Math.floor(beamProgress * 6);
            var beamX = scanNodes[Math.min(beamIdx, 5)].x;

            // Draw connections: input -> nodes -> output
            ctx.beginPath();
            ctx.moveTo(inputNode.x, inputNode.y);
            scanNodes.forEach(function(node) {
                ctx.lineTo(node.x, node.y);
            });
            ctx.lineTo(outputNode.x, outputNode.y);
            ctx.strokeStyle = 'rgba(52, 211, 153, 0.08)';
            ctx.lineWidth = 2;
            ctx.stroke();

            // Animated beam along the pipeline
            var beamT = ((time * 0.4) % 1);
            var totalNodes = scanNodes.length + 2;
            var allPipelineNodes = [inputNode].concat(scanNodes).concat([outputNode]);
            var segT = beamT * (allPipelineNodes.length - 1);
            var segIdx = Math.min(Math.floor(segT), allPipelineNodes.length - 2);
            var localT = segT - segIdx;
            var fromN = allPipelineNodes[segIdx];
            var toN = allPipelineNodes[segIdx + 1];
            var bx = fromN.x + (toN.x - fromN.x) * localT;
            var by = fromN.y + (toN.y - fromN.y) * localT;

            // Beam glow
            ctx.beginPath();
            ctx.arc(bx, by, 12, 0, Math.PI * 2);
            ctx.fillStyle = 'rgba(52, 211, 153, 0.15)';
            ctx.fill();
            ctx.beginPath();
            ctx.arc(bx, by, 5, 0, Math.PI * 2);
            ctx.fillStyle = 'rgba(52, 211, 153, 0.6)';
            ctx.fill();

            // Beam line sweep
            var grad = ctx.createLinearGradient(beamX - 50, 0, beamX + 50, 0);
            grad.addColorStop(0, 'rgba(52, 211, 153, 0)');
            grad.addColorStop(0.5, 'rgba(52, 211, 153, 0.06)');
            grad.addColorStop(1, 'rgba(52, 211, 153, 0)');
            ctx.fillStyle = grad;
            ctx.fillRect(beamX - 50, 0, 100, h);

            // Draw scan nodes
            scanNodes.forEach(function(node, i) {
                var isActive = i === beamIdx;
                var isPast = i < beamIdx;
                var pulse = isActive ? 0.6 + Math.sin(time * 6) * 0.2 : 0.3;

                // Glow if active
                if (isActive) {
                    ctx.beginPath();
                    ctx.arc(node.x, node.y, 20, 0, Math.PI * 2);
                    ctx.fillStyle = node.color + '18';
                    ctx.fill();
                }

                // Node
                ctx.beginPath();
                ctx.arc(node.x, node.y, 12, 0, Math.PI * 2);
                ctx.fillStyle = isPast || isActive ? 'rgba(15, 23, 42, 0.85)' : 'rgba(15, 23, 42, 0.7)';
                ctx.fill();
                var strokeColor = isPast || isActive ? node.color + Math.floor(pulse * 255).toString(16).padStart(2, '0') : 'rgba(148,163,184,0.15)';
                ctx.strokeStyle = strokeColor;
                ctx.lineWidth = isActive ? 2 : 1.2;
                ctx.stroke();

                ctx.fillStyle = isPast || isActive ? node.color : '#475569';
                ctx.font = 'bold 9px monospace';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(node.label, node.x, node.y);

                // Checkmark if passed
                if (isPast) {
                    ctx.fillStyle = '#34d399';
                    ctx.font = 'bold 10px monospace';
                    ctx.fillText('✓', node.x, node.y - 18);
                }

                // Finding count
                if (node.finds > 0 && (isPast || isActive)) {
                    ctx.beginPath();
                    ctx.arc(node.x + 10, node.y - 10, 5, 0, Math.PI * 2);
                    ctx.fillStyle = node.finds > 2 ? '#ef4444' : '#fbbf24';
                    ctx.fill();
                    ctx.fillStyle = '#fff';
                    ctx.font = 'bold 8px monospace';
                    ctx.fillText(node.finds, node.x + 10, node.y - 10);
                }
            });

            // Draw input/output nodes
            [inputNode, outputNode].forEach(function(node) {
                ctx.beginPath();
                ctx.arc(node.x, node.y, 14, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(15, 23, 42, 0.85)';
                ctx.fill();
                ctx.strokeStyle = node.color + '40';
                ctx.lineWidth = 1.3;
                ctx.stroke();
                ctx.fillStyle = node.color;
                ctx.font = 'bold 10px monospace';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(node.label, node.x, node.y);
            });

            // Threat detection radar pings
            if (Math.random() < 0.015) {
                scanDetected.push({
                    x: w * (0.2 + Math.random() * 0.6),
                    y: h * (0.15 + Math.random() * 0.7),
                    life: 1, size: Math.random() * 15 + 8
                });
            }
            for (var d = scanDetected.length - 1; d >= 0; d--) {
                var det = scanDetected[d];
                det.life -= 0.025;
                if (det.life <= 0) { scanDetected.splice(d, 1); continue; }
                // Expanding ring
                ctx.beginPath();
                ctx.arc(det.x, det.y, det.size * (1 - det.life * 0.5), 0, Math.PI * 2);
                ctx.strokeStyle = 'rgba(239, 68, 68, ' + (det.life * 0.4) + ')';
                ctx.lineWidth = 1.5;
                ctx.stroke();
                // Crosshair
                ctx.strokeStyle = 'rgba(239, 68, 68, ' + (det.life * 0.5) + ')';
                ctx.lineWidth = 1;
                ctx.beginPath();
                ctx.moveTo(det.x - 6, det.y);
                ctx.lineTo(det.x + 6, det.y);
                ctx.moveTo(det.x, det.y - 6);
                ctx.lineTo(det.x, det.y + 6);
                ctx.stroke();
            }

            // Stats
            ctx.textAlign = 'right';
            ctx.textBaseline = 'top';
            ctx.fillStyle = '#64748b';
            ctx.font = 'bold 10px monospace';
            ctx.fillText('2,847 scans', w - 10, 12);
            ctx.fillText('99.6% safe', w - 10, 26);
            ctx.fillStyle = '#34d399';
            ctx.fillText('6 scanners', w - 10, 40);

            animId = requestAnimationFrame(scannerAnimation);
        }

        // Conductor animation — message flow between producers, brokers, consumers
        var conductorPackets = [];
        var conductorSpawnTimer = 0;

        function conductorAnimation() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            var w = canvas.width, h = canvas.height;
            var time = Date.now() * 0.001;

            // Broker nodes (center cluster)
            var brokers = [
                { x: w * 0.5, y: h * 0.35, label: 'Kafka', color: '#34d399' },
                { x: w * 0.5, y: h * 0.65, label: 'RabbitMQ', color: '#38bdf8' }
            ];

            // Producer nodes (left side)
            var producers = [
                { x: w * 0.1, y: h * 0.2, label: 'P1', color: '#a78bfa' },
                { x: w * 0.08, y: h * 0.5, label: 'P2', color: '#a78bfa' },
                { x: w * 0.1, y: h * 0.8, label: 'P3', color: '#a78bfa' }
            ];

            // Consumer nodes (right side)
            var consumers = [
                { x: w * 0.9, y: h * 0.15, label: 'C1', color: '#fbbf24' },
                { x: w * 0.92, y: h * 0.4, label: 'C2', color: '#fbbf24' },
                { x: w * 0.9, y: h * 0.65, label: 'C3', color: '#fbbf24' },
                { x: w * 0.92, y: h * 0.9, label: 'C4', color: '#fbbf24' }
            ];

            // Draw producer → broker connections
            producers.forEach(function(prod, pi) {
                brokers.forEach(function(broker, bi) {
                    var grad = ctx.createLinearGradient(prod.x, prod.y, broker.x, broker.y);
                    grad.addColorStop(0, 'rgba(167, 139, 250, 0.1)');
                    grad.addColorStop(1, 'rgba(52, 211, 153, 0.08)');

                    var cpx = (prod.x + broker.x) / 2;
                    var cpy = (prod.y + broker.y) / 2 + Math.sin(pi + bi) * 15;

                    ctx.beginPath();
                    ctx.moveTo(prod.x, prod.y);
                    ctx.quadraticCurveTo(cpx, cpy, broker.x, broker.y);
                    ctx.strokeStyle = grad;
                    ctx.lineWidth = 1;
                    ctx.stroke();
                });
            });

            // Draw broker → consumer connections
            brokers.forEach(function(broker, bi) {
                consumers.forEach(function(cons, ci) {
                    var grad = ctx.createLinearGradient(broker.x, broker.y, cons.x, cons.y);
                    grad.addColorStop(0, 'rgba(52, 211, 153, 0.08)');
                    grad.addColorStop(1, 'rgba(251, 191, 36, 0.1)');

                    var cpx = (broker.x + cons.x) / 2;
                    var cpy = (broker.y + cons.y) / 2 + Math.sin(bi + ci) * 15;

                    ctx.beginPath();
                    ctx.moveTo(broker.x, broker.y);
                    ctx.quadraticCurveTo(cpx, cpy, cons.x, cons.y);
                    ctx.strokeStyle = grad;
                    ctx.lineWidth = 1;
                    ctx.stroke();
                });
            });

            // Draw broker nodes
            brokers.forEach(function(broker, i) {
                var pulse = 0.3 + Math.sin(time * 2 + i) * 0.1;

                ctx.beginPath();
                ctx.arc(broker.x, broker.y, 20, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(52, 211, 153, ' + pulse + ')';
                ctx.fill();

                ctx.beginPath();
                ctx.arc(broker.x, broker.y, 14, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(15, 23, 42, 0.9)';
                ctx.fill();

                ctx.beginPath();
                ctx.arc(broker.x, broker.y, 10, 0, Math.PI * 2);
                ctx.fillStyle = broker.color + '20';
                ctx.fill();

                ctx.fillStyle = broker.color;
                ctx.font = 'bold 9px monospace';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(broker.label, broker.x, broker.y);
            });

            // Draw producer nodes
            producers.forEach(function(prod, i) {
                var pulse = 0.4 + Math.sin(time * 3 + i * 2) * 0.15;

                ctx.beginPath();
                ctx.arc(prod.x, prod.y, 11, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(15, 23, 42, 0.85)';
                ctx.fill();
                ctx.strokeStyle = prod.color + Math.floor(pulse * 255).toString(16).padStart(2, '0');
                ctx.lineWidth = 1.5;
                ctx.stroke();

                ctx.fillStyle = prod.color;
                ctx.font = 'bold 9px monospace';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(prod.label, prod.x, prod.y);
            });

            // Draw consumer nodes
            consumers.forEach(function(cons, i) {
                var pulse = 0.4 + Math.sin(time * 2.5 + i * 1.8) * 0.15;

                ctx.beginPath();
                ctx.arc(cons.x, cons.y, 11, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(15, 23, 42, 0.85)';
                ctx.fill();
                ctx.strokeStyle = cons.color + Math.floor(pulse * 255).toString(16).padStart(2, '0');
                ctx.lineWidth = 1.5;
                ctx.stroke();

                ctx.fillStyle = cons.color;
                ctx.font = 'bold 9px monospace';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(cons.label, cons.x, cons.y);
            });

            // Spawn message packets from producers to brokers
            conductorSpawnTimer += 0.016;
            if (conductorSpawnTimer > 0.25) {
                conductorSpawnTimer = 0;
                var srcProd = producers[Math.floor(Math.random() * producers.length)];
                var tgtBroker = brokers[Math.floor(Math.random() * brokers.length)];
                conductorPackets.push({
                    x: srcProd.x, y: srcProd.y,
                    tx: tgtBroker.x, ty: tgtBroker.y,
                    cx: (srcProd.x + tgtBroker.x) / 2 + (Math.random() - 0.5) * 20,
                    cy: (srcProd.y + tgtBroker.y) / 2 + (Math.random() - 0.5) * 20,
                    t: 0, speed: 0.015 + Math.random() * 0.01,
                    color: '#a78bfa', size: Math.random() * 2 + 1.5
                });

                // Also spawn from brokers to consumers
                var tgtCons = consumers[Math.floor(Math.random() * consumers.length)];
                conductorPackets.push({
                    x: tgtBroker.x, y: tgtBroker.y,
                    tx: tgtCons.x, ty: tgtCons.y,
                    cx: (tgtBroker.x + tgtCons.x) / 2 + (Math.random() - 0.5) * 20,
                    cy: (tgtBroker.y + tgtCons.y) / 2 + (Math.random() - 0.5) * 20,
                    t: 0, speed: 0.015 + Math.random() * 0.01,
                    color: '#fbbf24', size: Math.random() * 2 + 1.5
                });
            }

            // Draw & update message packets
            for (var p = conductorPackets.length - 1; p >= 0; p--) {
                var pkt = conductorPackets[p];
                pkt.t += pkt.speed;
                if (pkt.t >= 1) {
                    conductorPackets.splice(p, 1);
                    continue;
                }

                var tt = pkt.t;
                var ppx = (1 - tt) * (1 - tt) * pkt.x + 2 * (1 - tt) * tt * pkt.cx + tt * tt * pkt.tx;
                var ppy = (1 - tt) * (1 - tt) * pkt.y + 2 * (1 - tt) * tt * pkt.cy + tt * tt * pkt.ty;

                // Trail
                for (var tr = 0; tr < 3; tr++) {
                    var trT = Math.max(tt - tr * 0.04, 0);
                    var trx = (1 - trT) * (1 - trT) * pkt.x + 2 * (1 - trT) * trT * pkt.cx + trT * trT * pkt.tx;
                    var try_ = (1 - trT) * (1 - trT) * pkt.y + 2 * (1 - trT) * trT * pkt.cy + trT * trT * pkt.ty;
                    ctx.beginPath();
                    ctx.arc(trx, try_, pkt.size * (1 - tr * 0.25), 0, Math.PI * 2);
                    ctx.fillStyle = pkt.color + Math.floor((0.25 - tr * 0.08) * 255).toString(16).padStart(2, '0');
                    ctx.fill();
                }

                // Main packet
                ctx.beginPath();
                ctx.arc(ppx, ppy, pkt.size, 0, Math.PI * 2);
                ctx.fillStyle = pkt.color + 'cc';
                ctx.fill();

                ctx.beginPath();
                ctx.arc(ppx, ppy, pkt.size + 4, 0, Math.PI * 2);
                ctx.fillStyle = pkt.color + '15';
                ctx.fill();
            }

            // Stats in corner
            ctx.textAlign = 'right';
            ctx.textBaseline = 'top';
            ctx.fillStyle = '#64748b';
            ctx.font = 'bold 10px monospace';
            ctx.fillText('34.2K msg/min', w - 10, 12);
            ctx.fillText('2 brokers', w - 10, 26);
            ctx.fillStyle = '#34d399';
            ctx.fillText('2ms latency', w - 10, 40);

            animId = requestAnimationFrame(conductorAnimation);
        }

        // Analytics animation — bar chart with flowing data
        var analyticsBars = [];
        var analyticsSpawnTimer = 0;

        function analyticsAnimation() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            var w = canvas.width, h = canvas.height;
            var time = Date.now() * 0.001;

            // Draw grid lines
            for (var i = 0; i < 5; i++) {
                var y = h * 0.2 + (h * 0.6 / 4) * i;
                ctx.beginPath();
                ctx.moveTo(w * 0.05, y);
                ctx.lineTo(w * 0.95, y);
                ctx.strokeStyle = 'rgba(148, 163, 184, 0.06)';
                ctx.lineWidth = 1;
                ctx.stroke();
            }

            // Animated bar chart
            var barCount = 12;
            var barWidth = (w * 0.9) / barCount;
            var barGap = barWidth * 0.2;

            for (var i = 0; i < barCount; i++) {
                var barHeight = (Math.sin(time * 2 + i * 0.5) * 0.3 + 0.5) * h * 0.5;
                var x = w * 0.05 + i * barWidth + barGap / 2;
                var y = h * 0.7 - barHeight;

                var grad = ctx.createLinearGradient(x, y + barHeight, x, y);
                grad.addColorStop(0, 'rgba(52, 211, 153, 0.3)');
                grad.addColorStop(1, 'rgba(56, 189, 248, 0.5)');

                ctx.fillStyle = grad;
                ctx.beginPath();
                ctx.roundRect(x, y, barWidth - barGap, barHeight, [3, 3, 0, 0]);
                ctx.fill();
            }

            // Flowing line chart
            ctx.beginPath();
            ctx.strokeStyle = 'rgba(52, 211, 153, 0.4)';
            ctx.lineWidth = 2;
            for (var i = 0; i < barCount; i++) {
                var x = w * 0.05 + i * barWidth + barWidth / 2;
                var y = h * 0.3 + Math.sin(time * 1.5 + i * 0.8) * h * 0.15;
                if (i === 0) ctx.moveTo(x, y);
                else ctx.lineTo(x, y);
            }
            ctx.stroke();

            // Data dots
            for (var i = 0; i < barCount; i++) {
                var x = w * 0.05 + i * barWidth + barWidth / 2;
                var y = h * 0.3 + Math.sin(time * 1.5 + i * 0.8) * h * 0.15;

                ctx.beginPath();
                ctx.arc(x, y, 3, 0, Math.PI * 2);
                ctx.fillStyle = '#34d399';
                ctx.fill();
            }

            // Stats in corner
            ctx.textAlign = 'right';
            ctx.textBaseline = 'top';
            ctx.fillStyle = '#64748b';
            ctx.font = 'bold 10px monospace';
            ctx.fillText('24.8K requests', w - 10, 12);
            ctx.fillText('5 dashboards', w - 10, 26);
            ctx.fillStyle = '#34d399';
            ctx.fillText('98.2% uptime', w - 10, 40);

            animId = requestAnimationFrame(analyticsAnimation);
        }

        // GIS animation — map with pulsing markers
        var gisMarkers = [];
        var gisSpawnTimer = 0;

        function gisAnimation() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            var w = canvas.width, h = canvas.height;
            var time = Date.now() * 0.001;

            // Draw grid (map grid)
            ctx.strokeStyle = 'rgba(148, 163, 184, 0.04)';
            ctx.lineWidth = 1;
            for (var x = 0; x < w; x += 30) {
                ctx.beginPath();
                ctx.moveTo(x, 0);
                ctx.lineTo(x, h);
                ctx.stroke();
            }
            for (var y = 0; y < h; y += 30) {
                ctx.beginPath();
                ctx.moveTo(0, y);
                ctx.lineTo(w, y);
                ctx.stroke();
            }

            // Draw regions (filled areas)
            var regions = [
                { x: w * 0.15, y: h * 0.2, rx: w * 0.2, ry: h * 0.15, color: '52, 211, 153' },
                { x: w * 0.6, y: h * 0.4, rx: w * 0.25, ry: h * 0.2, color: '56, 189, 248' },
                { x: w * 0.3, y: h * 0.65, rx: w * 0.18, ry: h * 0.12, color: '167, 139, 250' }
            ];

            regions.forEach(function(r) {
                ctx.beginPath();
                ctx.ellipse(r.x, r.y, r.rx, r.ry, 0, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(' + r.color + ', 0.06)';
                ctx.fill();
                ctx.strokeStyle = 'rgba(' + r.color + ', 0.15)';
                ctx.lineWidth = 1;
                ctx.stroke();
            });

            // Draw markers with pulse
            var markers = [
                { x: w * 0.2, y: h * 0.25, color: '#34d399' },
                { x: w * 0.7, y: h * 0.35, color: '#38bdf8' },
                { x: w * 0.4, y: h * 0.6, color: '#a78bfa' },
                { x: w * 0.8, y: h * 0.5, color: '#fbbf24' },
                { x: w * 0.55, y: h * 0.75, color: '#f472b6' }
            ];

            markers.forEach(function(m, i) {
                var pulse = Math.sin(time * 3 + i) * 0.5 + 0.5;

                // Outer pulse ring
                ctx.beginPath();
                ctx.arc(m.x, m.y, 12 + pulse * 8, 0, Math.PI * 2);
                ctx.fillStyle = m.color + '10';
                ctx.fill();

                // Inner marker
                ctx.beginPath();
                ctx.arc(m.x, m.y, 4, 0, Math.PI * 2);
                ctx.fillStyle = m.color;
                ctx.fill();
            });

            // Draw connections between markers
            ctx.strokeStyle = 'rgba(148, 163, 184, 0.08)';
            ctx.lineWidth = 1;
            for (var i = 0; i < markers.length - 1; i++) {
                ctx.beginPath();
                ctx.moveTo(markers[i].x, markers[i].y);
                ctx.lineTo(markers[i + 1].x, markers[i + 1].y);
                ctx.stroke();
            }

            // Stats in corner
            ctx.textAlign = 'right';
            ctx.textBaseline = 'top';
            ctx.fillStyle = '#64748b';
            ctx.font = 'bold 10px monospace';
            ctx.fillText('6 map layers', w - 10, 12);
            ctx.fillText('2,847 points', w - 10, 26);
            ctx.fillStyle = '#38bdf8';
            ctx.fillText('89 vessels', w - 10, 40);

            animId = requestAnimationFrame(gisAnimation);
        }

        // IAM animation — shield with user nodes
        var iamParticles = [];
        var iamSpawnTimer = 0;

        function iamAnimation() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            var w = canvas.width, h = canvas.height;
            var time = Date.now() * 0.001;

            // Central shield
            var cx = w * 0.5, cy = h * 0.45;
            var shieldSize = Math.min(w, h) * 0.2;

            // Shield glow
            ctx.beginPath();
            ctx.arc(cx, cy, shieldSize + 10 + Math.sin(time * 2) * 5, 0, Math.PI * 2);
            ctx.fillStyle = 'rgba(52, 211, 153, 0.05)';
            ctx.fill();

            // Shield shape
            ctx.beginPath();
            ctx.moveTo(cx, cy - shieldSize);
            ctx.lineTo(cx + shieldSize * 0.8, cy - shieldSize * 0.3);
            ctx.lineTo(cx + shieldSize * 0.8, cy + shieldSize * 0.3);
            ctx.lineTo(cx, cy + shieldSize);
            ctx.lineTo(cx - shieldSize * 0.8, cy + shieldSize * 0.3);
            ctx.lineTo(cx - shieldSize * 0.8, cy - shieldSize * 0.3);
            ctx.closePath();
            ctx.strokeStyle = 'rgba(52, 211, 153, 0.3)';
            ctx.lineWidth = 2;
            ctx.stroke();
            ctx.fillStyle = 'rgba(52, 211, 153, 0.08)';
            ctx.fill();

            // Lock icon in center
            ctx.fillStyle = '#34d399';
            ctx.font = 'bold 14px sans-serif';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText('🔒', cx, cy);

            // User nodes around shield
            var users = [
                { x: w * 0.15, y: h * 0.2, role: 'Admin', color: '#34d399' },
                { x: w * 0.85, y: h * 0.2, role: 'User', color: '#38bdf8' },
                { x: w * 0.1, y: h * 0.7, role: 'Viewer', color: '#a78bfa' },
                { x: w * 0.9, y: h * 0.7, role: 'Editor', color: '#fbbf24' },
                { x: w * 0.3, y: h * 0.9, role: 'Manager', color: '#f472b6' },
                { x: w * 0.7, y: h * 0.9, role: 'SysAdmin', color: '#ef4444' }
            ];

            users.forEach(function(u, i) {
                // Connection line to shield
                var grad = ctx.createLinearGradient(u.x, u.y, cx, cy);
                grad.addColorStop(0, u.color + '30');
                grad.addColorStop(1, 'rgba(52, 211, 153, 0.1)');
                ctx.beginPath();
                ctx.moveTo(u.x, u.y);
                ctx.lineTo(cx, cy);
                ctx.strokeStyle = grad;
                ctx.lineWidth = 1;
                ctx.stroke();

                // User node
                ctx.beginPath();
                ctx.arc(u.x, u.y, 6, 0, Math.PI * 2);
                ctx.fillStyle = u.color + '40';
                ctx.fill();
                ctx.strokeStyle = u.color;
                ctx.lineWidth = 1.5;
                ctx.stroke();

                // Role label
                ctx.fillStyle = u.color;
                ctx.font = 'bold 8px monospace';
                ctx.textAlign = 'center';
                ctx.fillText(u.role, u.x, u.y + 14);
            });

            // Stats in corner
            ctx.textAlign = 'right';
            ctx.textBaseline = 'top';
            ctx.fillStyle = '#64748b';
            ctx.font = 'bold 10px monospace';
            ctx.fillText('3 realms', w - 10, 12);
            ctx.fillText('1,247 users', w - 10, 26);
            ctx.fillStyle = '#a78bfa';
            ctx.fillText('38 roles', w - 10, 40);

            animId = requestAnimationFrame(iamAnimation);
        }

        // Network Intelligence animation — topology with data flow
        var netintelPackets = [];
        var netintelSpawnTimer = 0;

        function netintelAnimation() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            var w = canvas.width, h = canvas.height;
            var time = Date.now() * 0.001;

            // Network nodes
            var nodes = [
                { x: w * 0.5, y: h * 0.3, type: 'core', color: '#34d399', label: 'Core' },
                { x: w * 0.2, y: h * 0.5, type: 'switch', color: '#38bdf8', label: 'Switch' },
                { x: w * 0.8, y: h * 0.5, type: 'switch', color: '#38bdf8', label: 'Switch' },
                { x: w * 0.1, y: h * 0.8, type: 'endpoint', color: '#a78bfa', label: 'Server' },
                { x: w * 0.35, y: h * 0.8, type: 'endpoint', color: '#a78bfa', label: 'DB' },
                { x: w * 0.65, y: h * 0.8, type: 'endpoint', color: '#a78bfa', label: 'Cache' },
                { x: w * 0.9, y: h * 0.8, type: 'endpoint', color: '#a78bfa', label: 'API' }
            ];

            // Draw connections
            var connections = [
                [0, 1], [0, 2], [1, 3], [1, 4], [2, 5], [2, 6]
            ];

            connections.forEach(function(c) {
                var from = nodes[c[0]];
                var to = nodes[c[1]];

                var grad = ctx.createLinearGradient(from.x, from.y, to.x, to.y);
                grad.addColorStop(0, from.color + '20');
                grad.addColorStop(1, to.color + '20');

                ctx.beginPath();
                ctx.moveTo(from.x, from.y);
                ctx.lineTo(to.x, to.y);
                ctx.strokeStyle = grad;
                ctx.lineWidth = 1.5;
                ctx.stroke();
            });

            // Draw nodes
            nodes.forEach(function(n, i) {
                var pulse = Math.sin(time * 2 + i) * 0.3 + 0.7;

                // Glow
                ctx.beginPath();
                ctx.arc(n.x, n.y, 12 + pulse * 4, 0, Math.PI * 2);
                ctx.fillStyle = n.color + '10';
                ctx.fill();

                // Node
                var size = n.type === 'core' ? 8 : n.type === 'switch' ? 6 : 5;
                ctx.beginPath();
                ctx.arc(n.x, n.y, size, 0, Math.PI * 2);
                ctx.fillStyle = n.color + '60';
                ctx.fill();
                ctx.strokeStyle = n.color;
                ctx.lineWidth = 1.5;
                ctx.stroke();

                // Label
                ctx.fillStyle = n.color;
                ctx.font = 'bold 8px monospace';
                ctx.textAlign = 'center';
                ctx.fillText(n.label, n.x, n.y + size + 10);
            });

            // Data packets flowing
            netintelSpawnTimer += 0.016;
            if (netintelSpawnTimer > 0.3) {
                netintelSpawnTimer = 0;
                var conn = connections[Math.floor(Math.random() * connections.length)];
                netintelPackets.push({
                    from: nodes[conn[0]],
                    to: nodes[conn[1]],
                    progress: 0,
                    color: nodes[conn[0]].color
                });
            }

            for (var p = netintelPackets.length - 1; p >= 0; p--) {
                var pkt = netintelPackets[p];
                pkt.progress += 0.02;

                if (pkt.progress >= 1) {
                    netintelPackets.splice(p, 1);
                    continue;
                }

                var px = pkt.from.x + (pkt.to.x - pkt.from.x) * pkt.progress;
                var py = pkt.from.y + (pkt.to.y - pkt.from.y) * pkt.progress;

                ctx.beginPath();
                ctx.arc(px, py, 3, 0, Math.PI * 2);
                ctx.fillStyle = pkt.color + 'cc';
                ctx.fill();
            }

            // Stats in corner
            ctx.textAlign = 'right';
            ctx.textBaseline = 'top';
            ctx.fillStyle = '#64748b';
            ctx.font = 'bold 10px monospace';
            ctx.fillText('24 nodes', w - 10, 12);
            ctx.fillText('2.4ms latency', w - 10, 26);
            ctx.fillStyle = '#ef4444';
            ctx.fillText('3 anomalies', w - 10, 40);

            animId = requestAnimationFrame(netintelAnimation);
        }

        var animations = {
            api: apiAnimation,
            storage: storageAnimation,
            cdc: cdcAnimation,
            scanner: scannerAnimation,
            conductor: conductorAnimation,
            analytics: analyticsAnimation,
            gis: gisAnimation,
            iam: iamAnimation,
            netintel: netintelAnimation
        };

        return {
            start: function() {
                // Ensure proper sizing with a frame delay
                requestAnimationFrame(function() {
                    resize();
                    // If canvas is still 0 size, try again
                    if (canvas.width === 0 || canvas.height === 0) {
                        requestAnimationFrame(function() {
                            resize();
                            if (animations[type]) animations[type]();
                        });
                    } else {
                        if (animations[type]) animations[type]();
                    }
                });
            },
            stop: function() {
                if (animId) {
                    cancelAnimationFrame(animId);
                    animId = null;
                }
                particles = [];
                apiPackets = [];
                apiSpawnTimer = 0;
                storageDropPackets = [];
                cdcPackets = [];
                scanDetected = [];
                conductorPackets = [];
                conductorSpawnTimer = 0;
                analyticsBars = [];
                analyticsSpawnTimer = 0;
                gisMarkers = [];
                gisSpawnTimer = 0;
                iamParticles = [];
                iamSpawnTimer = 0;
                netintelPackets = [];
                netintelSpawnTimer = 0;
                ctx.clearRect(0, 0, canvas.width, canvas.height);
            },
            resize: resize
        };
    }

    // Attach hover events to each card
    hoverCanvases.forEach(function(canvas) {
        var card = canvas.parentElement;
        var anim = initHoverCanvas(canvas);

        card.addEventListener('mouseenter', function() {
            anim.start();
        });

        card.addEventListener('mouseleave', function() {
            anim.stop();
        });
    });

    // Resize handler
    window.addEventListener('resize', function() {
        hoverCanvases.forEach(function(canvas) {
            var rect = canvas.parentElement.getBoundingClientRect();
            canvas.width = rect.width;
            canvas.height = rect.height;
        });
    });

    // ============================================================
    // SCANNER DEMO ROW ANIMATION
    // ============================================================
    var scannerDemo = document.getElementById('scannerDemo');
    if (scannerDemo) {
        var scanObserver = new IntersectionObserver(function(entries) {
            if (entries[0].isIntersecting) {
                var rows = scannerDemo.querySelectorAll('.scanner-demo__row:not(.scanner-demo__row--header)');
                rows.forEach(function(row, i) {
                    row.style.opacity = '0';
                    row.style.transform = 'translateX(-8px)';
                    row.style.transition = 'opacity 0.4s ease, transform 0.4s ease';
                    setTimeout(function() {
                        row.style.opacity = '1';
                        row.style.transform = 'translateX(0)';
                    }, i * 100);
                });
                scanObserver.unobserve(scannerDemo);
            }
        }, { threshold: 0.3 });
        scanObserver.observe(scannerDemo);
    }

    // ============================================================
    // API DEMO TABS
    // ============================================================
    var apiDemoTabs = document.querySelectorAll('.api-demo__tab');
    var apiDemoPanels = document.querySelectorAll('.api-demo__panel');

    apiDemoTabs.forEach(function(tab) {
        tab.addEventListener('click', function(e) {
            e.stopPropagation();
            var targetId = 'demo-' + tab.getAttribute('data-demo');
            apiDemoTabs.forEach(function(t) { t.classList.remove('active'); });
            apiDemoPanels.forEach(function(p) { p.classList.remove('active'); });
            tab.classList.add('active');
            var panel = document.getElementById(targetId);
            if (panel) panel.classList.add('active');
        });
    });

    // Auto-cycle API demo panels
    var demoPanelNames = ['request', 'response', 'endpoints'];
    var demoCycleIdx = 0;
    var demoCycleTimer = null;

    function cycleApiDemo() {
        demoCycleIdx = (demoCycleIdx + 1) % demoPanelNames.length;
        var targetId = 'demo-' + demoPanelNames[demoCycleIdx];
        apiDemoTabs.forEach(function(t) {
            t.classList.toggle('active', t.getAttribute('data-demo') === demoPanelNames[demoCycleIdx]);
        });
        apiDemoPanels.forEach(function(p) { p.classList.remove('active'); });
        var panel = document.getElementById(targetId);
        if (panel) panel.classList.add('active');
    }

    // Start auto-cycle when visible
    var apiDemo = document.getElementById('apiDemo');
    if (apiDemo) {
        var demoObserver = new IntersectionObserver(function(entries) {
            if (entries[0].isIntersecting) {
                demoCycleTimer = setInterval(cycleApiDemo, 3500);
            } else {
                clearInterval(demoCycleTimer);
            }
        }, { threshold: 0.5 });
        demoObserver.observe(apiDemo);

        // Stop auto-cycle on manual click
        apiDemoTabs.forEach(function(tab) {
            tab.addEventListener('click', function() {
                clearInterval(demoCycleTimer);
                demoCycleIdx = demoPanelNames.indexOf(tab.getAttribute('data-demo'));
            });
        });
    }

    // ============================================================
    // STORAGE DEMO BAR ANIMATION
    // ============================================================
    var storageBars = document.querySelectorAll('.storage-demo__bar-fill');
    if (storageBars.length > 0) {
        var storageObserver = new IntersectionObserver(function(entries) {
            entries.forEach(function(entry) {
                if (entry.isIntersecting) {
                    var bars = entry.target.querySelectorAll('.storage-demo__bar-fill');
                    bars.forEach(function(bar, i) {
                        var width = bar.getAttribute('data-width') || '50';
                        setTimeout(function() {
                            bar.style.width = width + '%';
                        }, i * 200);
                    });
                    storageObserver.unobserve(entry.target);
                }
            });
        }, { threshold: 0.3 });
        var storageDemo = document.getElementById('storageDemo');
        if (storageDemo) storageObserver.observe(storageDemo);
    }

    // ---- Particle System ----
    var canvas = document.getElementById('particleCanvas');
    if (canvas) {
        var ctx = canvas.getContext('2d');
        var particles = [];
        var particleCount = 80;
        var connectionDistance = 150;
        var mouseParticle = { x: -1000, y: -1000, radius: 150 };

        function resizeCanvas() {
            canvas.width = canvas.offsetWidth;
            canvas.height = canvas.offsetHeight;
        }
        resizeCanvas();
        window.addEventListener('resize', resizeCanvas);

        function Particle() {
            this.x = Math.random() * canvas.width;
            this.y = Math.random() * canvas.height;
            this.vx = (Math.random() - 0.5) * 0.5;
            this.vy = (Math.random() - 0.5) * 0.5;
            this.radius = Math.random() * 1.5 + 0.5;
            this.opacity = Math.random() * 0.5 + 0.2;
        }

        for (var i = 0; i < particleCount; i++) {
            particles.push(new Particle());
        }

        // Track mouse for particle repulsion
        document.addEventListener('mousemove', function(e) {
            var rect = canvas.getBoundingClientRect();
            mouseParticle.x = e.clientX - rect.left;
            mouseParticle.y = e.clientY - rect.top;
        });

        function animateParticles() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);

            for (var i = 0; i < particles.length; i++) {
                var p = particles[i];

                // Mouse repulsion
                var dx = p.x - mouseParticle.x;
                var dy = p.y - mouseParticle.y;
                var dist = Math.sqrt(dx * dx + dy * dy);
                if (dist < mouseParticle.radius) {
                    var force = (mouseParticle.radius - dist) / mouseParticle.radius;
                    p.vx += (dx / dist) * force * 0.3;
                    p.vy += (dy / dist) * force * 0.3;
                }

                // Damping
                p.vx *= 0.99;
                p.vy *= 0.99;

                p.x += p.vx;
                p.y += p.vy;

                // Wrap around
                if (p.x < -10) p.x = canvas.width + 10;
                if (p.x > canvas.width + 10) p.x = -10;
                if (p.y < -10) p.y = canvas.height + 10;
                if (p.y > canvas.height + 10) p.y = -10;

                // Draw particle with glow
                ctx.beginPath();
                ctx.arc(p.x, p.y, p.radius + 2, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(52, 211, 153, ' + (p.opacity * 0.15) + ')';
                ctx.fill();
                ctx.beginPath();
                ctx.arc(p.x, p.y, p.radius, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(52, 211, 153, ' + p.opacity + ')';
                ctx.fill();

                // Draw connections
                for (var j = i + 1; j < particles.length; j++) {
                    var p2 = particles[j];
                    var dx2 = p.x - p2.x;
                    var dy2 = p.y - p2.y;
                    var dist2 = Math.sqrt(dx2 * dx2 + dy2 * dy2);
                    if (dist2 < connectionDistance) {
                        var alpha = (1 - dist2 / connectionDistance) * 0.15;
                        ctx.beginPath();
                        ctx.moveTo(p.x, p.y);
                        ctx.lineTo(p2.x, p2.y);
                        ctx.strokeStyle = 'rgba(52, 211, 153, ' + alpha + ')';
                        ctx.lineWidth = 0.5;
                        ctx.stroke();
                    }
                }

                // Draw connection to mouse
                if (dist < mouseParticle.radius * 1.5) {
                    var alphaMouse = (1 - dist / (mouseParticle.radius * 1.5)) * 0.2;
                    ctx.beginPath();
                    ctx.moveTo(p.x, p.y);
                    ctx.lineTo(mouseParticle.x, mouseParticle.y);
                    ctx.strokeStyle = 'rgba(56, 189, 248, ' + alphaMouse + ')';
                    ctx.lineWidth = 0.8;
                    ctx.stroke();
                }
            }

            requestAnimationFrame(animateParticles);
        }
        animateParticles();
    }

    // ---- Enhanced Cursor System ----
    var cursorGlow = document.getElementById('cursorGlow');
    var cursorRing = document.getElementById('cursorRing');
    var cursorCrosshair = document.getElementById('cursorCrosshair');
    var cursorTrailCanvas = document.getElementById('cursorTrail');

    var cursorX = 0, cursorY = 0;
    var glowX = 0, glowY = 0;
    var ringX = 0, ringY = 0;
    var crossX = 0, crossY = 0;
    var trailParticles = [];
    var isMouseInDoc = false;

    document.addEventListener('mousemove', function(e) {
        cursorX = e.clientX;
        cursorY = e.clientY;
        isMouseInDoc = true;
        if (cursorGlow) cursorGlow.classList.add('active');
        if (cursorRing) cursorRing.classList.add('active');
        if (cursorCrosshair) cursorCrosshair.classList.add('active');

        // Add trail particle
        trailParticles.push({
            x: cursorX,
            y: cursorY,
            life: 1,
            vx: (Math.random() - 0.5) * 1.5,
            vy: (Math.random() - 0.5) * 1.5,
            size: Math.random() * 2 + 1
        });
        if (trailParticles.length > 50) trailParticles.shift();
    });

    document.addEventListener('mouseleave', function() {
        isMouseInDoc = false;
        if (cursorGlow) cursorGlow.classList.remove('active');
        if (cursorRing) cursorRing.classList.remove('active');
        if (cursorCrosshair) cursorCrosshair.classList.remove('active');
    });

    // Click ripple effect
    document.addEventListener('mousedown', function(e) {
        if (cursorRing) cursorRing.classList.add('clicking');
        // Create ripple
        var ripple = document.createElement('div');
        ripple.className = 'cursor-ripple';
        ripple.style.left = e.clientX + 'px';
        ripple.style.top = e.clientY + 'px';
        document.body.appendChild(ripple);
        setTimeout(function() { ripple.remove(); }, 600);
        // Burst particles
        for (var b = 0; b < 8; b++) {
            var angle = (Math.PI * 2 / 8) * b;
            trailParticles.push({
                x: e.clientX,
                y: e.clientY,
                vx: Math.cos(angle) * 3.5,
                vy: Math.sin(angle) * 3.5,
                life: 1,
                size: Math.random() * 2.5 + 1.5
            });
        }
    });
    document.addEventListener('mouseup', function() {
        if (cursorRing) cursorRing.classList.remove('clicking');
    });

    // Detect hoverable elements for ring expansion + particle burst
    var lastHoverTarget = null;
    document.addEventListener('mouseover', function(e) {
        var target = e.target;
        if (target.closest('a, button, [data-tilt], .search-trigger, .deep__tab, .bento__card, .arch__card, .search-result, .terminal__autocomplete-item, .api-demo__tab')) {
            if (cursorRing) cursorRing.classList.add('hovering');
            // Mini particle burst on hover enter
            var el = target.closest('a, button, [data-tilt], .search-trigger, .deep__tab, .bento__card, .arch__card, .search-result, .terminal__autocomplete-item, .api-demo__tab');
            if (el !== lastHoverTarget) {
                lastHoverTarget = el;
                for (var h = 0; h < 5; h++) {
                    trailParticles.push({
                        x: cursorX + (Math.random() - 0.5) * 20,
                        y: cursorY + (Math.random() - 0.5) * 20,
                        vx: (Math.random() - 0.5) * 2,
                        vy: (Math.random() - 0.5) * 2,
                        life: 0.8,
                        size: Math.random() * 2 + 1
                    });
                }
            }
        }
    });
    document.addEventListener('mouseout', function(e) {
        var target = e.target;
        if (target.closest('a, button, [data-tilt], .search-trigger, .deep__tab, .bento__card, .arch__card, .search-result, .terminal__autocomplete-item, .api-demo__tab')) {
            if (cursorRing) cursorRing.classList.remove('hovering');
            lastHoverTarget = null;
        }
    });

    // Trail canvas setup
    if (cursorTrailCanvas) {
        var trailCtx = cursorTrailCanvas.getContext('2d');
        function resizeTrailCanvas() {
            cursorTrailCanvas.width = window.innerWidth;
            cursorTrailCanvas.height = window.innerHeight;
        }
        resizeTrailCanvas();
        window.addEventListener('resize', resizeTrailCanvas);
    }

    function animateCursorSystem() {
        // Smooth follow for glow
        glowX += (cursorX - glowX) * 0.06;
        glowY += (cursorY - glowY) * 0.06;
        if (cursorGlow) {
            cursorGlow.style.left = glowX + 'px';
            cursorGlow.style.top = glowY + 'px';
        }

        // Ring follows with more delay
        ringX += (cursorX - ringX) * 0.15;
        ringY += (cursorY - ringY) * 0.15;
        if (cursorRing) {
            cursorRing.style.left = ringX + 'px';
            cursorRing.style.top = ringY + 'px';
        }

        // Crosshair follows with medium delay
        crossX += (cursorX - crossX) * 0.1;
        crossY += (cursorY - crossY) * 0.1;
        if (cursorCrosshair) {
            cursorCrosshair.style.left = crossX + 'px';
            cursorCrosshair.style.top = crossY + 'px';
        }

        // Draw trail particles with glow and flowing line
        if (cursorTrailCanvas && trailCtx) {
            trailCtx.clearRect(0, 0, cursorTrailCanvas.width, cursorTrailCanvas.height);

            // Draw flowing cursor trail line
            if (trailParticles.length > 3 && isMouseInDoc) {
                var recent = trailParticles.slice(-15);
                trailCtx.beginPath();
                trailCtx.moveTo(recent[0].x, recent[0].y);
                for (var t = 1; t < recent.length - 1; t++) {
                    var cpx = (recent[t].x + recent[t + 1].x) / 2;
                    var cpy = (recent[t].y + recent[t + 1].y) / 2;
                    trailCtx.quadraticCurveTo(recent[t].x, recent[t].y, cpx, cpy);
                }
                var lastP = recent[recent.length - 1];
                trailCtx.lineTo(lastP.x, lastP.y);
                trailCtx.strokeStyle = 'rgba(52, 211, 153, 0.12)';
                trailCtx.lineWidth = 2;
                trailCtx.stroke();
            }
            for (var i = trailParticles.length - 1; i >= 0; i--) {
                var p = trailParticles[i];
                p.x += p.vx;
                p.y += p.vy;
                p.vx *= 0.98;
                p.vy *= 0.98;
                p.life -= 0.02;
                if (p.life <= 0) {
                    trailParticles.splice(i, 1);
                    continue;
                }
                trailCtx.beginPath();
                trailCtx.arc(p.x, p.y, p.size * p.life, 0, Math.PI * 2);
                trailCtx.fillStyle = 'rgba(52, 211, 153, ' + (p.life * 0.4) + ')';
                trailCtx.fill();
            }

            // Draw connections between nearby trail particles
            for (var j = 0; j < trailParticles.length; j++) {
                for (var k = j + 1; k < trailParticles.length; k++) {
                    var a = trailParticles[j];
                    var b = trailParticles[k];
                    var dx = a.x - b.x;
                    var dy = a.y - b.y;
                    var dist = Math.sqrt(dx * dx + dy * dy);
                    if (dist < 40) {
                        var alpha = (1 - dist / 40) * a.life * b.life * 0.2;
                        trailCtx.beginPath();
                        trailCtx.moveTo(a.x, a.y);
                        trailCtx.lineTo(b.x, b.y);
                        trailCtx.strokeStyle = 'rgba(52, 211, 153, ' + alpha + ')';
                        trailCtx.lineWidth = 0.5;
                        trailCtx.stroke();
                    }
                }
            }
        }

        requestAnimationFrame(animateCursorSystem);
    }
    animateCursorSystem();

    // ---- Scroll Progress Bar ----
    var scrollProgressBar = document.getElementById('scrollProgressBar');
    if (scrollProgressBar) {
        var ticking = false;
        window.addEventListener('scroll', function() {
            if (!ticking) {
                requestAnimationFrame(function() {
                    var scrollTop = window.pageYOffset || document.documentElement.scrollTop;
                    var docHeight = document.documentElement.scrollHeight - window.innerHeight;
                    var scrollPercent = (scrollTop / docHeight) * 100;
                    scrollProgressBar.style.width = scrollPercent + '%';
                    ticking = false;
                });
                ticking = true;
            }
        });
    }

    // ---- Back to Top ----
    var backToTopBtn = document.getElementById('backToTop');
    if (backToTopBtn) {
        var bttTicking = false;
        window.addEventListener('scroll', function() {
            if (!bttTicking) {
                requestAnimationFrame(function() {
                    var scrollTop = window.pageYOffset || document.documentElement.scrollTop;
                    var docHeight = document.documentElement.scrollHeight - window.innerHeight;
                    // Show when scrolled past 70% of the page
                    if (scrollTop > docHeight * 0.7) {
                        backToTopBtn.classList.add('is-visible');
                    } else {
                        backToTopBtn.classList.remove('is-visible');
                    }
                    bttTicking = false;
                });
                bttTicking = true;
            }
        });
        backToTopBtn.addEventListener('click', function() {
            window.scrollTo({ top: 0, behavior: 'smooth' });
        });
    }

    // ---- Scroll Reveal (IntersectionObserver) ----
    var revealElements = document.querySelectorAll('[data-reveal]');
    if (revealElements.length > 0) {
        var revealObserver = new IntersectionObserver(function(entries) {
            entries.forEach(function(entry) {
                if (entry.isIntersecting) {
                    var parent = entry.target.parentElement;
                    var siblings = parent ? parent.querySelectorAll('[data-reveal]') : [];
                    var delay = 0;
                    for (var i = 0; i < siblings.length; i++) {
                        if (siblings[i] === entry.target) {
                            delay = i * 60;
                            break;
                        }
                    }

                    // Get animation type from data attribute
                    var animType = entry.target.getAttribute('data-reveal-type') || 'default';

                    setTimeout(function() {
                        entry.target.classList.add('revealed');
                        entry.target.classList.add('revealed--' + animType);
                    }, delay);

                    revealObserver.unobserve(entry.target);
                }
            });
        }, { threshold: 0.1, rootMargin: '0px 0px -40px 0px' });

        revealElements.forEach(function(el) {
            revealObserver.observe(el);
        });
    }

    // ---- Animated Counters ----
    var counterElements = document.querySelectorAll('[data-count]');
    var counterAnimated = false;

    function formatNumber(num) {
        return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
    }

    function animateCounters() {
        if (counterAnimated) return;
        counterAnimated = true;

        counterElements.forEach(function(el, index) {
            var target = parseInt(el.getAttribute('data-count'), 10);
            var suffix = el.getAttribute('data-suffix') || '';
            if (isNaN(target)) return;

            var duration = 2000 + (index * 200); // Stagger each counter
            var startTime = null;

            // Add counting class for visual effect
            el.classList.add('counting');

            function step(timestamp) {
                if (!startTime) startTime = timestamp;
                var progress = Math.min((timestamp - startTime) / duration, 1);
                // Ease out cubic
                var eased = 1 - Math.pow(1 - progress, 3);
                var current = Math.floor(eased * target);

                el.textContent = formatNumber(current) + (progress >= 1 ? suffix : '');

                if (progress < 1) {
                    requestAnimationFrame(step);
                } else {
                    el.textContent = formatNumber(target) + suffix;
                    el.classList.remove('counting');
                    el.classList.add('counted');
                }
            }

            // Stagger start of each counter
            setTimeout(function() {
                requestAnimationFrame(step);
            }, index * 150);
        });
    }

    var heroStats = document.querySelector('.hero__stats');
    if (heroStats) {
        var statsObserver = new IntersectionObserver(function(entries) {
            if (entries[0].isIntersecting) {
                animateCounters();
                statsObserver.unobserve(heroStats);
            }
        }, { threshold: 0.5 });
        statsObserver.observe(heroStats);
    }

    // ---- 3D Tilt Effect on Cards with Glare ----
    var tiltCards = document.querySelectorAll('[data-tilt]');
    tiltCards.forEach(function(card) {
        // Create glare element if not exists
        var glare = card.querySelector('.bento__card-glare');
        if (!glare) {
            glare = document.createElement('div');
            glare.className = 'bento__card-glare';
            card.appendChild(glare);
        }

        card.addEventListener('mousemove', function(e) {
            var rect = card.getBoundingClientRect();
            var x = e.clientX - rect.left;
            var y = e.clientY - rect.top;
            var centerX = rect.width / 2;
            var centerY = rect.height / 2;
            var rotateX = ((y - centerY) / centerY) * -5;
            var rotateY = ((x - centerX) / centerX) * 5;
            card.style.transform = 'perspective(800px) rotateX(' + rotateX + 'deg) rotateY(' + rotateY + 'deg) translateY(-6px)';

            // Move spotlight
            var spotlight = card.querySelector('.bento__card-spotlight');
            if (spotlight) {
                spotlight.style.left = x + 'px';
                spotlight.style.top = y + 'px';
            }

            // Update glare effect
            var glareX = (x / rect.width) * 100;
            var glareY = (y / rect.height) * 100;
            glare.style.background = 'radial-gradient(circle at ' + glareX + '% ' + glareY + '%, rgba(255, 255, 255, 0.08), transparent 60%)';
            glare.style.opacity = '1';
        });

        card.addEventListener('mouseleave', function() {
            card.style.transform = '';
            glare.style.opacity = '0';
        });
    });

    // ---- Magnetic Hover on Arch Cards ----
    var magneticCards = document.querySelectorAll('.arch__card');
    magneticCards.forEach(function(card) {
        card.setAttribute('data-magnetic', '');
        card.addEventListener('mousemove', function(e) {
            var rect = card.getBoundingClientRect();
            var x = e.clientX - rect.left;
            var y = e.clientY - rect.top;
            card.style.setProperty('--mouse-x', x + 'px');
            card.style.setProperty('--mouse-y', y + 'px');

            // Subtle magnetic pull
            var centerX = rect.width / 2;
            var centerY = rect.height / 2;
            var pullX = ((x - centerX) / centerX) * 4;
            var pullY = ((y - centerY) / centerY) * 4;
            card.style.transform = 'translateY(-6px) translate(' + pullX + 'px, ' + pullY + 'px)';
        });
        card.addEventListener('mouseleave', function() {
            card.style.transform = '';
        });
    });

    // ---- Deep Feature Tabs ----
    var deepTabs = document.querySelectorAll('.deep__tab');
    var deepPanels = document.querySelectorAll('.deep__panel');
    var deepSection = document.getElementById('deep-features');
    var deepAnimated = false;

    // Staggered animation for items in a panel
    function animateDeepPanel(panel) {
        var items = panel.querySelectorAll('.deep__item');

        // Reset items
        panel.classList.add('animating-in');
        items.forEach(function(item) {
            item.classList.remove('animate-in');
        });

        // Stagger animate each item
        items.forEach(function(item, index) {
            setTimeout(function() {
                item.classList.add('animate-in');
            }, index * 80 + 100);
        });

        // Clean up after all animations
        setTimeout(function() {
            panel.classList.remove('animating-in');
        }, items.length * 80 + 700);
    }

    // Tab click handler with animations
    deepTabs.forEach(function(tab) {
        tab.addEventListener('click', function() {
            var targetId = 'panel-' + tab.getAttribute('data-tab');

            // Don't re-animate if clicking the same tab
            if (tab.classList.contains('active')) return;

            deepTabs.forEach(function(t) { t.classList.remove('active'); });
            deepPanels.forEach(function(p) { p.classList.remove('active'); });

            tab.classList.add('active');
            var targetPanel = document.getElementById(targetId);
            if (targetPanel) {
                targetPanel.classList.add('active');
                animateDeepPanel(targetPanel);
            }

            // Ripple effect on tab
            var ripple = document.createElement('span');
            ripple.style.cssText = 'position:absolute;inset:0;background:radial-gradient(circle at center,rgba(52,211,153,0.2),transparent 70%);pointer-events:none;border-radius:inherit;';
            tab.style.position = 'relative';
            tab.style.overflow = 'hidden';
            tab.appendChild(ripple);
            setTimeout(function() { ripple.remove(); }, 600);
        });
    });

    // ---- Deep Section Scroll-Triggered Animation ----
    if (deepSection) {
        var deepScrollObserver = new IntersectionObserver(function(entries) {
            entries.forEach(function(entry) {
                if (entry.isIntersecting && !deepAnimated) {
                    deepAnimated = true;

                    // Animate the header
                    var header = deepSection.querySelector('.deep__header');
                    if (header) {
                        header.style.opacity = '0';
                        header.style.transform = 'translateY(20px)';
                        setTimeout(function() {
                            header.style.transition = 'all 0.6s cubic-bezier(0.16, 1, 0.3, 1)';
                            header.style.opacity = '1';
                            header.style.transform = 'translateY(0)';
                        }, 100);
                    }

                    // Animate the tabs with stagger
                    var tabs = deepSection.querySelector('.deep__tabs');
                    if (tabs) {
                        tabs.style.opacity = '0';
                        tabs.style.transform = 'translateY(20px)';
                        setTimeout(function() {
                            tabs.style.transition = 'all 0.6s cubic-bezier(0.16, 1, 0.3, 1)';
                            tabs.style.opacity = '1';
                            tabs.style.transform = 'translateY(0)';
                        }, 300);
                    }

                    // Animate the active panel items
                    var activePanel = deepSection.querySelector('.deep__panel.active');
                    if (activePanel) {
                        setTimeout(function() {
                            animateDeepPanel(activePanel);
                        }, 500);
                    }

                    deepScrollObserver.unobserve(entry.target);
                }
            });
        }, { threshold: 0.15 });

        deepScrollObserver.observe(deepSection);
    }

    // ---- Data Visualization Bar Animation ----
    var analyticsBars = document.querySelectorAll('.analytics-demo__bar');
    if (analyticsBars.length > 0) {
        var barsObserver = new IntersectionObserver(function(entries) {
            entries.forEach(function(entry) {
                if (entry.isIntersecting) {
                    var bars = entry.target.querySelectorAll('.analytics-demo__bar');
                    bars.forEach(function(bar, index) {
                        setTimeout(function() {
                            bar.classList.add('animate');
                        }, index * 100);
                    });
                    barsObserver.unobserve(entry.target);
                }
            });
        }, { threshold: 0.3 });

        var barsContainer = document.querySelector('.analytics-demo__bars');
        if (barsContainer) {
            barsObserver.observe(barsContainer);
        }
    }

    // ---- Text Scramble Effect ----
    var scrambleChars = '!@#$%^&*()_+-=[]{}|;:,.<>?/~`ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';

    function scrambleText(element, finalText, duration, callback) {
        var length = finalText.length;
        var frame = 0;
        var totalFrames = Math.floor(duration / 30); // ~30ms per frame

        function update() {
            var progress = frame / totalFrames;
            var revealed = Math.floor(progress * length);
            var text = '';

            for (var i = 0; i < length; i++) {
                if (i < revealed) {
                    text += finalText[i];
                } else if (finalText[i] === ' ') {
                    text += ' ';
                } else {
                    text += scrambleChars[Math.floor(Math.random() * scrambleChars.length)];
                }
            }

            element.textContent = text;
            frame++;

            if (frame <= totalFrames) {
                requestAnimationFrame(update);
            } else {
                element.textContent = finalText;
                if (callback) callback();
            }
        }

        requestAnimationFrame(update);
    }

    // Apply scramble to section titles on scroll
    var scrambleElements = document.querySelectorAll('[data-scramble]');
    if (scrambleElements.length > 0) {
        var scrambleObserver = new IntersectionObserver(function(entries) {
            entries.forEach(function(entry) {
                if (entry.isIntersecting) {
                    var el = entry.target;
                    var text = el.getAttribute('data-scramble') || el.textContent;
                    el.setAttribute('data-scramble', text);
                    scrambleText(el, text, 800);
                    scrambleObserver.unobserve(el);
                }
            });
        }, { threshold: 0.5 });

        scrambleElements.forEach(function(el) {
            el.setAttribute('data-scramble', el.textContent);
            scrambleObserver.observe(el);
        });
    }

    // ---- Section Blur on Scroll ----
    var sections = document.querySelectorAll('.bento, .deep, .arch, .api-lifecycle, .cli');
    if (sections.length > 0) {
        var blurObserver = new IntersectionObserver(function(entries) {
            entries.forEach(function(entry) {
                if (entry.isIntersecting) {
                    entry.target.classList.remove('section-blur');
                } else {
                    // Only blur if section is far from viewport
                    var rect = entry.boundingClientRect;
                    var viewportHeight = window.innerHeight;
                    if (rect.bottom < -viewportHeight * 0.5 || rect.top > viewportHeight * 1.5) {
                        entry.target.classList.add('section-blur');
                    }
                }
            });
        }, { threshold: 0.1, rootMargin: '100px 0px' });

        sections.forEach(function(section) {
            blurObserver.observe(section);
        });
    }

    // ---- Parallax on Scroll ----
    var parallaxElements = document.querySelectorAll('[data-parallax]');
    if (parallaxElements.length > 0) {
        var ticking = false;
        window.addEventListener('scroll', function() {
            if (!ticking) {
                requestAnimationFrame(function() {
                    var scrollY = window.pageYOffset;
                    parallaxElements.forEach(function(el) {
                        var speed = parseFloat(el.getAttribute('data-parallax')) || 0.3;
                        var rect = el.getBoundingClientRect();
                        var offset = (rect.top + scrollY) * speed;
                        el.style.transform = 'translateY(' + (scrollY * speed - offset) + 'px)';
                    });
                    ticking = false;
                });
                ticking = true;
            }
        });
    }

    // ---- Hero Parallax Orbs (mouse-reactive) ----
    var heroSection = document.getElementById('hero');
    var heroOrbs = document.querySelectorAll('.hero__orb');
    var heroLogoBg = document.getElementById('heroLogoBg');
    var heroLogoSvg = document.getElementById('heroLogoSvg');

    if (heroSection) {
        heroSection.addEventListener('mousemove', function(e) {
            var rect = heroSection.getBoundingClientRect();
            var x = (e.clientX - rect.left) / rect.width - 0.5;
            var y = (e.clientY - rect.top) / rect.height - 0.5;

            // Orbs react to mouse
            heroOrbs.forEach(function(orb, i) {
                var depth = (i + 1) * 8;
                orb.style.transform = 'translate(' + (x * depth) + 'px, ' + (y * depth) + 'px)';
            });

            // Logo subtle mouse follow
            if (heroLogoBg) {
                heroLogoBg.style.transform = 'translate(calc(-50% + ' + (x * 15) + 'px), calc(-50% + ' + (y * 15) + 'px))';
            }
        });

        // Logo parallax on scroll — fades as you scroll
        var logoParallaxTicking = false;
        window.addEventListener('scroll', function() {
            if (!logoParallaxTicking) {
                requestAnimationFrame(function() {
                    var scrollY = window.pageYOffset;
                    var heroH = heroSection.offsetHeight;
                    var progress = Math.min(scrollY / heroH, 1);
                    if (heroLogoBg) {
                        heroLogoBg.style.opacity = Math.max(1 - progress * 1.5, 0);
                    }
                    logoParallaxTicking = false;
                });
                logoParallaxTicking = true;
            }
        });
    }

    // ============================================================
    // LEGO ASSEMBLY / DISASSEMBLY ANIMATION
    // ============================================================
    var logoPieces = document.querySelectorAll('.logo-piece');
    var legoState = 'assembled'; // assembled | disassembled
    var legoTimer = null;

    function legoDisassemble() {
        legoState = 'disassembled';
        logoPieces.forEach(function(piece, i) {
            var fromX = parseFloat(piece.getAttribute('data-from-x')) || 0;
            var fromY = parseFloat(piece.getAttribute('data-from-y')) || 0;
            var fromR = parseFloat(piece.getAttribute('data-from-r')) || 0;
            var fromS = parseFloat(piece.getAttribute('data-from-s')) || 0.5;

            setTimeout(function() {
                piece.style.transition = 'transform 0.7s cubic-bezier(0.55, -0.6, 0.27, 1.4), opacity 0.5s ease';
                piece.style.transform = 'translate(' + fromX + 'px, ' + fromY + 'px) rotate(' + fromR + 'deg) scale(' + fromS + ')';
                piece.style.opacity = '0.4';
            }, i * 60);
        });

        legoTimer = setTimeout(legoAssemble, 2000);
    }

    function legoAssemble() {
        legoState = 'assembled';
        logoPieces.forEach(function(piece, i) {
            setTimeout(function() {
                // Overshoot scale for a "snap" feel, then settle
                piece.style.transition = 'transform 0.9s cubic-bezier(0.34, 1.56, 0.64, 1), opacity 0.6s ease';
                piece.style.transform = 'translate(0, 0) rotate(0deg) scale(1.08)';
                piece.style.opacity = '1';
                // Settle to 1.0 scale after overshoot
                setTimeout(function() {
                    piece.style.transition = 'transform 0.3s ease-out';
                    piece.style.transform = 'translate(0, 0) rotate(0deg) scale(1)';
                }, 500);
            }, i * 100);
        });

        legoTimer = setTimeout(legoDisassemble, 4000);
    }

    // Start the cycle when hero is visible
    if (logoPieces.length > 0) {
        var legoObserver = new IntersectionObserver(function(entries) {
            if (entries[0].isIntersecting) {
                // Initial assembly animation — pieces fly in from scattered positions
                logoPieces.forEach(function(piece) {
                    var fromX = parseFloat(piece.getAttribute('data-from-x')) || 0;
                    var fromY = parseFloat(piece.getAttribute('data-from-y')) || 0;
                    var fromR = parseFloat(piece.getAttribute('data-from-r')) || 0;
                    var fromS = parseFloat(piece.getAttribute('data-from-s')) || 0.5;
                    piece.style.transform = 'translate(' + fromX + 'px, ' + fromY + 'px) rotate(' + fromR + 'deg) scale(' + fromS + ')';
                    piece.style.opacity = '0';
                });

                // Animate in after a short delay
                setTimeout(legoAssemble, 800);
                legoObserver.unobserve(heroSection);
            }
        }, { threshold: 0.3 });
        legoObserver.observe(heroSection);
    }

    // ---- Terminal Typing Effect ----
    var terminalBody = document.querySelector('.terminal__body');
    if (terminalBody) {
        var lines = terminalBody.querySelectorAll('.terminal__line');
        var termObserver = new IntersectionObserver(function(entries) {
            if (entries[0].isIntersecting) {
                lines.forEach(function(line, i) {
                    line.style.opacity = '0';
                    line.style.transform = 'translateX(-10px)';
                    line.style.transition = 'opacity 0.4s ease, transform 0.4s ease';
                    setTimeout(function() {
                        line.style.opacity = '1';
                        line.style.transform = 'translateX(0)';
                    }, i * 120);
                });
                termObserver.unobserve(terminalBody);
            }
        }, { threshold: 0.3 });
        termObserver.observe(terminalBody);
    }

    // ---- Hero Platform Status ----
    var statusEl = document.getElementById('heroStatus');
    if (statusEl) {
        var backendURL = (document.getElementById('backendURL') || {}).textContent || '';
        backendURL = backendURL.trim();
        if (!backendURL) {
            backendURL = window.BACKEND_URL || 'http://localhost:8000';
        }
        if (backendURL.length > 1 && backendURL.charAt(backendURL.length - 1) === '/') {
            backendURL = backendURL.slice(0, -1);
        }

        fetch(backendURL + '/health', { signal: AbortSignal.timeout ? AbortSignal.timeout(5000) : undefined })
            .then(function(r) { return r.json(); })
            .then(function() {
                statusEl.innerHTML = '<span class="pulse-dot pulse-dot--ok"></span>';
            })
            .catch(function() {
                statusEl.innerHTML = '<span class="pulse-dot" style="background:#ef4444"></span>';
            });
    }

})();
