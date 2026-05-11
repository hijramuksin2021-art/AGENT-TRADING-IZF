package trader

import (
	"fmt"
	"nofx/kernel"
	"nofx/logger"
	"nofx/market"
	"nofx/store"
	"time"
)

// executeDecisionWithRecord executes AI decision and records detailed information
func (at *AutoTrader) executeDecisionWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	switch decision.Action {
	case "open_long":
		return at.executeOpenLongWithRecord(decision, actionRecord)
	case "open_short":
		return at.executeOpenShortWithRecord(decision, actionRecord)
	case "close_long":
		return at.executeCloseLongWithRecord(decision, actionRecord)
	case "close_short":
		return at.executeCloseShortWithRecord(decision, actionRecord)
	case "hold", "wait":
		// No execution needed, just record
		return nil
	default:
		return fmt.Errorf("unknown action: %s", decision.Action)
	}
}

// executeOpenLongWithRecord executes open long position and records detailed information
func (at *AutoTrader) executeOpenLongWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  📈 Open long: %s", decision.Symbol)

	// ⚠️ Get current positions for multiple checks
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("failed to get positions: %w", err)
	}

	// [CODE ENFORCED] Check max positions limit
	if err := at.enforceMaxPositions(len(positions)); err != nil {
		return err
	}

	// Check if there's already a position in the same symbol and direction
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
			return fmt.Errorf("❌ %s already has long position, close it first", decision.Symbol)
		}
	}

	isMT5 := at.exchange == "mt5"

	// ── Get current price ─────────────────────────────────────────────────────
	var currentPrice float64
	if isMT5 {
		// MT5: get price directly from the bridge (not CoinAok)
		price, err := at.trader.GetMarketPrice(decision.Symbol)
		if err != nil {
			logger.Infof("  ⚠️ [MT5] Failed to get market price, continuing: %v", err)
		}
		currentPrice = price
	} else {
		marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
		if err != nil {
			return err
		}
		currentPrice = marketData.CurrentPrice
	}

	// ── Calculate quantity ───────────────────────────────────────────────────
	var quantity float64

	if isMT5 {
		// Forex/MT5: quantity = lot size (0.01, 0.10, 1.00, etc.)
		quantity = decision.LotSize
		if quantity <= 0 {
			quantity = 0.01 // Default to micro lot
			logger.Infof("  ⚠️ [MT5] LotSize not set by AI, defaulting to 0.01 lot")
		}
		logger.Infof("  📊 [MT5] Forex trade: %s LONG %.2f lots", decision.Symbol, quantity)
	} else {
		// Crypto: get balance and calculate USD-based quantity
		balance, err := at.trader.GetBalance()
		if err != nil {
			return fmt.Errorf("failed to get account balance: %w", err)
		}
		availableBalance := 0.0
		if avail, ok := balance["availableBalance"].(float64); ok {
			availableBalance = avail
		}
		equity := 0.0
		if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
			equity = eq
		} else if eq, ok := balance["totalWalletBalance"].(float64); ok && eq > 0 {
			equity = eq
		} else {
			equity = availableBalance
		}

		// [CODE ENFORCED] Position Value Ratio Check
		adjustedPositionSize, wasCapped := at.enforcePositionValueRatio(decision.PositionSizeUSD, equity, decision.Symbol)
		if wasCapped {
			decision.PositionSizeUSD = adjustedPositionSize
		}

		// ⚠️ Auto-adjust position size if insufficient margin
		marginFactor := 1.01/float64(decision.Leverage) + 0.001
		maxAffordablePositionSize := availableBalance / marginFactor
		actualPositionSize := decision.PositionSizeUSD
		if actualPositionSize > maxAffordablePositionSize {
			adjustedSize := maxAffordablePositionSize * 0.98
			logger.Infof("  ⚠️ Position size %.2f exceeds max affordable %.2f, auto-reducing to %.2f",
				actualPositionSize, maxAffordablePositionSize, adjustedSize)
			actualPositionSize = adjustedSize
			decision.PositionSizeUSD = actualPositionSize
		}

		// [CODE ENFORCED] Minimum position size check
		if err := at.enforceMinPositionSize(decision.PositionSizeUSD); err != nil {
			return err
		}

		quantity = actualPositionSize / currentPrice
	}

	actionRecord.Quantity = quantity
	actionRecord.Price = currentPrice

	// Set margin mode (no-op for MT5)
	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		logger.Infof("  ⚠️ Failed to set margin mode: %v", err)
	}

	// Open position
	order, err := at.trader.OpenLong(decision.Symbol, quantity, decision.Leverage)
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	logger.Infof("  ✓ Position opened successfully, order ID: %v, quantity: %.4f", order["orderId"], quantity)

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "open_long", quantity, currentPrice, decision.Leverage, 0)

	// Record position opening time
	posKey := decision.Symbol + "_long"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// Set stop loss and take profit
	if err := at.trader.SetStopLoss(decision.Symbol, "LONG", quantity, decision.StopLoss); err != nil {
		logger.Infof("  ⚠ Failed to set stop loss: %v", err)
	}
	if err := at.trader.SetTakeProfit(decision.Symbol, "LONG", quantity, decision.TakeProfit); err != nil {
		logger.Infof("  ⚠ Failed to set take profit: %v", err)
	}

	return nil
}

