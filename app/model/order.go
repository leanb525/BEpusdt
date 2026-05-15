package model

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spf13/cast"
)

const (
	OrderNotifyStateSucc = 1 // 回调成功
	OrderNotifyStateFail = 0 // 回调失败

	OrderStatusWaiting    = 1 // 等待支付
	OrderStatusSuccess    = 2 // 交易确认成功
	OrderStatusExpired    = 3 // 订单过期
	OrderStatusCanceled   = 4 // 订单取消
	OrderStatusConfirming = 5 // 等待交易确认
	OrderStatusFailed     = 6 // 交易确认失败

	BscBnb      TradeType = "bsc.bnb"
	EthereumEth TradeType = "ethereum.eth"
	TronTrx     TradeType = "tron.trx"

	UsdtTrc20    TradeType = "usdt.trc20"
	UsdcTrc20    TradeType = "usdc.trc20"
	UsdtPolygon  TradeType = "usdt.polygon"
	UsdcPolygon  TradeType = "usdc.polygon"
	UsdtArbitrum TradeType = "usdt.arbitrum"
	UsdcArbitrum TradeType = "usdc.arbitrum"
	UsdtErc20    TradeType = "usdt.erc20"
	UsdcErc20    TradeType = "usdc.erc20"
	UsdtBep20    TradeType = "usdt.bep20"
	UsdcBep20    TradeType = "usdc.bep20"
	UsdtXlayer   TradeType = "usdt.xlayer"
	UsdcXlayer   TradeType = "usdc.xlayer"
	UsdcBase     TradeType = "usdc.base"
	UsdtSolana   TradeType = "usdt.solana"
	UsdcSolana   TradeType = "usdc.solana"
	UsdtAptos    TradeType = "usdt.aptos"
	UsdcAptos    TradeType = "usdc.aptos"
	UsdtPlasma   TradeType = "usdt.plasma"
)

const (
	OrderApiTypeEpusdt = "epusdt" // epusdt
	OrderApiTypeEpay   = "epay"   // 彩虹易支付
	OrderApiTypeAdmin  = "admin"  // 管理后台
)

type Order struct {
	Id
	OrderId       string     `gorm:"column:order_id;type:varchar(128);not null;index;comment:商户ID" json:"order_id"`
	TradeId       string     `gorm:"column:trade_id;type:varchar(128);not null;uniqueIndex;comment:本地ID" json:"trade_id"`
	TradeType     TradeType  `gorm:"column:trade_type;type:varchar(20);not null;index;comment:交易类型" json:"trade_type"`
	Fiat          Fiat       `gorm:"column:fiat;type:varchar(16);not null;index;default:CNY;comment:法定货币" json:"fiat"`
	Crypto        Crypto     `gorm:"column:crypto;type:varchar(16);not null;index;default:USDT;comment:加密货币" json:"crypto"`
	CurrencyLimit string     `gorm:"column:currency_limit;type:varchar(255);not null;default:'';comment:限定币种" json:"currency_limit"`
	Rate          string     `gorm:"column:rate;type:varchar(10);not null;comment:交易汇率" json:"rate"`
	Amount        string     `gorm:"column:amount;type:varchar(32);not null;default:0.00;comment:交易数额" json:"amount"`
	Money         string     `gorm:"column:money;type:varchar(32);not null;default:0.00;comment:交易金额" json:"money"`
	Address       string     `gorm:"column:address;type:varchar(128);index;not null;comment:收款地址" json:"address"`
	FromAddress   string     `gorm:"column:from_address;type:varchar(128);not null;default:'';comment:支付地址" json:"from_address"`
	AddressLocked bool       `gorm:"column:address_locked;not null;default:false;comment:地址锁定 1:独占 0:共享" json:"address_locked"`
	Status        int        `gorm:"column:status;not null;default:1;index;index:idx_order_notify_retry,priority:1;comment:交易状态" json:"status"`
	Name          string     `gorm:"column:name;type:varchar(64);not null;default:'';comment:商品名称" json:"name"`
	ApiType       string     `gorm:"column:api_type;type:varchar(20);not null;default:'epusdt';comment:API类型" json:"api_type"`
	ReturnUrl     string     `gorm:"column:return_url;type:varchar(255);not null;default:'';comment:同步地址" json:"return_url"`
	NotifyUrl     string     `gorm:"column:notify_url;type:varchar(255);not null;default:'';comment:异步地址" json:"notify_url"`
	NotifyNum     int        `gorm:"column:notify_num;not null;default:0;index:idx_order_notify_retry,priority:3;comment:回调次数" json:"notify_num"`
	NotifyState   int        `gorm:"column:notify_state;not null;default:0;index:idx_order_notify_retry,priority:2;comment:回调状态 1：成功 0：失败" json:"notify_state"`
	RefHash       string     `gorm:"column:ref_hash;type:varchar(128);not null;default:'';index;comment:交易哈希" json:"ref_hash"`
	RefBlockNum   int        `gorm:"column:ref_block_num;not null;default:0;comment:区块索引" json:"ref_block_num"`
	ExpiredAt     time.Time  `gorm:"column:expired_at;not null;comment:失效时间" json:"expired_at"`
	ConfirmedAt   *time.Time `gorm:"column:confirmed_at;not null;comment:交易确认时间" json:"confirmed_at"`
	AutoTimeAt
}

