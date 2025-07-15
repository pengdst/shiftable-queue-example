# Changelog

This document tracks the project's evolution and state within specific timeframes. It's not just a list of commits, but a curated snapshot of the architecture and functionality.

---

## Period: 2025-07-08 - 2025-07-15

### Key Developments

-   **Comprehensive Error Handling Overhaul**: Refactored core application components (`Config`, `GORM`, `Server`, `QueueProcessor`) to return errors instead of calling `log.Fatal`. This change significantly improves application stability and testability by allowing for graceful error management.
-   **Massive Test Coverage Expansion**: Increased test coverage to over 93% by adding extensive unit and integration tests for the repository, queue processor, middleware, server, and configuration loading. This includes a new `server_test.go` file and significant additions to almost all existing test files.
-   **Queue Processor Refactoring (Functional Options)**: The `NewQueueProcessor` constructor was refactored to use the functional options pattern. This decouples the processor from its dependencies (like RabbitMQ connections) and allows for easy injection of mocks during testing.
-   **Improved JSON and Middleware Logic**: Enhanced the `WriteJSON` and `WriteJSONError` helper functions to handle JSON marshaling errors gracefully. The logging middleware was also improved with more robust tests.
-   **Build Process Streamlining**: The `Makefile` was updated to optimize the `mock` and `vet` commands for a more efficient development workflow.

---

## Period: 2025-07-01 - 2025-07-07

### Key Developments

-   **Core Queue Implementation**: Developed the primary feature of a shiftable queue system, including logic for adding, processing, and deleting queue items.
-   **API Endpoints**: Created HTTP endpointsman to interact with the queue, such as marking items as eligible for processing and removing items from the queue.
-   **Queue Processor Logic**: Implemented a processor to handle various queueing scenarios, including starvation and burst processing.
-   **Comprehensive Testing**:
    -   Built an extensive suite of unit and integration tests for the queue processor, middleware, and database repository.
    -   Utilized in-memory SQLite for robust and isolated database testing.
    -   Integrated mockery for generating mocks and isolating dependencies.
-   **Standardized Tooling**:
    -   Configured a `Makefile` to automate and standardize common development tasks like building, testing, running the application, and generating mocks.
-   **Code Refinements**:
    -   Fixed bugs related to queue ordering logic.
    -   Improved the server and testing architecture using the options pattern.
    -   Removed unused code to improve maintainability.