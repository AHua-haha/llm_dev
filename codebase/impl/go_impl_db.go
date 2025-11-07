package impl

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io/fs"
	"llm_dev/codebase/common"
	"llm_dev/database"
	"llm_dev/utils"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
	ignore "github.com/sabhiram/go-gitignore"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/tools/go/packages"
)

const (
	typeQueryStr string = `
(type_identifier) @type
	`
	nameQueryStr string = `
(identifier) @type
`
	varSpecQueryStr string = `
(var_spec) @var
`
)

var typeTSQuery *common.TSQuery
var nameTSQuery *common.TSQuery
var varTSQuery *common.TSQuery

func InitTSQuery() {
	var err error
	lang := tree_sitter.NewLanguage(golang.Language())
	typeTSQuery, err = common.NewTSQuery(typeQueryStr, lang)
	if err != nil {
		log.Fatal().Err(err).Msg("init query failed")
	}
	nameTSQuery, err = common.NewTSQuery(nameQueryStr, lang)
	if err != nil {
		log.Fatal().Err(err).Msg("init query failed")
	}
	varTSQuery, err = common.NewTSQuery(varSpecQueryStr, lang)
	if err != nil {
		log.Fatal().Err(err).Msg("init query failed")
	}
}
func CloseTSQuery() {
	if typeTSQuery != nil {
		typeTSQuery.Close()
	}
	if nameTSQuery != nil {
		nameTSQuery.Close()
	}
	if varTSQuery != nil {
		varTSQuery.Close()
	}
}

type TypeInfo struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"` // Maps to MongoDB _id
	Identifier   string
	Keyword      []string
	DeclareFile  string
	UseFile      string
	IsDependency bool
}

func (info *TypeInfo) genIDFilter() bson.M {
	builder := database.NewFilter()
	builder.AddKV("_id", info.ID)
	return builder.Build()
}
func (info *TypeInfo) genFilterForDef() bson.M {
	var file *string
	var identifier *string
	if info.DeclareFile != "" {
		file = &info.DeclareFile
	}
	if info.Identifier != "" {
		identifier = &info.Identifier
	}
	return GenDefFilter(file, identifier, info.Keyword)
}

func (info *TypeInfo) addKeyword(value string) {
	info.Keyword = append(info.Keyword, value)
}

type Definition struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"` // Maps to MongoDB _id
	Identifier string
	Point      common.Point
	Keyword    []string
	Summary    utils.Range
	Content    utils.Range
	MinPrefix  string
	RelFile    string
}

func NewDef(node *tree_sitter.Node, data []byte) []Definition {
	defs := []Definition{}
	def := Definition{}
	Kind := node.Kind()
	switch Kind {
	case "method_declaration", "function_declaration":
		def.Content = utils.Range{
			StartLine: node.StartPosition().Row + 1,
			EndLine:   node.EndPosition().Row + 1 + 1,
		}
		def.Summary = utils.Range{
			StartLine: node.StartPosition().Row + 1,
			EndLine:   node.ChildByFieldName("body").StartPosition().Row + 1 + 1,
		}
	default:
		def.Content = utils.Range{
			StartLine: node.StartPosition().Row + 1,
			EndLine:   node.EndPosition().Row + 1 + 1,
		}
		def.Summary = def.Content
	}
	switch Kind {
	case "var_declaration":
		res := varTSQuery.Query(node, data)
		for _, value := range res {
			name := value.Node.ChildByFieldName("name").Utf8Text(data)
			typeNode := value.Node.ChildByFieldName("type")
			types := typeTSQuery.QueryStr(typeNode, data)
			def.Identifier = name
			def.Keyword = append(def.Keyword, "var", name)
			def.Keyword = append(def.Keyword, types...)
			defs = append(defs, def)
		}
	case "short_var_declaration":
		res := nameTSQuery.Query(node, data)
		for _, value := range res {
			name := value.Node.Utf8Text(data)
			def.Identifier = name
			def.Keyword = []string{"var", name}
			defs = append(defs, def)
		}
	case "package_clause":
		identifier := node.Child(1).Utf8Text(data)
		def.Identifier = identifier
		def.Keyword = []string{"package", identifier}
		defs = append(defs, def)
	case "import_declaration":
		def.Keyword = []string{"import"}
		defs = append(defs, def)
	case "type_declaration":
		identifier := node.Child(1).ChildByFieldName("name").Utf8Text(data)
		def.Identifier = identifier
		def.Keyword = []string{"type", identifier}
		defs = append(defs, def)
	case "function_declaration":
		name := node.ChildByFieldName("name").Utf8Text(data)
		def.Identifier = name
		def.Keyword = []string{"function", name}
		defs = append(defs, def)
	case "method_declaration":
		receiver := node.ChildByFieldName("receiver")
		name := node.ChildByFieldName("name").Utf8Text(data)
		types := typeTSQuery.QueryStr(receiver, data)
		def.Identifier = name
		def.Keyword = []string{"method", name}
		def.Keyword = append(def.Keyword, types...)
		defs = append(defs, def)
	default:
		log.Fatal().Msg("unsupported treesitter node type")
	}
	return defs
}

