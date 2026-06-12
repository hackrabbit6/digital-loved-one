// Package grounding provides the grounding layer for pre-inference validation.
package grounding

import (
	"strings"
	"time"

	"digital-loved-one/backend/schema"
)

// Config holds configuration for the grounding layer.
type Config struct {
	SufficiencyThreshold         float64
	ConfidencePenaltyPerConflict float64
	MaxContextExcerpts           int
}

// DefaultConfig returns the default grounding configuration.
func DefaultConfig() Config {
	return Config{
		SufficiencyThreshold:         0.3,
		ConfidencePenaltyPerConflict: 0.15,
		MaxContextExcerpts:           10,
	}
}

// Layer validates memory context BEFORE LLM inference to enforce honesty.
type Layer struct {
	config Config
}

// NewLayer creates a new grounding layer.
func NewLayer(config Config) *Layer {
	if config.MaxContextExcerpts == 0 {
		config = DefaultConfig()
	}
	return &Layer{config: config}
}

// ValidateForInference is the main entry point: validate whether inference should proceed.
func (l *Layer) ValidateForInference(query string, context map[string]interface{}) *schema.ValidationResult {
	excerpts := l.getExcerpts(context)
	conflicts := l.getConflicts(context)

	// 1. Retrieval sufficiency check
	sufficient := l.checkSufficiency(query, excerpts)

	// 2. Contradiction surface check
	_, conflictMarkers := l.checkContradictions(conflicts, excerpts)

	// 3. Citation annotation
	citations := l.annotateCitations(excerpts)

	// Calculate confidence score
	confidence := l.calculateConfidence(excerpts, conflictMarkers)

	// Determine if blocked
	blocked := !sufficient

	// Create ambiguity flags
	ambiguityFlags := l.createAmbiguityFlags(excerpts, conflictMarkers, sufficient)

	return &schema.ValidationResult{
		Sufficient:      sufficient,
		Conflicts:       conflictMarkers,
		Citations:       citations,
		Blocked:         blocked,
		ConfidenceScore: confidence,
		AmbiguityFlags:  ambiguityFlags,
	}
}

// checkSufficiency checks if retrieved memory is sufficient to answer the query.
func (l *Layer) checkSufficiency(query string, excerpts []*schema.SourceExcerpt) bool {
	if len(excerpts) == 0 {
		return false
	}

	relevantCount := 0
	for _, e := range excerpts {
		relevance := l.calculateRelevance(query, e)
		if relevance >= l.config.SufficiencyThreshold {
			relevantCount++
		}
	}

	return relevantCount > 0
}

// checkContradictions checks if topic has unresolved conflicts that must be surfaced.
func (l *Layer) checkContradictions(conflicts []schema.ConflictMarker, excerpts []*schema.SourceExcerpt) (bool, []schema.ConflictMarker) {
	if len(conflicts) == 0 {
		return false, nil
	}

	var unresolved []schema.ConflictMarker
	for _, c := range conflicts {
		if c.Unresolved {
			unresolved = append(unresolved, c)
		}
	}

	// If conflicts involve current excerpts, surface them
	var relevantConflicts []schema.ConflictMarker
	excerptIDs := make(map[string]bool)
	for _, e := range excerpts {
		excerptIDs[e.ID] = true
	}

	for _, conflict := range unresolved {
		for _, eid := range conflict.ExcerptIDs {
			if excerptIDs[eid] {
				relevantConflicts = append(relevantConflicts, conflict)
				break
			}
		}
	}

	return len(relevantConflicts) > 0, relevantConflicts
}

// annotateCitations creates citation map for excerpts.
func (l *Layer) annotateCitations(excerpts []*schema.SourceExcerpt) map[string]string {
	citations := make(map[string]string)
	max := l.config.MaxContextExcerpts
	if max > len(excerpts) {
		max = len(excerpts)
	}

	for i := 0; i < max; i++ {
		e := excerpts[i]
		truncated := e.VerbatimText
		if len(truncated) > 100 {
			truncated = truncated[:100] + "..."
		}

		sourceType := e.SourceType
		date := "unknown"
		if d, ok := e.SourceMeta["date"].(string); ok {
			date = d
		}

		citations[e.ID] = sourceType + " " + date + `: "` + truncated + `"`
	}

	return citations
}

// calculateRelevance calculates relevance score between query and excerpt.
func (l *Layer) calculateRelevance(query string, excerpt *schema.SourceExcerpt) float64 {
	queryWords := l.tokenize(query)
	excerptWords := l.tokenize(excerpt.VerbatimText)

	if len(queryWords) == 0 || len(excerptWords) == 0 {
		return 0.0
	}

	overlap := 0
	for _, qw := range queryWords {
		for _, ew := range excerptWords {
			if qw == ew {
				overlap++
				break
			}
		}
	}

	return float64(overlap) / float64(len(queryWords))
}

