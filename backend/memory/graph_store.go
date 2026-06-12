// Package memory provides append-only persistence for the Digital Loved One system.
package memory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"digital-loved-one/backend/schema"
)

// GraphStore is an append-only memory store with JSON file persistence.
type GraphStore struct {
	basePath     string
	mu           sync.RWMutex
	personasPath string
	excerptsPath string
	topicsPath   string
}

// NewGraphStore creates a new GraphStore at the specified base path.
func NewGraphStore(basePath string) (*GraphStore, error) {
	gs := &GraphStore{
		basePath:     basePath,
		personasPath: filepath.Join(basePath, "personas"),
		excerptsPath: filepath.Join(basePath, "excerpts"),
		topicsPath:   filepath.Join(basePath, "topics"),
	}

	// Ensure directories exist
	for _, p := range []string{gs.personasPath, gs.excerptsPath, gs.topicsPath} {
		if err := os.MkdirAll(p, 0755); err != nil {
			return nil, err
		}
	}

	return gs, nil
}

// === PersonaProfile CRUD ===

// CreatePersona creates a new persona profile. Fails if already exists.
func (gs *GraphStore) CreatePersona(persona *schema.PersonaProfile) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	filePath := gs.personaFile(persona.ID)
	if _, err := os.Stat(filePath); err == nil {
		return ErrAlreadyExists
	}

	persona.CreatedAt = time.Now()
	persona.UpdatedAt = time.Now()
	return gs.writeJSON(filePath, persona)
}

// GetPersona retrieves a persona by ID.
func (gs *GraphStore) GetPersona(personaID string) (*schema.PersonaProfile, error) {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	filePath := gs.personaFile(personaID)
	data, err := gs.readJSON(filePath)
	if err != nil {
		return nil, err
	}

	var persona schema.PersonaProfile
	if err := json.Unmarshal(data, &persona); err != nil {
		return nil, err
	}
	return &persona, nil
}

// UpdatePersona updates an existing persona.
func (gs *GraphStore) UpdatePersona(persona *schema.PersonaProfile) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	filePath := gs.personaFile(persona.ID)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return ErrNotFound
	}

	persona.UpdatedAt = time.Now()
	return gs.writeJSON(filePath, persona)
}

// === SourceExcerpt CRUD ===

// CreateExcerpt creates a new source excerpt.
func (gs *GraphStore) CreateExcerpt(excerpt *schema.SourceExcerpt) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	filePath := gs.excerptFile(excerpt.ID)
	if _, err := os.Stat(filePath); err == nil {
		return ErrAlreadyExists
	}

	excerpt.CreatedAt = time.Now()
	return gs.writeJSON(filePath, excerpt)
}

// GetExcerpt retrieves an excerpt by ID.
func (gs *GraphStore) GetExcerpt(excerptID string) (*schema.SourceExcerpt, error) {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	filePath := gs.excerptFile(excerptID)
	data, err := gs.readJSON(filePath)
	if err != nil {
		return nil, err
	}

	var excerpt schema.SourceExcerpt
	if err := json.Unmarshal(data, &excerpt); err != nil {
		return nil, err
	}
	return &excerpt, nil
}

// GetExcerptsByIDs retrieves multiple excerpts by their IDs.
func (gs *GraphStore) GetExcerptsByIDs(excerptIDs []string) ([]*schema.SourceExcerpt, error) {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	var excerpts []*schema.SourceExcerpt
	for _, id := range excerptIDs {
		filePath := gs.excerptFile(id)
		data, err := gs.readJSON(filePath)
		if err != nil {
			continue
		}
		var excerpt schema.SourceExcerpt
		if json.Unmarshal(data, &excerpt) == nil {
			excerpts = append(excerpts, &excerpt)
		}
	}
	return excerpts, nil
}

// UpdateExcerpt updates an existing source excerpt.
func (gs *GraphStore) UpdateExcerpt(excerpt *schema.SourceExcerpt) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	filePath := gs.excerptFile(excerpt.ID)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return ErrNotFound
	}

	return gs.writeJSON(filePath, excerpt)
}

// === TopicNode CRUD ===

// CreateTopic creates a new topic node.
func (gs *GraphStore) CreateTopic(topic *schema.TopicNode) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	filePath := gs.topicFile(topic.ID)
	if _, err := os.Stat(filePath); err == nil {
		return ErrAlreadyExists
	}

	topic.CreatedAt = time.Now()
	topic.UpdatedAt = time.Now()
	return gs.writeJSON(filePath, topic)
}

// GetTopic retrieves a topic by ID.
func (gs *GraphStore) GetTopic(topicID string) (*schema.TopicNode, error) {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	filePath := gs.topicFile(topicID)
	data, err := gs.readJSON(filePath)
	if err != nil {
		return nil, err
	}

	var topic schema.TopicNode
	if err := json.Unmarshal(data, &topic); err != nil {
		return nil, err
	}
	return &topic, nil
}

