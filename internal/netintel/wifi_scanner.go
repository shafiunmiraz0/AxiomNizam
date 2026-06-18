package netintel

import (
	"fmt"
	"math"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// =====================================================
// Network Intelligence — WiFi Scanner
// Scans nearby WiFi networks using OS-level commands
// =====================================================

// WiFiNetwork represents a single discovered WiFi network.
type WiFiNetwork struct {
	SSID       string  `json:"ssid"`
	BSSID      string  `json:"bssid"`       // AP MAC address
	Signal     int     `json:"signal_pct"`  // 0-100 percentage
	SignalDBM  int     `json:"signal_dbm"`  // dBm value
	Channel    int     `json:"channel"`
	Frequency  int     `json:"frequency_mhz"`
	Band       string  `json:"band"`        // 2.4GHz, 5GHz, 6GHz
	Encryption string  `json:"encryption"`  // WPA2, WPA3, Open, WEP, etc.
	Auth       string  `json:"auth"`        // PSK, SAE, Open, 802.1X
	Cipher     string  `json:"cipher"`      // CCMP, TKIP, etc.
	Radio      string  `json:"radio"`       // 802.11a/b/g/n/ac/ax
	Distance   float64 `json:"distance_m"` // estimated distance in meters
}

// WiFiScanResult holds the result of a WiFi scan operation.
type WiFiScanResult struct {
	Interface  string        `json:"interface"`
	Networks   []WiFiNetwork `json:"networks"`
	Total      int           `json:"total"`
	ScanTime   time.Time     `json:"scan_at"`
	DurationMs int64         `json:"duration_ms"`
	Platform   string        `json:"platform"`
	Error      string        `json:"error,omitempty"`
}

// WiFiScanner performs OS-level WiFi scanning.
type WiFiScanner struct {
	mu          sync.RWMutex
	lastResult  *WiFiScanResult
	scanHistory []*WiFiScanResult
	maxHistory  int
	simulate    bool // when true, returns simulated scan data
}

// NewWiFiScanner creates a new WiFiScanner.
func NewWiFiScanner() *WiFiScanner {
	return &WiFiScanner{
		scanHistory: make([]*WiFiScanResult, 0),
		maxHistory:  50,
	}
}

// SetSimulate enables or disables simulation mode.
func (ws *WiFiScanner) SetSimulate(on bool) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.simulate = on
}

// SimulateMode returns whether simulation mode is active.
func (ws *WiFiScanner) SimulateMode() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.simulate
}

// Scan triggers a WiFi scan on the default wireless interface and returns results.
func (ws *WiFiScanner) Scan() (*WiFiScanResult, error) {
	start := time.Now()

	ws.mu.RLock()
	simulate := ws.simulate
	ws.mu.RUnlock()

	// If simulate mode is on, skip OS scan
	if simulate {
		return ws.simulateScan(start), nil
	}

	var networks []WiFiNetwork
	var iface string
	var err error

	switch runtime.GOOS {
	case "windows":
		networks, iface, err = ws.scanWindows()
	case "linux":
		networks, iface, err = ws.scanLinux()
	case "darwin":
		networks, iface, err = ws.scanMacOS()
	default:
		return nil, fmt.Errorf("wifi scan not supported on %s", runtime.GOOS)
	}

	if err != nil {
		// Auto-enable simulation on tool-not-found errors
		if isToolNotFoundError(err) {
			ws.mu.Lock()
			ws.simulate = true
			ws.mu.Unlock()
			return ws.simulateScan(start), nil
		}
		return &WiFiScanResult{
			Interface:  iface,
			Networks:   []WiFiNetwork{},
			Total:      0,
			ScanTime:   time.Now(),
			DurationMs: time.Since(start).Milliseconds(),
			Platform:   runtime.GOOS,
			Error:      err.Error(),
		}, err
	}

	result := &WiFiScanResult{
		Interface:  iface,
		Networks:   networks,
		Total:      len(networks),
		ScanTime:   time.Now(),
		DurationMs: time.Since(start).Milliseconds(),
		Platform:   runtime.GOOS,
	}

	ws.mu.Lock()
	ws.lastResult = result
	ws.scanHistory = append(ws.scanHistory, result)
	if len(ws.scanHistory) > ws.maxHistory {
		ws.scanHistory = ws.scanHistory[1:]
	}
	ws.mu.Unlock()

	return result, nil
}

