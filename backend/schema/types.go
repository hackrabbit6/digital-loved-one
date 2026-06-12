// Package memory provides core data types for the Digital Loved One system.
package schema

import (
	"time"
)

// Time is a wrapper around time.Time for backwards compatibility.
type Time = time.Time

// VoiceDescriptors holds voice characteristics extracted from audio samples.
type VoiceDescriptors struct {
	PitchRangeHz        [2]float64 // (min, max)
	TempoRangeWPM      [2]float64 // words per minute
	TimbreDescription  string
	CharacteristicPhrases []string
}

// SpeechPatterns captures speech and language patterns for persona reconstruction.
type SpeechPatterns struct {
	VocabularyProfile        map[string]int
	SentenceStructurePatterns []string
	CharacteristicPhrases    []string
	ReasoningStyle           string // e.g., "analytical", "emotional", "pragmatic"
}

// PersonaProfile is the root entity representing the deceased person.
type PersonaProfile struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	VoiceDescriptors VoiceDescriptors `json:"voice_descriptors"`
	SpeechPatterns   SpeechPatterns   `json:"speech_patterns"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// ConflictMarker marks a contradiction in the person's recorded history.
type ConflictMarker struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"` // "factual" or "belief"
	ExcerptIDs    []string  `json:"excerpt_ids"`
	Unresolved    bool      `json:"unresolved"`
	ResolutionNote *string  `json:"resolution_note,omitempty"`
	DetectedAt    time.Time `json:"detected_at"`
}

// AmbiguityFlag flags areas of uncertainty in the memory.
type AmbiguityFlag struct {
	ID              string    `json:"id"`
	TopicNodeID     string    `json:"topic_node_id"`
	Description     string    `json:"description"`
	ConfidenceScore float64   `json:"confidence_score"` // 0.0-1.0
	TriggeredBy     []string  `json:"triggered_by"`     // source excerpt IDs
	DetectedAt      time.Time `json:"detected_at"`
}

// TopicNode represents a topic area in the person's memory.
type TopicNode struct {
	ID               string           `json:"id"`
	PersonaID        string           `json:"persona_id"`
	TopicLabel       string           `json:"topic_label"` // e.g., "father_son_relationship", "career"
	CoreSummary      string           `json:"core_summary"` // compressed; intentionally ambiguous when sources disagree
	SummaryVersion   int              `json:"summary_version"`
	SourceExcerptIDs []string         `json:"source_excerpt_ids"`
	ConflictMarkers  []ConflictMarker `json:"conflict_markers"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

// SourceExcerpt is an exact original text or transcript.
type SourceExcerpt struct {
	ID             string            `json:"id"`
	PersonaID      string            `json:"persona_id"`
	TopicNodeIDs    []string          `json:"topic_node_ids"`
	VerbatimText    string            `json:"verbatim_text"`
	SourceType      string            `json:"source_type"` // "chat", "audio", "video", "text", "photo"
	SourceMeta      map[string]any   `json:"source_meta"` // platform, date, speaker/account
	AudioTimestamp  *[2]float64      `json:"audio_timestamp,omitempty"` // (start, end) in seconds
	CreatedAt       time.Time         `json:"created_at"`
}

// MemoryUpdateResult is the result of processing new data.
type MemoryUpdateResult struct {
	NewExcerpts      []SourceExcerpt   `json:"new_excerpts"`
	NewTopics       []TopicNode       `json:"new_topics"`
	UpdatedTopics   []TopicNode       `json:"updated_topics"`
	ConflictsDetected []ConflictMarker `json:"conflicts_detected"`
	AmbiguitiesFlagged []AmbiguityFlag `json:"ambiguities_flagged"`
}

// MemoryDelta tracks changes between memory versions.
type MemoryDelta struct {
	OldVersion    int                    `json:"old_version"`
	NewVersion    int                    `json:"new_version"`
	AddedExcerpts []SourceExcerpt        `json:"added_excerpts"`
	UpdatedTopics [][3]string           `json:"updated_topics"` // (topic_id, old_summary, new_summary)
	NewConflicts  []ConflictMarker       `json:"new_conflicts"`
}

// ValidationResult is the result of grounding layer validation.
type ValidationResult struct {
	Sufficient      bool             `json:"sufficient"`
	Conflicts       []ConflictMarker `json:"conflicts"`
	Citations       map[string]string `json:"citations"` // excerpt_id -> citation_text
	Blocked         bool             `json:"blocked"` // if true, caller should return "I don't know"
	ConfidenceScore float64          `json:"confidence_score"`
	AmbiguityFlags  []AmbiguityFlag  `json:"ambiguity_flags"`
}

// ChatResponse is the response from a chat interaction.
type ChatResponse struct {
	Text              string           `json:"text"`
	Citations          []map[string]any `json:"citations"`
	ConfidenceScore    float64          `json:"confidence_score"`
	ConflictsSurfaced  []string         `json:"conflicts_surfaced"`
	AmbiguityFlags    []AmbiguityFlag  `json:"ambiguity_flags"`
}