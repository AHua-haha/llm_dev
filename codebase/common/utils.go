package common

import (
	"bufio"
	"fmt"
	"io/fs"
	_ "llm_dev/utils"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	ignore "github.com/sabhiram/go-gitignore"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

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
