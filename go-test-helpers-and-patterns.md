# My Favorite Go Test Helper Patterns: DBs, External Services, and Cleanup

Over time, I've found a few patterns that make my Go tests much more reliable, readable, and easy to maintain—whether I'm testing with SQLite, RabbitMQ, or any other external dependency. Here's how I approach test setup and teardown, and why these patterns work so well for me.

## 1. Setup Helpers That Return a Cleanup Function

I always try to write my test setup helpers so they return a cleanup function. This lets me do `defer cleanup()` in my test, and guarantees that resources are cleaned up no matter how the test exits.

**Example:**
```go
func setupTestDB(t *testing.T) (*sql.DB, func()) {
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatalf("failed to open sqlite: %v", err)
    }
    cleanup := func() { db.Close() }
    return db, cleanup
}

func TestSomething(t *testing.T) {
    db, cleanup := setupTestDB(t)
    defer cleanup()
    // ... test logic ...
}
```

This pattern keeps my tests safe and clean, and I never have to worry about leaking resources.

## 2. Dedicated Helpers for DBs and External Services

Instead of repeating connection logic everywhere, I write dedicated helpers for opening DBs or connecting to services. For SQLite, I have a function like this:

```go
func openInMemorySQLite(t *testing.T) (*sql.DB, func()) {
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatalf("failed to open sqlite: %v", err)
    }
    cleanup := func() { db.Close() }
    return db, cleanup
}
```

If I need a special version (e.g., with custom SQL functions), I just make a new helper. This keeps all setup logic in one place, and my tests focused on what matters.

## 3. Skipping Tests When External Services Aren't Available (e.g., RabbitMQ)

Sometimes, my tests depend on external services like RabbitMQ. I don't want my test suite to fail just because RabbitMQ isn't running locally or in CI. Instead, I try to connect, and if it fails, I skip the test.

**Example:**
```go
import (
    "os"
    "testing"
    "github.com/rabbitmq/amqp091-go"
)

func setupRabbitMQ(t *testing.T) (*amqp.Connection, func()) {
    url := os.Getenv("RABBITMQ_URL")
    if url == "" {
        url = "amqp://guest:guest@localhost:5672/"
    }
    conn, err := amqp.Dial(url)
    if err != nil {
        t.Skipf("skipping test: could not connect to RabbitMQ at %s: %v", url, err)
    }
    cleanup := func() { conn.Close() }
    return conn, cleanup
}

func TestRabbitMQPublish(t *testing.T) {
    conn, cleanup := setupRabbitMQ(t)
    defer cleanup()
    // ... test logic ...
}
```

This way, my tests are robust: they run if the service is available, and skip (not fail) if it isn't. This is especially useful for CI environments or when running tests locally without all dependencies.

## 4. Why These Patterns Matter

- **Reliability:** Tests don't leak resources, and always clean up after themselves.
- **Clarity:** Test logic is separate from setup/teardown logic, making tests easier to read and maintain.
- **Flexibility:** I can easily add new helpers for different DBs or services, or tweak existing ones for special cases.
- **Robustness:** Tests that depend on external services don't cause the whole suite to fail if the service isn't available—they just skip.

## 5. Summary

- Use setup helpers that return cleanup functions for all resources.
- Write dedicated helpers for DBs and external services, and keep all setup logic there.
- For external dependencies, try to connect and skip the test if the service isn't available.
- Keep test logic focused on what's being tested, not on setup/teardown boilerplate.

These patterns have made my Go test suites much more robust and maintainable, and I recommend them for any project that deals with databases or external services in tests. 