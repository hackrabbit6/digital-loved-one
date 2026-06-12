//go:build objectbox

package memory

import (
	"errors"

	"digital-loved-one/backend/schema"
)

// ObjectBoxStore is the ObjectBox-backed implementation of Store.
//
// This file is intentionally behind the "objectbox" build tag because
// ObjectBox Go requires generated binding code and native runtime libraries.
// Keep GraphStore as the default development store, then fill this adapter once
// ObjectBox code generation is part of the build.
type ObjectBoxStore struct {
	path string
}

// NewObjectBoxStore creates an ObjectBox-backed memory store.
func NewObjectBoxStore(path string) (*ObjectBoxStore, error) {
	if path == "" {
		return nil, errors.New("objectbox path is required")
	}
	return &ObjectBoxStore{path: path}, nil
}

func (s *ObjectBoxStore) CreatePersona(persona *schema.PersonaProfile) error {
	return errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) GetPersona(personaID string) (*schema.PersonaProfile, error) {
	return nil, errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) UpdatePersona(persona *schema.PersonaProfile) error {
	return errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) CreateExcerpt(excerpt *schema.SourceExcerpt) error {
	return errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) GetExcerpt(excerptID string) (*schema.SourceExcerpt, error) {
	return nil, errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) GetExcerptsByIDs(excerptIDs []string) ([]*schema.SourceExcerpt, error) {
	return nil, errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) UpdateExcerpt(excerpt *schema.SourceExcerpt) error {
	return errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) CreateTopic(topic *schema.TopicNode) error {
	return errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) GetTopic(topicID string) (*schema.TopicNode, error) {
	return nil, errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) UpdateTopic(topic *schema.TopicNode) error {
	return errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) AttachConflictToTopic(topicID string, conflict *schema.ConflictMarker) error {
	return errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) ResolveConflict(topicID, conflictID, resolutionNote string) error {
	return errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) GetTopicConflicts(topicID string) ([]schema.ConflictMarker, error) {
	return nil, errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) GetPersonaTopics(personaID string) ([]*schema.TopicNode, error) {
	return nil, errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) GetRelevantExcerpts(topicLabel string, threshold float64) ([]*schema.SourceExcerpt, error) {
	return nil, errObjectBoxNotGenerated()
}

func (s *ObjectBoxStore) GetAllTopics() ([]*schema.TopicNode, error) {
	return nil, errObjectBoxNotGenerated()
}

func errObjectBoxNotGenerated() error {
	return errors.New("objectbox adapter requires generated ObjectBox bindings")
}
