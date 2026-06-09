// =============================================
// AxiomNizam GIS Dashboard JavaScript
// Supports: General, Agriculture, Industries, Medical (Domestic BD)
//           Satellite, Airplane, Ship (International)
// =============================================

const GIS_API = ((typeof window.resolveBackendURL === 'function' ? window.resolveBackendURL() : (window.BACKEND_URL || 'http://localhost:8000'))) + '/api/v1/gis';
const GIS_SAVED_VIEWS_KEY = 'axiomnizam_gis_saved_views_v1';

function readAuthCookie(name) {
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

function buildAuthHeaders(includeJSONContentType) {
    if (typeof getAuthHeaders === 'function') {
        const headers = getAuthHeaders();
        if (!includeJSONContentType) {
            delete headers['Content-Type'];
        }
        return headers;
    }

    const headers = {};
    const token = localStorage.getItem('authToken') || readAuthCookie('authToken');
    if (token) {
        headers.Authorization = token.startsWith('Bearer ') ? token : 'Bearer ' + token;
    }
    if (includeJSONContentType) {
        headers['Content-Type'] = 'application/json';
    }
    return headers;
}

// Current dashboard type
let currentDashType = 'general';

// State
let gisMap = null;
let gisState = {
    layers: [],
    regions: [],
    markers: [],
    datasets: [],
    activeDataset: null,
    selectedRegion: null,
    mapLayers: {},
    divisionPolygons: {},
    districtPolygons: {},
    markerGroup: null,
    currentPage: 1,
    pageSize: 10,
    tableData: [],
    filteredData: [],
    tableColumns: [],
    isMeasuring: false,
    measurePoints: [],
    measureLine: null,
    baseLayers: {},
    dashboardConfig: {},
    labelsVisible: true,
    mapSearchIndex: [],
    temporarySearchMarker: null,
    useMarkerClustering: true,
    drawLayerGroup: null,
    activeQueryShape: null,
    drawHandlers: {},
    savedViews: [],
    importedGeoJsonLayer: null,
};

// Dashboard theme configurations
const DASH_THEMES = {
    general:     { title: 'GENERAL',      accent: '#3b82f6', legendTitle: 'Population' },
    agriculture: { title: 'AGRICULTURE',  accent: '#2ecc71', legendTitle: 'Rice Production (MT)' },
    industries:  { title: 'INDUSTRIES',   accent: '#e74c3c', legendTitle: 'Industrial Output (Cr)' },
    medical:     { title: 'MEDICAL',      accent: '#e74c3c', legendTitle: 'EPI Coverage %' },
    train:       { title: 'TRAIN (INDIA)', accent: '#f59e0b', legendTitle: 'Train Routes' },
    'bd-train':  { title: 'TRAIN (BD)',    accent: '#06b6d4', legendTitle: 'Train Routes' },
    satellite:   { title: 'SATELLITE',    accent: '#00bcd4', legendTitle: 'Orbit Type' },
    airplane:    { title: 'AIRPLANE',     accent: '#ff5722', legendTitle: 'Traffic Density' },
    ship:        { title: 'SHIP',         accent: '#0277bd', legendTitle: 'Port Throughput (TEU)' },
};

const DASH_PANEL_ICONS = {
    general: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-globe"><circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/></svg>',
    agriculture: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-leaf"><path d="M11 20A7 7 0 0 1 9.8 6.1C15.5 5 17 4.48 19 2c1 2 2 4.18 2 8 0 5.5-4.78 10-10 10Z"/><path d="M2 21c0-3 1.85-5.36 5.08-6C9.5 14.52 12 13 13 12"/></svg>',
    industries: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-factory"><path d="M2 20a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V8l-7 5V8l-7 5V4a2 2 0 0 0-2-2H4a2 2 0 0 0-2 2Z"/><path d="M17 18h1"/><path d="M12 18h1"/><path d="M7 18h1"/></svg>',
    medical: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-hospital"><path d="M12 6v4"/><path d="M14 8h-4"/><path d="M16 21V5a2 2 0 0 0-2-2H2v16"/><path d="M8 21v-4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v4"/><path d="M22 21V9a2 2 0 0 0-2-2h-4"/></svg>',
    train: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-train-front"><path d="M8 3.1V7a4 4 0 0 0 8 0V3.1"/><path d="m9 15-1-1"/><path d="m15 15 1-1"/><path d="M9 19c-2.8 0-5-2.2-5-5v-4a8 8 0 0 1 16 0v4c0 2.8-2.2 5-5 5Z"/><path d="m8 19-2 3"/><path d="m16 19 2 3"/></svg>',
    'bd-train': '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-train-front"><path d="M8 3.1V7a4 4 0 0 0 8 0V3.1"/><path d="m9 15-1-1"/><path d="m15 15 1-1"/><path d="M9 19c-2.8 0-5-2.2-5-5v-4a8 8 0 0 1 16 0v4c0 2.8-2.2 5-5 5Z"/><path d="m8 19-2 3"/><path d="m16 19 2 3"/></svg>',
    satellite: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-satellite"><path d="M13 7 9 3 5 7l4 4"/><path d="m17 11 4 4-4-4-4-4"/><path d="m8 12 4 4 6-6-4-4Z"/><path d="m16 8 3-3"/><path d="M9 21a6 6 0 0 0-6-6"/></svg>',
    airplane: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-plane"><path d="M17.8 19.2 16 11l3.5-3.5C21 6 21.5 4 21 3c-1-.5-3 0-4.5 1.5L13 8 4.8 6.2c-.5-.1-.9.2-1.1.7l-1.6 4.6L9 15l-4 4-4-1-1-4 4-4 3.5 6.9 8.4-1.6c.5-.1.8-.5.7-1Z"/></svg>',
    ship: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-ship"><path d="M2 21c.6.5 1.2 1 2.5 1 2.5 0 2.5-2 5-2 1.3 0 1.9.5 2.5 1 .6.5 1.2 1 2.5 1 2.5 0 2.5-2 5-2 1.3 0 1.9.5 2.5 1"/><path d="M19.38 20A11.6 11.6 0 0 0 21 14l-9-4-9 4c0 2.9.94 5.34 2.81 7.76"/><path d="M19 13V7a2 2 0 0 0-2-2H7a2 2 0 0 0-2 2v6"/><path d="M12 10v4"/><path d="M12 2v3"/></svg>',
};

// Map configurations per scope
const MAP_CONFIG = {
    domestic:      { center: [23.6850, 90.3563], zoom: 7, maxBounds: [[18, 85], [28, 96]], minZoom: 5 },
    international: { center: [20, 0], zoom: 2, maxBounds: [[-85, -180], [85, 180]], minZoom: 2 },
    train:         { center: [22.5, 82.0], zoom: 5, maxBounds: [[6, 65], [38, 98]], minZoom: 4 },
    'bd-train':    { center: [23.6850, 90.3563], zoom: 7, maxBounds: [[20.5, 87.5], [26.7, 92.8]], minZoom: 5 },
};

// =============================================
// Initialization
// =============================================

document.addEventListener('DOMContentLoaded', function () {
    initMap();
    initExplorerUX();
    syncWorkspacePanelsToggle();
    loadSavedViews();
    loadGISData().then(() => {
        bootstrapViewStateFromURL();
        syncClusterToggleUI();
        updateShareLinkPreview();
    });
});

function initMap() {
    gisMap = window.__axm.map('gisMap', {
        center: [23.6850, 90.3563],
        zoom: 7,
        preferCanvas: true,
        zoomControl: false,
        attributionControl: true,
        maxBounds: [[18, 85], [28, 96]],
        minZoom: 5,
        maxZoom: 18,
    });

    // Base tile layers
    const osmLight = window.__axm.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '&copy; OpenStreetMap contributors',
        maxZoom: 19,
    });
    const osmTopo = window.__axm.tileLayer('https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png', {
        attribution: '&copy; OpenTopoMap',
        maxZoom: 17,
    });
    const cartoDark = window.__axm.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png', {
        attribution: '&copy; CARTO',
        maxZoom: 19,
    });
    const cartoLight = window.__axm.tileLayer('https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png', {
        attribution: '&copy; CARTO',
        maxZoom: 19,
    });

    // Use light tiles by default, matching the screenshot style
    cartoLight.addTo(gisMap);

    // Store base layers for controls
    gisState.baseLayers = {
        'CartoDB Light': cartoLight,
        'OpenStreetMap': osmLight,
        'Topographic': osmTopo,
        'CartoDB Dark': cartoDark,
    };

    // Add Layer control
    window.__axm.control.layers(gisState.baseLayers, null, { position: 'topright', collapsed: true }).addTo(gisMap);

    // Scale bar
    window.__axm.control.scale({ position: 'bottomright', imperial: false }).addTo(gisMap);

    // Track mouse position
    gisMap.on('mousemove', function (e) {
        const el = document.getElementById('mapCoords');
        if (el) el.textContent = e.latlng.lat.toFixed(4) + ', ' + e.latlng.lng.toFixed(4);
    });

    gisMap.on('zoomend', function () {
        const el = document.getElementById('mapZoom');
        if (el) el.textContent = 'Zoom ' + gisMap.getZoom();
        updateShareLinkPreview();
    });

    gisMap.on('moveend', function () {
        updateShareLinkPreview();
    });

    // Measure mode click handler
    gisMap.on('click', function (e) {
        if (!gisState.isMeasuring) return;
        addMeasurePoint(e.latlng);
    });

    initDrawQueryTools();
}

function initExplorerUX() {
    document.addEventListener('click', function (event) {
        if (event.target.closest('.gis-menu-wrap') || event.target.closest('.gis-more-dot')) {
            return;
        }
        closeAllMenus();
    });

    document.addEventListener('keydown', function (event) {
        if (event.key === 'Escape') {
            closeAllMenus();
        }
    });
}

function toggleWorkspacePanels() {
    const hub = document.getElementById('gisHubContainer');
    if (!hub) return;

    const willExpand = hub.classList.contains('tools-collapsed');
    hub.classList.toggle('tools-collapsed');

    if (willExpand) {
        const sidebar = document.getElementById('gisSidebar');
        if (sidebar) sidebar.classList.remove('collapsed');
    }

    syncWorkspacePanelsToggle();
    updateShareLinkPreview();
    setTimeout(() => {
        if (gisMap) gisMap.invalidateSize();
    }, 280);
}

function syncWorkspacePanelsToggle() {
    const hub = document.getElementById('gisHubContainer');
    const btn = document.getElementById('gisMoreWorkspaceBtn');
    if (!hub || !btn) return;

    const expanded = !hub.classList.contains('tools-collapsed');
    btn.setAttribute('aria-expanded', expanded ? 'true' : 'false');
    btn.textContent = expanded ? 'Hide Panels' : 'More';
    btn.title = expanded ? 'Hide panels and summary' : 'Show panels and summary';
}

