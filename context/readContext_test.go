package context

import (
	"bytes"
	"fmt"
	"testing"
)

func TestReadMgr(t *testing.T) {
	t.Run("test read content mgr", func(t *testing.T) {
		mgr := ReadContextMgr{
			Root: "/root/workspace/llm_dev",
		}
		mgr.readContent("context.log", 1, "after")
		var buf bytes.Buffer
		mgr.WriteContext(&buf)
		fmt.Printf("%s\n", buf.String())
	})
}
