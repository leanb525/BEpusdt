package task

import (
	"context"
	"fmt"
	"time"

	"github.com/v03413/bepusdt/app/log"
	"github.com/v03413/bepusdt/app/model"
	"github.com/v03413/bepusdt/app/notifier"
	"github.com/v03413/bepusdt/app/task/notify"
	"github.com/v03413/bepusdt/app/utils"
	"github.com/v03413/go-cache"
)

const notifyRetryInflightTTL = 15 * time.Second // 单订单 retry 进行中标记 TTL；防止 3s ticker 多次 spawn 同一订单 goroutine 引发 race

func init() {
	Register(Task{Duration: time.Second * 3, Callback: notifyRetry})
	Register(Task{Duration: time.Second * 30, Callback: notifyRoll})
}

// notifyRetry 回调失败重试
func notifyRetry(context.Context) {
	tradeOrders, err := model.GetNotifyFailedTradeOrders()
	if err != nil {
		log.Task.Error("待回调订单获取失败", err)

		return
	}

	for _, order := range tradeOrders {
		next := utils.CalcNextNotifyTime(*order.ConfirmedAt, order.NotifyNum)
		if time.Now().Unix() < next.Unix() {
			continue
		}

		// in-flight 标记防 ticker 重叠时重复 spawn；HTTP 超时 10s，TTL 15s 留余量
		key := fmt.Sprintf("notify_retry_inflight_%s", order.TradeId)
		if _, ok := cache.Get(key); ok {
			continue
		}
		cache.Set(key, true, notifyRetryInflightTTL)

		go func(o model.Order) {
			defer cache.Delete(key)
			_ = notify.Handle(o)
		}(order)
	}
}

func notifyRoll(context.Context) {
	for _, o := range model.GetOrderByStatus(model.OrderStatusWaiting) {
		notify.Bepusdt(o)
	}
}

// notifyOrderSuccess 统一触发订单成功后的回调与订单通知。
func notifyOrderSuccess(order model.Order) {
	go notify.Handle(order)
	go notifier.Success(order)
}
