package services

import (
	"context"
	"fmt"
	"lambda_server/internal/config"

	"google.golang.org/genai"
)

// GeminiService encapsulates the thread-safe genai.Client
type GeminiService struct {
	genaiClient *genai.Client
}

// NewGeminiService creates and returns the singleton AI service
func NewGeminiService(cfg *config.Config) (*GeminiService, error) {
	// Use the APIKey from the loaded config
	apiKey := cfg.GeminiAPIKey
	if apiKey == "" {
		return nil, fmt.Errorf("GOOGLE_API_KEY not set")
	}

	// Create the client using the Gemini Developer API backend
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	return &GeminiService{genaiClient: client}, nil
}

// AnalyzeImages processes multiple images with a prompt based on type and language
func (s *GeminiService) AnalyzeImages(ctx context.Context, imagesData [][]byte, requestType, language, model string) (string, error) {
	// Construct the prompt based on type
	var prompt string
	if requestType == "leetcode" {
		// For leetcode type, include the language in the prompt
		prompt = fmt.Sprintf("Analyze these images (which may be parts of a LeetCode problem) and provide a solution in %s. Explain your approach and provide the complete code solution.", language)
	} else {
		// For other type, use a general prompt
		prompt = "Analyze these images and provide a solution to the problem you see."
	}

	// Create parts
	parts := make([]*genai.Part, 0, len(imagesData)+1)

	// Add images
	for _, data := range imagesData {
		// Default to PNG, could detect MIME type in the future or use what we stored
		// Since we store what we receive, let's assume it's compatible or we cleaned it up.
		// For now simple pass-through.
		parts = append(parts, genai.NewPartFromBytes(data, "image/png"))
	}

	// Create text part from prompt
	parts = append(parts, genai.NewPartFromText(prompt))

	// Create content with image and text parts
	content := genai.NewContentFromParts(parts, genai.RoleUser)

	// Generate content using the Models service
	resp, err := s.genaiClient.Models.GenerateContent(ctx, "models/"+model, []*genai.Content{content}, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	// Extract text response using the Text() method
	text := resp.Text()
	if text == "" {
		return "", fmt.Errorf("empty response from Gemini API")
	}

	return text, nil
}
