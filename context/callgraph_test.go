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
		// mgr.buildCtxOps.GenAllDefs()
		// mgr.buildCtxOps.GenAllUsedDefs()
		// mgr.buildCtxOps.SetMinPreFix()
		res, err := mgr.findUsedDefs("codebase/impl/go_impl_db.go", "extractUsedTypeInfo", 489)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			return
		}
		// for _, elem := range res {
		// 	fmt.Printf("%s %s %v\n", elem.DefFile, elem.DefIdentifier, elem.DefKeyword)
		// }
		output := mgr.genUseOutput(res)
		fmt.Printf("%v\n", output)

	})
}