func (l *Layer) tokenize(text string) []string {
	text = strings.ToLower(text)
	var words []string
	var current []rune

	for _, r := range text {
		// Chinese characters (unicode range for CJK)
		if r >= 0x4e00 && r <= 0x9fff || r >= 0x3400 && r <= 0x4dbf {
			if len(current) > 0 {
				words = append(words, string(current))
				current = nil
			}
			words = append(words, string(r))
		} else if r == ' ' || r == ',' || r == '.' || r == '!' || r == '?' {
			if len(current) > 0 {
				words = append(words, string(current))
				current = nil
			}
		} else {
			current = append(current, r)
		}
	}

	if len(current) > 0 {
		words = append(words, string(current))
	}

	return words
}

// calculateConfidence calculates overall confidence score.
func (l *Layer) calculateConfidence(excerpts []*schema.SourceExcerpt, conflicts []schema.ConflictMarker) float64 {
	var baseConf float64
	switch {
	case len(excerpts) == 0:
		return 0.0
	case len(excerpts) < 3:
		baseConf = 0.4
	case len(excerpts) < 10:
		baseConf = 0.7
	default:
		baseConf = 0.9
	}

	conflictPenalty := float64(len(conflicts)) * l.config.ConfidencePenaltyPerConflict
	return max(0.0, baseConf-conflictPenalty)
}

// createAmbiguityFlags creates ambiguity flags for the response.
func (l *Layer) createAmbiguityFlags(excerpts []*schema.SourceExcerpt, conflicts []schema.ConflictMarker, sufficient bool) []schema.AmbiguityFlag {
	var flags []schema.AmbiguityFlag

	if !sufficient {
		var ids []string
		for _, e := range excerpts {
			ids = append(ids, e.ID)
		}
		flags = append(flags, schema.AmbiguityFlag{
			ID:              newID(),
			TopicNodeID:     "unknown",
			Description:     "Insufficient memory to answer this query confidently",
			ConfidenceScore: 0.0,
			TriggeredBy:     ids,
			DetectedAt:      time.Now(),
		})
	}

	for _, conflict := range conflicts {
		if conflict.Unresolved {
			flags = append(flags, schema.AmbiguityFlag{
				ID:              newID(),
				TopicNodeID:     "unknown",
				Description:     "Unresolved conflict of type '" + conflict.Type + "' exists in memory",
				ConfidenceScore: 0.5,
				TriggeredBy:     conflict.ExcerptIDs,
				DetectedAt:      time.Now(),
			})
		}
	}

	return flags
}

// FormatGroundedPrompt builds a grounded prompt for the LLM.
func (l *Layer) FormatGroundedPrompt(query string, context map[string]interface{}, validation *schema.ValidationResult) string {
	excerpts := l.getExcerpts(context)
	citations := validation.Citations

	var prompt strings.Builder
	prompt.WriteString("Query: ")
	prompt.WriteString(query)
	prompt.WriteString("\n")

	if validation.Blocked {
		prompt.WriteString("我没有足够的记忆资料来回答这个问题。")
		return prompt.String()
	}

	prompt.WriteString("\nMemory context (with citations):\n")
	for _, e := range excerpts {
		if citation, ok := citations[e.ID]; ok {
			prompt.WriteString("- ")
			prompt.WriteString(citation)
			prompt.WriteString("\n")
		}
	}

	if len(validation.Conflicts) > 0 {
		prompt.WriteString("\n重要：以下记忆存在冲突：\n")
		for _, cm := range validation.Conflicts {
			prompt.WriteString("  - [")
			prompt.WriteString(cm.Type)
			prompt.WriteString("] 检测到冲突，回答时必须说明不确定性。\n")
		}
	}

	prompt.WriteString("\n指令：只根据上面的记忆回答，并且只用中文。")
	prompt.WriteString("如果不知道，就说“我不知道”。")
	prompt.WriteString("如果存在冲突，要诚实说明。")

	return prompt.String()
}

// === Helpers ===

func (l *Layer) getExcerpts(context map[string]interface{}) []*schema.SourceExcerpt {
	if v, ok := context["excerpts"]; ok {
		if excerpts, ok := v.([]*schema.SourceExcerpt); ok {
			return excerpts
		}
	}
	return nil
}

func (l *Layer) getConflicts(context map[string]interface{}) []schema.ConflictMarker {
	if v, ok := context["conflicts"]; ok {
		if conflicts, ok := v.([]schema.ConflictMarker); ok {
			return conflicts
		}
	}
	return nil
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func newID() string {
	return "amb_" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}
