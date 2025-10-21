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
		{
			name:       "find by relfile",
			relfile:    strPtr("main.go"),
			identifier: nil,
		},
	}
	database.InitDB()
	defer database.CloseDB()
	client := database.GetDBClient()
	var op BuildCodeBaseCtxOps
	op.db = client.Database("llm_dev")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: construct the receiver type.
			got := op.findDefs(tt.relfile, tt.identifier, tt.keyword)
			fmt.Printf("len(got): %v\n", len(got))
			for _, def := range got {
				fmt.Printf("def: %v\n", def)
			}
		})
	}
}