function getDashboardPanelIconMarkup(type) {
    return DASH_PANEL_ICONS[type] || DASH_PANEL_ICONS.general;
}

// =============================================
// Dashboard Type Switching
// =============================================

function getDashScope() {
    if (currentDashType === 'train' || currentDashType === 'bd-train') return currentDashType;
    return (currentDashType === 'general' || currentDashType === 'agriculture' || currentDashType === 'industries' || currentDashType === 'medical') ? 'domestic' : 'international';
}

function switchDashboardType(type) {
    if (type === currentDashType) return;
    currentDashType = type;
    closeAllMenus();

    // Update type bar button states
    document.querySelectorAll('.gis-type-btn').forEach(b => b.classList.remove('active'));
    const activeBtn = document.querySelector('.gis-type-btn[data-type="' + type + '"]');
    if (activeBtn) activeBtn.classList.add('active');

    // Update dashboard info panel
    const theme = DASH_THEMES[type] || DASH_THEMES.general;
    const iconEl = document.getElementById('dashInfoIcon');
    const titleEl = document.getElementById('dashInfoTitle');
    if (iconEl) iconEl.innerHTML = getDashboardPanelIconMarkup(type);
    if (titleEl) titleEl.textContent = theme.title;

    // Update accent color
    document.documentElement.style.setProperty('--gis-accent', theme.accent);

    // Clear map layers
    clearMapLayers();

    // Reconfigure map for scope
    const scope = getDashScope();
    const mapCfg = MAP_CONFIG[scope];
    gisMap.setMaxBounds(mapCfg.maxBounds);
    gisMap.setMinZoom(mapCfg.minZoom);
    gisMap.flyTo(mapCfg.center, mapCfg.zoom, { duration: 1.2 });

    // Load data for new type
    return loadGISData().then(() => {
        syncClusterToggleUI();
        updateShareLinkPreview();
    });
}

function clearMapLayers() {
    Object.values(gisState.divisionPolygons).forEach(p => gisMap.removeLayer(p));
    Object.values(gisState.districtPolygons).forEach(p => gisMap.removeLayer(p));
    if (gisState.markerGroup) gisMap.removeLayer(gisState.markerGroup);
    gisState.divisionPolygons = {};
    gisState.districtPolygons = {};
    gisState.markerGroup = null;
    gisState.layers = [];
    gisState.regions = [];
    gisState.markers = [];
    gisState.datasets = [];
    gisState.activeDataset = null;
    gisState.selectedRegion = null;
    gisState.mapSearchIndex = [];

    if (gisState.importedGeoJsonLayer) {
        gisMap.removeLayer(gisState.importedGeoJsonLayer);
        gisState.importedGeoJsonLayer = null;
    }
}

// =============================================
// Data Loading
// =============================================

async function loadGISData() {
    try {
        if (currentDashType === 'general') {
            // Use original GIS endpoints
            const [summaryRes, layersRes, regionsRes, markersRes, datasetsRes] = await Promise.all([
                fetch(GIS_API + '/summary', { headers: buildAuthHeaders(false) }),
                fetch(GIS_API + '/layers', { headers: buildAuthHeaders(false) }),
                fetch(GIS_API + '/regions', { headers: buildAuthHeaders(false) }),
                fetch(GIS_API + '/markers', { headers: buildAuthHeaders(false) }),
                fetch(GIS_API + '/datasets', { headers: buildAuthHeaders(false) }),
            ]);
            if (!summaryRes.ok || !layersRes.ok || !regionsRes.ok || !markersRes.ok || !datasetsRes.ok) {
                throw new Error('One or more API calls failed');
            }
            const summary = await summaryRes.json();
            gisState.layers = await layersRes.json();
            gisState.regions = await regionsRes.json();
            gisState.markers = await markersRes.json();
            gisState.datasets = await datasetsRes.json();

            updateSummaryCards(summary);
            renderDashDescription('Population, divisions, districts and points of interest across Bangladesh');
        } else if (currentDashType === 'train' || currentDashType === 'bd-train') {
            // Use dedicated train dashboard endpoints
            var trainBase = currentDashType === 'train' ? '/trains' : '/bd-trains';
            const res = await fetch(GIS_API + trainBase + '/dashboard', {
                headers: buildAuthHeaders(false)
            });
            if (!res.ok) throw new Error('Train Dashboard API failed: ' + res.status);
            const data = await res.json();

            gisState.layers = data.layers || [];
            gisState.regions = data.regions || [];
            gisState.markers = data.markers || [];
            gisState.datasets = data.datasets || [];
            gisState.dashboardConfig = data.config || {};

            const summary = {
                totalLayers: gisState.layers.length,
                totalRegions: gisState.regions.length,
                totalMarkers: gisState.markers.length,
                totalDatasets: gisState.datasets.length,
                regionsByType: {},
            };
            gisState.regions.forEach(r => {
                summary.regionsByType[r.type] = (summary.regionsByType[r.type] || 0) + 1;
            });
            updateSummaryCards(summary);
            renderDashDescription(data.description || '');
        } else {
            // Use specialized dashboard endpoint
            const res = await fetch(GIS_API + '/dashboards/' + encodeURIComponent(currentDashType), {
                headers: buildAuthHeaders(false)
            });
            if (!res.ok) throw new Error('Dashboard API failed: ' + res.status);
            const data = await res.json();

            gisState.layers = data.layers || [];
            gisState.regions = data.regions || [];
            gisState.markers = data.markers || [];
            gisState.datasets = data.datasets || [];
            gisState.dashboardConfig = data.config || {};

            const summary = {
                totalLayers: gisState.layers.length,
                totalRegions: gisState.regions.length,
                totalMarkers: gisState.markers.length,
                totalDatasets: gisState.datasets.length,
                regionsByType: {},
            };
            gisState.regions.forEach(r => {
                summary.regionsByType[r.type] = (summary.regionsByType[r.type] || 0) + 1;
            });
            updateSummaryCards(summary);
            renderDashDescription(data.description || '');
        }

        renderLayers();
        renderDivisionGrid();
        renderRegionsOnMap();
        renderMarkersOnMap();
        populateFilterDropdowns();
        populateDatasetSelector();
        buildMapQuickSearchIndex();
        syncLabelToggleButton();

        if (gisState.datasets.length > 0) {
            loadDataset(gisState.datasets[0].id);
        }

        // Reset properties panel
        const emptyEl = document.getElementById('propertiesEmpty');
        const contentEl = document.getElementById('propertiesContent');
        if (emptyEl) emptyEl.style.display = 'flex';
        if (contentEl) contentEl.style.display = 'none';

        syncClusterToggleUI();
        updateShareLinkPreview();
    } catch (err) {
        console.error('Failed to load GIS data:', err);
    }
}

function renderDashDescription(desc) {
    const el = document.getElementById('dashInfoDesc');
    if (el) el.textContent = desc;
}

// =============================================
// Summary Cards
// =============================================

function updateSummaryCards(summary) {
    const cards = getSummaryCardConfig(summary);
    const cardElements = document.querySelectorAll('.gis-summary-card');

    cards.forEach((card, i) => {
        if (cardElements[i]) {
            const cardEl = cardElements[i];
            const iconEl = cardEl.querySelector('.summary-icon');
            const valEl = cardEl.querySelector('.summary-value');
            const lblEl = cardEl.querySelector('.summary-label');
            cardEl.style.borderColor = card.color;
            if (iconEl) iconEl.style.background = card.color;
            if (valEl) valEl.textContent = card.value;
            if (lblEl) lblEl.textContent = card.label;
        }
    });
}

function getSummaryCardConfig(summary) {
    const rbt = summary.regionsByType || {};
    const totalRegions = Object.values(rbt).reduce((a, b) => a + b, 0);

    switch (currentDashType) {
        case 'general':
            return [
                { color: '#3b82f6', value: rbt.division || 0, label: 'Divisions' },
                { color: '#10b981', value: rbt.district || 0, label: 'Districts' },
                { color: '#8b5cf6', value: calcTotalPopField('population'), label: 'Population' },
                { color: '#f59e0b', value: summary.totalMarkers || 0, label: 'Markers' },
                { color: '#ef4444', value: summary.totalLayers || 0, label: 'Layers' },
                { color: '#06b6d4', value: summary.totalDatasets || 0, label: 'Datasets' },
            ];
        case 'agriculture':
            return [
                { color: '#2ecc71', value: totalRegions, label: 'Agri Zones' },
                { color: '#27ae60', value: calcFieldTotal('rice_production'), label: 'Rice (MT)' },
                { color: '#f39c12', value: calcFieldTotal('wheat_production'), label: 'Wheat (MT)' },
                { color: '#3498db', value: summary.totalMarkers || 0, label: 'Facilities' },
                { color: '#2980b9', value: calcFieldAvg('irrigation_pct') + '%', label: 'Avg Irrigation' },
                { color: '#8e44ad', value: summary.totalDatasets || 0, label: 'Datasets' },
            ];
        case 'industries':
            return [
                { color: '#e74c3c', value: totalRegions, label: 'Ind. Divisions' },
                { color: '#c0392b', value: calcFieldTotal('factories'), label: 'Factories' },
                { color: '#f39c12', value: calcFieldTotal('employment'), label: 'Employment' },
                { color: '#3498db', value: calcFieldTotal('garment_units'), label: 'Garment Units' },
                { color: '#9b59b6', value: summary.totalMarkers || 0, label: 'Markers' },
                { color: '#1abc9c', value: summary.totalDatasets || 0, label: 'Datasets' },
            ];
        case 'medical':
            return [
                { color: '#e74c3c', value: calcFieldTotal('hospitals'), label: 'Hospitals' },
                { color: '#3498db', value: calcFieldTotal('beds'), label: 'Hospital Beds' },
                { color: '#2ecc71', value: calcFieldTotal('doctors'), label: 'Doctors' },
                { color: '#f39c12', value: calcFieldAvg('epi_coverage') + '%', label: 'Avg EPI Coverage' },
                { color: '#9b59b6', value: calcFieldTotal('community_clinics'), label: 'Clinics' },
                { color: '#1abc9c', value: summary.totalDatasets || 0, label: 'Datasets' },
            ];
        case 'satellite':
            return [
                { color: '#00bcd4', value: totalRegions, label: 'Orbit Zones' },
                { color: '#ff9800', value: calcFieldTotal('satellites'), label: 'Satellites' },
                { color: '#e91e63', value: countMarkerCategory('launch_site'), label: 'Launch Sites' },
                { color: '#f44336', value: countMarkerCategory('ground_station'), label: 'Ground Stations' },
                { color: '#4caf50', value: summary.totalMarkers || 0, label: 'Total Markers' },
                { color: '#9c27b0', value: summary.totalDatasets || 0, label: 'Datasets' },
            ];
        case 'airplane':
            return [
                { color: '#ff5722', value: totalRegions, label: 'Air Regions' },
                { color: '#2196f3', value: countMarkerCategory('airport'), label: 'Airports' },
                { color: '#ff9800', value: countMarkerCategory('aircraft'), label: 'Aircraft' },
                { color: '#4caf50', value: calcFieldTotal('annual_passengers'), label: 'Passengers/yr' },
                { color: '#9c27b0', value: calcFieldTotal('airlines'), label: 'Airlines' },
                { color: '#607d8b', value: summary.totalDatasets || 0, label: 'Datasets' },
            ];
        case 'ship':
            return [
                { color: '#0277bd', value: totalRegions, label: 'Ocean Zones' },
                { color: '#f44336', value: countMarkerCategory('port'), label: 'Ports' },
                { color: '#ff9800', value: countMarkerCategory('vessel'), label: 'Vessels' },
                { color: '#00897b', value: countMarkerCategory('canal') + countMarkerCategory('strait'), label: 'Chokepoints' },
                { color: '#4caf50', value: summary.totalMarkers || 0, label: 'Total Markers' },
                { color: '#607d8b', value: summary.totalDatasets || 0, label: 'Datasets' },
            ];
        default:
            return [
                { color: '#3b82f6', value: totalRegions, label: 'Regions' },
                { color: '#f59e0b', value: summary.totalMarkers || 0, label: 'Markers' },
                { color: '#ef4444', value: summary.totalLayers || 0, label: 'Layers' },
                { color: '#06b6d4', value: summary.totalDatasets || 0, label: 'Datasets' },
                { color: '#10b981', value: '-', label: '-' },
                { color: '#8b5cf6', value: '-', label: '-' },
            ];
    }
}