// executeOpenShortWithRecord executes open short position and records detailed information
func (at *AutoTrader) executeOpenShortWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  📉 Open short: %s", decision.Symbol)

	// ⚠️ Get current positions for multiple checks
	positions, err := at.trader.GetPositions()
	if err != nil {
		return fmt.Errorf("failed to get positions: %w", err)
	}

	// [CODE ENFORCED] Check max positions limit
	if err := at.enforceMaxPositions(len(positions)); err != nil {
		return err
	}

	// Check if there's already a position in the same symbol and direction
	for _, pos := range positions {
		if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
			return fmt.Errorf("❌ %s already has short position, close it first", decision.Symbol)
		}
	}

	isMT5 := at.exchange == "mt5"

	// ── Get current price ─────────────────────────────────────────────────────
	var currentPrice float64
	if isMT5 {
		price, err := at.trader.GetMarketPrice(decision.Symbol)
		if err != nil {
			logger.Infof("  ⚠️ [MT5] Failed to get market price, continuing: %v", err)
		}
		currentPrice = price
	} else {
		marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
		if err != nil {
			return err
		}
		currentPrice = marketData.CurrentPrice
	}

	// ── Calculate quantity ───────────────────────────────────────────────────
	var quantity float64

	if isMT5 {
		quantity = decision.LotSize
		if quantity <= 0 {
			quantity = 0.01
			logger.Infof("  ⚠️ [MT5] LotSize not set by AI, defaulting to 0.01 lot")
		}
		logger.Infof("  📊 [MT5] Forex trade: %s SHORT %.2f lots", decision.Symbol, quantity)
	} else {
		balance, err := at.trader.GetBalance()
		if err != nil {
			return fmt.Errorf("failed to get account balance: %w", err)
		}
		availableBalance := 0.0
		if avail, ok := balance["availableBalance"].(float64); ok {
			availableBalance = avail
		}
		equity := 0.0
		if eq, ok := balance["totalEquity"].(float64); ok && eq > 0 {
			equity = eq
		} else if eq, ok := balance["totalWalletBalance"].(float64); ok && eq > 0 {
			equity = eq
		} else {
			equity = availableBalance
		}

		// [CODE ENFORCED] Position Value Ratio Check
		adjustedPositionSize, wasCapped := at.enforcePositionValueRatio(decision.PositionSizeUSD, equity, decision.Symbol)
		if wasCapped {
			decision.PositionSizeUSD = adjustedPositionSize
		}

		marginFactor := 1.01/float64(decision.Leverage) + 0.001
		maxAffordablePositionSize := availableBalance / marginFactor
		actualPositionSize := decision.PositionSizeUSD
		if actualPositionSize > maxAffordablePositionSize {
			adjustedSize := maxAffordablePositionSize * 0.98
			logger.Infof("  ⚠️ Position size %.2f exceeds max affordable %.2f, auto-reducing to %.2f",
				actualPositionSize, maxAffordablePositionSize, adjustedSize)
			actualPositionSize = adjustedSize
			decision.PositionSizeUSD = actualPositionSize
		}

		if err := at.enforceMinPositionSize(decision.PositionSizeUSD); err != nil {
			return err
		}

		quantity = actualPositionSize / currentPrice
	}

	actionRecord.Quantity = quantity
	actionRecord.Price = currentPrice

	// Set margin mode (no-op for MT5)
	if err := at.trader.SetMarginMode(decision.Symbol, at.config.IsCrossMargin); err != nil {
		logger.Infof("  ⚠️ Failed to set margin mode: %v", err)
	}

	// Open position
	order, err := at.trader.OpenShort(decision.Symbol, quantity, decision.Leverage)
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	logger.Infof("  ✓ Position opened successfully, order ID: %v, quantity: %.4f", order["orderId"], quantity)

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "open_short", quantity, currentPrice, decision.Leverage, 0)

	// Record position opening time
	posKey := decision.Symbol + "_short"
	at.positionFirstSeenTime[posKey] = time.Now().UnixMilli()

	// Set stop loss and take profit
	if err := at.trader.SetStopLoss(decision.Symbol, "SHORT", quantity, decision.StopLoss); err != nil {
		logger.Infof("  ⚠ Failed to set stop loss: %v", err)
	}
	if err := at.trader.SetTakeProfit(decision.Symbol, "SHORT", quantity, decision.TakeProfit); err != nil {
		logger.Infof("  ⚠ Failed to set take profit: %v", err)
	}

	return nil
}

