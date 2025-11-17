package context

import (
	"bytes"
	"fmt"
	"testing"
)

func TestTaskMgr(t *testing.T) {
	t.Run("task task management", func(t *testing.T) {
		var mgr TaskContextMgr
		mgr.createTask("abbc")
		mgr.finishTask(1)
		mgr.createTask("dddb")
		var buf bytes.Buffer
		mgr.WriteContext(&buf)
		fmt.Printf("%s\n", buf.String())
	})
}
