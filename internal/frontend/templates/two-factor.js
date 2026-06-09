/**
 * Two-Factor Authentication Frontend
 * Connects to Gatekeeper MFA API at /api/v1/mfa/
 */
(function () {
    'use strict';

    var MFA_API = (window.resolveBackendURL ? window.resolveBackendURL() : (window.BACKEND_URL || 'http://localhost:8000')).replace(/\/+$/, '') + '/api/v1/mfa';
    var BUILDER_API = (window.resolveBackendURL ? window.resolveBackendURL() : (window.BACKEND_URL || 'http://localhost:8000')).replace(/\/+$/, '') + '/api/v1/builder';
    var currentUserId = null;
    var enrolledFactors = [];
    var pendingFactorId = null;
    var pendingSecret = null;

    // ── Helpers ──────────────────────────────────────────────────────────

    function getAuthToken() {
        return localStorage.getItem('iamToken') || localStorage.getItem('authToken') || '';
    }

    function mfaHeaders() {
        return {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer ' + getAuthToken()
        };
    }

    function mfaFetch(path, opts) {
        var url = MFA_API + path;
        var options = opts || {};
        options.headers = Object.assign({}, mfaHeaders(), options.headers || {});
        return fetch(url, options).then(function (resp) {
            if (!resp.ok) {
                return resp.json().catch(function () { return {}; }).then(function (body) {
                    throw new Error(body.error || body.message || 'Request failed (' + resp.status + ')');
                });
            }
            return resp.json();
        });
    }

    function escapeHTML(s) {
        var d = document.createElement('div');
        d.textContent = s || '';
        return d.innerHTML;
    }

    function toast(msg, type) {
        type = type || 'info';
        var el = document.createElement('div');
        el.className = 'tfa-toast tfa-toast-' + type;
        el.textContent = msg;
        document.body.appendChild(el);
        setTimeout(function () { el.remove(); }, 4000);
    }

    function resolveUserId() {
        if (currentUserId) return Promise.resolve(currentUserId);
        var uid = localStorage.getItem('userId') || localStorage.getItem('user_id') || '';
        if (uid) { currentUserId = uid; return Promise.resolve(uid); }
        var base = (window.resolveBackendURL ? window.resolveBackendURL() : (window.BACKEND_URL || 'http://localhost:8000')).replace(/\/+$/, '');
        return fetch(base + '/iam/auth/whoami', { headers: mfaHeaders() })
            .then(function (r) { return r.json(); })
            .then(function (data) {
                currentUserId = data.user_id || data.id || data.sub || '';
                if (!currentUserId) throw new Error('Could not determine user ID');
                localStorage.setItem('userId', currentUserId);
                return currentUserId;
            });
    }

    // ── Tab Switching ────────────────────────────────────────────────────

    window.tfaSwitch = function (tab) {
        document.querySelectorAll('.tfa-panel').forEach(function (p) { p.classList.remove('active'); });
        document.querySelectorAll('.tfa-tab').forEach(function (t) { t.classList.remove('active'); });
        var panel = document.getElementById('tfa-panel-' + tab);
        if (panel) panel.classList.add('active');
        var tabs = document.querySelectorAll('.tfa-tab');
        var tabMap = { status: 0, setup: 1, verify: 2, backup: 3, devices: 4, metrics: 5 };
        if (tabs[tabMap[tab]]) tabs[tabMap[tab]].classList.add('active');
        if (tab === 'status') tfaLoadStatus();
        if (tab === 'verify') tfaPopulateFactorSelects();
        if (tab === 'backup') tfaPopulateFactorSelects();
        if (tab === 'devices') tfaLoadTrustedDevices();
        if (tab === 'metrics') tfaLoadMetrics();
    };

    // ── Status Panel ─────────────────────────────────────────────────────

    function tfaLoadStatus() {
        resolveUserId().then(function (uid) {
            // Load factors
            mfaFetch('/factors/' + uid).then(function (data) {
                enrolledFactors = data.factors || [];
                var active = enrolledFactors.filter(function (f) { return (f.phase || (f.status || f.Status || {}).phase) === 'Active'; });
                document.getElementById('tfaFactorCount').textContent = active.length;
                renderFactorsTable(enrolledFactors);
            }).catch(function (err) {
                enrolledFactors = [];
                document.getElementById('tfaFactorCount').textContent = '0';
                var box = document.getElementById('tfaFactorsTable');
                box.innerHTML = '<div class="tfa-empty"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M12 8v4"/><path d="M12 16h.01"/></svg><p style="color:var(--text-muted,#888)">Could not load factors. The 2FA module may still be initializing.</p><p style="font-size:0.82rem;color:var(--text-muted,#666);margin-top:4px;">' + escapeHTML(err.message) + '</p></div>';
            });

            // Load policy
            mfaFetch('/policy/' + uid).then(function (data) {
                document.getElementById('tfaPolicyStatus').textContent = data.requires_mfa ? 'Required' : 'Optional';
            }).catch(function () {
                document.getElementById('tfaPolicyStatus').textContent = '-';
            });

            // Load trusted devices
            mfaFetch('/trust-device/list/' + uid).then(function (data) {
                document.getElementById('tfaDeviceCount').textContent = (data.devices || []).length;
            }).catch(function () {
                document.getElementById('tfaDeviceCount').textContent = '0';
            });

            document.getElementById('tfaBackupCount').textContent = '-';
        });
    }

    function renderFactorsTable(factors) {
        var box = document.getElementById('tfaFactorsTable');
        if (!factors || factors.length === 0) {
            box.innerHTML = '<div class="tfa-empty"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg><p>No 2FA factors enrolled yet.</p><p style="margin-top:8px;"><a href="#" onclick="tfaSwitch(\'setup\');return false;" style="color:var(--primary-color,#60a5fa)">Set up 2FA now</a></p></div>';
            return;
        }
        var html = '<table class="tfa-table"><thead><tr><th>Name</th><th>Type</th><th>Status</th><th>Issuer</th><th>Created</th><th>Last Verified</th><th>Actions</th></tr></thead><tbody>';
        factors.forEach(function (f) {
            var spec = f.spec || f.Spec || {};
            var status = f.status || f.Status || {};
            var phase = f.phase || status.phase || status.Phase || 'Unknown';
            var badgeClass = 'tfa-badge-' + phase.toLowerCase();
            var type = f.factor_type || spec.type || spec.Type || 'totp';
            var label = f.label || spec.label || spec.Label || '-';
            var issuer = f.issuer || spec.issuer || spec.Issuer || '-';
            var created = f.created_at || f.CreatedAt || '';
            var activatedAt = f.activated_at || status.activated_at || status.ActivatedAt || '';
            var factorId = f.id || f.ID || '';
            if (created) created = new Date(created).toLocaleDateString();
            if (activatedAt) activatedAt = new Date(activatedAt).toLocaleString();
            else activatedAt = 'Never';
            html += '<tr>';
            html += '<td>' + escapeHTML(label) + '</td>';
            html += '<td>' + escapeHTML(type.toUpperCase()) + '</td>';
            html += '<td><span class="tfa-badge ' + badgeClass + '">' + escapeHTML(phase) + '</span></td>';
            html += '<td>' + escapeHTML(issuer) + '</td>';
            html += '<td>' + escapeHTML(created) + '</td>';
            html += '<td>' + escapeHTML(activatedAt) + '</td>';
            html += '<td>';
            if (phase === 'Active' || phase === 'Pending' || phase === 'Disabled') {
                html += '<button class="tfa-btn tfa-btn-danger tfa-btn-sm" onclick="tfaDeleteFactor(\'' + factorId + '\')" title="Permanently delete this factor">Delete</button>';
            }
            html += '</td>';
            html += '</tr>';
        });
        html += '</tbody></table>';
        box.innerHTML = html;
    }

    // ── Delete Factor ────────────────────────────────────────────────────

    window.tfaDeleteFactor = function (factorId) {
        if (!confirm('Are you sure you want to permanently delete this 2FA factor? This cannot be undone.')) return;
        mfaFetch('/factor/' + factorId, { method: 'DELETE' }).then(function () {
            toast('Factor deleted', 'success');
            tfaLoadStatus();
        }).catch(function (err) {
            toast('Delete failed: ' + err.message, 'error');
        });
    };

    // ── Enrollment (Setup Wizard) ────────────────────────────────────────

    function generateQRCodeSVG(text) {
        if (typeof qrcode === 'undefined') {
            return '<p style="color:#c00;font-size:0.85rem;">QR library not loaded. Use the secret key below.</p>';
        }
        var qr = qrcode(0, 'M');
        qr.addData(text);
        qr.make();
        var size = 6;
        var modules = qr.getModuleCount();
        var dim = modules * size;
        var svg = '<svg xmlns="http://www.w3.org/2000/svg" width="' + dim + '" height="' + dim + '" viewBox="0 0 ' + dim + ' ' + dim + '" shape-rendering="crispEdges">';
        for (var r = 0; r < modules; r++) {
            for (var c = 0; c < modules; c++) {
                if (qr.isDark(r, c)) {
                    svg += '<rect x="' + (c * size) + '" y="' + (r * size) + '" width="' + size + '" height="' + size + '" fill="#000"/>';
                }
            }
        }
        svg += '</svg>';
        return svg;
    }

    window.tfaStartEnroll = function () {
        var factorType = document.getElementById('tfaFactorType').value;
        var label = document.getElementById('tfaFactorLabel').value.trim();
        var email = document.getElementById('tfaEnrollEmail').value.trim();
        var enrollUid = null;

        resolveUserId().then(function (uid) {
            enrollUid = uid;
            var body = { user_id: uid, factor_type: factorType };
            if (label) body.label = label;
            if (email) body.email = email;
            return mfaFetch('/enroll', { method: 'POST', body: JSON.stringify(body) });
        }).then(function (data) {
            pendingSecret = data.secret || '';
            pendingFactorId = data.factor_id || data.id || '';

            // If backend didn't return factor_id, fetch factors to find the pending one
            if (!pendingFactorId) {
                return mfaFetch('/factors/' + enrollUid).then(function (fData) {
                    var factors = fData.factors || [];
                    var pending = factors.filter(function (f) {
                        return (f.status || f.Status || {}).phase === 'Pending';
                    });
                    if (pending.length > 0) {
                        pendingFactorId = pending[0].id || pending[0].ID || '';
                    }
                    return data;
                });
            }
            return data;
        }).then(function () {
            if (!pendingFactorId) {
                toast('Enrollment created but could not determine factor ID. Try refreshing.', 'error');
                return;
            }
            document.getElementById('tfaSecretText').textContent = pendingSecret;
            // Build otpauth URI
            var issuer = 'AxiomNizam';
            var account = email || 'user';
            var uri = 'otpauth://totp/' + encodeURIComponent(issuer + ':' + account) + '?secret=' + pendingSecret + '&issuer=' + encodeURIComponent(issuer) + '&algorithm=SHA1&digits=6&period=30';
            // Generate QR code SVG
            var qrBox = document.getElementById('tfaQRCode');
            qrBox.innerHTML = generateQRCodeSVG(uri);
            tfaSetupGoto(2);
            toast('Factor enrolled! Scan the QR code.', 'success');
        }).catch(function (err) {
            toast('Enrollment failed: ' + err.message, 'error');
        });
    };

    window.tfaSetupGoto = function (step) {
        [1, 2, 3].forEach(function (n) {
            var el = document.getElementById('tfaSetupStep' + n);
            if (el) el.classList.toggle('tfa-wizard-hidden', n !== step);
            var stepEl = document.getElementById('tfaStep' + n);
            if (stepEl) {
                stepEl.classList.remove('active', 'done');
                if (n < step) stepEl.classList.add('done');
                if (n === step) stepEl.classList.add('active');
            }
        });
        document.getElementById('tfaSetupProgress').style.width = (step * 33) + '%';
    };

    window.tfaCopySecret = function () {
        if (navigator.clipboard) {
            navigator.clipboard.writeText(pendingSecret || '');
            toast('Secret copied to clipboard', 'success');
        }
    };

    window.tfaActivateFactor = function () {
        var code = document.getElementById('tfaActivateCode').value.trim();
        if (!code || code.length !== 6) {
            toast('Please enter a 6-digit code', 'error');
            return;
        }
        if (!pendingFactorId) {
            toast('No pending factor. Please restart setup.', 'error');
            return;
        }
        mfaFetch('/activate', {
            method: 'POST',
            body: JSON.stringify({ factor_id: pendingFactorId, code: code })
        }).then(function (data) {
            var codes = data.backup_codes || [];
            toast('2FA activated successfully!', 'success');
            if (codes.length > 0) {
                var display = document.getElementById('tfaBackupCodesDisplay');
                display.innerHTML = renderBackupCodesHTML(codes);
            }
            pendingFactorId = null;
            pendingSecret = null;
            tfaSwitch('status');
        }).catch(function (err) {
            toast('Activation failed: ' + err.message, 'error');
        });
    };

    // ── Verify Code ──────────────────────────────────────────────────────

    function tfaPopulateFactorSelects() {
        resolveUserId().then(function (uid) {
            mfaFetch('/factors/' + uid).then(function (data) {
                var factors = (data.factors || []).filter(function (f) {
                    return (f.phase || (f.status || f.Status || {}).phase) === 'Active';
                });
                var selects = ['tfaVerifyFactorId', 'tfaBackupFactorId'];
                selects.forEach(function (selId) {
                    var sel = document.getElementById(selId);
                    if (!sel) return;
                    var current = sel.value;
                    sel.innerHTML = '<option value="">-- Select a factor --</option>';
                    factors.forEach(function (f) {
                        var spec = f.spec || f.Spec || {};
                        var label = f.label || spec.label || spec.Label || '';
                        var typeName = (f.factor_type || spec.type || spec.Type || 'TOTP').toUpperCase();
                        var display = label ? typeName + ' — ' + label : typeName + ' — ' + (f.issuer || spec.issuer || spec.Issuer || 'Unknown');
                        var opt = document.createElement('option');
                        opt.value = f.id || f.ID;
                        opt.textContent = display;
                        sel.appendChild(opt);
                    });
                    if (current) sel.value = current;
                });
            }).catch(function () {});
        });
    }

    window.tfaBeginAndVerify = function () {
        var factorId = document.getElementById('tfaVerifyFactorId').value;
        var code = document.getElementById('tfaVerifyCode').value.trim();
        var resultBox = document.getElementById('tfaVerifyResult');
        if (!factorId) { toast('Select a factor first', 'error'); return; }
        if (!code || code.length !== 6) { toast('Enter a 6-digit code', 'error'); return; }

        resolveUserId().then(function (uid) {
            return mfaFetch('/challenge/begin', {
                method: 'POST',
                body: JSON.stringify({ user_id: uid, factor_id: factorId })
            });
        }).then(function (data) {
            var challengeId = data.challenge_id || data.id || '';
            return mfaFetch('/challenge/verify', {
                method: 'POST',
                body: JSON.stringify({ challenge_id: challengeId, code: code })
            });
        }).then(function (data) {
            if (data.verified) {
                resultBox.innerHTML = '<div style="background:rgba(34,197,94,0.1);border:1px solid rgba(34,197,94,0.3);border-radius:8px;padding:14px;color:#22c55e;">Code verified successfully! Your 2FA is working correctly.</div>';
                toast('Verification successful!', 'success');
            } else {
                resultBox.innerHTML = '<div style="background:rgba(239,68,68,0.1);border:1px solid rgba(239,68,68,0.3);border-radius:8px;padding:14px;color:#ef4444;">Verification failed. The code may have expired. Try again.</div>';
                toast('Verification failed', 'error');
            }
        }).catch(function (err) {
            resultBox.innerHTML = '<div style="background:rgba(239,68,68,0.1);border:1px solid rgba(239,68,68,0.3);border-radius:8px;padding:14px;color:#ef4444;">Error: ' + escapeHTML(err.message) + '</div>';
            toast('Verification error: ' + err.message, 'error');
        });
    };

    // ── Backup Codes ─────────────────────────────────────────────────────

    function renderBackupCodesHTML(codes) {
        var html = '<div style="margin-bottom:12px;"><strong style="color:var(--text-primary,#e0e0e0);">Your Backup Codes</strong>';
        html += '<p style="color:var(--text-muted,#888);font-size:0.82rem;margin-top:4px;">Save these codes securely. Each code can only be used once.</p></div>';
        html += '<div class="tfa-backup-grid">';
        codes.forEach(function (c) {
            html += '<div class="tfa-backup-code">' + escapeHTML(c) + '</div>';
        });
        html += '</div>';
        html += '<div class="tfa-actions"><button class="tfa-btn tfa-btn-secondary" onclick="tfaCopyBackupCodes()">Copy All Codes</button></div>';
        return html;
    }

    var lastBackupCodes = [];

    window.tfaCopyBackupCodes = function () {
        if (navigator.clipboard && lastBackupCodes.length) {
            navigator.clipboard.writeText(lastBackupCodes.join('\n'));
            toast('Backup codes copied', 'success');
        }
    };

    window.tfaRegenerateBackupCodes = function () {
        var factorId = document.getElementById('tfaBackupFactorId').value;
        if (!factorId) { toast('Select a factor first', 'error'); return; }
        mfaFetch('/backup-codes/regenerate', {
            method: 'POST',
            body: JSON.stringify({ factor_id: factorId })
        }).then(function (data) {
            var codes = data.backup_codes || data.codes || [];
            lastBackupCodes = codes;
            var display = document.getElementById('tfaBackupCodesDisplay');
            if (codes.length > 0) {
                display.innerHTML = renderBackupCodesHTML(codes);
                toast('Backup codes generated!', 'success');
            } else {
                display.innerHTML = '<div class="tfa-empty"><p>No codes returned. The endpoint may not be implemented yet.</p></div>';
            }
        }).catch(function (err) {
            toast('Failed to generate backup codes: ' + err.message, 'error');
        });
    };

    // ── Trusted Devices ──────────────────────────────────────────────────

    window.tfaLoadTrustedDevices = function () {
        var box = document.getElementById('tfaDevicesTable');
        resolveUserId().then(function (uid) {
            return mfaFetch('/trust-device/list/' + uid);
        }).then(function (data) {
            var devices = data.devices || [];
            if (devices.length === 0) {
                box.innerHTML = '<div class="tfa-empty"><svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="2"><rect x="5" y="2" width="14" height="20" rx="2"/><path d="M12 18h.01"/></svg><p>No trusted devices found.</p></div>';
                return;
            }
            var html = '<table class="tfa-table"><thead><tr><th>Device</th><th>IP Address</th><th>Fingerprint</th><th>Expires</th><th>Actions</th></tr></thead><tbody>';
            devices.forEach(function (d) {
                var ua = d.user_agent || d.UserAgent || 'Unknown';
                if (ua.length > 40) ua = ua.substring(0, 40) + '...';
                var ip = d.ip_address || d.IPAddress || '-';
                var fp = d.fingerprint || d.Fingerprint || '-';
                if (fp.length > 20) fp = fp.substring(0, 20) + '...';
                var exp = d.expires_at || d.ExpiresAt || '';
                if (exp) exp = new Date(exp).toLocaleDateString();
                html += '<tr>';
                html += '<td>' + escapeHTML(ua) + '</td>';
                html += '<td>' + escapeHTML(ip) + '</td>';
                html += '<td><code>' + escapeHTML(fp) + '</code></td>';
                html += '<td>' + escapeHTML(exp) + '</td>';
                html += '<td><button class="tfa-btn tfa-btn-danger tfa-btn-sm" onclick="tfaRevokeDevice(\'' + (d.id || d.ID) + '\')">Revoke</button></td>';
                html += '</tr>';
            });
            html += '</tbody></table>';
            box.innerHTML = html;
        }).catch(function () {
            box.innerHTML = '<div class="tfa-empty"><p>Could not load trusted devices.</p></div>';
        });
    };

    window.tfaRevokeDevice = function (deviceId) {
        if (!confirm('Revoke this trusted device?')) return;
        mfaFetch('/trust-device/' + deviceId, { method: 'DELETE' }).then(function () {
            toast('Device revoked', 'success');
            tfaLoadTrustedDevices();
        }).catch(function (err) {
            toast('Revoke failed: ' + err.message, 'error');
        });
    };

    // ── Metrics Panel ────────────────────────────────────────────────────

    function tfaLoadMetrics() {
        var box = document.getElementById('tfaMetricsContent');
        box.innerHTML = '<div style="text-align:center;padding:20px;color:var(--text-muted,#888)">Loading metrics...</div>';

        // Load scanner health for gatekeeper-related metrics
        var base = (window.resolveBackendURL ? window.resolveBackendURL() : (window.BACKEND_URL || 'http://localhost:8000')).replace(/\/+$/, '');

        resolveUserId().then(function (uid) {
            // Fetch factors for per-factor stats
            return mfaFetch('/factors/' + uid).then(function (fData) {
                var factors = fData.factors || [];
                var active = factors.filter(function (f) { return (f.phase || (f.status || f.Status || {}).phase) === 'Active'; });
                var pending = factors.filter(function (f) { return (f.phase || (f.status || f.Status || {}).phase) === 'Pending'; });
                var disabled = factors.filter(function (f) {
                    var p = f.phase || (f.status || f.Status || {}).phase;
                    return p === 'Disabled' || p === 'Revoked' || p === 'Failed';
                });

                // Build metrics HTML
                var html = '';

                // Summary cards
                html += '<div class="tfa-grid" style="margin-bottom:20px">';
                html += '<div class="tfa-stat"><div class="stat-value">' + factors.length + '</div><div class="stat-label">Total Factors</div></div>';
                html += '<div class="tfa-stat"><div class="stat-value" style="color:#22c55e">' + active.length + '</div><div class="stat-label">Active</div></div>';
                html += '<div class="tfa-stat"><div class="stat-value" style="color:#eab308">' + pending.length + '</div><div class="stat-label">Pending</div></div>';
                html += '<div class="tfa-stat"><div class="stat-value" style="color:#6b7280">' + disabled.length + '</div><div class="stat-label">Disabled / Revoked</div></div>';
                html += '</div>';

                // Per-factor breakdown
                if (factors.length > 0) {
                    html += '<div class="tfa-card"><h3>Factor Details</h3>';
                    html += '<table class="tfa-table"><thead><tr><th>Factor ID</th><th>Name</th><th>Type</th><th>Phase</th><th>Issuer</th><th>Created</th><th>Last Verified</th><th>Conditions</th></tr></thead><tbody>';
                    factors.forEach(function (f) {
                        var spec = f.spec || f.Spec || {};
                        var status = f.status || f.Status || {};
                        var conditions = status.conditions || status.Conditions || [];
                        var condStr = conditions.map(function (c) { return c.type + '=' + c.status; }).join(', ') || '-';
                        var phase = f.phase || status.phase || status.Phase || '-';
                        var badgeClass = 'tfa-badge-' + phase.toLowerCase();
                        var label = f.label || spec.label || spec.Label || '-';
                        var created = f.created_at || f.CreatedAt || '';
                        if (created) created = new Date(created).toLocaleString();
                        var activatedAt = f.activated_at || status.activated_at || status.ActivatedAt || '';
                        if (activatedAt) activatedAt = new Date(activatedAt).toLocaleString();
                        else activatedAt = 'Never';
                        html += '<tr>';
                        html += '<td><code style="font-size:0.78rem">' + escapeHTML((f.id || f.ID || '').substring(0, 8)) + '...</code></td>';
                        html += '<td>' + escapeHTML(label) + '</td>';
                        html += '<td>' + escapeHTML((f.factor_type || spec.type || spec.Type || 'totp').toUpperCase()) + '</td>';
                        html += '<td><span class="tfa-badge ' + badgeClass + '">' + escapeHTML(phase) + '</span></td>';
                        html += '<td>' + escapeHTML(f.issuer || spec.issuer || spec.Issuer || '-') + '</td>';
                        html += '<td>' + escapeHTML(created) + '</td>';
                        html += '<td>' + escapeHTML(activatedAt) + '</td>';
                        html += '<td style="font-size:0.78rem">' + escapeHTML(condStr) + '</td>';
                        html += '</tr>';
                    });
                    html += '</tbody></table></div>';
                }

                // Try to load audit events from backend
                html += '<div class="tfa-card"><h3>MFA Audit Events</h3>';
                html += '<div id="tfaAuditEvents"><div style="text-align:center;padding:12px;color:var(--text-muted,#888)">Loading...</div></div></div>';

                box.innerHTML = html;

                // Fetch audit events
                fetch(base + '/api/v1/builder/scanner/health?metrics=true', { headers: mfaHeaders() })
                    .then(function (r) { return r.json(); })
                    .then(function () {
                        // Scanner health loaded - show gatekeeper status
                        var auditBox = document.getElementById('tfaAuditEvents');
                        auditBox.innerHTML = '<p style="color:var(--text-muted,#888);font-size:0.85rem">MFA audit events are logged in the platform audit system. Check the Governance Console for detailed audit trails.</p>';
                    })
                    .catch(function () {
                        var auditBox = document.getElementById('tfaAuditEvents');
                        auditBox.innerHTML = '<p style="color:var(--text-muted,#888);font-size:0.85rem">Audit event retrieval is not available yet.</p>';
                    });
            });
        }).catch(function (err) {
            box.innerHTML = '<div class="tfa-empty"><p>Could not load metrics: ' + escapeHTML(err.message) + '</p></div>';
        });
    }

    // ── Init ─────────────────────────────────────────────────────────────

    function init() {
        tfaLoadStatus();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
