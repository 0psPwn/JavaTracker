package javatracker

import (
	"bufio"
	"os"
	"sync"
)

type SourceCache struct {
	mu    sync.RWMutex
	lines map[string][]string
}

func NewSourceCache() *SourceCache {
	return &SourceCache{
		lines: make(map[string][]string),
	}
}

func (c *SourceCache) Lines(path string) ([]string, error) {
	c.mu.RLock()
	if lines, ok := c.lines[path]; ok {
		c.mu.RUnlock()
		return lines, nil
	}
	c.mu.RUnlock()

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	lines := make([]string, 0, 256)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.lines[path] = lines
	c.mu.Unlock()

	return lines, nil
}

func (c *SourceCache) Slice(path string, startLine, endLine int) ([]SourceLine, error) {
	lines, err := c.Lines(path)
	if err != nil {
		return nil, err
	}
	if startLine < 1 {
		startLine = 1
	}
	if endLine <= 0 || endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine {
		return nil, nil
	}

	out := make([]SourceLine, 0, endLine-startLine+1)
	for i := startLine - 1; i < endLine; i++ {
		out = append(out, SourceLine{
			Number: i + 1,
			Text:   lines[i],
		})
	}
	return out, nil
}