function calcTotalPopField(field) {
    const divs = gisState.regions.filter(r => r.type === 'division');
    let total = 0;
    divs.forEach(r => { if (r.properties && r.properties[field]) total += r.properties[field]; });
    return formatPopulation(total);
}

function calcFieldTotal(field) {
    let total = 0;
    gisState.regions.forEach(r => {
        if (r.properties && r.properties[field]) total += Number(r.properties[field]);
    });
    return formatPopulation(total);
}

function calcFieldAvg(field) {
    let total = 0, count = 0;
    gisState.regions.forEach(r => {
        if (r.properties && r.properties[field] !== undefined) {
            total += Number(r.properties[field]);
            count++;
        }
    });
    return count > 0 ? Math.round(total / count) : 0;
}

function countMarkerCategory(cat) {
    return gisState.markers.filter(m => m.category === cat).length;
}

function formatPopulation(num) {
    if (num >= 1e9) return (num / 1e9).toFixed(2) + 'B';
    if (num >= 1e6) return (num / 1e6).toFixed(2) + 'M';
    if (num >= 1e3) return (num / 1e3).toFixed(1) + 'K';
    return num.toString();
}

// =============================================
// Layers Panel
// =============================================

function renderLayers() {
    const container = document.getElementById('layersList');
    if (!container) return;
    container.innerHTML = '';

    gisState.layers.forEach(layer => {
        const item = document.createElement('div');
        item.className = 'gis-layer-item';
        item.innerHTML = `
            <label class="gis-layer-label">
                <input type="checkbox" ${layer.visible ? 'checked' : ''} 
                    onchange="toggleLayer('${layer.id}', this.checked)">
                <span class="layer-icon">${getLayerIcon(layer.type)}</span>
                <span class="layer-name">${layer.name}</span>
            </label>
        `;
        container.appendChild(item);
    });
}

function getLayerIcon(type) {
    const icons = {
        geojson: '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-map"><path d="M14.106 5.553a2 2 0 0 0 1.788 0l3.659-1.83A1 1 0 0 1 21 4.619v12.764a1 1 0 0 1-.553.894l-4.553 2.277a2 2 0 0 1-1.788 0l-4.212-2.106a2 2 0 0 0-1.788 0l-3.659 1.83A1 1 0 0 1 3 19.381V6.618a1 1 0 0 1 .553-.894l4.553-2.277a2 2 0 0 1 1.788 0z"/><path d="M15 5.764v15"/><path d="M9 3.236v15"/></svg>',
        tile: '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-grid-3x3"><rect width="18" height="18" x="3" y="3" rx="2"/><path d="M3 9h18"/><path d="M3 15h18"/><path d="M9 3v18"/><path d="M15 3v18"/></svg>',
        marker: '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-map-pin"><path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z"/><circle cx="12" cy="10" r="3"/></svg>',
        heatmap: '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-flame"><path d="M8.5 14.5A2.5 2.5 0 0 0 11 17h2a2 2 0 0 0 0-4c0-2 1-3.5 2.5-5 .8 1.5 2.2 3.2 2.2 5A4 4 0 0 1 14 17h-4"/><path d="M12 2c1.5 2.2 4 4.5 4 8a4 4 0 0 1-8 0c0-1.5.5-3.2 1.5-4.6"/></svg>',
    };
    return icons[type] || '<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-file"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/></svg>';
}

function toggleLayer(id, visible) {
    const layer = gisState.layers.find(l => l.id === id);
    if (layer) layer.visible = visible;

    // Determine which map group to toggle by layer index/type
    const geoLayers = gisState.layers.filter(l => l.type === 'geojson' || l.type === 'heatmap');
    const markerLayers = gisState.layers.filter(l => l.type === 'marker');

    if (geoLayers.length > 0 && geoLayers[0].id === id) {
        Object.values(gisState.divisionPolygons).forEach(p => {
            if (visible) p.addTo(gisMap); else gisMap.removeLayer(p);
        });
    } else if (geoLayers.length > 1 && geoLayers[1].id === id) {
        Object.values(gisState.districtPolygons).forEach(p => {
            if (visible) p.addTo(gisMap); else gisMap.removeLayer(p);
        });
    } else if (markerLayers.some(l => l.id === id)) {
        if (gisState.markerGroup) {
            if (visible) gisState.markerGroup.addTo(gisMap); else gisMap.removeLayer(gisState.markerGroup);
        }
    }
}

function switchBaseMap(name) {
    Object.values(gisState.baseLayers).forEach(l => gisMap.removeLayer(l));
    gisState.baseLayers[name].addTo(gisMap);
}

// =============================================
// Division Grid (Quick Navigation)
// =============================================

function renderDivisionGrid() {
    const container = document.getElementById('divisionGrid');
    if (!container) return;
    container.innerHTML = '';

    const primaryTypes = getPrimaryRegionTypes();
    const primaries = gisState.regions.filter(r => primaryTypes.includes(r.type));
    primaries.sort((a, b) => a.name.localeCompare(b.name));

    primaries.forEach(div => {
        const btn = document.createElement('button');
        btn.className = 'gis-division-btn';
        btn.textContent = div.name.length > 18 ? div.name.substring(0, 16) + '…' : div.name;
        btn.title = div.name;
        btn.onclick = () => flyToRegion(div);
        container.appendChild(btn);
    });

    // Update panel header
    const panelHeader = container.closest('.gis-panel');
    if (panelHeader) {
        const h3 = panelHeader.querySelector('h3');
        if (h3) h3.textContent = getRegionGridLabel();
    }
}

function getPrimaryRegionTypes() {
    switch (currentDashType) {
        case 'general': case 'agriculture': case 'industries': case 'medical':
            return ['division'];
        case 'satellite': return ['orbit_zone'];
        case 'airplane': return ['air_region'];
        case 'ship': return ['ocean_zone'];
        default: return ['division'];
    }
}

function getRegionGridLabel() {
    switch (currentDashType) {
        case 'general': case 'agriculture': case 'industries': case 'medical':
            return 'DIVISIONS';
        case 'satellite': return 'ORBIT ZONES';
        case 'airplane': return 'AIR REGIONS';
        case 'ship': return 'OCEAN ZONES';
        default: return 'REGIONS';
    }
}

function flyToRegion(region) {
    const scope = getDashScope();
    const zoomLevel = scope === 'domestic' ? 9 : 4;
    gisMap.flyTo(region.center, zoomLevel, { duration: 1 });
    showRegionProperties(region);
    if (currentDashType === 'general') loadDistrictsForDivision(region.id);
}

async function loadDistrictsForDivision(divisionId) {
    if (currentDashType !== 'general') return;
    try {
        const res = await fetch(GIS_API + '/regions?type=district&parent=' + encodeURIComponent(divisionId), {
            headers: buildAuthHeaders(false)
        });
        if (!res.ok) return;
        const districts = await res.json();
        if (districts.length > 0) {
            const tableRows = districts.map(d => ({
                name: d.name,
                population: d.properties?.population || 0,
            }));
            populateDataTable(
                [
                    { key: 'name', label: 'District', type: 'string' },
                    { key: 'population', label: 'Population', type: 'number' },
                ],
                tableRows,
                'District Population'
            );
        }
    } catch (err) {
        console.error('Failed loading districts:', err);
    }
}

// =============================================
// Map Regions (Polygons / Circles)
// =============================================

function renderRegionsOnMap() {
    const primaryTypes = getPrimaryRegionTypes();
    const primaries = gisState.regions.filter(r => primaryTypes.includes(r.type));
    const secondaries = gisState.regions.filter(r => !primaryTypes.includes(r.type));

    const primaryLayerVisible = gisState.layers.length > 0 ? gisState.layers[0].visible : true;
    const secondaryLayerVisible = gisState.layers.length > 1 ? gisState.layers[1].visible : false;

    const scope = getDashScope();

    primaries.forEach(region => {
        const color = region.properties?.color || randomColor();
        const val = region.properties?.population || 0;
        const radius = scope === 'domestic'
            ? Math.max(25000, Math.sqrt(Math.max(val, 0) / 100) * 500)
            : Math.max(500000, Math.sqrt(Math.max(val, 0) / 1000) * 5000);

        const circle = window.__axm.circle(region.center, {
            radius: radius,
            color: darkenColor(color),
            weight: 2,
            fillColor: color,
            fillOpacity: 0.3,
        });
        circle.__regionName = region.name;

        circle.bindTooltip(region.name, {
            permanent: scope === 'domestic',
            direction: 'center',
            className: 'gis-division-label',
        });

        circle.on('click', () => {
            showRegionProperties(region);
            if (currentDashType === 'general') loadDistrictsForDivision(region.id);
        });
        circle.on('mouseover', function () { this.setStyle({ fillOpacity: 0.5, weight: 3 }); });
        circle.on('mouseout', function () { this.setStyle({ fillOpacity: 0.3, weight: 2 }); });

        if (primaryLayerVisible) circle.addTo(gisMap);
        gisState.divisionPolygons[region.id] = circle;
    });

    secondaries.forEach(region => {
        const val = region.properties?.population || 0;
        const radius = scope === 'domestic'
            ? Math.max(5000, Math.sqrt(Math.max(val, 0) / 100) * 250)
            : Math.max(200000, Math.sqrt(Math.max(val, 0) / 1000) * 2000);

        const circle = window.__axm.circle(region.center, {
            radius: radius,
            color: '#666',
            weight: 1,
            fillColor: getValueColor(val),
            fillOpacity: 0.4,
        });
        circle.__regionName = region.name;

        circle.bindTooltip(region.name, { direction: 'top' });
        circle.on('click', () => showRegionProperties(region));

        if (secondaryLayerVisible) circle.addTo(gisMap);
        gisState.districtPolygons[region.id] = circle;
    });

    applyRegionLabelMode();

    renderLegend();
}

