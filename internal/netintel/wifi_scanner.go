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
}

// NewWiFiScanner creates a new WiFiScanner.
func NewWiFiScanner() *WiFiScanner {
	return &WiFiScanner{
		scanHistory: make([]*WiFiScanResult, 0),
		maxHistory:  50,
	}
}

// Scan triggers a WiFi scan on the default wireless interface and returns results.
func (ws *WiFiScanner) Scan() (*WiFiScanResult, error) {
	start := time.Now()

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
