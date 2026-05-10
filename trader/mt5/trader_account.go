package mt5

import (
	"encoding/json"
	"fmt"
	"nofx/trader/types"
	"time"
)

// GetBalance gets account balance from MT5 via bridge
func (t *MT5Trader) GetBalance() (map[string]interface{}, error) {
	// Check cache
	t.cacheMutex.RLock()
	if t.cachedBalance != nil && time.Since(t.balanceCacheTime) < t.cacheDuration {
		cached := t.cachedBalance
		t.cacheMutex.RUnlock()
		return cached, nil
	}
	t.cacheMutex.RUnlock()

	body, err := t.doGet("/account")
	if err != nil {
		return nil, fmt.Errorf("failed to get MT5 account: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse account response: %w", err)
	}

	// Update cache
	t.cacheMutex.Lock()
	t.cachedBalance = result
	t.balanceCacheTime = time.Now()
	t.cacheMutex.Unlock()

	return result, nil
}

// GetPositions gets all open positions from MT5
func (t *MT5Trader) GetPositions() ([]map[string]interface{}, error) {
	body, err := t.doGet("/positions")
	if err != nil {
		return nil, fmt.Errorf("failed to get MT5 positions: %w", err)
	}

	var result struct {
		Positions []map[string]interface{} `json:"positions"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse positions response: %w", err)
	}

	return result.Positions, nil
}

// GetClosedPnL gets closed position PnL records from MT5 trade history
func (t *MT5Trader) GetClosedPnL(startTime time.Time, limit int) ([]types.ClosedPnLRecord, error) {
	days := 30
	if !startTime.IsZero() {
		days = int(time.Since(startTime).Hours()/24) + 1
	}

	path := fmt.Sprintf("/history?days=%d&limit=%d", days, limit)
	body, err := t.doGet(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get MT5 history: %w", err)
	}

	var result struct {
		Trades []struct {
			Ticket  int64   `json:"ticket"`
			Symbol  string  `json:"symbol"`
			Type    string  `json:"type"`
			Volume  float64 `json:"volume"`
			Price   float64 `json:"price"`
			Profit  float64 `json:"profit"`
			Fee     float64 `json:"fee"`
			Time    int64   `json:"time"`
			Comment string  `json:"comment"`
		} `json:"trades"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse history response: %w", err)
	}

	var records []types.ClosedPnLRecord
	for _, trade := range result.Trades {
		side := "long"
		if trade.Type == "SELL" {
			side = "short"
		}

		closeType := "manual"
		if trade.Comment == "NOFX_CLOSE" {
			closeType = "ai_decision"
		}

		records = append(records, types.ClosedPnLRecord{
			Symbol:      trade.Symbol,
			Side:        side,
			ExitPrice:   trade.Price,
			Quantity:    trade.Volume,
			RealizedPnL: trade.Profit,
			Fee:         trade.Fee,
			ExitTime:    time.UnixMilli(trade.Time),
			OrderID:     fmt.Sprintf("%d", trade.Ticket),
			CloseType:   closeType,
		})
	}

	return records, nil
}

// GetOpenOrders gets pending orders from MT5
func (t *MT5Trader) GetOpenOrders(symbol string) ([]types.OpenOrder, error) {
	// MT5 bridge doesn't have a dedicated pending orders endpoint yet
	// Return empty for now - MT5 mostly uses market orders
	return nil, nil
}