function getValueColor(val) {
    switch (currentDashType) {
        case 'agriculture':
            if (val > 6000000) return '#1b5e20';
            if (val > 4000000) return '#2e7d32';
            if (val > 3000000) return '#43a047';
            if (val > 2000000) return '#66bb6a';
            return '#a5d6a7';
        case 'industries':
            if (val > 200000) return '#b71c1c';
            if (val > 100000) return '#d32f2f';
            if (val > 50000) return '#e53935';
            if (val > 30000) return '#ef5350';
            return '#ef9a9a';
        case 'medical':
            if (val > 200) return '#1b5e20';
            if (val > 100) return '#388e3c';
            if (val > 50) return '#66bb6a';
            return '#a5d6a7';
        default:
            if (val > 10000000) return '#1a5276';
            if (val > 5000000) return '#2471a3';
            if (val > 3000000) return '#2e86c1';
            if (val > 2000000) return '#5dade2';
            if (val > 1000000) return '#85c1e9';
            return '#d6eaf8';
    }
}

function renderLegend() {
    const body = document.getElementById('legendBody');
    const titleEl = document.getElementById('legendTitle');
    if (!body) return;
    body.innerHTML = '';

    const theme = DASH_THEMES[currentDashType] || DASH_THEMES.general;
    if (titleEl) titleEl.textContent = theme.legendTitle;

    const levels = getLegendLevels();
    levels.forEach(lvl => {
        const item = document.createElement('div');
        item.className = 'gis-legend-item';
        item.innerHTML = '<span class="legend-color" style="background:' + lvl.color + '"></span><span>' + lvl.label + '</span>';
        body.appendChild(item);
    });
}

function getLegendLevels() {
    switch (currentDashType) {
        case 'agriculture':
            return [
                { color: '#a5d6a7', label: '< 3M MT' },
                { color: '#66bb6a', label: '3M - 4M' },
                { color: '#43a047', label: '4M - 5M' },
                { color: '#2e7d32', label: '5M - 6M' },
                { color: '#1b5e20', label: '> 6M MT' },
            ];
        case 'industries':
            return [
                { color: '#ef9a9a', label: '< 30K Cr' },
                { color: '#ef5350', label: '30K - 50K' },
                { color: '#e53935', label: '50K - 100K' },
                { color: '#d32f2f', label: '100K - 200K' },
                { color: '#b71c1c', label: '> 200K Cr' },
            ];
        case 'medical':
            return [
                { color: '#a5d6a7', label: '< 50 Hospitals' },
                { color: '#66bb6a', label: '50 - 100' },
                { color: '#388e3c', label: '100 - 200' },
                { color: '#1b5e20', label: '> 200' },
            ];
        case 'satellite':
            return [
                { color: '#e3f2fd', label: 'LEO (160-2000km)' },
                { color: '#fff3e0', label: 'MEO (2000-35786km)' },
                { color: '#fce4ec', label: 'GEO (35786km)' },
                { color: '#e8eaf6', label: 'SSO (Polar)' },
            ];
        case 'airplane':
            return [
                { color: '#ffecb3', label: 'Asia-Pacific' },
                { color: '#bbdefb', label: 'Europe' },
                { color: '#c8e6c9', label: 'North America' },
                { color: '#f8bbd0', label: 'Middle East' },
                { color: '#d1c4e9', label: 'Africa' },
                { color: '#ffe0b2', label: 'Latin America' },
            ];
        case 'ship':
            return [
                { color: '#b3e5fc', label: 'Indian Ocean' },
                { color: '#c8e6c9', label: 'Pacific Ocean' },
                { color: '#dcedc8', label: 'Atlantic Ocean' },
                { color: '#fff9c4', label: 'Mediterranean' },
                { color: '#ffe0b2', label: 'South China Sea' },
                { color: '#e1bee7', label: 'Bay of Bengal' },
            ];
        default:
            return [
                { color: '#d6eaf8', label: '< 1M' },
                { color: '#85c1e9', label: '1M - 2M' },
                { color: '#5dade2', label: '2M - 3M' },
                { color: '#2e86c1', label: '3M - 5M' },
                { color: '#2471a3', label: '5M - 10M' },
                { color: '#1a5276', label: '> 10M' },
            ];
    }
}

// =============================================
// Map Markers
// =============================================

function renderMarkersOnMap() {
    if (gisState.markerGroup) gisMap.removeLayer(gisState.markerGroup);

    const canCluster = isMarkerClusteringAvailable();
    const useCluster = gisState.useMarkerClustering && canCluster;

    gisState.markerGroup = useCluster
        ? window.__axm.markerClusterGroup({
            chunkedLoading: true,
            chunkInterval: 120,
            chunkDelay: 25,
            disableClusteringAtZoom: 13,
            maxClusterRadius: 44,
            showCoverageOnHover: false,
        })
        : window.__axm.layerGroup();

    gisState.markers.forEach(m => {
        const color = m.color || '#3388ff';
        const glyph = getMarkerGlyph(m.category);

        const icon = window.__axm.divIcon({
            html: '<div class="gis-marker-icon"><svg viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg" aria-hidden="true"><path d="M16 2C9.4 2 4 7.4 4 14c0 7.5 8.8 15.6 11.2 17.6a1.2 1.2 0 0 0 1.6 0C19.2 29.6 28 21.5 28 14 28 7.4 22.6 2 16 2Z" fill="' + color + '"/><circle cx="16" cy="14" r="6" fill="#ffffff"/><text x="16" y="17" text-anchor="middle" font-size="8" font-weight="700" fill="#0f172a" font-family="Arial, sans-serif">' + glyph + '</text></svg></div>',
            className: 'gis-marker-container',
            iconSize: [32, 32],
            iconAnchor: [16, 31],
            popupAnchor: [0, -28],
        });

        const marker = window.__axm.marker([m.lat, m.lng], { icon: icon });
        marker.bindPopup('<strong>' + m.name + '</strong><br><em>' + m.category + '</em>');
        marker.on('click', () => showMarkerProperties(m));
        marker.__source = m;
        gisState.markerGroup.addLayer(marker);
    });

    // Check if any marker layer is visible
    const markerLayers = gisState.layers.filter(l => l.type === 'marker');
    const anyVisible = markerLayers.length === 0 || markerLayers.some(l => l.visible);
    if (anyVisible) gisState.markerGroup.addTo(gisMap);

    syncClusterToggleUI();
}

function getMarkerGlyph(category) {
    const glyphMap = {
        capital: 'C',
        port: 'P',
        airport: 'A',
        infrastructure: 'I',
        tourism: 'T',
        nature: 'N',
        research: 'R',
        cold_storage: 'S',
        processing: 'P',
        tea_estate: 'T',
        dairy: 'D',
        seed_bank: 'B',
        market: 'M',
        epz: 'E',
        industrial_zone: 'Z',
        textile: 'T',
        port_industry: 'P',
        tech_park: 'K',
        sez: 'S',
        power: 'W',
        hospital: 'H',
        clinic: 'C',
        vaccine: 'V',
        blood_bank: 'B',
        satellite: 'S',
        constellation: 'C',
        navigation: 'N',
        weather: 'W',
        ground_station: 'G',
        launch_site: 'L',
        earth_observation: 'E',
        aircraft: 'A',
        canal: 'C',
        strait: 'S',
        vessel: 'V',
        train: 'T',
        station: 'S',
    };

    if (glyphMap[category]) {
        return glyphMap[category];
    }

    const fallback = String(category || '').trim();
    return fallback ? fallback.charAt(0).toUpperCase() : 'M';
}

function isMarkerClusteringAvailable() {
    return typeof window.__axm !== 'undefined' && typeof window.__axm.markerClusterGroup === 'function';
}

// =============================================
// Properties Panel
// =============================================

function showRegionProperties(region) {
    gisState.selectedRegion = region;
    document.getElementById('propertiesEmpty').style.display = 'none';
    document.getElementById('propertiesContent').style.display = 'block';
    document.getElementById('propTitle').textContent = region.name;

    const grid = document.getElementById('propGrid');
    grid.innerHTML = '';

    const props = {
        'Type': region.type.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase()),
        'ID': region.id,
        'Center': region.center ? region.center[0].toFixed(4) + ', ' + region.center[1].toFixed(4) : '-',
    };

    if (region.properties) {
        Object.keys(region.properties).forEach(key => {
            if (key === 'color') return;
            const val = region.properties[key];
            const label = key.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
            props[label] = typeof val === 'number' ? val.toLocaleString() : val;
        });
    }

    Object.entries(props).forEach(([label, value]) => {
        const row = document.createElement('div');
        row.className = 'gis-prop-row';
        row.innerHTML = `<span class="prop-label">${label}</span><span class="prop-value">${value}</span>`;
        grid.appendChild(row);
    });

    // Switch to properties tab
    switchDataTab('properties');
}

function showMarkerProperties(marker) {
    document.getElementById('propertiesEmpty').style.display = 'none';
    document.getElementById('propertiesContent').style.display = 'block';
    document.getElementById('propTitle').textContent = marker.name;

    const grid = document.getElementById('propGrid');
    grid.innerHTML = '';

    const props = {
        'Category': marker.category.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase()),
        'Latitude': marker.lat.toFixed(4),
        'Longitude': marker.lng.toFixed(4),
    };

    if (marker.properties) {
        Object.entries(marker.properties).forEach(([k, v]) => {
            const label = k.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
            props[label] = typeof v === 'number' ? v.toLocaleString() : v;
        });
    }

    Object.entries(props).forEach(([label, value]) => {
        const row = document.createElement('div');
        row.className = 'gis-prop-row';
        row.innerHTML = `<span class="prop-label">${label}</span><span class="prop-value">${value}</span>`;
        grid.appendChild(row);
    });

    switchDataTab('properties');
}

