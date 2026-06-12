// Package ingestion provides data ingestion pipeline.
package ingestion

import (
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"digital-loved-one/backend/conflict"
	"digital-loved-one/backend/ingestion/parsers"
	"digital-loved-one/backend/memory"
	"digital-loved-one/backend/schema"
)

// Pipeline orchestrates data parsing, extraction, and storage.
type Pipeline struct {
	store       memory.Store
	conflictDet *conflict.Detector
	parsers     map[string]parsers.Parser
}

// NewPipeline creates a new ingestion pipeline.
func NewPipeline(store memory.Store) *Pipeline {
	return &Pipeline{
		store:       store,
		conflictDet: conflict.NewDetector(0.7, 24.0),
		parsers: map[string]parsers.Parser{
			"whatsapp": &parsers.WhatsAppParser{},
			"wechat":   &parsers.WeChatParser{},
			"audio":    &parsers.AudioParser{},
			"text":     &parsers.TextParser{},
		},
	}
}

// IngestData ingests a data file and creates excerpts.
func (p *Pipeline) IngestData(filePath, personaID, sourceType string) ([]*schema.SourceExcerpt, error) {
	// 1. Detect format and parse
	formatType := sourceType
	if formatType == "" {
		formatType = detectFormat(filePath)
	}

	parser, ok := p.parsers[formatType]
	if !ok {
		parser = p.parsers["text"]
	}

	parsedData, err := parser.Parse(filePath)
	if err != nil {
		return nil, err
	}

	// 2. Extract excerpts
	excerpts := p.extractExcerpts(parsedData, personaID, formatType)

	// 3. Store excerpts
	for _, e := range excerpts {
		if err := p.store.CreateExcerpt(e); err != nil {
			// Skip if already exists
		}
	}

	// 4. Run conflict detection
	conflicts := p.conflictDet.DetectConflicts(excerpts)

	// 5. Update topics with excerpts
	p.updateTopics(excerpts, conflicts)

	return excerpts, nil
}

func (p *Pipeline) extractExcerpts(parsed []map[string]interface{}, personaID, sourceType string) []*schema.SourceExcerpt {
	var excerpts []*schema.SourceExcerpt

	for _, item := range parsed {
		text, _ := item["text"].(string)
		if text == "" {
			continue
		}

		date, _ := item["date"].(string)
		speaker, _ := item["speaker"].(string)
		topic, _ := item["topic"].(string)

		excerpts = append(excerpts, &schema.SourceExcerpt{
			ID:           newExcerptID(),
			PersonaID:    personaID,
			TopicNodeIDs: []string{},
			VerbatimText: text,
			SourceType:   sourceType,
			SourceMeta: map[string]any{
				"date":     date,
				"speaker":  speaker,
				"topic":    topic,
				"platform": sourceType,
			},
			CreatedAt: now(),
		})
	}

	return excerpts
}

func (p *Pipeline) updateTopics(excerpts []*schema.SourceExcerpt, conflicts []*schema.ConflictMarker) {
	// Group excerpts by topic
	topicGroups := make(map[string][]*schema.SourceExcerpt)
	for _, e := range excerpts {
		topic := "general"
		if t, ok := e.SourceMeta["topic"].(string); ok {
			topic = t
		}
		topicGroups[topic] = append(topicGroups[topic], e)
	}

	// Create or update topics
	for topicLabel, topicExcerpts := range topicGroups {
		topics, _ := p.store.GetAllTopics()
		var existing *schema.TopicNode

		for _, t := range topics {
			if t.TopicLabel == topicLabel && t.PersonaID == topicExcerpts[0].PersonaID {
				existing = t
				break
			}
		}

		if existing != nil {
			// Update existing topic
			for _, e := range topicExcerpts {
				existing.SourceExcerptIDs = append(existing.SourceExcerptIDs, e.ID)
				e.TopicNodeIDs = appendUnique(e.TopicNodeIDs, existing.ID)
				_ = p.store.UpdateExcerpt(e)
			}
			for _, c := range conflicts {
				if conflictInvolvesAny(c, existing.SourceExcerptIDs) {
					existing.ConflictMarkers = appendConflictOnce(existing.ConflictMarkers, *c)
				}
			}
			p.store.UpdateTopic(existing)
		} else {
			// Create new topic
			var excerptIDs []string
			for _, e := range topicExcerpts {
				excerptIDs = append(excerptIDs, e.ID)
			}

			topic := &schema.TopicNode{
				ID:               newTopicID(),
				PersonaID:        topicExcerpts[0].PersonaID,
				TopicLabel:       topicLabel,
				CoreSummary:      buildSummary(topicExcerpts),
				SummaryVersion:   1,
				SourceExcerptIDs: excerptIDs,
				ConflictMarkers:  conflictsForExcerpts(conflicts, excerptIDs),
				CreatedAt:        now(),
				UpdatedAt:        now(),
			}
			p.store.CreateTopic(topic)
			for _, e := range topicExcerpts {
				e.TopicNodeIDs = appendUnique(e.TopicNodeIDs, topic.ID)
				_ = p.store.UpdateExcerpt(e)
			}
		}
	}
}

func detectFormat(filePath string) string {
	ext := filepath.Ext(filePath)

	formatMap := map[string]string{
		".json": "whatsapp",
		".txt":  "text",
		".csv":  "text",
		".mp3":  "audio",
		".mp4":  "audio",
		".m4a":  "audio",
		".wav":  "audio",
	}

	if format, ok := formatMap[ext]; ok {
		return format
	}

	return "text"
}

func buildSummary(excerpts []*schema.SourceExcerpt) string {
	if len(excerpts) == 0 {
		return ""
	}

	// Simple: combine first few texts
	text := excerpts[0].VerbatimText
	if len(text) > 500 {
		return text[:500] + "..."
	}
	return text
}

func newExcerptID() string {
	return "excerpt_" + randomID(12)
}

func newTopicID() string {
	return "topic_" + randomID(12)
}

func randomID(n int) string {
	id := uuid.NewString()
	if n > 0 && n < len(id) {
		return id[:n]
	}
	return id
}

func now() schema.Time {
	return time.Now()
}

func appendUnique(ids []string, id string) []string {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func conflictsForExcerpts(conflicts []*schema.ConflictMarker, excerptIDs []string) []schema.ConflictMarker {
	var result []schema.ConflictMarker
	for _, c := range conflicts {
		if conflictInvolvesAny(c, excerptIDs) {
			result = appendConflictOnce(result, *c)
		}
	}
	return result
}

func conflictInvolvesAny(conflict *schema.ConflictMarker, excerptIDs []string) bool {
	excerptSet := make(map[string]bool, len(excerptIDs))
	for _, id := range excerptIDs {
		excerptSet[id] = true
	}
	for _, id := range conflict.ExcerptIDs {
		if excerptSet[id] {
			return true
		}
	}
	return false
}

func appendConflictOnce(conflicts []schema.ConflictMarker, conflict schema.ConflictMarker) []schema.ConflictMarker {
	for _, existing := range conflicts {
		if existing.ID == conflict.ID {
			return conflicts
		}
	}
	return append(conflicts, conflict)
}
