package task

import (
	"context"
	"time"

	"github.com/v03413/bepusdt/app/model"
)

type rConf struct {
}

// init 配置刷新到内存，高频读取。
// SetK 写入路径自带 defer RefreshC()，进程内变更会立即反映；这里的周期刷新仅为兜底"绕过 SetK 的直写"
// （如手动 SQL 改 bep_conf）。3s 间隔过于激进，30s 足够，能减一个数量级的全表 SELECT
func init() {
	var c = rConf{}

	Register(Task{Duration: time.Second * 30, Callback: c.Refresh})
}

func (rConf) Refresh(ctx context.Context) {
	model.RefreshC()
}
