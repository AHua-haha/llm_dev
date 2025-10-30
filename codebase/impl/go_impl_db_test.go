package impl

import (
	"fmt"
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
