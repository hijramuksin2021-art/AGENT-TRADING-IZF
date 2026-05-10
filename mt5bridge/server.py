"""
MT5 Bridge Server - Penerjemah antara NOFX Backend (Go) dan MetaTrader 5.

Jalankan: python server.py
Endpoint: http://localhost:5555

Pastikan MetaTrader 5 sudah terbuka dan login sebelum menjalankan server ini.
"""

import json
import time
import logging
from datetime import datetime, timedelta
from flask import Flask, request, jsonify
from flask_cors import CORS

try:
    import MetaTrader5 as mt5
except ImportError:
    print("ERROR: MetaTrader5 package not installed!")
    print("Run: pip install MetaTrader5")
    exit(1)

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [MT5Bridge] %(levelname)s: %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)
logger = logging.getLogger(__name__)

app = Flask(__name__)
CORS(app)  # Allow NOFX backend to call this

# ============================================================
# MT5 Connection Management
# ============================================================

def ensure_mt5_connected():
    """Ensure MT5 is connected, attempt reconnect if not."""
    if not mt5.terminal_info():
        logger.info("🔄 Attempting to connect to MT5...")
        if not mt5.initialize():
            error = mt5.last_error()
            logger.error(f"❌ Failed to connect to MT5: {error}")
            return False, f"MT5 connection failed: {error}"
        logger.info("✅ Connected to MT5")
    return True, None


def get_mt5_error():
    """Get last MT5 error as string."""
    error = mt5.last_error()
    if error:
        return f"MT5 Error {error[0]}: {error[1]}"
    return "Unknown MT5 error"


# ============================================================
# Health & Account Endpoints
# ============================================================

@app.route('/health', methods=['GET'])
def health():
    """Check if MT5 is connected and responsive."""
    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"connected": False, "error": err}), 503

    info = mt5.terminal_info()
    account = mt5.account_info()

    return jsonify({
        "connected": True,
        "terminal": {
            "name": info.name if info else "unknown",
            "company": info.company if info else "unknown",
            "connected": info.connected if info else False,
            "trade_allowed": info.trade_allowed if info else False,
        },
        "account": {
            "login": account.login if account else 0,
            "server": account.server if account else "unknown",
            "currency": account.currency if account else "USD",
            "balance": account.balance if account else 0,
            "leverage": account.leverage if account else 0,
            "trade_mode": "demo" if (account and account.trade_mode == 0) else "live",
        } if account else None
    })


@app.route('/account', methods=['GET'])
def get_account():
    """Get account balance and info."""
    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"error": err}), 503

    account = mt5.account_info()
    if account is None:
        return jsonify({"error": get_mt5_error()}), 500

    return jsonify({
        "balance": account.balance,
        "equity": account.equity,
        "margin": account.margin,
        "free_margin": account.margin_free,
        "profit": account.profit,
        "currency": account.currency,
        "leverage": account.leverage,
        "login": account.login,
        "server": account.server,
        "trade_mode": "demo" if account.trade_mode == 0 else "live",
        # NOFX compatible fields
        "totalWalletBalance": account.balance,
        "availableBalance": account.margin_free,
        "totalUnrealizedProfit": account.profit,
        "totalEquity": account.equity,
        "total_equity": account.equity,
    })


# ============================================================
# Positions Endpoint
# ============================================================

@app.route('/positions', methods=['GET'])
def get_positions():
    """Get all open positions."""
    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"error": err}), 503

    positions = mt5.positions_get()
    if positions is None:
        return jsonify({"positions": []})

    result = []
    for pos in positions:
        side = "LONG" if pos.type == mt5.ORDER_TYPE_BUY else "SHORT"
        result.append({
            "ticket": pos.ticket,
            "symbol": pos.symbol,
            "side": side,
            "positionAmt": pos.volume,
            "entryPrice": pos.price_open,
            "markPrice": pos.price_current,
            "unRealizedProfit": pos.profit,
            "leverage": 1,  # MT5 leverage is per-account
            "mgnMode": "cross",
            "notionalValue": pos.volume * pos.price_current,
            "sl": pos.sl,
            "tp": pos.tp,
            "swap": pos.swap,
            "comment": pos.comment,
            "time": int(pos.time * 1000),  # milliseconds
            "magic": pos.magic,
        })

    return jsonify({"positions": result})


