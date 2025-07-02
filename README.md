# Shiftable Queue Example

This repository is a **sample project** demonstrating a robust queue processing system with a _shifting mechanism_ for error handling, built with Go and RabbitMQ. The project is designed for engineers and architects who want to understand or prototype how to avoid queue starvation and ensure fair processing order—even when some jobs fail and new jobs keep arriving.

## How It Works

1. **Queue items** are inserted with `StatusPending`.
2. The processor picks the next eligible queue (pending/failed, ordered by shifting logic).
3. If processing fails, the item is marked as failed, `last_retry_at` is updated, and it is shifted back in the queue.
4. New items can be inserted at any time; the shifting logic ensures fairness and prevents starvation.
5. The process continues until all items are completed.

## Getting Started

1. **Clone the repository:**
   ```sh
   git clone github.com/pengdst/shiftable-queue-example
   cd shiftable-queue-example
   ```
2. **Copy the example environment file and fill in your database credentials:**
   ```sh
   cp .env.example .env
   # Edit .env and set your DB credentials (host, user, password, dbname, etc)
   ```
3. **Start RabbitMQ** (locally or via Docker):
   ```sh
   docker run -d --name rabbitmq -p 5672:5672 rabbitmq:3
   ```
4. **Run the test:**
   ```sh
   make test
   ```
4. **Run the app:**
   ```sh
   make run cmd=api
   make run cmd=queue
   ```

**Happy queueing!**

## Key Features

- **Queue Shifting Mechanism:**
  - When a queue item fails to process, it is shifted to the back (or reprioritized) based on its `last_retry_at` timestamp.
  - Prevents starvation: failed jobs will eventually be retried and completed, even if new jobs keep coming in.
- **RabbitMQ Integration:**
  - Uses RabbitMQ for real-world queue processing simulation.
- **Simulation via Integration Test:**
  - The shifting and anti-starvation logic is simulated and validated using integration tests.
  - **Note:** As of the latest update, both Postgres and SQLite (including in-memory) are fully supported for all integration tests and shifting/anti-starvation logic. The shifting mechanism is now robust and reliable on both backends, thanks to a fix using UnixNano for timestamp comparison in SQLite.

## API Endpoints

### Create Queue
- **POST** `/api/v1/queues`
- **Request Body:**
  ```json
  { "name": "queue-name" }
  ```
- **Response:**
  - `201 Created` on success

### List Queues
- **GET** `/api/v1/queues`
- **Response:**
  ```json
  { "data": [ { "id": 1, "name": "queue-name", ... } ] }
  ```

### Delete Queue by ID
- **DELETE** `/api/v1/queues/{id}`
- **Response:**
  - `200 OK` on success

### Leave Queue (Delete by Name)
- **POST** `/api/v1/queues/leave`
- **Request Body:**
  ```json
  { "name": "queue-name" }
  ```
- **Response:**
  - `200 OK` with message on success
    ```json
    { "message": "queue queue-name deleted" }
    ```
  - `500 Internal Server Error` if queue not found or error
    ```json
    { "error": "record not found" }
    ```

---

## Example: Shifting & Anti-Starvation

### POSITIVE-ShiftingQueue_AntiStarvation

**Initial State:**
| Pos | Name          | created_at | last_retry_at | retry | status   |
|-----|---------------|------------|--------------|-------|----------|
| 1   | queue-oldest  | 07:27:58   | 00:00:00     | 0     | pending  |
| 2   | queue-middle  | 07:57:58   | 00:00:00     | 0     | pending  |
| 3   | queue-newest  | 08:27:58   | 00:00:00     | 0     | pending  |

**After 1st process:**
| Pos | Name          | created_at | last_retry_at | retry | status   |
|-----|---------------|------------|--------------|-------|----------|
| 1   | queue-middle  | 07:57:58   | 00:00:00     | 0     | pending  |
| 2   | queue-newest  | 08:27:58   | 00:00:00     | 0     | pending  |
| 3   | queue-oldest* | 07:27:58   | 08:27:58     | 1     | failed   |

**After 2nd process:**
| Pos | Name          | created_at | last_retry_at | retry | status     |
|-----|---------------|------------|--------------|-------|------------|
| 1   | queue-newest  | 08:27:58   | 00:00:00     | 0     | pending    |
| 2   | queue-oldest  | 07:27:58   | 08:27:58     | 1     | failed     |
| 3   | queue-middle* | 07:57:58   | 00:00:00     | 0     | completed  |

