package common

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/token"
	"io/fs"
	_ "llm_dev/utils"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"golang.org/x/tools/go/packages"

	ignore "github.com/sabhiram/go-gitignore"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

type TSQuery struct {
	query       *tree_sitter.Query
	cpatureName []string
}

type QueryRes struct {
	Node        *tree_sitter.Node
	CaptureName string
}

func NewTSQuery(queryStr string, lang *tree_sitter.Language) (*TSQuery, error) {
	query, err := tree_sitter.NewQuery(lang, queryStr)
	if err != nil {
		return nil, err
	}
	return &TSQuery{
		query:       query,
		cpatureName: query.CaptureNames(),
	}, nil
}

func (q *TSQuery) Close() {
	q.query.Close()
}
func (q *TSQuery) QueryStr(root *tree_sitter.Node, data []byte) []string {
	queryRes := q.Query(root, data)
	strRes := make([]string, len(queryRes))

	for i, elem := range queryRes {
		strRes[i] = elem.Node.Utf8Text(data)
	}
	return strRes
}

func (q *TSQuery) Query(root *tree_sitter.Node, data []byte) []QueryRes {
	if root == nil {
		return nil
	}
	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()
	matches := cursor.Matches(q.query, root, data)

	var res []QueryRes
	for {
		match := matches.Next()
		if match == nil {
			break
		}

		for _, cap := range match.Captures {
			queryRes := QueryRes{
				Node:        &cap.Node,
				CaptureName: q.cpatureName[cap.Index],
			}
			res = append(res, queryRes)
		}
	}
	return res
}

func WalkAst(root *tree_sitter.Node, op AstNodeOps) {
	walk_child := op(root)
	if walk_child {
		for i := uint(0); i < root.ChildCount(); i++ {
			child := root.Child(i)
			WalkAst(child, op)
		}
	}
}

func GenIgnoreOps(root string, op FileOps) FileOps {
	ig, err := ignore.CompileIgnoreFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		log.Error().Msgf("compile ignore failed")
		return op
	}
	ignore_ops := func(path string, d fs.DirEntry) (Node, bool) {
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			log.Error().
				Str("root", root).
				Str("path", path).
				Msg("get relative path failed")
			return nil, false
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil, false
		}

		if ig.MatchesPath(relPath) {
			return nil, false
		}
		return op(path, d)
	}
	return ignore_ops
}
func WalkDirGenNode(root string, file_op FileOps) Node {
	info, err := os.Stat(root)
	if err != nil {
		log.Error().Err(err).
			Str("file", root).
			Msg("get file stat fail")
		return nil
	}
	return walkDir(root, fs.FileInfoToDirEntry(info), file_op)
}

func walkDir(root string, d fs.DirEntry, file_op FileOps) Node {
	node, walk_child := file_op(root, d)
	if node == nil {
		return nil
	}
	if !walk_child {
		return node
	}
	if !d.IsDir() {
		return node
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		log.Error().Err(err).
			Str("file", root).
			Msg("get file entry failed")
		return node
	}

	for _, entry := range entries {
		child := walkDir(filepath.Join(root, entry.Name()), entry, file_op)
		if child != nil {
			node.AddChild(child)
		}
	}
	return node
}

func WalkNode(root Node, node_ops NodeOps) {
	walk_child := node_ops(root)
	if !walk_child {
		return
	}
	for _, child := range root.Child() {
		WalkNode(child, node_ops)
	}
}

func CommonPrefix(s1, s2 string) string {
	minLen := min(len(s1), len(s2))
	i := 0
	for i < minLen && s1[i] == s2[i] {
		i++
	}
	return s1[:i]
}

type FileFilter struct {
	path string
	d    fs.DirEntry
	keep bool
}

func NewFilter(path string, d fs.DirEntry) *FileFilter {
	return &FileFilter{
		path: path,
		d:    d,
		keep: true,
	}
}

func (f *FileFilter) Keep() bool {
	return f.keep
}

func (f *FileFilter) FilterGitIgnore(root string, ig *ignore.GitIgnore) *FileFilter {
	if !f.keep {
		return f
	}
	relPath, err := filepath.Rel(root, f.path)
	if err != nil {
		return f
	}
	if ig.MatchesPath(relPath) {
		f.keep = false
	}
	return f
}
func (f *FileFilter) FilterSymlink() *FileFilter {
	if !f.keep {
		return f
	}
	if f.d.Type()&os.ModeSymlink != 0 {
		f.keep = false
	}
	return f
}

func (f *FileFilter) FilterDir() *FileFilter {
	if !f.keep {
		return f
	}
	if f.d.IsDir() {
		f.keep = false
	}
	return f
}

func GetModulePath(goModPath string) (string, error) {
	f, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("module directive not found in %s", goModPath)
}

