package impl

import (
	"fmt"
	"llm_dev/codebase/common"
	"llm_dev/database"
	"testing"
)

func TestBuildCodeBaseCtxOps_findDefs(t *testing.T) {
	strPtr := func(s string) *string {
		return &s
	}
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		relfile    *string
		identifier *string
		keyword    []string
		want       []Definition
	}{
		// TODO: Add test cases.
		// {
		// 	name:       "find by relfile",
		// 	relfile:    strPtr("main.go"),
		// 	identifier: nil,
		// },
		// {
		// 	name:       "find by keyword",
		// 	relfile:    nil,
		// 	identifier: nil,
		// 	keyword:    []string{"BuildCodeBaseCtxOps"},
		// },
		{
			name:       "find by identifier",
			relfile:    nil,
			identifier: strPtr("BuildCodeBaseCtxOps"),
		},
	}
	database.InitDB()
	defer database.CloseDB()
	client := database.GetDBClient()
	var op BuildCodeBaseCtxOps
	op.Db = client.Database("llm_dev")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: construct the receiver type.
			filter := GenDefFilter(tt.relfile, tt.identifier, tt.keyword)
			got := op.FindDefs(filter)
			fmt.Printf("len(got): %v\n", len(got))
			for _, def := range got {
				fmt.Printf("def: %v\n", def)
			}
		})
	}
}

func TestBuildCtx(t *testing.T) {

	t.Run("test build ctx op", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		op := BuildCodeBaseCtxOps{
			RootPath: "/root/workspace/llm_dev",
			Db:       database.GetDBClient().Database("llm_dev"),
		}
		file := op.WalkProjectFileTree()
		for f := range file {
			fmt.Printf("%s\n", f.Path)
		}
	})
}

func TestGenAllDefs(t *testing.T) {
	t.Run("test gen all Defs", func(t *testing.T) {
	})
}

func TestTypeCtxHandler(t *testing.T) {
	t.Run("test type info ctx handler", func(t *testing.T) {
		root := "/root/workspace/llm_dev"
		op := BuildCodeBaseCtxOps{
			RootPath: root,
		}
		ctx := common.WalkGoProjectTypeAst(root, op.typeCtxHandler)
		for res := range ctx.OutputChan {
			usedDefs := common.GetMapas[[]UsedDef](res, "used Defs")
			fmt.Printf("len(usedDefs): %v\n", len(usedDefs))
			if len(usedDefs) == 0 {
				continue
			}
			loc := usedDefs[0]
			fmt.Printf("%s %s %v uses:\n", loc.File, loc.Identifier, loc.Keyword)
			for _, used := range usedDefs {
				if used.Isdependency {
					fmt.Printf("  %s %s %v %s\n", used.DefFile, used.DefIdentifier, used.DefKeyword, used.PkgPath)
				} else {
					fmt.Printf("  %s %s %v\n", used.DefFile, used.DefIdentifier, used.DefKeyword)
				}
			}
		}
		fmt.Println()
	})
}

func TestAstCtxHandler(t *testing.T) {
	t.Run("test type info ctx handler", func(t *testing.T) {
		file := "/root/workspace/llm_dev/codebase/impl/go_impl_db_test.go"
		op := BuildCodeBaseCtxOps{
			RootPath: "/root/workspace/llm_dev",
		}
		ctx := common.WalkFileStaticAst(file, op.astCtxHandler)
		for res := range ctx.OutputChan {
			defs := common.GetMapas[[]Definition](res, "defs")
			for _, def := range defs {
				fmt.Printf("%s %s %v\n", def.RelFile, def.Identifier, def.Keyword)
			}
		}
		fmt.Println()
	})
}

func TestFileTreeHanler(t *testing.T) {
	t.Run("test type info ctx handler", func(t *testing.T) {
		op := BuildCodeBaseCtxOps{
			RootPath: "/root/workspace/llm_dev",
		}
		ctx := common.WalkFileTree("/root/workspace/llm_dev", op.fileTreeCtxHandler())
		for res := range ctx.OutputChan {
			relPath := common.GetMapas[string](res, "relPath")
			fmt.Printf("relPath: %v\n", relPath)
		}
	})
}

func TestGenDefs(t *testing.T) {
	t.Run("test type info ctx handler", func(t *testing.T) {
		op := BuildCodeBaseCtxOps{
			RootPath: "/root/workspace/llm_dev",
		}
		op.GenAllDefs()
	})
}

func TestSetMinPrefix(t *testing.T) {
	t.Run("test set min prefix", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		op := BuildCodeBaseCtxOps{
			RootPath: "/root/workspace/llm_dev",
			Db:       database.GetDBClient().Database("llm_dev"),
		}
		// op.GenAllDefs()
		// op.GenAllUsedDefs()
		op.SetMinPreFix()
	})
}

func TestFileOutline(t *testing.T) {
	t.Run("test set min prefix", func(t *testing.T) {
		database.InitDB()
		defer database.CloseDB()
		op := BuildCodeBaseCtxOps{
			RootPath: "/root/workspace/llm_dev",
			Db:       database.GetDBClient().Database("llm_dev"),
		}
		// op.GenAllDefs()
		// op.GenAllUsedDefs()
		// op.SetMinPreFix()
		defs := op.FindUsedDefOutline("")
		for _, def := range defs {
			fmt.Printf("%s %s %s\n", def.RelFile, def.Identifier, def.MinPrefix)
		}
	})
}
