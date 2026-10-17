package lsp

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Noooblien/agentvcr/pkg/speculate"
)

// Document represents an in-memory editor buffer tracked by the LSP daemon.
type Document struct {
	URI        string
	LanguageID string
	Version    int
	Content    string
	Lines      []string
}

// DocumentStore manages synchronized document state across client open/change/close notifications.
type DocumentStore struct {
	mu   sync.RWMutex
	docs map[string]*Document
}

// NewDocumentStore initializes an empty thread-safe document store.
func NewDocumentStore() *DocumentStore {
	return &DocumentStore{
		docs: make(map[string]*Document),
	}
}

// Open stores a freshly opened text document buffer.
func (s *DocumentStore) Open(uri, languageID string, version int, text string) *Document {
	s.mu.Lock()
	defer s.mu.Unlock()

	lines := splitLines(text)
	doc := &Document{
		URI:        uri,
		LanguageID: languageID,
		Version:    version,
		Content:    text,
		Lines:      lines,
	}
	s.docs[uri] = doc
	return doc
}

// Change updates an existing document buffer with either full or incremental content changes.
func (s *DocumentStore) Change(uri string, version int, changes []TextDocumentContentChangeEvent) (*Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	doc, exists := s.docs[uri]
	if !exists {
		return nil, fmt.Errorf("document not found: %s", uri)
	}

	doc.Version = version

	for _, change := range changes {
		if change.Range == nil {
			// Full document update
			doc.Content = change.Text
			doc.Lines = splitLines(change.Text)
		} else {
			// Incremental update within range
			doc.Content = applyIncrementalChange(doc.Content, doc.Lines, change)
			doc.Lines = splitLines(doc.Content)
		}
	}

	return doc, nil
}

// Close removes a document from active tracking.
func (s *DocumentStore) Close(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, uri)
}

// Get retrieves a pointer to a tracked document.
func (s *DocumentStore) Get(uri string) (*Document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.docs[uri]
	return doc, ok
}

// Count returns the number of active buffers currently maintained.
func (s *DocumentStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.docs)
}

// ExtractASTContext extracts the syntactic context and cursor position for speculate.ASTPredictor.
func (s *DocumentStore) ExtractASTContext(uri string, pos Position) (*speculate.ASTContext, *speculate.CursorPosition, error) {
	s.mu.RLock()
	doc, exists := s.docs[uri]
	s.mu.RUnlock()

	if !exists {
		return nil, nil, fmt.Errorf("document not open: %s", uri)
	}

	lineIdx := pos.Line
	charIdx := pos.Character

	if lineIdx < 0 || lineIdx >= len(doc.Lines) {
		lineIdx = len(doc.Lines) - 1
		if lineIdx < 0 {
			lineIdx = 0
		}
	}

	lang := normalizeLanguage(doc.LanguageID, uri)

	// Gather leading lines (up to 8 lines preceding cursor)
	startLine := lineIdx - 8
	if startLine < 0 {
		startLine = 0
	}
	var leading []string
	if len(doc.Lines) > 0 && lineIdx < len(doc.Lines) {
		leading = append(leading, doc.Lines[startLine:lineIdx]...)
		// Include current line up to character
		currLine := doc.Lines[lineIdx]
		if charIdx <= len(currLine) {
			leading = append(leading, currLine[:charIdx])
		} else {
			leading = append(leading, currLine)
		}
	}

	// Gather trailing lines (up to 4 lines following cursor)
	endLine := lineIdx + 5
	if endLine > len(doc.Lines) {
		endLine = len(doc.Lines)
	}
	var trailing []string
	if lineIdx+1 < len(doc.Lines) {
		trailing = append(trailing, doc.Lines[lineIdx+1:endLine]...)
	}

	// Determine indentation level
	indent := 0
	if lineIdx < len(doc.Lines) {
		line := doc.Lines[lineIdx]
		for _, r := range line {
			if r == '\t' {
				indent++
			} else if r == ' ' {
				indent++
			} else {
				break
			}
		}
	}

	astCtx := &speculate.ASTContext{
		Language:      lang,
		LeadingLines:  leading,
		TrailingLines: trailing,
		IndentLevel:   indent,
	}

	cursorPos := &speculate.CursorPosition{
		FilePath: uri,
		Line:     pos.Line + 1,      // convert to 1-indexed for ASTPredictor
		Column:   pos.Character + 1, // convert to 1-indexed
	}

	return astCtx, cursorPos, nil
}

func splitLines(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(normalized, "\n")
}

func applyIncrementalChange(content string, lines []string, change TextDocumentContentChangeEvent) string {
	r := change.Range
	if r == nil {
		return change.Text
	}

	// Compute start and end offsets in content
	startOffset := getOffset(lines, r.Start)
	endOffset := getOffset(lines, r.End)

	if startOffset > len(content) {
		startOffset = len(content)
	}
	if endOffset > len(content) {
		endOffset = len(content)
	}
	if startOffset > endOffset {
		startOffset = endOffset
	}

	return content[:startOffset] + change.Text + content[endOffset:]
}

func getOffset(lines []string, pos Position) int {
	offset := 0
	for i := 0; i < pos.Line && i < len(lines); i++ {
		offset += len(lines[i]) + 1 // +1 for newline
	}
	offset += pos.Character
	return offset
}

func normalizeLanguage(languageID, uri string) string {
	if languageID != "" {
		l := strings.ToLower(languageID)
		switch l {
		case "go", "golang":
			return "go"
		case "typescript", "typescriptreact", "ts", "tsx":
			return "typescript"
		case "python", "py":
			return "python"
		case "javascript", "javascriptreact", "js", "jsx":
			return "javascript"
		case "rust", "rs":
			return "rust"
		default:
			return l
		}
	}

	ext := strings.ToLower(filepath.Ext(uri))
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".js", ".jsx":
		return "javascript"
	case ".rs":
		return "rust"
	default:
		return "text"
	}
}