// isToolNotFoundError returns true when the error is caused by missing WiFi tools.
func isToolNotFoundError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "not found in")
}

// GetLastResult returns the most recent scan result without triggering a new scan.
func (ws *WiFiScanner) GetLastResult() *WiFiScanResult {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.lastResult
}

// GetHistory returns past scan results.
func (ws *WiFiScanner) GetHistory() []*WiFiScanResult {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	result := make([]*WiFiScanResult, len(ws.scanHistory))
	copy(result, ws.scanHistory)
	return result
}

// WiFiNetworkDiff represents changes between two scans.
type WiFiNetworkDiff struct {
	Timestamp       time.Time      `json:"timestamp"`
	Added           []WiFiNetwork  `json:"added"`
	Removed         []WiFiNetwork  `json:"removed"`
	SignalChanges   []SignalChange `json:"signal_changes"`
	TotalBefore     int            `json:"total_before"`
	TotalAfter      int            `json:"total_after"`
	ChangesDetected bool           `json:"changes_detected"`
}

// SignalChange represents a signal strength change for a network between scans.
type SignalChange struct {
	SSID       string `json:"ssid"`
	BSSID      string `json:"bssid"`
	OldSignal  int    `json:"old_signal_dbm"`
	NewSignal  int    `json:"new_signal_dbm"`
	Delta      int    `json:"delta_dbm"`
}

// CompareScans compares two scan results and returns the diff.
func (ws *WiFiScanner) CompareScans(old, new *WiFiScanResult) *WiFiNetworkDiff {
	if old == nil || new == nil {
		return &WiFiNetworkDiff{Timestamp: time.Now()}
	}

	oldMap := make(map[string]WiFiNetwork)
	for _, n := range old.Networks {
		key := n.BSSID
		if key == "" {
			key = n.SSID
		}
		oldMap[key] = n
	}

	newMap := make(map[string]WiFiNetwork)
	for _, n := range new.Networks {
		key := n.BSSID
		if key == "" {
			key = n.SSID
		}
		newMap[key] = n
	}

	diff := &WiFiNetworkDiff{
		Timestamp:   time.Now(),
		TotalBefore: len(old.Networks),
		TotalAfter:  len(new.Networks),
		Added:       make([]WiFiNetwork, 0),
		Removed:     make([]WiFiNetwork, 0),
		SignalChanges: make([]SignalChange, 0),
	}

	// Find added and signal-changed networks
	for key, net := range newMap {
		if oldNet, exists := oldMap[key]; !exists {
			diff.Added = append(diff.Added, net)
		} else if oldNet.SignalDBM != net.SignalDBM {
			diff.SignalChanges = append(diff.SignalChanges, SignalChange{
				SSID:      net.SSID,
				BSSID:     net.BSSID,
				OldSignal: oldNet.SignalDBM,
				NewSignal: net.SignalDBM,
				Delta:     net.SignalDBM - oldNet.SignalDBM,
			})
		}
	}

	// Find removed networks
	for key, net := range oldMap {
		if _, exists := newMap[key]; !exists {
			diff.Removed = append(diff.Removed, net)
		}
	}

	diff.ChangesDetected = len(diff.Added) > 0 || len(diff.Removed) > 0 || len(diff.SignalChanges) > 0
	return diff
}

