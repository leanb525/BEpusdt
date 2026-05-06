package conf

import (
	"fmt"
	"sync"
	"time"
)

const maxRecords = 1000

var endpointCooldown sync.Map // key: endpoint URL string, value: time.Time (冷却到期时间)

// MarkEndpointBad 将某个 RPC 端点临时标记为不可用，PickEndpoint 在 d 时间内会跳过它
func MarkEndpointBad(endpoint string, d time.Duration) {
	if endpoint == "" || d <= 0 {
		return
	}
	endpointCooldown.Store(endpoint, time.Now().Add(d))
}

// IsEndpointBad 判断某个 RPC 端点当前是否处于冷却中；过期会顺手清理
func IsEndpointBad(endpoint string) bool {
	v, ok := endpointCooldown.Load(endpoint)
	if !ok {
		return false
	}
	if time.Now().Before(v.(time.Time)) {
		return true
	}
	endpointCooldown.Delete(endpoint)
	return false
}

type stat struct {
	mu      sync.RWMutex
	records []bool
	index   int // 当前位置
	total   int // 已记录总数
	succ    int // 成功记录数
}

var (
	data sync.Map // map[string]*stat
)

func getStat(net string) *stat {
	val, _ := data.LoadOrStore(net, &stat{
		records: make([]bool, maxRecords),
	})
	return val.(*stat)
}

func RecordSuccess(net string) {
	s := getStat(net)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.total >= maxRecords && !s.records[s.index] {
		s.succ++
	} else if s.total < maxRecords {
		s.succ++
	}

	s.records[s.index] = true
	s.index = (s.index + 1) % maxRecords
	if s.total < maxRecords {
		s.total++
	}
}

func RecordFailure(net string) {
	s := getStat(net)
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.total >= maxRecords && s.records[s.index] {
		s.succ--
	}

	s.records[s.index] = false
	s.index = (s.index + 1) % maxRecords
	if s.total < maxRecords {
		s.total++
	}
}

func GetSuccessRate(net string) string {
	s := getStat(net)
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.total == 0 {
		return "100.00%"
	}

	return fmt.Sprintf("%.2f%%", float64(s.succ)/float64(s.total)*100)
}
