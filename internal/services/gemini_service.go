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

// AnalyzeImage processes an image with a prompt based on type and language
func (s *GeminiService) AnalyzeImage(ctx context.Context, imageData []byte, requestType, language string) (string, error) {
	// Construct the prompt based on type
	var prompt string
	if requestType == "leetcode" {
		// For leetcode type, include the language in the prompt
		prompt = fmt.Sprintf("Analyze this LeetCode problem and provide a solution in %s. Explain your approach and provide the complete code solution.", language)
	} else {
		// For other type, use a general prompt
		prompt = "Analyze this image and provide a solution to the problem you see."
	}

	// Create image part from bytes
	// Default to PNG, could detect MIME type in the future
	imagePart := genai.NewPartFromBytes(imageData, "image/png")

	// Create text part from prompt
	textPart := genai.NewPartFromText(prompt)

	// Create content with image and text parts
	content := genai.NewContentFromParts([]*genai.Part{imagePart, textPart}, genai.RoleUser)

	// Generate content using the Models service
	resp, err := s.genaiClient.Models.GenerateContent(ctx, "models/gemini-2.5-pro", []*genai.Content{content}, nil)
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