// CompareLastTwoScans compares the two most recent scans.
func (ws *WiFiScanner) CompareLastTwoScans() *WiFiNetworkDiff {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	if len(ws.scanHistory) < 2 {
		return &WiFiNetworkDiff{
			Timestamp:   time.Now(),
			TotalBefore: 0,
			TotalAfter:  0,
		}
	}

	old := ws.scanHistory[len(ws.scanHistory)-2]
	new := ws.scanHistory[len(ws.scanHistory)-1]

	return ws.compareLocked(old, new)
}

func (ws *WiFiScanner) compareLocked(old, new *WiFiScanResult) *WiFiNetworkDiff {
	oldMap := make(map[string]WiFiNetwork)
	for _, n := range old.Networks {
		key := n.BSSID
		if key == "" {
			key = n.SSID
		}
		oldMap[key] = n
	}

	newMap := make(map[string]WiFiNetwork)
	for _, n := range new.Networks {
		key := n.BSSID
		if key == "" {
			key = n.SSID
		}
		newMap[key] = n
	}

	diff := &WiFiNetworkDiff{
		Timestamp:   time.Now(),
		TotalBefore: len(old.Networks),
		TotalAfter:  len(new.Networks),
		Added:       make([]WiFiNetwork, 0),
		Removed:     make([]WiFiNetwork, 0),
		SignalChanges: make([]SignalChange, 0),
	}

	for key, net := range newMap {
		if oldNet, exists := oldMap[key]; !exists {
			diff.Added = append(diff.Added, net)
		} else if oldNet.SignalDBM != net.SignalDBM {
			diff.SignalChanges = append(diff.SignalChanges, SignalChange{
				SSID:      net.SSID,
				BSSID:     net.BSSID,
				OldSignal: oldNet.SignalDBM,
				NewSignal: net.SignalDBM,
				Delta:     net.SignalDBM - oldNet.SignalDBM,
			})
		}
	}

	for key, net := range oldMap {
		if _, exists := newMap[key]; !exists {
			diff.Removed = append(diff.Removed, net)
		}
	}

	diff.ChangesDetected = len(diff.Added) > 0 || len(diff.Removed) > 0 || len(diff.SignalChanges) > 0
	return diff
}

// --- Simulation Fallback ---

