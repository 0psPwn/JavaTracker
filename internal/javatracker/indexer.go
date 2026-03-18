package javatracker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type BuildProgress struct {
	root      string
	startedAt time.Time
	filesSeen atomic.Int64
	javaFiles atomic.Int64
}

func BuildProject(root string) (*Project, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	progress := &BuildProgress{
		root:      root,
		startedAt: started,
	}

	paths, err := discoverJavaFiles(root, progress)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errors.New("no Java files found")
	}

	project := &Project{
		Root:              root,
		Classes:           make(map[string]*ClassInfo, len(paths)),
		Fields:            make(map[string]*FieldInfo, len(paths)*2),
		Methods:           make(map[string]*MethodInfo, len(paths)*5),
		SearchByID:        make(map[string]SearchItem, len(paths)*8),
		Outgoing:          make(map[string][]GraphEdge, len(paths)*6),
		Incoming:          make(map[string][]GraphEdge, len(paths)*6),
		ClassBySimpleName: make(map[string][]string, len(paths)),
		MethodByName:      make(map[string][]string, len(paths)*4),
		MethodByClassName: make(map[string][]string, len(paths)*6),
		FileCache:         NewSourceCache(),
	}

	parsedFiles, err := parseJavaFiles(paths)
	if err != nil {
		return nil, err
	}

	project.collectDefinitions(parsedFiles)
	project.resolveEdges()
	project.buildSearchIndex()
	project.Stats = project.computeStats(started)

	return project, nil
}

func discoverJavaFiles(root string, progress *BuildProgress) ([]string, error) {
	paths := make([]string, 0, 1024)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		progress.filesSeen.Add(1)
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".idea", ".gradle", "target", "build", "out", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".java") {
			progress.javaFiles.Add(1)
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

func parseJavaFiles(paths []string) ([]*ParsedFile, error) {
	type result struct {
		file *ParsedFile
		err  error
	}

	workerCount := runtime.NumCPU()
	if workerCount < 4 {
		workerCount = 4
	}

	pathCh := make(chan string, workerCount)
	resultCh := make(chan result, workerCount)
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range pathCh {
				file, err := ParseJavaFile(path)
				resultCh <- result{file: file, err: err}
			}
		}()
	}

	go func() {
		for _, path := range paths {
			pathCh <- path
		}
		close(pathCh)
		wg.Wait()
		close(resultCh)
	}()

	files := make([]*ParsedFile, 0, len(paths))
	var errs []string
	for res := range resultCh {
		if res.err != nil {
			errs = append(errs, res.err.Error())
			continue
		}
		if res.file != nil {
			files = append(files, res.file)
		}
	}
	if len(files) == 0 && len(errs) > 0 {
		return nil, errors.New(errs[0])
	}
	return files, nil
}

func (p *Project) collectDefinitions(files []*ParsedFile) {
	for _, file := range files {
		for _, cls := range file.Classes {
			classID := cls.FullName
			info := &ClassInfo{
				ID:          classID,
				Name:        cls.Name,
				FullName:    cls.FullName,
				Kind:        cls.Kind,
				Package:     file.Package,
				FilePath:    file.Path,
				StartLine:   cls.StartLine,
				EndLine:     cls.EndLine,
				Extends:     strings.TrimSpace(cls.Extends),
				Implements:  append([]string(nil), cls.Implements...),
				Imports:     append([]string(nil), file.Imports...),
				Description: fmt.Sprintf("%s %s", cls.Kind, cls.FullName),
			}
			p.Classes[classID] = info
			p.ClassBySimpleName[cls.Name] = appendUnique(p.ClassBySimpleName[cls.Name], classID)

			for _, field := range cls.Fields {
				fieldID := fieldNodeID(classID, field.Name)
				fieldInfo := &FieldInfo{
					ID:        fieldID,
					ClassID:   classID,
					ClassName: cls.FullName,
					Name:      field.Name,
					Type:      field.Type,
					FilePath:  file.Path,
					StartLine: field.StartLine,
				}
				p.Fields[fieldID] = fieldInfo
				info.FieldIDs = append(info.FieldIDs, fieldID)
			}

			for _, method := range cls.Methods {
				methodID := methodNodeID(classID, method.Name, len(method.Parameters))
				methodInfo := &MethodInfo{
					ID:            methodID,
					ClassID:       classID,
					ClassName:     cls.FullName,
					Name:          method.Name,
					ReturnType:    method.ReturnType,
					Parameters:    append([]ParsedParam(nil), method.Parameters...),
					ParamCount:    len(method.Parameters),
					FilePath:      file.Path,
					StartLine:     method.StartLine,
					EndLine:       method.EndLine,
					LocalVarTypes: cloneStringMap(method.LocalVarTypes),
					CallSites:     append([]CallSite(nil), method.CallSites...),
					Description:   formatMethodDescription(cls.FullName, method),
				}
				p.Methods[methodID] = methodInfo
				p.MethodByName[method.Name] = appendUnique(p.MethodByName[method.Name], methodID)
				p.MethodByClassName[cls.FullName+":"+method.Name] = appendUnique(p.MethodByClassName[cls.FullName+":"+method.Name], methodID)
				info.MethodIDs = append(info.MethodIDs, methodID)
			}

			sort.Strings(info.MethodIDs)
			sort.Strings(info.FieldIDs)
		}
	}
}

