// Package memory provides incremental learning for memory updates.
package memory

import (
	"strings"
	"time"

	"digital-loved-one/backend/conflict"
	"digital-loved-one/backend/schema"
)

// IncrementalLearner handles delta extraction and memory integration.
type IncrementalLearner struct {
	store       Store
	conflictDet *conflict.Detector
}

// NewIncrementalLearner creates a new incremental learner.
func NewIncrementalLearner(store Store, similarityThreshold, temporalWindowHours float64) *IncrementalLearner {
	return &IncrementalLearner{
		store:       store,
		conflictDet: conflict.NewDetector(similarityThreshold, temporalWindowHours),
	}
}

// LearnResult contains the result of an incremental learning operation.
type LearnResult struct {
	NewExcerpts   []*schema.SourceExcerpt
	UpdatedTopics []*schema.TopicNode
	NewConflicts  []*schema.ConflictMarker
	Errors        []error
}

// Learn processes new excerpts and integrates them into existing schema.
func (l *IncrementalLearner) Learn(newExcerpts []*schema.SourceExcerpt) *LearnResult {
	result := &LearnResult{
		NewExcerpts:   []*schema.SourceExcerpt{},
		UpdatedTopics: []*schema.TopicNode{},
		NewConflicts:  []*schema.ConflictMarker{},
		Errors:        []error{},
	}

	if len(newExcerpts) == 0 {
		return result
	}

	// 1. Get existing topics for each persona
	personaTopics := make(map[string][]*schema.TopicNode)
	personaExcerpts := make(map[string][]*schema.SourceExcerpt)

	for _, e := range newExcerpts {
		topics, err := l.store.GetPersonaTopics(e.PersonaID)
		if err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		personaTopics[e.PersonaID] = topics

		// Collect all existing excerpts for conflict detection
		var existing []*schema.SourceExcerpt
		for _, t := range topics {
			excerpts, err := l.store.GetExcerptsByIDs(t.SourceExcerptIDs)
			if err != nil {
				continue
			}
			existing = append(existing, excerpts...)
		}
		personaExcerpts[e.PersonaID] = existing
	}

	// 2. Create new excerpts
	for _, e := range newExcerpts {
		if err := l.store.CreateExcerpt(e); err != nil {
			// Skip if already exists
			result.Errors = append(result.Errors, err)
			continue
		}
		result.NewExcerpts = append(result.NewExcerpts, e)
	}

	// 3. Detect conflicts between new and existing excerpts
	for personaID, existing := range personaExcerpts {
		for _, newE := range newExcerpts {
			if newE.PersonaID != personaID {
				continue
			}
			// Check for conflicts with existing excerpts
			conflicts := l.detectConflictsWithExisting(newE, existing)
			result.NewConflicts = append(result.NewConflicts, conflicts...)
		}
	}

	// 4. Update topics with new excerpts
	for personaID, topics := range personaTopics {
		for _, topic := range topics {
			// Find new excerpts for this topic
			var topicNewExcerpts []*schema.SourceExcerpt
			for _, e := range newExcerpts {
				if e.PersonaID != personaID {
					continue
				}
				if l.topicMatchesExcerpt(topic, e) {
					topicNewExcerpts = append(topicNewExcerpts, e)
				}
			}

			if len(topicNewExcerpts) > 0 {
				// Update existing topic
				for _, e := range topicNewExcerpts {
					topic.SourceExcerptIDs = append(topic.SourceExcerptIDs, e.ID)
				}
				topic.SummaryVersion++
				if err := l.store.UpdateTopic(topic); err != nil {
					result.Errors = append(result.Errors, err)
				} else {
					result.UpdatedTopics = append(result.UpdatedTopics, topic)
				}

				// Attach any new conflicts to this topic
				for _, c := range result.NewConflicts {
					if l.conflictInvolvesTopic(c, topic.SourceExcerptIDs) {
						l.store.AttachConflictToTopic(topic.ID, c)
					}
				}
			}
		}

		// 5. Create new topics for new excerpt topics not covered by existing
		existingTopicLabels := make(map[string]bool)
		for _, t := range topics {
			existingTopicLabels[t.TopicLabel] = true
		}

		for _, e := range newExcerpts {
			if e.PersonaID != personaID {
				continue
			}
			topicLabel := l.extractTopicLabel(e)
			if !existingTopicLabels[topicLabel] {
				// Create new topic
				topic := &schema.TopicNode{
					ID:               newTopicID(),
					PersonaID:        personaID,
					TopicLabel:       topicLabel,
					CoreSummary:      buildSummary([]*schema.SourceExcerpt{e}),
					SummaryVersion:   1,
					SourceExcerptIDs: []string{e.ID},
					ConflictMarkers:  []schema.ConflictMarker{},
					CreatedAt:        now(),
					UpdatedAt:        now(),
				}
				if err := l.store.CreateTopic(topic); err != nil {
					result.Errors = append(result.Errors, err)
				} else {
					result.UpdatedTopics = append(result.UpdatedTopics, topic)
				}
				existingTopicLabels[topicLabel] = true
			}
		}
	}

	return result
}