type UsedDef struct {
	ID            primitive.ObjectID `bson:"_id,omitempty"` // Maps to MongoDB _id
	Identifier    string
	Keyword       []string
	File          string
	DefIdentifier string
	DefKeyword    []string
	DefFile       string
	PkgPath       string
	Isdependency  bool
}

func NewUseDef(loc types.Object, usedDef types.Object) UsedDef {
	id, key := genTypeInfo(loc)
	useid, usekey := genTypeInfo(usedDef)
	res := UsedDef{
		Identifier:    id,
		Keyword:       key,
		DefIdentifier: useid,
		DefKeyword:    usekey,
	}
	return res
}

func (use *UsedDef) AddKeyword(value string) {
	use.Keyword = append(use.Keyword, value)
}
func (use *UsedDef) AddDefKeyword(value string) {
	use.DefKeyword = append(use.DefKeyword, value)
}

func genTypeInfo(obj types.Object) (string, []string) {
	var identifier string
	var keyword []string
	switch obj := obj.(type) {
	case *types.Var:
		typeName := obj.Type().String()
		idx := strings.LastIndex(typeName, ".")
		shortName := typeName[idx+1:]
		identifier = obj.Name()
		keyword = []string{"var", obj.Name(), shortName}
	case *types.PkgName:
		identifier = obj.Name()
		keyword = []string{"package", obj.Name()}
	case *types.TypeName:
		identifier = obj.Name()
		keyword = []string{"type", obj.Name()}
	case *types.Func:
		identifier = obj.Name()
		rece := obj.Signature().Recv()
		if rece != nil {
			typeName := rece.Type().String()
			idx := strings.LastIndex(typeName, ".")
			shortName := typeName[idx+1:]
			keyword = []string{"method", obj.Name(), shortName}
		} else {
			keyword = []string{"function", obj.Name()}
		}
	default:
		log.Fatal().Msg("unsupported node type")
	}
	return identifier, keyword
}

func GenDefFilter(relfile *string, identifier *string, keyword []string) bson.M {
	builder := database.NewFilter()
	if relfile != nil {
		builder.AddKV("relfile", relfile)
	}
	if identifier != nil {
		builder.AddKV("identifier", identifier)
	}
	if len(keyword) != 0 {
		keywordFilter := database.NewFilterKV(database.All, keyword)
		builder.AddFilter("keyword", keywordFilter)
	}
	filter := builder.Build()
	return filter
}
func (def *Definition) getValue(key string) any {
	switch key {
	case "id":
		return def.ID
	case "identifier":
		return def.Identifier
	case "keyword":
		return def.Keyword
	case "relfile":
		return def.RelFile
	case "minprefix":
		return def.MinPrefix
	default:
		return nil
	}
}

