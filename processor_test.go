package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestQueueProcessor_MockSetup(t *testing.T) {
	t.Run("POSITIVE-MockRestAPI_SimulateProcessing", func(t *testing.T) {
		// Create mock REST API
		mockAPI := &MockRestAPI{}

		// Set up expectation
		queue := &Queue{
			Name:   "test-queue",
			Status: StatusPending,
		}

		// Mock expects SimulateProcessing to be called with our queue and return nil
		mockAPI.EXPECT().SimulateProcessing(queue).Return(nil).Once()

		// Call the mock
		err := mockAPI.SimulateProcessing(queue)

		// Assertions
		assert.NoError(t, err)
		mockAPI.AssertExpectations(t)
	})

	t.Run("POSITIVE-MockQueueTrigger_TriggerProcessing", func(t *testing.T) {
		// Create mock queue trigger
		mockTrigger := &MockQueueTrigger{}

		ctx := context.Background()

		// Mock expects TriggerProcessing to be called with context and return nil
		mockTrigger.EXPECT().TriggerProcessing(ctx).Return(nil).Once()

		// Call the mock
		err := mockTrigger.TriggerProcessing(ctx)

		// Assertions
		assert.NoError(t, err)
		mockTrigger.AssertExpectations(t)
	})

	t.Run("POSITIVE-MockRestAPI_SimulateProcessingError", func(t *testing.T) {
		// Create mock REST API
		mockAPI := &MockRestAPI{}

		queue := &Queue{
			Name:   "test-queue-error",
			Status: StatusPending,
		}

		// Mock expects SimulateProcessing to be called and return error
		expectedError := assert.AnError
		mockAPI.EXPECT().SimulateProcessing(queue).Return(expectedError).Once()

		// Call the mock
		err := mockAPI.SimulateProcessing(queue)

		// Assertions
		assert.Error(t, err)
		assert.Equal(t, expectedError, err)
		mockAPI.AssertExpectations(t)
	})

	t.Run("POSITIVE-MockWithAnyMatcher", func(t *testing.T) {
		// Create mock REST API
		mockAPI := &MockRestAPI{}

		// Mock expects SimulateProcessing to be called with any queue
		mockAPI.EXPECT().SimulateProcessing(mock.AnythingOfType("*main.Queue")).Return(nil).Once()

		// Call with any queue
		queue := &Queue{Name: "any-queue", Status: StatusProcessing}
		err := mockAPI.SimulateProcessing(queue)

		// Assertions
		assert.NoError(t, err)
		mockAPI.AssertExpectations(t)
	})
}
