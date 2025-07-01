package main

import (
	"context"
	"errors"

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

func (r *Repository) DeleteByName(ctx context.Context, name string) error {
	result := r.db.WithContext(ctx).Delete(&Queue{}, "name LIKE ?", name)
	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("queue not found")
	}

	return nil
}

func (r *Repository) GetQueueByName(ctx context.Context, name string) (*Queue, error) {
	var queues Queue

	// Get queues that are pending or failed (eligible for processing)
	// Order by shifting queue logic: GREATEST(created_at, COALESCE(last_retry_at, created_at))
	err := r.db.WithContext(ctx).
		Where("name", name).
		First(&queues).Error

	if err != nil {
		return nil, err
	}

	return &queues, nil
}

func (r *Repository) GetEligibleQueues(ctx context.Context) ([]Queue, error) {
	var queues []Queue

	// Get queues that are pending or failed (eligible for processing)
	// Order by shifting queue logic: GREATEST(created_at, COALESCE(last_retry_at, created_at))
	err := r.db.WithContext(ctx).
		Where("status IN ?", []QueueStatus{StatusPending, StatusFailed}).
		Order("GREATEST(created_at, COALESCE(last_retry_at, created_at)) ASC").
		Limit(1).
		Find(&queues).Error

	if err != nil {
		return nil, err
	}

	return queues, nil
}
