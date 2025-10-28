package context

import (
	"bytes"
	"fmt"
	"llm_dev/database"
	"testing"
)

func TestFileContentCtxMgr_WriteExternalDefs(t *testing.T) {
	t.Run("test file context mgr", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		mgr := NewFileCtxMgr("/root/workspace/llm_dev")
		// mgr.buildCodeBaseCtxop.ExtractDefs()
		var buf bytes.Buffer
		// mgr.WriteUsedDefs(&buf)
		mgr.WriteFileTree(&buf)
		fmt.Print(buf.String())
	})
}
