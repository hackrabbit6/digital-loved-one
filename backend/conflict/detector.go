// Package conflict provides conflict detection for factual and belief conflicts.
package conflict

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"digital-loved-one/backend/schema"
)

// Detector detects contradictions in the person's recorded history.
type Detector struct {
	similarityThreshold float64
	temporalWindowHours float64
}

// NewDetector creates a new conflict detector.
func NewDetector(similarityThreshold, temporalWindowHours float64) *Detector {
	return &Detector{
		similarityThreshold: similarityThreshold,
		temporalWindowHours: temporalWindowHours,
	}
}

// DetectConflicts detects all conflicts in a list of excerpts.
func (d *Detector) DetectConflicts(excerpts []*schema.SourceExcerpt) []*schema.ConflictMarker {
	var conflicts []*schema.ConflictMarker

	// Detect factual conflicts: same speaker, opposite statements
	factual := d.detectFactualConflicts(excerpts)
	conflicts = append(conflicts, factual...)

	// Detect belief conflicts: different speakers disagreeing about person
	belief := d.detectBeliefConflicts(excerpts)
	conflicts = append(conflicts, belief...)

	return conflicts
}

func (d *Detector) detectFactualConflicts(excerpts []*schema.SourceExcerpt) []*schema.ConflictMarker {
	var conflicts []*schema.ConflictMarker
	speakers := groupBySpeaker(excerpts)

	for _, speakerExcerpts := range speakers {
		for i, e1 := range speakerExcerpts {
			for _, e2 := range speakerExcerpts[i+1:] {
				if areSemanticallyOpposite(e1.VerbatimText, e2.VerbatimText) {
					conflict := &schema.ConflictMarker{
						ID:         uuid.New().String(),
						Type:       "factual",
						ExcerptIDs: []string{e1.ID, e2.ID},
						Unresolved: true,
						DetectedAt: time.Now(),
					}
					conflicts = append(conflicts, conflict)
				}
			}
		}
	}
	return conflicts
}

func (d *Detector) detectBeliefConflicts(excerpts []*schema.SourceExcerpt) []*schema.ConflictMarker {
	var conflicts []*schema.ConflictMarker
	topics := groupByTopic(excerpts)

	for _, topicExcerpts := range topics {
		speakers := groupBySpeaker(topicExcerpts)
		if len(speakers) < 2 {
			continue
		}

		speakerIDs := make([]string, 0, len(speakers))
		for id := range speakers {
			speakerIDs = append(speakerIDs, id)
		}

		for i := 0; i < len(speakerIDs); i++ {
			for j := i + 1; j < len(speakerIDs); j++ {
				excerpts1 := speakers[speakerIDs[i]]
				excerpts2 := speakers[speakerIDs[j]]

				if haveOpposingViews(excerpts1, excerpts2) {
					var excerptIDs []string
					if len(excerpts1) > 0 {
						excerptIDs = append(excerptIDs, excerpts1[0].ID)
					}
					if len(excerpts2) > 0 {
						excerptIDs = append(excerptIDs, excerpts2[0].ID)
					}

					conflict := &schema.ConflictMarker{
						ID:         uuid.New().String(),
						Type:       "belief",
						ExcerptIDs: excerptIDs,
						Unresolved: true,
						DetectedAt: time.Now(),
					}
					conflicts = append(conflicts, conflict)
				}
			}
		}
	}
	return conflicts
}

// CalculateConfidenceScore calculates confidence score penalizing conflicts.
func (d *Detector) CalculateConfidenceScore(markers []schema.ConflictMarker) float64 {
	if len(markers) == 0 {
		return 1.0
	}

	unresolvedCount := 0
	for _, m := range markers {
		if m.Unresolved {
			unresolvedCount++
		}
	}

	penalty := float64(unresolvedCount) * 0.15
	return max(0.0, 1.0-penalty)
}

// CreateAmbiguityFlag creates an ambiguity flag for topics with conflicting info.
func (d *Detector) CreateAmbiguityFlag(topicNodeID, description string, triggeredBy []string) *schema.AmbiguityFlag {
	return &schema.AmbiguityFlag{
		ID:              uuid.New().String(),
		TopicNodeID:     topicNodeID,
		Description:     description,
		ConfidenceScore: 0.5,
		TriggeredBy:     triggeredBy,
		DetectedAt:      time.Now(),
	}
}

// === Helpers ===

func groupBySpeaker(excerpts []*schema.SourceExcerpt) map[string][]*schema.SourceExcerpt {
	groups := make(map[string][]*schema.SourceExcerpt)
	for _, e := range excerpts {
		speaker := getSpeaker(e)
		groups[speaker] = append(groups[speaker], e)
	}
	return groups
}

func groupByTopic(excerpts []*schema.SourceExcerpt) map[string][]*schema.SourceExcerpt {
	groups := make(map[string][]*schema.SourceExcerpt)
	for _, e := range excerpts {
		topic := getTopic(e)
		groups[topic] = append(groups[topic], e)
	}
	return groups
}

func getSpeaker(e *schema.SourceExcerpt) string {
	if speaker, ok := e.SourceMeta["speaker"].(string); ok {
		return speaker
	}
	if account, ok := e.SourceMeta["account"].(string); ok {
		return account
	}
	return "unknown"
}

func getTopic(e *schema.SourceExcerpt) string {
	if topic, ok := e.SourceMeta["topic"].(string); ok {
		return topic
	}
	return "general"
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
		if strings.Contains(strings.ToLower(text1), neg) && strings.Contains(strings.ToLower(text2), neg) {
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

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}