# ============================================================
# Market Price Endpoint
# ============================================================

@app.route('/market-price', methods=['GET'])
def get_market_price():
    """Get current market price for a symbol."""
    symbol = request.args.get('symbol', '')
    if not symbol:
        return jsonify({"error": "symbol parameter required"}), 400

    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"error": err}), 503

    tick = mt5.symbol_info_tick(symbol)
    if tick is None:
        # Try with common suffixes
        for suffix in ['.', '.m', '.pro', '.z', '.raw', '.std', '']:
            tick = mt5.symbol_info_tick(symbol + suffix)
            if tick:
                break

    if tick is None:
        return jsonify({"error": f"Symbol {symbol} not found"}), 404

    return jsonify({
        "symbol": symbol,
        "bid": tick.bid,
        "ask": tick.ask,
        "last": tick.last if tick.last > 0 else (tick.bid + tick.ask) / 2,
        "spread": round(tick.ask - tick.bid, 6),
        "time": int(tick.time * 1000),
    })


# ============================================================
# Order Endpoints
# ============================================================

@app.route('/order/open', methods=['POST'])
def open_order():
    """Open a new position."""
    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"error": err}), 503

    data = request.json
    symbol = data.get('symbol', '')
    side = data.get('side', '').upper()  # BUY or SELL
    volume = float(data.get('volume', 0.01))
    sl = float(data.get('sl', 0))
    tp = float(data.get('tp', 0))
    comment = data.get('comment', 'NOFX')
    magic = int(data.get('magic', 123456))

    if not symbol or side not in ('BUY', 'SELL'):
        return jsonify({"error": "Invalid symbol or side"}), 400

    # Get symbol info for filling
    symbol_info = mt5.symbol_info(symbol)
    if symbol_info is None:
        # Try resolve symbol
        resolved = resolve_symbol(symbol)
        if resolved:
            symbol = resolved
            symbol_info = mt5.symbol_info(symbol)

    if symbol_info is None:
        return jsonify({"error": f"Symbol {symbol} not found"}), 404

    if not symbol_info.visible:
        if not mt5.symbol_select(symbol, True):
            return jsonify({"error": f"Failed to select symbol {symbol}"}), 500

    # Get current price
    tick = mt5.symbol_info_tick(symbol)
    if tick is None:
        return jsonify({"error": f"Failed to get price for {symbol}"}), 500

    price = tick.ask if side == 'BUY' else tick.bid
    order_type = mt5.ORDER_TYPE_BUY if side == 'BUY' else mt5.ORDER_TYPE_SELL

    # Build order request
    order_request = {
        "action": mt5.TRADE_ACTION_DEAL,
        "symbol": symbol,
        "volume": volume,
        "type": order_type,
        "price": price,
        "deviation": 20,
        "magic": magic,
        "comment": comment,
        "type_time": mt5.ORDER_TIME_GTC,
        "type_filling": mt5.ORDER_FILLING_IOC,
    }

    if sl > 0:
        order_request["sl"] = sl
    if tp > 0:
        order_request["tp"] = tp

    # Send order
    result = mt5.order_send(order_request)
    if result is None:
        return jsonify({"error": get_mt5_error()}), 500

    if result.retcode != mt5.TRADE_RETCODE_DONE:
        return jsonify({
            "error": f"Order failed: {result.comment}",
            "retcode": result.retcode,
        }), 400

    logger.info(f"✅ Order placed: {side} {volume} {symbol} @ {result.price}, ticket={result.order}")

    return jsonify({
        "success": True,
        "order_id": str(result.order),
        "price": result.price,
        "volume": result.volume,
        "comment": result.comment,
    })