// =============================================
// Data Table
// =============================================

function populateDatasetSelector() {
    const sel = document.getElementById('filterDataset');
    if (!sel) return;
    sel.innerHTML = '';

    gisState.datasets.forEach(ds => {
        const opt = document.createElement('option');
        opt.value = ds.id;
        opt.textContent = ds.name;
        sel.appendChild(opt);
    });
}

async function switchDataset() {
    const sel = document.getElementById('filterDataset');
    if (!sel) return;
    const dsId = sel.value;
    if (dsId) {
        await loadDataset(dsId);
        updateShareLinkPreview();
    }
}

async function loadDataset(id) {
    try {
        let ds;
        if (currentDashType === 'general') {
            const res = await fetch(GIS_API + '/datasets/' + encodeURIComponent(id), {
                headers: buildAuthHeaders(false)
            });
            if (!res.ok) return;
            ds = await res.json();
        } else {
            // For specialized dashboards, datasets are already in state
            ds = gisState.datasets.find(d => d.id === id);
        }
        if (!ds) return;

        gisState.activeDataset = ds;
        populateDataTable(ds.columns, ds.rows, ds.name);
        computeStats(ds);
        switchDataTab('data');
    } catch (err) {
        console.error('Failed loading dataset:', err);
    }
}

function populateDataTable(columns, rows, title) {
    document.getElementById('tableTitle').textContent = title || 'Data';

    // Build thead
    const thead = document.getElementById('dataTableHead');
    thead.innerHTML = '<tr>' + columns.map(c => `<th>${c.label}</th>`).join('') + '</tr>';

    // Store data
    gisState.tableData = rows;
    gisState.filteredData = rows;
    gisState.tableColumns = columns;
    gisState.currentPage = 1;

    renderTablePage();
}

function renderTablePage() {
    const tbody = document.getElementById('dataTableBody');
    const data = gisState.filteredData;
    const cols = gisState.tableColumns;
    if (!cols || !data) return;
    const start = (gisState.currentPage - 1) * gisState.pageSize;
    const pageData = data.slice(start, start + gisState.pageSize);

    tbody.innerHTML = '';
    pageData.forEach(row => {
        const tr = document.createElement('tr');
        cols.forEach(col => {
            const td = document.createElement('td');
            let val = row[col.key];
            if (col.type === 'number' && typeof val === 'number') {
                val = val >= 1e6 ? formatPopulation(val) : val.toLocaleString();
            }
            td.textContent = val ?? '-';
            tr.appendChild(td);
        });
        tr.onclick = () => highlightRegionFromTable(row);
        tbody.appendChild(tr);
    });

    // Footer
    const totalPages = Math.ceil(data.length / gisState.pageSize);
    document.getElementById('tableInfo').textContent =
        data.length > 0 ? `${start + 1}-${Math.min(start + gisState.pageSize, data.length)} of ${data.length}` : '0 rows';

    const pagination = document.getElementById('tablePagination');
    pagination.innerHTML = '';
    if (totalPages > 1) {
        const prevBtn = createPageBtn('‹', gisState.currentPage > 1, () => { gisState.currentPage--; renderTablePage(); });
        pagination.appendChild(prevBtn);
        for (let i = 1; i <= totalPages; i++) {
            const btn = createPageBtn(String(i), true, () => { gisState.currentPage = i; renderTablePage(); });
            if (i === gisState.currentPage) btn.classList.add('active');
            pagination.appendChild(btn);
        }
        const nextBtn = createPageBtn('›', gisState.currentPage < totalPages, () => { gisState.currentPage++; renderTablePage(); });
        pagination.appendChild(nextBtn);
    }
}

function createPageBtn(text, enabled, onclick) {
    const btn = document.createElement('button');
    btn.className = 'gis-page-btn' + (enabled ? '' : ' disabled');
    btn.textContent = text;
    if (enabled) btn.onclick = onclick;
    return btn;
}

function filterTable() {
    const query = (document.getElementById('tableSearch').value || '').toLowerCase();
    if (!query) {
        gisState.filteredData = gisState.tableData;
    } else {
        gisState.filteredData = gisState.tableData.filter(row =>
            Object.values(row).some(v => String(v).toLowerCase().includes(query))
        );
    }
    gisState.currentPage = 1;
    renderTablePage();
}

function highlightRegionFromTable(row) {
    const name = row.name || row.Name || row.division;
    if (!name) return;
    const region = gisState.regions.find(r => r.name === name);
    if (region) {
        const scope = getDashScope();
        gisMap.flyTo(region.center, scope === 'domestic' ? 10 : 4, { duration: 0.8 });
        showRegionProperties(region);
    }
}

function exportTableCSV() {
    if (!gisState.tableColumns || !gisState.filteredData) return;
    const header = gisState.tableColumns.map(c => c.label).join(',');
    const rows = gisState.filteredData.map(row =>
        gisState.tableColumns.map(c => JSON.stringify(row[c.key] ?? '')).join(',')
    );
    const csv = [header, ...rows].join('\n');
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob);
    link.download = (gisState.activeDataset?.name || 'data') + '.csv';
    link.click();
    URL.revokeObjectURL(link.href);
}

// =============================================
// Statistics Panel
// =============================================

function computeStats(ds) {
    const grid = document.getElementById('statsGrid');
    if (!grid) return;
    grid.innerHTML = '';

    const numCols = ds.columns.filter(c => c.type === 'number');
    numCols.forEach(col => {
        const values = ds.rows.map(r => Number(r[col.key]) || 0);
        const sum = values.reduce((a, b) => a + b, 0);
        const min = Math.min(...values);
        const max = Math.max(...values);
        const avg = sum / values.length;

        const card = document.createElement('div');
        card.className = 'gis-stats-card';
        card.innerHTML = `
            <h4>${col.label}</h4>
            <div class="stat-row"><span>Total</span><span>${formatPopulation(sum)}</span></div>
            <div class="stat-row"><span>Average</span><span>${formatPopulation(Math.round(avg))}</span></div>
            <div class="stat-row"><span>Min</span><span>${formatPopulation(min)}</span></div>
            <div class="stat-row"><span>Max</span><span>${formatPopulation(max)}</span></div>
            <div class="stat-row"><span>Count</span><span>${values.length}</span></div>
        `;
        grid.appendChild(card);
    });
}

// =============================================
// Filters
// =============================================

function populateFilterDropdowns() {
    const divSelect = document.getElementById('filterDivision');
    if (!divSelect) return;

    const primaryTypes = getPrimaryRegionTypes();
    const primaries = gisState.regions.filter(r => primaryTypes.includes(r.type));
    primaries.sort((a, b) => a.name.localeCompare(b.name));

    const label = getRegionGridLabel().replace(/S$/, '');
    divSelect.innerHTML = '<option value="">All ' + label + 's</option>';
    primaries.forEach(d => {
        const opt = document.createElement('option');
        opt.value = d.id;
        opt.textContent = d.name;
        divSelect.appendChild(opt);
    });

    // Update region type dropdown
    const typeSelect = document.getElementById('filterRegionType');
    if (typeSelect) {
        const regionTypes = [...new Set(gisState.regions.map(r => r.type))];
        typeSelect.innerHTML = '<option value="">All Types</option>';
        regionTypes.forEach(t => {
            const opt = document.createElement('option');
            opt.value = t;
            opt.textContent = t.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
            typeSelect.appendChild(opt);
        });
    }
}

function applyFilters() {
    const divId = document.getElementById('filterDivision').value;
    const regionType = document.getElementById('filterRegionType').value;

    if (divId) {
        const region = gisState.regions.find(r => r.id === divId);
        if (region) {
            const scope = getDashScope();
            gisMap.flyTo(region.center, scope === 'domestic' ? 9 : 4, { duration: 1 });
            showRegionProperties(region);
            if (currentDashType === 'general') loadDistrictsForDivision(divId);
        }
    }

    const primaryTypes = getPrimaryRegionTypes();
    if (!regionType || primaryTypes.includes(regionType)) {
        Object.values(gisState.divisionPolygons).forEach(p => p.addTo(gisMap));
    } else {
        Object.values(gisState.divisionPolygons).forEach(p => gisMap.removeLayer(p));
    }

    if (!regionType || !primaryTypes.includes(regionType)) {
        Object.values(gisState.districtPolygons).forEach(p => p.addTo(gisMap));
    } else {
        Object.values(gisState.districtPolygons).forEach(p => gisMap.removeLayer(p));
    }
}

function clearFilters() {
    document.getElementById('filterDivision').value = '';
    document.getElementById('filterRegionType').value = '';
    resetMapView();
}

// =============================================
// Map Tools
// =============================================

function resetMapView() {
    const scope = getDashScope();
    const cfg = MAP_CONFIG[scope];
    gisMap.flyTo(cfg.center, cfg.zoom, { duration: 1 });
}

function toggleFullscreen() {
    const mapEl = document.querySelector('.gis-map-wrapper');
    if (!document.fullscreenElement) {
        mapEl.requestFullscreen();
    } else {
        document.exitFullscreen();
    }
}

function locateUser() {
    gisMap.locate({ setView: true, maxZoom: 12 });
    gisMap.on('locationfound', function (e) {
        window.__axm.marker(e.latlng).addTo(gisMap).bindPopup('You are here').openPopup();
    });
}

function toggleMeasure() {
    gisState.isMeasuring = !gisState.isMeasuring;
    const btn = document.getElementById('measureBtn');
    if (btn) btn.classList.toggle('active', gisState.isMeasuring);

    if (!gisState.isMeasuring) {
        // Clear measurement
        if (gisState.measureLine) {
            gisMap.removeLayer(gisState.measureLine);
            gisState.measureLine = null;
        }
        gisState.measurePoints.forEach(m => gisMap.removeLayer(m));
        gisState.measurePoints = [];
    }
}

function addMeasurePoint(latlng) {
    const marker = window.__axm.circleMarker(latlng, { radius: 5, color: '#e74c3c', fillColor: '#e74c3c', fillOpacity: 1 }).addTo(gisMap);
    gisState.measurePoints.push(marker);

    if (gisState.measurePoints.length > 1) {
        const points = gisState.measurePoints.map(m => m.getLatLng());
        if (gisState.measureLine) gisMap.removeLayer(gisState.measureLine);
        gisState.measureLine = window.__axm.polyline(points, { color: '#e74c3c', weight: 2, dashArray: '5,10' }).addTo(gisMap);

        // Calculate total distance
        let totalDist = 0;
        for (let i = 1; i < points.length; i++) {
            totalDist += points[i - 1].distanceTo(points[i]);
        }
        const distStr = totalDist > 1000 ? (totalDist / 1000).toFixed(2) + ' km' : Math.round(totalDist) + ' m';
        gisState.measureLine.bindTooltip(distStr, { permanent: true, direction: 'center' });
    }
}

