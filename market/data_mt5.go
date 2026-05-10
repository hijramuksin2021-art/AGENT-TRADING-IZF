package market

import (
	"encoding/json"
	"fmt"
	"net/http"
	"nofx/logger"
	"time"
)

const mt5BridgeBaseURL = "http://localhost:5555"

// MT5 bridge kline response structure
type mt5KlineResponse struct {
	Klines []struct {
		Time   int64   `json:"time"`
		Open   float64 `json:"open"`
		High   float64 `json:"high"`
		Low    float64 `json:"low"`
		Close  float64 `json:"close"`
		Volume float64 `json:"volume"`
	} `json:"klines"`
}

// GetFromMT5Bridge fetches market data for a forex symbol from the MT5 bridge
// This is the forex equivalent of GetWithTimeframes, using MT5 bridge /klines endpoint
func GetFromMT5Bridge(symbol string, timeframes []string, primaryTimeframe string, count int) (*Data, error) {
	if len(timeframes) == 0 {
		return nil, fmt.Errorf("at least one timeframe is required")
	}

	if primaryTimeframe == "" {
		primaryTimeframe = timeframes[0]
	}

	// Ensure primary timeframe is in the list
	hasPrimary := false
	for _, tf := range timeframes {
		if tf == primaryTimeframe {
			hasPrimary = true
			break
		}
	}
	if !hasPrimary {
		timeframes = append([]string{primaryTimeframe}, timeframes...)
	}

	if count <= 0 {
		count = 30
	}

	// Store data for all timeframes
	timeframeData := make(map[string]*TimeframeSeriesData)
	var primaryKlines []Kline

	client := &http.Client{Timeout: 15 * time.Second}

	// Fetch klines for each timeframe from MT5 bridge
	for _, tf := range timeframes {
		klines, err := fetchMT5Klines(client, symbol, tf, count+50) // extra bars for indicator calculation
		if err != nil {
			logger.Infof("⚠️ Failed to get %s %s klines from MT5 bridge: %v", symbol, tf, err)
			continue
		}

		if len(klines) == 0 {
			logger.Infof("⚠️ %s %s klines from MT5 bridge is empty", symbol, tf)
			continue
		}

		// Save primary timeframe klines
		if tf == primaryTimeframe {
			primaryKlines = klines
		}

		// Calculate series data for this timeframe
		seriesData := calculateTimeframeSeries(klines, tf, count)
		timeframeData[tf] = seriesData
	}

	// If primary timeframe data is empty, return error
	if len(primaryKlines) == 0 {
		return nil, fmt.Errorf("primary timeframe %s kline data from MT5 bridge is empty for %s", primaryTimeframe, symbol)
	}

	// Calculate current indicators from primary timeframe
	currentPrice := primaryKlines[len(primaryKlines)-1].Close
	currentEMA20 := calculateEMA(primaryKlines, 20)
	currentMACD := calculateMACD(primaryKlines)
	currentRSI7 := calculateRSI(primaryKlines, 7)

	// Calculate price changes
	priceChange1h := calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 60)
	priceChange4h := calculatePriceChangeByBars(primaryKlines, primaryTimeframe, 240)

	logger.Infof("✅ MT5 bridge: fetched %s data — price=%.5f, %d timeframes, %d primary bars",
		symbol, currentPrice, len(timeframeData), len(primaryKlines))

	return &Data{
		Symbol:        symbol,
		CurrentPrice:  currentPrice,
		PriceChange1h: priceChange1h,
		PriceChange4h: priceChange4h,
		CurrentEMA20:  currentEMA20,
		CurrentMACD:   currentMACD,
		CurrentRSI7:   currentRSI7,
		OpenInterest:  nil, // No OI for forex
		FundingRate:   0,   // No funding rate for forex
		TimeframeData: timeframeData,
	}, nil
}

// fetchMT5Klines calls MT5 bridge /klines endpoint and converts to internal Kline format
func fetchMT5Klines(client *http.Client, symbol, timeframe string, count int) ([]Kline, error) {
	url := fmt.Sprintf("%s/klines?symbol=%s&timeframe=%s&count=%d", mt5BridgeBaseURL, symbol, timeframe, count)

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("MT5 bridge request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MT5 bridge returned status %d for %s %s", resp.StatusCode, symbol, timeframe)
	}

	var result mt5KlineResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse MT5 klines: %w", err)
	}

	// Convert to internal Kline format
	klines := make([]Kline, len(result.Klines))
	for i, k := range result.Klines {
		klines[i] = Kline{
			OpenTime:  k.Time,
			Open:      k.Open,
			High:      k.High,
			Low:       k.Low,
			Close:     k.Close,
			Volume:    k.Volume,
			CloseTime: k.Time + getTimeframeMs(timeframe), // Estimate close time
		}
	}

	return klines, nil
}

// getTimeframeMs returns duration in milliseconds for a timeframe string
func getTimeframeMs(tf string) int64 {
	switch tf {
	case "1m", "M1":
		return 60 * 1000
	case "3m":
		return 3 * 60 * 1000
	case "5m", "M5":
		return 5 * 60 * 1000
	case "15m", "M15":
		return 15 * 60 * 1000
	case "30m", "M30":
		return 30 * 60 * 1000
	case "1h", "H1":
		return 60 * 60 * 1000
	case "2h", "H2":
		return 2 * 60 * 60 * 1000
	case "4h", "H4":
		return 4 * 60 * 60 * 1000
	case "1d", "D1":
		return 24 * 60 * 60 * 1000
	case "1w", "W1":
		return 7 * 24 * 60 * 60 * 1000
	default:
		return 60 * 60 * 1000 // Default 1h
	}
}
