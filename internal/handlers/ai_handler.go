package handlers

import (
	"log"
	"net/http"

	"lambda_server/internal/services"
	"lambda_server/internal/store"

	"github.com/gin-gonic/gin"
)

type AIHandler struct {
	geminiService   *services.GeminiService
	screenshotStore *store.ScreenshotStore
}

func NewAIHandler(geminiService *services.GeminiService, screenshotStore *store.ScreenshotStore) *AIHandler {
	return &AIHandler{
		geminiService:   geminiService,
		screenshotStore: screenshotStore,
	}
}

// AnalyzeRequest defines the expected JSON body for the analyze endpoint
type AnalyzeRequest struct {
	ScreenshotIDs []string `json:"screenshot_ids" binding:"required"`
	RequestType   string   `json:"type" binding:"required"`  // leetcode, other
	Language      string   `json:"language"`                 // for leetcode
	Model         string   `json:"model" binding:"required"` // gemini model
}

// AnalyzeResponse defines the JSON response
type AnalyzeResponse struct {
	Result string `json:"result"`
}

// Analyze handles the AI analysis request
// @Summary      Analyze screenshots
// @Description  Analyze previously uploaded screenshots using Gemini AI
// @Tags         ai
// @Accept       json
// @Produce      json
// @Param        request body AnalyzeRequest true "Analysis Request"
// @Success      200  {object}  AnalyzeResponse
// @Failure      400  {object}  map[string]string  "Bad request"
// @Failure      404  {object}  map[string]string  "Screenshot not found"
// @Failure      500  {object}  map[string]string  "Internal server error"
// @Router       /ai/analyze [post]
func (h *AIHandler) Analyze(c *gin.Context) {
	var req AnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Retrieve images from store
	var imagesData [][]byte
	for _, id := range req.ScreenshotIDs {
		data, exists := h.screenshotStore.Get(id)
		if !exists {
			log.Printf("AIHandler: Screenshot %s not found", id)
			// Decide behavior: fail all or partial? Let's fail for consistency.
			c.JSON(http.StatusNotFound, gin.H{"error": "Screenshot not found: " + id})
			return
		}
		imagesData = append(imagesData, data)
	}

	if len(imagesData) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No screenshots provided"})
		return
	}

	// Check if Gemini service is available
	if h.geminiService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI service not available"})
		return
	}

	// Call AI Service
	log.Printf("AIHandler: analyzing %d images with model %s", len(imagesData), req.Model)
	ctx := c.Request.Context()
	result, err := h.geminiService.AnalyzeImages(ctx, imagesData, req.RequestType, req.Language, req.Model)
	if err != nil {
		log.Printf("AIHandler: Analysis failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI processing failed: " + err.Error()})
		return
	}

	// Cleanup used screenshots (and all screenshots for this user, as requested)
	// We need clientID from context
	userID, exists := c.Get("userID")
	if exists {
		if uid, ok := userID.(string); ok {
			h.screenshotStore.DeleteByClientID(uid)
		}
	}

	c.JSON(http.StatusOK, AnalyzeResponse{Result: result})
}
