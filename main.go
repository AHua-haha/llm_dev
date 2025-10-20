package main

import (
	"fmt"
	"llm_dev/codebase/impl"
)

var sss string

func main() {
	var op *impl.BuildCodeBaseCtxOps
	op.ExtractDefs()
	fmt.Println("Hello, Go!")
}
