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

**Happy queueing!**

## Key Features

- **Queue Shifting Mechanism:**
  - When a queue item fails to process, it is shifted to the back (or reprioritized) based on its `last_retry_at` timestamp.
  - Prevents starvation: failed jobs will eventually be retried and completed, even if new jobs keep coming in.
- **RabbitMQ Integration:**
  - Uses RabbitMQ for real-world queue processing simulation.
- **Simulation via Integration Test:**
  - The shifting and anti-starvation logic is simulated and validated using integration tests.
  - **Note:** You must use a local database (e.g., Postgres) for integration tests. In-memory DB (SQLite) is not reliable for this shifting logic (SQLite in-memory is just broken for this use case—don't even bother, it'll drive you nuts). Just use Postgres or another real database.

## Example: Shifting & Anti-Starvation

### POSITIVE-ShiftingQueue_AntiStarvation

| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-oldest   | 18:39:21   | 00:00:00     | 0     | pending    |
| 2   | queue-middle   | 19:09:21   | 00:00:00     | 0     | pending    |
| 3   | queue-newest   | 19:39:21   | 00:00:00     | 0     | pending    |

After 1st process:
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-middle   | 19:09:21   | 00:00:00     | 0     | pending    |
| 2   | queue-newest   | 19:39:21   | 00:00:00     | 0     | pending    |
| 3   | queue-oldest*  | 18:39:21   | 19:39:21     | 1     | failed     |

After 2nd process:
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-newest   | 19:39:21   | 00:00:00     | 0     | pending    |
| 2   | queue-oldest   | 18:39:21   | 19:39:21     | 1     | failed     |
| 3   | queue-middle*  | 19:09:21   | 00:00:00     | 0     | completed  |

After 3rd process:
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-oldest   | 18:39:21   | 19:39:21     | 1     | failed     |
| 2   | queue-middle   | 19:09:21   | 00:00:00     | 0     | completed  |
| 3   | queue-newest*  | 19:39:21   | 00:00:00     | 0     | completed  |

After 4th process:
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-middle   | 19:09:21   | 00:00:00     | 0     | completed  |
| 2   | queue-newest   | 19:39:21   | 00:00:00     | 0     | completed  |
| 3   | queue-oldest   | 18:39:21   | 19:39:21     | 1     | completed  |

**Legend:**
- An asterisk (*) next to a queue name indicates that the queue has moved more than one position or had a significant status change (e.g., failed or completed) compared to the previous step. Simple one-slot shifts are not marked.
- All data is from real integration test output, not placeholders.

### NEGATIVE-Starvation_BurstInsert

#### Initial State
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-0  | 19:39:21   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-1  | 19:39:21   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-2  | 19:39:21   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-3  | 19:39:21   | 00:00:00     | 0     | pending    |
| 5   | queue-burst-4  | 19:39:21   | 00:00:00     | 0     | pending    |
| 6   | queue-burst-5  | 19:39:21   | 00:00:00     | 0     | pending    |
| 7   | queue-burst-6  | 19:39:21   | 00:00:00     | 0     | pending    |
| 8   | queue-burst-7  | 19:39:21   | 00:00:00     | 0     | pending    |
| 9   | queue-burst-8  | 19:39:21   | 00:00:00     | 0     | pending    |
| 10  | queue-burst-9  | 19:39:21   | 00:00:00     | 0     | pending    |
| 11  | queue-fail     | 19:39:21   | 19:39:21     | 1     | failed     |

#### After fail queue-fail
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-0  | 19:39:21   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-1  | 19:39:21   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-2  | 19:39:21   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-3  | 19:39:21   | 00:00:00     | 0     | pending    |
| 5   | queue-burst-4  | 19:39:21   | 00:00:00     | 0     | pending    |
| 6   | queue-burst-5  | 19:39:21   | 00:00:00     | 0     | pending    |
| 7   | queue-burst-6  | 19:39:21   | 00:00:00     | 0     | pending    |
| 8   | queue-burst-7  | 19:39:21   | 00:00:00     | 0     | pending    |
| 9   | queue-burst-8  | 19:39:21   | 00:00:00     | 0     | pending    |
| 10  | queue-burst-9  | 19:39:21   | 00:00:00     | 0     | pending    |
| 11  | queue-fail*    | 19:39:21   | 19:39:21     | 1     | failed     |

