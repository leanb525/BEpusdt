package task

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"
	"github.com/shopspring/decimal"
	"github.com/smallnest/chanx"
	"github.com/spf13/cast"
	"github.com/tidwall/gjson"
	"github.com/v03413/bepusdt/app/conf"
	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/utils"
)

const (
	blockParseMaxNum     = 10 // 每次解析区块的默认最大数量；可通过 EvmBlockParseMaxNum 覆盖
	blockDispatchPoolNum = 3  // 区块消费 worker 默认数量；可通过 EvmBlockDispatchPool 覆盖
	evmTransferEvent     = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	evmEndpointCooldown  = 30 * time.Second // 端点限流/网络错误后的冷却时长
	evmEndpointBadCfgCD  = 10 * time.Minute // 配置不匹配类错误（如 chain id 不对、方法不支持）的端点冷却时长，长时间踢出轮询
	evmBlockMaxAttempts  = 5                // 单个区块范围最大重试次数，超过则丢弃，避免坏 range 无限循环把成功率指标拖花
	evmRpcTimeout        = 20 * time.Second // 单次扫块 RPC 总超时；BSC 公共节点 Parse=true 时 3-8s 常态，留出足够余量
)

// errRateLimited 标识 RPC 限流类错误（-32005 / "limit exceeded" / HTTP 429 等），上层据此做端点冷却 + 区间二分
var errRateLimited = errors.New("rpc rate limited")

// errBadEndpoint 标识端点本身就不该被使用（链 id 不对、方法不支持等），上层据此做长冷却，把端点踢出轮询
var errBadEndpoint = errors.New("rpc endpoint mismatch")

var chainBlockNum sync.Map

// isBadChainErr 判断响应是否属于"端点根本不支持当前链"或"方法/参数不支持"类错误；
// 这些错误重试同一端点没有意义，应当将端点踢出轮询较长时间
func isBadChainErr(errMsg string) bool {
	if errMsg == "" {
		return false
	}
	low := strings.ToLower(errMsg)
	if strings.Contains(low, "invalid chain") {
		return true
	}
	if strings.Contains(low, "unsupported chain") {
		return true
	}
	if strings.Contains(low, "wrong chain") {
		return true
	}
	if strings.Contains(low, "chain not supported") {
		return true
	}
	if strings.Contains(low, "method not found") {
		return true
	}
	if strings.Contains(low, "method not supported") {
		return true
	}
	if strings.Contains(low, "-32601") { // jsonrpc method not found
		return true
	}
	if strings.Contains(low, "-32001") { // 部分实现用 -32001 表示 invalid chain
		return true
	}
	return false
}

// isRateLimitErr 判断 jsonrpc error 字符串或 HTTP 状态码是否属于限流类
func isRateLimitErr(httpStatus int, errMsg string) bool {
	if httpStatus == http.StatusTooManyRequests {
		return true
	}
	if errMsg == "" {
		return false
	}
	low := strings.ToLower(errMsg)
	if strings.Contains(low, "-32005") {
		return true
	}
	if strings.Contains(low, "limit exceeded") {
		return true
	}
	if strings.Contains(low, "rate limit") {
		return true
	}
	if strings.Contains(low, "too many requests") {
		return true
	}
	if strings.Contains(low, "request limit") {
		return true
	}
	return false
}

type block struct {
	RollDelayOffset int64 // 延迟偏移量，某些RPC节点如果不延迟，会报错 block is out of range，目前发现 https://rpc.xlayer.tech/ 存在此问题
	ConfirmedOffset int   // 确认偏移量，开启交易确认后，区块高度需要减去此值认为交易已确认
}

type evmNative struct {
	Parse     bool
	Decimal   int32
	TradeType model.TradeType
}

type evm struct {
	Network         string
	Block           block
	Native          evmNative
	Client          *http.Client
	AvgBlockTime    int64          // 平均出块时间，单位秒；一个大概值，用于计算首次启动时需要回溯的区块数量，尽量准确设置，默认1秒一个区块
	BatchSizeKey    model.ConfKey  // 可选：覆盖 blockParseMaxNum 的配置键；为空时使用硬编码默认值
	DispatchPoolKey model.ConfKey  // 可选：覆盖 blockDispatchPoolNum 的配置键；为空时使用硬编码默认值
	blockScanQueue  *chanx.UnboundedChan[evmBlock]
}

// batchSize 返回当前链每次扫块覆盖的最大区块数量；优先读 BatchSizeKey，未设置或非正数时回退默认
func (e *evm) batchSize() int64 {
	if e.BatchSizeKey != "" {
		if v := cast.ToInt64(model.GetC(e.BatchSizeKey)); v > 0 {
			return v
		}
	}
	return blockParseMaxNum
}

