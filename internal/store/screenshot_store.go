package store

import (
	"fmt"
	"lambda_server/internal/database"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// ScreenshotTTL is the time-to-live for screenshots before automatic cleanup
	ScreenshotTTL = 3 * time.Minute
	// CleanupInterval is how often the cleanup routine runs
	CleanupInterval = 1 * time.Minute
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

// StartCleanupRoutine starts a background goroutine that periodically deletes expired screenshots
func (s *ScreenshotStore) StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(CleanupInterval)
		defer ticker.Stop()

		for range ticker.C {
			s.deleteExpired()
		}
	}()
	fmt.Println("Screenshot cleanup routine started (TTL: 3 minutes)")
}

// deleteExpired removes all screenshots older than ScreenshotTTL
func (s *ScreenshotStore) deleteExpired() {
	expirationTime := time.Now().Add(-ScreenshotTTL)

	var screenshots []database.Screenshot
	if err := s.db.Where("created_at < ?", expirationTime).Find(&screenshots).Error; err != nil {
		fmt.Printf("Error finding expired screenshots: %v\n", err)
		return
	}

	if len(screenshots) == 0 {
		return
	}

	// Delete files from disk
	for _, sc := range screenshots {
		if err := os.Remove(sc.FilePath); err != nil && !os.IsNotExist(err) {
			fmt.Printf("Error removing screenshot file %s: %v\n", sc.FilePath, err)
		}
	}

	// Delete from database
	if err := s.db.Where("created_at < ?", expirationTime).Delete(&database.Screenshot{}).Error; err != nil {
		fmt.Printf("Error deleting expired screenshots from database: %v\n", err)
		return
	}

	fmt.Printf("Cleaned up %d expired screenshots\n", len(screenshots))
}