func (def *Definition) genUpdate(keys ...string) bson.M {
	kv := bson.M{}
	for _, key := range keys {
		value := def.getValue(key)
		kv[key] = value
	}
	return bson.M{"$set": kv}
}

func (def *Definition) genIDFilter() bson.M {
	builder := database.NewFilter()
	builder.AddKV("_id", def.ID)
	return builder.Build()
}

func (def *Definition) AddKeyword(value string) {
	def.Keyword = append(def.Keyword, value)
}

type FileDirInfo struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"` // Maps to MongoDB _id
	RelPath  string
	IsDir    bool
	UsedDefs []Definition
}

func (fd *FileDirInfo) GetSummary() map[string]utils.FileContent {
	defsByFile := fd.getDefByFile()
	res := make(map[string]utils.FileContent, len(defsByFile))
	for file, defs := range defsByFile {
		fc := utils.FileContent{}
		for _, def := range defs {
			fc.AddChunk(def.Summary)
		}
		res[file] = fc
	}
	return res
}

func (fd *FileDirInfo) getDefByFile() map[string][]Definition {
	res := make(map[string][]Definition)
	for _, usedDef := range fd.UsedDefs {
		res[usedDef.RelFile] = append(res[usedDef.RelFile], usedDef)
	}
	for _, v := range res {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Summary.StartLine < v[j].Summary.StartLine
		})
	}
	return res
}

type BuildCodeBaseCtxOps struct {
	RootPath string
	Db       *mongo.Database
}

func (op *BuildCodeBaseCtxOps) ExtractDefs() {
	// op.genAllDefs()
	fmt.Printf("done\n")
}
func (op *BuildCodeBaseCtxOps) FindUsedDefOutline(relpath string) []Definition {
	relpath = filepath.Clean(relpath)
	filter := bson.M{
		"minprefix": bson.M{
			"$not": bson.M{
				"$regex": fmt.Sprintf("^%s(/|$)", relpath),
			},
		},
		"relfile": bson.M{
			"$regex": fmt.Sprintf("^%s(/|$)", relpath),
		},
	}
	result := op.FindDefs(filter)
	return result
}
func (op *BuildCodeBaseCtxOps) GenAllUsedDefs() {
	ctx := common.WalkGoProjectTypeAst(op.RootPath, op.typeCtxHandler)
	for res := range ctx.OutputChan {
		usedDef := common.GetMapas[[]UsedDef](res, "used Defs")
		op.insertUsedTypeInfo(usedDef)
	}
}
func (op *BuildCodeBaseCtxOps) GenAllDefs() {
	ctx := common.WalkFileTree(op.RootPath, op.fileTreeCtxHandler())
	goFiles := []string{}
	for res := range ctx.OutputChan {
		path := common.GetMapas[string](res, "path")
		d := common.GetMapas[fs.DirEntry](res, "direntry")
		ext := filepath.Ext(d.Name())
		if d.IsDir() || ext != ".go" {
			continue
		}
		goFiles = append(goFiles, path)
	}
	InitTSQuery()
	defer CloseTSQuery()
	for _, file := range goFiles {
		fileAlldefs := []Definition{}
		ctx := common.WalkFileStaticAst(file, op.astCtxHandler)
		for res := range ctx.OutputChan {
			defs := common.GetMapas[[]Definition](res, "defs")
			fileAlldefs = append(fileAlldefs, defs...)
		}
		op.insertDefs(fileAlldefs)
	}
}