// executeCloseLongWithRecord executes close long position and records detailed information
func (at *AutoTrader) executeCloseLongWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  🔄 Close long: %s", decision.Symbol)

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice

	// Normalize symbol for database lookup
	normalizedSymbol := market.Normalize(decision.Symbol)

	// Get entry price and quantity - prioritize local database for accurate quantity
	var entryPrice float64
	var quantity float64

	// First try to get from local database (more accurate for quantity)
	if at.store != nil {
		if openPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, normalizedSymbol, "LONG"); err == nil && openPos != nil {
			quantity = openPos.Quantity
			entryPrice = openPos.EntryPrice
			logger.Infof("  📊 Using local position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
		}
	}

	// Fallback to exchange API if local data not found
	if quantity == 0 {
		positions, err := at.trader.GetPositions()
		if err == nil {
			for _, pos := range positions {
				if pos["symbol"] == decision.Symbol && pos["side"] == "long" {
					if ep, ok := pos["entryPrice"].(float64); ok {
						entryPrice = ep
					}
					if amt, ok := pos["positionAmt"].(float64); ok && amt > 0 {
						quantity = amt
					}
					break
				}
			}
		}
		logger.Infof("  📊 Using exchange position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
	}

	// Close position
	order, err := at.trader.CloseLong(decision.Symbol, 0) // 0 = close all
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "close_long", quantity, marketData.CurrentPrice, 0, entryPrice)

	logger.Infof("  ✓ Position closed successfully")
	return nil
}

// executeCloseShortWithRecord executes close short position and records detailed information
func (at *AutoTrader) executeCloseShortWithRecord(decision *kernel.Decision, actionRecord *store.DecisionAction) error {
	logger.Infof("  🔄 Close short: %s", decision.Symbol)

	// Get current price
	marketData, err := market.GetWithExchange(decision.Symbol, at.exchange)
	if err != nil {
		return err
	}
	actionRecord.Price = marketData.CurrentPrice

	// Normalize symbol for database lookup
	normalizedSymbol := market.Normalize(decision.Symbol)

	// Get entry price and quantity - prioritize local database for accurate quantity
	var entryPrice float64
	var quantity float64

	// First try to get from local database (more accurate for quantity)
	if at.store != nil {
		if openPos, err := at.store.Position().GetOpenPositionBySymbol(at.id, normalizedSymbol, "SHORT"); err == nil && openPos != nil {
			quantity = openPos.Quantity
			entryPrice = openPos.EntryPrice
			logger.Infof("  📊 Using local position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
		}
	}

	// Fallback to exchange API if local data not found
	if quantity == 0 {
		positions, err := at.trader.GetPositions()
		if err == nil {
			for _, pos := range positions {
				if pos["symbol"] == decision.Symbol && pos["side"] == "short" {
					if ep, ok := pos["entryPrice"].(float64); ok {
						entryPrice = ep
					}
					if amt, ok := pos["positionAmt"].(float64); ok {
						quantity = -amt // positionAmt is negative for short
					}
					break
				}
			}
		}
		logger.Infof("  📊 Using exchange position data: qty=%.8f, entry=%.2f", quantity, entryPrice)
	}

	// Close position
	order, err := at.trader.CloseShort(decision.Symbol, 0) // 0 = close all
	if err != nil {
		return err
	}

	// Record order ID
	if orderID, ok := order["orderId"].(int64); ok {
		actionRecord.OrderID = orderID
	}

	// Record order to database and poll for confirmation
	at.recordAndConfirmOrder(order, decision.Symbol, "close_short", quantity, marketData.CurrentPrice, 0, entryPrice)

	logger.Infof("  ✓ Position closed successfully")
	return nil
}
