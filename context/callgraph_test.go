package context

import (
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/database"
	"testing"
)

func TestFindReference(t *testing.T) {
	t.Run("test find reference and used def", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		root := "/root/workspace/llm_dev"
		mgr := CallGraphContextMgr{
			rootPath: root,
			buildCtxOps: &impl.BuildCodeBaseCtxOps{
				RootPath: root,
				Db:       database.GetDBClient().Database("llm_dev"),
			},
		}
		res, err := mgr.findReference("codebase/impl/go_impl_db.go", "extractUsedTypeInfo", 486)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			return
		}
		output := mgr.genReferenceOutput(res)
		fmt.Printf("%v\n", output)

	})
}

func TestFindUsedDef(t *testing.T) {
	t.Run("test find reference and used def", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		root := "/root/workspace/llm_dev"
		mgr := CallGraphContextMgr{
			rootPath: root,
			buildCtxOps: &impl.BuildCodeBaseCtxOps{
				RootPath: root,
				Db:       database.GetDBClient().Database("llm_dev"),
			},
		}
		res, err := mgr.findUsedDefs("codebase/impl/go_impl_db.go", "extractUsedTypeInfo", 486)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			return
		}
		output := mgr.genUseOutput(res)
		fmt.Printf("%v\n", output)

	})
}
