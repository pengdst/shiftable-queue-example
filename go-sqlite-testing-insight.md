# Using SQLite for Testing in Go: What I Learned

After working with SQLite in Go tests, I've picked up some practical lessons and best practices that are worth documenting for future reference. Here's my personal summary, with examples and gotchas.

## 1. Why Use SQLite for Testing?

SQLite is a lightweight, file-based database that's easy to spin up and tear down. For most Go projects, it's a great choice for integration tests because:
- No external dependencies or services are needed.
- It's fast and runs in-memory if needed.
- Schema and data are isolated per test run.

## 2. How to Use SQLite in Go Tests

The most common approach is to use the `github.com/mattn/go-sqlite3` driver and open a new database for each test. For true isolation and speed, I use the in-memory mode:

```go
import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
    "testing"
)

func setupTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatalf("failed to open sqlite: %v", err)
    }
    // Run schema migrations or setup here
    return db
}

func TestSomething(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()
    // ... run test logic using db ...
}
```

## 3. Common Pitfalls and Gotchas

### a. In-Memory DB Lifetime
- Each `sql.Open("sqlite3", ":memory:")` call creates a new, empty database. If you open multiple connections, they do NOT share the same in-memory DB.
- If you need to share the same in-memory DB across multiple connections, use the special URI: `file::memory:?cache=shared` and set `?_cache=shared`.

```go
db, _ := sql.Open("sqlite3", "file::memory:?cache=shared")
```

### b. Transactions and Concurrency
- SQLite is not designed for high concurrency. For most tests, single-threaded access is fine.
- If you use transactions, remember that rolling back or closing the DB will lose all data (in-memory mode).

### c. Foreign Keys Are Disabled by Default
- By default, SQLite disables foreign key constraints. If your schema relies on them, you must enable them manually:

```go
_, _ = db.Exec("PRAGMA foreign_keys = ON;")
```

### d. Clean Up After Tests
- Always close the DB (`defer db.Close()`) to avoid leaking file handles.
- For file-based DBs, remove the file after the test if you're not using in-memory mode.

## 4. SQLite vs Postgres: Missing Functions and Workarounds

One of the biggest challenges I faced was the difference in SQL functions between SQLite and Postgres. Some functions that are standard in Postgres simply don't exist in SQLite, especially around date/time and math operations.

### a. Missing Date/Time Functions
- Postgres has rich date/time functions like `EXTRACT(EPOCH FROM ...)`, `date_trunc`, and direct support for timestamps in microseconds or nanoseconds.
- SQLite is much more limited. For example, it doesn't have `EXTRACT(EPOCH ...)` or native nanosecond timestamp support.

#### **My Struggle: unix vs unixnano, Date/Time Format Hell, Timezones, and Multi-Format Parsing**
I needed to store and query timestamps in nanoseconds ("unixnano") for my tests, but SQLite only has `strftime('%s', ...)` which returns seconds since epoch ("unix").

At first, I tried to use `strftime('%s', ...)` and multiply by 1e9 in Go, but this didn't work for sub-second precision. I needed a way to get nanosecond-precision timestamps in SQLite to match my Postgres logic.

But the real pain started when I tried to use Go's `time.RFC3339Nano` format for storing and parsing timestamps in SQLite. I assumed that since RFC3339Nano is the most precise and standard format in Go, it would "just work" with SQLite's date/time functions. I was wrong.

**SQLite's Date/Time Handling is Tricky:**
- SQLite stores dates as TEXT, REAL, or INTEGER, and its date/time functions expect very specific string formats.
- RFC3339Nano (e.g., `2024-06-01T12:34:56.789123456Z`) is not always recognized by SQLite's built-in functions, especially if you include the trailing 'Z' or too many fractional digits.
- SQLite prefers formats like `YYYY-MM-DD HH:MM:SS.SSS` (with a space, not 'T', and up to 3 digits for milliseconds), or ISO8601 with limited precision.

**Timezone Struggles:**
- Go's `time.Time` and Postgres both handle timezones and offsets natively. Postgres can store and query timestamps with time zone (`timestamptz`) and will always keep the offset or 'Z' (UTC) info.
- SQLite, on the other hand, is very loose with timezones. If you store a timestamp with a 'Z' (for UTC) or an explicit offset (like `+07:00`), SQLite will just treat it as a string. Its date/time functions may ignore the offset, or worse, fail to parse the value at all.
- I found that if I stored timestamps as `2024-06-01T12:34:56.789123456Z`, SQLite would sometimes treat it as a date, sometimes as a string, and sometimes just return NULL for date/time functions.

**Multi-Format Parsing: The Only Reliable Way**
Because of all these inconsistencies, I realized I couldn't rely on a single time format for parsing timestamps in my Go code—especially when data could come from Go, Postgres, or SQLite, each with their own quirks. Sometimes the timestamp would have a timezone offset, sometimes not. Sometimes it would be in UTC with a 'Z', sometimes with `+00:00`, sometimes with no offset at all.

To make my tests robust, I started using a slice of possible layouts and looping through them until one worked. For example:

```go
formats := []string{
    "2006-01-02 15:04:05.999999999-07:00",
    "2006-01-02 15:04:05-07:00",
    "2006-01-02 15:04:05",
}

var t time.Time
var err error
for _, layout := range formats {
    t, err = time.Parse(layout, input)
    if err == nil {
        break
    }
}
if err != nil {
    // handle error: unknown format
}
```

