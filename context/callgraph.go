package context

import (
	"bytes"
	"fmt"
	"llm_dev/codebase/impl"
	"llm_dev/utils"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

type CallGraphContextMgr struct {
	rootPath    string
	buildCtxOps *impl.BuildCodeBaseCtxOps
}

func (mgr *CallGraphContextMgr) genReferenceOutput(usedDefs []impl.UsedDef) string {
	var buf bytes.Buffer
	defMap := make(map[string][]impl.UsedDef)
	for _, useddef := range usedDefs {
		if useddef.Isdependency {
			continue
		}
		defMap[useddef.File] = append(defMap[useddef.File], useddef)
	}
	buf.WriteString("# The following code use the definition\n\n")
	for file, defs := range defMap {
		fc := utils.FileContent{}
		for _, usedef := range defs {
			def := impl.Definition{
				RelFile:    usedef.File,
				Identifier: usedef.Identifier,
				Keyword:    usedef.Keyword,
			}
			findDef, err := mgr.buildCtxOps.FindOneDef(def)
			if err == nil {
				log.Error().Err(err).Msg("find exact one def fail")
				continue
			}
			fc.AddChunk(findDef.Summary)
		}
		buf.WriteString(fmt.Sprintf("- %s\n\n", file))
		fc.WriteContent(&buf, filepath.Join(mgr.rootPath, file))
	}
	buf.WriteByte('\n')
	return buf.String()
}

func (mgr *CallGraphContextMgr) genUseOutput(usedDefs []impl.UsedDef) string {
	var buf bytes.Buffer
	defMap := make(map[string][]impl.UsedDef)
	dependencyDefMap := make(map[string][]impl.UsedDef)
	for _, useddef := range usedDefs {
		if useddef.Isdependency {
			dependencyDefMap[useddef.PkgPath] = append(dependencyDefMap[useddef.PkgPath], useddef)
		} else {
			defMap[useddef.DefFile] = append(defMap[useddef.DefFile], useddef)
		}
	}
	buf.WriteString("# Use Definition In the codebase\n\n")
	for file, defs := range defMap {
		fc := utils.FileContent{}
		for _, usedef := range defs {
			fmt.Printf("%s %s %v\n", usedef.DefFile, usedef.DefIdentifier, usedef.DefKeyword)
			def := impl.Definition{
				RelFile:    usedef.DefFile,
				Identifier: usedef.DefIdentifier,
				Keyword:    usedef.DefKeyword,
			}
			findDef, err := mgr.buildCtxOps.FindOneDef(def)
			if err != nil {
				log.Error().Err(err).Msg("find exact one def fail")
				continue
			}
			fc.AddChunk(findDef.Summary)
		}
		buf.WriteString(fmt.Sprintf("- %s\n\n", file))
		fc.WriteContent(&buf, filepath.Join(mgr.rootPath, file))
	}
	buf.WriteByte('\n')
	buf.WriteString("# Use Definition from Dependency\n\n")
	for pkg, useddefs := range dependencyDefMap {
		buf.WriteString(fmt.Sprintf("- pkg path %s\n\n", pkg))
		for _, usedef := range useddefs {
			kind := usedef.DefKeyword[0]
			if kind == "type" {
				buf.WriteString(fmt.Sprintf("%s %s\n", usedef.DefKeyword[0], usedef.DefKeyword[1]))
			}
			if kind == "var" {
				buf.WriteString(fmt.Sprintf("%s %s %s\n", usedef.DefKeyword[0], usedef.DefKeyword[1], usedef.DefKeyword[2]))
			}
			if kind == "function" {
				buf.WriteString(fmt.Sprintf("%s %s\n", usedef.DefKeyword[0], usedef.DefKeyword[1]))
			}
			if kind == "method" {
				buf.WriteString(fmt.Sprintf("%s %s of type %s\n", usedef.DefKeyword[0], usedef.DefKeyword[1], usedef.DefKeyword[2]))
			}
		}
	}
	buf.WriteByte('\n')
	return buf.String()
}

func (mgr *CallGraphContextMgr) findReference(file string, identifier string, line uint) ([]impl.UsedDef, error) {
	filter := bson.M{
		"relfile":           file,
		"identifier":        identifier,
		"content.startLine": line,
	}
	res := mgr.buildCtxOps.FindDefs(filter)
	if len(res) != 1 {
		return nil, fmt.Errorf("could not identify the exact definition using %s %s %d", file, identifier, line)
	}
	def := res[0]
	useDefFilter := bson.M{
		"DefFile":       def.RelFile,
		"DefIdentifier": def.Identifier,
		"DefKeyword": bson.M{
			"$all": def.Keyword,
		},
	}
	useDefRes := mgr.buildCtxOps.FindUsedDefs(useDefFilter)
	return useDefRes, nil
}
func (mgr *CallGraphContextMgr) findUsedDefs(file string, identifier string, line uint) ([]impl.UsedDef, error) {
	filter := bson.M{
		"relfile":    file,
		"identifier": identifier,
		// "content.startLine": line,
	}
	res := mgr.buildCtxOps.FindDefs(filter)
	if len(res) != 1 {
		return nil, fmt.Errorf("could not identify the exact definition using %s %s %d", file, identifier, line)
	}
	def := res[0]
	useDefFilter := bson.M{
		"file":       def.RelFile,
		"identifier": def.Identifier,
		"keyword": bson.M{
			"$all": def.Keyword,
		},
	}
	useDefRes := mgr.buildCtxOps.FindUsedDefs(useDefFilter)
	return useDefRes, nil
}