func (p *Project) resolveEdges() {
	for _, classInfo := range p.Classes {
		for _, methodID := range classInfo.MethodIDs {
			p.addEdge(GraphEdge{Source: classInfo.ID, Target: methodID, Kind: EdgeHasMethod, Label: "contains"})
		}
		for _, fieldID := range classInfo.FieldIDs {
			p.addEdge(GraphEdge{Source: classInfo.ID, Target: fieldID, Kind: EdgeHasField, Label: "contains"})
		}

		if parent := p.resolveTypeReference(classInfo.ID, classInfo.Extends); parent != "" {
			p.addEdge(GraphEdge{Source: classInfo.ID, Target: parent, Kind: EdgeExtends, Label: "extends"})
		}

		for _, iface := range classInfo.Implements {
			if target := p.resolveTypeReference(classInfo.ID, iface); target != "" {
				p.addEdge(GraphEdge{Source: classInfo.ID, Target: target, Kind: EdgeImplements, Label: "implements"})
			}
		}
	}

	for _, method := range p.Methods {
		accessed := make(map[string]bool)
		for _, fieldID := range p.resolveFieldAccess(method) {
			if !accessed[fieldID] {
				p.addEdge(GraphEdge{Source: method.ID, Target: fieldID, Kind: EdgeAccesses, Label: "accesses"})
				method.AccessedFields = append(method.AccessedFields, fieldID)
				accessed[fieldID] = true
			}
		}

		for _, call := range method.CallSites {
			for _, target := range p.resolveCallTargets(method, call) {
				p.addEdge(GraphEdge{
					Source: method.ID,
					Target: target,
					Kind:   EdgeCalls,
					Label:  call.Name,
				})
			}
		}
	}
}

func (p *Project) resolveFieldAccess(method *MethodInfo) []string {
	lines, err := p.FileCache.Slice(method.FilePath, method.StartLine, method.EndLine)
	if err != nil {
		return nil
	}

	fieldNames := make(map[string]string)
	if cls := p.Classes[method.ClassID]; cls != nil {
		for _, fieldID := range cls.FieldIDs {
			if field := p.Fields[fieldID]; field != nil {
				fieldNames[field.Name] = field.ID
			}
		}
	}

	accessed := make(map[string]bool)
	for _, line := range lines {
		text := stripInlineComments(line.Text)
		for name, id := range fieldNames {
			if strings.Contains(text, name) {
				if wordBoundaryContains(text, name) {
					accessed[id] = true
				}
			}
		}
	}

	out := make([]string, 0, len(accessed))
	for id := range accessed {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func wordBoundaryContains(text, target string) bool {
	idx := strings.Index(text, target)
	for idx >= 0 {
		leftOK := idx == 0 || !isIdentPart(rune(text[idx-1]))
		end := idx + len(target)
		rightOK := end >= len(text) || !isIdentPart(rune(text[end]))
		if leftOK && rightOK {
			return true
		}
		next := strings.Index(text[end:], target)
		if next < 0 {
			break
		}
		idx = end + next
	}
	return false
}

func (p *Project) resolveCallTargets(method *MethodInfo, call CallSite) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, 4)

	add := func(id string) {
		if id == "" || id == method.ID || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}

	if call.Qualifier == "" || call.Qualifier == "this" || call.Qualifier == "super" {
		for _, id := range p.MethodByClassName[method.ClassID+":"+call.Name] {
			add(id)
		}
		if call.Qualifier == "super" {
			if cls := p.Classes[method.ClassID]; cls != nil {
				parent := p.resolveTypeReference(method.ClassID, cls.Extends)
				for _, id := range p.MethodByClassName[parent+":"+call.Name] {
					add(id)
				}
			}
		}
	}

	if qualifier := call.Qualifier; qualifier != "" && qualifier != "this" && qualifier != "super" {
		if targetClass := p.resolveQualifierType(method, qualifier); targetClass != "" {
			for _, id := range p.MethodByClassName[targetClass+":"+call.Name] {
				add(id)
			}
		}
	}

	if len(out) == 0 {
		for _, id := range p.MethodByName[call.Name] {
			add(id)
			if len(out) >= 8 {
				break
			}
		}
	}

	sort.Strings(out)
	return out
}

func (p *Project) resolveQualifierType(method *MethodInfo, qualifier string) string {
	if qualifier == "" {
		return ""
	}

	if classes := p.ClassBySimpleName[qualifier]; len(classes) > 0 {
		return classes[0]
	}

	if fieldID := fieldNodeID(method.ClassID, qualifier); p.Fields[fieldID] != nil {
		return p.resolveTypeReference(method.ClassID, p.Fields[fieldID].Type)
	}

	if typ, ok := method.LocalVarTypes[qualifier]; ok {
		return p.resolveTypeReference(method.ClassID, typ)
	}

	return ""
}