func (op *BuildCodeBaseCtxOps) SetMinPreFix() {
	usedDef := make(map[string]*Definition)
	collection := op.Db.Collection("Used")
	cursor, err := collection.Find(context.TODO(), bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("find used definition fail")
		return
	}
	defer cursor.Close(context.TODO())
	for cursor.Next(context.TODO()) {
		var useInfo UsedDef
		if err := cursor.Decode(&useInfo); err != nil {
			log.Error().Err(err).Msg("decode doc to UseDef fail")
			continue
		}
		if useInfo.Isdependency {
			continue
		}
		key := useInfo.DefFile + " " + strings.Join(useInfo.DefKeyword, " ")
		def, exist := usedDef[key]
		if !exist {
			usedDef[key] = &Definition{
				RelFile:    useInfo.DefFile,
				Identifier: useInfo.DefIdentifier,
				Keyword:    useInfo.DefKeyword,
				MinPrefix:  useInfo.DefFile,
			}
			continue
		}

		minPrefix := common.CommonRootDir(def.MinPrefix, useInfo.File)
		if minPrefix == def.MinPrefix {
			continue
		}
		def.MinPrefix = minPrefix
	}

	for _, def := range usedDef {
		finddef, err := op.FindOneDef(*def)
		if err != nil {
			log.Error().Err(err).Msg("find one def fail")
			continue
		}
		def.MinPrefix = filepath.Clean(def.MinPrefix)
		update := def.genUpdate("minprefix")
		collection := op.Db.Collection("Defs")
		_, err = collection.UpdateByID(context.TODO(), finddef.ID, update)
		if err != nil {
			log.Error().Err(err).Any("def", def).Msg("update definition failed")
		} else {
			log.Info().Any("def keyword", def.Keyword).Msg("update def minprefix")
		}
	}
}