**After 3rd process:**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-oldest   | 07:27:58   | 08:27:58     | 1     | failed     |
| 2   | queue-middle   | 07:57:58   | 00:00:00     | 0     | completed  |
| 3   | queue-newest*  | 08:27:58   | 00:00:00     | 0     | completed  |

**After 4th process:**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-middle   | 07:57:58   | 00:00:00     | 0     | completed  |
| 2   | queue-newest   | 08:27:58   | 00:00:00     | 0     | completed  |
| 3   | queue-oldest*  | 07:27:58   | 08:27:58     | 1     | completed  |

### NEGATIVE-Starvation_BurstInsert

**Initial State:**
| Pos | Name           | created_at | last_retry_at | retry | status   |
|-----|----------------|------------|--------------|-------|----------|
| 1   | queue-burst-0  | 00:23:57   | 00:00:00     | 0     | pending  |
| 2   | queue-burst-1  | 00:23:57   | 00:00:00     | 0     | pending  |
| 3   | queue-burst-2  | 00:23:57   | 00:00:00     | 0     | pending  |
| 4   | queue-burst-3  | 00:23:57   | 00:00:00     | 0     | pending  |
| 5   | queue-burst-4  | 00:23:57   | 00:00:00     | 0     | pending  |
| 6   | queue-burst-5  | 00:23:57   | 00:00:00     | 0     | pending  |
| 7   | queue-burst-6  | 00:23:57   | 00:00:00     | 0     | pending  |
| 8   | queue-burst-7  | 00:23:57   | 00:00:00     | 0     | pending  |
| 9   | queue-burst-8  | 00:23:57   | 00:00:00     | 0     | pending  |
| 10  | queue-burst-9  | 00:23:57   | 00:00:00     | 0     | pending  |
| 11  | queue-fail     | 00:23:57   | 00:23:57     | 1     | failed   |

**After fail queue-fail**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-0  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-1  | 00:23:57   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-2  | 00:23:57   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-3  | 00:23:57   | 00:00:00     | 0     | pending    |
| 5   | queue-burst-4  | 00:23:57   | 00:00:00     | 0     | pending    |
| 6   | queue-burst-5  | 00:23:57   | 00:00:00     | 0     | pending    |
| 7   | queue-burst-6  | 00:23:57   | 00:00:00     | 0     | pending    |
| 8   | queue-burst-7  | 00:23:57   | 00:00:00     | 0     | pending    |
| 9   | queue-burst-8  | 00:23:57   | 00:00:00     | 0     | pending    |
| 10  | queue-burst-9  | 00:23:57   | 00:00:00     | 0     | pending    |
| 11  | queue-fail*    | 00:23:57   | 00:23:57     | 1     | failed     |

**After process burst-0**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-1  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-2  | 00:23:57   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-3  | 00:23:57   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-4  | 00:23:57   | 00:00:00     | 0     | pending    |
| 5   | queue-burst-5  | 00:23:57   | 00:00:00     | 0     | pending    |
| 6   | queue-burst-6  | 00:23:57   | 00:00:00     | 0     | pending    |
| 7   | queue-burst-7  | 00:23:57   | 00:00:00     | 0     | pending    |
| 8   | queue-burst-8  | 00:23:57   | 00:00:00     | 0     | pending    |
| 9   | queue-burst-9  | 00:23:57   | 00:00:00     | 0     | pending    |
| 10  | queue-fail     | 00:23:57   | 00:23:57     | 1     | failed     |
| 11  | queue-burst-0* | 00:23:57   | 00:00:00     | 0     | completed  |

**After process burst-1**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-2  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-3  | 00:23:57   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-4  | 00:23:57   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-5  | 00:23:57   | 00:00:00     | 0     | pending    |
| 5   | queue-burst-6  | 00:23:57   | 00:00:00     | 0     | pending    |
| 6   | queue-burst-7  | 00:23:57   | 00:00:00     | 0     | pending    |
| 7   | queue-burst-8  | 00:23:57   | 00:00:00     | 0     | pending    |
| 8   | queue-burst-9  | 00:23:57   | 00:00:00     | 0     | pending    |
| 9   | queue-fail     | 00:23:57   | 00:23:57     | 1     | failed     |
| 10  | queue-burst-0  | 00:23:57   | 00:00:00     | 0     | completed  |
| 11  | queue-burst-1* | 00:23:57   | 00:00:00     | 0     | completed  |