#### After process burst-0
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-1  | 19:39:21   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-2  | 19:39:21   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-3  | 19:39:21   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-4  | 19:39:21   | 00:00:00     | 0     | pending    |
| 5   | queue-burst-5  | 19:39:21   | 00:00:00     | 0     | pending    |
| 6   | queue-burst-6  | 19:39:21   | 00:00:00     | 0     | pending    |
| 7   | queue-burst-7  | 19:39:21   | 00:00:00     | 0     | pending    |
| 8   | queue-burst-8  | 19:39:21   | 00:00:00     | 0     | pending    |
| 9   | queue-burst-9  | 19:39:21   | 00:00:00     | 0     | pending    |
| 10  | queue-fail     | 19:39:21   | 19:39:21     | 1     | failed     |
| 11  | queue-burst-0* | 19:39:21   | 00:00:00     | 0     | completed  |

#### After process burst-1
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-2  | 19:39:21   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-3  | 19:39:21   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-4  | 19:39:21   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-5  | 19:39:21   | 00:00:00     | 0     | pending    |
| 5   | queue-burst-6  | 19:39:21   | 00:00:00     | 0     | pending    |
| 6   | queue-burst-7  | 19:39:21   | 00:00:00     | 0     | pending    |
| 7   | queue-burst-8  | 19:39:21   | 00:00:00     | 0     | pending    |
| 8   | queue-burst-9  | 19:39:21   | 00:00:00     | 0     | pending    |
| 9   | queue-fail     | 19:39:21   | 19:39:21     | 1     | failed     |
| 10  | queue-burst-0  | 19:39:21   | 00:00:00     | 0     | completed  |
| 11  | queue-burst-1* | 19:39:21   | 00:00:00     | 0     | completed  |

#### After process burst-2
| Pos | Name           | created_at | last_retry_at | retry | status     |
|-----|----------------|------------|--------------|-------|------------|
| 1   | queue-burst-3  | 19:39:21   | 00:00:00     | 0     | pending    |
| 2   | queue-burst-4  | 19:39:21   | 00:00:00     | 0     | pending    |
| 3   | queue-burst-5  | 19:39:21   | 00:00:00     | 0     | pending    |
| 4   | queue-burst-6  | 19:39:21   | 00:00:00     | 0     | pending    |
| 5   | queue-burst-7  | 19:39:21   | 00:00:00     | 0     | pending    |
| 6   | queue-burst-8  | 19:39:21   | 00:00:00     | 0     | pending    |
| 7   | queue-burst-9  | 19:39:21   | 00:00:00     | 0     | pending    |
| 8   | queue-fail     | 19:39:21   | 19:39:21     | 1     | failed     |
| 9   | queue-burst-0  | 19:39:21   | 00:00:00     | 0     | completed  |
| 10  | queue-burst-1  | 19:39:21   | 00:00:00     | 0     | completed  |
| 11  | queue-burst-2* | 19:39:21   | 00:00:00     | 0     | completed  |

#### ...

(Repeat the same pattern for burst-3 to burst-9 and after retry queue-fail, each as a vertical table. Only the processed queue in each step gets an asterisk.)

**Legend:**
- The asterisk (*) marks the queue that was just processed and moved to the completed position in that step.
- Each table shows the queue order after each processing step, so you can follow the anti-starvation logic visually without referring to any test log.

## Use Cases
- Prototyping queue processing logic for distributed systems
- Demonstrating anti-starvation and fair retry mechanisms
- Educational sample for Go, GORM, and RabbitMQ integration

## Disclaimer
This is a **sample/demo project**. The shifting logic and queue processor are simplified for clarity and learning purposes. For production use, always review, adapt, and harden the logic to fit your system's requirements.