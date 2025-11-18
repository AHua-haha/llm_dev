package context

import (
	"bytes"
	"fmt"
	"testing"
)

func TestTaskMgr(t *testing.T) {
	t.Run("task task management", func(t *testing.T) {
		mgr := NewTaskCtxMgr("test this")
		mgr.createTask("abbc", "")
		mgr.finishTask("1", Failed)
		mgr.createTask("dddb", "")
		mgr.createTask("dddb", "2")
		var buf bytes.Buffer
		mgr.WriteContext(&buf)
		fmt.Printf("%s\n", buf.String())
	})
}