**After process burst-2**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-3  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-4  | 00:23:57   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-5  | 00:23:57   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-6  | 00:23:57   | 00:00:00     | 0     | pending    |
| 5   | queue-burst-7  | 00:23:57   | 00:00:00     | 0     | pending    |
| 6   | queue-burst-8  | 00:23:57   | 00:00:00     | 0     | pending    |
| 7   | queue-burst-9  | 00:23:57   | 00:00:00     | 0     | pending    |
| 8   | queue-fail     | 00:23:57   | 00:23:57     | 1     | failed     |
| 9   | queue-burst-0  | 00:23:57   | 00:00:00     | 0     | completed  |
| 10  | queue-burst-1  | 00:23:57   | 00:00:00     | 0     | completed  |
| 11  | queue-burst-2* | 00:23:57   | 00:00:00     | 0     | completed  |

**After process burst-3**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-4  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-5  | 00:23:57   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-6  | 00:23:57   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-7  | 00:23:57   | 00:00:00     | 0     | pending    |
| 5   | queue-burst-8  | 00:23:57   | 00:00:00     | 0     | pending    |
| 6   | queue-burst-9  | 00:23:57   | 00:00:00     | 0     | pending    |
| 7   | queue-fail     | 00:23:57   | 00:23:57     | 1     | failed     |
| 8   | queue-burst-0  | 00:23:57   | 00:00:00     | 0     | completed  |
| 9   | queue-burst-1  | 00:23:57   | 00:00:00     | 0     | completed  |
| 10  | queue-burst-2  | 00:23:57   | 00:00:00     | 0     | completed  |
| 11  | queue-burst-3* | 00:23:57   | 00:00:00     | 0     | completed  |

**After process burst-4**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-5  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-6  | 00:23:57   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-7  | 00:23:57   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-8  | 00:23:57   | 00:00:00     | 0     | pending    |
| 5   | queue-burst-9  | 00:23:57   | 00:00:00     | 0     | pending    |
| 6   | queue-fail     | 00:23:57   | 00:23:57     | 1     | failed     |
| 7   | queue-burst-0  | 00:23:57   | 00:00:00     | 0     | completed  |
| 8   | queue-burst-1  | 00:23:57   | 00:00:00     | 0     | completed  |
| 9   | queue-burst-2  | 00:23:57   | 00:00:00     | 0     | completed  |
| 10  | queue-burst-3  | 00:23:57   | 00:00:00     | 0     | completed  |
| 11  | queue-burst-4* | 00:23:57   | 00:00:00     | 0     | completed  |

**After process burst-5**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-6  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-7  | 00:23:57   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-8  | 00:23:57   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-9  | 00:23:57   | 00:00:00     | 0     | pending    |
| 5   | queue-fail     | 00:23:57   | 00:23:57     | 1     | failed     |
| 6   | queue-burst-0  | 00:23:57   | 00:00:00     | 0     | completed  |
| 7   | queue-burst-1  | 00:23:57   | 00:00:00     | 0     | completed  |
| 8   | queue-burst-2  | 00:23:57   | 00:00:00     | 0     | completed  |
| 9   | queue-burst-3  | 00:23:57   | 00:00:00     | 0     | completed  |
| 10  | queue-burst-4  | 00:23:57   | 00:00:00     | 0     | completed  |
| 11  | queue-burst-5* | 00:23:57   | 00:00:00     | 0     | completed  |

**After process burst-6**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-7  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-8  | 00:23:57   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-9  | 00:23:57   | 00:00:00     | 0     | pending    |
| 4   | queue-fail     | 00:23:57   | 00:23:57     | 1     | failed     |
| 5   | queue-burst-0  | 00:23:57   | 00:00:00     | 0     | completed  |
| 6   | queue-burst-1  | 00:23:57   | 00:00:00     | 0     | completed  |
| 7   | queue-burst-2  | 00:23:57   | 00:00:00     | 0     | completed  |
| 8   | queue-burst-3  | 00:23:57   | 00:00:00     | 0     | completed  |
| 9   | queue-burst-4  | 00:23:57   | 00:00:00     | 0     | completed  |
| 10  | queue-burst-5  | 00:23:57   | 00:00:00     | 0     | completed  |
| 11  | queue-burst-6* | 00:23:57   | 00:00:00     | 0     | completed  |