@app.route('/order/close', methods=['POST'])
def close_order():
    """Close a position by ticket or symbol."""
    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"error": err}), 503

    data = request.json
    ticket = data.get('ticket', 0)
    symbol = data.get('symbol', '')
    volume = float(data.get('volume', 0))  # 0 = close all

    # Find position to close
    if ticket:
        positions = mt5.positions_get(ticket=int(ticket))
    elif symbol:
        positions = mt5.positions_get(symbol=symbol)
        if not positions:
            resolved = resolve_symbol(symbol)
            if resolved:
                positions = mt5.positions_get(symbol=resolved)
    else:
        return jsonify({"error": "ticket or symbol required"}), 400

    if not positions:
        return jsonify({"error": "No position found to close"}), 404

    results = []
    for pos in positions:
        close_volume = volume if volume > 0 else pos.volume
        close_type = mt5.ORDER_TYPE_SELL if pos.type == mt5.ORDER_TYPE_BUY else mt5.ORDER_TYPE_BUY
        tick = mt5.symbol_info_tick(pos.symbol)
        if not tick:
            results.append({"ticket": pos.ticket, "error": "Failed to get price"})
            continue

        close_price = tick.bid if pos.type == mt5.ORDER_TYPE_BUY else tick.ask

        close_request = {
            "action": mt5.TRADE_ACTION_DEAL,
            "symbol": pos.symbol,
            "volume": close_volume,
            "type": close_type,
            "position": pos.ticket,
            "price": close_price,
            "deviation": 20,
            "magic": pos.magic,
            "comment": "NOFX_CLOSE",
            "type_time": mt5.ORDER_TIME_GTC,
            "type_filling": mt5.ORDER_FILLING_IOC,
        }

        result = mt5.order_send(close_request)
        if result and result.retcode == mt5.TRADE_RETCODE_DONE:
            logger.info(f"✅ Closed position {pos.ticket}: {pos.symbol} @ {result.price}")
            results.append({
                "ticket": pos.ticket,
                "closed": True,
                "price": result.price,
                "volume": result.volume,
                "profit": pos.profit,
            })
        else:
            error = result.comment if result else get_mt5_error()
            results.append({"ticket": pos.ticket, "error": error})

    return jsonify({"results": results})


@app.route('/order/modify', methods=['POST'])
def modify_order():
    """Modify SL/TP for a position."""
    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"error": err}), 503

    data = request.json
    ticket = int(data.get('ticket', 0))
    symbol = data.get('symbol', '')
    sl = float(data.get('sl', 0))
    tp = float(data.get('tp', 0))

    # Find the position
    if ticket:
        positions = mt5.positions_get(ticket=ticket)
    elif symbol:
        positions = mt5.positions_get(symbol=symbol)
        if not positions:
            resolved = resolve_symbol(symbol)
            if resolved:
                positions = mt5.positions_get(symbol=resolved)
    else:
        return jsonify({"error": "ticket or symbol required"}), 400

    if not positions:
        return jsonify({"error": "No position found"}), 404

    results = []
    for pos in positions:
        modify_request = {
            "action": mt5.TRADE_ACTION_SLTP,
            "symbol": pos.symbol,
            "position": pos.ticket,
            "sl": sl if sl > 0 else pos.sl,
            "tp": tp if tp > 0 else pos.tp,
        }

        result = mt5.order_send(modify_request)
        if result and result.retcode == mt5.TRADE_RETCODE_DONE:
            logger.info(f"✅ Modified position {pos.ticket}: SL={sl}, TP={tp}")
            results.append({"ticket": pos.ticket, "modified": True})
        else:
            error = result.comment if result else get_mt5_error()
            results.append({"ticket": pos.ticket, "error": error})

    return jsonify({"results": results})


@app.route('/order/cancel-all', methods=['POST'])
def cancel_all_orders():
    """Cancel all pending orders for a symbol."""
    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"error": err}), 503

    data = request.json
    symbol = data.get('symbol', '')

    orders = mt5.orders_get(symbol=symbol) if symbol else mt5.orders_get()
    if not orders:
        return jsonify({"cancelled": 0})

    cancelled = 0
    for order in orders:
        cancel_request = {
            "action": mt5.TRADE_ACTION_REMOVE,
            "order": order.ticket,
        }
        result = mt5.order_send(cancel_request)
        if result and result.retcode == mt5.TRADE_RETCODE_DONE:
            cancelled += 1

    return jsonify({"cancelled": cancelled})


# ============================================================
# History & Klines Endpoints
# ============================================================

