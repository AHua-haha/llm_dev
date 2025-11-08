package context

import (
	"bytes"
	"llm_dev/model"
)

type ContextMgr interface {
	WriteContext(buf *bytes.Buffer)
	GetToolDef() []model.ToolDef
}
