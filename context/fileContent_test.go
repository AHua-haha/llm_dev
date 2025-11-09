package context

import (
	"llm_dev/database"
	"testing"
)

func TestFileContentCtxMgr_WriteExternalDefs(t *testing.T) {
	t.Run("test file context mgr", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		// buildOp := impl.BuildCodeBaseCtxOps{
		// 	RootPath: "/root/workspace/llm_dev",
		// 	Db:       database.GetDBClient().Database("llm_dev"),
		// }
	})
}
