package context

import (
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/database"
	"testing"
)

func TestHistoryMgr(t *testing.T) {
	t.Run("test reveal references", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		mgr := HistoryContextMgr{
			BuildOps: &impl.BuildCodeBaseCtxOps{
				RootPath: "/root/workspace/llm_dev",
				Db:       database.GetDBClient().Database("llm_dev"),
			},
		}
		refs := []string{
			"main.go:33",
			"agent/baseAgent.go:243",
			"agent/baseAgent.go:244-252",
			"agent/baseAgent.go:253-270",
		}
		refPtr := []*string{}
		for i := range refs {
			refPtr = append(refPtr, &refs[i])
		}

		mgr.revealRefs(refPtr)
		for _, ref := range refs {
			fmt.Printf("%s\n", ref)
		}
	})
}
