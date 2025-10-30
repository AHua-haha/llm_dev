package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"sort"
)

type Range struct {
	StartLine uint
	EndLine   uint
}

type FileContent struct {
	chunks []Range
}

func (fc *FileContent) AddChunk(r Range) {
	fc.chunks = append(fc.chunks, r)
}
func (fc *FileContent) AddChunks(chunk []Range) {
	fc.chunks = append(fc.chunks, chunk...)
}

func (fc *FileContent) tidy() Range {
	if len(fc.chunks) == 0 {
		return Range{}
	}
	sort.Slice(fc.chunks, func(i, j int) bool {
		return fc.chunks[i].StartLine < fc.chunks[j].StartLine
	})
	size := len(fc.chunks)
	r := fc.chunks[0]
	var res []Range
	for i := 1; i < size; i++ {
		chunk := fc.chunks[i]
		if chunk.StartLine <= r.EndLine {
			r.EndLine = max(r.EndLine, chunk.EndLine)
		} else {
			res = append(res, r)
			r = chunk
		}
	}
	res = append(res, r)
	fc.chunks = res
	return Range{
		StartLine: res[0].StartLine,
		EndLine:   r.EndLine,
	}
}
func (fc *FileContent) WriteContent(buf *bytes.Buffer, filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	maxRange := fc.tidy()

	var lines []string
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum < int(maxRange.StartLine) {
			continue // skip until we reach the start
		}
		if lineNum == int(maxRange.EndLine) {
			break // stop once we've read the last desired line
		}
		lines = append(lines, scanner.Text())
	}
	lineSize := len(lines)
	chunkSize := len(fc.chunks)
	for i, chunk := range fc.chunks {
		for i := chunk.StartLine; i < chunk.EndLine; i++ {
			lineNum := i - maxRange.StartLine
			if lineNum >= uint(lineSize) {
				break
			}
			buf.WriteString(fmt.Sprintf("%3d| ", i))
			buf.WriteString(lines[lineNum])
			buf.WriteByte('\n')
		}
		if i != chunkSize-1 {
			buf.WriteString("...\n")
		}
	}
	return nil
}
