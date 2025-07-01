package main

import (
	"context"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) Save(ctx context.Context, queue *Queue) error {
	err := r.db.WithContext(ctx).Save(queue).Error
	return err
}

func (r *Repository) Fetch(ctx context.Context) ([]Queue, error) {
	var queues []Queue
	err := r.db.WithContext(ctx).
		Find(&queues).
		Order("GREATEST(created_at, COALESCE(last_retry_at, created_at)) ASC").
		Error
	if err != nil {
		return nil, err
	}

	return queues, nil
}

func (r *Repository) DeleteByID(ctx context.Context, id int) error {
	err := r.db.WithContext(ctx).First(&Queue{}, id).Error
	if err != nil {
		return err
	}

	return r.db.WithContext(ctx).Delete(&Queue{}, id).Error
}