**After process burst-7**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-8  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-9  | 00:23:57   | 00:00:00     | 0     | pending    |
| 3   | queue-fail     | 00:23:57   | 00:23:57     | 1     | failed     |
| 4   | queue-burst-0  | 00:23:57   | 00:00:00     | 0     | completed  |
| 5   | queue-burst-1  | 00:23:57   | 00:00:00     | 0     | completed  |
| 6   | queue-burst-2  | 00:23:57   | 00:00:00     | 0     | completed  |
| 7   | queue-burst-3  | 00:23:57   | 00:00:00     | 0     | completed  |
| 8   | queue-burst-4  | 00:23:57   | 00:00:00     | 0     | completed  |
| 9   | queue-burst-5  | 00:23:57   | 00:00:00     | 0     | completed  |
| 10  | queue-burst-6  | 00:23:57   | 00:00:00     | 0     | completed  |
| 11  | queue-burst-7* | 00:23:57   | 00:00:00     | 0     | completed  |

**After process burst-8**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-9  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue-fail     | 00:23:57   | 00:23:57     | 1     | failed     |
| 3   | queue-burst-0  | 00:23:57   | 00:00:00     | 0     | completed  |
| 4   | queue-burst-1  | 00:23:57   | 00:00:00     | 0     | completed  |
| 5   | queue-burst-2  | 00:23:57   | 00:00:00     | 0     | completed  |
| 6   | queue-burst-3  | 00:23:57   | 00:00:00     | 0     | completed  |
| 7   | queue-burst-4  | 00:23:57   | 00:00:00     | 0     | completed  |
| 8   | queue-burst-5  | 00:23:57   | 00:00:00     | 0     | completed  |
| 9   | queue-burst-6  | 00:23:57   | 00:00:00     | 0     | completed  |
| 10  | queue-burst-7  | 00:23:57   | 00:00:00     | 0     | completed  |
| 11  | queue-burst-8* | 00:23:57   | 00:00:00     | 0     | completed  |

**After process burst-9**
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-fail     | 00:23:57   | 00:23:57     | 1     | failed     |
| 2   | queue-burst-0  | 00:23:57   | 00:00:00     | 0     | completed  |
| 3   | queue-burst-1  | 00:23:57   | 00:00:00     | 0     | completed  |
| 4   | queue-burst-2  | 00:23:57   | 00:00:00     | 0     | completed  |
| 5   | queue-burst-3  | 00:23:57   | 00:00:00     | 0     | completed  |
| 6   | queue-burst-4  | 00:23:57   | 00:00:00     | 0     | completed  |
| 7   | queue-burst-5  | 00:23:57   | 00:00:00     | 0     | completed  |
| 8   | queue-burst-6  | 00:23:57   | 00:00:00     | 0     | completed  |
| 9   | queue-burst-7  | 00:23:57   | 00:00:00     | 0     | completed  |
| 10  | queue-burst-8  | 00:23:57   | 00:00:00     | 0     | completed  |
| 11  | queue-burst-9* | 00:23:57   | 00:00:00     | 0     | completed  |

### POSITIVE-MultipleFailures_RetryCount

**After 1st fail:**
| Pos | Name        | created_at | last_retry_at | retry | status  |
|-----|-------------|------------|--------------|-------|---------|
| 1   | queue-retry | 00:23:57   | 00:23:57     | 1     | failed  |

**After 2nd fail:**
| Pos | Name        | created_at | last_retry_at | retry | status  |
|-----|-------------|------------|--------------|-------|---------|
| 1   | queue-retry | 00:23:57   | 00:23:57     | 2     | failed  |

**After success:**
| Pos | Name        | created_at | last_retry_at | retry | status     |
|-----|-------------|------------|--------------|-------|------------|
| 1   | queue-retry*| 00:23:57   | 00:23:57     | 2     | completed  |

### POSITIVE-InterleavedSuccessFailure

**After queue1 fail:**
| Pos | Name    | created_at | last_retry_at | retry | status  |
|-----|---------|------------|--------------|-------|---------|
| 1   | queue2  | 00:23:57   | 00:00:00     | 0     | pending |
| 2   | queue1  | 00:23:57   | 00:23:57     | 1     | failed  |

**After queue2 success:**
| Pos | Name    | created_at | last_retry_at | retry | status     |
|-----|---------|------------|--------------|-------|------------|
| 1   | queue2* | 00:23:57   | 00:00:00     | 0     | completed  |
| 2   | queue1  | 00:23:57   | 00:23:57     | 1     | failed     |

