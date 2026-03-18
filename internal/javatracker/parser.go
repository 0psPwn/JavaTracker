package javatracker

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"unicode"
)

type token struct {
	Value  string
	Line   int
	Offset int
}

type parser struct {
	content string
	tokens  []token
	pos     int
	file    *ParsedFile
}

var (
	reVarDecl = regexp.MustCompile(`(?:^|[;{}()\s])(?:final\s+)?([A-Za-z_][A-Za-z0-9_<>\[\]\.$]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*(=|;|,)`)
	reCall    = regexp.MustCompile(`(?m)(?:(this|super|[A-Za-z_][A-Za-z0-9_]*)\s*\.\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

func ParseJavaFile(path string) (*ParsedFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseJavaSource(path, string(content))
}

func ParseJavaSource(path, content string) (*ParsedFile, error) {
	file := &ParsedFile{Path: path}
	p := &parser{
		content: content,
		tokens:  lexJava(content),
		file:    file,
	}
	if len(p.tokens) == 0 {
		return file, nil
	}
	p.parseFile()
	return file, nil
}

func lexJava(input string) []token {
	tokens := make([]token, 0, len(input)/4)
	line := 1
	for i := 0; i < len(input); {
		ch := input[i]

		if ch == '\n' {
			line++
			i++
			continue
		}
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\f' {
			i++
			continue
		}

		if ch == '/' && i+1 < len(input) {
			switch input[i+1] {
			case '/':
				i += 2
				for i < len(input) && input[i] != '\n' {
					i++
				}
				continue
			case '*':
				i += 2
				for i < len(input)-1 {
					if input[i] == '\n' {
						line++
					}
					if input[i] == '*' && input[i+1] == '/' {
						i += 2
						break
					}
					i++
				}
				continue
			}
		}

		if ch == '"' || ch == '\'' {
			quote := ch
			start := i
			i++
			for i < len(input) {
				if input[i] == '\n' {
					line++
				}
				if input[i] == '\\' {
					i += 2
					continue
				}
				if input[i] == quote {
					i++
					break
				}
				i++
			}
			tokens = append(tokens, token{Value: input[start:i], Line: line, Offset: start})
			continue
		}

		if i+2 < len(input) && input[i:i+3] == "..." {
			tokens = append(tokens, token{Value: "...", Line: line, Offset: i})
			i += 3
			continue
		}

		if isIdentStart(rune(ch)) {
			start := i
			i++
			for i < len(input) {
				r := rune(input[i])
				if isIdentPart(r) || input[i] == '.' {
					i++
					continue
				}
				break
			}
			tokens = append(tokens, token{Value: input[start:i], Line: line, Offset: start})
			continue
		}

		tokens = append(tokens, token{Value: string(ch), Line: line, Offset: i})
		i++
	}
	return tokens
}

func isIdentStart(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r)
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || unicode.IsDigit(r)
}

func (p *parser) parseFile() {
	for p.pos < len(p.tokens) {
		switch p.peek().Value {
		case "package":
			p.parsePackage()
		case "import":
			p.parseImport()
		default:
			if p.looksLikeClassStart() {
				if cls := p.parseClass(""); cls != nil {
					p.file.Classes = append(p.file.Classes, cls)
					continue
				}
			}
			p.pos++
		}
	}
}

func (p *parser) parsePackage() {
	p.pos++
	start := p.pos
	for p.pos < len(p.tokens) && p.tokens[p.pos].Value != ";" {
		p.pos++
	}
	if start < p.pos {
		p.file.Package = tokenText(p.tokens[start:p.pos])
	}
	p.consume(";")
}

func (p *parser) parseImport() {
	p.pos++
	start := p.pos
	for p.pos < len(p.tokens) && p.tokens[p.pos].Value != ";" {
		if p.tokens[p.pos].Value == "static" {
			p.pos++
			start = p.pos
			continue
		}
		p.pos++
	}
	if start < p.pos {
		p.file.Imports = append(p.file.Imports, tokenText(p.tokens[start:p.pos]))
	}
	p.consume(";")
}

func (p *parser) looksLikeClassStart() bool {
	i := p.pos
	for i < len(p.tokens) {
		switch p.tokens[i].Value {
		case "@":
			i = p.skipAnnotation(i)
		case "public", "protected", "private", "abstract", "final", "static", "sealed", "non-sealed":
			i++
		case "class", "interface", "enum":
			return true
		default:
			return false
		}
	}
	return false
}

func (p *parser) parseClass(owner string) *ParsedClass {
	startLine := p.peek().Line
	p.skipAnnotationsAndModifiers()

	kindValue := p.peek().Value
	kind := map[string]NodeKind{
		"class":     NodeClass,
		"interface": NodeInterface,
		"enum":      NodeEnum,
	}[kindValue]
	if kind == "" {
		return nil
	}
	p.pos++

	if p.pos >= len(p.tokens) {
		return nil
	}
	name := p.peek().Value
	p.pos++

	fullName := name
	if p.file.Package != "" {
		fullName = p.file.Package + "." + name
	}
	if owner != "" {
		fullName = owner + "$" + name
	}

	cls := &ParsedClass{
		Name:      name,
		FullName:  fullName,
		Kind:      kind,
		StartLine: startLine,
	}

	for p.pos < len(p.tokens) && p.peek().Value != "{" {
		switch p.peek().Value {
		case "extends":
			p.pos++
			cls.Extends = p.readTypeRef()
		case "implements":
			p.pos++
			for p.pos < len(p.tokens) && p.peek().Value != "{" {
				if p.peek().Value == "," {
					p.pos++
					continue
				}
				cls.Implements = append(cls.Implements, p.readTypeRef())
			}
		default:
			p.pos++
		}
	}
	if !p.consume("{") {
		return cls
	}

	bodyDepth := 1
	for p.pos < len(p.tokens) && bodyDepth > 0 {
		if p.looksLikeClassStart() {
			nested := p.parseClass(cls.FullName)
			if nested != nil {
				p.file.Classes = append(p.file.Classes, nested)
				continue
			}
		}

		switch p.peek().Value {
		case "{":
			bodyDepth++
			p.pos++
			continue
		case "}":
			bodyDepth--
			cls.EndLine = p.peek().Line
			p.pos++
			continue
		}

		if bodyDepth != 1 {
			p.pos++
			continue
		}

		if p.looksLikeMethodStart(cls.Name) {
			if method, err := p.parseMethod(cls.Name); err == nil && method != nil {
				cls.Methods = append(cls.Methods, method)
				continue
			}
		}

		if fields := p.parseFieldDeclaration(); len(fields) > 0 {
			cls.Fields = append(cls.Fields, fields...)
			continue
		}

		p.pos++
	}

	return cls
}

func (p *parser) looksLikeMethodStart(className string) bool {
	i := p.pos
	seenAssign := false
	for i < len(p.tokens) {
		if p.tokens[i].Value == ";" || p.tokens[i].Value == "}" {
			return false
		}
		if p.tokens[i].Value == "@" {
			i = p.skipAnnotation(i)
			continue
		}
		if p.tokens[i].Value == "=" {
			seenAssign = true
			i++
			continue
		}
		if p.tokens[i].Value == "(" {
			if seenAssign {
				return false
			}
			if i == 0 {
				return false
			}
			name := p.tokens[i-1].Value
			if isControlKeyword(name) || name == "new" {
				return false
			}
			j, ok := findBalanced(p.tokens, i, "(", ")")
			if !ok || j+1 >= len(p.tokens) {
				return false
			}
			next := p.tokens[j+1].Value
			for next == "throws" && j+1 < len(p.tokens) {
				j++
				for j+1 < len(p.tokens) && p.tokens[j+1].Value != "{" && p.tokens[j+1].Value != ";" {
					j++
				}
				if j+1 < len(p.tokens) {
					next = p.tokens[j+1].Value
				}
			}
			if next == "{" || next == ";" {
				return name == className || !strings.Contains(name, ".")
			}
			return false
		}
		if p.tokens[i].Value == "=" {
			return false
		}
		i++
	}
	return false
}

func (p *parser) parseMethod(className string) (*ParsedMethod, error) {
	start := p.pos
	startLine := p.peek().Line
	p.skipAnnotationsAndModifiers()
	if p.pos >= len(p.tokens) {
		return nil, errors.New("unexpected end while parsing method")
	}

	if p.peek().Value == "<" {
		end, ok := findBalanced(p.tokens, p.pos, "<", ">")
		if !ok {
			return nil, errors.New("unbalanced generics")
		}
		p.pos = end + 1
	}

	nameIndex := -1
	for i := p.pos; i < len(p.tokens); i++ {
		if p.tokens[i].Value == "(" {
			nameIndex = i - 1
			break
		}
		if p.tokens[i].Value == ";" || p.tokens[i].Value == "=" {
			return nil, errors.New("not a method")
		}
	}
	if nameIndex < p.pos || nameIndex >= len(p.tokens) {
		return nil, errors.New("missing method name")
	}

	name := p.tokens[nameIndex].Value
	var returnType string
	if name == className {
		returnType = "constructor"
	} else {
		returnType = strings.TrimSpace(tokenText(p.tokens[p.pos:nameIndex]))
	}
	if returnType == "" {
		returnType = "void"
	}

	openParen := nameIndex + 1
	closeParen, ok := findBalanced(p.tokens, openParen, "(", ")")
	if !ok {
		return nil, errors.New("unbalanced parameter list")
	}
	params := parseParameters(p.tokens[openParen+1 : closeParen])
	p.pos = closeParen + 1

	for p.pos < len(p.tokens) && p.peek().Value == "throws" {
		p.pos++
		for p.pos < len(p.tokens) && p.peek().Value != "{" && p.peek().Value != ";" {
			p.pos++
		}
	}

	method := &ParsedMethod{
		Name:       name,
		ReturnType: returnType,
		Parameters: params,
		StartLine:  startLine,
		EndLine:    startLine,
	}

	if p.pos < len(p.tokens) && p.peek().Value == ";" {
		method.EndLine = p.peek().Line
		p.pos++
		return method, nil
	}
	if p.pos >= len(p.tokens) || p.peek().Value != "{" {
		p.pos = start + 1
		return nil, errors.New("method body not found")
	}

	openBrace := p.pos
	closeBrace, ok := findBalanced(p.tokens, openBrace, "{", "}")
	if !ok {
		return nil, errors.New("unbalanced method body")
	}

	method.StartLine = p.tokens[start].Line
	method.EndLine = p.tokens[closeBrace].Line
	bodyStart := p.tokens[openBrace].Offset + 1
	bodyEnd := p.tokens[closeBrace].Offset
	if bodyStart >= 0 && bodyEnd >= bodyStart && bodyEnd <= len(p.content) {
		method.Body = p.content[bodyStart:bodyEnd]
	}
	method.LocalVarTypes, method.CallSites = analyzeMethodBody(method.Body, method.StartLine)
	p.pos = closeBrace + 1

	return method, nil
}

func (p *parser) parseFieldDeclaration() []*ParsedField {
	start := p.pos
	i := p.pos
	seenAssign := false
	depthParen := 0
	depthBracket := 0
	depthAngle := 0
	depthBrace := 0
	for i < len(p.tokens) {
		switch p.tokens[i].Value {
		case ";":
			if depthParen == 0 && depthBracket == 0 && depthAngle == 0 && depthBrace == 0 {
				goto found
			}
		case "=":
			if depthParen == 0 && depthBracket == 0 && depthAngle == 0 && depthBrace == 0 {
				seenAssign = true
			}
		case "(":
			if !seenAssign && depthParen == 0 && depthBracket == 0 && depthAngle == 0 && depthBrace == 0 {
				return nil
			}
			depthParen++
		case ")":
			if depthParen > 0 {
				depthParen--
			}
		case "[":
			depthBracket++
		case "]":
			if depthBracket > 0 {
				depthBracket--
			}
		case "<":
			depthAngle++
		case ">":
			if depthAngle > 0 {
				depthAngle--
			}
		case "{":
			if !seenAssign && depthParen == 0 && depthBracket == 0 && depthAngle == 0 && depthBrace == 0 {
				return nil
			}
			depthBrace++
		case "}":
			if depthBrace > 0 {
				depthBrace--
			} else if !seenAssign {
				return nil
			}
		}
		i++
	}
	return nil

found:
	if i >= len(p.tokens) || p.tokens[i].Value != ";" {
		return nil
	}
	if i == start {
		return nil
	}

	decl := append([]token(nil), p.tokens[start:i]...)
	p.pos = i + 1
	decl = stripAnnotationsAndModifiers(decl)
	if len(decl) < 2 {
		return nil
	}

	typeEnd := 0
	genericDepth := 0
	for typeEnd < len(decl)-1 {
		switch decl[typeEnd].Value {
		case "<":
			genericDepth++
		case ">":
			if genericDepth > 0 {
				genericDepth--
			}
		case "[", "]":
		case ",":
			if genericDepth == 0 {
				goto doneType
			}
		case "=":
			if genericDepth == 0 {
				goto doneType
			}
		}
		if genericDepth == 0 && typeEnd+1 < len(decl) && isSimpleIdentifier(decl[typeEnd+1].Value) {
			break
		}
		typeEnd++
	}
doneType:
	if typeEnd+1 >= len(decl) {
		return nil
	}
	fieldType := strings.TrimSpace(tokenText(decl[:typeEnd+1]))
	fields := make([]*ParsedField, 0, 2)
	line := decl[typeEnd+1].Line

	for j := typeEnd + 1; j < len(decl); {
		if !isSimpleIdentifier(decl[j].Value) {
			j++
			continue
		}
		name := decl[j].Value
		fields = append(fields, &ParsedField{
			Name:      name,
			Type:      fieldType,
			StartLine: line,
		})
		j++
		assignParen := 0
		assignBrace := 0
		assignBracket := 0
		assignAngle := 0
		for j < len(decl) {
			switch decl[j].Value {
			case ",":
				if assignParen == 0 && assignBrace == 0 && assignBracket == 0 && assignAngle == 0 {
					j++
					goto nextField
				}
			case "(":
				assignParen++
			case ")":
				if assignParen > 0 {
					assignParen--
				}
			case "{":
				assignBrace++
			case "}":
				if assignBrace > 0 {
					assignBrace--
				}
			case "[":
				assignBracket++
			case "]":
				if assignBracket > 0 {
					assignBracket--
				}
			case "<":
				assignAngle++
			case ">":
				if assignAngle > 0 {
					assignAngle--
				}
			}
			j++
		}
	nextField:
	}

	return fields
}

func (p *parser) skipAnnotationsAndModifiers() {
	for p.pos < len(p.tokens) {
		switch p.peek().Value {
		case "@":
			p.pos = p.skipAnnotation(p.pos)
		case "public", "protected", "private", "static", "final", "abstract", "synchronized", "native", "strictfp", "default", "transient", "volatile", "sealed", "non-sealed":
			p.pos++
		default:
			return
		}
	}
}

func stripAnnotationsAndModifiers(tokens []token) []token {
	out := make([]token, 0, len(tokens))
	for i := 0; i < len(tokens); {
		switch tokens[i].Value {
		case "@":
			i = skipAnnotationTokens(tokens, i)
		case "public", "protected", "private", "static", "final", "abstract", "transient", "volatile":
			i++
		default:
			out = append(out, tokens[i])
			i++
		}
	}
	return out
}

func skipAnnotationTokens(tokens []token, i int) int {
	i++
	if i < len(tokens) {
		i++
	}
	if i < len(tokens) && tokens[i].Value == "(" {
		if end, ok := findBalanced(tokens, i, "(", ")"); ok {
			return end + 1
		}
	}
	return i
}

func (p *parser) skipAnnotation(i int) int {
	return skipAnnotationTokens(p.tokens, i)
}

func parseParameters(tokens []token) []ParsedParam {
	if len(tokens) == 0 {
		return nil
	}
	parts := splitTopLevel(tokens, ",")
	params := make([]ParsedParam, 0, len(parts))
	for _, part := range parts {
		part = stripAnnotationsAndModifiers(part)
		if len(part) == 0 {
			continue
		}
		nameIndex := -1
		for i := len(part) - 1; i >= 0; i-- {
			if isSimpleIdentifier(part[i].Value) {
				nameIndex = i
				break
			}
		}
		if nameIndex <= 0 {
			continue
		}
		params = append(params, ParsedParam{
			Name: part[nameIndex].Value,
			Type: strings.TrimSpace(tokenText(part[:nameIndex])),
		})
	}
	return params
}

func analyzeMethodBody(body string, startLine int) (map[string]string, []CallSite) {
	localVars := make(map[string]string)
	callSites := make([]CallSite, 0, 8)

	lines := strings.Split(body, "\n")
	for idx, line := range lines {
		cleanLine := stripInlineComments(line)
		for _, match := range reVarDecl.FindAllStringSubmatch(cleanLine, -1) {
			if len(match) >= 3 {
				localVars[match[2]] = match[1]
			}
		}
		for _, match := range reCall.FindAllStringSubmatch(cleanLine, -1) {
			if len(match) < 3 {
				continue
			}
			name := match[2]
			if isControlKeyword(name) || name == "new" {
				continue
			}
			qualifier := strings.TrimSpace(match[1])
			callSites = append(callSites, CallSite{
				Name:      name,
				Qualifier: qualifier,
				ArgCount:  estimateArgCount(cleanLine, match[0]),
				Line:      startLine + idx,
				Raw:       strings.TrimSpace(cleanLine),
			})
		}
	}

	return localVars, callSites
}

func stripInlineComments(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func estimateArgCount(line, fragment string) int {
	callStart := strings.Index(line, fragment)
	if callStart < 0 {
		return -1
	}
	open := strings.Index(fragment, "(")
	if open < 0 {
		return -1
	}
	cursor := callStart + strings.Index(line[callStart:], "(")
	depth := 0
	args := 0
	hasToken := false
	for i := cursor; i < len(line); i++ {
		switch line[i] {
		case '(':
			depth++
		case ')':
			if depth == 1 {
				if hasToken {
					args++
				}
				return args
			}
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 1 {
				args++
				hasToken = false
			}
		case ' ', '\t', '\r', '\n':
		default:
			if depth == 1 {
				hasToken = true
			}
		}
	}
	return -1
}

func splitTopLevel(tokens []token, separator string) [][]token {
	parts := make([][]token, 0, 4)
	start := 0
	depthParen := 0
	depthAngle := 0
	depthBracket := 0
	for i, tok := range tokens {
		switch tok.Value {
		case "(":
			depthParen++
		case ")":
			if depthParen > 0 {
				depthParen--
			}
		case "<":
			depthAngle++
		case ">":
			if depthAngle > 0 {
				depthAngle--
			}
		case "[":
			depthBracket++
		case "]":
			if depthBracket > 0 {
				depthBracket--
			}
		}
		if tok.Value == separator && depthParen == 0 && depthAngle == 0 && depthBracket == 0 {
			parts = append(parts, tokens[start:i])
			start = i + 1
		}
	}
	parts = append(parts, tokens[start:])
	return parts
}

func findBalanced(tokens []token, start int, open, close string) (int, bool) {
	if start >= len(tokens) || tokens[start].Value != open {
		return 0, false
	}
	depth := 1
	for i := start + 1; i < len(tokens); i++ {
		switch tokens[i].Value {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func (p *parser) readTypeRef() string {
	start := p.pos
	depth := 0
	for p.pos < len(p.tokens) {
		switch p.peek().Value {
		case "<":
			depth++
			p.pos++
		case ">":
			if depth > 0 {
				depth--
			}
			p.pos++
		case ",":
			if depth == 0 {
				return strings.TrimSpace(tokenText(p.tokens[start:p.pos]))
			}
			p.pos++
		case "{", "implements", "extends":
			if depth == 0 {
				return strings.TrimSpace(tokenText(p.tokens[start:p.pos]))
			}
			p.pos++
		default:
			if depth == 0 && (p.peek().Value == "public" || p.peek().Value == "private" || p.peek().Value == "protected") {
				return strings.TrimSpace(tokenText(p.tokens[start:p.pos]))
			}
			p.pos++
		}
	}
	return strings.TrimSpace(tokenText(p.tokens[start:p.pos]))
}

func (p *parser) consume(value string) bool {
	if p.pos < len(p.tokens) && p.tokens[p.pos].Value == value {
		p.pos++
		return true
	}
	return false
}

func (p *parser) peek() token {
	if p.pos >= len(p.tokens) {
		return token{}
	}
	return p.tokens[p.pos]
}

func tokenText(tokens []token) string {
	var b strings.Builder
	for i, tok := range tokens {
		if tok.Value == "" {
			continue
		}
		if i > 0 && needSpace(tokens[i-1].Value, tok.Value) {
			b.WriteByte(' ')
		}
		b.WriteString(tok.Value)
	}
	return b.String()
}

func needSpace(prev, curr string) bool {
	if prev == "" || curr == "" {
		return false
	}
	leftTight := strings.ContainsAny(prev[len(prev)-1:], ".(<[@,")
	rightTight := strings.ContainsAny(curr[:1], ".>)];,@")
	if leftTight || rightTight {
		return false
	}
	if prev == "<" || curr == ">" || curr == "(" || prev == "[" || curr == "]" {
		return false
	}
	return true
}

func isControlKeyword(value string) bool {
	switch value {
	case "if", "for", "while", "switch", "catch", "try", "synchronized", "return", "throw":
		return true
	default:
		return false
	}
}

func isSimpleIdentifier(value string) bool {
	if value == "" || strings.Contains(value, ".") {
		return false
	}
	r := rune(value[0])
	if !isIdentStart(r) {
		return false
	}
	for _, ch := range value[1:] {
		if !isIdentPart(ch) {
			return false
		}
	}
	return true
}