// dispatchPool 返回当前链区块消费 worker 数量；优先读 DispatchPoolKey，未设置或非正数时回退默认
func (e *evm) dispatchPool() int {
	if e.DispatchPoolKey != "" {
		if v := cast.ToInt(model.GetC(e.DispatchPoolKey)); v > 0 {
			return v
		}
	}
	return blockDispatchPoolNum
}

type evmBlock struct {
	From     int64
	To       int64
	Attempts int // 已重试次数；每次 requeue 自增，达到 evmBlockMaxAttempts 后丢弃
}

// requeue 把扫块任务重新入队列；超过最大重试次数则丢弃并告警，防止坏 range 无限循环
func (e *evm) requeue(b evmBlock) {
	b.Attempts++
	if b.Attempts >= evmBlockMaxAttempts {
		log.Task.Error(fmt.Sprintf("%s 区块扫描重试超过 %d 次已放弃：%d → %d", e.Network, evmBlockMaxAttempts, b.From, b.To))

		return
	}

	e.blockScanQueue.In <- b
}

func (e *evm) syncBlocksForward(ctx context.Context) {
	if syncBreak(e.Network, e.blockScanQueue.Len()) {

		return
	}

	endpoint := e.rpcEndpoint()
	post := []byte(`{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(post))
	if err != nil {
		log.Task.Warn("Error creating request:", err)

		return
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := e.Client.Do(req)
	if err != nil {
		conf.MarkEndpointBad(endpoint, evmEndpointCooldown)
		log.Task.Warn("Error sending request:", err)

		return
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		conf.MarkEndpointBad(endpoint, evmEndpointCooldown)
		log.Task.Warn("Error reading response body:", err)

		return
	}

	// 端点返回非 2xx（如 Cloudflare 521/502 这类源站不可用）或响应不是 JSON 对象（节点返回 HTML 错误页），都冷却端点，避免下轮再被命中
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		conf.MarkEndpointBad(endpoint, evmEndpointCooldown)
		log.Task.Warn(fmt.Sprintf("EVM 数据解析错误(%s): endpoint=%s status=%d body=%s", e.Network, endpoint, resp.StatusCode, string(body)))

		return
	}

	var res = gjson.ParseBytes(body)
	if !res.IsObject() {
		conf.MarkEndpointBad(endpoint, evmEndpointCooldown)
		log.Task.Warn(fmt.Sprintf("EVM 数据解析错误(%s): endpoint=%s body=%s", e.Network, endpoint, string(body)))

		return
	}

	if res.Get("error").Exists() {
		errStr := res.Get("error").String()
		if isBadChainErr(errStr) {
			conf.MarkEndpointBad(endpoint, evmEndpointBadCfgCD)
		} else {
			conf.MarkEndpointBad(endpoint, evmEndpointCooldown)
		}
		log.Task.Warn(fmt.Sprintf("%s eth_blockNumber response error endpoint=%s err=%s", e.Network, endpoint, errStr))

		return
	}

	var now = utils.HexStr2Int(res.Get("result").String()).Int64() - e.Block.RollDelayOffset
	if now <= 0 {

		return
	}

	var lastBlockNumber int64
	if v, ok := chainBlockNum.Load(e.Network); ok {

		lastBlockNumber = v.(int64)
	} else {
		e.syncBlocksBackward(now) // 不存在，说明是第一次启动
	}

	if now-lastBlockNumber > cast.ToInt64(model.GetC(model.BlockHeightMaxDiff)) {

		lastBlockNumber = now - 1
	}

	chainBlockNum.Store(e.Network, now)
	if now <= lastBlockNumber {

		return
	}

	step := e.batchSize()
	for from := lastBlockNumber + 1; from <= now; from += step {
		to := from + step - 1
		if to > now {
			to = now
		}

		e.blockScanQueue.In <- evmBlock{From: from, To: to}
	}
}

func (e *evm) syncBlocksBackward(now int64) {
	if e.AvgBlockTime <= 0 { // 未设置平均出块时间，默认1秒一个
		e.AvgBlockTime = 1
	}

	var o model.Order
	trade := model.GetNetworkTrades(model.Network(e.Network))
	model.Db.Model(&model.Order{}).Where("status = ? and trade_type in (?)", model.OrderStatusWaiting, trade).Order("created_at asc").Limit(1).Find(&o)
	if o.ID == 0 {

		return
	}

	sub := ((time.Now().Unix() - o.CreatedAt.Time().Unix()) / e.AvgBlockTime) + 30 //计算需要回溯的区块数量，同时冗余30个区块
	start := now - sub

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		step := e.batchSize()
		for from := start; from <= now; from += step {
			if syncBreak(e.Network, e.blockScanQueue.Len()) {

				return
			}

			to := from + step - 1
			if to > now {
				to = now
			}

			e.blockScanQueue.In <- evmBlock{From: from, To: to}

			<-ticker.C
		}
	}()
}

func (e *evm) blockDispatch(ctx context.Context) {
	p, err := ants.NewPoolWithFunc(e.dispatchPool(), e.getBlockByNumber)
	if err != nil {
		log.Task.Warn("Error creating pool:", err)

		return
	}

	defer p.Release()

	for {
		select {
		case <-ctx.Done():
			return
		case n := <-e.blockScanQueue.Out:
			if err := p.Invoke(n); err != nil {
				e.requeue(n)

				log.Task.Warn("Evm Block Dispatch Error invoking process block:", err)
			}
		}
	}
}

func (e *evm) getBlockByNumber(a any) {
	b, ok := a.(evmBlock)
	if !ok {
		log.Task.Warn("Evm Block Parse Error: expected evmBlock, got", a)

		return
	}

	// 单出口指标记录：函数结束时统一记一次成功或失败，避免一次调用同时写入 success + failure 把成功率锚定在 50%
	success := false
	defer func() {
		if success {
			conf.RecordSuccess(e.Network)
		} else {
			conf.RecordFailure(e.Network)
		}
	}()

	endpoint := model.PickEndpoint(model.Network(e.Network))
	if endpoint == "" {
		log.Task.Warn(fmt.Sprintf("%s 未配置 RPC 端点", e.Network))

		return
	}

	items := make([]string, 0)
	for i := b.From; i <= b.To; i++ {
		items = append(items, fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x%x",%t],"id":%d}`, i, e.Native.Parse, i))
	}

	ctx, cancel := context.WithTimeout(context.Background(), evmRpcTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer([]byte(fmt.Sprintf(`[%s]`, strings.Join(items, ",")))))
	if err != nil {
		log.Task.Warn("Error creating request:", err)

		return
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := e.Client.Do(req)
	if err != nil {
		conf.MarkEndpointBad(endpoint, evmEndpointCooldown)
		e.requeue(b)
		log.Task.Warn("eth_getBlockByNumber Error sending request:", err)

		return
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		conf.MarkEndpointBad(endpoint, evmEndpointCooldown)
		e.requeue(b)
		log.Task.Warn("eth_getBlockByNumber Error reading response body:", err)

		return
	}

	if isRateLimitErr(resp.StatusCode, string(body)) {
		conf.MarkEndpointBad(endpoint, evmEndpointCooldown)
		e.requeueOnRateLimit(b)
		log.Task.Warn(fmt.Sprintf("%s eth_getBlockByNumber rate limited, endpoint=%s status=%d", e.Network, endpoint, resp.StatusCode))

		return
	}

	nativeTransfers := make([]transfer, 0)
	blockTimestamp := make(map[string]time.Time)
	for _, itm := range gjson.ParseBytes(body).Array() {
		if itm.Get("error").Exists() {
			errStr := itm.Get("error").String()

			if isRateLimitErr(0, errStr) {
				conf.MarkEndpointBad(endpoint, evmEndpointCooldown)
				e.requeueOnRateLimit(b)
				log.Task.Warn(fmt.Sprintf("%s eth_getBlockByNumber rate limited, endpoint=%s err=%s", e.Network, endpoint, errStr))

				return
			}

			if isBadChainErr(errStr) {
				conf.MarkEndpointBad(endpoint, evmEndpointBadCfgCD)
				e.requeue(b)
				log.Task.Warn(fmt.Sprintf("%s eth_getBlockByNumber bad endpoint, endpoint=%s err=%s", e.Network, endpoint, errStr))

				return
			}

			e.requeue(b)
			log.Task.Warn(fmt.Sprintf("%s eth_getBlockByNumber response error %s", e.Network, errStr))

			return
		}

		timestamp := utils.HexStr2Int(itm.Get("result.timestamp").String()).Int64()
		blockTime := time.Unix(timestamp, 0)
		blockNumHex := itm.Get("result.number").String()
		blockTimestamp[blockNumHex] = blockTime

		var array = itm.Get("result.transactions").Array()
		if e.Native.Parse && len(array) != 0 {

			nativeTransfers = append(nativeTransfers, e.parseNativeTransfer(array, int(utils.HexStr2Int(blockNumHex).Int64()), blockTime)...)
		}
	}

	transfers, err := e.parseEventTransfer(ctx, endpoint, b, blockTimestamp)
	if err != nil {
		if errors.Is(err, errRateLimited) {
			conf.MarkEndpointBad(endpoint, evmEndpointCooldown)
			e.requeueOnRateLimit(b)
			log.Task.Warn(fmt.Sprintf("%s eth_getLogs rate limited, endpoint=%s err=%s", e.Network, endpoint, err))

			return
		}

		if errors.Is(err, errBadEndpoint) {
			conf.MarkEndpointBad(endpoint, evmEndpointBadCfgCD)
			e.requeue(b)
			log.Task.Warn(fmt.Sprintf("%s eth_getLogs bad endpoint, endpoint=%s err=%s", e.Network, endpoint, err))

			return
		}

		// 兜底：parseEventTransfer 的其他错误基本都是网络层（Do/ReadAll 超时或断开），冷却端点避免下一轮被同一个端点拖累
		conf.MarkEndpointBad(endpoint, evmEndpointCooldown)
		e.requeue(b)
		log.Task.Warn(fmt.Sprintf("Evm Block Parse Error parsing block transfer endpoint=%s err=%s", endpoint, err))

		return
	}

	if len(nativeTransfers) > 0 {
		transferQueue.In <- nativeTransfers
	}
	if len(transfers) > 0 {
		transferQueue.In <- transfers
	}

	success = true
	log.Task.Info(fmt.Sprintf("区块扫描完成(%s): %d → %d 成功率：%s", e.Network, b.From, b.To, conf.GetSuccessRate(e.Network)))
}

