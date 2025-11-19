package context

import (
	"bytes"
	"encoding/json"
	"fmt"
	"llm_dev/model"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

var readPrompt = `
You have access to 'pin_chunk', 'grep', 'read_content' tools for searching and reading text.
- read_content: read the file content to read buffer.
- grep: use ripgrep to search text.
- pin_chunk: pin the relevant content chunk for overall understand.

`

var pinChunk = openai.FunctionDefinition{
	Name:   "pin_chunk",
	Strict: true,
	Description: `
Pin a chunk of content of a file.
When a chunk is more than 50 lines, it will be omitted to 50 lines.

Usage:
- pin the relevant chunk that is helpful to solve the task.
- summary long range chunk and pin it with comment.
`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"file": {
				Type:        jsonschema.String,
				Description: "the path of the file",
			},
			"startline": {
				Type:        jsonschema.Number,
				Description: "the start line number of the chunk",
			},
			"endline": {
				Type:        jsonschema.Number,
				Description: "the end line number of the chunk",
			},
			"comment": {
				Type:        jsonschema.String,
				Description: "comment for the pined chunk",
			},
		},
		Required: []string{"arguments"},
	},
}
var grep = openai.FunctionDefinition{
	Name:   "grep",
	Strict: true,
	Description: `
A powerful search tool built on ripgrep.

Usage:
- use grep for searching plain text in the file.
- Always use -n options to add line number in the result

<example>
Grep arguments= -T js -T ts "router"
Grep arguments= -C 3 "pattern"
</example>

`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"arguments": {
				Type:        jsonschema.String,
				Description: "the argument to pass to ripgrep",
			},
		},
		Required: []string{"arguments"},
	},
}
var readContent = openai.FunctionDefinition{
	Name:   "read_content",
	Strict: true,
	Description: `
Read certain range of lines in a file to the read buffer.
The read buffer can at most show 200 lines of content.

# Usage
You should specify three arguments:
- file: the file path of the file.
- line: the line number of the read position, line number start from 1
- mode: the read mode, before, after, middle.

Three mode:
- before: read at most 200 lines before the read position.
- after: read at most 200 lines after the read position.
- middle: read both sides of the read position, at most 200 lines.

`,
	Parameters: jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"file": {
				Type:        jsonschema.String,
				Description: "the path of the file",
			},
			"line": {
				Type:        jsonschema.Number,
				Description: "the line number of the read position",
			},
			"mode": {
				Type:        jsonschema.String,
				Description: "the mode of the read, before, after, middle",
				Enum:        []string{"before", "after", "middle"},
			},
		},
		Required: []string{"file", "line", "mode"},
	},
}

type Chunk struct {
	startline uint
	endline   uint
	comment   string
}

type file struct {
	chunks []Chunk
}

func (self *file) addChunk(s, e uint, comment string) {
	self.chunks = append(self.chunks, Chunk{
		startline: s,
		endline:   e,
		comment:   comment,
	})
}
func (self *file) write(buf *bytes.Buffer, path string) {
	if len(self.chunks) == 0 {
		return
	}
	sort.Slice(self.chunks, func(i, j int) bool {
		return self.chunks[i].startline < self.chunks[j].startline
	})
	for _, c := range self.chunks {
		optios := fmt.Sprintf(`NR>=%d && NR<=%d {print NR ": " $0}`, c.startline, c.endline)
		cmd := exec.Command("awk", optios, path)
		output, err := cmd.Output()
		if err != nil {
			continue
		}
		buf.WriteString(fmt.Sprintf("Range: %d, %d\n", c.startline, c.endline))
		buf.WriteString(fmt.Sprintf("Comment: %s\n", c.comment))
		buf.WriteString("```\n")
		buf.Write(output)
		buf.WriteString("```\n")
	}
}

type ReadContextMgr struct {
	Root       string
	ReadBuffer string

	chunkByFile map[string]*file
}

func (mgr *ReadContextMgr) grep(arguments string) string {
	rgStr := "rg " + arguments
	cmd := exec.Command("bash", "-c", rgStr)
	cmd.Dir = mgr.Root
	output, err := cmd.Output()
	var buf bytes.Buffer
	if err != nil {
		return fmt.Sprintf("Execute grep failed, Error:%s", err)
	}
	buf.WriteString("Grep result:\n\n```\n")
	buf.Write(output)
	buf.WriteString("```\n")
	return buf.String()
}

func (mgr *ReadContextMgr) readContent(file string, line uint, mode string) string {
	var s, e int
	if mode == "before" {
		e = int(line)
		s = max(1, e-199)
	} else if mode == "after" {
		s = int(line)
		e = s + 199
	} else if mode == "middle" {
		s = max(int(line)-100, 1)
		e = int(line) + 100
	}
	path := filepath.Join(mgr.Root, file)
	optios := fmt.Sprintf(`NR>=%d && NR<=%d {print NR ": " $0}`, s, e)
	cmd := exec.Command("awk", optios, path)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Sprintf("Read lines %d-%d in file %s failed", s, e, file)
	}
	var buf bytes.Buffer
	lineCount, _ := exec.Command("wc", "-l", path).Output()
	number := strings.Fields(string(lineCount))[0]
	buf.WriteString(fmt.Sprintf("Read file %s, file total line count %s\n", file, number))
	buf.WriteString("```\n")
	buf.Write(output)
	buf.WriteString("```\n")
	mgr.ReadBuffer = buf.String()
	return fmt.Sprintf("Read lines %d-%d in file %s success", s, e, file)
}
func (mgr *ReadContextMgr) WriteContext(buf *bytes.Buffer) {
	buf.WriteString("### READ & GREP ###\n")
	buf.WriteString(readPrompt)
	buf.WriteString("# Pinned Chunks\n\n")
	for path, file := range mgr.chunkByFile {
		buf.WriteString(fmt.Sprintf("- %s\n", path))
		file.write(buf, filepath.Join(mgr.Root, path))
	}
	buf.WriteString("# Read Buffer\n\n")
	buf.WriteString(mgr.ReadBuffer)
	buf.WriteString("### END OF READ & GREP ###\n")
}

func (mgr *ReadContextMgr) GetToolDef() []model.ToolDef {
	readHandler := func(argsStr string) (string, error) {
		args := struct {
			File string
			Line uint
			Mode string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		return mgr.readContent(args.File, args.Line, args.Mode), nil
	}
	grepHandler := func(argsStr string) (string, error) {
		args := struct {
			Arguments string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		return mgr.grep(args.Arguments), nil
	}
	pinHandler := func(argsStr string) (string, error) {
		args := struct {
			File      string
			Startline uint
			Endline   uint
			Comment   string
		}{}
		err := json.Unmarshal([]byte(argsStr), &args)
		if err != nil {
			return "", err
		}
		return "", nil
	}
	res := []model.ToolDef{
		{FunctionDefinition: readContent, Handler: readHandler},
		{FunctionDefinition: grep, Handler: grepHandler},
		{FunctionDefinition: pinChunk, Handler: pinHandler},
	}
	return res
}
