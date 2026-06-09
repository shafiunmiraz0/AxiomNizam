// AxiomNizam Landing Page Dashboard JS

const BACKEND_URL = (() => {
    if (typeof window.resolveBackendURL === 'function') {
        return window.resolveBackendURL();
    }
    const value = String(window.BACKEND_URL || '').trim();
    if (value) return value.endsWith('/') ? value.slice(0, -1) : value;
    return 'http://localhost:8000';
})();

function isAuthenticated() {
    return !!(localStorage.getItem('authToken') || readLandingCookie('authToken'));
}

function readLandingCookie(name) {
    const prefix = name + '=';
    const parts = document.cookie.split(';');
    for (let i = 0; i < parts.length; i++) {
        const item = parts[i].trim();
        if (item.startsWith(prefix)) return decodeURIComponent(item.substring(prefix.length));
    }
    return '';
}

const LANDING_SVG_ICONS = {
    category: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="1.5"></rect><rect x="14" y="3" width="7" height="7" rx="1.5"></rect><rect x="3" y="14" width="7" height="7" rx="1.5"></rect><rect x="14" y="14" width="7" height="7" rx="1.5"></rect></svg>',
    feature: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M13 2 4 14h6l-1 8 9-12h-6z"></path></svg>',
    database: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><ellipse cx="12" cy="5" rx="8" ry="3"></ellipse><path d="M4 5v14c0 1.66 3.58 3 8 3s8-1.34 8-3V5"></path><path d="M4 12c0 1.66 3.58 3 8 3s8-1.34 8-3"></path></svg>',
    architecture: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="1.5"></rect><rect x="14" y="3" width="7" height="7" rx="1.5"></rect><rect x="8.5" y="14" width="7" height="7" rx="1.5"></rect><path d="M10 6.5h4"></path><path d="M12 10v4"></path></svg>'
};

function replaceLandingEmojiIcons() {
    const landingRoot = document.querySelector('.landing-page');
    if (!landingRoot) return;

    const iconTargets = [
        ['.category-icon', LANDING_SVG_ICONS.category],
        ['.feature-card-icon', LANDING_SVG_ICONS.feature],
        ['.db-icon', LANDING_SVG_ICONS.database],
        ['.arch-icon', LANDING_SVG_ICONS.architecture]
    ];

    iconTargets.forEach(function(pair) {
        const selector = pair[0];
        const svg = pair[1];
        landingRoot.querySelectorAll(selector).forEach(function(node) {
            node.innerHTML = svg;
            node.setAttribute('aria-hidden', 'true');
        });
    });
}

// Gate feature access behind auth
function requireAuth(targetPath) {
    if (isAuthenticated()) {
        window.location.href = targetPath;
    } else {
        window.location.href = '/signup';
    }
}

// Fetch health status for the hero section
function fetchLandingHealth() {
    const el = document.getElementById('statHealth');
    if (!el) return;

    fetch('/api/health', { mode: 'cors' })
        .then(function(r) {
            if (!r.ok) throw new Error('HTTP ' + r.status);
            return r.json();
        })
        .then(function(data) {
            const status = (data.status || 'unknown').toUpperCase();
            if (status === 'OK' || status === 'HEALTHY') {
                el.innerHTML = '<span class="stat-dot stat-dot-ok"></span> Online';
            } else {
                el.innerHTML = '<span class="stat-dot stat-dot-warn"></span> ' + status;
            }
        })
        .catch(function() {
            el.innerHTML = '<span class="stat-dot stat-dot-err"></span> Offline';
        });
}

// Animate feature cards on scroll
function initScrollAnimations() {
    const cards = document.querySelectorAll('.feature-card, .arch-card, .db-card');
    if (!('IntersectionObserver' in window)) {
        cards.forEach(function(c) { c.classList.add('visible'); });
        return;
    }
    const observer = new IntersectionObserver(function(entries) {
        entries.forEach(function(entry) {
            if (entry.isIntersecting) {
                entry.target.classList.add('visible');
                observer.unobserve(entry.target);
            }
        });
    }, { threshold: 0.1, rootMargin: '0px 0px -40px 0px' });
    cards.forEach(function(c) { observer.observe(c); });
}

