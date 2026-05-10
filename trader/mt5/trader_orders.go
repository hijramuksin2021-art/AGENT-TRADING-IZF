package mt5

import (
	"encoding/json"
	"fmt"
	"nofx/logger"
	"strings"
)

// OpenLong opens a long (BUY) position on MT5
func (t *MT5Trader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	logger.Infof("📈 [MT5] Opening LONG %s qty=%.4f", symbol, quantity)

	payload := map[string]interface{}{
		"symbol":  symbol,
		"side":    "BUY",
		"volume":  quantity,
		"comment": "NOFX_LONG",
		"magic":   123456,
	}

	body, err := t.doPost("/order/open", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to open long: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse order response: %w", err)
	}

	logger.Infof("✅ [MT5] Long opened: %v", result)
	return result, nil
}

// OpenShort opens a short (SELL) position on MT5
func (t *MT5Trader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	logger.Infof("📉 [MT5] Opening SHORT %s qty=%.4f", symbol, quantity)

	payload := map[string]interface{}{
		"symbol":  symbol,
		"side":    "SELL",
		"volume":  quantity,
		"comment": "NOFX_SHORT",
		"magic":   123456,
	}

	body, err := t.doPost("/order/open", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to open short: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse order response: %w", err)
	}

	logger.Infof("✅ [MT5] Short opened: %v", result)
	return result, nil
}

// CloseLong closes a long position (quantity=0 means close all)
func (t *MT5Trader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	logger.Infof("📊 [MT5] Closing LONG %s qty=%.4f", symbol, quantity)

	payload := map[string]interface{}{
		"symbol": symbol,
		"volume": quantity,
	}

	body, err := t.doPost("/order/close", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to close long: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse close response: %w", err)
	}

	return result, nil
}

// CloseShort closes a short position (quantity=0 means close all)
func (t *MT5Trader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	logger.Infof("📊 [MT5] Closing SHORT %s qty=%.4f", symbol, quantity)

	payload := map[string]interface{}{
		"symbol": symbol,
		"volume": quantity,
	}

	body, err := t.doPost("/order/close", payload)
	if err != nil {
		return nil, fmt.Errorf("failed to close short: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse close response: %w", err)
	}

	return result, nil
}

// SetLeverage sets leverage - in MT5, leverage is per-account, not per-symbol
func (t *MT5Trader) SetLeverage(symbol string, leverage int) error {
	// MT5 leverage is set at account level, not per-symbol
	// This is a no-op since we can't change it via API
	logger.Infof("ℹ️ [MT5] Leverage is account-level in MT5 (requested %dx for %s)", leverage, symbol)
	return nil
}

// SetMarginMode sets margin mode - MT5 doesn't support switching dynamically
func (t *MT5Trader) SetMarginMode(symbol string, isCrossMargin bool) error {
	// MT5 margin mode is set at account level
	logger.Infof("ℹ️ [MT5] Margin mode is account-level in MT5")
	return nil
}

// GetMarketPrice gets market price from MT5
func (t *MT5Trader) GetMarketPrice(symbol string) (float64, error) {
	path := fmt.Sprintf("/market-price?symbol=%s", symbol)
	body, err := t.doGet(path)
	if err != nil {
		return 0, fmt.Errorf("failed to get market price: %w", err)
	}

	var result struct {
		Bid  float64 `json:"bid"`
		Ask  float64 `json:"ask"`
		Last float64 `json:"last"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("parse price response: %w", err)
	}

	// Use mid price
	price := result.Last
	if price <= 0 {
		price = (result.Bid + result.Ask) / 2
	}

	return price, nil
}

// SetStopLoss sets stop-loss for a position
func (t *MT5Trader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	logger.Infof("🛡️ [MT5] Setting SL for %s %s @ %.5f", symbol, positionSide, stopPrice)

	payload := map[string]interface{}{
		"symbol": symbol,
		"sl":     stopPrice,
	}

	_, err := t.doPost("/order/modify", payload)
	if err != nil {
		return fmt.Errorf("failed to set stop loss: %w", err)
	}

	return nil
}

// SetTakeProfit sets take-profit for a position
func (t *MT5Trader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	logger.Infof("🎯 [MT5] Setting TP for %s %s @ %.5f", symbol, positionSide, takeProfitPrice)

	payload := map[string]interface{}{
		"symbol": symbol,
		"tp":     takeProfitPrice,
	}

	_, err := t.doPost("/order/modify", payload)
	if err != nil {
		return fmt.Errorf("failed to set take profit: %w", err)
	}

	return nil
}

// CancelStopLossOrders cancels stop-loss orders
func (t *MT5Trader) CancelStopLossOrders(symbol string) error {
	// In MT5, SL is part of position, set to 0 to remove
	payload := map[string]interface{}{
		"symbol": symbol,
		"sl":     0,
	}
	_, err := t.doPost("/order/modify", payload)
	return err
}

// CancelTakeProfitOrders cancels take-profit orders
func (t *MT5Trader) CancelTakeProfitOrders(symbol string) error {
	// In MT5, TP is part of position, set to 0 to remove
	payload := map[string]interface{}{
		"symbol": symbol,
		"tp":     0,
	}
	_, err := t.doPost("/order/modify", payload)
	return err
}

// CancelAllOrders cancels all pending orders for a symbol
func (t *MT5Trader) CancelAllOrders(symbol string) error {
	payload := map[string]interface{}{
		"symbol": symbol,
	}
	_, err := t.doPost("/order/cancel-all", payload)
	return err
}

// CancelStopOrders cancels stop orders (SL + TP)
func (t *MT5Trader) CancelStopOrders(symbol string) error {
	payload := map[string]interface{}{
		"symbol": symbol,
		"sl":     0,
		"tp":     0,
	}
	_, err := t.doPost("/order/modify", payload)
	return err
}

// FormatQuantity formats quantity to correct precision for MT5
func (t *MT5Trader) FormatQuantity(symbol string, quantity float64) (string, error) {
	// MT5 typically uses 0.01 lot increments for forex
	if strings.Contains(symbol, "JPY") || strings.Contains(symbol, "XAU") {
		return fmt.Sprintf("%.2f", quantity), nil
	}
	return fmt.Sprintf("%.2f", quantity), nil
}

// GetOrderStatus gets order status from MT5
func (t *MT5Trader) GetOrderStatus(symbol string, orderID string) (map[string]interface{}, error) {
	path := fmt.Sprintf("/order/status?ticket=%s", orderID)
	body, err := t.doGet(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get order status: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse order status response: %w", err)
	}

	return result, nil
}