func (p *Project) resolveTypeReference(contextClassID, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	ref = stripGenericsAndArrays(ref)
	if p.Classes[ref] != nil {
		return ref
	}
	if list := p.ClassBySimpleName[ref]; len(list) > 0 {
		if cls := p.Classes[contextClassID]; cls != nil {
			pkgRef := cls.Package + "." + ref
			for _, candidate := range list {
				if candidate == pkgRef {
					return candidate
				}
			}
			for _, imp := range cls.Imports {
				if strings.HasSuffix(imp, "."+ref) {
					return imp
				}
			}
		}
		return list[0]
	}
	return ""
}

func stripGenericsAndArrays(ref string) string {
	ref = strings.TrimSpace(ref)
	if idx := strings.Index(ref, "<"); idx >= 0 {
		ref = ref[:idx]
	}
	ref = strings.TrimSuffix(ref, "[]")
	ref = strings.TrimPrefix(ref, "? extends ")
	ref = strings.TrimPrefix(ref, "? super ")
	return strings.TrimSpace(ref)
}

func (p *Project) addEdge(edge GraphEdge) {
	for _, existing := range p.Outgoing[edge.Source] {
		if existing.Target == edge.Target && existing.Kind == edge.Kind && existing.Label == edge.Label {
			return
		}
	}
	p.Outgoing[edge.Source] = append(p.Outgoing[edge.Source], edge)
	p.Incoming[edge.Target] = append(p.Incoming[edge.Target], edge)
}

func (p *Project) buildSearchIndex() {
	items := make([]SearchItem, 0, len(p.Classes)+len(p.Fields)+len(p.Methods))

	for _, classInfo := range p.Classes {
		item := SearchItem{
			ID:          classInfo.ID,
			Label:       classInfo.FullName,
			Kind:        classInfo.Kind,
			FilePath:    classInfo.FilePath,
			Line:        classInfo.StartLine,
			Description: classInfo.Description,
		}
		items = append(items, item)
		p.SearchByID[item.ID] = item
	}

	for _, field := range p.Fields {
		item := SearchItem{
			ID:          field.ID,
			Label:       field.ClassName + "." + field.Name,
			Kind:        NodeField,
			ClassName:   field.ClassName,
			FilePath:    field.FilePath,
			Line:        field.StartLine,
			Description: field.Type,
		}
		items = append(items, item)
		p.SearchByID[item.ID] = item
	}

	for _, method := range p.Methods {
		signature := formatMethodSignature(method)
		item := SearchItem{
			ID:          method.ID,
			Label:       method.ClassName + "." + method.Name,
			Kind:        NodeMethod,
			ClassName:   method.ClassName,
			FilePath:    method.FilePath,
			Line:        method.StartLine,
			Signature:   signature,
			Description: method.Description,
		}
		items = append(items, item)
		p.SearchByID[item.ID] = item
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Label < items[j].Label
	})
	p.SearchItems = items
}

func (p *Project) computeStats(started time.Time) ProjectStats {
	stats := ProjectStats{
		IndexedAt:  time.Now(),
		DurationMS: time.Since(started).Milliseconds(),
		JavaFiles:  len(uniqueFilePaths(p.Classes)),
		Classes:    0,
		Interfaces: 0,
		Enums:      0,
		Methods:    len(p.Methods),
		Fields:     len(p.Fields),
	}
	for _, cls := range p.Classes {
		switch cls.Kind {
		case NodeClass:
			stats.Classes++
		case NodeInterface:
			stats.Interfaces++
		case NodeEnum:
			stats.Enums++
		}
	}
	for _, edges := range p.Outgoing {
		for _, edge := range edges {
			switch edge.Kind {
			case EdgeCalls:
				stats.CallEdges++
			case EdgeAccesses:
				stats.AccessEdges++
			case EdgeExtends, EdgeImplements:
				stats.InheritEdges++
			}
		}
	}
	return stats
}

func uniqueFilePaths(classes map[string]*ClassInfo) map[string]struct{} {
	set := make(map[string]struct{}, len(classes))
	for _, cls := range classes {
		set[cls.FilePath] = struct{}{}
	}
	return set
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func appendUnique(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}

func fieldNodeID(classID, fieldName string) string {
	return classID + "#field:" + fieldName
}

func methodNodeID(classID, methodName string, paramCount int) string {
	return fmt.Sprintf("%s#method:%s/%d", classID, methodName, paramCount)
}

func formatMethodSignature(method *MethodInfo) string {
	parts := make([]string, 0, len(method.Parameters))
	for _, param := range method.Parameters {
		parts = append(parts, strings.TrimSpace(param.Type)+" "+param.Name)
	}
	return fmt.Sprintf("%s %s(%s)", method.ReturnType, method.Name, strings.Join(parts, ", "))
}

func formatMethodDescription(className string, method *ParsedMethod) string {
	parts := make([]string, 0, len(method.Parameters))
	for _, param := range method.Parameters {
		parts = append(parts, param.Type)
	}
	return fmt.Sprintf("%s.%s(%s)", className, method.Name, strings.Join(parts, ", "))
}
