package javatracker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type SnippetPayload struct {
	FileName string `json:"file_name"`
	Code     string `json:"code"`
}

func SaveJavaSnippet(baseDir string, payload SnippetPayload) (UploadResult, error) {
	code := strings.TrimSpace(payload.Code)
	if code == "" {
		return UploadResult{}, fmt.Errorf("code is required")
	}

	fileName := strings.TrimSpace(payload.FileName)
	if fileName == "" {
		fileName = guessJavaFileName(code)
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".java") {
		fileName += ".java"
	}
	fileName = sanitizeUploadName(fileName)
	if fileName == "" {
		fileName = "Main.java"
	}

	sessionRoot := filepath.Join(baseDir, "uploads", time.Now().UTC().Format("20060102-150405")+"-snippet")
	if err := os.MkdirAll(sessionRoot, 0o755); err != nil {
		return UploadResult{}, err
	}

	targetPath := filepath.Join(sessionRoot, fileName)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return UploadResult{}, err
	}
	if err := os.WriteFile(targetPath, []byte(code+"\n"), 0o644); err != nil {
		return UploadResult{}, err
	}

	return UploadResult{
		Root:  sessionRoot,
		Files: []string{targetPath},
	}, nil
}

func guessJavaFileName(code string) string {
	re := regexp.MustCompile(`\b(class|interface|enum)\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	matches := re.FindStringSubmatch(code)
	if len(matches) >= 3 {
		return matches[2] + ".java"
	}
	return "Main.java"
}