// UpdateTopic updates a topic. Creates new version (append-only principle).
func (gs *GraphStore) UpdateTopic(topic *schema.TopicNode) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	filePath := gs.topicFile(topic.ID)

	// Archive old version
	if _, err := os.Stat(filePath); err == nil {
		data, _ := gs.readJSON(filePath)
		if data != nil {
			archivePath := gs.topicFile(topic.ID + "_v" + versionString(topic.SummaryVersion))
			os.WriteFile(archivePath, data, 0644)
		}
	}

	topic.SummaryVersion++
	topic.UpdatedAt = time.Now()
	return gs.writeJSON(filePath, topic)
}

// AttachConflictToTopic attaches a conflict marker to a topic.
func (gs *GraphStore) AttachConflictToTopic(topicID string, conflict *schema.ConflictMarker) error {
	topic, err := gs.GetTopic(topicID)
	if err != nil {
		return err
	}

	topic.ConflictMarkers = append(topic.ConflictMarkers, *conflict)
	return gs.UpdateTopic(topic)
}

// ResolveConflict marks a conflict as resolved with family adjudication.
func (gs *GraphStore) ResolveConflict(topicID, conflictID, resolutionNote string) error {
	topic, err := gs.GetTopic(topicID)
	if err != nil {
		return err
	}

	for i := range topic.ConflictMarkers {
		if topic.ConflictMarkers[i].ID == conflictID {
			topic.ConflictMarkers[i].Unresolved = false
			topic.ConflictMarkers[i].ResolutionNote = &resolutionNote
			break
		}
	}

	return gs.UpdateTopic(topic)
}

// GetTopicConflicts returns all unresolved conflict markers for a topic.
func (gs *GraphStore) GetTopicConflicts(topicID string) ([]schema.ConflictMarker, error) {
	topic, err := gs.GetTopic(topicID)
	if err != nil {
		return nil, err
	}

	var unresolved []schema.ConflictMarker
	for _, cm := range topic.ConflictMarkers {
		if cm.Unresolved {
			unresolved = append(unresolved, cm)
		}
	}
	return unresolved, nil
}

// GetPersonaTopics returns all topics for a persona.
func (gs *GraphStore) GetPersonaTopics(personaID string) ([]*schema.TopicNode, error) {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	var topics []*schema.TopicNode
	entries, err := os.ReadDir(gs.topicsPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		// Skip archived versions
		if strings.Contains(entry.Name(), "_v") {
			continue
		}

		data, err := gs.readJSON(filepath.Join(gs.topicsPath, entry.Name()))
		if err != nil {
			continue
		}
		var topic schema.TopicNode
		if json.Unmarshal(data, &topic) == nil && topic.PersonaID == personaID {
			topics = append(topics, &topic)
		}
	}
	return topics, nil
}

// GetRelevantExcerpts returns excerpts relevant to a topic label.
func (gs *GraphStore) GetRelevantExcerpts(topicLabel string, threshold float64) ([]*schema.SourceExcerpt, error) {
	topics, err := gs.GetAllTopics()
	if err != nil {
		return nil, err
	}

	var relevant []*schema.SourceExcerpt
	for _, t := range topics {
		if labelSimilarity(topicLabel, t.TopicLabel) >= threshold {
			for _, eid := range t.SourceExcerptIDs {
				if e, err := gs.GetExcerpt(eid); err == nil {
					relevant = append(relevant, e)
				}
			}
		}
	}
	return relevant, nil
}

// GetAllTopics returns all topics.
func (gs *GraphStore) GetAllTopics() ([]*schema.TopicNode, error) {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	var topics []*schema.TopicNode
	entries, err := os.ReadDir(gs.topicsPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if strings.Contains(entry.Name(), "_v") {
			continue
		}

		data, err := gs.readJSON(filepath.Join(gs.topicsPath, entry.Name()))
		if err != nil {
			continue
		}
		var topic schema.TopicNode
		if json.Unmarshal(data, &topic) == nil {
			topics = append(topics, &topic)
		}
	}
	return topics, nil
}

// === Helpers ===

func (gs *GraphStore) personaFile(id string) string {
	return filepath.Join(gs.personasPath, id+".json")
}

func (gs *GraphStore) excerptFile(id string) string {
	return filepath.Join(gs.excerptsPath, id+".json")
}

func (gs *GraphStore) topicFile(id string) string {
	return filepath.Join(gs.topicsPath, id+".json")
}

func (gs *GraphStore) writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (gs *GraphStore) readJSON(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func labelSimilarity(label1, label2 string) float64 {
	words1 := strings.Split(strings.ToLower(label1), "_")
	words2 := strings.Split(strings.ToLower(label2), "_")

	set1 := make(map[string]bool)
	set2 := make(map[string]bool)
	for _, w := range words1 {
		set1[w] = true
	}
	for _, w := range words2 {
		set2[w] = true
	}

	var overlap float64
	for w := range set1 {
		if set2[w] {
			overlap++
		}
	}

	union := float64(len(set1) + len(set2) - int(overlap))
	if union == 0 {
		return 0.0
	}
	return overlap / union
}

func versionString(version int) string {
	return strconv.Itoa(version)
}

// Errors
var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)
