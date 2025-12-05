package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Screenshot represents a captured screenshot metadata stored in the database
type Screenshot struct {
	ID        string    `gorm:"primaryKey;type:uuid"`
	ClientID  string    `gorm:"index;not null"`
	FilePath  string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// BeforeCreate generates a UUID for the screenshot if not present
func (s *Screenshot) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return
}
