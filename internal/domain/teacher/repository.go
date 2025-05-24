package teacher

import (
	"context"
)

// Repository defines the operations for persisting and retrieving Teacher entities.
type Repository interface {
	Create(ctx context.Context, teacher *Teacher) error
	GetByID(ctx context.Context, id int64) (*Teacher, error)
	GetByTelegramID(ctx context.Context, telegramID int64) (*Teacher, error)
	Update(ctx context.Context, teacher *Teacher) error // Should handle updates to FirstName, LastName, IsActive
	ListActive(ctx context.Context) ([]*Teacher, error)
	ListAll(ctx context.Context) ([]*Teacher, error) // For admin purposes
}