func (op *BuildCodeBaseCtxOps) GenFileMap() map[string]*FileDirInfo {
	fileChan := op.WalkProjectFileTree()
	fileMap := make(map[string]*FileDirInfo)
	for fileInfo := range fileChan {
		relpath, _ := filepath.Rel(op.RootPath, fileInfo.Path)
		info := &FileDirInfo{
			RelPath: relpath,
		}
		if fileInfo.D.IsDir() {
			info.IsDir = true
		} else {
			info.IsDir = false
			ext := filepath.Ext(fileInfo.D.Name())
			if ext != ".go" {
				continue
			}
		}
		fileMap[relpath] = info
	}
	filter := bson.M{
		database.Expr: bson.M{
			database.Ne: []string{"$minprefix", "$relfile"},
		},
	}
	result := op.FindDefs(filter)
	for _, def := range result {
		p := def.RelFile
		root := def.MinPrefix
		if def.MinPrefix == "" {
			root = "."
		}
		for {
			if strings.HasPrefix(root, p) {
				break
			}
			fileMap[p].UsedDefs = append(fileMap[p].UsedDefs, def)
			p = filepath.Dir(p)
		}
	}
	return fileMap
}
func (op *BuildCodeBaseCtxOps) extractUsedTypeInfo(ctx *common.ContextHandler, typeObj types.Object) []UsedDef {
	cfg := common.GetAs[*packages.Config](ctx, "cfg")
	file := common.GetAs[string](ctx, "file")
	node := common.GetAs[ast.Node](ctx, "node")
	pkg := common.GetAs[*packages.Package](ctx, "pkg")
	mainModule := common.GetAs[*packages.Module](ctx, "mainModule")
	existMap := make(map[types.Object]struct{})
	s, e := node.Pos(), node.End()
	rootPos := cfg.Fset.Position(s)
	relPath, _ := filepath.Rel(op.RootPath, rootPos.Filename)
	checkPos := func(p token.Pos) bool {
		pos := cfg.Fset.Position(p)
		if pos.Filename == "" {
			return false
		}
		if pos.Filename != file {
			return true
		}
		if p >= s && p < e {
			return false
		}
		return true
	}
	ast.Inspect(node, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		typeObj := pkg.TypesInfo.Uses[ident]
		if typeObj == nil {
			return true
		}
		if _, exist := existMap[typeObj]; exist {
			return true
		}
		p := typeObj.Pos()
		if !checkPos(p) {
			return true
		}
		existMap[typeObj] = struct{}{}

		return true
	})
	res := []UsedDef{}
	for obj := range existMap {
		var useDef UsedDef
		switch obj := obj.(type) {
		case *types.Var:
			if obj.IsField() {
				continue
			}
			useDef = NewUseDef(typeObj, obj)
		case *types.TypeName, *types.Func:
			useDef = NewUseDef(typeObj, obj)
		default:
			continue
		}
		p := cfg.Fset.Position(obj.Pos())
		objRelPath, _ := filepath.Rel(op.RootPath, p.Filename)
		useDef.File = relPath
		useDef.DefFile = objRelPath
		path := obj.Pkg().Path()
		if !strings.HasPrefix(path, mainModule.Path) {
			useDef.Isdependency = true
			useDef.PkgPath = path
			useDef.DefFile = ""
		}
		res = append(res, useDef)
	}
	return res
}
func (op *BuildCodeBaseCtxOps) typeCtxHandler(ctx *common.ContextHandler, level uint) bool {
	if level == 0 {
		file := common.GetAs[string](ctx, "file")
		walkChild := strings.HasPrefix(file, op.RootPath)
		return walkChild
	}
	if level == 1 {
		node := common.GetAs[ast.Node](ctx, "node")
		pkg := common.GetAs[*packages.Package](ctx, "pkg")
		switch node := node.(type) {
		case *ast.File:
			return true
		case *ast.FuncDecl:
			// fmt.Printf("node.Name: %v\n", node.Name)
			typeObj := pkg.TypesInfo.Defs[node.Name]
			if typeObj != nil {
				usedDefs := op.extractUsedTypeInfo(ctx, typeObj)
				ctx.Push(map[string]any{
					"used Defs": usedDefs,
				})
			}
			return false
		case *ast.TypeSpec:
			// fmt.Printf("node.Name: %v\n", node.Name)
			typeObj := pkg.TypesInfo.Defs[node.Name]
			if typeObj != nil {
				usedDefs := op.extractUsedTypeInfo(ctx, typeObj)
				ctx.Push(map[string]any{
					"used Defs": usedDefs,
				})
			}
			return false
		default:
			return true
		}
	}
	return false
}
func (op *BuildCodeBaseCtxOps) astCtxHandler(ctx *common.ContextHandler, level uint) bool {
	if level == 0 {
		file := common.GetAs[string](ctx, "file")
		data := common.GetAs[[]byte](ctx, "data")
		node := common.GetAs[*tree_sitter.Node](ctx, "node")

		kind := node.Kind()
		switch kind {
		case "source_file":
			return true
		case "var_declaration", "short_var_declaration", "package_clause", "import_declaration", "type_declaration", "function_declaration", "method_declaration":
			defs := NewDef(node, data)
			relFile, _ := filepath.Rel(op.RootPath, file)
			for i := range defs {
				defs[i].RelFile = relFile
				defs[i].MinPrefix = relFile
			}
			ctx.OutputChan <- map[string]any{
				"defs": defs,
			}
		default:
			return false
		}
	}
	return false
}
func (op *BuildCodeBaseCtxOps) fileTreeCtxHandler() common.HandlerFunc {
	ig, err := ignore.CompileIgnoreFile(filepath.Join(op.RootPath, ".gitignore"))
	if err != nil {
		log.Error().Msgf("compile ignore failed")
	}
	handlerFunc := func(ctx *common.ContextHandler, level uint) bool {
		if level == 0 {
			path := common.GetAs[string](ctx, "path")
			d := common.GetAs[fs.DirEntry](ctx, "direntry")
			relPath, _ := filepath.Rel(op.RootPath, path)
			if d.Type()&os.ModeSymlink != 0 {
				return false
			}
			if ig != nil && ig.MatchesPath(relPath) {
				return false
			}
			ctx.OutputChan <- map[string]any{
				"path":     path,
				"relPath":  relPath,
				"direntry": d,
			}
			return true
		}
		return false
	}
	return handlerFunc
}