@app.route('/history', methods=['GET'])
def get_history():
    """Get trade history."""
    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"error": err}), 503

    days = int(request.args.get('days', 30))
    limit = int(request.args.get('limit', 100))

    from_date = datetime.now() - timedelta(days=days)
    to_date = datetime.now()

    deals = mt5.history_deals_get(from_date, to_date)
    if deals is None:
        return jsonify({"trades": []})

    trades = []
    for deal in deals[-limit:]:
        if deal.entry == 0:  # Skip entry deals, only show exits
            continue
        trades.append({
            "ticket": deal.ticket,
            "order": deal.order,
            "symbol": deal.symbol,
            "type": "BUY" if deal.type == mt5.DEAL_TYPE_BUY else "SELL",
            "volume": deal.volume,
            "price": deal.price,
            "profit": deal.profit,
            "fee": deal.commission + deal.swap,
            "commission": deal.commission,
            "swap": deal.swap,
            "time": int(deal.time * 1000),
            "comment": deal.comment,
            "magic": deal.magic,
        })

    return jsonify({"trades": trades})


@app.route('/symbols', methods=['GET'])
def get_symbols():
    """Get available trading symbols, optionally filtered by category."""
    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"error": err}), 503

    category_filter = request.args.get('category', '').lower()  # forex, metals, indices, crypto, etc.

    symbols = mt5.symbols_get()
    if symbols is None:
        return jsonify({"symbols": []})

    result = []
    for s in symbols:
        if not s.visible:
            continue

        # Determine category from symbol path
        path_lower = s.path.lower() if s.path else ''
        cat = classify_symbol(s.name, path_lower)

        # Filter by category if requested
        if category_filter and cat != category_filter:
            continue

        result.append({
            "name": s.name,
            "description": s.description,
            "path": s.path,
            "category": cat,
            "spread": s.spread,
            "digits": s.digits,
            "trade_mode": s.trade_mode,
            "volume_min": s.volume_min,
            "volume_max": s.volume_max,
            "volume_step": s.volume_step,
        })

    return jsonify({"symbols": result, "count": len(result)})


def classify_symbol(name, path_lower):
    """Classify a symbol into category based on name and path."""
    name_upper = name.upper()

    # Check path first (most reliable)
    if 'forex' in path_lower or 'fx' in path_lower or 'currency' in path_lower:
        return 'forex'
    if 'metal' in path_lower or 'gold' in path_lower or 'silver' in path_lower or 'commodit' in path_lower:
        return 'metals'
    if 'index' in path_lower or 'indice' in path_lower:
        return 'indices'
    if 'crypto' in path_lower or 'coin' in path_lower:
        return 'crypto'
    if 'stock' in path_lower or 'share' in path_lower or 'equit' in path_lower:
        return 'stocks'
    if 'energ' in path_lower or 'oil' in path_lower or 'gas' in path_lower:
        return 'energy'

    # Fallback: classify by symbol name patterns
    forex_currencies = ['USD', 'EUR', 'GBP', 'JPY', 'AUD', 'NZD', 'CAD', 'CHF']
    metals_patterns = ['XAU', 'XAG', 'GOLD', 'SILVER', 'XPTUSD', 'XPDUSD']
    index_patterns = ['US30', 'US500', 'NAS100', 'USTEC', 'UK100', 'DE30', 'DE40', 'JP225', 'AUS200', 'FRA40', 'ESP35']
    energy_patterns = ['XBRUSD', 'XTIUSD', 'BRENT', 'WTI', 'NGAS', 'USOIL', 'UKOIL']
    crypto_patterns = ['BTC', 'ETH', 'XRP', 'LTC', 'ADA', 'DOT', 'SOL', 'DOGE', 'BNB']

    # Strip common suffixes for matching
    clean = name_upper.replace('.', '').replace('-', '').replace('_', '')
    for suffix in ['M', 'PRO', 'Z', 'RAW', 'STD', 'I', 'E', 'C', 'B']:
        if clean.endswith(suffix) and len(clean) > 6:
            clean = clean[:-len(suffix)]

    for m in metals_patterns:
        if m in clean:
            return 'metals'
    for e in energy_patterns:
        if e in clean:
            return 'energy'
    for idx in index_patterns:
        if idx in clean:
            return 'indices'
    for cr in crypto_patterns:
        if clean.startswith(cr) and ('USD' in clean or 'BTC' in clean):
            return 'crypto'

    # Check if it's a forex pair (6 chars, two 3-letter currencies)
    if len(clean) == 6:
        base = clean[:3]
        quote = clean[3:]
        if base in forex_currencies and quote in forex_currencies:
            return 'forex'

    # Check if contains any forex currency pair pattern
    for c1 in forex_currencies:
        for c2 in forex_currencies:
            if c1 != c2 and c1 + c2 in clean:
                return 'forex'

    return 'other'


