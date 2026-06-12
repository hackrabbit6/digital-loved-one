package memory

import "digital-loved-one/backend/schema"

// Store is the persistence boundary for local persona memory.
//
// The default implementation is GraphStore, a JSON-file store used for local
// development. ObjectBoxStore can implement this interface for device-side
// persistence without changing ingestion, grounding, or persona code.
type Store interface {
	CreatePersona(persona *schema.PersonaProfile) error
	GetPersona(personaID string) (*schema.PersonaProfile, error)
	UpdatePersona(persona *schema.PersonaProfile) error

	CreateExcerpt(excerpt *schema.SourceExcerpt) error
	GetExcerpt(excerptID string) (*schema.SourceExcerpt, error)
	GetExcerptsByIDs(excerptIDs []string) ([]*schema.SourceExcerpt, error)
	UpdateExcerpt(excerpt *schema.SourceExcerpt) error

	CreateTopic(topic *schema.TopicNode) error
	GetTopic(topicID string) (*schema.TopicNode, error)
	UpdateTopic(topic *schema.TopicNode) error
	AttachConflictToTopic(topicID string, conflict *schema.ConflictMarker) error
	ResolveConflict(topicID, conflictID, resolutionNote string) error
	GetTopicConflicts(topicID string) ([]schema.ConflictMarker, error)
	GetPersonaTopics(personaID string) ([]*schema.TopicNode, error)
	GetRelevantExcerpts(topicLabel string, threshold float64) ([]*schema.SourceExcerpt, error)
	GetAllTopics() ([]*schema.TopicNode, error)
}