func (op *BuildCodeBaseCtxOps) FindOneDef(def Definition) (*Definition, error) {
	filter := GenDefFilter(&def.RelFile, &def.Identifier, nil)
	res := op.FindDefs(filter)
	resLen := len(res)
	if resLen == 1 {
		return &res[0], nil
	}
	if resLen == 0 {
		return nil, fmt.Errorf("def not found: %s %s", def.RelFile, def.Identifier)
	}
	matchCount := make([]int, resLen)
	defKeyMap := make(map[string]struct{}, len(def.Keyword))
	for _, str := range def.Keyword {
		defKeyMap[str] = struct{}{}
	}
	max := 0
	for i, elem := range res {
		for _, str := range elem.Keyword {
			if _, exist := defKeyMap[str]; exist {
				matchCount[i]++
			}
		}
		if matchCount[i] > max {
			max = matchCount[i]
		}
	}
	idx := 0
	maxCountCount := 0
	for i, count := range matchCount {
		if count == max {
			maxCountCount++
			idx = i
		}
	}
	if maxCountCount > 1 {
		return nil, fmt.Errorf("found multiple definition: %s %s %v", def.RelFile, def.Identifier, def.Keyword)
	}
	return &res[idx], nil
}

func ToAnySlice[T any](input []T) []any {
	result := make([]any, len(input))
	for i, v := range input {
		result[i] = v
	}
	return result
}
func (op *BuildCodeBaseCtxOps) insertUsedTypeInfo(array []UsedDef) {
	anySlice := ToAnySlice(array)
	op.Db.Collection("Used").InsertMany(context.TODO(), anySlice)
}

func (op *BuildCodeBaseCtxOps) insertDefs(array []Definition) {
	anySlice := ToAnySlice(array)
	op.Db.Collection("Defs").InsertMany(context.TODO(), anySlice)
}

func (op *BuildCodeBaseCtxOps) FindUsedDefs(filter bson.M) []UsedDef {
	collection := op.Db.Collection("Used")
	cursor, err := collection.Find(context.TODO(), filter)
	if err != nil {
		log.Error().Err(err).Any("filter", filter).Msgf("run find failed")
		return nil
	}
	defer cursor.Close(context.TODO())
	result := []UsedDef{}
	err = cursor.All(context.TODO(), &result)
	if err != nil {
		log.Error().Err(err).Msg("parse result to []UseDef failed")
		return nil
	}
	return result
}
func (op *BuildCodeBaseCtxOps) FindDefs(filter bson.M) []Definition {
	collection := op.Db.Collection("Defs")
	cursor, err := collection.Find(context.TODO(), filter)
	if err != nil {
		log.Error().Err(err).Any("filter", filter).Msgf("run fild failed")
		return nil
	}
	defer cursor.Close(context.TODO())
	result := []Definition{}
	err = cursor.All(context.TODO(), &result)
	if err != nil {
		log.Error().Err(err).Msg("parse result to []Definition failed")
		return nil
	}
	return result
}

type FileTreeCtx struct {
	Path string
	D    fs.DirEntry
}

func (op *BuildCodeBaseCtxOps) WalkProjectFileTree() <-chan FileTreeCtx {
	outputChan := make(chan FileTreeCtx, 10)
	go func() {
		ig, err := ignore.CompileIgnoreFile(filepath.Join(op.RootPath, ".gitignore"))
		if err != nil {
			log.Error().Msgf("compile ignore failed")
			return
		}
		walkDirFunc := func(path string, d fs.DirEntry, err error) error {
			keep := common.NewFilter(path, d).
				FilterSymlink().
				FilterGitIgnore(op.RootPath, ig).Keep()
			if !keep {
				if d.IsDir() {
					return filepath.SkipDir
				} else {
					return nil
				}
			}
			outputChan <- FileTreeCtx{
				Path: path,
				D:    d,
			}
			return nil
		}
		filepath.WalkDir(op.RootPath, walkDirFunc)
		close(outputChan)
	}()
	return outputChan
}