func (o *Order) SetCanceled() error {
	o.Status = OrderStatusCanceled

	return Db.Save(o).Error
}

func (o *Order) SetExpired() {
	o.Status = OrderStatusExpired

	Db.Save(o)
}

func (o *Order) SetSuccess() {
	o.Status = OrderStatusSuccess

	Db.Save(o)
}

func (o *Order) SetFailed() {
	o.Status = OrderStatusFailed

	Db.Save(o)
}

func (o *Order) MarkConfirming(blockNum int, from, hash string, at time.Time, amount decimal.Decimal) {
	o.FromAddress = from
	o.ConfirmedAt = &at
	o.RefHash = hash
	o.RefBlockNum = blockNum
	o.Status = OrderStatusConfirming
	if o.AddressLocked {
		rate, _ := decimal.NewFromString(o.Rate)
		o.Amount = amount.String()
		o.Money = rate.Mul(amount).String()
	}

	Db.Save(o)
}

func (o *Order) SetNotifyState(state int) error {
	// ABA 防护：用 WHERE notify_num = ? 防止两个并发 retry 都把计数从 N 写成 N+1 导致丢失；
	// 仅更新 notify_num + notify_state 两列，避免全字段 Save 把过期的内存值覆盖落盘
	res := Db.Model(&Order{}).
		Where("id = ? and notify_num = ?", o.ID, o.NotifyNum).
		Updates(map[string]any{
			"notify_num":   o.NotifyNum + 1,
			"notify_state": state,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 命中并发：另一个 goroutine 已经把 notify_num 写过了，本次跳过
		return errors.New("notify state conflict, another retry is in progress")
	}

	o.NotifyNum += 1
	o.NotifyState = state

	return nil
}

func (o *Order) GetStatusLabel() string {
	label := "🟢收款成功"
	if o.Status == OrderStatusExpired {
		label = "🔴交易过期"
	}
	if o.Status == OrderStatusWaiting {
		label = "🟡等待支付"
	}
	if o.Status == OrderStatusCanceled {
		label = "⚪️订单取消"
	}

	return label
}

func (o *Order) GetStatusEmoji() string {
	label := "🟢"
	if o.Status == OrderStatusExpired {
		label = "🔴"
	}
	if o.Status == OrderStatusWaiting {
		label = "🟡"
	}
	if o.Status == OrderStatusCanceled {
		label = "⚪️"
	}

	return label
}

func (o *Order) GetTxUrl() string {
	return GetTxUrl(o.TradeType, o.RefHash)
}

func (o *Order) TableName() string {
	return "bep_order"
}

func GetTradeOrder(tradeId string) (Order, bool) {
	var order Order
	res := Db.Where("trade_id = ?", tradeId).Limit(1).Find(&order)

	return order, res.RowsAffected > 0
}

func GetOrderByStatus(Status int) []Order {
	orders := make([]Order, 0)

	Db.Where("status = ?", Status).Find(&orders)

	return orders
}

func GetNotifyFailedTradeOrders() ([]Order, error) {
	var orders []Order
	maxRetry := cast.ToInt(GetC(NotifyMaxRetry))
	if maxRetry <= 0 {
		maxRetry = cast.ToInt(defaultConf[NotifyMaxRetry])
	}

	res := Db.Where("status = ?", OrderStatusSuccess).
		Where("notify_state = ?", OrderNotifyStateFail).
		Where("notify_num <= ?", maxRetry).Find(&orders)

	return orders, res.Error
}

// CalcTradeAmount 计算当前实际可用的交易金额
func CalcTradeAmount(address []string, rate decimal.Decimal, p OrderParams) (string, string, error) {
	if p.AddressLocked {
		return LockTradeAddress(address, p.TradeType)
	}

	// 只 Select 两列，避免把 Order 全字段（含 notify_url/return_url 等 varchar(255)）都加载到内存；
	// 高并发下单时这里被频繁触发，列裁剪能显著降 IO 与内存分配
	var rows []struct {
		Address string
		Amount  string
	}
	lock := make(map[string]bool)
	status := []int{OrderStatusConfirming, OrderStatusWaiting}
	Db.Model(&Order{}).Select("address", "amount").
		Where("status in (?) and trade_type = ?", status, p.TradeType).Find(&rows)
	for _, r := range rows {
		lock[r.Address+r.Amount] = true
	}

	atom, precision := GetAtomicity(p.TradeType)
	if rate.LessThanOrEqual(decimal.Zero) || precision <= 0 {
		return "", "", errors.New(fmt.Sprintf("[%v - %v]原子颗粒度计算异常，联系管理员处理！", atom, precision))
	}

	amount := p.Money.DivRound(rate, precision)
	if amount.LessThan(atom) { // 低于最小原子精度，从最小原子精度开始计算
		amount = atom
	}

	var i = 0
	var m = 100
	for {
		for _, addr := range address {
			k := addr + amount.String()
			if _, ok := lock[k]; ok {
				continue
			}

			return addr, amount.String(), nil
		}

		// 已经被占用，每次递增一个原子精度
		amount = amount.Add(atom)
		if i++; i > m {
			return "", "", errors.New("计算交易金额异常，联系管理员处理！")
		}
	}
}

// LockTradeAddress 检测交易地址，独占使用
// 原实现为每个 address 一次 SELECT 的 N+1 查询；改为一次 IN 查询拿出全部已占用的地址，
// 内存里做差集判定，N 个地址 → 1 次 DB
func LockTradeAddress(address []string, t TradeType) (string, string, error) {
	zero := decimal.Zero.String()
	status := []int{OrderStatusConfirming, OrderStatusWaiting}

	var taken []string
	Db.Model(&Order{}).
		Where("address in (?) and status in (?) and trade_type = ? and address_locked = ?", address, status, t, true).
		Distinct().Pluck("address", &taken)

	takenSet := make(map[string]struct{}, len(taken))
	for _, a := range taken {
		takenSet[a] = struct{}{}
	}

	for _, addr := range address {
		if _, ok := takenSet[addr]; !ok {

			return addr, zero, nil
		}
	}

	return "", zero, errors.New("暂无可用钱包地址")
}

// CalcTradeExpiredAt 计算订单过期时间 最小180，最大3600，默认1200
func CalcTradeExpiredAt(sec int64) time.Time {
	if sec >= 180 && sec <= 3600 {
		return time.Now().Add(time.Duration(sec) * time.Second)
	}

	return time.Now().Add(time.Duration(cast.ToUint64(GetC(PaymentTimeout))) * time.Second)
}

func GetAtomicity(t TradeType) (decimal.Decimal, int32) {
	confKey, ok := GetTradeAtomKey(t)
	if !ok {
		confKey = "atom_usdt"
	}

	atom, _ := decimal.NewFromString(GetC(confKey))

	return atom, cast.ToInt32(math.Abs(float64(atom.Exponent())))
}