// Feature search / filter
function initFeatureSearch() {
    var input = document.getElementById('featureSearch');
    var clearBtn = document.getElementById('featureSearchClear');
    var countEl = document.getElementById('featureSearchCount');
    if (!input) return;

    var categories = document.querySelectorAll('.feature-category');
    var allCards = document.querySelectorAll('.feature-card');
    var chips = document.querySelectorAll('.hero-search-chip');

    function normalizeQuery(value) {
        return String(value || '').trim().toLowerCase();
    }

    function syncChipState(queryValue) {
        var normalized = normalizeQuery(queryValue);
        chips.forEach(function(chip) {
            var chipValue = normalizeQuery(chip.getAttribute('data-search-term') || chip.textContent);
            chip.classList.toggle('active', !!normalized && chipValue === normalized);
        });
    }

    function runFilter() {
        var raw = normalizeQuery(input.value);
        if (clearBtn) {
            clearBtn.style.display = raw ? 'flex' : 'none';
        }
        syncChipState(raw);

        if (!raw) {
            allCards.forEach(function(c) { c.classList.remove('search-hidden', 'search-highlight'); });
            categories.forEach(function(cat) { cat.classList.remove('search-hidden'); });
            if (countEl) {
                countEl.classList.remove('empty');
                countEl.textContent = 'Type to filter all platform capabilities';
            }
            return;
        }

        var terms = raw.split(/\s+/);
        var matchCount = 0;

        categories.forEach(function(cat) {
            var cards = cat.querySelectorAll('.feature-card');
            var catVisible = 0;

            cards.forEach(function(card) {
                var title = (card.querySelector('h4') || {}).textContent || '';
                var desc = (card.querySelector('p') || {}).textContent || '';
                var tags = '';
                card.querySelectorAll('.tag').forEach(function(t) { tags += ' ' + t.textContent; });
                var haystack = (title + ' ' + desc + ' ' + tags).toLowerCase();

                var match = terms.every(function(term) { return haystack.indexOf(term) !== -1; });
                if (match) {
                    card.classList.remove('search-hidden');
                    card.classList.add('search-highlight');
                    catVisible++;
                    matchCount++;
                } else {
                    card.classList.add('search-hidden');
                    card.classList.remove('search-highlight');
                }
            });

            if (catVisible === 0) {
                cat.classList.add('search-hidden');
            } else {
                cat.classList.remove('search-hidden');
            }
        });

        if (countEl) {
            if (matchCount > 0) {
                countEl.classList.remove('empty');
                countEl.textContent = matchCount + ' feature' + (matchCount !== 1 ? 's' : '') + ' found';
            } else {
                countEl.classList.add('empty');
                countEl.textContent = 'No features found. Try broader keywords like API, data, policy, or analytics.';
            }
        }
    }

    input.addEventListener('input', runFilter);

    if (clearBtn) {
        clearBtn.addEventListener('click', function() {
            input.value = '';
            runFilter();
            input.focus();
        });
    }

    chips.forEach(function(chip) {
        chip.addEventListener('click', function() {
            var searchTerm = (chip.getAttribute('data-search-term') || chip.textContent || '').trim();
            if (!searchTerm) return;
            input.value = searchTerm;
            runFilter();
            input.focus();
        });
    });

    document.addEventListener('keydown', function(event) {
        if (event.key !== '/' || event.defaultPrevented || event.ctrlKey || event.metaKey || event.altKey) {
            return;
        }
        var target = event.target;
        var isEditable = target && (
            target.tagName === 'INPUT' ||
            target.tagName === 'TEXTAREA' ||
            target.isContentEditable
        );
        if (isEditable) return;

        event.preventDefault();
        input.focus();
        input.select();
    });

    runFilter();
}

window.addEventListener('DOMContentLoaded', function() {
    replaceLandingEmojiIcons();
    fetchLandingHealth();
    initScrollAnimations();
    initFeatureSearch();
});
