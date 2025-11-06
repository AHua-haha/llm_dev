package context

import (
	"bytes"
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/database"
	"testing"
)

func TestOutlineContextMgr_writeLeafNode(t *testing.T) {
	t.Run("test write outline for lead", func(t *testing.T) {
		// TODO: construct the receiver type.
		database.InitDB()
		defer database.CloseDB()
		mgr := OutlineContextMgr{
			rootPath: "/root/workspace/llm_dev",
			buildCtxOp: impl.BuildCodeBaseCtxOps{
				RootPath: "/root/workspace/llm_dev",
				Db:       database.GetDBClient().Database("llm_dev"),
			},
		}
		var buf bytes.Buffer
		mgr.writeLeafNode(&buf, "context")
		fmt.Printf("%s\n", buf.String())
	})
}