**After insert queue3:**
| Pos | Name    | created_at | last_retry_at | retry | status     |
|-----|---------|------------|--------------|-------|------------|
| 1   | queue3  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue2  | 00:23:57   | 00:00:00     | 0     | completed  |
| 3   | queue1  | 00:23:57   | 00:23:57     | 1     | failed     |

**After queue1 retry success:**
| Pos | Name    | created_at | last_retry_at | retry | status     |
|-----|---------|------------|--------------|-------|------------|
| 1   | queue3  | 00:23:57   | 00:00:00     | 0     | pending    |
| 2   | queue2  | 00:23:57   | 00:00:00     | 0     | completed  |
| 3   | queue1* | 00:23:57   | 00:23:57     | 1     | completed  |

**After queue3 success:**
| Pos | Name    | created_at | last_retry_at | retry | status     |
|-----|---------|------------|--------------|-------|------------|
| 1   | queue2  | 00:23:57   | 00:00:00     | 0     | completed  |
| 2   | queue1  | 00:23:57   | 00:23:57     | 1     | completed  |
| 3   | queue3* | 00:23:57   | 00:00:00     | 0     | completed  |

### POSITIVE-AllQueuesFailedThenSucceed

**After fail-1:**
| Pos | Name   | created_at | last_retry_at | retry | status  |
|-----|--------|------------|--------------|-------|---------|
| 1   | fail2  | 00:23:57   | 00:00:00     | 0     | pending |
| 2   | fail3  | 00:23:57   | 00:00:00     | 0     | pending |
| 3   | fail1  | 00:23:57   | 00:23:57     | 1     | failed  |

**After fail-2:**
| Pos | Name   | created_at | last_retry_at | retry | status  |
|-----|--------|------------|--------------|-------|---------|
| 1   | fail3  | 00:23:57   | 00:00:00     | 0     | pending |
| 2   | fail1  | 00:23:57   | 00:23:57     | 1     | failed  |
| 3   | fail2  | 00:23:57   | 00:23:57     | 1     | failed  |

**After fail-3:**
| Pos | Name   | created_at | last_retry_at | retry | status  |
|-----|--------|------------|--------------|-------|---------|
| 1   | fail1  | 00:23:57   | 00:23:57     | 1     | failed  |
| 2   | fail2  | 00:23:57   | 00:23:57     | 1     | failed  |
| 3   | fail3  | 00:23:57   | 00:23:57     | 1     | failed  |

**After retry success-1:**
| Pos | Name   | created_at | last_retry_at | retry | status     |
|-----|--------|------------|--------------|-------|------------|
| 1   | fail1* | 00:23:57   | 00:23:57     | 1     | completed  |
| 2   | fail2  | 00:23:57   | 00:23:57     | 1     | failed     |
| 3   | fail3  | 00:23:57   | 00:23:57     | 1     | failed     |

**After retry success-2:**
| Pos | Name   | created_at | last_retry_at | retry | status     |
|-----|--------|------------|--------------|-------|------------|
| 1   | fail1  | 00:23:57   | 00:23:57     | 1     | completed  |
| 2   | fail2* | 00:23:57   | 00:23:57     | 1     | completed  |
| 3   | fail3  | 00:23:57   | 00:23:57     | 1     | failed     |

**After retry success-3:**
| Pos | Name   | created_at | last_retry_at | retry | status     |
|-----|--------|------------|--------------|-------|------------|
| 1   | fail1  | 00:23:57   | 00:23:57     | 1     | completed  |
| 2   | fail2  | 00:23:57   | 00:23:57     | 1     | completed  |
| 3   | fail3* | 00:23:57   | 00:23:57     | 1     | completed  |

**Legend:**
- The asterisk (*) marks the queue that was just processed and moved to the completed position in that step.
- Each table shows the queue order after each processing step, so you can follow the anti-starvation logic visually without referring to any test log.

## Use Cases
- Prototyping queue processing logic for distributed systems
- Demonstrating anti-starvation and fair retry mechanisms
- Educational sample for Go, GORM, and RabbitMQ integration

## Disclaimer
This is a **sample/demo project**. The shifting logic and queue processor are simplified for clarity and learning purposes. For production use, always review, adapt, and harden the logic to fit your system's requirements.