@app.route('/klines', methods=['GET'])
def get_klines():
    """Get OHLCV candle data."""
    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"error": err}), 503

    symbol = request.args.get('symbol', 'EURUSD')
    timeframe_str = request.args.get('timeframe', 'H1')
    count = int(request.args.get('count', 100))

    # Map timeframe string to MT5 constant
    tf_map = {
        'M1': mt5.TIMEFRAME_M1, '1m': mt5.TIMEFRAME_M1,
        'M5': mt5.TIMEFRAME_M5, '5m': mt5.TIMEFRAME_M5,
        'M15': mt5.TIMEFRAME_M15, '15m': mt5.TIMEFRAME_M15,
        'M30': mt5.TIMEFRAME_M30, '30m': mt5.TIMEFRAME_M30,
        'H1': mt5.TIMEFRAME_H1, '1h': mt5.TIMEFRAME_H1,
        'H4': mt5.TIMEFRAME_H4, '4h': mt5.TIMEFRAME_H4,
        'D1': mt5.TIMEFRAME_D1, '1d': mt5.TIMEFRAME_D1,
        'W1': mt5.TIMEFRAME_W1, '1w': mt5.TIMEFRAME_W1,
        'MN1': mt5.TIMEFRAME_MN1, '1M': mt5.TIMEFRAME_MN1,
    }
    timeframe = tf_map.get(timeframe_str, mt5.TIMEFRAME_H1)

    rates = mt5.copy_rates_from_pos(symbol, timeframe, 0, count)
    if rates is None:
        # Try resolve
        resolved = resolve_symbol(symbol)
        if resolved:
            rates = mt5.copy_rates_from_pos(resolved, timeframe, 0, count)

    if rates is None:
        return jsonify({"error": f"No data for {symbol}"}), 404

    klines = []
    for r in rates:
        klines.append({
            "time": int(r['time'] * 1000),
            "open": float(r['open']),
            "high": float(r['high']),
            "low": float(r['low']),
            "close": float(r['close']),
            "volume": float(r['tick_volume']),
        })

    return jsonify({"klines": klines})


# ============================================================
# Symbol Resolution Helper
# ============================================================

def resolve_symbol(symbol):
    """Try to find the correct symbol name with broker-specific suffixes."""
    # Try exact match first
    info = mt5.symbol_info(symbol)
    if info:
        return symbol

    # Try common suffixes
    suffixes = ['', '.', '.m', '.pro', '.z', '.raw', '.std', '.i', '.e']
    for suffix in suffixes:
        candidate = symbol + suffix
        info = mt5.symbol_info(candidate)
        if info:
            return candidate

    # Try lowercase
    info = mt5.symbol_info(symbol.lower())
    if info:
        return symbol.lower()

    return None


# ============================================================
# Order Status Endpoint
# ============================================================

@app.route('/order/status', methods=['GET'])
def get_order_status():
    """Get status of an order by ticket."""
    ok, err = ensure_mt5_connected()
    if not ok:
        return jsonify({"error": err}), 503

    ticket = int(request.args.get('ticket', 0))
    if not ticket:
        return jsonify({"error": "ticket parameter required"}), 400

    # Check if it's a pending order
    orders = mt5.orders_get(ticket=ticket)
    if orders:
        order = orders[0]
        return jsonify({
            "ticket": order.ticket,
            "status": "NEW",
            "symbol": order.symbol,
            "type": "BUY" if order.type in (mt5.ORDER_TYPE_BUY, mt5.ORDER_TYPE_BUY_LIMIT, mt5.ORDER_TYPE_BUY_STOP) else "SELL",
            "volume": order.volume_current,
            "price": order.price_open,
            "sl": order.sl,
            "tp": order.tp,
        })

    # Check history
    deals = mt5.history_deals_get(ticket=ticket)
    if deals:
        deal = deals[0]
        return jsonify({
            "ticket": deal.ticket,
            "status": "FILLED",
            "symbol": deal.symbol,
            "volume": deal.volume,
            "price": deal.price,
            "profit": deal.profit,
        })

    return jsonify({"error": "Order not found"}), 404


