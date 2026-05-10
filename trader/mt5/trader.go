package mt5

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"nofx/logger"
	"sync"
	"time"
)

// MT5Trader implements the Trader interface for MetaTrader 5 via Python bridge
type MT5Trader struct {
	bridgeURL string
	client    *http.Client

	// Balance cache
	cacheMutex       sync.RWMutex
	cachedBalance    map[string]interface{}
	balanceCacheTime time.Time
	cacheDuration    time.Duration
}

// NewMT5Trader creates a new MT5 trader connected to the Python bridge
func NewMT5Trader(bridgeURL string) *MT5Trader {
	if bridgeURL == "" {
		bridgeURL = "http://localhost:5555"
	}
	return &MT5Trader{
		bridgeURL:     bridgeURL,
		client:        &http.Client{Timeout: 15 * time.Second},
		cacheDuration: 5 * time.Second,
	}
}

// doGet performs a GET request to the bridge
func (t *MT5Trader) doGet(path string) ([]byte, error) {
	url := t.bridgeURL + path
	resp, err := t.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("MT5 bridge request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("MT5 bridge read failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("MT5 bridge error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("MT5 bridge error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// doPost performs a POST request to the bridge
func (t *MT5Trader) doPost(path string, payload interface{}) ([]byte, error) {
	url := t.bridgeURL + path

	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		bodyReader = io.NopCloser(
			&readCloserWrapper{data: data, pos: 0},
		)
	}

	resp, err := t.client.Post(url, "application/json", bodyReader)
	if err != nil {
		return nil, fmt.Errorf("MT5 bridge request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("MT5 bridge read failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("MT5 bridge error: %s", errResp.Error)
		}
		return nil, fmt.Errorf("MT5 bridge error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// readCloserWrapper wraps byte slice as io.Reader
type readCloserWrapper struct {
	data []byte
	pos  int
}

func (r *readCloserWrapper) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// TestConnection checks if the MT5 bridge is reachable and MT5 is connected
func (t *MT5Trader) TestConnection() (map[string]interface{}, error) {
	body, err := t.doGet("/health")
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse health response: %w", err)
	}

	connected, _ := result["connected"].(bool)
	if !connected {
		return nil, fmt.Errorf("MT5 is not connected")
	}

	logger.Infof("✅ [MT5] Bridge connected, account info available")
	return result, nil
}

// GetExchangeType returns exchange type
func (t *MT5Trader) GetExchangeType() string {
	return "mt5"
}
