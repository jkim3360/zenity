# Design

## The shape

```
Jetstream WS ──► reader ──► routeOf(event) ──┬─► [post queue]       ─► post worker        (keyword match)
 (one conn,       (decode,    (kind +        ├─► [engagement queue] ─► engagement worker  (rolling counts)
  cursor)         track       collection +   ├─► [follow queue]     ─► follow worker      (burst detection)
                  time_us)    operation)     └─► [delete queue]     ─► delete worker      (retraction path)
```

One process, one WebSocket connection, four lanes. A lane is a bounded queue with one worker that owns that lane's state. The reader decodes each event, notes its timestamp, picks a lane and hands the event off without waiting.

## Key decisions

**Routing lives in one function.** `routeOf` looks at the event's kind, operation and collection, and returns a lane or nothing. Any delete goes to the delete lane, creates go by collection, and everything else is ignored. It's the only place that knows the event types, so adding one means one constant, one case and one handler.

**Each lane is isolated.** Every lane has its own queue, worker and counters. If a handler crashes, the error is logged and the lane moves on to the next event. If a handler gets stuck, only its own queue fills up. Both cases have a test.

**When a lane is full, drop and count. Never block the reader.** If the reader waited on a full lane, the slowest lane would set the pace for all of them, which is the coupling the brief asks to avoid. Dropping is fine for notifications and rolling counts, as long as it's visible, so every lane reports how many events are queued, processed and dropped every 10 seconds. For something like payments that would be the wrong call; there the answer is a durable queue, not blocking. In a flood test (200,000 likes as fast as possible, with queues of 10), Go dropped 1.3% of the likes while the post lane next to it still delivered every notification.

**No locks.** Each lane's state is only touched by its own worker, so events within a lane stay in order. If one lane ever needs more throughput, I'd split it by key across several workers rather than share a pool.

**Fixed windows on event time.** Counts reset each window, and an alert fires once, when a key reaches the threshold. Windows follow each event's own timestamp rather than the clock, so tests are deterministic and a replay doesn't get squeezed into one window. The tradeoff is that a burst straddling two windows can be missed. A sliding window would fix that.

**Filtering at the source.** I ask Jetstream for only posts, likes, reposts and follows. I expected this to cut a lot of traffic, but when I measured it, the filtered feed still kept 96% of the events, because those four types are most of Bluesky's activity. It's still worth doing. It limits the delete lane to things we actually track, it keeps unknown types out of the router, and it matters a lot for narrower consumers: a follow-only service would keep about 6% of the feed.

**Resume from the last timestamp.** After a disconnect, the reader waits 2 seconds and reconnects with a cursor 5 seconds before the last event it saw. Replaying a few seconds means nothing is lost, at the cost of maybe counting a few events twice. The cursor is kept in memory, so a pod restart starts from live. That's a known limit of the prototype.

**One replica.** There's one stream, so a second replica would just process everything twice. The deployment uses `Recreate`, so a rollout never runs two at once. The pod runs as non-root with a read-only filesystem, no Linux capabilities and a memory limit.

**Fail fast, and keep content out of the logs.** Bad config stops startup instead of falling back to a default. Post text is only used for keyword matching and is never logged.

## Live numbers

I measured the live stream over two minutes on a Sunday morning, US Eastern time (rates vary through the day). It ran at about 400 events per second: roughly 320 likes and reposts, 50 posts, 23 follows and 15 deletes per second. Every queue stayed at zero with no drops, in both Go and JavaScript. The Go version handled about 100,000 events per second in the flood test, so there's plenty of headroom.

## Alternatives I considered

| Option | Why not, for now |
|---|---|
| A message broker (Kafka, NATS) between the reader and the workers | It's the right production answer, but too much setup for a prototype. The lane boundary is where a broker topic would go. |
| One shared worker pool | Simpler, but one slow event type could tie up every worker. |
| Blocking the reader when a lane is full | No data loss, but the slowest lane would slow everything down. |
| Several workers per lane, split by key | More throughput per lane. Not needed at 400 events per second, but it's the next step if a lane falls behind. |
| One deployment per event type, each with its own connection | Real independence, but more connections and cursors to manage, and the likes-and-reposts lane would still carry about 80% of the traffic on its own. A reasonable step before adding a broker. |

## Getting to production

1. **Add a broker.** The reader publishes to one topic per lane, and each lane becomes its own deployment that scales on its backlog. Lanes then fail and scale independently, as separate services.
2. **Save the cursor somewhere durable,** so a restart picks up where it left off.
3. **Deduplicate on the event's DID, collection and record key,** so a replay never double-counts or sends a duplicate notification.
4. **Send to real outputs,** with timeouts, retries and a dead-letter queue per lane. Drops should trigger an alert, not just bump a counter.
5. **Add metrics and health checks:** queue depth, drops, handler latency and how far behind the stream we are. Add readiness and liveness probes, plus a timeout for a connection that goes quiet.
6. **Use sliding windows and live config,** so thresholds and keywords can change without a restart.
7. **Make reconnects smarter,** with backoff, jitter and failover across Jetstream hosts.

## Stack

I'd keep Go and Kubernetes. A lane maps directly onto a channel and a goroutine, the image is a 15 MB static binary, and once a broker is in place Kubernetes gives each lane its own deployment and autoscaling. The JavaScript port shows the difference. It's the same design with identical results on the scripted test, but it runs on one thread, so under a flood it dropped 66% of the likes against Go's 1.3%. For production, the change that matters is adding the broker, not switching languages.