function handleMapSearchKey(event) {
    if (event.key !== 'Enter') return;
    event.preventDefault();
    searchMapFeature();
}

function searchMapFeature() {
    const input = document.getElementById('mapQuickSearch');
    if (!input) return;
    const query = (input.value || '').trim();
    if (!query) return;

    const coordMatch = query.match(/^\s*(-?\d+(?:\.\d+)?)\s*,\s*(-?\d+(?:\.\d+)?)\s*$/);
    if (coordMatch) {
        const lat = Number(coordMatch[1]);
        const lng = Number(coordMatch[2]);
        if (Number.isFinite(lat) && Number.isFinite(lng)) {
            placeTemporaryCoordinateMarker(lat, lng, 'Coordinate');
            gisMap.flyTo([lat, lng], Math.max(10, gisMap.getZoom()), { duration: 0.9 });
        }
        return;
    }

    const normalized = query.toLowerCase();
    let match = gisState.mapSearchIndex.find(item => item.nameLower === normalized);
    if (!match) {
        match = gisState.mapSearchIndex.find(item => item.nameLower.startsWith(normalized));
    }
    if (!match) {
        match = gisState.mapSearchIndex.find(item => item.nameLower.includes(normalized));
    }
    if (!match) {
        input.classList.add('search-miss');
        setTimeout(() => input.classList.remove('search-miss'), 500);
        return;
    }

    if (match.kind === 'region' && match.ref) {
        const scope = getDashScope();
        gisMap.flyTo(match.ref.center, scope === 'domestic' ? 10 : 5, { duration: 0.9 });
        showRegionProperties(match.ref);
        return;
    }

    if (match.kind === 'marker' && match.ref) {
        gisMap.flyTo([match.ref.lat, match.ref.lng], Math.max(9, gisMap.getZoom()), { duration: 0.9 });
        showMarkerProperties(match.ref);
    }
}

function buildMapQuickSearchIndex() {
    const list = [];
    const datalist = document.getElementById('mapQuickSearchList');
    const seen = new Set();

    gisState.regions.forEach(region => {
        if (!region || !region.name) return;
        const key = 'region:' + region.name.toLowerCase();
        if (seen.has(key)) return;
        seen.add(key);
        list.push({ kind: 'region', ref: region, nameLower: region.name.toLowerCase() });
    });

    gisState.markers.forEach(marker => {
        if (!marker || !marker.name) return;
        const key = 'marker:' + marker.name.toLowerCase();
        if (seen.has(key)) return;
        seen.add(key);
        list.push({ kind: 'marker', ref: marker, nameLower: marker.name.toLowerCase() });
    });

    gisState.mapSearchIndex = list;

    if (!datalist) return;
    datalist.innerHTML = '';
    list.slice(0, 250).forEach(item => {
        const opt = document.createElement('option');
        opt.value = item.ref.name;
        datalist.appendChild(opt);
    });
}

function placeTemporaryCoordinateMarker(lat, lng, label) {
    if (gisState.temporarySearchMarker) {
        gisMap.removeLayer(gisState.temporarySearchMarker);
    }
    gisState.temporarySearchMarker = window.__axm.circleMarker([lat, lng], {
        radius: 7,
        color: '#f59e0b',
        fillColor: '#f59e0b',
        fillOpacity: 0.75,
        weight: 2,
    }).addTo(gisMap).bindPopup(label + ': ' + lat.toFixed(5) + ', ' + lng.toFixed(5));
    gisState.temporarySearchMarker.openPopup();
}

function fitVisibleData() {
    const bounds = window.__axm.latLngBounds([]);

    Object.values(gisState.divisionPolygons).forEach(layer => {
        if (gisMap.hasLayer(layer)) bounds.extend(layer.getBounds());
    });
    Object.values(gisState.districtPolygons).forEach(layer => {
        if (gisMap.hasLayer(layer)) bounds.extend(layer.getBounds());
    });

    if (gisState.markerGroup && gisMap.hasLayer(gisState.markerGroup)) {
        gisState.markerGroup.eachLayer(layer => {
            if (layer.getLatLng) bounds.extend(layer.getLatLng());
        });
    }

    if (bounds.isValid()) {
        gisMap.fitBounds(bounds, { padding: [24, 24], maxZoom: 12 });
    } else {
        resetMapView();
    }
}

function focusMarkersOnly() {
    Object.values(gisState.divisionPolygons).forEach(layer => gisMap.removeLayer(layer));
    Object.values(gisState.districtPolygons).forEach(layer => gisMap.removeLayer(layer));

    gisState.layers.forEach(layer => {
        if (layer.type === 'marker') {
            layer.visible = true;
        }
        if (layer.type === 'geojson' || layer.type === 'heatmap') {
            layer.visible = false;
        }
    });

    if (gisState.markerGroup) {
        gisState.markerGroup.addTo(gisMap);
    }

    renderLayers();
    fitVisibleData();
}

function toggleRegionLabels() {
    gisState.labelsVisible = !gisState.labelsVisible;
    applyRegionLabelMode();
    syncLabelToggleButton();
}

function applyRegionLabelMode() {
    const apply = function (layer) {
        if (!layer) return;
        if (gisState.labelsVisible) {
            if (!layer.getTooltip() && layer.__regionName) {
                layer.bindTooltip(layer.__regionName, {
                    permanent: true,
                    direction: 'center',
                    className: 'gis-division-label',
                });
            }
            return;
        }
        if (layer.getTooltip()) {
            layer.unbindTooltip();
        }
    };

    Object.values(gisState.divisionPolygons).forEach(apply);
}

function syncLabelToggleButton() {
    const btn = document.getElementById('labelToggleBtn');
    if (!btn) return;
    btn.textContent = gisState.labelsVisible ? 'Labels On' : 'Labels Off';
    btn.classList.toggle('active', gisState.labelsVisible);
}

function setAllLayersVisibility(visible) {
    gisState.layers.forEach(layer => {
        layer.visible = visible;
    });

    Object.values(gisState.divisionPolygons).forEach(layer => {
        if (visible) {
            layer.addTo(gisMap);
        } else {
            gisMap.removeLayer(layer);
        }
    });

    Object.values(gisState.districtPolygons).forEach(layer => {
        if (visible) {
            layer.addTo(gisMap);
        } else {
            gisMap.removeLayer(layer);
        }
    });

    if (gisState.markerGroup) {
        if (visible) {
            gisState.markerGroup.addTo(gisMap);
        } else {
            gisMap.removeLayer(gisState.markerGroup);
        }
    }

    renderLayers();
}

function toggleMarkerClustering(forceState) {
    const desired = (typeof forceState === 'boolean') ? forceState : !gisState.useMarkerClustering;

    if (desired && !isMarkerClusteringAvailable()) {
        gisState.useMarkerClustering = false;
        syncClusterToggleUI();
        setDrawQueryResult('Marker clustering plugin is not available in this environment.', true);
        return;
    }

    gisState.useMarkerClustering = desired;
    renderMarkersOnMap();
    updateShareLinkPreview();
}

function syncClusterToggleUI() {
    const isAvailable = isMarkerClusteringAvailable();
    const isOn = gisState.useMarkerClustering && isAvailable;

    const text = !isAvailable ? 'Clustering: Unavailable' : (isOn ? 'Clustering: On' : 'Clustering: Off');

    const panelBtn = document.getElementById('clusterToggleBtn');
    if (panelBtn) {
        panelBtn.textContent = text;
        panelBtn.classList.toggle('active', isOn);
    }

    const mapBtn = document.getElementById('clusterToolBtn');
    if (mapBtn) {
        mapBtn.classList.toggle('active', isOn);
        mapBtn.title = isOn ? 'Disable Marker Clustering' : 'Enable Marker Clustering';
    }
}

function initDrawQueryTools() {
    gisState.drawLayerGroup = window.__axm.featureGroup().addTo(gisMap);

    if (!isDrawToolAvailable()) {
        setDrawQueryResult('Draw tools unavailable. Map Draw is not loaded.', true);
        return;
    }

    gisMap.on(window.__axm.Draw.Event.CREATED, function (event) {
        if (!gisState.drawLayerGroup) return;

        gisState.drawLayerGroup.clearLayers();
        const layer = event.layer;
        if (layer.setStyle) {
            layer.setStyle({ color: '#0ea5e9', weight: 2, dashArray: '6,4', fillOpacity: 0.08 });
        }

        gisState.drawLayerGroup.addLayer(layer);
        gisState.activeQueryShape = layer;
        switchDataTab('tools');
        runSpatialQuery(layer);
    });

    gisMap.on(window.__axm.Draw.Event.DELETED, function () {
        gisState.activeQueryShape = null;
        setDrawQueryResult('Selection cleared. Draw a new area to query.', false);
    });
}

function isDrawToolAvailable() {
    return typeof window.__axm !== 'undefined' && typeof window.__axm.Draw !== 'undefined' && typeof window.__axm.Draw.Rectangle === 'function';
}

function startDrawTool(toolType) {
    switchDataTab('tools');

    if (!isDrawToolAvailable()) {
        setDrawQueryResult('Draw tools unavailable. Map Draw plugin failed to load.', true);
        return;
    }

    const shapeOptions = { color: '#0ea5e9', weight: 2, dashArray: '6,4', fillOpacity: 0.08 };

    if (toolType === 'rectangle') {
        const drawer = new window.__axm.Draw.Rectangle(gisMap, { shapeOptions: shapeOptions });
        drawer.enable();
        return;
    }

    if (toolType === 'polygon') {
        const drawer = new window.__axm.Draw.Polygon(gisMap, {
            allowIntersection: false,
            showArea: true,
            shapeOptions: shapeOptions,
        });
        drawer.enable();
    }
}

function clearDrawQuery() {
    if (gisState.drawLayerGroup) {
        gisState.drawLayerGroup.clearLayers();
    }
    gisState.activeQueryShape = null;
    setDrawQueryResult('Selection cleared. Draw an area and run query.', false);
}