func (ws *WiFiScanner) simulateScan(start time.Time) *WiFiScanResult {
	now := time.Now()
	networks := []WiFiNetwork{
		{SSID: "AxiomNizam-Corp", BSSID: "aa:bb:cc:dd:ee:01", Signal: 92, SignalDBM: -34, Channel: 6, Frequency: 2437, Band: "2.4GHz", Encryption: "WPA2-PSK", Auth: "PSK", Cipher: "CCMP", Radio: "802.11n", Distance: 1.2},
		{SSID: "AxiomNizam-5G", BSSID: "aa:bb:cc:dd:ee:02", Signal: 85, SignalDBM: -38, Channel: 36, Frequency: 5180, Band: "5GHz", Encryption: "WPA3-SAE", Auth: "SAE", Cipher: "CCMP", Radio: "802.11ac", Distance: 2.1},
		{SSID: "Guest-WiFi", BSSID: "aa:bb:cc:dd:ee:03", Signal: 78, SignalDBM: -42, Channel: 11, Frequency: 2462, Band: "2.4GHz", Encryption: "WPA2-PSK", Auth: "PSK", Cipher: "CCMP", Radio: "802.11n", Distance: 3.5},
		{SSID: "IoT-Network", BSSID: "aa:bb:cc:dd:ee:04", Signal: 70, SignalDBM: -46, Channel: 1, Frequency: 2412, Band: "2.4GHz", Encryption: "WPA2-PSK", Auth: "PSK", Cipher: "TKIP", Radio: "802.11g", Distance: 5.0},
		{SSID: "", BSSID: "de:ad:be:ef:00:01", Signal: 55, SignalDBM: -54, Channel: 44, Frequency: 5220, Band: "5GHz", Encryption: "Open", Auth: "Open", Cipher: "", Radio: "802.11ac", Distance: 8.3},
		{SSID: "Lab-5GHz", BSSID: "aa:bb:cc:dd:ee:06", Signal: 48, SignalDBM: -58, Channel: 149, Frequency: 5745, Band: "5GHz", Encryption: "WPA3-SAE", Auth: "SAE", Cipher: "CCMP", Radio: "802.11ax", Distance: 10.7},
		{SSID: "Eduroam", BSSID: "11:22:33:44:55:01", Signal: 42, SignalDBM: -61, Channel: 36, Frequency: 5180, Band: "5GHz", Encryption: "WPA2-802.1X", Auth: "802.1X", Cipher: "CCMP", Radio: "802.11ac", Distance: 14.2},
		{SSID: "Neighbors-WiFi", BSSID: "ff:ee:dd:cc:bb:01", Signal: 25, SignalDBM: -72, Channel: 9, Frequency: 2452, Band: "2.4GHz", Encryption: "WPA2-PSK", Auth: "PSK", Cipher: "CCMP", Radio: "802.11n", Distance: 28.5},
		{SSID: "TP-Link_2.4G", BSSID: "50:c7:bf:12:34:56", Signal: 18, SignalDBM: -76, Channel: 13, Frequency: 2472, Band: "2.4GHz", Encryption: "WPA2-PSK", Auth: "PSK", Cipher: "CCMP", Radio: "802.11n", Distance: 35.0},
		{SSID: "AndroidAP", BSSID: "02:00:00:00:00:01", Signal: 12, SignalDBM: -80, Channel: 6, Frequency: 2437, Band: "2.4GHz", Encryption: "WPA2-PSK", Auth: "PSK", Cipher: "CCMP", Radio: "802.11n", Distance: 45.0},
	}

	// Add slight random signal jitter so consecutive scans differ
	for i := range networks {
		jitter := int(now.UnixNano()%7 - 3) // -3 to +3
		networks[i].SignalDBM += jitter
		networks[i].Signal = signalDBMToPct(networks[i].SignalDBM)
		networks[i].Distance = estimateDistance(networks[i].SignalDBM, networks[i].Frequency)
	}

	result := &WiFiScanResult{
		Interface:  "wlan0 (simulated)",
		Networks:   networks,
		Total:      len(networks),
		ScanTime:   now,
		DurationMs: time.Since(start).Milliseconds(),
		Platform:   runtime.GOOS,
		Error:      "",
	}

	ws.mu.Lock()
	ws.lastResult = result
	ws.scanHistory = append(ws.scanHistory, result)
	if len(ws.scanHistory) > ws.maxHistory {
		ws.scanHistory = ws.scanHistory[1:]
	}
	ws.mu.Unlock()

	return result
}

// --- Windows: netsh wlan show networks ---

func (ws *WiFiScanner) scanWindows() ([]WiFiNetwork, string, error) {
	iface := "Wi-Fi"
	out, err := exec.Command("netsh", "wlan", "show", "networks", "mode=bssid").Output()
	if err != nil {
		return nil, iface, fmt.Errorf("netsh wlan failed: %w", err)
	}

	return ws.parseNetshOutput(string(out), iface)
}

