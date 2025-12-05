package store

import (
	"fmt"
	"lambda_server/internal/database"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ScreenshotStore manages the persistent storage of screenshots
type ScreenshotStore struct {
	db      *gorm.DB
	baseDir string
}

// NewScreenshotStore creates a new instance of ScreenshotStore
func NewScreenshotStore(db *gorm.DB) *ScreenshotStore {
	// Ensure uploads directory exists
	baseDir := "uploads/screenshots"
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		fmt.Printf("Error creating screenshots directory: %v\n", err)
	}

	return &ScreenshotStore{
		db:      db,
		baseDir: baseDir,
	}
}

// Save stores the image data to disk and metadata to DB, returning the unique ID
func (s *ScreenshotStore) Save(clientID string, data []byte) (string, error) {
	id := uuid.New().String()
	filename := fmt.Sprintf("%s.png", id)
	filePath := filepath.Join(s.baseDir, filename)

	// 1. Save to disk
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write screenshot to disk: %w", err)
	}

	// 2. Save metadata to DB
	screenshot := database.Screenshot{
		ID:       id,
		ClientID: clientID,
		FilePath: filePath,
	}

	if err := s.db.Create(&screenshot).Error; err != nil {
		// Cleanup file if DB insert fails
		os.Remove(filePath)
		return "", fmt.Errorf("failed to save screenshot metadata: %w", err)
	}

	return id, nil
}

// Get retrieves the image data by ID (reads from disk)
func (s *ScreenshotStore) Get(id string) ([]byte, bool) {
	var screenshot database.Screenshot
	if err := s.db.First(&screenshot, "id = ?", id).Error; err != nil {
		return nil, false
	}

	data, err := os.ReadFile(screenshot.FilePath)
	if err != nil {
		return nil, false
	}

	return data, true
}

// Delete removes images by their IDs (DB + Disk)
func (s *ScreenshotStore) Delete(ids []string) {
	// Find records first to know file paths
	var screenshots []database.Screenshot
	if err := s.db.Where("id IN ?", ids).Find(&screenshots).Error; err != nil {
		return
	}

	for _, sc := range screenshots {
		// Remove file
		os.Remove(sc.FilePath)
	}

	// Remove from DB
	s.db.Delete(&database.Screenshot{}, "id IN ?", ids)
}

// DeleteByClientID removes all images for a specific client (DB + Disk)
func (s *ScreenshotStore) DeleteByClientID(clientID string) {
	var screenshots []database.Screenshot
	if err := s.db.Where("client_id = ?", clientID).Find(&screenshots).Error; err != nil {
		return
	}

	for _, sc := range screenshots {
		os.Remove(sc.FilePath)
	}

	s.db.Where("client_id = ?", clientID).Delete(&database.Screenshot{})
}