func (l *IncrementalLearner) detectConflictsWithExisting(new *schema.SourceExcerpt, existing []*schema.SourceExcerpt) []*schema.ConflictMarker {
	var conflicts []*schema.ConflictMarker

	// Same speaker conflicts (factual)
	speaker := getSpeakerFromMeta(new)
	for _, e := range existing {
		if getSpeakerFromMeta(e) != speaker {
			continue
		}
		if areSemanticallyOpposite(new.VerbatimText, e.VerbatimText) {
			conflicts = append(conflicts, &schema.ConflictMarker{
				ID:         newConflictID(),
				Type:       "factual",
				ExcerptIDs: []string{new.ID, e.ID},
				Unresolved: true,
				DetectedAt: now(),
			})
		}
	}

	// Belief conflicts (different speakers on same topic)
	topic := getTopicFromMeta(new)
	if topic == "" {
		topic = "general"
	}

	var sameTopicExcerpts []*schema.SourceExcerpt
	for _, e := range existing {
		if getTopicFromMeta(e) == topic {
			sameTopicExcerpts = append(sameTopicExcerpts, e)
		}
	}

	if len(sameTopicExcerpts) >= 1 {
		// Different speakers with opposing views
		existingSpeakers := make(map[string]bool)
		for _, e := range sameTopicExcerpts {
			existingSpeakers[getSpeakerFromMeta(e)] = true
		}

		if len(existingSpeakers) >= 1 && !existingSpeakers[speaker] {
			if haveOpposingViews([]*schema.SourceExcerpt{new}, sameTopicExcerpts) {
				conflicts = append(conflicts, &schema.ConflictMarker{
					ID:         newConflictID(),
					Type:       "belief",
					ExcerptIDs: []string{new.ID},
					Unresolved: true,
					DetectedAt: now(),
				})
			}
		}
	}

	return conflicts
}

func (l *IncrementalLearner) topicMatchesExcerpt(topic *schema.TopicNode, excerpt *schema.SourceExcerpt) bool {
	topicLabel := topic.TopicLabel
	excerptTopic := l.extractTopicLabel(excerpt)

	if topicLabel == excerptTopic {
		return true
	}

	// Check word overlap
	topicWords := splitWords(topicLabel)
	excerptWords := splitWords(excerptTopic)

	overlap := 0
	for _, tw := range topicWords {
		for _, ew := range excerptWords {
			if tw == ew {
				overlap++
				break
			}
		}
	}

	return overlap > 0 && overlap >= min(len(topicWords), len(excerptWords))/2
}

func (l *IncrementalLearner) extractTopicLabel(excerpt *schema.SourceExcerpt) string {
	if topic, ok := excerpt.SourceMeta["topic"].(string); ok && topic != "" {
		return topic
	}
	return "general"
}

func (l *IncrementalLearner) conflictInvolvesTopic(c *schema.ConflictMarker, excerptIDs []string) bool {
	topicExcerptSet := make(map[string]bool)
	for _, id := range excerptIDs {
		topicExcerptSet[id] = true
	}

	for _, eid := range c.ExcerptIDs {
		if topicExcerptSet[eid] {
			return true
		}
	}
	return false
}

// === Helpers ===

func getSpeakerFromMeta(e *schema.SourceExcerpt) string {
	if speaker, ok := e.SourceMeta["speaker"].(string); ok {
		return speaker
	}
	if account, ok := e.SourceMeta["account"].(string); ok {
		return account
	}
	return "unknown"
}

func getTopicFromMeta(e *schema.SourceExcerpt) string {
	if topic, ok := e.SourceMeta["topic"].(string); ok {
		return topic
	}
	return ""
}

func areSemanticallyOpposite(text1, text2 string) bool {
	positives := []string{"yes", "good", "happy", "love", "proud", "agree", "want", "like", "enjoy"}
	negatives := []string{"no", "bad", "sad", "hate", "disagree", "don't want", "dislike", "not"}

	sent1 := sentiment(text1, positives, negatives)
	sent2 := sentiment(text2, positives, negatives)

	if sent1 != "" && sent2 != "" && sent1 != sent2 {
		return true
	}

	negationWords := []string{"never", "not", "don't", "didn't", "won't", "wouldn't", "can't", "couldn't"}
	for _, neg := range negationWords {
		if contains(strings.ToLower(text1), neg) && contains(strings.ToLower(text2), neg) {
			return true
		}
	}

	return false
}

func sentiment(text string, positives, negatives []string) string {
	textLower := strings.ToLower(text)
	for _, w := range positives {
		if strings.Contains(textLower, w) {
			return "positive"
		}
	}
	for _, w := range negatives {
		if strings.Contains(textLower, w) {
			return "negative"
		}
	}
	return ""
}

func haveOpposingViews(excerpts1, excerpts2 []*schema.SourceExcerpt) bool {
	text1 := combineTexts(excerpts1)
	text2 := combineTexts(excerpts2)
	return areSemanticallyOpposite(text1, text2)
}

func combineTexts(excerpts []*schema.SourceExcerpt) string {
	var parts []string
	for _, e := range excerpts {
		parts = append(parts, e.VerbatimText)
	}
	return strings.Join(parts, " ")
}

func splitWords(s string) []string {
	var words []string
	var current []rune
	for _, r := range s {
		if r == '_' || r == ' ' || r == '-' {
			if len(current) > 0 {
				words = append(words, strings.ToLower(string(current)))
				current = nil
			}
		} else {
			current = append(current, r)
		}
	}
	if len(current) > 0 {
		words = append(words, strings.ToLower(string(current)))
	}
	return words
}

func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func newConflictID() string {
	return "conflict_" + randomID(12)
}

func randomID(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}

func newTopicID() string {
	return "topic_" + randomID(12)
}

func buildSummary(excerpts []*schema.SourceExcerpt) string {
	if len(excerpts) == 0 {
		return ""
	}
	text := excerpts[0].VerbatimText
	if len(text) > 500 {
		return text[:500] + "..."
	}
	return text
}

func now() time.Time {
	return time.Now()
}