function runSpatialQuery(overrideShape) {
    const shape = overrideShape || gisState.activeQueryShape;
    if (!shape) {
        setDrawQueryResult('No selection found. Draw rectangle or polygon first.', true);
        return;
    }

    const matchedMarkers = gisState.markers.filter(marker => {
        return isLatLngInsideShape(window.__axm.latLng(marker.lat, marker.lng), shape);
    });

    const matchedRegions = gisState.regions.filter(region => {
        if (!Array.isArray(region.center) || region.center.length < 2) return false;
        return isLatLngInsideShape(window.__axm.latLng(region.center[0], region.center[1]), shape);
    });

    const preview = matchedMarkers.slice(0, 5).map(item => item.name).join(', ');
    const summary = 'Markers: ' + matchedMarkers.length + ' | Regions: ' + matchedRegions.length;
    const details = preview ? ('\nTop: ' + preview) : '';

    setDrawQueryResult(summary + details, false);
}

function isLatLngInsideShape(latLng, shape) {
    if (!shape || !latLng) return false;

    if (shape instanceof window.__axm.Circle) {
        return shape.getLatLng().distanceTo(latLng) <= shape.getRadius();
    }

    if (shape instanceof window.__axm.Rectangle) {
        return shape.getBounds().contains(latLng);
    }

    if (shape instanceof window.__axm.Polygon) {
        const latLngs = shape.getLatLngs();
        const ring = Array.isArray(latLngs[0]) ? latLngs[0] : latLngs;
        return isPointInPolygon(latLng, ring);
    }

    return false;
}

function isPointInPolygon(point, polygon) {
    const x = point.lng;
    const y = point.lat;
    let inside = false;

    for (let i = 0, j = polygon.length - 1; i < polygon.length; j = i++) {
        const xi = polygon[i].lng;
        const yi = polygon[i].lat;
        const xj = polygon[j].lng;
        const yj = polygon[j].lat;

        const intersects = ((yi > y) !== (yj > y))
            && (x < (xj - xi) * (y - yi) / ((yj - yi) || Number.EPSILON) + xi);

        if (intersects) inside = !inside;
    }

    return inside;
}

function setDrawQueryResult(message, isError) {
    const box = document.getElementById('drawQueryResult');
    if (!box) return;

    box.textContent = message;
    box.classList.toggle('error', !!isError);
}

function loadSavedViews() {
    let parsed = [];
    try {
        parsed = JSON.parse(localStorage.getItem(GIS_SAVED_VIEWS_KEY) || '[]');
    } catch (_) {
        parsed = [];
    }

    gisState.savedViews = Array.isArray(parsed) ? parsed : [];
    renderSavedViewsSelect();
}

function renderSavedViewsSelect() {
    const select = document.getElementById('savedViewsSelect');
    if (!select) return;

    select.innerHTML = '';

    if (gisState.savedViews.length === 0) {
        const opt = document.createElement('option');
        opt.value = '';
        opt.textContent = 'No saved views';
        select.appendChild(opt);
        return;
    }

    gisState.savedViews.forEach(view => {
        const opt = document.createElement('option');
        opt.value = view.id;
        opt.textContent = view.name + ' (' + (view.type || 'general') + ')';
        select.appendChild(opt);
    });
}

function persistSavedViews() {
    localStorage.setItem(GIS_SAVED_VIEWS_KEY, JSON.stringify(gisState.savedViews));
    renderSavedViewsSelect();
}

function collectCurrentViewState() {
    const center = gisMap ? gisMap.getCenter() : { lat: 23.685, lng: 90.3563 };
    const zoom = gisMap ? gisMap.getZoom() : 7;
    const datasetSelect = document.getElementById('filterDataset');

    return {
        type: currentDashType,
        center: { lat: Number(center.lat.toFixed(6)), lng: Number(center.lng.toFixed(6)) },
        zoom: zoom,
        visibleLayers: gisState.layers.filter(layer => layer.visible).map(layer => layer.id),
        datasetId: gisState.activeDataset?.id || datasetSelect?.value || '',
        sidebarCollapsed: document.getElementById('gisSidebar')?.classList.contains('collapsed') || false,
        dataPanelCollapsed: document.getElementById('gisDataPanel')?.classList.contains('collapsed') || false,
        workspaceCollapsed: document.getElementById('gisHubContainer')?.classList.contains('tools-collapsed') || false,
        clustering: !!gisState.useMarkerClustering,
    };
}

function buildShareableLink(state) {
    const view = state || collectCurrentViewState();
    const url = new URL(window.location.href);
    const params = url.searchParams;

    params.set('gisType', view.type || currentDashType);
    params.set('gisLat', String(view.center.lat));
    params.set('gisLng', String(view.center.lng));
    params.set('gisZoom', String(view.zoom));
    params.set('gisLayers', (view.visibleLayers || []).join(','));
    params.set('gisDataset', view.datasetId || '');
    params.set('gisCluster', view.clustering ? '1' : '0');
    params.set('gisSB', view.sidebarCollapsed ? '1' : '0');
    params.set('gisDP', view.dataPanelCollapsed ? '1' : '0');
    params.set('gisWP', view.workspaceCollapsed ? '1' : '0');

    url.search = params.toString();
    return url.toString();
}

function updateShareLinkPreview() {
    const input = document.getElementById('shareLinkInput');
    if (!input) return;
    input.value = buildShareableLink();
}

function saveCurrentView() {
    const input = document.getElementById('viewNameInput');
    const name = (input?.value || '').trim() || ('View ' + new Date().toLocaleString());
    const state = collectCurrentViewState();
    state.id = 'view-' + Date.now();
    state.name = name;
    state.savedAt = new Date().toISOString();

    gisState.savedViews.unshift(state);
    gisState.savedViews = gisState.savedViews.slice(0, 50);
    persistSavedViews();

    if (input) input.value = '';
    setDrawQueryResult('Saved view: ' + name, false);
}

function findSelectedSavedView() {
    const select = document.getElementById('savedViewsSelect');
    if (!select || !select.value) return null;
    return gisState.savedViews.find(view => view.id === select.value) || null;
}

async function loadSelectedView() {
    const selected = findSelectedSavedView();
    if (!selected) {
        setDrawQueryResult('Select a saved view first.', true);
        return;
    }

    await applyViewState(selected);
    setDrawQueryResult('Loaded view: ' + selected.name, false);
}

function deleteSelectedView() {
    const selected = findSelectedSavedView();
    if (!selected) {
        setDrawQueryResult('Select a saved view first.', true);
        return;
    }

    gisState.savedViews = gisState.savedViews.filter(view => view.id !== selected.id);
    persistSavedViews();
    setDrawQueryResult('Deleted view: ' + selected.name, false);
}

async function copyShareableLink() {
    const link = buildShareableLink();

    try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
            await navigator.clipboard.writeText(link);
        } else {
            const temp = document.createElement('textarea');
            temp.value = link;
            document.body.appendChild(temp);
            temp.select();
            document.execCommand('copy');
            document.body.removeChild(temp);
        }
        setDrawQueryResult('Shareable link copied to clipboard.', false);
    } catch (_) {
        setDrawQueryResult('Unable to copy link automatically. Copy it manually from the box.', true);
    }

    updateShareLinkPreview();
}

function parseViewStateFromURL() {
    const params = new URLSearchParams(window.location.search);
    const keys = ['gisType', 'gisLat', 'gisLng', 'gisZoom', 'gisLayers', 'gisDataset', 'gisCluster', 'gisSB', 'gisDP', 'gisWP'];
    const hasState = keys.some(key => params.has(key));
    if (!hasState) return null;

    const lat = Number(params.get('gisLat'));
    const lng = Number(params.get('gisLng'));
    const zoom = Number(params.get('gisZoom'));

    return {
        type: params.get('gisType') || currentDashType,
        center: (Number.isFinite(lat) && Number.isFinite(lng)) ? { lat: lat, lng: lng } : null,
        zoom: Number.isFinite(zoom) ? zoom : null,
        visibleLayers: (params.get('gisLayers') || '').split(',').filter(Boolean),
        datasetId: params.get('gisDataset') || '',
        clustering: params.get('gisCluster') === '1',
        sidebarCollapsed: params.get('gisSB') === '1',
        dataPanelCollapsed: params.get('gisDP') === '1',
        workspaceCollapsed: params.get('gisWP') === '1',
    };
}

async function bootstrapViewStateFromURL() {
    const state = parseViewStateFromURL();
    if (!state) return;
    await applyViewState(state);
}

async function applyViewState(state) {
    if (!state) return;

    if (state.type && state.type !== currentDashType) {
        await switchDashboardType(state.type);
    }

    const hub = document.getElementById('gisHubContainer');
    if (hub && typeof state.workspaceCollapsed === 'boolean') {
        hub.classList.toggle('tools-collapsed', state.workspaceCollapsed);
    }
    syncWorkspacePanelsToggle();

    const sidebar = document.getElementById('gisSidebar');
    if (sidebar && typeof state.sidebarCollapsed === 'boolean') {
        sidebar.classList.toggle('collapsed', state.sidebarCollapsed);
    }

    const dataPanel = document.getElementById('gisDataPanel');
    if (dataPanel && typeof state.dataPanelCollapsed === 'boolean') {
        dataPanel.classList.toggle('collapsed', state.dataPanelCollapsed);
    }

    if (typeof state.clustering === 'boolean') {
        gisState.useMarkerClustering = state.clustering;
        renderMarkersOnMap();
    }

    if (Array.isArray(state.visibleLayers) && state.visibleLayers.length > 0) {
        const visible = new Set(state.visibleLayers);
        gisState.layers.forEach(layer => {
            const shouldShow = visible.has(layer.id);
            toggleLayer(layer.id, shouldShow);
        });
        renderLayers();
    }

    if (state.center && typeof state.zoom === 'number') {
        gisMap.setView([state.center.lat, state.center.lng], state.zoom, { animate: false });
    }

    if (state.datasetId) {
        const datasetSelect = document.getElementById('filterDataset');
        if (datasetSelect) datasetSelect.value = state.datasetId;
        await loadDataset(state.datasetId);
    }

    syncClusterToggleUI();
    updateShareLinkPreview();
    setTimeout(() => gisMap.invalidateSize(), 120);
}

function buildCurrentGISJsonPayload() {
    return {
        schema: 'axiomnizam-gis-json-v1',
        exportedAt: new Date().toISOString(),
        dashboardType: currentDashType,
        summary: {
            layerCount: gisState.layers.length,
            regionCount: gisState.regions.length,
            markerCount: gisState.markers.length,
            datasetCount: gisState.datasets.length,
        },
        viewState: collectCurrentViewState(),
        layers: gisState.layers,
        regions: gisState.regions,
        markers: gisState.markers,
        datasets: gisState.datasets,
        activeDataset: gisState.activeDataset || null,
    };
}

