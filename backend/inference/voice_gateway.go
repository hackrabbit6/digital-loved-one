// Package inference provides voice call gateway with STT → Grounding → LLM → TTS.
package inference

import (
	"os"
	"strings"

	"digital-loved-one/backend/grounding"
	"digital-loved-one/backend/schema"
)

// VoiceGateway handles real-time voice calls with the virtual person.
type VoiceGateway struct {
	groundingLayer *grounding.Layer
	llmAPIKey      string
}

// NewVoiceGateway creates a new voice gateway.
func NewVoiceGateway(groundingLayer *grounding.Layer) *VoiceGateway {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("CLAUDE_API_KEY")
	}

	return &VoiceGateway{
		groundingLayer: groundingLayer,
		llmAPIKey:      apiKey,
	}
}

// VoiceConfig holds voice synthesis configuration.
type VoiceConfig struct {
	VoiceID      string
	SpeakingRate float64 // 0.5 to 2.0
	Pitch        float64 // 0.5 to 2.0
	Model        string  // e.g., "elevenlabs", "coqui", "google"
}

// DefaultVoiceConfig returns default voice configuration.
func DefaultVoiceConfig() VoiceConfig {
	return VoiceConfig{
		VoiceID:      "default",
		SpeakingRate: 1.0,
		Pitch:        1.0,
		Model:        "elevenlabs",
	}
}

// VoiceExchange represents a single voice exchange.
type VoiceExchange struct {
	UserQuery   string
	PersonaText string
	AudioURL    string // URL to generated audio
	Confident   bool
	Blocked     bool
	Conflicts   []string
}

// ProcessVoice processes a voice query and returns a voice response.
// This is a stub implementation - production would integrate with STT/TTS services.
func (g *VoiceGateway) ProcessVoice(audioData []byte, personaID string, config VoiceConfig) (*VoiceExchange, error) {
	// 1. STT - Convert audio to text
	query, err := g.speechToText(audioData)
	if err != nil {
		return nil, err
	}

	// 2. Build context for grounding layer
	context := map[string]interface{}{
		"persona_id": personaID,
		"excerpts":   []*schema.SourceExcerpt{}, // Would fetch from store
		"conflicts":  []schema.ConflictMarker{},
	}

	// 3. Validate with grounding layer
	validation := g.groundingLayer.ValidateForInference(query, context)

	// 4. If blocked, return an honest fallback.
	if validation.Blocked {
		return &VoiceExchange{
			UserQuery:   query,
			PersonaText: "我现在没有足够的记忆资料来有把握地回答这个问题。",
			Confident:   false,
			Blocked:     true,
		}, nil
	}

	// 5. Build prompt and call LLM
	prompt := g.buildPrompt(query, validation)
	response := g.callLLM(prompt)

	// 6. TTS - Convert text to audio
	audioURL, err := g.textToSpeech(response, config)
	if err != nil {
		// Continue without audio - return text only
		audioURL = ""
	}

	// 7. Surface conflicts
	var conflicts []string
	for _, cm := range validation.Conflicts {
		conflicts = append(conflicts, "注意：我的记忆里有一个尚未解决的"+cm.Type+"冲突。")
	}

	return &VoiceExchange{
		UserQuery:   query,
		PersonaText: response,
		AudioURL:    audioURL,
		Confident:   validation.ConfidenceScore > 0.5,
		Blocked:     false,
		Conflicts:   conflicts,
	}, nil
}

// ProcessTextQuery processes a text query and returns a text + optional audio response.
func (g *VoiceGateway) ProcessTextQuery(query, personaID string, config VoiceConfig) (*VoiceExchange, error) {
	// Build context
	context := map[string]interface{}{
		"persona_id": personaID,
		"excerpts":   []*schema.SourceExcerpt{},
		"conflicts":  []schema.ConflictMarker{},
	}

	// Validate with grounding layer
	validation := g.groundingLayer.ValidateForInference(query, context)

	// If blocked
	if validation.Blocked {
		return &VoiceExchange{
			UserQuery:   query,
			PersonaText: "我现在没有足够的记忆资料来有把握地回答这个问题。",
			Confident:   false,
			Blocked:     true,
		}, nil
	}

	// Build prompt and call LLM
	prompt := g.buildPrompt(query, validation)
	response := g.callLLM(prompt)

	// TTS
	audioURL, _ := g.textToSpeech(response, config)

	// Surface conflicts
	var conflicts []string
	for _, cm := range validation.Conflicts {
		conflicts = append(conflicts, "注意：我的记忆里有一个尚未解决的"+cm.Type+"冲突。")
	}

	return &VoiceExchange{
		UserQuery:   query,
		PersonaText: response,
		AudioURL:    audioURL,
		Confident:   validation.ConfidenceScore > 0.5,
		Blocked:     false,
		Conflicts:   conflicts,
	}, nil
}

func (g *VoiceGateway) speechToText(audioData []byte) (string, error) {
	// STUB: In production, integrate with:
	// - Whisper (OpenAI)
	// - Google Speech-to-Text
	// - Deepgram
	// - ElevenLabs API

	if len(audioData) == 0 {
		return "", nil
	}

	// For now, return a placeholder indicating STT would happen here
	return "[STT would convert audio to text]", nil
}

func (g *VoiceGateway) textToSpeech(text string, config VoiceConfig) (string, error) {
	// STUB: In production, integrate with:
	// - ElevenLabs (voice cloning)
	// - Coqui (open source TTS)
	// - Google Cloud TTS
	// - AWS Polly

	if text == "" {
		return "", nil
	}

	// For now, return empty - no audio generated in stub
	return "", nil
}

func (g *VoiceGateway) buildPrompt(query string, validation *schema.ValidationResult) string {
	var prompt strings.Builder

	prompt.WriteString("Query: ")
	prompt.WriteString(query)
	prompt.WriteString("\n")

	if validation.Blocked {
		prompt.WriteString("我没有足够的记忆资料来回答这个问题。")
		return prompt.String()
	}

	prompt.WriteString("\nMemory context (with citations):\n")
	for _, e := range validation.Citations {
		prompt.WriteString("- ")
		prompt.WriteString(e)
		prompt.WriteString("\n")
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

func (g *VoiceGateway) callLLM(prompt string) string {
	if g.llmAPIKey == "" {
		return g.stubResponse(prompt)
	}

	// STUB: In production, make actual API call to:
	// - Anthropic Claude
	// - OpenAI GPT
	// - Local model (Llama, etc.)

	return g.stubResponse(prompt)
}

func (g *VoiceGateway) stubResponse(prompt string) string {
	if strings.Contains(prompt, "I don't have sufficient memory") || strings.Contains(prompt, "没有足够") {
		return "我现在没有足够的记忆资料来回答这个问题。"
	}
	return "根据我保存的记忆，我能看到一些相关线索，但现在还没有接入真正的生成模型。"
}