# ============================================================
# Main
# ============================================================

if __name__ == '__main__':
    logger.info("=" * 60)
    logger.info("🚀 MT5 Bridge Server Starting...")
    logger.info("=" * 60)

    # Try to connect to MT5
    if mt5.initialize():
        account = mt5.account_info()
        if account:
            mode = "DEMO" if account.trade_mode == 0 else "LIVE"
            logger.info(f"✅ Connected to MT5!")
            logger.info(f"   Account: {account.login} ({mode})")
            logger.info(f"   Server:  {account.server}")
            logger.info(f"   Balance: {account.balance} {account.currency}")
            logger.info(f"   Leverage: 1:{account.leverage}")
        else:
            logger.warning("⚠️  Connected to MT5 but no account info available")
    else:
        logger.warning("⚠️  MT5 not running. Start MT5 first, then restart this bridge.")
        logger.warning("   Bridge will try to connect when requests come in.")

    # ─── Ngrok Auto Tunnel ───────────────────────────────────────
    # Attempts to create a public URL automatically so IZF hosted
    # on the internet can reach this local bridge.
    public_url = None
    try:
        from pyngrok import ngrok, conf

        # Optional: set your ngrok authtoken for stable sessions
        # You can get a free token at https://dashboard.ngrok.com
        # Uncomment and fill in your token:
        # conf.get_default().auth_token = "YOUR_NGROK_AUTHTOKEN_HERE"

        tunnel = ngrok.connect(5555, "http")
        public_url = tunnel.public_url

        logger.info("")
        logger.info("=" * 60)
        logger.info("🌐 NGROK TUNNEL ACTIVE")
        logger.info("=" * 60)
        logger.info(f"   Public URL : {public_url}")
        logger.info("")
        logger.info("   👉 Langkah selanjutnya:")
        logger.info("   1. Buka IZF di browser kamu")
        logger.info("   2. Pergi ke Konfigurasi → Exchange → MetaTrader 5")
        logger.info(f"   3. Paste URL ini ke kolom Bridge URL:")
        logger.info(f"      {public_url}")
        logger.info("   4. Klik 'Uji Koneksi' → Simpan Konfigurasi")
        logger.info("")
        logger.info("   ⚠️  URL ini akan berubah setiap kali server di-restart.")
        logger.info("      Daftar akun ngrok GRATIS di https://ngrok.com")
        logger.info("      untuk URL yang lebih stabil.")
        logger.info("=" * 60)

    except ImportError:
        logger.info("")
        logger.info("=" * 60)
        logger.info("⚠️  pyngrok tidak terinstall. Server hanya berjalan lokal.")
        logger.info("   Untuk akses publik dari internet, jalankan:")
        logger.info("   pip install pyngrok")
        logger.info("   Lalu restart server ini.")
        logger.info("")
        logger.info(f"   Jika IZF di PC yang sama, gunakan:")
        logger.info(f"   Bridge URL: http://localhost:5555")
        logger.info("=" * 60)

    except Exception as e:
        logger.warning(f"⚠️  Ngrok gagal dijalankan: {e}")
        logger.warning("   Server tetap berjalan di localhost:5555")
        logger.info(f"   Bridge URL lokal: http://localhost:5555")

    logger.info(f"\n🌐 Local bridge listening on http://localhost:5555")
    logger.info("   Tekan Ctrl+C untuk menghentikan server.\n")

    try:
        app.run(host='127.0.0.1', port=5555, debug=False)
    finally:
        # Cleanup ngrok tunnel on exit
        try:
            from pyngrok import ngrok as _ngrok
            _ngrok.kill()
            logger.info("🔌 Ngrok tunnel closed.")
        except Exception:
            pass

