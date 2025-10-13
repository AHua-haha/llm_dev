package impl

import (
	"testing"
)

func Test_fileSymExtractOps_extractSymbol(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		fileops fileSymExtractOps
	}{
		// TODO: Add test cases.
		{
			name: "test extract operation",
			fileops: fileSymExtractOps{
				path: "go_impl.go",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: construct the receiver type.
			tt.fileops.extractSymbol()
		})
	}
}
