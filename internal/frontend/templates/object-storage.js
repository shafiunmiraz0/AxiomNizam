(function() {
    'use strict';

    // ===== API Configuration =====
    const OS_API = (() => {
        let base = (typeof window.resolveBackendURL === 'function') ?
            window.resolveBackendURL() : String(window.BACKEND_URL || '').trim();
        if (!base) {
            const bh = String(window.location.hostname || '').toLowerCase();
            if (bh && bh !== 'localhost' && bh !== '127.0.0.1') {
                const proto = window.location.protocol || 'https:';
                base = bh.indexOf('axiomnizam.') === 0
                    ? proto + '//axiomnizam-platform.' + bh.substring('axiomnizam.'.length)
                    : proto + '//' + bh;
            } else {
                base = 'http://localhost:8000';
            }
        }
        if (base.endsWith('/')) base = base.slice(0, -1);
        return base + '/api/v1/storage';
    })();

    function osHeaders() {
        const h = { 'Content-Type': 'application/json' };
        const tok = localStorage.getItem('iamToken') || localStorage.getItem('authToken') ||
                    (document.cookie.match(/(?:^|;\s*)authToken=([^;]*)/) || [])[1] || '';
        if (tok) h['Authorization'] = 'Bearer ' + tok;
        return h;
    }

    async function osFetch(path, opts) {
        opts = opts || {};
        if (!opts.headers) opts.headers = osHeaders();
        try {
            return await fetch(OS_API + path, opts);
        } catch (e) {
            osToast('Network error: ' + e.message, true);
            throw e;
        }
    }

    // ===== Utilities =====
    function escHtml(s) { const d=document.createElement('div'); d.textContent=s||''; return d.innerHTML; }

    function fmtSize(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        const k = 1024, units = ['B','KB','MB','GB','TB','PB'];
        const i = Math.floor(Math.log(bytes)/Math.log(k));
        return (bytes / Math.pow(k, i)).toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
    }

    function fmtDate(s) {
        if (!s) return '-';
        const d = new Date(s);
        return isNaN(d.getTime()) ? s : d.toLocaleString();
    }

    function osIntOrNull(v) {
        const n = parseInt(v, 10);
        return isNaN(n) ? null : n;
    }

    function osFullURL(path) {
        return OS_API + path;
    }

    const osBucketHoverCacheTtlMs = 10000;
    const osBucketHoverMetricsCache = {};
    const osBucketLiveRateState = {};
    let osBucketHoverRequestToken = 0;
    let osBucketHoverHideTimer = null;

    function osBucketMetricsKey(bucket, tenantId) {
        return String(tenantId || 'default').trim() + '|' + String(bucket || '').trim();
    }

    function osNormalizeName(v) {
        return String(v || '').trim().toLowerCase();
    }

    function osMethodToOpClass(method) {
        const m = String(method || '').toUpperCase();
        if (m === 'PUT' || m === 'POST' || m === 'DELETE' || m === 'PATCH') {
            return 'write';
        }
        return 'read';
    }

    function osFormatRatePerMinute(v, showDefaultLabel) {
        const n = Number(v || 0);
        if (!n || n <= 0) return showDefaultLabel ? 'default' : '-';
        return n + '/min';
    }

    function osFormatEpochSeconds(epochSeconds) {
        const n = Number(epochSeconds || 0);
        if (!n || n <= 0) return '-';
        const d = new Date(n * 1000);
        return isNaN(d.getTime()) ? '-' : d.toLocaleTimeString();
    }

    function osStoreBucketRateHeaderState(bucket, tenantId, method, limit, remaining, resetEpoch) {
        if (limit === null || remaining === null) return;
        const key = osBucketMetricsKey(bucket, tenantId);
        const opClass = osMethodToOpClass(method);
        const current = osBucketLiveRateState[key] || {};
        current[opClass] = {
            limit: limit,
            remaining: remaining,
            resetEpoch: resetEpoch,
            updatedAt: Date.now()
        };
        osBucketLiveRateState[key] = current;
    }

    function osCaptureRateHeaders(bucket, tenantId, method, resp) {
        if (!resp || !resp.headers) return;
        const limit = osIntOrNull(resp.headers.get('X-RateLimit-Limit'));
        const remaining = osIntOrNull(resp.headers.get('X-RateLimit-Remaining'));
        const resetEpoch = osIntOrNull(resp.headers.get('X-RateLimit-Reset'));
        osStoreBucketRateHeaderState(bucket, tenantId, method, limit, remaining, resetEpoch);
    }

    function osCaptureRateHeadersFromXHR(bucket, tenantId, method, xhr) {
        if (!xhr || typeof xhr.getResponseHeader !== 'function') return;
        const limit = osIntOrNull(xhr.getResponseHeader('X-RateLimit-Limit'));
        const remaining = osIntOrNull(xhr.getResponseHeader('X-RateLimit-Remaining'));
        const resetEpoch = osIntOrNull(xhr.getResponseHeader('X-RateLimit-Reset'));
        osStoreBucketRateHeaderState(bucket, tenantId, method, limit, remaining, resetEpoch);
    }

    function osRefreshBucketNameSuggestions() {
        const datalist = document.getElementById('osBucketNameSuggestions');
        if (!datalist) return;

        const source = (osBucketCatalogCache && osBucketCatalogCache.length) ? osBucketCatalogCache : osBucketsCache;
        const rows = (source || []).map(b => {
            return {
                name: (b && b.metadata && b.metadata.name) ? String(b.metadata.name) : '',
                tenantId: (b && b.metadata && b.metadata.tenantId) ? String(b.metadata.tenantId) : ''
            };
        }).filter(r => !!r.name);

        rows.sort((a, b) => {
            const byName = a.name.localeCompare(b.name);
            if (byName !== 0) return byName;
            return a.tenantId.localeCompare(b.tenantId);
        });

        const seen = {};
        let html = '';
        rows.forEach(r => {
            const key = osNormalizeName(r.name) + '|' + osNormalizeName(r.tenantId);
            if (seen[key]) return;
            seen[key] = true;

            const label = r.tenantId ? (r.name + ' (tenant: ' + r.tenantId + ')') : r.name;
            html += '<option value="' + escHtml(r.name) + '" label="' + escHtml(label) + '"></option>';
        });
        datalist.innerHTML = html;
    }

    function osRenderAccessKeyScopeSuggestions() {
        const host = document.getElementById('osAccessKeyScopeSuggestions');
        if (!host) return;

        const names = [];
        const seen = {};
        const source = (osBucketCatalogCache && osBucketCatalogCache.length) ? osBucketCatalogCache : osBucketsCache;
        (source || []).forEach(b => {
            const name = (b && b.metadata && b.metadata.name) ? String(b.metadata.name).trim() : '';
            if (!name) return;
            const k = osNormalizeName(name);
            if (seen[k]) return;
            seen[k] = true;
            names.push(name);
        });

        if (!names.length) {
            host.innerHTML = '<span style="color:var(--text-muted);">No bucket suggestions available yet</span>';
            return;
        }

        names.sort((a, b) => a.localeCompare(b));
        host.innerHTML = names.map(name => {
            return '<button type="button" class="os-btn os-btn-secondary os-btn-sm" style="padding:3px 8px;" data-bucket="' + escHtml(name) + '" onclick="osAddBucketScopeSuggestion(this.dataset.bucket)">' + escHtml(name) + '</button>';
        }).join(' ');
    }

    window.osAddBucketScopeSuggestion = function(bucketName) {
        const input = document.getElementById('osNewAccessKeyBucketScope');
        if (!input) return;

        const add = String(bucketName || '').trim();
        if (!add) return;

        const current = String(input.value || '');
        const parts = current.split(',').map(s => s.trim()).filter(Boolean);
        const lower = parts.map(osNormalizeName);
        if (lower.indexOf(osNormalizeName(add)) === -1) {
            parts.push(add);
        }
        input.value = parts.join(', ');
        input.focus();
    };

    function osFindDuplicateBucket(name, tenant) {
        const n = osNormalizeName(name);
        const t = osNormalizeName(tenant || 'default');
        if (!n) return null;

        const source = (osBucketCatalogCache && osBucketCatalogCache.length) ? osBucketCatalogCache : osBucketsCache;
        let sameNameAnyTenant = null;
        for (let i = 0; i < (source || []).length; i++) {
            const b = source[i] || {};
            const m = b.metadata || {};
            const bn = osNormalizeName(m.name || '');
            const bt = osNormalizeName(m.tenantId || 'default');
            if (!bn || bn !== n) continue;

            if (!sameNameAnyTenant) {
                sameNameAnyTenant = { name: m.name || name, tenantId: m.tenantId || 'default' };
            }
            if (bt === t) {
                return { name: m.name || name, tenantId: m.tenantId || 'default', exactTenant: true };
            }
        }

        if (sameNameAnyTenant) {
            return { name: sameNameAnyTenant.name, tenantId: sameNameAnyTenant.tenantId, exactTenant: false };
        }
        return null;
    }

    function osUpdateCreateBucketNameHint(duplicateInfo) {
        const hint = document.getElementById('osNewBucketNameHint');
        if (!hint) return;

        if (!duplicateInfo) {
            hint.style.color = 'var(--text-muted)';
            hint.textContent = 'Suggestions show existing bucket names. Duplicate bucket name is blocked.';
            return;
        }

        hint.style.color = '#f59e0b';
        if (duplicateInfo.exactTenant) {
            hint.textContent = 'This bucket already exists for tenant "' + duplicateInfo.tenantId + '".';
            return;
        }
        hint.textContent = 'A bucket with this name already exists under tenant "' + duplicateInfo.tenantId + '".';
    }

    window.osValidateCreateBucketName = function(showToast) {
        const name = (document.getElementById('osNewBucketName').value || '').trim();
        const tenant = (document.getElementById('osNewBucketTenant').value || 'default').trim() || 'default';
        const dup = osFindDuplicateBucket(name, tenant);

        osUpdateCreateBucketNameHint(dup);
        if (!dup) return true;

        if (showToast) {
            osToast('Bucket "' + name + '" already exists' + (dup.tenantId ? (' (tenant: ' + dup.tenantId + ')') : ''), true);
        }
        return false;
    };

    function osGetBucketLiveRateState(bucket, tenantId) {
        return osBucketLiveRateState[osBucketMetricsKey(bucket, tenantId)] || {};
    }

    function osEnsureBucketHoverCard() {
        let el = document.getElementById('osBucketHoverMetricsCard');
        if (!el) {
            el = document.createElement('div');
            el.id = 'osBucketHoverMetricsCard';
            el.className = 'os-bucket-hover-metrics';
            el.setAttribute('aria-hidden', 'true');
            document.body.appendChild(el);
        }
        return el;
    }

    function osPositionBucketHoverCard(anchor, card) {
        if (!anchor || !card) return;
        const rect = anchor.getBoundingClientRect();
        const margin = 10;

        card.style.left = (rect.left + rect.width / 2) + 'px';
        card.style.top = (rect.bottom + margin) + 'px';

        const cRect = card.getBoundingClientRect();
        let left = rect.left + rect.width / 2;
        let top = rect.bottom + margin;

        const minLeft = (cRect.width / 2) + 8;
        const maxLeft = window.innerWidth - (cRect.width / 2) - 8;
        left = Math.max(minLeft, Math.min(maxLeft, left));

        if (top + cRect.height > window.innerHeight - 8) {
            top = rect.top - cRect.height - margin;
        }
        if (top < 8) top = 8;

        card.style.left = left + 'px';
        card.style.top = top + 'px';
    }

    function osBuildBucketHoverContent(bucket, tenantId, metrics, rateInfo, loading, errorMsg) {
        const live = osGetBucketLiveRateState(bucket, tenantId);
        const liveRead = live.read;
        const liveWrite = live.write;

        const req = metrics ? (metrics.requestCount || 0) : '-';
        const getReq = metrics ? (metrics.getRequests || 0) : '-';
        const putReq = metrics ? (metrics.putRequests || 0) : '-';
        const delReq = metrics ? (metrics.deleteRequests || 0) : '-';
        const errs = metrics ? (metrics.errorCount || 0) : '-';
        const avg = metrics && metrics.requestCount > 0 ? Number(metrics.avgLatencyMs || 0).toFixed(1) + ' ms' : '-';
        const bytesIn = metrics ? fmtSize(metrics.bytesIn || 0) : '-';
        const bytesOut = metrics ? fmtSize(metrics.bytesOut || 0) : '-';
        const lastAccessed = metrics ? fmtDate(metrics.lastAccessed) : '-';

        const cfgRead = rateInfo ? osFormatRatePerMinute(rateInfo.readOpsPerMinute, true) : '-';
        const cfgWrite = rateInfo ? osFormatRatePerMinute(rateInfo.writeOpsPerMinute, true) : '-';
        const effRead = rateInfo ? osFormatRatePerMinute(rateInfo.effectiveReadOpsPerMinute, false) : '-';
        const effWrite = rateInfo ? osFormatRatePerMinute(rateInfo.effectiveWriteOpsPerMinute, false) : '-';

        const liveReadText = liveRead ? (liveRead.remaining + '/' + liveRead.limit + ' remaining (reset ' + osFormatEpochSeconds(liveRead.resetEpoch) + ')') : 'No recent read header';
        const liveWriteText = liveWrite ? (liveWrite.remaining + '/' + liveWrite.limit + ' remaining (reset ' + osFormatEpochSeconds(liveWrite.resetEpoch) + ')') : 'No recent write header';

        let statusText = '';
        if (loading) statusText = 'Loading latest metrics...';
        if (errorMsg) statusText = errorMsg;

        return '' +
            '<div class="os-bucket-hover-title">' + escHtml(bucket || '-') + '</div>' +
            '<div class="os-bucket-hover-subtitle">Tenant: ' + escHtml(tenantId || 'default') + '</div>' +
            (statusText ? ('<div class="os-bucket-hover-status">' + escHtml(statusText) + '</div>') : '') +
            '<div class="os-bucket-hover-grid">' +
                '<div class="os-bucket-hover-cell"><span>Requests</span><strong>' + req + '</strong></div>' +
                '<div class="os-bucket-hover-cell"><span>Errors</span><strong>' + errs + '</strong></div>' +
                '<div class="os-bucket-hover-cell"><span>GET</span><strong>' + getReq + '</strong></div>' +
                '<div class="os-bucket-hover-cell"><span>PUT</span><strong>' + putReq + '</strong></div>' +
                '<div class="os-bucket-hover-cell"><span>DELETE</span><strong>' + delReq + '</strong></div>' +
                '<div class="os-bucket-hover-cell"><span>Latency</span><strong>' + avg + '</strong></div>' +
                '<div class="os-bucket-hover-cell"><span>Bytes In</span><strong>' + escHtml(bytesIn) + '</strong></div>' +
                '<div class="os-bucket-hover-cell"><span>Bytes Out</span><strong>' + escHtml(bytesOut) + '</strong></div>' +
            '</div>' +
            '<div class="os-bucket-hover-rate-row"><span>Read Limit:</span><strong>' + escHtml(cfgRead) + ' (effective ' + escHtml(effRead) + ')</strong></div>' +
            '<div class="os-bucket-hover-rate-row"><span>Write Limit:</span><strong>' + escHtml(cfgWrite) + ' (effective ' + escHtml(effWrite) + ')</strong></div>' +
            '<div class="os-bucket-hover-rate-row"><span>Live Read Remaining:</span><strong>' + escHtml(liveReadText) + '</strong></div>' +
            '<div class="os-bucket-hover-rate-row"><span>Live Write Remaining:</span><strong>' + escHtml(liveWriteText) + '</strong></div>' +
            '<div class="os-bucket-hover-footer">Last accessed: ' + escHtml(lastAccessed) + '</div>';
    }

    async function osFetchBucketHoverData(bucket, tenantId) {
        const [metricsResp, rateResp] = await Promise.all([
            osFetch('/metrics/' + encodeURIComponent(bucket) + '?tenantId=' + encodeURIComponent(tenantId)),
            osFetch('/buckets/' + encodeURIComponent(bucket) + '/rate-limit?tenantId=' + encodeURIComponent(tenantId))
        ]);

        let metrics = null;
        let rateInfo = null;

        if (metricsResp.ok) {
            metrics = await metricsResp.json().catch(() => null);
        }
        if (rateResp.ok) {
            rateInfo = await rateResp.json().catch(() => null);
        }

        return { metrics: metrics, rateInfo: rateInfo };
    }

    window.osShowBucketHoverMetrics = async function(anchor) {
        if (!anchor) return;
        if (osBucketHoverHideTimer) {
            clearTimeout(osBucketHoverHideTimer);
            osBucketHoverHideTimer = null;
        }

        const bucket = String(anchor.getAttribute('data-bucket') || '').trim();
        const tenantId = String(anchor.getAttribute('data-tenant') || 'default').trim() || 'default';
        if (!bucket) return;

        const card = osEnsureBucketHoverCard();
        const reqToken = ++osBucketHoverRequestToken;

        card.innerHTML = osBuildBucketHoverContent(bucket, tenantId, null, null, true, '');
        card.classList.add('show');
        card.setAttribute('aria-hidden', 'false');
        osPositionBucketHoverCard(anchor, card);

        const cacheKey = osBucketMetricsKey(bucket, tenantId);
        const cached = osBucketHoverMetricsCache[cacheKey];
        if (cached && (Date.now() - cached.at) < osBucketHoverCacheTtlMs) {
            card.innerHTML = osBuildBucketHoverContent(bucket, tenantId, cached.data.metrics, cached.data.rateInfo, false, '');
            osPositionBucketHoverCard(anchor, card);
            return;
        }

        try {
            const data = await osFetchBucketHoverData(bucket, tenantId);
            osBucketHoverMetricsCache[cacheKey] = { at: Date.now(), data: data };

            if (reqToken !== osBucketHoverRequestToken) return;
            card.innerHTML = osBuildBucketHoverContent(bucket, tenantId, data.metrics, data.rateInfo, false, '');
            osPositionBucketHoverCard(anchor, card);
        } catch (e) {
            if (reqToken !== osBucketHoverRequestToken) return;
            card.innerHTML = osBuildBucketHoverContent(bucket, tenantId, null, null, false, e.message || 'Failed to load metrics');
            osPositionBucketHoverCard(anchor, card);
        }
    };

    window.osHideBucketHoverMetrics = function(immediate) {
        if (osBucketHoverHideTimer) {
            clearTimeout(osBucketHoverHideTimer);
            osBucketHoverHideTimer = null;
        }
        const card = document.getElementById('osBucketHoverMetricsCard');
        if (!card) return;

        const doHide = function() {
            card.classList.remove('show');
            card.setAttribute('aria-hidden', 'true');
        };

        if (immediate) {
            doHide();
            return;
        }
        osBucketHoverHideTimer = setTimeout(doHide, 220);
    };

    let osBucketHoverEventsBound = false;

    function osBindBucketHoverEvents() {
        if (osBucketHoverEventsBound) return;
        osBucketHoverEventsBound = true;

        const selector = '.os-dash-bucket-select, .os-bucket-hover-anchor';

        document.addEventListener('mouseover', function(e) {
            const target = e.target && e.target.closest ? e.target.closest(selector) : null;
            if (!target) return;
            window.osShowBucketHoverMetrics(target);
        });

        document.addEventListener('mouseout', function(e) {
            const from = e.target && e.target.closest ? e.target.closest(selector) : null;
            if (!from) return;
            const to = e.relatedTarget && e.relatedTarget.closest ? e.relatedTarget.closest(selector) : null;
            if (to === from) return;
            window.osHideBucketHoverMetrics();
        });

        document.addEventListener('scroll', function() {
            window.osHideBucketHoverMetrics(true);
        }, true);
    }

    function osBuildBucketPolicyTemplate(bucketName) {
        const b = String(bucketName || '').trim() || 'your-bucket-name';
        return JSON.stringify({
            Version: '2012-10-17',
            Statement: [
                {
                    Sid: 'AllowListBucket',
                    Effect: 'Allow',
                    Principal: '*',
                    Action: ['s3:ListBucket'],
                    Resource: ['arn:aws:s3:::' + b]
                },
                {
                    Sid: 'AllowObjectReadWrite',
                    Effect: 'Allow',
                    Principal: '*',
                    Action: ['s3:GetObject', 's3:PutObject', 's3:DeleteObject'],
                    Resource: ['arn:aws:s3:::' + b + '/*']
                }
            ]
        }, null, 2);
    }

    let osDashboardTemplateBucket = 'your-bucket-name';

    function osRenderDashboardPolicyTemplate(bucketName) {
        const b = String(bucketName || '').trim() || 'your-bucket-name';
        osDashboardTemplateBucket = b;
        const templateEl = document.getElementById('osDashboardPolicyTemplate');
        if (templateEl) {
            templateEl.value = osBuildBucketPolicyTemplate(b);
        }
        setText('osDashPolicyBucketName', b);

        document.querySelectorAll('#osDashRecentBuckets .os-dash-bucket-select').forEach(btn => {
            if ((btn.dataset.bucket || '') === b) {
                btn.classList.add('active');
            } else {
                btn.classList.remove('active');
            }
        });
    }

    window.osSelectDashboardPolicyBucket = function(bucketName) {
        osRenderDashboardPolicyTemplate(bucketName);
    };

    function osToast(msg, isErr) {
        const t = document.createElement('div');
        t.className = 'os-toast ' + (isErr ? 'os-toast-error' : 'os-toast-success');
        t.textContent = msg;
        document.body.appendChild(t);
        setTimeout(() => t.remove(), 4000);
    }

    function phaseBadge(phase) {
        const cls = phase === 'Ready' ? 'os-badge-ready' : phase === 'Pending' ? 'os-badge-pending' : 'os-badge-error';
        return '<span class="os-badge ' + cls + '">' + escHtml(phase || 'Unknown') + '</span>';
    }

    function versionBadge(v) {
        const cls = v === 'Enabled' ? 'os-badge-enabled' : 'os-badge-disabled';
        return '<span class="os-badge ' + cls + '">' + escHtml(v || 'Disabled') + '</span>';
    }

    // ===== Tab Switching =====
    window.osSwitch = function(tabId) {
        if (typeof window.osHideBucketHoverMetrics === 'function') {
            window.osHideBucketHoverMetrics(true);
        }
        if (tabId !== 'os-access-keys' && typeof window.osResetCreatedAccessKeyResult === 'function') {
            window.osResetCreatedAccessKeyResult();
        }
        document.querySelectorAll('.os-panel').forEach(p => p.classList.remove('active'));
        document.querySelectorAll('.os-tab').forEach(b => b.classList.remove('active'));
        const panel = document.getElementById(tabId);
        if (panel) panel.classList.add('active');
        document.querySelectorAll('.os-tab').forEach(b => {
            if (b.getAttribute('onclick') && b.getAttribute('onclick').indexOf(tabId) !== -1) b.classList.add('active');
        });
        if (tabId === 'os-dashboard') osLoadDashboard();
        if (tabId === 'os-buckets') osLoadBuckets();
        if (tabId === 'os-browser') osPopulateBucketSelects();
        if (tabId === 'os-upload') osPopulateBucketSelects();
        if (tabId === 'os-policies') osLoadPolicies();
        if (tabId === 'os-access-keys') osLoadAccessKeys();
        if (tabId === 'os-metrics') osLoadMetrics();
        if (tabId === 'os-events') osLoadEvents();
        if (tabId === 'os-settings') {
            osPopulateSettingsBuckets();
            osLoadBucketSettings();
        }
    };

    // ===== Modals =====
    window.osCloseModal = function(id) { document.getElementById(id).classList.remove('show'); };
    function osOpenModal(id) { document.getElementById(id).classList.add('show'); }
    document.addEventListener('click', function(e) {
        if (e.target.classList.contains('os-modal')) e.target.classList.remove('show');
    });

    // ===== Data Cache =====
    let osBucketsCache = [];
    let osBucketCatalogCache = [];
    let osObjectsCache = [];
    let osPoliciesCache = [];
    let osAccessKeysCache = [];

    // ===== Dashboard =====
    window.osLoadDashboard = async function() {
        try {
            const [statsResp, healthResp, bucketsResp, metricsResp] = await Promise.all([
                osFetch('/stats'),
                osFetch('/health'),
                osFetch('/buckets'),
                osFetch('/metrics')
            ]);
            if (statsResp.ok) {
                const st = await statsResp.json();
                setText('osStatBuckets', st.totalBuckets || 0);
                setText('osStatObjects', st.totalObjects || 0);
                setText('osStatSize', fmtSize(st.totalSizeBytes));
                setText('osDashBuckets', st.totalBuckets || 0);
                setText('osDashObjects', st.totalObjects || 0);
                setText('osDashSize', fmtSize(st.totalSizeBytes));
            }
            if (bucketsResp.ok) {
                const buckets = await bucketsResp.json();
                osBucketsCache = Array.isArray(buckets) ? buckets : [];
                osBucketCatalogCache = osBucketsCache.slice();
                osRefreshBucketNameSuggestions();
                osRenderAccessKeyScopeSuggestions();
                renderDashRecentBuckets(osBucketsCache.slice(0, 5));
                const firstBucket = osBucketsCache.length ? (osBucketsCache[0].metadata && osBucketsCache[0].metadata.name) : '';
                osRenderDashboardPolicyTemplate(firstBucket || 'your-bucket-name');
            } else {
                osRenderDashboardPolicyTemplate('your-bucket-name');
            }
            if (healthResp.ok) {
                const h = await healthResp.json();
                const online = h.status === 'healthy';
                setText('osStatHealth', online ? 'Online' : 'Offline');
                document.getElementById('osStatHealth').style.color = online ? '#22c55e' : '#ef4444';
                document.getElementById('osDashHealth').textContent = JSON.stringify(h, null, 2);
            } else {
                let hErr = { status: 'unhealthy', endpoint: '', checkedAt: '' };
                try {
                    hErr = await healthResp.json();
                } catch (_) {}
                setText('osStatHealth', 'Offline');
                document.getElementById('osStatHealth').style.color = '#ef4444';
                document.getElementById('osDashHealth').textContent = JSON.stringify({
                    status: hErr.status || 'unhealthy',
                    endpoint: hErr.endpoint || '',
                    checkedAt: hErr.checkedAt || '',
                    error: hErr.error || ('health request failed with status ' + healthResp.status)
                }, null, 2);
            }
            if (metricsResp.ok) {
                const m = await metricsResp.json();
                setText('osStatRequests', m.totalRequests || 0);
                setText('osStatErrors', m.totalErrors || 0);
                setText('osDashRequests', m.totalRequests || 0);
                setText('osDashErrors', m.totalErrors || 0);
                const avgMs = m.totalRequests > 0 ? ((m.avgLatencyMs || 0)).toFixed(1) : '0';
                setText('osDashLatency', avgMs + ' ms');

                if (!healthResp.ok) {
                    const backendHealthy = !!m.backendHealthy;
                    setText('osStatHealth', backendHealthy ? 'Online' : 'Offline');
                    document.getElementById('osStatHealth').style.color = backendHealthy ? '#22c55e' : '#ef4444';
                }
            } else {
                setText('osStatRequests', 0);
                setText('osStatErrors', 0);
            }
        } catch(e) {
            document.getElementById('osDashHealth').textContent = 'Error: ' + e.message;
        }
    };

    function setText(id, val) { const el = document.getElementById(id); if (el) el.textContent = val; }

    function renderDashRecentBuckets(buckets) {
        const el = document.getElementById('osDashRecentBuckets');
        if (!buckets.length) { el.innerHTML = '<p style="color:var(--text-muted);">No buckets yet. Create one to get started.</p>'; return; }
        let html = '<table class="os-table"><thead><tr><th>Name</th><th>Tenant</th><th>Phase</th><th>Objects</th><th>Size</th></tr></thead><tbody>';
        buckets.forEach(b => {
            const name = (b && b.metadata && b.metadata.name) ? b.metadata.name : '';
            const tenant = (b && b.metadata && b.metadata.tenantId) ? b.metadata.tenantId : '';
            const selectedClass = name === osDashboardTemplateBucket ? ' active' : '';
            html += '<tr><td><button type="button" class="os-dash-bucket-select' + selectedClass + '" data-bucket="' + escHtml(name) + '" data-tenant="' + escHtml(tenant || 'default') + '" onmouseenter="osShowBucketHoverMetrics(this)" onmouseleave="osHideBucketHoverMetrics()" onfocus="osShowBucketHoverMetrics(this)" onblur="osHideBucketHoverMetrics()" onclick="osSelectDashboardPolicyBucket(this.dataset.bucket)">' + escHtml(name || '-') + '</button></td><td>' + escHtml(tenant) + '</td><td>' + phaseBadge(b.status?.phase) + '</td><td>' + (b.status?.objectCount||0) + '</td><td class="os-size">' + fmtSize(b.status?.totalSize) + '</td></tr>';
        });
        html += '</tbody></table>';
        el.innerHTML = html;
    }

    // ===== Buckets =====
    window.osLoadBuckets = async function() {
        const tenant = document.getElementById('osTenantFilter').value;
        const qs = tenant ? '?tenantId=' + encodeURIComponent(tenant) : '';
        try {
            const resp = await osFetch('/buckets' + qs);
            if (!resp.ok) { osToast('Failed to load buckets: ' + resp.status, true); return; }
            const data = await resp.json();
            osBucketsCache = Array.isArray(data) ? data : [];
            if (!tenant) {
                osBucketCatalogCache = osBucketsCache.slice();
            }
            osRefreshBucketNameSuggestions();
            osRenderAccessKeyScopeSuggestions();
            renderBuckets(osBucketsCache);
            osPopulateBucketSelects();
            osPopulateSettingsBuckets();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    function renderBuckets(buckets) {
        const tbody = document.getElementById('osBucketsBody');
        if (!buckets.length) {
            tbody.innerHTML = '<tr><td colspan="9" class="os-empty"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/><path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/></svg><br>No buckets found</td></tr>';
            return;
        }
        tbody.innerHTML = buckets.map(b => {
            const m = b.metadata || {};
            const s = b.spec || {};
            const st = b.status || {};
            return '<tr>' +
                '<td><span tabindex="0" class="os-bucket-hover-anchor" data-bucket="' + escHtml(m.name) + '" data-tenant="' + escHtml(m.tenantId || 'default') + '" onmouseenter="osShowBucketHoverMetrics(this)" onmouseleave="osHideBucketHoverMetrics()" onfocus="osShowBucketHoverMetrics(this)" onblur="osHideBucketHoverMetrics()"><strong>' + escHtml(m.name) + '</strong></span></td>' +
                '<td>' + escHtml(m.tenantId) + '</td>' +
                '<td>' + phaseBadge(st.phase) + '</td>' +
                '<td>' + versionBadge(s.versioning) + '</td>' +
                '<td>' + (st.objectCount||0) + '</td>' +
                '<td class="os-size">' + fmtSize(st.totalSize) + '</td>' +
                '<td><button class="os-btn os-btn-secondary os-btn-sm" onclick="osShowBucketTags(\'' + escHtml(m.name) + '\',\'' + escHtml(m.tenantId) + '\')">Tags</button></td>' +
                '<td>' + fmtDate(m.createdAt) + '</td>' +
                '<td style="white-space:nowrap;">' +
                    '<button class="os-btn os-btn-secondary os-btn-sm" onclick="osBrowseThisBucket(\'' + escHtml(m.name) + '\',\'' + escHtml(m.tenantId) + '\')">Browse</button> ' +
                    '<button class="os-btn os-btn-danger os-btn-sm" onclick="osDeleteBucket(\'' + escHtml(m.name) + '\',\'' + escHtml(m.tenantId) + '\')">Delete</button>' +
                '</td></tr>';
        }).join('');
    }

    window.osFilterBuckets = function() {
        const q = (document.getElementById('osBucketSearch').value || '').toLowerCase();
        renderBuckets(osBucketsCache.filter(b => {
            const n = (b.metadata?.name || '').toLowerCase();
            const t = (b.metadata?.tenantId || '').toLowerCase();
            return n.indexOf(q) !== -1 || t.indexOf(q) !== -1;
        }));
    };

    window.osShowCreateBucket = function() {
        osRefreshBucketNameSuggestions();
        window.osValidateCreateBucketName(false);
        osOpenModal('osCreateBucketModal');
    };

    window.osCreateBucket = async function() {
        const name = document.getElementById('osNewBucketName').value.trim();
        const tenant = document.getElementById('osNewBucketTenant').value.trim();
        if (!name || !tenant) { osToast('Name and Tenant ID required', true); return; }
        if (!window.osValidateCreateBucketName(true)) return;
        const body = {
            name: name,
            tenantId: tenant,
            versioning: document.getElementById('osNewBucketVersioning').value,
            region: document.getElementById('osNewBucketRegion').value.trim()
        };
        try {
            const resp = await osFetch('/buckets', { method: 'POST', body: JSON.stringify(body) });
            const data = await resp.json();
            if (!resp.ok) { osToast(data.error || 'Create failed', true); return; }
            osToast('Bucket "' + name + '" created');
            osCloseModal('osCreateBucketModal');
            document.getElementById('osNewBucketName').value = '';
            window.osValidateCreateBucketName(false);
            osLoadBuckets();
            osLoadDashboard();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osDeleteBucket = async function(name, tenantId) {
        if (!confirm('Delete bucket "' + name + '"? This cannot be undone.')) return;
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(name) + '?tenantId=' + encodeURIComponent(tenantId), { method: 'DELETE' });
            if (!resp.ok) { const d = await resp.json().catch(()=>({})); osToast(d.error || 'Delete failed', true); return; }
            osToast('Bucket deleted');
            osLoadBuckets();
            osLoadDashboard();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    // ===== Object Browser =====
    function osPopulateBucketSelects() {
        const selects = ['osBrowserBucket', 'osUploadBucket', 'osPresignBucket', 'osCopyDestBucket'];
        selects.forEach(id => {
            const sel = document.getElementById(id);
            if (!sel) return;
            const current = sel.value;
            const optStart = '<option value="">Select bucket...</option>';
            sel.innerHTML = optStart + osBucketsCache.map(b => {
                const n = b.metadata?.name || '';
                const t = b.metadata?.tenantId || '';
                return '<option value="' + escHtml(n) + '" data-tenant="' + escHtml(t) + '">' + escHtml(n) + ' (' + escHtml(t) + ')</option>';
            }).join('');
            if (current) sel.value = current;
        });
    }

    function osPopulateSettingsBuckets() {
        const sel = document.getElementById('osSettingsBucket');
        if (!sel) return;
        const current = sel.value;
        sel.innerHTML = '<option value="">Select a bucket...</option>' + osBucketsCache.map(b => {
            const n = b.metadata?.name || '';
            const t = b.metadata?.tenantId || '';
            return '<option value="' + escHtml(n) + '" data-tenant="' + escHtml(t) + '">' + escHtml(n) + ' (' + escHtml(t) + ')</option>';
        }).join('');
        if (current) sel.value = current;
    }

    window.osBrowseBucket = async function() {
        const sel = document.getElementById('osBrowserBucket');
        const bucket = sel.value;
        if (!bucket) return;
        const opt = sel.options[sel.selectedIndex];
        const tenantId = opt.getAttribute('data-tenant') || 'default';
        const prefix = (document.getElementById('osBrowserPrefix').value || '').trim();

        const qs = '?tenantId=' + encodeURIComponent(tenantId) + (prefix ? '&prefix=' + encodeURIComponent(prefix) : '');
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(bucket) + '/objects' + qs);
            osCaptureRateHeaders(bucket, tenantId, 'GET', resp);
            if (!resp.ok) { osToast('Failed to list objects', true); return; }
            const data = await resp.json();
            osObjectsCache = data.objects || [];
            renderObjects(osObjectsCache, bucket, tenantId);
            // Breadcrumb
            const bc = document.getElementById('osBrowserBreadcrumb');
            bc.innerHTML = '<a href="#" onclick="document.getElementById(\'osBrowserPrefix\').value=\'\'; osBrowseBucket(); return false;">Bucket ' + escHtml(bucket) + '</a>';
            if (prefix) bc.innerHTML += ' / <span>' + escHtml(prefix) + '</span>';
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osBrowseThisBucket = function(name, tenantId) {
        osSwitch('os-browser');
        setTimeout(() => {
            const sel = document.getElementById('osBrowserBucket');
            for (let i = 0; i < sel.options.length; i++) {
                if (sel.options[i].value === name) { sel.selectedIndex = i; break; }
            }
            osBrowseBucket();
        }, 100);
    };

    function renderObjects(objects, bucket, tenantId) {
        const tbody = document.getElementById('osBrowserBody');
        if (!objects.length) {
            tbody.innerHTML = '<tr><td colspan="7" class="os-empty"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg><br>No objects in this bucket</td></tr>';
            document.getElementById('osBatchDeleteBtn').style.display = 'none';
            document.getElementById('osCopyObjectBtn').style.display = 'none';
            return;
        }
        document.getElementById('osBatchDeleteBtn').style.display = '';
        document.getElementById('osCopyObjectBtn').style.display = '';
        window._osBrowserBucket = bucket;
        window._osBrowserTenant = tenantId;
        tbody.innerHTML = objects.map(o => {
            return '<tr>' +
                '<td><input type="checkbox" class="os-obj-check" data-key="' + escHtml(o.key) + '"></td>' +
                '<td class="os-mono">' + escHtml(o.key) + '</td>' +
                '<td class="os-size">' + fmtSize(o.size) + '</td>' +
                '<td>' + escHtml(o.contentType || '-') + '</td>' +
                '<td>' + fmtDate(o.lastModified) + '</td>' +
                '<td class="os-mono" style="max-width:120px; overflow:hidden; text-overflow:ellipsis;">' + escHtml(o.etag || '-') + '</td>' +
                '<td style="white-space:nowrap;">' +
                    '<button class="os-btn os-btn-secondary os-btn-sm" onclick="osShareObject(\'' + escHtml(bucket) + '\',\'' + escHtml(o.key) + '\',\'' + escHtml(tenantId) + '\')">Share</button> ' +
                    '<button class="os-btn os-btn-secondary os-btn-sm" onclick="osDownloadObject(\'' + escHtml(bucket) + '\',\'' + escHtml(o.key) + '\',\'' + escHtml(tenantId) + '\')">Download</button> ' +
                    '<button class="os-btn os-btn-secondary os-btn-sm" onclick="osShowCopySingle(\'' + escHtml(bucket) + '\',\'' + escHtml(o.key) + '\',\'' + escHtml(tenantId) + '\')">Copy</button> ' +
                    '<button class="os-btn os-btn-danger os-btn-sm" onclick="osDeleteObjectFromBrowser(\'' + escHtml(bucket) + '\',\'' + escHtml(o.key) + '\',\'' + escHtml(tenantId) + '\')">Delete</button>' +
                '</td></tr>';
        }).join('');
    }

    window.osDownloadObject = async function(bucket, key, tenantId) {
        const path = '/buckets/' + encodeURIComponent(bucket) + '/objects/' + encodeURIComponent(key) + '?tenantId=' + encodeURIComponent(tenantId);
        try {
            const resp = await osFetch(path, { method: 'GET' });
            osCaptureRateHeaders(bucket, tenantId, 'GET', resp);
            if (!resp.ok) {
                const d = await resp.json().catch(() => ({}));
                osToast(d.error || 'Download failed', true);
                return;
            }
            const blob = await resp.blob();
            const a = document.createElement('a');
            const url = window.URL.createObjectURL(blob);
            a.href = url;
            a.download = key.split('/').pop() || 'download';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            window.URL.revokeObjectURL(url);
        } catch (e) {
            osToast('Error: ' + e.message, true);
        }
    };

    let osShareContext = null;
    let osShareKeysCache = [];

    function osRoleAllowsRead(role) {
        const r = String(role || '').toLowerCase();
        return r === 'storage-admin' || r === 'storage-writer' || r === 'storage-reader' || r === 'storage-browser';
    }

    function osAccessKeyExpired(ak) {
        if (!ak || !ak.expiresAt) return false;
        const ms = Date.parse(ak.expiresAt);
        return !isNaN(ms) && ms <= Date.now();
    }

    function osDefaultAccessKeyName(bucket) {
        const ts = new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14);
        return 'share-reader-' + bucket + '-' + ts;
    }

    function osNormalizeBucketName(v) {
        return String(v || '').trim().toLowerCase();
    }

    function osBucketScopeMatches(scopeEntry, logicalBucket, storageBucket) {
        const scope = osNormalizeBucketName(scopeEntry);
        if (!scope) return false;
        if (scope === '*') return true;

        const logical = osNormalizeBucketName(logicalBucket);
        const storage = osNormalizeBucketName(storageBucket);
        if (!logical && !storage) return false;

        if (scope === logical || scope === storage) return true;
        if (storage && storage.endsWith('-' + scope)) return true;
        if (logical && scope.endsWith('-' + logical)) return true;
        return false;
    }

    function osAccessKeyAllowsBucket(ak, logicalBucket, storageBucket) {
        const scopes = Array.isArray(ak && ak.bucketScope) ? ak.bucketScope : [];
        if (!scopes.length) return true;
        return scopes.some(scope => osBucketScopeMatches(scope, logicalBucket, storageBucket));
    }

    function osAccessKeyAllowsObjectKey(ak, key) {
        const prefix = String((ak && ak.prefixScope) || '').trim();
        if (!prefix) return true;
        const objectKey = String(key || '').replace(/^\/+/, '');
        return objectKey === prefix || objectKey.indexOf(prefix) === 0;
    }

    function osAccessKeySupportsShare(ak, logicalBucket, storageBucket, key) {
        return ak &&
            ak.active !== false &&
            !osAccessKeyExpired(ak) &&
            osRoleAllowsRead(ak.role) &&
            osAccessKeyAllowsBucket(ak, logicalBucket, storageBucket) &&
            osAccessKeyAllowsObjectKey(ak, key);
    }

    async function osListAccessKeys() {
        const resp = await osFetch('/access-keys');
        if (!resp.ok) {
            const d = await resp.json().catch(() => ({}));
            throw new Error(d.error || ('Failed to list access keys (' + resp.status + ')'));
        }
        const data = await resp.json();
        if (Array.isArray(data)) return data;
        if (Array.isArray(data.accessKeys)) return data.accessKeys;
        return [];
    }

    function osFindBucketResource(logicalBucket, tenantId) {
        for (let i = 0; i < osBucketsCache.length; i++) {
            const b = osBucketsCache[i] || {};
            const m = b.metadata || {};
            if (m.name !== logicalBucket) continue;
            if (tenantId && m.tenantId && m.tenantId !== tenantId) continue;
            return b;
        }
        return null;
    }

    function osResolvedStorageBucketName(logicalBucket, tenantId) {
        const bucket = osFindBucketResource(logicalBucket, tenantId);
        const resolved = bucket && bucket.spec && bucket.spec.name;
        return resolved || logicalBucket;
    }

    function osShareSetupError(msg) {
        const el = document.getElementById('osShareSetupError');
        if (!el) return;
        if (!msg) {
            el.style.display = 'none';
            el.textContent = '';
            return;
        }
        el.style.display = '';
        el.textContent = msg;
    }

    function osDescribeAccessKeyScope(ak) {
        const scopes = Array.isArray(ak && ak.bucketScope) ? ak.bucketScope.filter(Boolean) : [];
        return scopes.length ? scopes.join(', ') : '*';
    }

    function osRenderShareAccessKeyOptions(preferredAccessKeyId) {
        const select = document.getElementById('osShareAccessKey');
        const hint = document.getElementById('osShareAccessKeyHint');
        if (!select || !hint) return;

        if (!osShareKeysCache.length) {
            select.innerHTML = '<option value="">No compatible active key found</option>';
            select.value = '';
            hint.textContent = 'No compatible key available for this object. Create one below.';
            return;
        }

        select.innerHTML = osShareKeysCache.map(ak => {
            const label = (ak.name || '(unnamed)') + ' [' + (ak.accessKeyId || '-') + '] role=' + (ak.role || '-') + ' scope=' + osDescribeAccessKeyScope(ak);
            return '<option value="' + escHtml(ak.accessKeyId || '') + '">' + escHtml(label) + '</option>';
        }).join('');

        if (preferredAccessKeyId) {
            const exists = osShareKeysCache.some(ak => ak.accessKeyId === preferredAccessKeyId);
            if (exists) {
                select.value = preferredAccessKeyId;
            }
        }
        if (!select.value && osShareKeysCache[0] && osShareKeysCache[0].accessKeyId) {
            select.value = osShareKeysCache[0].accessKeyId;
        }
        hint.textContent = osShareKeysCache.length + ' compatible key(s) available for this object.';
    }

    window.osShareReloadAccessKeys = async function(preferredAccessKeyId) {
        if (!osShareContext) return;
        try {
            const keys = await osListAccessKeys();
            osShareKeysCache = keys.filter(ak => {
                return osAccessKeySupportsShare(ak, osShareContext.bucket, osShareContext.storageBucket, osShareContext.key);
            });
            osRenderShareAccessKeyOptions(preferredAccessKeyId);
            osShareSetupError('');
        } catch (e) {
            osShareKeysCache = [];
            osRenderShareAccessKeyOptions('');
            osShareSetupError(e.message || 'Failed to load access keys');
        }
    };

    window.osShareCreateAccessKeyFromModal = async function() {
        if (!osShareContext) return;

        const nameInput = document.getElementById('osShareNewAccessKeyName');
        const restrict = !!document.getElementById('osShareRestrictToBucket').checked;
        const name = (nameInput.value || '').trim();
        if (!name) {
            osShareSetupError('Access key name is required');
            return;
        }

        const body = {
            name: name,
            role: 'storage-reader'
        };

        if (restrict) {
            body.bucketScope = [osShareContext.bucket];
            if (osShareContext.storageBucket && osShareContext.storageBucket !== osShareContext.bucket) {
                body.bucketScope.push(osShareContext.storageBucket);
            }
        }

        try {
            const resp = await osFetch('/access-keys', {
                method: 'POST',
                body: JSON.stringify(body)
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                osShareSetupError(data.error || ('Failed to create access key (' + resp.status + ')'));
                return;
            }

            osToast('Access key created: ' + (data.accessKeyId || ''));
            nameInput.value = osDefaultAccessKeyName(osShareContext.bucket);
            osShareSetupError('');
            await window.osShareReloadAccessKeys(data.accessKeyId || '');
        } catch (e) {
            osShareSetupError(e.message || 'Failed to create access key');
        }
    };

    window.osShareGenerateFromModal = async function() {
        if (!osShareContext) return;

        const accessKeyId = (document.getElementById('osShareAccessKey').value || '').trim();
        const seconds = parseInt(document.getElementById('osShareSetupExpiry').value, 10);

        if (!accessKeyId) {
            osShareSetupError('Select an access key or create a new one first');
            return;
        }
        if (!seconds || seconds < 60) {
            osShareSetupError('Expiry must be at least 60 seconds');
            return;
        }

        try {
            const payload = { key: osShareContext.key, expires: seconds, accessKeyId: accessKeyId };
            const resp = await osFetch('/buckets/' + encodeURIComponent(osShareContext.bucket) + '/share-object?tenantId=' + encodeURIComponent(osShareContext.tenantId), {
                method: 'POST',
                body: JSON.stringify(payload)
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                osShareSetupError(data.error || 'Share failed');
                osToast(data.error || 'Share failed', true);
                return;
            }

            osCloseModal('osShareSetupModal');
            document.getElementById('osShareURL').value = data.url || '';
            document.getElementById('osShareExpires').textContent = fmtDate(data.expiresAt);
            document.getElementById('osShareObjectKey').textContent = osShareContext.bucket + '/' + osShareContext.key;
            osOpenModal('osShareObjectModal');
            osToast('Shareable link generated');
        } catch (e) {
            osShareSetupError(e.message || 'Failed to generate share link');
        }
    }

    // ===== Share Object (Shareable URL) =====
    window.osShareObject = async function(bucket, key, tenantId) {
        try {
            osShareContext = {
                bucket: bucket,
                key: key,
                tenantId: tenantId,
                storageBucket: osResolvedStorageBucketName(bucket, tenantId)
            };

            document.getElementById('osShareSetupObject').value = bucket + '/' + key;
            document.getElementById('osShareSetupExpiry').value = 86400;
            document.getElementById('osShareNewAccessKeyName').value = osDefaultAccessKeyName(bucket);
            document.getElementById('osShareRestrictToBucket').checked = true;
            document.getElementById('osShareSetupBucketScope').textContent = bucket;
            document.getElementById('osShareSetupStorageBucket').textContent = osShareContext.storageBucket;

            osShareSetupError('');
            await window.osShareReloadAccessKeys('');
            osOpenModal('osShareSetupModal');
        } catch (e) {
            osToast('Error: ' + e.message, true);
        }
    };

    window.osCopyShareURL = function() {
        const el = document.getElementById('osShareURL');
        el.select();
        if (navigator.clipboard) {
            navigator.clipboard.writeText(el.value).then(() => osToast('Copied to clipboard'));
        } else {
            document.execCommand('copy');
            osToast('Copied to clipboard');
        }
    };

    window.osDeleteObjectFromBrowser = async function(bucket, key, tenantId) {
        if (!confirm('Delete object "' + key + '"?')) return;
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(bucket) + '/objects/' + encodeURIComponent(key) + '?tenantId=' + encodeURIComponent(tenantId), { method: 'DELETE' });
            osCaptureRateHeaders(bucket, tenantId, 'DELETE', resp);
            if (!resp.ok) { osToast('Delete failed', true); return; }
            osToast('Object deleted');
            osBrowseBucket();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    // ===== Upload =====
    let osSelectedFile = null;

    window.osHandleDrop = function(e) {
        e.preventDefault();
        document.getElementById('osUploadZone').classList.remove('dragover');
        if (e.dataTransfer.files.length > 0) {
            osSelectedFile = e.dataTransfer.files[0];
            document.getElementById('osUploadFileName').textContent = osSelectedFile.name + ' (' + fmtSize(osSelectedFile.size) + ')';
        }
    };
    window.osHandleDragOver = function(e) { e.preventDefault(); document.getElementById('osUploadZone').classList.add('dragover'); };
    window.osHandleDragLeave = function(e) { document.getElementById('osUploadZone').classList.remove('dragover'); };
    window.osFileSelected = function(e) {
        if (e.target.files.length > 0) {
            osSelectedFile = e.target.files[0];
            document.getElementById('osUploadFileName').textContent = osSelectedFile.name + ' (' + fmtSize(osSelectedFile.size) + ')';
        }
    };

    window.osUploadObject = async function() {
        const bucket = document.getElementById('osUploadBucket').value;
        const tenantId = (document.getElementById('osUploadTenant').value || 'default').trim();
        let key = (document.getElementById('osUploadKey').value || '').trim();
        if (!bucket) { osToast('Select a bucket', true); return; }
        if (!osSelectedFile) { osToast('Select a file to upload', true); return; }
        if (!key) key = osSelectedFile.name;

        const url = OS_API + '/buckets/' + encodeURIComponent(bucket) + '/objects/' + encodeURIComponent(key) + '?tenantId=' + encodeURIComponent(tenantId);

        const progress = document.getElementById('osUploadProgress');
        const bar = document.getElementById('osUploadProgressBar');
        progress.style.display = 'block';
        bar.style.width = '0%';

        try {
            const tok = localStorage.getItem('iamToken') || localStorage.getItem('authToken') || '';
            const xhr = new XMLHttpRequest();
            xhr.open('PUT', url, true);
            xhr.setRequestHeader('Content-Type', osSelectedFile.type || 'application/octet-stream');
            if (tok) xhr.setRequestHeader('Authorization', 'Bearer ' + tok);

            xhr.upload.onprogress = function(e) {
                if (e.lengthComputable) bar.style.width = Math.round(e.loaded/e.total*100) + '%';
            };
            xhr.onload = function() {
                osCaptureRateHeadersFromXHR(bucket, tenantId, 'PUT', xhr);
                if (xhr.status >= 200 && xhr.status < 300) {
                    osToast('Upload complete: ' + key);
                    bar.style.width = '100%';
                    osSelectedFile = null;
                    document.getElementById('osUploadFileName').textContent = '';
                    document.getElementById('osUploadKey').value = '';
                } else {
                    osToast('Upload failed: ' + xhr.status, true);
                }
                setTimeout(() => { progress.style.display = 'none'; }, 2000);
            };
            xhr.onerror = function() { osToast('Upload failed', true); progress.style.display = 'none'; };
            xhr.send(osSelectedFile);
        } catch(e) {
            osToast('Error: ' + e.message, true);
            progress.style.display = 'none';
        }
    };

    // ===== Pre-signed URLs =====
    window.osGeneratePresign = async function() {
        const bucket = document.getElementById('osPresignBucket').value;
        const tenantId = (document.getElementById('osPresignTenant').value || 'default').trim();
        const key = (document.getElementById('osPresignKey').value || '').trim();
        const method = document.getElementById('osPresignMethod').value;
        const expires = parseInt(document.getElementById('osPresignExpiry').value) || 900;

        if (!bucket || !key) { osToast('Bucket and key are required', true); return; }

        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(bucket) + '/presign?tenantId=' + encodeURIComponent(tenantId), {
                method: 'POST',
                body: JSON.stringify({ key: key, method: method, expires: expires })
            });
            if (!resp.ok) { const d = await resp.json().catch(()=>({})); osToast(d.error || 'Failed', true); return; }
            const data = await resp.json();
            document.getElementById('osPresignURL').value = data.url || '';
            document.getElementById('osPresignExpires').textContent = fmtDate(data.expiresAt);
            document.getElementById('osPresignResult').style.display = 'block';
            osToast('Pre-signed URL generated');
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osCopyPresign = function() {
        const el = document.getElementById('osPresignURL');
        el.select();
        document.execCommand('copy');
        osToast('Copied to clipboard');
    };

    // ===== Access Keys =====
    function osFormatAccessKeyScope(ak) {
        const scopes = Array.isArray(ak && ak.bucketScope) ? ak.bucketScope.filter(Boolean) : [];
        return scopes.length ? scopes.join(', ') : '*';
    }

    function osAccessKeyStatusInfo(ak) {
        if (!ak || ak.active === false) {
            return { label: 'Inactive', cls: 'os-badge-error' };
        }
        if (ak.expiresAt) {
            const expMs = Date.parse(ak.expiresAt);
            if (!isNaN(expMs) && expMs <= Date.now()) {
                return { label: 'Expired', cls: 'os-badge-error' };
            }
        }
        return { label: 'Active', cls: 'os-badge-ready' };
    }

    let osAccessKeysViewMode = 'active';
    let osCreatedAccessKeyHideTimer = null;

    window.osResetCreatedAccessKeyResult = function() {
        if (osCreatedAccessKeyHideTimer) {
            clearTimeout(osCreatedAccessKeyHideTimer);
            osCreatedAccessKeyHideTimer = null;
        }
        const resultBox = document.getElementById('osAccessKeyCreateResult');
        const resultEmpty = document.getElementById('osAccessKeyCreateResultEmpty');
        const idField = document.getElementById('osCreatedAccessKeyIdField');
        const idText = document.getElementById('osCreatedAccessKeyId');
        const secretField = document.getElementById('osCreatedAccessKeySecret');
        if (idField) idField.value = '';
        if (idText) idText.textContent = '';
        if (secretField) secretField.value = '';
        if (resultBox) resultBox.style.display = 'none';
        if (resultEmpty) resultEmpty.style.display = '';
    };

    function osScheduleHideCreatedAccessKeyResult() {
        if (osCreatedAccessKeyHideTimer) {
            clearTimeout(osCreatedAccessKeyHideTimer);
        }
        osCreatedAccessKeyHideTimer = setTimeout(() => {
            window.osResetCreatedAccessKeyResult();
        }, 60000);
    }

    function osAccessKeyIsOld(ak) {
        return osAccessKeyStatusInfo(ak).label !== 'Active';
    }

    function osFilterAccessKeys(keys) {
        const mode = osAccessKeysViewMode;
        if (mode === 'all') return keys.slice();
        if (mode === 'old') return keys.filter(osAccessKeyIsOld);
        return keys.filter(ak => !osAccessKeyIsOld(ak));
    }

    window.osApplyAccessKeyFilter = function() {
        const sel = document.getElementById('osAccessKeysFilter');
        osAccessKeysViewMode = sel && sel.value ? sel.value : 'active';
        const filtered = osFilterAccessKeys(osAccessKeysCache || []);
        renderAccessKeys(filtered, (osAccessKeysCache || []).length);
    };

    function renderAccessKeys(keys, totalCount) {
        const tbody = document.getElementById('osAccessKeysBody');
        const count = document.getElementById('osAccessKeysCount');
        const total = Number.isFinite(totalCount) ? totalCount : (keys.length || 0);
        if (count) {
            count.textContent = keys.length === total
                ? total + ' key(s)'
                : keys.length + ' shown of ' + total + ' key(s)';
        }

        if (!keys.length) {
            const msg = total > 0
                ? 'No access keys found for selected filter'
                : 'No access keys found';
            tbody.innerHTML = '<tr><td colspan="8" class="os-empty">' + msg + '</td></tr>';
            return;
        }

        tbody.innerHTML = keys.map(ak => {
            const status = osAccessKeyStatusInfo(ak);
            const canDelete = status.label !== 'Active';
            let actions = '<button class="os-btn os-btn-danger os-btn-sm" data-keyid="' + escHtml(ak.accessKeyId || '') + '" onclick="osRevokeAccessKey(this.dataset.keyid)">Revoke</button>';
            if (canDelete) {
                actions += ' <button class="os-btn os-btn-secondary os-btn-sm" data-keyid="' + escHtml(ak.accessKeyId || '') + '" onclick="osDeleteAccessKey(this.dataset.keyid)">Delete</button>';
            }
            return '<tr>' +
                '<td>' + escHtml(ak.name || '-') + '</td>' +
                '<td class="os-mono">' + escHtml(ak.accessKeyId || '-') + '</td>' +
                '<td><span class="os-badge os-badge-enabled">' + escHtml(ak.role || '-') + '</span></td>' +
                '<td class="os-mono" style="max-width:220px; overflow:hidden; text-overflow:ellipsis;">' + escHtml(osFormatAccessKeyScope(ak)) + '</td>' +
                '<td>' + fmtDate(ak.expiresAt) + '</td>' +
                '<td>' + fmtDate(ak.lastUsedAt) + '</td>' +
                '<td><span class="os-badge ' + status.cls + '">' + status.label + '</span></td>' +
                '<td style="white-space:nowrap;">' +
                    actions +
                '</td>' +
            '</tr>';
        }).join('');
    }

    window.osLoadAccessKeys = async function() {
        const tbody = document.getElementById('osAccessKeysBody');
        window.osResetCreatedAccessKeyResult();
        if (tbody) {
            tbody.innerHTML = '<tr><td colspan="8" style="text-align:center; padding:24px; color:var(--text-muted);">Loading access keys...</td></tr>';
        }

        try {
            const resp = await osFetch('/access-keys');
            if (!resp.ok) {
                const d = await resp.json().catch(() => ({}));
                osToast(d.error || 'Failed to load access keys', true);
                renderAccessKeys([]);
                return;
            }

            const data = await resp.json();
            osAccessKeysCache = Array.isArray(data) ? data : (Array.isArray(data.accessKeys) ? data.accessKeys : []);
            osAccessKeysCache.sort((a, b) => Date.parse(b.createdAt || '') - Date.parse(a.createdAt || ''));
            window.osApplyAccessKeyFilter();
        } catch (e) {
            osToast('Error: ' + e.message, true);
            renderAccessKeys([], 0);
        }
    };

    window.osClearAccessKeyForm = function() {
        document.getElementById('osNewAccessKeyName').value = '';
        document.getElementById('osNewAccessKeyRole').value = 'storage-reader';
        document.getElementById('osNewAccessKeyDescription').value = '';
        document.getElementById('osNewAccessKeyBucketScope').value = '';
        document.getElementById('osNewAccessKeyExpiresAt').value = '';
    };

    window.osCreateAccessKeyFromTab = async function() {
        const name = (document.getElementById('osNewAccessKeyName').value || '').trim();
        const role = document.getElementById('osNewAccessKeyRole').value;
        const description = (document.getElementById('osNewAccessKeyDescription').value || '').trim();
        const scopeRaw = (document.getElementById('osNewAccessKeyBucketScope').value || '').trim();
        const expiresAtRaw = (document.getElementById('osNewAccessKeyExpiresAt').value || '').trim();

        if (!name) { osToast('Access key name is required', true); return; }
        if (!role) { osToast('Access key role is required', true); return; }

        const body = { name: name, role: role };
        if (description) body.description = description;

        const scopes = scopeRaw.split(',').map(s => s.trim()).filter(Boolean);
        if (scopes.length) body.bucketScope = scopes;

        if (expiresAtRaw) {
            const expMs = Date.parse(expiresAtRaw);
            if (isNaN(expMs)) {
                osToast('Invalid expiry date/time', true);
                return;
            }
            body.expiresAt = new Date(expMs).toISOString();
        }

        try {
            const resp = await osFetch('/access-keys', {
                method: 'POST',
                body: JSON.stringify(body)
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                osToast(data.error || 'Failed to create access key', true);
                return;
            }

            const resultBox = document.getElementById('osAccessKeyCreateResult');
            const resultEmpty = document.getElementById('osAccessKeyCreateResultEmpty');
            if (resultBox) resultBox.style.display = '';
            if (resultEmpty) resultEmpty.style.display = 'none';
            const createdId = data.accessKeyId || '';
            setText('osCreatedAccessKeyId', createdId || '-');
            const idField = document.getElementById('osCreatedAccessKeyIdField');
            if (idField) idField.value = createdId;
            document.getElementById('osCreatedAccessKeySecret').value = data.secretAccessKey || '';

            osToast('Access key created');
            osScheduleHideCreatedAccessKeyResult();
            window.osClearAccessKeyForm();
            const safeCreated = Object.assign({}, data || {}, { secretAccessKey: '' });
            osAccessKeysCache = [safeCreated].concat((osAccessKeysCache || []).filter(k => (k && k.accessKeyId) !== safeCreated.accessKeyId));
            osAccessKeysCache.sort((a, b) => Date.parse((b && b.createdAt) || '') - Date.parse((a && a.createdAt) || ''));
            window.osApplyAccessKeyFilter();
        } catch (e) {
            osToast('Error: ' + e.message, true);
        }
    };

    window.osCopyCreatedAccessKeyId = function() {
        const idField = document.getElementById('osCreatedAccessKeyIdField');
        const fallback = document.getElementById('osCreatedAccessKeyId').textContent || '';
        const text = idField ? idField.value.trim() : fallback.trim();
        if (!text || text === '-') { osToast('No access key ID to copy', true); return; }
        if (navigator.clipboard) {
            navigator.clipboard.writeText(text).then(() => osToast('Access key ID copied'));
        } else {
            osToast('Clipboard API not available', true);
        }
    };

    window.osCopyCreatedAccessKeySecret = function() {
        const el = document.getElementById('osCreatedAccessKeySecret');
        const text = el ? (el.value || '') : '';
        if (!text) { osToast('No secret key to copy', true); return; }

        if (navigator.clipboard) {
            navigator.clipboard.writeText(text).then(() => {
                osToast('Secret key copied and hidden');
                window.osResetCreatedAccessKeyResult();
            });
            return;
        }

        if (el) {
            el.select();
            document.execCommand('copy');
            osToast('Secret key copied and hidden');
            window.osResetCreatedAccessKeyResult();
        }
    };

    window.osRevokeAccessKey = async function(keyId) {
        if (!keyId) { osToast('Invalid access key', true); return; }
        if (!confirm('Revoke access key ' + keyId + '?')) return;

        try {
            const resp = await osFetch('/access-keys/' + encodeURIComponent(keyId), { method: 'DELETE' });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                osToast(data.error || 'Failed to revoke access key', true);
                return;
            }

            osToast('Access key revoked');
            await window.osLoadAccessKeys();

            const shareSetup = document.getElementById('osShareSetupModal');
            if (shareSetup && shareSetup.classList.contains('show') && typeof window.osShareReloadAccessKeys === 'function') {
                await window.osShareReloadAccessKeys('');
            }
        } catch (e) {
            osToast('Error: ' + e.message, true);
        }
    };

    window.osDeleteAccessKey = async function(keyId) {
        if (!keyId) { osToast('Invalid access key', true); return; }
        if (!confirm('Permanently delete access key ' + keyId + '? This cannot be undone.')) return;

        try {
            const resp = await osFetch('/access-keys/' + encodeURIComponent(keyId) + '/permanent', { method: 'DELETE' });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                osToast(data.error || 'Failed to delete access key', true);
                return;
            }

            osToast('Access key deleted');
            await window.osLoadAccessKeys();

            const shareSetup = document.getElementById('osShareSetupModal');
            if (shareSetup && shareSetup.classList.contains('show') && typeof window.osShareReloadAccessKeys === 'function') {
                await window.osShareReloadAccessKeys('');
            }
        } catch (e) {
            osToast('Error: ' + e.message, true);
        }
    };

    // ===== Policies =====
    window.osLoadPolicies = async function() {
        try {
            const resp = await osFetch('/policies');
            if (!resp.ok) { osToast('Failed to load policies', true); return; }
            const data = await resp.json();
            osPoliciesCache = Array.isArray(data) ? data : (Array.isArray(data.policies) ? data.policies : []);
            renderPolicies(osPoliciesCache);
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    function renderPolicies(policies) {
        const tbody = document.getElementById('osPoliciesBody');
        if (!policies.length) {
            tbody.innerHTML = '<tr><td colspan="6" class="os-empty">No access policies defined</td></tr>';
            return;
        }
        tbody.innerHTML = policies.map((p, idx) => {
            return '<tr>' +
                '<td>' + escHtml(p.tenantId) + '</td>' +
                '<td>' + escHtml(p.userId) + '</td>' +
                '<td class="os-mono">' + escHtml(p.bucketName) + '</td>' +
                '<td><span class="os-badge os-badge-enabled">' + escHtml(p.role) + '</span></td>' +
                '<td class="os-mono">' + escHtml(p.prefix || '*') + '</td>' +
                '<td style="white-space:nowrap;">' +
                    '<button class="os-btn os-btn-secondary os-btn-sm" data-idx="' + idx + '" onclick="osViewPolicyDocByIndex(this.dataset.idx)">View</button> ' +
                    '<button class="os-btn os-btn-danger os-btn-sm" data-tenant="' + escHtml(p.tenantId || '') + '" data-user="' + escHtml(p.userId || '') + '" data-bucket="' + escHtml(p.bucketName || '') + '" onclick="osDeletePolicyFromButton(this)">Delete</button>' +
                '</td></tr>';
        }).join('');
    }

    window.osShowCreatePolicy = function() { osOpenModal('osCreatePolicyModal'); };

    window.osCreatePolicy = async function() {
        const body = {
            tenantId: document.getElementById('osNewPolicyTenant').value.trim(),
            userId: document.getElementById('osNewPolicyUser').value.trim(),
            bucketName: document.getElementById('osNewPolicyBucket').value.trim(),
            role: document.getElementById('osNewPolicyRole').value,
            prefix: document.getElementById('osNewPolicyPrefix').value.trim()
        };
        if (!body.userId || !body.bucketName) { osToast('User ID and Bucket Name are required', true); return; }
        if (!body.tenantId) delete body.tenantId;
        try {
            const resp = await osFetch('/policies', { method: 'POST', body: JSON.stringify(body) });
            const data = await resp.json();
            if (!resp.ok) { osToast(data.error || 'Create failed', true); return; }
            osToast('Policy created');
            osCloseModal('osCreatePolicyModal');
            osLoadPolicies();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osDeletePolicy = async function(tenantId, userId, bucket) {
        if (!confirm('Delete this access policy?')) return;
        try {
            const resp = await osFetch('/policies/' + encodeURIComponent(tenantId) + '/' + encodeURIComponent(userId) + '/' + encodeURIComponent(bucket), { method: 'DELETE' });
            if (!resp.ok) { osToast('Delete failed', true); return; }
            osToast('Policy deleted');
            osLoadPolicies();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osDeletePolicyFromButton = function(btn) {
        if (!btn) return;
        const tenantId = btn.getAttribute('data-tenant') || '';
        const userId = btn.getAttribute('data-user') || '';
        const bucket = btn.getAttribute('data-bucket') || '';
        window.osDeletePolicy(tenantId, userId, bucket);
    };

    window.osViewPolicyDocByIndex = function(idx) {
        const n = parseInt(idx, 10);
        if (isNaN(n) || n < 0 || n >= osPoliciesCache.length) {
            osToast('Policy not found', true);
            return;
        }
        document.getElementById('osViewPolicyJSON').textContent = JSON.stringify(osPoliciesCache[n], null, 2);
        osOpenModal('osViewPolicyModal');
    };

    window.osViewPolicyDoc = function(jsonStr) {
        try {
            const parsed = JSON.parse(jsonStr);
            document.getElementById('osViewPolicyJSON').textContent = typeof parsed === 'string' ? parsed : JSON.stringify(parsed, null, 2);
        } catch(e) {
            document.getElementById('osViewPolicyJSON').textContent = jsonStr;
        }
        osOpenModal('osViewPolicyModal');
    };

    // ===== Metrics =====
    window.osLoadMetrics = async function() {
        try {
            const [metricsResp, bucketsResp] = await Promise.all([
                osFetch('/metrics'),
                osFetch('/buckets')
            ]);
            if (!metricsResp.ok) { osToast('Failed to load metrics', true); return; }

            const m = await metricsResp.json();
            setText('osMetricRequests', m.totalRequests || 0);
            setText('osMetricBytesIn', fmtSize(m.totalBytesIn));
            setText('osMetricBytesOut', fmtSize(m.totalBytesOut));
            setText('osMetricErrors', m.totalErrors || 0);
            const avgMs = m.totalRequests > 0 ? (m.avgLatencyMs || 0).toFixed(1) : '0';
            setText('osMetricAvgLatency', avgMs + ' ms');
            setText('osMetricUptime', m.uptime || '-');

            const tbody = document.getElementById('osMetricsBucketBody');
            if (!bucketsResp.ok) {
                tbody.innerHTML = '<tr><td colspan="10" style="text-align:center; padding:20px; color:var(--text-muted);">Unable to load bucket list</td></tr>';
                return;
            }

            const buckets = await bucketsResp.json();
            const bucketList = Array.isArray(buckets) ? buckets : [];
            if (!bucketList.length) {
                tbody.innerHTML = '<tr><td colspan="10" style="text-align:center; padding:20px; color:var(--text-muted);">No per-bucket metrics yet</td></tr>';
                return;
            }

            const metricRows = await Promise.all(bucketList.map(async (b) => {
                const name = (b.metadata && b.metadata.name) ? b.metadata.name : '';
                const tenantId = (b.metadata && b.metadata.tenantId) ? b.metadata.tenantId : 'default';
                if (!name) return null;
                const resp = await osFetch('/metrics/' + encodeURIComponent(name) + '?tenantId=' + encodeURIComponent(tenantId));
                if (!resp.ok) return null;
                const bm = await resp.json();
                return {
                    bucketName: bm.bucketName || name,
                    requestCount: bm.requestCount || 0,
                    getRequests: bm.getRequests || 0,
                    putRequests: bm.putRequests || 0,
                    deleteRequests: bm.deleteRequests || 0,
                    bytesIn: bm.bytesIn || 0,
                    bytesOut: bm.bytesOut || 0,
                    errorCount: bm.errorCount || 0,
                    avgLatencyMs: Number(bm.avgLatencyMs || 0),
                    lastAccessed: bm.lastAccessed || ''
                };
            }));

            const rows = metricRows.filter(Boolean);
            if (!rows.length) {
                tbody.innerHTML = '<tr><td colspan="10" style="text-align:center; padding:20px; color:var(--text-muted);">No per-bucket metrics available</td></tr>';
                return;
            }

            rows.sort((a, b) => b.requestCount - a.requestCount);
            tbody.innerHTML = rows.map((b) => {
                const bAvg = b.requestCount > 0 ? b.avgLatencyMs.toFixed(1) : '0';
                return '<tr>' +
                    '<td><strong>' + escHtml(b.bucketName) + '</strong></td>' +
                    '<td>' + b.requestCount + '</td>' +
                    '<td>' + b.getRequests + '</td>' +
                    '<td>' + b.putRequests + '</td>' +
                    '<td>' + b.deleteRequests + '</td>' +
                    '<td class="os-size">' + fmtSize(b.bytesIn) + '</td>' +
                    '<td class="os-size">' + fmtSize(b.bytesOut) + '</td>' +
                    '<td style="color:#ef4444;">' + b.errorCount + '</td>' +
                    '<td>' + bAvg + ' ms</td>' +
                    '<td>' + fmtDate(b.lastAccessed) + '</td>' +
                '</tr>';
            }).join('');
        } catch(e) { osToast('Error loading metrics: ' + e.message, true); }

        // Load SafeGate scanner health metrics
        osLoadScanMetrics();
    };

    async function osLoadScanMetrics() {
        const badge = document.getElementById('osScanHealthBadge');
        const tbody = document.getElementById('osScannerPerfBody');
        try {
            // Use the storage scanner health endpoint (same API base as storage)
            const resp = await fetch(OS_API + '/scanner/health?metrics=true', {
                headers: osHeaders()
            });
            if (!resp.ok) {
                if (badge) { badge.textContent = 'Unavailable'; badge.style.background = '#ef444422'; badge.style.color = '#ef4444'; }
                if (tbody) tbody.innerHTML = '<tr><td colspan="8" style="text-align:center; padding:16px; color:var(--text-muted);">Scanner health endpoint unavailable</td></tr>';
                return;
            }
            const data = await resp.json();
            const h = data.health || {};

            // Health badge
            if (badge) {
                const status = (h.status || 'unknown').toLowerCase();
                const statusColors = { healthy: '#22c55e', degraded: '#f59e0b', unavailable: '#ef4444' };
                const color = statusColors[status] || '#94a3b8';
                badge.textContent = status.charAt(0).toUpperCase() + status.slice(1);
                badge.style.background = color + '22';
                badge.style.color = color;
            }

            // Throughput cards — metrics is the MetricsSnapshot
            var metrics = h.metrics || {};
            var total = metrics.total_scans || h.total_scans || 0;
            var safe = metrics.total_safe || 0;
            var unsafe = metrics.total_unsafe || 0;
            setText('osScanTotal', total);
            setText('osScanSafe', safe);
            setText('osScanUnsafe', unsafe);

            // Severity distribution
            var sev = metrics.findings_by_severity || {};
            var critical = sev.critical || 0;
            var high = sev.high || 0;
            var medium = sev.medium || 0;
            var low = sev.low || 0;
            var info = sev.info || 0;
            var sevTotal = critical + high + medium + low + info;

            setText('osSevCriticalN', critical);
            setText('osSevHighN', high);
            setText('osSevMediumN', medium);
            setText('osSevLowN', low);
            setText('osSevInfoN', info);

            if (sevTotal > 0) {
                document.getElementById('osSevCritical').style.width = (critical / sevTotal * 100) + '%';
                document.getElementById('osSevHigh').style.width = (high / sevTotal * 100) + '%';
                document.getElementById('osSevMedium').style.width = (medium / sevTotal * 100) + '%';
                document.getElementById('osSevLow').style.width = (low / sevTotal * 100) + '%';
                document.getElementById('osSevInfo').style.width = (info / sevTotal * 100) + '%';
            } else {
                ['osSevCritical','osSevHigh','osSevMedium','osSevLow','osSevInfo'].forEach(function(id) {
                    document.getElementById(id).style.width = '0%';
                });
            }

            // Per-scanner performance table — scanners is an array
            var scannerList = (metrics.scanners && Array.isArray(metrics.scanners)) ? metrics.scanners : [];
            if (!scannerList.length) {
                if (tbody) tbody.innerHTML = '<tr><td colspan="8" style="text-align:center; padding:16px; color:var(--text-muted);">No scanner activity recorded yet</td></tr>';
                return;
            }

            scannerList.sort(function(a, b) { return (b.total_runs || 0) - (a.total_runs || 0); });
            if (tbody) {
                tbody.innerHTML = scannerList.map(function(s) {
                    var runs = s.total_runs || 0;
                    var findings = s.total_findings || 0;
                    var errors = s.total_errors || 0;
                    var timeouts = s.total_timeouts || 0;
                    var avgMs = s.avg_ms != null ? Number(s.avg_ms).toFixed(1) : '0';
                    var totalMs = s.total_ms || 0;
                    var errColor = errors > 0 ? '#ef4444' : 'inherit';
                    var toColor = timeouts > 0 ? '#f59e0b' : 'inherit';
                    var findColor = findings > 0 ? '#8b5cf6' : 'inherit';
                    return '<tr>' +
                        '<td><strong>' + escHtml(s.name || '-') + '</strong></td>' +
                        '<td>' + runs + '</td>' +
                        '<td style="color:' + findColor + ';">' + findings + '</td>' +
                        '<td style="color:' + errColor + ';">' + errors + '</td>' +
                        '<td style="color:' + toColor + ';">' + timeouts + '</td>' +
                        '<td>' + avgMs + ' ms</td>' +
                        '<td>' + totalMs + ' ms</td>' +
                        '<td>' + (runs > 0 ? (totalMs / runs).toFixed(1) : '0') + ' ms/scan</td>' +
                    '</tr>';
                }).join('');
            }
        } catch (e) {
            if (badge) { badge.textContent = 'Error'; badge.style.background = '#ef444422'; badge.style.color = '#ef4444'; }
            if (tbody) tbody.innerHTML = '<tr><td colspan="8" style="text-align:center; padding:16px; color:var(--text-muted);">Error loading scan metrics</td></tr>';
        }
    }

    function fmtDuration(sec) {
        if (!sec || sec <= 0) return '-';
        const d = Math.floor(sec / 86400);
        const h = Math.floor((sec % 86400) / 3600);
        const m = Math.floor((sec % 3600) / 60);
        if (d > 0) return d + 'd ' + h + 'h';
        if (h > 0) return h + 'h ' + m + 'm';
        return m + 'm ' + Math.floor(sec % 60) + 's';
    }

    // ===== Events =====
    window.osLoadEvents = async function() {
        const evType = document.getElementById('osEventsTypeFilter').value;
        const limit = document.getElementById('osEventsLimitFilter').value || '100';
        let qs = '?limit=' + limit;
        if (evType) qs += '&type=' + encodeURIComponent(evType);
        try {
            const resp = await osFetch('/events' + qs);
            if (!resp.ok) {
                document.getElementById('osEventsBody').innerHTML = '<tr><td colspan="7" style="text-align:center; padding:30px; color:var(--text-muted);">Failed to load events</td></tr>';
                return;
            }
            const payload = await resp.json();
            const evts = Array.isArray(payload) ? payload : (payload && Array.isArray(payload.events) ? payload.events : []);
            renderEvents(evts);
        } catch(e) { osToast('Error loading events: ' + e.message, true); }
    };

    function eventBadge(type) {
        const colors = {
            'bucket.created': '#22c55e', 'bucket.deleted': '#ef4444',
            'object.uploaded': '#60a5fa', 'object.downloaded': '#a78bfa',
            'object.deleted': '#ef4444', 'object.copied': '#f59e0b',
            'object.multi-deleted': '#ef4444', 'policy.created': '#22c55e',
            'policy.deleted': '#ef4444', 'presign.generated': '#06b6d4',
            'object.shared': '#8b5cf6'
        };
        const color = colors[type] || '#94a3b8';
        return '<span class="os-badge" style="background:' + color + '22; color:' + color + ';">' + escHtml(type) + '</span>';
    }

    function renderEvents(events) {
        const tbody = document.getElementById('osEventsBody');
        if (!events.length) {
            tbody.innerHTML = '<tr><td colspan="7" style="text-align:center; padding:30px; color:var(--text-muted);">No events found</td></tr>';
            return;
        }
        tbody.innerHTML = events.map(e => {
            return '<tr>' +
                '<td style="white-space:nowrap;">' + fmtDate(e.timestamp) + '</td>' +
                '<td>' + eventBadge(e.type) + '</td>' +
                '<td>' + escHtml(e.tenantId || '-') + '</td>' +
                '<td class="os-mono">' + escHtml(e.bucket || '-') + '</td>' +
                '<td class="os-mono" style="max-width:200px; overflow:hidden; text-overflow:ellipsis;">' + escHtml(e.key || '-') + '</td>' +
                '<td class="os-size">' + (e.size ? fmtSize(e.size) : '-') + '</td>' +
                '<td>' + escHtml(e.details || '-') + '</td>' +
            '</tr>';
        }).join('');
    }

    // ===== Bucket Tags =====
    let osCurrentTagsBucket = '';
    let osCurrentTagsTenant = '';
    let osCurrentTags = [];

    window.osShowBucketTags = async function(bucket, tenantId) {
        osCurrentTagsBucket = bucket;
        osCurrentTagsTenant = tenantId;
        document.getElementById('osTagsBucketName').textContent = bucket;
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(bucket) + '/tags?tenantId=' + encodeURIComponent(tenantId));
            if (resp.ok) {
                const data = await resp.json();
                osCurrentTags = data.tags || [];
            } else {
                osCurrentTags = [];
            }
        } catch(e) { osCurrentTags = []; }
        renderTags();
        osOpenModal('osBucketTagsModal');
    };

    function renderTags() {
        const el = document.getElementById('osTagsList');
        if (!osCurrentTags.length) {
            el.innerHTML = '<p style="color:var(--text-muted); font-size:.9rem;">No tags set</p>';
            return;
        }
        el.innerHTML = '<table class="os-table"><thead><tr><th>Key</th><th>Value</th><th></th></tr></thead><tbody>' +
            osCurrentTags.map((t, i) => '<tr><td class="os-mono">' + escHtml(t.key) + '</td><td class="os-mono">' + escHtml(t.value) + '</td><td><button class="os-btn os-btn-danger os-btn-sm" onclick="osRemoveTag(' + i + ')">Ã—</button></td></tr>').join('') +
            '</tbody></table>';
    }

    window.osAddTag = function() {
        const k = document.getElementById('osNewTagKey').value.trim();
        const v = document.getElementById('osNewTagValue').value.trim();
        if (!k) { osToast('Tag key required', true); return; }
        osCurrentTags.push({ key: k, value: v });
        document.getElementById('osNewTagKey').value = '';
        document.getElementById('osNewTagValue').value = '';
        renderTags();
    };

    window.osRemoveTag = function(idx) {
        osCurrentTags.splice(idx, 1);
        renderTags();
    };

    window.osSaveTags = async function() {
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(osCurrentTagsBucket) + '/tags?tenantId=' + encodeURIComponent(osCurrentTagsTenant), {
                method: 'PUT',
                body: JSON.stringify({ tags: osCurrentTags })
            });
            if (!resp.ok) { const d = await resp.json().catch(()=>({})); osToast(d.error || 'Save tags failed', true); return; }
            osToast('Tags saved');
            osCloseModal('osBucketTagsModal');
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osDeleteAllTags = async function() {
        if (!confirm('Delete all tags from "' + osCurrentTagsBucket + '"?')) return;
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(osCurrentTagsBucket) + '/tags?tenantId=' + encodeURIComponent(osCurrentTagsTenant), { method: 'DELETE' });
            if (!resp.ok) { osToast('Delete tags failed', true); return; }
            osToast('Tags deleted');
            osCurrentTags = [];
            renderTags();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    // ===== Batch Delete =====
    window.osToggleSelectAll = function(checked) {
        document.querySelectorAll('.os-obj-check').forEach(cb => { cb.checked = checked; });
    };

    function osGetSelectedKeys() {
        const keys = [];
        document.querySelectorAll('.os-obj-check:checked').forEach(cb => keys.push(cb.getAttribute('data-key')));
        return keys;
    }

    window.osBatchDelete = async function() {
        const keys = osGetSelectedKeys();
        if (!keys.length) { osToast('No objects selected', true); return; }
        if (!confirm('Delete ' + keys.length + ' selected object(s)?')) return;
        const bucket = window._osBrowserBucket;
        const tenantId = window._osBrowserTenant || 'default';
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(bucket) + '/multi-delete?tenantId=' + encodeURIComponent(tenantId), {
                method: 'POST',
                body: JSON.stringify({ keys: keys })
            });
            osCaptureRateHeaders(bucket, tenantId, 'POST', resp);
            if (!resp.ok) { const d = await resp.json().catch(()=>({})); osToast(d.error || 'Batch delete failed', true); return; }
            const data = await resp.json();
            osToast('Deleted ' + (data.deleted || 0) + ' of ' + (data.total || keys.length) + ' objects');
            document.getElementById('osSelectAll').checked = false;
            osBrowseBucket();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    // ===== Copy Object =====
    let osCopySrcBucket = '';
    let osCopySrcKey = '';
    let osCopySrcTenant = '';

    window.osShowCopySingle = function(bucket, key, tenantId) {
        osCopySrcBucket = bucket;
        osCopySrcKey = key;
        osCopySrcTenant = tenantId;
        document.getElementById('osCopySrc').value = bucket + '/' + key;
        document.getElementById('osCopyDestKey').value = key;
        document.getElementById('osCopyTenant').value = tenantId;
        // Populate dest bucket select
        const sel = document.getElementById('osCopyDestBucket');
        sel.innerHTML = '<option value="">Select bucket...</option>' +
            osBucketsCache.map(b => {
                const n = b.metadata?.name || '';
                return '<option value="' + escHtml(n) + '">' + escHtml(n) + '</option>';
            }).join('');
        osOpenModal('osCopyObjectModal');
    };

    window.osShowCopyModal = function() {
        const keys = osGetSelectedKeys();
        if (keys.length !== 1) { osToast('Select exactly one object to copy', true); return; }
        const bucket = window._osBrowserBucket;
        const tenantId = window._osBrowserTenant || 'default';
        osShowCopySingle(bucket, keys[0], tenantId);
    };

    window.osDoCopy = async function() {
        const destBucket = document.getElementById('osCopyDestBucket').value;
        const destKey = document.getElementById('osCopyDestKey').value.trim();
        const tenantId = (document.getElementById('osCopyTenant').value || 'default').trim();
        if (!destBucket || !destKey) { osToast('Destination bucket and key required', true); return; }
        try {
            const resp = await osFetch('/copy', {
                method: 'POST',
                body: JSON.stringify({
                    sourceBucket: osCopySrcBucket,
                    sourceKey: osCopySrcKey,
                    destBucket: destBucket,
                    destKey: destKey,
                    tenantId: tenantId
                })
            });
            if (!resp.ok) { const d = await resp.json().catch(()=>({})); osToast(d.error || 'Copy failed', true); return; }
            osToast('Object copied to ' + destBucket + '/' + destKey);
            osCloseModal('osCopyObjectModal');
            osBrowseBucket();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    // ===== Bucket Settings =====
    let osSettingsBucketName = '';
    let osSettingsTenantId = '';

    function osRenderAppAccessEndpoints(bucket, tenantId) {
        const tbody = document.getElementById('osAppAccessEndpointsBody');
        if (!tbody) return;

        const t = String(tenantId || 'default').trim() || 'default';
        const bucketPath = '/buckets/' + encodeURIComponent(bucket);
        const tenantQS = '?tenantId=' + encodeURIComponent(t);

        const rows = [
            { method: 'GET', purpose: 'List objects in bucket', path: bucketPath + '/objects' + tenantQS },
            { method: 'GET', purpose: 'Download object', path: bucketPath + '/objects/{objectKey}' + tenantQS },
            { method: 'PUT', purpose: 'Upload or overwrite object', path: bucketPath + '/objects/{objectKey}' + tenantQS },
            { method: 'DELETE', purpose: 'Delete object', path: bucketPath + '/objects/{objectKey}' + tenantQS },
            { method: 'POST', purpose: 'Generate pre-signed URL', path: bucketPath + '/presign' + tenantQS },
            { method: 'POST', purpose: 'Generate share URL', path: bucketPath + '/share-object' + tenantQS }
        ];

        tbody.innerHTML = rows.map(r => {
            return '<tr>' +
                '<td><span class="os-badge os-badge-enabled">' + r.method + '</span></td>' +
                '<td>' + escHtml(r.purpose) + '</td>' +
                '<td class="os-mono" style="max-width:420px; overflow:hidden; text-overflow:ellipsis;">' + escHtml(osFullURL(r.path)) + '</td>' +
            '</tr>';
        }).join('');

        setText('osAppEndpointBucket', bucket);
        setText('osAppEndpointTenant', t);
        const headerEl = document.getElementById('osAppEndpointAuthHeaders');
        if (headerEl) {
            headerEl.textContent = 'X-Storage-Access-Key: <accessKeyId>\nX-Storage-Secret-Key: <secretAccessKey>';
        }
    }

    function osFormatOpsPerMinuteLabel(configured, effective) {
        const cfg = Number(configured || 0);
        const eff = Number(effective || 0);
        if (cfg > 0) return cfg + '/min';
        if (eff > 0) return 'Default (' + eff + '/min)';
        return 'Unlimited';
    }

    window.osLoadBucketSettings = async function() {
        const sel = document.getElementById('osSettingsBucket');
        const bucket = sel.value;
        if (!bucket) {
            document.getElementById('osSettingsContent').style.display = 'none';
            document.getElementById('osSettingsPlaceholder').style.display = '';
            return;
        }
        const opt = sel.options[sel.selectedIndex];
        const tenantId = opt.getAttribute('data-tenant') || 'default';
        osSettingsBucketName = bucket;
        osSettingsTenantId = tenantId;
        document.getElementById('osSettingsContent').style.display = '';
        document.getElementById('osSettingsPlaceholder').style.display = 'none';
        osRenderAppAccessEndpoints(bucket, tenantId);

        const qs = '?tenantId=' + encodeURIComponent(tenantId);

        // Load all settings in parallel
        const [encResp, lockResp, corsResp, quotaResp, rateResp, notifResp, policyResp] = await Promise.allSettled([
            osFetch('/buckets/' + encodeURIComponent(bucket) + '/encryption' + qs),
            osFetch('/buckets/' + encodeURIComponent(bucket) + '/object-lock' + qs),
            osFetch('/buckets/' + encodeURIComponent(bucket) + '/cors' + qs),
            osFetch('/buckets/' + encodeURIComponent(bucket) + '/quota' + qs),
            osFetch('/buckets/' + encodeURIComponent(bucket) + '/rate-limit' + qs),
            osFetch('/buckets/' + encodeURIComponent(bucket) + '/notifications' + qs),
            osFetch('/buckets/' + encodeURIComponent(bucket) + '/policy' + qs)
        ]);

        // Encryption
        if (encResp.status === 'fulfilled' && encResp.value.ok) {
            const encData = await encResp.value.json();
            const enc = encData.encryption || {};
            document.getElementById('osEncAlgo').value = enc.algorithm || '';
            document.getElementById('osEncKeyId').value = enc.kmsKeyId || '';
            document.getElementById('osEncCurrent').textContent = enc.enabled ? (enc.algorithm + (enc.kmsKeyId ? ' (' + enc.kmsKeyId + ')' : '')) : 'Not Encrypted';
        } else {
            document.getElementById('osEncCurrent').textContent = 'Not Encrypted';
            document.getElementById('osEncAlgo').value = '';
            document.getElementById('osEncKeyId').value = '';
        }

        // Object Lock
        if (lockResp.status === 'fulfilled' && lockResp.value.ok) {
            const lockData = await lockResp.value.json();
            const lock = lockData.objectLock || {};
            document.getElementById('osLockMode').value = lock.mode || '';
            document.getElementById('osLockDays').value = lock.retentionDays || 0;
            document.getElementById('osLockCurrent').textContent = lock.enabled ? (lock.mode + ' (' + (lock.retentionDays || 0) + ' days)') : 'Disabled';
        } else {
            document.getElementById('osLockCurrent').textContent = 'Disabled';
            document.getElementById('osLockMode').value = '';
            document.getElementById('osLockDays').value = 0;
        }

        // CORS
        if (corsResp.status === 'fulfilled' && corsResp.value.ok) {
            const corsData = await corsResp.value.json();
            const rules = corsData.cors || [];
            if (rules.length > 0) {
                document.getElementById('osCorsOrigins').value = (rules[0].allowedOrigins || []).join('\n');
                document.getElementById('osCorsMethods').value = (rules[0].allowedMethods || []).join(', ');
                document.getElementById('osCorsHeaders').value = (rules[0].allowedHeaders || []).join(', ');
            } else {
                document.getElementById('osCorsOrigins').value = '';
                document.getElementById('osCorsMethods').value = 'GET, PUT, DELETE, HEAD';
                document.getElementById('osCorsHeaders').value = '*';
            }
        } else {
            document.getElementById('osCorsOrigins').value = '';
            document.getElementById('osCorsMethods').value = 'GET, PUT, DELETE, HEAD';
            document.getElementById('osCorsHeaders').value = '*';
        }

        // Quota
        if (quotaResp.status === 'fulfilled' && quotaResp.value.ok) {
            const q = await quotaResp.value.json();
            setText('osQuotaLimit', q.quotaBytes ? fmtSize(q.quotaBytes) : 'Unlimited');
            setText('osQuotaUsage', fmtSize(q.usedBytes || 0));
            setText('osQuotaObjects', q.objectCount || 0);
        } else {
            setText('osQuotaLimit', 'Unlimited');
            setText('osQuotaUsage', '-');
            setText('osQuotaObjects', '-');
        }

        // Object Operation Rate Limits
        if (rateResp.status === 'fulfilled' && rateResp.value.ok) {
            const rl = await rateResp.value.json();
            const readConfigured = Number(rl.readOpsPerMinute || 0);
            const writeConfigured = Number(rl.writeOpsPerMinute || 0);
            const readEffective = Number(rl.effectiveReadOpsPerMinute || 0);
            const writeEffective = Number(rl.effectiveWriteOpsPerMinute || 0);

            document.getElementById('osRateReadLimit').value = readConfigured;
            document.getElementById('osRateWriteLimit').value = writeConfigured;
            setText('osRateReadCurrent', osFormatOpsPerMinuteLabel(readConfigured, readEffective));
            setText('osRateWriteCurrent', osFormatOpsPerMinuteLabel(writeConfigured, writeEffective));
        } else {
            document.getElementById('osRateReadLimit').value = 0;
            document.getElementById('osRateWriteLimit').value = 0;
            setText('osRateReadCurrent', '-');
            setText('osRateWriteCurrent', '-');
        }

        // Notifications
        if (notifResp.status === 'fulfilled' && notifResp.value.ok) {
            const nData = await notifResp.value.json();
            const rules = (nData.notifications && nData.notifications.rules) || [];
            if (rules.length > 0) {
                document.getElementById('osNotifCurrent').innerHTML = '<p style="color:var(--text-muted); font-size:.9rem;">' + rules.length + ' notification rule(s) configured</p>';
                const first = rules[0] || {};
                document.getElementById('osNotifURL').value = first.url || '';
                document.getElementById('osNotifEvents').value = (first.events || []).join(',');
            } else {
                document.getElementById('osNotifCurrent').innerHTML = '<p style="color:var(--text-muted); font-size:.9rem;">No notifications configured</p>';
                document.getElementById('osNotifURL').value = '';
            }
        } else {
            document.getElementById('osNotifCurrent').innerHTML = '<p style="color:var(--text-muted); font-size:.9rem;">No notifications configured</p>';
            document.getElementById('osNotifURL').value = '';
        }

        // Bucket Policy
        if (policyResp.status === 'fulfilled' && policyResp.value.ok) {
            const p = await policyResp.value.json();
            document.getElementById('osBucketPolicyJSON').value = p.policy ? (typeof p.policy === 'string' ? p.policy : JSON.stringify(p.policy, null, 2)) : '';
        } else {
            document.getElementById('osBucketPolicyJSON').value = osBuildBucketPolicyTemplate(bucket);
        }
    };

    function osSettingsQS() {
        return '?tenantId=' + encodeURIComponent(osSettingsTenantId);
    }

    window.osSaveBucketRateLimit = async function() {
        const readRaw = (document.getElementById('osRateReadLimit').value || '').trim();
        const writeRaw = (document.getElementById('osRateWriteLimit').value || '').trim();

        const readLimit = readRaw === '' ? 0 : parseInt(readRaw, 10);
        const writeLimit = writeRaw === '' ? 0 : parseInt(writeRaw, 10);

        if (isNaN(readLimit) || readLimit < 0 || isNaN(writeLimit) || writeLimit < 0) {
            osToast('Read/Write limits must be integers >= 0', true);
            return;
        }

        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(osSettingsBucketName) + '/rate-limit' + osSettingsQS(), {
                method: 'PUT',
                body: JSON.stringify({
                    readOpsPerMinute: readLimit,
                    writeOpsPerMinute: writeLimit
                })
            });
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                osToast(data.error || 'Failed to save rate limits', true);
                return;
            }
            osToast('Bucket rate limits updated');
            osLoadBucketSettings();
        } catch (e) {
            osToast('Error: ' + e.message, true);
        }
    };

    window.osSaveEncryption = async function() {
        const algo = document.getElementById('osEncAlgo').value;
        if (!algo) { osToast('Select an encryption algorithm', true); return; }
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(osSettingsBucketName) + '/encryption' + osSettingsQS(), {
                method: 'PUT',
                body: JSON.stringify({ enabled: true, algorithm: algo, kmsKeyId: document.getElementById('osEncKeyId').value.trim() })
            });
            if (!resp.ok) { const d = await resp.json().catch(()=>({})); osToast(d.error || 'Failed', true); return; }
            osToast('Encryption updated');
            osLoadBucketSettings();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osRemoveEncryption = async function() {
        if (!confirm('Remove encryption from "' + osSettingsBucketName + '"?')) return;
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(osSettingsBucketName) + '/encryption' + osSettingsQS(), { method: 'DELETE' });
            if (!resp.ok) { osToast('Failed to remove encryption', true); return; }
            osToast('Encryption removed');
            osLoadBucketSettings();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osSaveObjectLock = async function() {
        const mode = document.getElementById('osLockMode').value;
        const days = parseInt(document.getElementById('osLockDays').value) || 0;
        const enabled = !!mode;
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(osSettingsBucketName) + '/object-lock' + osSettingsQS(), {
                method: 'PUT',
                body: JSON.stringify({ enabled: enabled, mode: mode, retentionDays: days })
            });
            if (!resp.ok) { const d = await resp.json().catch(()=>({})); osToast(d.error || 'Failed', true); return; }
            osToast('Object lock updated');
            osLoadBucketSettings();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osSaveCORS = async function() {
        const origins = document.getElementById('osCorsOrigins').value.split('\n').map(s => s.trim()).filter(Boolean);
        const methods = document.getElementById('osCorsMethods').value.split(',').map(s => s.trim()).filter(Boolean);
        const headers = document.getElementById('osCorsHeaders').value.split(',').map(s => s.trim()).filter(Boolean);
        if (!origins.length) { osToast('At least one origin required', true); return; }
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(osSettingsBucketName) + '/cors' + osSettingsQS(), {
                method: 'PUT',
                body: JSON.stringify({ rules: [{ allowedOrigins: origins, allowedMethods: methods, allowedHeaders: headers }] })
            });
            if (!resp.ok) { const d = await resp.json().catch(()=>({})); osToast(d.error || 'Failed', true); return; }
            osToast('CORS updated');
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osRemoveCORS = async function() {
        if (!confirm('Remove CORS configuration?')) return;
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(osSettingsBucketName) + '/cors' + osSettingsQS(), { method: 'DELETE' });
            if (!resp.ok) { osToast('Failed', true); return; }
            osToast('CORS removed');
            document.getElementById('osCorsOrigins').value = '';
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osSaveNotifications = async function() {
        const url = document.getElementById('osNotifURL').value.trim();
        const events = document.getElementById('osNotifEvents').value.split(',').map(s => s.trim()).filter(Boolean);
        if (!url) { osToast('Webhook URL required', true); return; }
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(osSettingsBucketName) + '/notifications' + osSettingsQS(), {
                method: 'PUT',
                body: JSON.stringify({
                    rules: [{
                        id: 'rule-' + Date.now(),
                        events: events,
                        target: 'webhook',
                        url: url
                    }]
                })
            });
            if (!resp.ok) { const d = await resp.json().catch(()=>({})); osToast(d.error || 'Failed', true); return; }
            osToast('Notifications saved');
            osLoadBucketSettings();
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osSaveBucketPolicy = async function() {
        const policyText = document.getElementById('osBucketPolicyJSON').value.trim();
        if (!policyText) { osToast('Enter a policy document', true); return; }
        let policy;
        try { policy = JSON.parse(policyText); } catch(e) { osToast('Invalid JSON: ' + e.message, true); return; }
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(osSettingsBucketName) + '/policy' + osSettingsQS(), {
                method: 'PUT',
                body: JSON.stringify(policy)
            });
            if (!resp.ok) { const d = await resp.json().catch(()=>({})); osToast(d.error || 'Failed', true); return; }
            osToast('Bucket policy saved');
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    window.osUseBucketPolicyTemplate = function() {
        if (!osSettingsBucketName) {
            osToast('Select a bucket first', true);
            return;
        }
        document.getElementById('osBucketPolicyJSON').value = osBuildBucketPolicyTemplate(osSettingsBucketName);
        osToast('Template inserted');
    };

    window.osCopyDashboardPolicyTemplate = function() {
        const el = document.getElementById('osDashboardPolicyTemplate');
        const text = el ? (el.value || '').trim() : '';
        if (!text) {
            osToast('No template available to copy', true);
            return;
        }
        if (navigator.clipboard) {
            navigator.clipboard.writeText(text).then(() => osToast('Policy template copied'));
            return;
        }
        if (el) {
            el.select();
            document.execCommand('copy');
            osToast('Policy template copied');
        }
    };

    window.osRemoveBucketPolicy = async function() {
        if (!confirm('Remove bucket policy?')) return;
        try {
            const resp = await osFetch('/buckets/' + encodeURIComponent(osSettingsBucketName) + '/policy' + osSettingsQS(), { method: 'DELETE' });
            if (!resp.ok) { osToast('Failed', true); return; }
            osToast('Bucket policy removed');
            document.getElementById('osBucketPolicyJSON').value = '';
        } catch(e) { osToast('Error: ' + e.message, true); }
    };

    // ===== Init =====
    async function osInit() {
        await osLoadDashboard();
        await osLoadBuckets();
        osPopulateBucketSelects();
        osPopulateSettingsBuckets();
        osRefreshBucketNameSuggestions();
        osRenderAccessKeyScopeSuggestions();
        osBindBucketHoverEvents();

        const newBucketInput = document.getElementById('osNewBucketName');
        if (newBucketInput) {
            newBucketInput.addEventListener('input', function() { window.osValidateCreateBucketName(false); });
        }
        const newBucketTenant = document.getElementById('osNewBucketTenant');
        if (newBucketTenant) {
            newBucketTenant.addEventListener('input', function() { window.osValidateCreateBucketName(false); });
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', osInit);
    } else {
        osInit();
    }

})();
