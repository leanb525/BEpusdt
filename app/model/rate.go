package model

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/spf13/cast"
	"github.com/tidwall/gjson"
	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/utils"
	"gorm.io/gorm"
)

type Rate struct {
	Id
	Rate    string  `gorm:"column:rate;type:varchar(32);not null;comment:订单汇率" json:"rate"`
	Fiat    string  `gorm:"column:fiat;type:varchar(16);not null;index:idx_rate_lookup,priority:2;comment:法币" json:"fiat"`
	Crypto  string  `gorm:"column:crypto;type:varchar(16);not null;index:idx_rate_lookup,priority:1;comment:加密货币" json:"crypto"`
	RawRate float64 `gorm:"column:raw_rate;type:decimal(10,4);not null;comment:基准汇率" json:"raw_rate"`
	Syntax  string  `gorm:"column:syntax;type:varchar(32);not null;default:'';comment:浮动语法" json:"syntax"`
	AutoTimeAt
}

// rateCache 缓存按 (crypto, fiat) 维度的最新一行 Rate；汇率每小时同步一次，1 分钟内存缓存足够
// GetPaymentItem 每打开收银台会循环 19 次 GetOrderRate，原本每次都要走 DB，现在命中缓存零 IO
var rateCache sync.Map // key: string("crypto:fiat"), value: rateCacheEntry

type rateCacheEntry struct {
	rate Rate
	at   time.Time
}

const rateCacheTTL = time.Minute

func (r *Rate) TableName() string {
	return "bep_rate"
}

func (r *Rate) BeforeCreate(*gorm.DB) error {
	var syntax = GetC(ConfKey(fmt.Sprintf("rate_float_%s_%s", r.Crypto, r.Fiat)))
	if syntax == "" {

		return nil
	}

	r.Syntax = syntax
	r.Rate = cast.ToString(ParseFloatRate(syntax, cast.ToFloat64(r.RawRate)))

	return nil
}

func CoingeckoRate() error {
	var fiats = make([]string, 0)
	for k := range supportFiat {
		fiats = append(fiats, string(k))
	}

	var ids = make([]string, 0)
	var tokens = make(map[CoinId]Crypto)
	for token, id := range supportCrypto {
		ids = append(ids, string(id))
		tokens[id] = token
	}

	var url = fmt.Sprintf("%s/api/v3/simple/price?ids=%s&vs_currencies=%s", GetC(RateSyncCoingeckoApiUrl), strings.Join(ids, ","), strings.Join(fiats, ","))
	var client = &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("x-cg-demo-api-key", GetC(RateSyncCoingeckoApiKey))

	resp, err := client.Do(req)
	if err != nil {

		return err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {

		return err
	}

	if resp.StatusCode != http.StatusOK {

		return errors.New("CoingeckoRate: " + http.StatusText(resp.StatusCode))
	}

	var data = gjson.ParseBytes(body)
	if data.Get("status.error_code").Exists() {

		return errors.New("CoingeckoRate: " + data.Get("status.error_message").String())
	}

	var rows = make([]Rate, 0)
	for id, v := range data.Map() {
		var token, ok = tokens[CoinId(id)]
		if !ok {

			continue
		}

		for fiat, val := range v.Map() {
			rows = append(rows, Rate{
				Rate:    val.String(),
				Fiat:    strings.ToUpper(fiat),
				Crypto:  string(token),
				RawRate: val.Float(),
			})
		}
	}

	if len(rows) == 0 {

		return errors.New("CoingeckoRate: no data")
	}

	Db.Create(&rows)

	// 写入后失效缓存，让下一次 GetOrderRate 读到最新值
	InvalidateRateCache()

	return nil
}

func ParseFloatRate(syntax string, rawVal float64) float64 {
	if syntax == "" {

		return rawVal
	}

	if utils.IsNumber(syntax) {

		return cast.ToFloat64(syntax)
	}

	match, err := regexp.MatchString(`^[~+-]\d+(\.\d+)?$`, syntax)
	if !match || err != nil {
		log.Error("浮动语法解析错误", err)

		return 0
	}

	var act = syntax[0:1]
	var raw = decimal.NewFromFloat(rawVal)
	var base = decimal.NewFromFloat(cast.ToFloat64(syntax[1:]))
	var result float64 = 0

	switch act {
	case "~":
		result = raw.Mul(base).InexactFloat64()
	case "+":
		result = raw.Add(base).InexactFloat64()
	case "-":
		result = raw.Sub(base).InexactFloat64()
	}

	return round(result, 4) // 保留4位小数
}

func round(val float64, precision int) float64 {
	// Round 四舍五入，ROUND_HALF_UP 模式实现
	// 返回将 val 根据指定精度 precision（十进制小数点后数字的数目）进行四舍五入的结果。precision 也可以是负数或零。

	if precision == 0 {
		return math.Round(val)
	}

	p := math.Pow10(precision)
	if precision < 0 {
		return math.Floor(val*p+0.5) * math.Pow10(-precision)
	}

	return math.Floor(val*p+0.5) / p
}

// getLatestRate 返回 (crypto, fiat) 最新一行 Rate；命中缓存零 IO，未命中走 DB 并填充
func getLatestRate(token Crypto, fiat Fiat) (Rate, error) {
	key := string(token) + ":" + string(fiat)
	if v, ok := rateCache.Load(key); ok {
		entry := v.(rateCacheEntry)
		if time.Since(entry.at) < rateCacheTTL {

			return entry.rate, nil
		}
	}

	var r Rate
	Db.Where("crypto = ? and fiat = ?", token, fiat).Order("created_at desc").Limit(1).Find(&r)
	if r.ID == 0 {

		return r, fmt.Errorf("创建失败，请检查汇率同步是否正常：%s %s", token, fiat)
	}

	rateCache.Store(key, rateCacheEntry{rate: r, at: time.Now()})

	return r, nil
}

// InvalidateRateCache 在 Rate 写入后调用，避免读到过期缓存；批量同步后调一次即可
func InvalidateRateCache() {
	rateCache.Range(func(k, _ any) bool {
		rateCache.Delete(k)

		return true
	})
}

func GetOrderRate(token Crypto, fiat Fiat, syntax string) (decimal.Decimal, error) {
	r, err := getLatestRate(token, fiat)
	if err != nil {

		return decimal.Decimal{}, err
	}

	if syntax == "" {

		return decimal.NewFromString(r.Rate)
	}

	return decimal.NewFromFloat(ParseFloatRate(syntax, r.RawRate)), nil
}
