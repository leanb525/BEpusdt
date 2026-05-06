package model

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/v03413/bepusdt/app/conf"
)

// endpointRR 每个网络对应一个 round-robin 计数器，用于多端点轮询
var endpointRR sync.Map // key: ConfKey, value: *uint64

// Endpoints 解析配置项中的 RPC 端点列表，支持按换行分隔多个端点；空值返回 nil
func Endpoints(net Network) []string {
	key, ok := networkEndpointMap[net]
	if !ok {
		return nil
	}

	raw := GetC(key)
	if raw == "" {
		raw = GetK(key)
	}

	return parseEndpoints(raw)
}

func parseEndpoints(raw string) []string {
	if raw == "" {
		return nil
	}

	out := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}

	return out
}

// PickEndpoint 在配置的多个 RPC 端点中按 round-robin 选择一个；
// 处于冷却中的端点会被跳过；若全部冷却则降级返回轮询位次的下一个，避免无端点可用
func PickEndpoint(net Network) string {
	list := Endpoints(net)
	if len(list) == 0 {
		return ""
	}
	if len(list) == 1 {
		return list[0]
	}

	key := networkEndpointMap[net]
	counterAny, _ := endpointRR.LoadOrStore(key, new(uint64))
	counter := counterAny.(*uint64)

	n := len(list)
	for i := 0; i < n; i++ {
		idx := int((atomic.AddUint64(counter, 1) - 1) % uint64(n))
		candidate := list[idx]
		if !conf.IsEndpointBad(candidate) {
			return candidate
		}
	}

	idx := int((atomic.AddUint64(counter, 1) - 1) % uint64(n))
	return list[idx]
}
