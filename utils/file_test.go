package utils

import (
	"bytes"
	"fmt"
	"testing"
)

func TestFileContent_WriteContent(t *testing.T) {
	t.Run("test gen file content", func(t *testing.T) {
		fc := FileContent{}
		var buf bytes.Buffer
		fc.WriteContent(&buf, "/root/workspace/llm_dev/context/fileContent.go")
		fmt.Print(buf.String())
	})
}
