// Package main provides the HTTP server for Digital Loved One backend.
package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"digital-loved-one/backend/grounding"
	"digital-loved-one/backend/ingestion"
	"digital-loved-one/backend/memory"
	"digital-loved-one/backend/schema"
)

type ChatRequest struct {
	Query     string `json:"query"`
	PersonaID string `json:"personaId"`
}

type ChatResponse struct {
	Text              string                 `json:"text"`
	Citations         []map[string]any       `json:"citations"`
	ConfidenceScore   float64                `json:"confidenceScore"`
	ConflictsSurfaced []string               `json:"conflictsSurfaced"`
	AmbiguityFlags    []schema.AmbiguityFlag `json:"ambiguityFlags"`
}

var ErrStoreUnavailable = errors.New("memory store is unavailable")

func main() {
	storePath := os.Getenv("STORE_PATH")
	if storePath == "" {
		storePath = "./data"
	}
	storeEngine := os.Getenv("STORE_ENGINE")

	store, err := memory.NewStore(storeEngine, storePath)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}

	config := grounding.DefaultConfig()
	groundingLayer := grounding.NewLayer(config)

	chatHandler := &ChatHandler{
		store:          store,
		groundingLayer: groundingLayer,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", chatHandler.HandleChat)
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/remember", handleRemember(store))
	mux.HandleFunc("/api/upload", handleUpload(store))

	handler := corsMiddleware(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Digital Loved One server starting on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

type ChatHandler struct {
	store          memory.Store
	groundingLayer *grounding.Layer
}

func (h *ChatHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.PersonaID == "" {
		req.PersonaID = "default"
	}

	if isMemoryReadQuery(req.Query) {
		topics, excerpts, err := loadPersonaMemory(h.store, req.PersonaID)
		if err != nil {
			http.Error(w, "Failed to read memory: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, buildMemoryReadResponse(topics, excerpts))
		return
	}

	if memoryText, memoryTopic, ok := extractRememberInstruction(req.Query); ok {
		excerpt, topic, err := saveConversationMemory(h.store, req.PersonaID, memoryText, "user", memoryTopic, "chat")
		if err != nil {
			http.Error(w, "Failed to save memory: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, ChatResponse{
			Text:            "我已经记住这条信息，归入 " + topic.TopicLabel + " 主题。",
			Citations:       formatExcerptCitations([]*schema.SourceExcerpt{excerpt}, 1),
			ConfidenceScore: 1,
		})
		return
	}

	var excerpts []*schema.SourceExcerpt
	var conflicts []schema.ConflictMarker
	var topics []*schema.TopicNode

	if h.store != nil {
		personaTopics, err := h.store.GetPersonaTopics(req.PersonaID)
		if err == nil && len(personaTopics) > 0 {
			topics = personaTopics
			for _, topic := range topics {
				topicExcerpts, err := h.store.GetExcerptsByIDs(topic.SourceExcerptIDs)
				if err == nil {
					excerpts = append(excerpts, topicExcerpts...)
				}
			}

			for _, topic := range topics {
				topicConflicts, err := h.store.GetTopicConflicts(topic.ID)
				if err == nil {
					conflicts = append(conflicts, topicConflicts...)
				}
			}
		}
	}

	excerpts = uniqueExcerpts(excerpts)

	context := map[string]interface{}{
		"persona_id": req.PersonaID,
		"excerpts":   excerpts,
		"conflicts":  conflicts,
	}

	validation := h.groundingLayer.ValidateForInference(req.Query, context)

	resp := ChatResponse{
		Text:              buildResponseText(req.Query, validation),
		Citations:         formatCitations(validation.Citations),
		ConfidenceScore:   validation.ConfidenceScore,
		ConflictsSurfaced: formatConflictsSurfaced(validation.Conflicts),
		AmbiguityFlags:    validation.AmbiguityFlags,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func buildResponseText(query string, validation *schema.ValidationResult) string {
	if validation.Blocked || !validation.Sufficient {
		return "我现在没有足够的记忆资料来有把握地回答这个问题。你可以先上传相关资料，或在对话里说“请记住：……”来补充记忆。"
	}

	if len(validation.Conflicts) > 0 {
		return "根据我保存的记忆，这个问题涉及一些互相冲突的信息。我会先说明不确定性，再基于已有记忆回答。"
	}

	return "根据我保存的记忆，我能看到一些相关线索，但现在还没有接入真正的生成模型，所以只能先给出引用和置信度。"
}

func isMemoryReadQuery(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return false
	}

	directPatterns := []string{
		"你记住了什么",
		"你记得什么",
		"记住了什么",
		"读取我的记忆",
		"查看我的记忆",
		"列出我的记忆",
		"show memories",
		"list memories",
		"read my memory",
		"what do you remember",
	}
	for _, pattern := range directPatterns {
		if strings.Contains(q, pattern) {
			return true
		}
	}

	hasMemoryWord := strings.Contains(q, "记忆") || strings.Contains(q, "memory") || strings.Contains(q, "memories")
	if !hasMemoryWord {
		return false
	}

	readVerbs := []string{"读取", "查看", "看看", "列出", "展示", "显示", "有哪些", "有什么", "全部", "所有", "show", "list", "read"}
	for _, verb := range readVerbs {
		if strings.Contains(q, verb) {
			return true
		}
	}
	return false
}

func extractRememberInstruction(query string) (string, string, bool) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return "", "", false
	}

	prefixes := []string{
		"请你记住：",
		"请你记住:",
		"请你记住",
		"请记住：",
		"请记住:",
		"请记住",
		"帮我记住：",
		"帮我记住:",
		"帮我记住",
		"记一下：",
		"记一下:",
		"记一下",
		"记住：",
		"记住:",
		"记住",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			text := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
			text = strings.TrimLeft(text, "：:，,。 ")
			if text == "" || text == "了什么" {
				return "", "", false
			}
			return text, "", true
		}
	}

	lower := strings.ToLower(trimmed)
	englishPrefixes := []string{
		"please remember that ",
		"please remember:",
		"please remember: ",
		"remember that ",
		"remember:",
		"remember: ",
	}
	for _, prefix := range englishPrefixes {
		if strings.HasPrefix(lower, prefix) {
			text := strings.TrimSpace(trimmed[len(prefix):])
			if text == "" {
				return "", "", false
			}
			return text, "", true
		}
	}

	return "", "", false
}

func buildMemoryReadResponse(topics []*schema.TopicNode, excerpts []*schema.SourceExcerpt) ChatResponse {
	if len(excerpts) == 0 {
		return ChatResponse{
			Text:            "我现在还没有保存任何记忆。你可以在左侧“对话记忆”里写一条内容并点击“记住”，也可以上传文本文件。",
			Citations:       []map[string]any{},
			ConfidenceScore: 0,
		}
	}

	topicByID := make(map[string]string, len(topics))
	for _, topic := range topics {
		topicByID[topic.ID] = topic.TopicLabel
	}

	var lines []string
	lines = append(lines, "我现在保存了这些记忆：")
	for i, excerpt := range excerpts {
		if i >= 12 {
			lines = append(lines, "...")
			break
		}
		topic := excerptTopicLabel(excerpt, topicByID)
		lines = append(lines, "- ["+topic+"] "+truncateRunes(excerpt.VerbatimText, 120))
	}

	return ChatResponse{
		Text:            strings.Join(lines, "\n"),
		Citations:       formatExcerptCitations(excerpts, 12),
		ConfidenceScore: 1,
	}
}

func excerptTopicLabel(excerpt *schema.SourceExcerpt, topicByID map[string]string) string {
	for _, topicID := range excerpt.TopicNodeIDs {
		if topic, ok := topicByID[topicID]; ok && topic != "" {
			return topic
		}
	}
	if topic, ok := excerpt.SourceMeta["topic"].(string); ok && topic != "" {
		return topic
	}
	return "conversation"
}

func uniqueExcerpts(excerpts []*schema.SourceExcerpt) []*schema.SourceExcerpt {
	seen := make(map[string]bool, len(excerpts))
	var result []*schema.SourceExcerpt
	for _, excerpt := range excerpts {
		if excerpt == nil || seen[excerpt.ID] {
			continue
		}
		seen[excerpt.ID] = true
		result = append(result, excerpt)
	}
	return result
}

func loadPersonaMemory(store memory.Store, personaID string) ([]*schema.TopicNode, []*schema.SourceExcerpt, error) {
	if store == nil {
		return nil, nil, nil
	}
	topics, err := store.GetPersonaTopics(personaID)
	if err != nil {
		return nil, nil, err
	}

	var excerpts []*schema.SourceExcerpt
	for _, topic := range topics {
		topicExcerpts, err := store.GetExcerptsByIDs(topic.SourceExcerptIDs)
		if err != nil {
			continue
		}
		excerpts = append(excerpts, topicExcerpts...)
	}
	return topics, uniqueExcerpts(excerpts), nil
}

func formatExcerptCitations(excerpts []*schema.SourceExcerpt, max int) []map[string]any {
	if max > len(excerpts) {
		max = len(excerpts)
	}
	citations := make([]map[string]any, 0, max)
	for i := 0; i < max; i++ {
		excerpt := excerpts[i]
		sourceType := excerpt.SourceType
		date := "unknown"
		if d, ok := excerpt.SourceMeta["date"].(string); ok && d != "" {
			date = d
		}
		citations = append(citations, map[string]any{
			"id":   excerpt.ID,
			"text": sourceType + " " + date + `: "` + truncateRunes(excerpt.VerbatimText, 100) + `"`,
		})
	}
	return citations
}

func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

func formatCitations(citations map[string]string) []map[string]any {
	var result []map[string]any
	for id, text := range citations {
		result = append(result, map[string]any{
			"id":   id,
			"text": text,
		})
	}
	return result
}

func formatConflictsSurfaced(conflicts []schema.ConflictMarker) []string {
	var result []string
	for _, c := range conflicts {
		result = append(result, "注意：我的记忆里有一个尚未解决的"+c.Type+"冲突。")
	}
	return result
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			return
		}

		next.ServeHTTP(w, r)
	})
}

