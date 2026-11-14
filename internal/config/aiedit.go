package config

import (
	"errors"
	"sort"
	"time"
)

const aiEditDocumentCap = 512

// AIEditDocument is one document the person allowed AI editing for, in a folder
// where the AI Editing switch alone is not enough: a team folder, or a machine
// whose team record cannot be read. Scope is what the document was decided
// about, so the allow stops applying when the folder becomes another team's.
// There is no stored refusal: taking the allow away removes the row, and the
// document asks again.
type AIEditDocument struct {
	Document  string `json:"document"`
	Scope     Scope  `json:"scope"`
	DecidedAt int64  `json:"decidedAt"`
}

func aiEditDocumentLess(a, b AIEditDocument) bool {
	if a.DecidedAt != b.DecidedAt {
		return a.DecidedAt < b.DecidedAt
	}
	return a.Document < b.Document
}

func sortAIEditDocuments(rows []AIEditDocument) {
	sort.SliceStable(rows, func(i, j int) bool { return aiEditDocumentLess(rows[i], rows[j]) })
}

func validAIEditDocument(d AIEditDocument) bool {
	return d.Document != "" && (d.Scope.Kind == ScopeTeam || d.Scope.Kind == ScopeUnknown)
}

// AllowAIEditDocument records the person's allow for one document in scope,
// replacing the row the document already had. It returns that row so a failed
// save can put it back. A local scope is refused: a document in a folder of this
// machine's own needs no row, and one must never be read as a team's answer. A
// full list drops its oldest row, whose document then asks again.
func (c *Config) AllowAIEditDocument(document string, scope Scope) (previous AIEditDocument, had bool, err error) {
	row := AIEditDocument{Document: document, Scope: scope, DecidedAt: time.Now().UnixNano()}
	if !validAIEditDocument(row) {
		return AIEditDocument{}, false, errors.New("AI editing is allowed per document only in a team or unknown scope")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, existing := range c.AIEditDocuments {
		if existing.Document == document {
			c.AIEditDocuments[i] = row
			sortAIEditDocuments(c.AIEditDocuments)
			return existing, true, nil
		}
	}
	if len(c.AIEditDocuments) >= aiEditDocumentCap {
		c.AIEditDocuments = c.AIEditDocuments[1:]
	}
	c.AIEditDocuments = append(c.AIEditDocuments, row)
	sortAIEditDocuments(c.AIEditDocuments)
	return AIEditDocument{}, false, nil
}

// AIEditDocumentAllowed reports whether the person allowed AI editing for
// document in exactly this scope.
func (c *Config) AIEditDocumentAllowed(document string, scope Scope) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, d := range c.AIEditDocuments {
		if d.Document == document {
			return d.Scope == scope
		}
	}
	return false
}

// ForgetAIEditDocument removes a document's allow, so it asks again.
func (c *Config) ForgetAIEditDocument(document string) (AIEditDocument, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, d := range c.AIEditDocuments {
		if d.Document == document {
			c.AIEditDocuments = append(c.AIEditDocuments[:i], c.AIEditDocuments[i+1:]...)
			return d, true
		}
	}
	return AIEditDocument{}, false
}

// RestoreAIEditDocument puts back a row a failed save removed or replaced.
func (c *Config) RestoreAIEditDocument(d AIEditDocument) bool {
	if !validAIEditDocument(d) {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.AIEditDocuments) >= aiEditDocumentCap {
		return false
	}
	for _, existing := range c.AIEditDocuments {
		if existing.Document == d.Document {
			return false
		}
	}
	c.AIEditDocuments = append(c.AIEditDocuments, d)
	sortAIEditDocuments(c.AIEditDocuments)
	return true
}

func (c *Config) AIEditDocumentList() []AIEditDocument {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]AIEditDocument(nil), c.AIEditDocuments...)
}

// normalizeAIEditDocuments drops malformed rows, keeps the newest row of a
// document listed twice, and trims the list to its cap, oldest first.
func (c *Config) normalizeAIEditDocuments() (dropped int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := make([]AIEditDocument, 0, len(c.AIEditDocuments))
	index := make(map[string]int, len(c.AIEditDocuments))
	for _, d := range c.AIEditDocuments {
		if !validAIEditDocument(d) {
			dropped++
			continue
		}
		if i, duplicate := index[d.Document]; duplicate {
			dropped++
			if d.DecidedAt > rows[i].DecidedAt {
				rows[i] = d
			}
			continue
		}
		index[d.Document] = len(rows)
		rows = append(rows, d)
	}
	sortAIEditDocuments(rows)
	if over := len(rows) - aiEditDocumentCap; over > 0 {
		dropped += over
		rows = rows[over:]
	}
	c.AIEditDocuments = rows
	return dropped
}
