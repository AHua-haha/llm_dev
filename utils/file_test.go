package utils

import (
	"bytes"
	"fmt"
	"testing"
)

func TestFileContent_WriteContent(t *testing.T) {
	t.Run("test gen file content", func(t *testing.T) {
		fc := FileContent{}
		fc.AddChunk(0, 13)
		fc.AddChunk(4, 55)
		fc.AddChunk(47, 156)
		fc.AddChunk(189, 210)
		var buf bytes.Buffer
		fc.WriteContent(&buf, "/root/workspace/llm_dev/context/fileContent.go")
		fmt.Print(buf.String())
	})
}