func CommonRootDir(path1, path2 string) string {
	// Clean and make absolute (optional, but recommended)
	p1 := filepath.Clean(path1)
	p2 := filepath.Clean(path2)

	// Split by OS-specific separator
	parts1 := strings.Split(p1, string(filepath.Separator))
	parts2 := strings.Split(p2, string(filepath.Separator))

	var commonParts []string

	// Find common prefix parts
	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		if parts1[i] == parts2[i] {
			commonParts = append(commonParts, parts1[i])
		} else {
			break
		}
	}

	if len(commonParts) == 0 {
		return ""
	}

	return filepath.Join(commonParts...)
}

type HandlerFunc func(ctx *ContextHandler, level uint) bool

type ContextHandler struct {
	OutputChan chan map[string]any
	ctxValue   map[string]any
	handler    HandlerFunc
}

func NewContextHandler(bufferSize uint, handler HandlerFunc) ContextHandler {
	return ContextHandler{
		OutputChan: make(chan map[string]any, bufferSize),
		ctxValue:   make(map[string]any),
		handler:    handler,
	}
}

func (ctx *ContextHandler) Push(res map[string]any) {
	ctx.OutputChan <- res
}
func (ctx *ContextHandler) Set(key string, value any) {
	ctx.ctxValue[key] = value
}

func GetMapas[T any](kv map[string]any, key string) T {
	value, exist := kv[key]
	if !exist {
		log.Fatal().Any("key", key).Msg("key does not exist")
	}
	dst, ok := value.(T)
	if !ok && value != nil {
		log.Fatal().Msg("type does not match")
	}
	return dst
}
func GetAs[T any](ctx *ContextHandler, key string) T {
	value, exist := ctx.ctxValue[key]
	if !exist {
		log.Fatal().Any("key", key).Msg("key does not exist")
	}
	dst, ok := value.(T)
	if !ok && value != nil {
		log.Fatal().Msg("type does not match")
	}
	return dst
}

func (ctx *ContextHandler) ProcessCtx(level uint) bool {
	return ctx.handler(ctx, level)
}

func WalkFileStaticAst(filePath string, handler HandlerFunc) *ContextHandler {
	ctx := NewContextHandler(10, handler)
	go func() {
		defer close(ctx.OutputChan)
		data, err := os.ReadFile(filePath)
		if err != nil {
			log.Error().Msgf("read file error %s", err)
			return
		}
		ctx.Set("file", filePath)
		ctx.Set("data", data)
		parser := tree_sitter.NewParser()
		parser.SetLanguage(tree_sitter.NewLanguage(golang.Language()))
		tree := parser.Parse(data, nil)
		defer tree.Clone()
		defer parser.Close()
		WalkAst(tree.RootNode(), func(root *tree_sitter.Node) bool {
			ctx.Set("node", root)
			return ctx.ProcessCtx(0)
		})
	}()
	return &ctx
}

func WalkGoProjectTypeAst(rootPath string, handler HandlerFunc) *ContextHandler {
	ctx := NewContextHandler(10, handler)
	go func() {
		defer close(ctx.OutputChan)
		cfg := &packages.Config{
			Mode:  packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedFiles | packages.NeedModule,
			Fset:  token.NewFileSet(),
			Dir:   rootPath,
			Tests: true,
		}
		ctx.Set("cfg", cfg)
		ctx.Set("root", rootPath)

		pkgs, err := packages.Load(cfg, "./...")
		if err != nil {
			log.Error().Err(err).Msg("type check project fail")
			return
		}
		var mainModule *packages.Module
		for _, pkg := range pkgs {
			if pkg.Module != nil && pkg.Module.Main {
				mainModule = pkg.Module
				break
			}
		}
		if mainModule == nil {
			log.Error().Msg("main module not found")
			return
		}
		ctx.Set("mainModule", mainModule)
		for _, pkg := range pkgs {
			if strings.Contains(pkg.ID, ".test") {
				continue // skip test and test variants
			}
			ctx.Set("pkg", pkg)
			for i, file := range pkg.Syntax {
				fileName := pkg.GoFiles[i]
				ctx.Set("file", fileName)
				if !ctx.ProcessCtx(0) {
					continue
				}
				ast.Inspect(file, func(n ast.Node) bool {
					ctx.Set("node", n)
					return ctx.ProcessCtx(1)
				})
			}
		}
	}()
	return &ctx
}

func WalkFileTree(rootPath string, handler HandlerFunc) *ContextHandler {
	ctx := NewContextHandler(10, handler)
	go func() {
		defer close(ctx.OutputChan)
		ctx.Set("root", rootPath)
		filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
			ctx.Set("path", path)
			ctx.Set("direntry", d)
			walkChild := ctx.ProcessCtx(0)
			if !d.IsDir() {
				return nil
			} else {
				if walkChild {
					return nil
				} else {
					return fs.SkipDir
				}
			}
		})

	}()
	return &ctx
}
