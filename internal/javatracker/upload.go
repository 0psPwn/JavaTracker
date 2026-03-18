package javatracker

import (
	"archive/zip"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type UploadResult struct {
	Root  string   `json:"root"`
	Files []string `json:"files"`
}

func SaveUploadedJavaProject(baseDir string, files []*multipart.FileHeader) (UploadResult, error) {
	if len(files) == 0 {
		return UploadResult{}, fmt.Errorf("no files uploaded")
	}

	sessionRoot := filepath.Join(baseDir, "uploads", time.Now().UTC().Format("20060102-150405"))
	if err := os.MkdirAll(sessionRoot, 0o755); err != nil {
		return UploadResult{}, err
	}

	saved := make([]string, 0, len(files))
	var extractedRoots []string
	for _, header := range files {
		if header == nil {
			continue
		}
		name := sanitizeUploadName(header.Filename)
		if name == "" {
			continue
		}

		lowerName := strings.ToLower(name)
		if strings.HasSuffix(lowerName, ".zip") {
			targetPath := filepath.Join(sessionRoot, filepath.Base(name))
			if err := saveMultipartFile(header, targetPath); err != nil {
				return UploadResult{}, err
			}
			extractRoot := strings.TrimSuffix(targetPath, filepath.Ext(targetPath))
			if err := unzipFile(targetPath, extractRoot); err != nil {
				return UploadResult{}, err
			}
			extractedRoots = append(extractedRoots, extractRoot)
			saved = append(saved, targetPath)
			continue
		}

		if !isAcceptedProjectFile(lowerName) {
			continue
		}

		targetPath := filepath.Join(sessionRoot, name)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return UploadResult{}, err
		}
		if err := saveMultipartFile(header, targetPath); err != nil {
			return UploadResult{}, err
		}
		saved = append(saved, targetPath)
	}

	if len(saved) == 0 {
		return UploadResult{}, fmt.Errorf("no valid Java project files uploaded")
	}

	sort.Strings(saved)
	root := sessionRoot
	if len(extractedRoots) == 1 {
		root = extractedRoots[0]
	}

	return UploadResult{
		Root:  root,
		Files: saved,
	}, nil
}

func saveMultipartFile(header *multipart.FileHeader, targetPath string) error {
	src, err := header.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func sanitizeUploadName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "/")
	parts := strings.Split(name, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		clean = append(clean, part)
	}
	return filepath.Clean(strings.Join(clean, string(filepath.Separator)))
}

func isAcceptedProjectFile(lowerName string) bool {
	switch {
	case strings.HasSuffix(lowerName, ".java"):
		return true
	case strings.HasSuffix(lowerName, ".xml"):
		return strings.HasSuffix(lowerName, "pom.xml")
	case strings.HasSuffix(lowerName, ".gradle"),
		strings.HasSuffix(lowerName, ".gradle.kts"),
		strings.HasSuffix(lowerName, "settings.gradle"),
		strings.HasSuffix(lowerName, "settings.gradle.kts"),
		strings.HasSuffix(lowerName, "mvnw"),
		strings.HasSuffix(lowerName, "mvnw.cmd"),
		strings.HasSuffix(lowerName, ".properties"),
		strings.HasSuffix(lowerName, ".yaml"),
		strings.HasSuffix(lowerName, ".yml"):
		return true
	default:
		return strings.Contains(lowerName, "/src/") || strings.HasPrefix(lowerName, "src/")
	}
}

func unzipFile(zipPath, destDir string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		name := sanitizeUploadName(file.Name)
		if name == "" {
			continue
		}
		targetPath := filepath.Join(destDir, name)
		if !strings.HasPrefix(targetPath, filepath.Clean(destDir)+string(filepath.Separator)) && filepath.Clean(targetPath) != filepath.Clean(destDir) {
			return fmt.Errorf("invalid zip entry: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.Create(targetPath)
		if err != nil {
			src.Close()
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			src.Close()
			return err
		}
		dst.Close()
		src.Close()
	}

	return nil
}