// requeueOnRateLimit 命中限流时把当前区块范围二分后重新入队，避免相同范围的请求立刻打回同样限流的端点；
// 单块限流时退避 1 秒后整段重入，等待端点冷却或下次切换到其他端点；
// 子段继承原 Attempts 计数，由 requeue 自增并触发上限丢弃，防止无限循环
func (e *evm) requeueOnRateLimit(b evmBlock) {
	if b.From >= b.To {
		time.Sleep(time.Second)
		e.requeue(b)

		return
	}

	mid := b.From + (b.To-b.From)/2
	e.requeue(evmBlock{From: b.From, To: mid, Attempts: b.Attempts})
	e.requeue(evmBlock{From: mid + 1, To: b.To, Attempts: b.Attempts})
}

func (e *evm) parseNativeTransfer(array []gjson.Result, num int, timestamp time.Time) []transfer {
	nativeTransfers := make([]transfer, 0)
	for _, tx := range array {
		if tx.Get("input").String() != "0x" {
			// 非原生币交易

			continue
		}

		valStr := tx.Get("value").String()
		if valStr == "0x0" || len(valStr) < 3 {
			// 过滤 0 值交易

			continue
		}

		amount, ok := big.NewInt(0).SetString(valStr[2:], 16)
		if !ok || amount.Sign() <= 0 {

			continue
		}

		toAddress := tx.Get("to").String()
		if toAddress == "" { // 合约创建交易 to 为空

			continue
		}

		nativeTransfers = append(nativeTransfers, transfer{
			Network:     e.Network,
			FromAddress: tx.Get("from").String(),
			RecvAddress: toAddress,
			Amount:      decimal.NewFromBigInt(amount, e.Native.Decimal),
			TxHash:      tx.Get("hash").String(),
			BlockNum:    num,
			Timestamp:   timestamp,
			TradeType:   e.Native.TradeType,
		})
	}

	return nativeTransfers
}

