// Package parsers provides data parsers for various formats.
package parsers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Parser defines the interface for data parsers.
type Parser interface {
	Parse(filePath string) ([]map[string]interface{}, error)
}

// WhatsAppParser parses WhatsApp chat export files.
type WhatsAppParser struct{}

func (p *WhatsAppParser) Parse(filePath string) ([]map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	ext := filepath.Ext(filePath)
	if ext == ".json" {
		return p.parseJSON(data)
	}
	return p.parseText(string(data))
}

func (p *WhatsAppParser) parseJSON(data []byte) ([]map[string]interface{}, error) {
	var messages []struct {
		Date    string `json:"date"`
		Sender  string `json:"sender"`
		Text    string `json:"text"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for _, m := range messages {
		text := m.Text
		if text == "" {
			text = m.Message
		}
		if text != "" {
			results = append(results, map[string]interface{}{
				"date":    m.Date,
				"speaker": m.Sender,
				"text":    text,
				"topic":   extractTopic(text),
			})
		}
	}
	return results, nil
}

func (p *WhatsAppParser) parseText(content string) ([]map[string]interface{}, error) {
	var results []map[string]interface{}
	lines := splitLines(content)

	for _, line := range lines {
		if line == "" || len(line) < 10 {
			continue
		}
		// Format: "date - sender: message" or "[date] sender: message"
		parsed := parseWhatsAppLine(line)
		if parsed != nil {
			results = append(results, parsed)
		}
	}
	return results, nil
}

// WeChatParser parses WeChat export files.
type WeChatParser struct{}

func (p *WeChatParser) Parse(filePath string) ([]map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var messages []struct {
		CreateTime string `json:"create_time"`
		Nickname   string `json:"nickname"`
		Content    string `json:"content"`
		Remark     string `json:"remark"`
	}

	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for _, m := range messages {
		if m.Content != "" {
			results = append(results, map[string]interface{}{
				"date":    m.CreateTime,
				"speaker": m.Nickname,
				"text":    m.Content,
				"topic":   extractTopic(m.Content),
			})
		}
	}
	return results, nil
}

// AudioParser parses audio transcription files.
type AudioParser struct{}

func (p *AudioParser) Parse(filePath string) ([]map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	ext := filepath.Ext(filePath)
	if ext == ".json" {
		var transcripts []struct {
			Timestamp string `json:"timestamp"`
			Speaker   string `json:"speaker"`
			Text      string `json:"text"`
		}
		if err := json.Unmarshal(data, &transcripts); err != nil {
			return nil, err
		}

		var results []map[string]interface{}
		for _, t := range transcripts {
			if t.Text != "" {
				results = append(results, map[string]interface{}{
					"date":    t.Timestamp,
					"speaker": t.Speaker,
					"text":    t.Text,
					"topic":   extractTopic(t.Text),
				})
			}
		}
		return results, nil
	}

	// Plain text transcription
	text := string(data)
	return []map[string]interface{}{
		{
			"date":    time.Now().Format(time.RFC3339),
			"speaker": "unknown",
			"text":    text,
			"topic":   extractTopic(text),
		},
	}, nil
}

// TextParser parses plain text files.
type TextParser struct{}

func (p *TextParser) Parse(filePath string) ([]map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	content := string(data)
	lines := splitLines(content)

	var results []map[string]interface{}
	for _, line := range lines {
		if line == "" || len(line) < 3 {
			continue
		}
		results = append(results, map[string]interface{}{
			"date":    "",
			"speaker": "unknown",
			"text":    line,
			"topic":   extractTopic(line),
		})
	}
	return results, nil
}

// === Helpers ===

func splitLines(content string) []string {
	var lines []string
	var current []rune
	for _, r := range []rune(content) {
		if r == '\n' {
			if len(current) > 0 {
				lines = append(lines, string(current))
				current = nil
			}
		} else {
			current = append(current, r)
		}
	}
	if len(current) > 0 {
		lines = append(lines, string(current))
	}
	return lines
}

func parseWhatsAppLine(line string) map[string]interface{} {
	// Simple pattern: "[date] sender: message" or "date - sender: message"
	colonIdx := -1
	for i, c := range line {
		if c == ':' && i+1 < len(line) {
			// Check if there's a sender pattern before colon
			prefix := line[:i]
			if len(prefix) > 5 && (prefix[len(prefix)-4:] == " - " || prefix[len(prefix)-1:] == "]") {
				colonIdx = i
				break
			}
		}
	}

	if colonIdx == -1 {
		return nil
	}

	before := line[:colonIdx]
	after := line[colonIdx+1:]

	// Extract speaker
	speaker := "unknown"
	if idx := findSenderPattern(before); idx != -1 {
		speaker = before[idx+1:]
	}

	// Extract date
	date := ""
	if len(before) > 0 && before[0] == '[' {
		if end := findMatchingBracket(before); end != -1 {
			date = before[1:end]
		}
	}

	return map[string]interface{}{
		"date":    date,
		"speaker": speaker,
		"text":    trimSpace(after),
		"topic":   extractTopic(after),
	}
}

func findSenderPattern(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '-' || s[i] == ']' {
			return i
		}
	}
	return -1
}

func findMatchingBracket(s string) int {
	count := 0
	for i, c := range s {
		if c == '[' {
			count++
		} else if c == ']' {
			count--
			if count == 0 {
				return i
			}
		}
	}
	return -1
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func extractTopic(text string) string {
	if len(text) < 10 {
		return "general"
	}
	// Simple keyword extraction
	keywords := []string{
		"family", "mom", "dad", "parent", "child", "son", "daughter",
		"work", "job", "career", "business",
		"health", "sick", "hospital", "doctor",
		"travel", "vacation", "trip", "holiday",
		"food", "eat", "restaurant", "cook",
		"hobby", "music", "movie", "book",
		"friend", "party", "celebrate",
		"life", "death", "remember", "miss",
	}

	lower := toLower(text)
	for _, kw := range keywords {
		if contains(lower, kw) {
			return kw
		}
	}
	return "general"
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
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