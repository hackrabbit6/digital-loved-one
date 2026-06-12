// Package person provides the chat engine for interacting with virtual persons.
package person

import (
	"os"

	"digital-loved-one/backend/conflict"
	"digital-loved-one/backend/grounding"
	"digital-loved-one/backend/memory"
	"digital-loved-one/backend/schema"
)

// Engine is the main chat interface for interacting with a virtual person.
type Engine struct {
	store          memory.Store
	groundingLayer *grounding.Layer
	conflictDet    *conflict.Detector
}

// NewEngine creates a new chat engine.
func NewEngine(store memory.Store) *Engine {
	config := grounding.DefaultConfig()
	return &Engine{
		store:          store,
		groundingLayer: grounding.NewLayer(config),
		conflictDet:    conflict.NewDetector(0.7, 24.0),
	}
}

// Chat processes a chat query and returns a response.
func (e *Engine) Chat(query, personaID string) (*schema.ChatResponse, error) {
	// 1. Retrieve relevant excerpts from memory
	excerpts, err := e.retrieveExcerpts(query, personaID)
	if err != nil {
		return nil, err
	}

	// 2. Get any conflicts for the relevant topics
	conflicts := e.getTopicConflicts(excerpts)

	// 3. Run grounding layer validation
	context := map[string]interface{}{
		"excerpts":   excerpts,
		"persona_id": personaID,
		"conflicts":  conflicts,
	}
	validation := e.groundingLayer.ValidateForInference(query, context)

	// 4. If blocked (insufficient memory), return an honest fallback.
	if validation.Blocked {
		return &schema.ChatResponse{
			Text:              "我现在没有足够的记忆资料来回答这个问题。",
			Citations:         []map[string]any{},
			ConfidenceScore:   0.0,
			ConflictsSurfaced: []string{},
			AmbiguityFlags:    validation.AmbiguityFlags,
		}, nil
	}

	// 5. Build grounded prompt
	groundedPrompt := e.groundingLayer.FormatGroundedPrompt(query, context, validation)

	// 6. Call LLM with grounded prompt
	llmResponse := e.callLLM(groundedPrompt, personaID)

	// 7. Format citations for response
	formattedCitations := e.formatCitations(validation.Citations)

	// 8. Surface conflicts in response text
	conflictsSurfaced := e.surfaceConflicts(validation.Conflicts, llmResponse)

	return &schema.ChatResponse{
		Text:              llmResponse,
		Citations:         formattedCitations,
		ConfidenceScore:   validation.ConfidenceScore,
		ConflictsSurfaced: conflictsSurfaced,
		AmbiguityFlags:    validation.AmbiguityFlags,
	}, nil
}

func (e *Engine) retrieveExcerpts(query, personaID string) ([]*schema.SourceExcerpt, error) {
	topics, err := e.store.GetPersonaTopics(personaID)
	if err != nil {
		return nil, err
	}

	var relevant []*schema.SourceExcerpt
	for _, t := range topics {
		excerpts, err := e.store.GetRelevantExcerpts(t.TopicLabel, 0.4)
		if err != nil {
			continue
		}
		relevant = append(relevant, excerpts...)
	}

	// Limit to 20 excerpts
	if len(relevant) > 20 {
		relevant = relevant[:20]
	}
	return relevant, nil
}

func (e *Engine) getTopicConflicts(excerpts []*schema.SourceExcerpt) []schema.ConflictMarker {
	var conflicts []schema.ConflictMarker
	topicIDs := make(map[string]bool)

	for _, e := range excerpts {
		for _, tid := range e.TopicNodeIDs {
			topicIDs[tid] = true
		}
	}

	for topicID := range topicIDs {
		topicConflicts, err := e.store.GetTopicConflicts(topicID)
		if err != nil {
			continue
		}
		conflicts = append(conflicts, topicConflicts...)
	}

	return conflicts
}

func (e *Engine) callLLM(prompt, personaID string) string {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("CLAUDE_API_KEY")
	}

	if apiKey == "" {
		return e.stubResponse(prompt)
	}

	// In production, make actual API call here
	return e.stubResponse(prompt)
}

func (e *Engine) stubResponse(prompt string) string {
	if contains(prompt, "I don't have enough information") || contains(prompt, "没有足够") {
		return "我现在没有足够的记忆资料来回答这个问题。"
	}
	return "根据我保存的记忆，我能看到一些相关线索，但现在还没有接入真正的生成模型。"
}

func (e *Engine) formatCitations(citations map[string]string) []map[string]any {
	var formatted []map[string]any
	for excerptID, citation := range citations {
		parts := splitCitation(citation)
		formatted = append(formatted, map[string]any{
			"id":      excerptID,
			"source":  parts.source,
			"excerpt": parts.excerpt,
			"text":    citation,
		})
	}
	return formatted
}

type citationParts struct {
	source  string
	excerpt string
}

func splitCitation(citation string) citationParts {
	// Format: "[type date]: "text...""
	// Simple split on "]"
	for i, c := range citation {
		if c == ']' {
			return citationParts{
				source:  citation[:i],
				excerpt: citation[i+1:],
			}
		}
	}
	return citationParts{source: "unknown", excerpt: citation}
}

func (e *Engine) surfaceConflicts(conflicts []schema.ConflictMarker, response string) []string {
	var surfaced []string
	for _, cm := range conflicts {
		surfaced = append(surfaced, "注意：我的记忆里有一个尚未解决的"+cm.Type+"冲突。")
	}
	return surfaced
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