type UploadRequest struct {
	PersonaID  string `json:"personaId"`
	SourceType string `json:"sourceType"`
}

type RememberRequest struct {
	PersonaID string `json:"personaId"`
	Text      string `json:"text"`
	Speaker   string `json:"speaker"`
	Topic     string `json:"topic"`
}

func handleRemember(store memory.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req RememberRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		req.Text = strings.TrimSpace(req.Text)
		if req.Text == "" {
			http.Error(w, "Text is required", http.StatusBadRequest)
			return
		}
		if req.PersonaID == "" {
			req.PersonaID = "default"
		}
		if req.Speaker == "" {
			req.Speaker = "user"
		}
		if req.Topic == "" {
			req.Topic = inferConversationTopic(req.Text)
		}

		excerpt, topic, err := saveConversationMemory(store, req.PersonaID, req.Text, req.Speaker, req.Topic, "frontend")
		if err != nil {
			http.Error(w, "Failed to save memory: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success":   true,
			"excerptId": excerpt.ID,
			"topicId":   topic.ID,
			"topic":     topic.TopicLabel,
			"personaId": req.PersonaID,
		})
	}
}

func saveConversationMemory(store memory.Store, personaID, text, speaker, topicLabel, platform string) (*schema.SourceExcerpt, *schema.TopicNode, error) {
	if store == nil {
		return nil, nil, ErrStoreUnavailable
	}
	text = strings.TrimSpace(text)
	if personaID == "" {
		personaID = "default"
	}
	if speaker == "" {
		speaker = "user"
	}
	if topicLabel == "" {
		topicLabel = inferConversationTopic(text)
	}
	if platform == "" {
		platform = "frontend"
	}

	now := time.Now()
	excerpt := &schema.SourceExcerpt{
		ID:           "excerpt_" + shortID(),
		PersonaID:    personaID,
		TopicNodeIDs: []string{},
		VerbatimText: text,
		SourceType:   "conversation",
		SourceMeta: map[string]any{
			"date":     now.Format(time.RFC3339),
			"speaker":  speaker,
			"topic":    topicLabel,
			"platform": platform,
		},
		CreatedAt: now,
	}

	if err := store.CreateExcerpt(excerpt); err != nil {
		return nil, nil, err
	}

	topic, err := upsertConversationTopic(store, excerpt, topicLabel, now)
	if err != nil {
		return nil, nil, err
	}

	excerpt.TopicNodeIDs = []string{topic.ID}
	if err := store.UpdateExcerpt(excerpt); err != nil {
		return nil, nil, err
	}
	return excerpt, topic, nil
}