function loadCurrentDataAsGISJson() {
    const editor = document.getElementById('gisJsonEditor');
    if (!editor) return;

    const payload = buildCurrentGISJsonPayload();
    editor.value = JSON.stringify(payload, null, 2);
    setGISJsonStatus('Loaded current GIS data into JSON editor.', false);
}

function validateGISJson() {
    const parsed = parseGISJsonEditorInput();
    if (!parsed) return false;

    const normalized = normalizeToGeoJSON(parsed);
    if (normalized) {
        const count = (normalized.features || []).length;
        setGISJsonStatus('Valid JSON. Convertible to GeoJSON with ' + count + ' feature(s).', false);
    } else {
        setGISJsonStatus('Valid JSON, but no recognizable GIS structures found.', true);
    }

    return true;
}

function importGISJsonToMap() {
    const parsed = parseGISJsonEditorInput();
    if (!parsed) return;

    const geo = normalizeToGeoJSON(parsed);
    if (!geo || !Array.isArray(geo.features) || geo.features.length === 0) {
        setGISJsonStatus('Import failed: no GeoJSON features found in provided JSON.', true);
        return;
    }

    clearGISJsonLayer();

    gisState.importedGeoJsonLayer = window.__axm.geoJSON(geo, {
        style: function () {
            return { color: '#0ea5e9', weight: 2, fillOpacity: 0.12 };
        },
        pointToLayer: function (_feature, latlng) {
            return window.__axm.circleMarker(latlng, {
                radius: 5,
                color: '#0ea5e9',
                fillColor: '#22d3ee',
                fillOpacity: 0.8,
                weight: 2,
            });
        },
        onEachFeature: function (feature, layer) {
            if (!feature || !feature.properties) return;
            const keys = Object.keys(feature.properties).slice(0, 8);
            if (keys.length === 0) return;
            const lines = keys.map(k => '<strong>' + k + '</strong>: ' + String(feature.properties[k]));
            layer.bindPopup(lines.join('<br>'));
        },
    }).addTo(gisMap);

    const bounds = gisState.importedGeoJsonLayer.getBounds();
    if (bounds && bounds.isValid()) {
        gisMap.fitBounds(bounds, { padding: [22, 22], maxZoom: 13 });
    }

    setGISJsonStatus('Imported ' + geo.features.length + ' feature(s) to map.', false);
}

function clearGISJsonLayer() {
    if (gisState.importedGeoJsonLayer) {
        gisMap.removeLayer(gisState.importedGeoJsonLayer);
        gisState.importedGeoJsonLayer = null;
        setGISJsonStatus('Cleared imported GIS JSON layer.', false);
    }
}

function downloadGISJson() {
    const editor = document.getElementById('gisJsonEditor');
    const raw = editor ? editor.value.trim() : '';

    let payload = null;
    if (raw) {
        try {
            payload = JSON.parse(raw);
        } catch (_) {
            setGISJsonStatus('Cannot download: JSON editor contains invalid JSON.', true);
            return;
        }
    } else {
        payload = buildCurrentGISJsonPayload();
    }

    const json = JSON.stringify(payload, null, 2);
    const blob = new Blob([json], { type: 'application/json;charset=utf-8;' });
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob);
    link.download = 'gis-' + currentDashType + '-' + new Date().toISOString().replace(/[:.]/g, '-') + '.json';
    link.click();
    URL.revokeObjectURL(link.href);

    setGISJsonStatus('GIS JSON downloaded.', false);
}

function parseGISJsonEditorInput() {
    const editor = document.getElementById('gisJsonEditor');
    if (!editor) return null;

    const raw = (editor.value || '').trim();
    if (!raw) {
        setGISJsonStatus('JSON editor is empty.', true);
        return null;
    }

    try {
        return JSON.parse(raw);
    } catch (err) {
        setGISJsonStatus('Invalid JSON: ' + err.message, true);
        return null;
    }
}

function normalizeToGeoJSON(payload) {
    if (!payload || typeof payload !== 'object') return null;

    if (payload.type === 'FeatureCollection' && Array.isArray(payload.features)) {
        return payload;
    }

    if (payload.type === 'Feature') {
        return { type: 'FeatureCollection', features: [payload] };
    }

    if (payload.type && ['Point', 'MultiPoint', 'LineString', 'MultiLineString', 'Polygon', 'MultiPolygon', 'GeometryCollection'].includes(payload.type)) {
        return { type: 'FeatureCollection', features: [{ type: 'Feature', geometry: payload, properties: {} }] };
    }

    if (payload.geojson && typeof payload.geojson === 'object') {
        return normalizeToGeoJSON(payload.geojson);
    }

    const features = [];

    if (Array.isArray(payload.markers)) {
        payload.markers.forEach(marker => {
            if (!marker || !Number.isFinite(marker.lat) || !Number.isFinite(marker.lng)) return;
            features.push({
                type: 'Feature',
                geometry: { type: 'Point', coordinates: [marker.lng, marker.lat] },
                properties: {
                    source: 'marker',
                    id: marker.id || '',
                    name: marker.name || '',
                    category: marker.category || '',
                    ...(marker.properties || {}),
                },
            });
        });
    }

    if (Array.isArray(payload.regions)) {
        payload.regions.forEach(region => {
            if (region && region.geojson && typeof region.geojson === 'object') {
                const regionGeo = normalizeToGeoJSON(region.geojson);
                if (regionGeo && Array.isArray(regionGeo.features)) {
                    regionGeo.features.forEach(f => {
                        features.push({
                            type: 'Feature',
                            geometry: f.geometry,
                            properties: {
                                source: 'region',
                                id: region.id || '',
                                name: region.name || '',
                                regionType: region.type || '',
                                ...(region.properties || {}),
                                ...(f.properties || {}),
                            },
                        });
                    });
                }
                return;
            }

            if (!region || !Array.isArray(region.center) || region.center.length < 2) return;
            const lat = Number(region.center[0]);
            const lng = Number(region.center[1]);
            if (!Number.isFinite(lat) || !Number.isFinite(lng)) return;

            features.push({
                type: 'Feature',
                geometry: { type: 'Point', coordinates: [lng, lat] },
                properties: {
                    source: 'region-center',
                    id: region.id || '',
                    name: region.name || '',
                    regionType: region.type || '',
                    ...(region.properties || {}),
                },
            });
        });
    }

    if (Array.isArray(payload.features)) {
        payload.features.forEach(feature => {
            if (feature && feature.type === 'Feature' && feature.geometry) {
                features.push(feature);
            }
        });
    }

    if (features.length === 0) return null;
    return { type: 'FeatureCollection', features: features };
}

function setGISJsonStatus(message, isError) {
    const box = document.getElementById('gisJsonStatus');
    if (!box) return;
    box.textContent = message;
    box.classList.toggle('error', !!isError);
}

// =============================================
// UI Panels
// =============================================

function togglePanelMenu(event, menuId) {
    if (event) {
        event.preventDefault();
        event.stopPropagation();
    }

    const menu = document.getElementById(menuId);
    if (!menu) return;

    const willOpen = !menu.classList.contains('open');
    closeAllMenus();

    if (willOpen) {
        menu.classList.add('open');
        if (event && event.currentTarget) {
            event.currentTarget.classList.add('active');
        }
    }
}

function closeAllMenus() {
    document.querySelectorAll('.gis-panel-menu.open').forEach(menu => menu.classList.remove('open'));
    document.querySelectorAll('.gis-more-dot.active, .gis-more-dot-chip.active').forEach(btn => btn.classList.remove('active'));
}

function runPanelAction(action) {
    switch (action) {
        case 'filters-apply':
            applyFilters();
            break;
        case 'filters-clear':
            clearFilters();
            break;
        case 'filters-markers':
            focusMarkersOnly();
            break;
        case 'layers-show-all':
            setAllLayersVisibility(true);
            break;
        case 'layers-hide-all':
            setAllLayersVisibility(false);
            break;
        case 'layers-default-basemap':
            switchBaseMap('CartoDB Light');
            renderLayers();
            break;
        case 'regions-fit-all':
            fitAllPrimaryRegions();
            break;
        case 'regions-reset-view':
            resetMapView();
            break;
        default:
            break;
    }
    closeAllMenus();
}

function fitAllPrimaryRegions() {
    const types = getPrimaryRegionTypes();
    const bounds = window.__axm.latLngBounds([]);

    gisState.regions
        .filter(region => types.includes(region.type) && Array.isArray(region.center))
        .forEach(region => bounds.extend(region.center));

    if (bounds.isValid()) {
        gisMap.fitBounds(bounds, { padding: [24, 24], maxZoom: 10 });
    } else {
        resetMapView();
    }
}

function toggleSidebar() {
    const sidebar = document.getElementById('gisSidebar');
    sidebar.classList.toggle('collapsed');
    updateShareLinkPreview();
    setTimeout(() => gisMap.invalidateSize(), 350);
}

function toggleDataPanel() {
    const panel = document.getElementById('gisDataPanel');
    panel.classList.toggle('collapsed');
    updateShareLinkPreview();
    setTimeout(() => gisMap.invalidateSize(), 350);
}

function switchDataTab(tabName) {
    document.querySelectorAll('.gis-data-tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.gis-data-content').forEach(p => p.classList.remove('active'));

    const tabBtn = document.getElementById('tab' + tabName.charAt(0).toUpperCase() + tabName.slice(1));
    const panel = document.getElementById('panel' + tabName.charAt(0).toUpperCase() + tabName.slice(1));
    if (tabBtn) tabBtn.classList.add('active');
    if (panel) panel.classList.add('active');
}

function toggleLegend() {
    const legend = document.getElementById('gisLegend');
    const body = document.getElementById('legendBody');
    if (!body || !legend) return;

    if (body.style.display === 'none') {
        body.style.display = 'block';
        legend.classList.remove('collapsed');
    } else {
        body.style.display = 'none';
        legend.classList.add('collapsed');
    }
}

// =============================================
// Helpers
// =============================================

function randomColor() {
    const colors = ['#e8f5e9', '#e3f2fd', '#fff3e0', '#f3e5f5', '#e8eaf6', '#e0f2f1', '#fce4ec', '#fff8e1'];
    return colors[Math.floor(Math.random() * colors.length)];
}

function darkenColor(hex) {
    hex = hex.replace('#', '');
    if (hex.length === 3) hex = hex.split('').map(c => c + c).join('');
    const r = Math.max(0, parseInt(hex.substr(0, 2), 16) - 60);
    const g = Math.max(0, parseInt(hex.substr(2, 2), 16) - 60);
    const b = Math.max(0, parseInt(hex.substr(4, 2), 16) - 60);
    return '#' + [r, g, b].map(v => v.toString(16).padStart(2, '0')).join('');
}
