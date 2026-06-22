package ids

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/FernasFragas/nandocodego/internal/types"
)

var ctr uint32

func New(kind types.TaskKind) string {
	ts := uint32(time.Now().UnixMilli() & 0xffffffff)
	c := atomic.AddUint32(&ctr, 1) & 0xffff
	return fmt.Sprintf("%s-%08x%04x", string(kind), ts, c)
}
