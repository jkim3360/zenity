# jetstream-router (JavaScript)

The same design as the [Go service](../go/README.md), ported file for file to Node so it can be read in JavaScript. It reads the live Bluesky Jetstream over one WebSocket, routes each event by `kind`, `commit.collection` and `commit.operation`, and hands it to one of four independent lanes. Each lane is a bounded queue with its own worker, so a slow or failing lane drops its own overflow (and counts it) without holding up the others.

| Lane | Events | Work (a JSON log line stands in for the real downstream action) |
|---|---|---|
| `post` | `app.bsky.feed.post` create | `keyword notification` when the post mentions a configured keyword |
| `engagement` | `app.bsky.feed.like` / `app.bsky.feed.repost` create | `engagement alert` when one post reaches `ENGAGEMENT_THRESHOLD` likes + reposts within `WINDOW_SECONDS` |
| `follow` | `app.bsky.graph.follow` create | `follow burst` when one account gains `FOLLOW_THRESHOLD` follows within `WINDOW_SECONDS` |
| `delete` | any delete | `retraction` (logged at debug level; the count is in `lane stats`) |

Every 10 s, and once more on shutdown, each lane logs `lane stats` with `queued`, `processed` and `dropped`.

| JS | Go | What it holds |
|---|---|---|
| `index.js` | `main.go` | Config from env, wiring, signal handling, periodic stats |
| `stream.js` | `stream.go` | WebSocket reader, `wantedCollections` filter, reconnect and `time_us` resume |
| `router.js` | `router.go` | `routeOf`, the bounded lanes and the `Dispatcher` |
| `handlers.js` | `handlers.go` | The four handlers, `WindowCounter`, keyword matching |
| `log.js` | `log/slog` | One JSON log line per record, with the same fields as Go |
| `router.test.js`, `handlers.test.js` | `router_test.go`, `handlers_test.go` | The same six tests |

## Prerequisites

Node 18.17 or later (for running it directly), Docker, [kind](https://kind.sigs.k8s.io/) and `kubectl`.

## Run it directly

From this `js/` folder:

```sh
npm install
npm test
KEYWORDS=the npm start
```

Ctrl-C stops the reader, lets each lane finish its queue and logs the final `lane stats`. Node exits 0; `npm` then reports the Ctrl-C as its own exit status, as it always does.

## Run it on kind

Run these from this `js/` folder.

1. Create a cluster (skip this if you already created it with the Go README; both apps can share it):
   ```sh
   kind create cluster --name jetstream
   ```
2. Build the image and load it into the cluster:
   ```sh
   docker build -t jetstream-router-js:dev .
   kind load docker-image jetstream-router-js:dev --name jetstream
   ```
3. Deploy and wait for it to come up:
   ```sh
   kubectl apply -f k8s/deployment.yaml
   kubectl rollout status deployment/jetstream-router-js
   ```
4. Watch it work. The first line after `starting` is `connected`, with the filtered URL. Keyword notifications for `bluesky` show up within seconds, and engagement alerts within about a minute (the first window has to fill):
   ```sh
   kubectl logs -f deployment/jetstream-router-js
   ```
   To see just the alerts, or the latest per-lane counts:
   ```sh
   kubectl logs deployment/jetstream-router-js | grep -E 'keyword notification|engagement alert|follow burst'
   kubectl logs deployment/jetstream-router-js | grep 'lane stats' | tail -4
   ```
5. Optional: watch a common word. Config is read at startup, so change the ConfigMap and restart. The old pod logs its final `lane stats` as it stops, and the new pod's notifications for `the` stream in (Ctrl-C to stop watching):
   ```sh
   kubectl patch configmap jetstream-router-js --type merge -p '{"data":{"KEYWORDS":"the"}}'
   kubectl rollout restart deployment/jetstream-router-js
   kubectl rollout status deployment/jetstream-router-js
   kubectl logs -f deployment/jetstream-router-js | grep 'keyword notification'
   ```
6. Clean up (the second command also removes the Go app if it is running there):
   ```sh
   kubectl delete -f k8s/deployment.yaml
   kind delete cluster --name jetstream
   ```

To run the image without Kubernetes: `docker run --rm -e KEYWORDS=the jetstream-router-js:dev`.

## Configuration

The same variables as Go, except the window:

| Variable | Default | Meaning |
|---|---|---|
| `JETSTREAM_URL` | `wss://jetstream2.us-east.bsky.network/subscribe` | Jetstream endpoint |
| `KEYWORDS` | `golang,kubernetes` (the ConfigMap adds `bluesky`) | Comma-separated, case-insensitive |
| `ENGAGEMENT_THRESHOLD` | `25` | Likes + reposts on one post within the window that raise an alert |
| `FOLLOW_THRESHOLD` | `10` | New follows of one account within the window that raise an alert |
| `WINDOW_SECONDS` | `60` | Window for both counters, in whole seconds |
| `QUEUE_SIZE` | `1000` | Events each lane can hold before it starts dropping |
| `LOG_LEVEL` | `info` | `debug` also logs every retraction |

An invalid value (for example `QUEUE_SIZE=abc` or `WINDOW_SECONDS=0`) stops startup with an error line and exit code 1. Nothing falls back to a default silently.

## Differences from Go

- **One thread.** Each lane is an async loop rather than a goroutine, so the lanes take turns on Node's event loop instead of running in parallel. A lane that is waiting on I/O (an `await`ed webhook, say) does not hold up the others, and each worker yields after every event so the socket keeps being read. A handler that burns CPU synchronously would stall everything, which in Go would only tie up its own goroutine. Under a flood the reader and the workers share that one thread, so JS drops more than Go does.
- **`WINDOW_SECONDS`** (an integer) instead of a Go duration such as `1m`.
- **A bigger image:** about 240 MB on `node:22-alpine`, against about 15 MB for the static Go binary on distroless. It runs as the image's `node` user (uid 1000).

The assumptions are the same as Go's: see [Assumptions](../go/README.md#assumptions).