This way, no matter what timezone or format the timestamp came in, my code could handle it gracefully.

**What Happened:**
- When I inserted timestamps in RFC3339Nano format (with 'Z' or offsets), SQLite would sometimes treat them as plain strings, not as dates, so functions like `strftime` or `date` would return NULL or wrong results.
- I tried to parse the string in Go using `time.Parse(time.RFC3339Nano, ...)` and then reformat it, but the mismatch in expectations kept breaking my tests.
- Even when I used the SQLite-friendly format (`2006-01-02 15:04:05.999999999`), if I added a timezone offset, SQLite would not always parse it correctly.

**How I Solved It:**
- I standardized on storing all timestamps in UTC, and always formatted them as `2006-01-02 15:04:05.999999999` **without** any timezone info or offset. This way, SQLite would always treat the value as a date/time, and my custom Go functions could safely assume UTC.
- In my custom Go functions (like `unixnano`), I made sure to parse both the SQLite-friendly format (assumed UTC), RFC3339Nano (with timezone info), and any other formats I expected, using a multi-format parsing loop as above.
- If I ever needed to store timezone-aware timestamps, I would store the offset in a separate column, or always convert to UTC before saving.

**Example:**
```go
// Store timestamps in UTC, SQLite-friendly format (no timezone info)
now := time.Now().UTC()
sqliteTime := now.Format("2006-01-02 15:04:05.999999999")

// Register a custom function that can parse multiple formats
sqliteConn.RegisterFunc("unixnano", func(ts string) int64 {
    formats := []string{
        time.RFC3339Nano,
        "2006-01-02 15:04:05.999999999-07:00",
        "2006-01-02 15:04:05-07:00",
        "2006-01-02 15:04:05.999999999",
        "2006-01-02 15:04:05",
    }
    var t time.Time
    var err error
    for _, layout := range formats {
        t, err = time.Parse(layout, ts)
        if err == nil {
            break
        }
    }
    if err != nil {
        return 0 // or handle error
    }
    return t.UTC().UnixNano()
}, true)
```

**Lesson Learned:**
- Never assume Go's or Postgres's default time formats (with timezone info) will "just work" with SQLite.
- Always check what format SQLite expects, and be ready to convert or parse multiple formats in your Go code.
- For maximum compatibility, store timestamps in UTC, in a format SQLite can parse, and handle timezones explicitly in your application logic if needed.
- If you want your tests to be portable between Postgres and SQLite, standardize on a format that both can handle, or add compatibility logic in your test helpers.
- Use multi-format parsing for any code that needs to handle timestamps from multiple sources or with/without timezone offsets.

### b. Other Function Gaps
- Some math and string functions are also missing in SQLite. For simple cases, I rewrote the SQL or did the calculation in Go.
- For more complex needs, I registered additional custom functions in Go, following the same pattern as above.

### c. General Advice for Compatibility
- Always check which SQL functions your production DB uses, and see if they're available in SQLite.
- For anything missing, consider:
    - Rewriting the query for tests
    - Doing the calculation in Go
    - Registering a custom function in SQLite
- Test your queries in both environments to catch subtle differences early.

## 5. Example: Full Test Setup

```go
func setupTestDB(t *testing.T) *sql.DB {
    db, err := sql.Open("sqlite3", ":memory:")
    if err != nil {
        t.Fatalf("failed to open sqlite: %v", err)
    }
    _, err = db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, created_at TEXT);`)
    if err != nil {
        t.Fatalf("failed to create schema: %v", err)
    }
    _, _ = db.Exec("PRAGMA foreign_keys = ON;")
    // Register unixnano as above if needed
    return db
}

func TestUserInsert(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()
    _, err := db.Exec(`INSERT INTO users (name, created_at) VALUES ("alice", "2024-06-01 12:34:56.789123456")`)
    if err != nil {
        t.Fatalf("insert failed: %v", err)
    }
    // Example query using unixnano
    row := db.QueryRow(`SELECT unixnano(created_at) FROM users WHERE name = ?`, "alice")
    var ts int64
    if err := row.Scan(&ts); err != nil {
        t.Fatalf("failed to scan: %v", err)
    }
    // ... assertions ...
}
```

## 6. My Takeaways

- SQLite is perfect for fast, isolated integration tests in Go.
- Always use in-memory mode for speed and isolation unless you need to persist data between tests.
- Be aware of connection and transaction semantics—each connection to `:memory:` is a separate DB.
- Enable foreign keys if your schema needs them.
- Clean up resources after each test.
- For missing SQL functions (especially date/time), leverage Go's ability to register custom functions in SQLite. This was a game changer for matching Postgres logic in tests.
- Always test queries in both environments to avoid subtle bugs.
- When dealing with date/time, always check and standardize the format between Go, SQLite, and Postgres. Don't assume RFC3339Nano or timezone info will work everywhere—experiment and adapt as needed. Store UTC if possible.
- Use multi-format parsing for any code that needs to handle timestamps from multiple sources or with/without timezone offsets.

**Bottom line:**
SQLite makes Go database testing easy and reliable, as long as I pay attention to its quirks and connection handling. With custom functions and careful date/time handling, I can bridge most gaps between SQLite and Postgres for testing purposes. 