package javatracker

import "time"

type NodeKind string

const (
	NodeClass      NodeKind = "class"
	NodeInterface  NodeKind = "interface"
	NodeEnum       NodeKind = "enum"
	NodeMethod     NodeKind = "method"
	NodeField      NodeKind = "field"
	NodeStatement  NodeKind = "statement"
	NodeControl    NodeKind = "control"
	NodeVariable   NodeKind = "variable"
	NodeReturn     NodeKind = "return"
	NodeExpression NodeKind = "expression"
)

type EdgeKind string

const (
	EdgeHasMethod  EdgeKind = "HAS_METHOD"
	EdgeHasField   EdgeKind = "HAS_FIELD"
	EdgeCalls      EdgeKind = "CALLS"
	EdgeAccesses   EdgeKind = "ACCESSES"
	EdgeExtends    EdgeKind = "EXTENDS"
	EdgeImplements EdgeKind = "IMPLEMENTS"
	EdgeAST        EdgeKind = "AST"
	EdgeCFG        EdgeKind = "CFG"
	EdgePDG        EdgeKind = "PDG"
)

type Direction string

const (
	DirectionBoth       Direction = "both"
	DirectionUpstream   Direction = "upstream"
	DirectionDownstream Direction = "downstream"
)

type ProjectStats struct {
	IndexedAt    time.Time `json:"indexed_at"`
	DurationMS   int64     `json:"duration_ms"`
	JavaFiles    int       `json:"java_files"`
	Classes      int       `json:"classes"`
	Interfaces   int       `json:"interfaces"`
	Enums        int       `json:"enums"`
	Methods      int       `json:"methods"`
	Fields       int       `json:"fields"`
	CallEdges    int       `json:"call_edges"`
	AccessEdges  int       `json:"access_edges"`
	InheritEdges int       `json:"inherit_edges"`
}

type SearchItem struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Kind        NodeKind `json:"kind"`
	ClassName   string   `json:"class_name,omitempty"`
	FilePath    string   `json:"file_path,omitempty"`
	Line        int      `json:"line,omitempty"`
	Signature   string   `json:"signature,omitempty"`
	Description string   `json:"description,omitempty"`
}

type GraphNode struct {
	ID          string         `json:"id"`
	Label       string         `json:"label"`
	Kind        NodeKind       `json:"kind"`
	Lane        string         `json:"lane"`
	Depth       int            `json:"depth"`
	FilePath    string         `json:"file_path,omitempty"`
	Line        int            `json:"line,omitempty"`
	Signature   string         `json:"signature,omitempty"`
	Description string         `json:"description,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
}

type GraphEdge struct {
	Source string   `json:"source"`
	Target string   `json:"target"`
	Kind   EdgeKind `json:"kind"`
	Label  string   `json:"label,omitempty"`
}

type GraphResponse struct {
	ProjectRoot string       `json:"project_root"`
	FocusID     string       `json:"focus_id"`
	Direction   Direction    `json:"direction"`
	Depth       int          `json:"depth"`
	Truncated   bool         `json:"truncated"`
	Nodes       []GraphNode  `json:"nodes"`
	Edges       []GraphEdge  `json:"edges"`
	Summary     GraphSummary `json:"summary"`
}

type GraphSummary struct {
	NodeCount       int `json:"node_count"`
	EdgeCount       int `json:"edge_count"`
	UpstreamCount   int `json:"upstream_count"`
	DownstreamCount int `json:"downstream_count"`
	BodyCount       int `json:"body_count"`
}

type NodeDetails struct {
	ID          string         `json:"id"`
	Label       string         `json:"label"`
	Kind        NodeKind       `json:"kind"`
	FilePath    string         `json:"file_path,omitempty"`
	StartLine   int            `json:"start_line,omitempty"`
	EndLine     int            `json:"end_line,omitempty"`
	Signature   string         `json:"signature,omitempty"`
	Description string         `json:"description,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
	Source      []SourceLine   `json:"source,omitempty"`
}

type SourceLine struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

type NodeRef struct {
	ID   string
	Kind NodeKind
}

type ParsedFile struct {
	Path    string
	Package string
	Imports []string
	Classes []*ParsedClass
}

type ParsedClass struct {
	Name       string
	FullName   string
	Kind       NodeKind
	StartLine  int
	EndLine    int
	Extends    string
	Implements []string
	Methods    []*ParsedMethod
	Fields     []*ParsedField
}

type ParsedField struct {
	Name      string
	Type      string
	StartLine int
}

type ParsedParam struct {
	Name string
	Type string
}

type ParsedMethod struct {
	Name          string
	ReturnType    string
	Parameters    []ParsedParam
	StartLine     int
	EndLine       int
	Body          string
	CallSites     []CallSite
	LocalVarTypes map[string]string
}

type ClassInfo struct {
	ID          string
	Name        string
	FullName    string
	Kind        NodeKind
	Package     string
	FilePath    string
	StartLine   int
	EndLine     int
	Extends     string
	Implements  []string
	Imports     []string
	MethodIDs   []string
	FieldIDs    []string
	Description string
}

type FieldInfo struct {
	ID        string
	ClassID   string
	ClassName string
	Name      string
	Type      string
	FilePath  string
	StartLine int
}

type CallSite struct {
	Name      string
	Qualifier string
	ArgCount  int
	Line      int
	Raw       string
}

type MethodInfo struct {
	ID             string
	ClassID        string
	ClassName      string
	Name           string
	ReturnType     string
	Parameters     []ParsedParam
	ParamCount     int
	FilePath       string
	StartLine      int
	EndLine        int
	LocalVarTypes  map[string]string
	CallSites      []CallSite
	AccessedFields []string
	Description    string
}

type Project struct {
	Root              string
	Classes           map[string]*ClassInfo
	Fields            map[string]*FieldInfo
	Methods           map[string]*MethodInfo
	SearchItems       []SearchItem
	SearchByID        map[string]SearchItem
	Outgoing          map[string][]GraphEdge
	Incoming          map[string][]GraphEdge
	ClassBySimpleName map[string][]string
	MethodByName      map[string][]string
	MethodByClassName map[string][]string
	FileCache         *SourceCache
	Stats             ProjectStats
}

type IndexStatus struct {
	Running      bool         `json:"running"`
	Root         string       `json:"root,omitempty"`
	StartedAt    time.Time    `json:"started_at,omitempty"`
	FinishedAt   time.Time    `json:"finished_at,omitempty"`
	DurationMS   int64        `json:"duration_ms,omitempty"`
	FilesSeen    int          `json:"files_seen"`
	JavaFiles    int          `json:"java_files"`
	Error        string       `json:"error,omitempty"`
	ProjectReady bool         `json:"project_ready"`
	Stats        ProjectStats `json:"stats"`
}
