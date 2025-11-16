package context

import (
	"bytes"
	"fmt"
	"testing"
)

func TestTaskMgr(t *testing.T) {
	t.Run("task task management", func(t *testing.T) {
		var mgr TaskContextMgr
		mgr.createTask("add log", false)
		mgr.createTask("a", true)
		mgr.finishTask("finish a", true)
		mgr.createTask("b", true)
		mgr.finishTask("finish b", true)
		mgr.createTask("c", true)
		mgr.finishTask("finish c", true)
		mgr.finishTask("finish log info", false)
		var buf bytes.Buffer
		mgr.WriteContext(&buf)
		fmt.Printf("%s\n", buf.String())
	})
}