func (ws *WiFiScanner) parseNetshOutput(output, iface string) ([]WiFiNetwork, string, error) {
	networks := make([]WiFiNetwork, 0)
	lines := strings.Split(output, "\n")

	var current WiFiNetwork
	inBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)

		if strings.HasPrefix(lower, "ssid") && strings.Contains(line, ":") {
			if inBlock && current.BSSID != "" {
				networks = append(networks, current)
				current = WiFiNetwork{}
			}
			val := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			current.SSID = val
			inBlock = true
		} else if strings.HasPrefix(lower, "bssid") && strings.Contains(line, ":") {
			val := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			current.BSSID = val
		} else if strings.HasPrefix(lower, "signal") && strings.Contains(line, ":") {
			val := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			val = strings.TrimSuffix(val, "%")
			if pct, err := strconv.Atoi(val); err == nil {
				current.Signal = pct
				current.SignalDBM = signalPctToDBM(pct)
			}
		} else if strings.HasPrefix(lower, "channel") && strings.Contains(line, ":") {
			val := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			if ch, err := strconv.Atoi(val); err == nil {
				current.Channel = ch
				current.Frequency = channelToFreq(ch)
				current.Band = channelToBand(ch)
			}
		} else if strings.HasPrefix(lower, "authentication") && strings.Contains(line, ":") {
			val := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			current.Encryption = val
			current.Auth = val
		} else if strings.HasPrefix(lower, "encryption") && strings.Contains(line, ":") {
			val := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			current.Cipher = val
		} else if strings.HasPrefix(lower, "radio type") && strings.Contains(line, ":") {
			val := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			current.Radio = val
		}
	}

	if inBlock && current.BSSID != "" {
		networks = append(networks, current)
	}

	for i := range networks {
		networks[i].Distance = estimateDistance(networks[i].SignalDBM, networks[i].Frequency)
	}

	return networks, iface, nil
}

// --- Linux: iwlist scan ---

func (ws *WiFiScanner) scanLinux() ([]WiFiNetwork, string, error) {
	iface := ws.detectLinuxIface()

	out, err := exec.Command("sudo", "iwlist", iface, "scan").Output()
	if err != nil {
		out2, err2 := exec.Command("iwlist", iface, "scan").Output()
		if err2 != nil {
			return nil, iface, fmt.Errorf("iwlist scan failed: %w (also tried without sudo: %v)", err, err2)
		}
		out = out2
	}

	return ws.parseIwlistOutput(string(out), iface)
}

func (ws *WiFiScanner) detectLinuxIface() string {
	out, err := exec.Command("iw", "dev").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Interface") {
				return strings.TrimSpace(strings.TrimPrefix(line, "Interface"))
			}
		}
	}
	out, err = exec.Command("ip", "-o", "link", "show").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, "wlan") || strings.Contains(line, "wlp") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					name := strings.TrimSuffix(parts[1], ":")
					return name
				}
			}
		}
	}
	return "wlan0"
}

func (ws *WiFiScanner) parseIwlistOutput(output, iface string) ([]WiFiNetwork, string, error) {
	networks := make([]WiFiNetwork, 0)
	blocks := strings.Split(output, "Cell ")

	for _, block := range blocks[1:] {
		network := WiFiNetwork{}

		if idx := strings.Index(block, "Address:"); idx != -1 {
			end := strings.IndexAny(block[idx:], "\n\r")
			if end != -1 {
				network.BSSID = strings.TrimSpace(block[idx+8 : idx+end])
			}
		}

		if idx := strings.Index(block, "ESSID:\""); idx != -1 {
			end := strings.Index(block[idx+7:], "\"")
			if end != -1 {
				network.SSID = block[idx+7 : idx+7+end]
			}
		}

		if idx := strings.Index(block, "Quality="); idx != -1 {
			qualityStr := block[idx+8:]
			eqIdx := strings.Index(qualityStr, "/")
			if eqIdx != -1 {
				qualityStr = qualityStr[:eqIdx]
			}
			parts := strings.Split(qualityStr, "=")
			if len(parts) == 2 {
				if q, err := strconv.Atoi(parts[0]); err == nil {
					network.Signal = q
					network.SignalDBM = signalPctToDBM(q)
				}
			}
		}

		if idx := strings.Index(block, "Frequency:"); idx != -1 {
			freqStr := block[idx+10:]
			end := strings.IndexAny(freqStr, " \n\r")
			if end != -1 {
				freqStr = freqStr[:end]
				freqStr = strings.TrimSuffix(freqStr, " GHz")
				if freq, err := strconv.ParseFloat(freqStr, 64); err == nil {
					network.Frequency = int(freq * 1000)
					network.Band = freqToBand(network.Frequency)
				}
			}
		}

		if idx := strings.Index(block, "Channel="); idx != -1 {
			chStr := block[idx+8:]
			end := strings.IndexAny(chStr, " \n\r")
			if end != -1 {
				chStr = chStr[:end]
				if ch, err := strconv.Atoi(chStr); err == nil {
					network.Channel = ch
				}
			}
		}

		if strings.Contains(block, "WPA2") {
			network.Encryption = "WPA2"
		} else if strings.Contains(block, "WPA3") || strings.Contains(block, "SAE") {
			network.Encryption = "WPA3"
		} else if strings.Contains(block, "WEP") {
			network.Encryption = "WEP"
		} else {
			network.Encryption = "Open"
		}

		network.Distance = estimateDistance(network.SignalDBM, network.Frequency)
		networks = append(networks, network)
	}

	return networks, iface, nil
}