func (e *evm) parseEventTransfer(ctx context.Context, endpoint string, b evmBlock, timestamp map[string]time.Time) ([]transfer, error) {
	transfers := make([]transfer, 0)
	// 拼接 address 过滤；部分公共节点（如 allnodes.com）要求 eth_getLogs 必须带 address，否则会返回 -32701；
	// 当前链没有合约（纯 native 链）则不带，保持兼容
	addrFilter := ""
	if contracts := model.GetNetworkContracts(model.Network(e.Network)); len(contracts) > 0 {
		quoted := make([]string, 0, len(contracts))
		for _, c := range contracts {
			quoted = append(quoted, fmt.Sprintf("%q", c))
		}
		addrFilter = fmt.Sprintf(`,"address":[%s]`, strings.Join(quoted, ","))
	}
	post := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getLogs","params":[{"fromBlock":"0x%x","toBlock":"0x%x"%s,"topics":["%s"]}],"id":1}`, b.From, b.To, addrFilter, evmTransferEvent))
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(post))
	if err != nil {

		return transfers, errors.Join(errors.New("eth_getLogs NewRequest Error"), err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := e.Client.Do(req)
	if err != nil {

		return transfers, errors.Join(errors.New("eth_getLogs Do Error"), err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {

		return transfers, errors.Join(errors.New("eth_getLogs ReadAll Error"), err)
	}

	if isRateLimitErr(resp.StatusCode, string(body)) {

		return transfers, errors.Join(errRateLimited, fmt.Errorf("status=%d body=%s", resp.StatusCode, string(body)))
	}

	data := gjson.ParseBytes(body)
	if data.Get("error").Exists() {
		errStr := data.Get("error").String()
		if isRateLimitErr(0, errStr) {

			return transfers, errors.Join(errRateLimited, fmt.Errorf("%s eth_getLogs response error %s", e.Network, errStr))
		}

		if isBadChainErr(errStr) {

			return transfers, errors.Join(errBadEndpoint, fmt.Errorf("%s eth_getLogs response error %s", e.Network, errStr))
		}

		return transfers, fmt.Errorf("%s eth_getLogs response error %s", e.Network, errStr)
	}

	for _, itm := range data.Get("result").Array() {
		to := itm.Get("address").String()
		tradeType, ok := model.GetContractTrade(to)
		if !ok {

			continue
		}

		topics := itm.Get("topics").Array()
		if len(topics) < 3 {

			continue
		}

		if topics[0].String() != evmTransferEvent { // transfer event signature

			continue
		}

		from := fmt.Sprintf("0x%s", topics[1].String()[26:])
		recv := fmt.Sprintf("0x%s", topics[2].String()[26:])
		amount, ok := big.NewInt(0).SetString(itm.Get("data").String()[2:], 16)
		if !ok || amount.Sign() <= 0 {

			continue
		}

		transfers = append(transfers, transfer{
			Network:     e.Network,
			FromAddress: from,
			RecvAddress: recv,
			Amount:      decimal.NewFromBigInt(amount, model.GetContractDecimal(to)),
			TxHash:      itm.Get("transactionHash").String(),
			BlockNum:    cast.ToInt(itm.Get("blockNumber").String()),
			Timestamp:   timestamp[itm.Get("blockNumber").String()],
			TradeType:   tradeType,
		})
	}

	return transfers, nil
}

func (e *evm) tradeConfirmHandle(ctx context.Context) {
	var orders = getConfirmingOrders(model.GetNetworkTrades(model.Network(e.Network)))
	var wg sync.WaitGroup

	var handle = func(o model.Order) {
		if model.GetC(model.BlockOffsetConfirm) == "1" {
			last, ok := chainBlockNum.Load(e.Network)
			if !ok {
				return
			}
			if cast.ToInt(last)-o.RefBlockNum < e.Block.ConfirmedOffset {
				return
			}
		}

		post := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getTransactionReceipt","params":["%s"],"id":1}`, o.RefHash))
		req, err := http.NewRequestWithContext(ctx, "POST", e.rpcEndpoint(), bytes.NewBuffer(post))
		if err != nil {
			log.Task.Warn("evm tradeConfirmHandle Error creating request:", err)

			return
		}

		req.Header.Set("Content-Type", "application/json")
		resp, err := e.Client.Do(req)
		if err != nil {
			log.Task.Warn("evm tradeConfirmHandle Error sending request:", err)

			return
		}

		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Task.Warn("evm tradeConfirmHandle Error reading response body:", err)

			return
		}

		data := gjson.ParseBytes(body)
		if data.Get("error").Exists() {
			log.Task.Warn(fmt.Sprintf("%s eth_getTransactionReceipt response error %s", e.Network, data.Get("error").String()))

			return
		}

		if data.Get("result.status").String() == "0x1" {
			markFinalConfirmed(o)
		}
	}

	for _, order := range orders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handle(order)
		}()
	}

	wg.Wait()
}

func (e *evm) rpcEndpoint() string {

	return model.Endpoint(model.Network(e.Network))
}

func syncBreak(network string, num int) bool {
	if num >= blockQueueLimit {
		log.Task.Warn(fmt.Sprintf("%s 同步阻塞，当前区块消费堆积数量：%d", network, num))

		return true
	}

	if mqttSubscribed(network) {
		return false
	}

	trades := model.GetNetworkTrades(model.Network(network))
	if len(trades) == 0 {

		return true
	}

	var count int64
	model.Db.Model(&model.Wallet{}).
		Where("other_notify = ? and trade_type in (?)", model.WaOtherEnable, trades).
		Count(&count)
	if count > 0 {

		return false
	}

	model.Db.Model(&model.Order{}).
		Where("status = ? and trade_type in (?)", model.OrderStatusWaiting, trades).
		Count(&count)

	return count == 0
}
