package context

import (
	"bytes"
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/utils"
	"path/filepath"
)

type OutlineContextMgr struct {
	rootPath   string
	buildCtxOp impl.BuildCodeBaseCtxOps
	leafNode   map[string]bool
}

func (mgr *OutlineContextMgr) writeLeafNode(buf *bytes.Buffer, path string) {
	usedDefs := mgr.buildCtxOp.FindUsedDefOutline(path)
	defByFile := make(map[string]*utils.FileContent)
	for _, def := range usedDefs {
		fc, exist := defByFile[def.RelFile]
		if !exist {
			defByFile[def.RelFile] = &utils.FileContent{}
			fc = defByFile[def.RelFile]
		}
		fc.AddChunk(def.Summary)
	}
	if len(defByFile) == 0 {
		buf.WriteString(fmt.Sprintf("# %s\n\n", path))
		buf.WriteString("NO Definition Used by Outer code\n\n")
		return
	}
	if true {
		buf.WriteString(fmt.Sprintf("# %s\n\n", path))
		for path, fc := range defByFile {
			file := filepath.Join(mgr.rootPath, path)
			buf.WriteString(fmt.Sprintf("- %s\n", path))
			fc.WriteContent(buf, filepath.Join(mgr.rootPath, file))
			buf.WriteByte('\n')
		}
		// } else {
		// 	if len(defByFile) > 1 {
		// 		log.Fatal().Any("file", path).Msg("def by file len is more than 1 for file")
		// 	}
		// 	fc := defByFile[path]
		// 	buf.WriteString(fmt.Sprintf("# %s\n\n", path))
		// 	fc.WriteContent(buf, filepath.Join(mgr.rootPath, path))
		// 	buf.WriteByte('\n')
	}
}

func (mgr *OutlineContextMgr) WriteOutline(buf *bytes.Buffer) {

}