// --- macOS: airport -s ---

func (ws *WiFiScanner) scanMacOS() ([]WiFiNetwork, string, error) {
	iface := "en0"
	airportPath := "/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources/airport"

	out, err := exec.Command(airportPath, "-s").Output()
	if err != nil {
		return nil, iface, fmt.Errorf("airport scan failed: %w", err)
	}

	return ws.parseAirportOutput(string(out), iface)
}

func (ws *WiFiScanner) parseAirportOutput(output, iface string) ([]WiFiNetwork, string, error) {
	networks := make([]WiFiNetwork, 0)
	lines := strings.Split(output, "\n")

	for _, line := range lines[1:] { // skip header
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Airport output: SSID BSSID RSSI CHANNEL HT CC SECURITY
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}

		network := WiFiNetwork{
			SSID:  fields[0],
			BSSID: fields[1],
		}

		if rssi, err := strconv.Atoi(fields[2]); err == nil {
			network.SignalDBM = rssi
			network.Signal = signalDBMToPct(rssi)
		}

		if ch, err := strconv.Atoi(fields[3]); err == nil {
			network.Channel = ch
			network.Frequency = channelToFreq(ch)
			network.Band = channelToBand(ch)
		}

		if len(fields) > 6 {
			network.Encryption = fields[6]
		}

		network.Distance = estimateDistance(network.SignalDBM, network.Frequency)
		networks = append(networks, network)
	}

	return networks, iface, nil
}

// --- Utility Functions ---

func signalPctToDBM(pct int) int {
	if pct <= 0 {
		return -100
	}
	if pct >= 100 {
		return -50
	}
	return -50 + (100-pct)/2
}

func signalDBMToPct(dbm int) int {
	if dbm <= -100 {
		return 0
	}
	if dbm >= -50 {
		return 100
	}
	return 2 * (dbm + 100)
}

func channelToFreq(ch int) int {
	if ch <= 14 {
		return 2407 + ch*5
	}
	if ch <= 165 {
		return 5000 + ch*5
	}
	return 5955 + (ch-1)*5
}

func freqToBand(freqMHz int) string {
	if freqMHz >= 2400 && freqMHz < 2500 {
		return "2.4GHz"
	}
	if freqMHz >= 5000 && freqMHz < 6000 {
		return "5GHz"
	}
	if freqMHz >= 5925 && freqMHz < 7125 {
		return "6GHz"
	}
	return "unknown"
}

func channelToBand(ch int) string {
	if ch <= 14 {
		return "2.4GHz"
	}
	if ch <= 165 {
		return "5GHz"
	}
	return "6GHz"
}

func estimateDistance(signalDBM, freqMHz int) float64 {
	if signalDBM == 0 || freqMHz == 0 {
		return 0
	}
	if signalDBM > -10 {
		return 0.5
	}
	refLoss := 40.0
	distance := math.Pow(10, (float64(-signalDBM)-refLoss)/(10*2.0))
	if distance < 0.5 {
		distance = 0.5
	}
	if distance > 200 {
		distance = 200
	}
	return math.Round(distance*10) / 10
}