func handleUpload(store memory.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "No file provided", http.StatusBadRequest)
			return
		}
		defer file.Close()

		personaID := r.FormValue("personaId")
		if personaID == "" {
			personaID = "default"
		}

		sourceType := r.FormValue("sourceType")
		if sourceType == "" {
			sourceType = "text"
		}

		// Save file temporarily
		tmpDir := os.TempDir()
		tmpFile := filepath.Join(tmpDir, header.Filename)
		out, err := os.Create(tmpFile)
		if err != nil {
			http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
			return
		}
		defer os.Remove(tmpFile)
		defer out.Close()

		if _, err := io.Copy(out, file); err != nil {
			http.Error(w, "Failed to save file", http.StatusInternalServerError)
			return
		}

		// Process through ingestion pipeline
		pipeline := ingestion.NewPipeline(store)
		excerpts, err := pipeline.IngestData(tmpFile, personaID, sourceType)
		if err != nil {
			http.Error(w, "Failed to process file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success":    true,
			"excerpts":   len(excerpts),
			"personaId":  personaID,
			"sourceType": sourceType,
		})
	}
}

func upsertConversationTopic(store memory.Store, excerpt *schema.SourceExcerpt, topicLabel string, now time.Time) (*schema.TopicNode, error) {
	topics, err := store.GetPersonaTopics(excerpt.PersonaID)
	if err != nil {
		return nil, err
	}

	for _, topic := range topics {
		if topic.TopicLabel == topicLabel {
			topic.SourceExcerptIDs = appendUniqueString(topic.SourceExcerptIDs, excerpt.ID)
			if topic.CoreSummary == "" {
				topic.CoreSummary = summarizeConversationMemory(excerpt.VerbatimText)
			}
			if err := store.UpdateTopic(topic); err != nil {
				return nil, err
			}
			return topic, nil
		}
	}

	topic := &schema.TopicNode{
		ID:               "topic_" + shortID(),
		PersonaID:        excerpt.PersonaID,
		TopicLabel:       topicLabel,
		CoreSummary:      summarizeConversationMemory(excerpt.VerbatimText),
		SummaryVersion:   1,
		SourceExcerptIDs: []string{excerpt.ID},
		ConflictMarkers:  []schema.ConflictMarker{},
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := store.CreateTopic(topic); err != nil {
		return nil, err
	}
	return topic, nil
}

func inferConversationTopic(text string) string {
	lower := strings.ToLower(text)
	keywords := map[string][]string{
		"family":       {"妈妈", "母亲", "爸爸", "父亲", "家人", "family", "mom", "dad"},
		"work":         {"工作", "职业", "公司", "work", "job", "career"},
		"health":       {"健康", "生病", "医院", "health", "sick", "hospital"},
		"preference":   {"喜欢", "讨厌", "爱吃", "偏好", "like", "love", "hate"},
		"relationship": {"关系", "朋友", "伴侣", "friend", "relationship"},
	}
	for topic, words := range keywords {
		for _, word := range words {
			if strings.Contains(lower, strings.ToLower(word)) {
				return topic
			}
		}
	}
	return "conversation"
}

func summarizeConversationMemory(text string) string {
	if len(text) > 500 {
		return text[:500] + "..."
	}
	return text
}

func appendUniqueString(items []string, item string) []string {
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func shortID() string {
	return uuid.NewString()[:12]
}
