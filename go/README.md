# jetstream-router (Go)

Reads the live Bluesky Jetstream over one WebSocket, works out what each event is from `kind`, `commit.collection` and `commit.operation`, and hands it to one of four independent lanes. Each lane is a bounded queue with its own worker, so a slow or failing lane drops its own overflow (and counts it) without holding up the others.

| Lane | Events | Work (a JSON log line stands in for the real downstream action) |
|---|---|---|
| `post` | `app.bsky.feed.post` create | `keyword notification` when the post mentions a configured keyword |
| `engagement` | `app.bsky.feed.like` / `app.bsky.feed.repost` create | `engagement alert` when one post reaches `ENGAGEMENT_THRESHOLD` likes + reposts within `WINDOW` |
| `follow` | `app.bsky.graph.follow` create | `follow burst` when one account gains `FOLLOW_THRESHOLD` follows within `WINDOW` |
| `delete` | any delete | `retraction` (logged at debug level; the count is in `lane stats`) |

Every 10 s, and once more on shutdown, each lane logs `lane stats` with `queued`, `processed` and `dropped`.

## Prerequisites

Docker, [kind](https://kind.sigs.k8s.io/) and `kubectl`. No local Go install is needed: tests and builds run in the `golang:1.25-alpine` image.

## Run it on kind

Run these from this `go/` folder.

1. Run the tests (in the Go 1.25 image, as your user, so no root-owned files are left behind):
   ```sh
   docker run --rm -v "$PWD:/src" -w /src -u "$(id -u):$(id -g)" -e GOCACHE=/tmp/gocache -e GOMODCACHE=/tmp/gomod golang:1.25-alpine go test -v ./...
   ```
2. Create a cluster:
   ```sh
   kind create cluster --name jetstream
   ```
3. Build the image and load it into the cluster:
   ```sh
   docker build -t jetstream-router:dev .
   kind load docker-image jetstream-router:dev --name jetstream
   ```
4. Deploy and wait for it to come up:
   ```sh
   kubectl apply -f k8s/deployment.yaml
   kubectl rollout status deployment/jetstream-router
   ```
5. Watch it work. The first line after `starting` is `connected`, with the filtered URL. Keyword notifications for `bluesky` show up within seconds, and engagement alerts within about a minute (the first window has to fill):
   ```sh
   kubectl logs -f deployment/jetstream-router
   ```
   To see just the alerts, or the latest per-lane counts:
   ```sh
   kubectl logs deployment/jetstream-router | grep -E 'keyword notification|engagement alert|follow burst'
   kubectl logs deployment/jetstream-router | grep 'lane stats' | tail -4
   ```
6. Optional: watch a common word. Config is read at startup, so change the ConfigMap and restart. The old pod logs its final `lane stats` as it stops, and the new pod's notifications for `the` stream in (Ctrl-C to stop watching):
   ```sh
   kubectl patch configmap jetstream-router --type merge -p '{"data":{"KEYWORDS":"the"}}'
   kubectl rollout restart deployment/jetstream-router
   kubectl rollout status deployment/jetstream-router
   kubectl logs -f deployment/jetstream-router | grep 'keyword notification'
   ```
7. Clean up:
   ```sh
   kubectl delete -f k8s/deployment.yaml
   kind delete cluster --name jetstream
   ```

## Run it without Kubernetes

After step 3's `docker build`:

```sh
docker run --rm -e KEYWORDS=the jetstream-router:dev
```

Ctrl-C stops the reader, lets each lane finish its queue, logs the final `lane stats` and exits 0. With Go 1.25 installed, `KEYWORDS=the go run .` does the same.

## Configuration

Environment variables, set from the ConfigMap in `k8s/deployment.yaml`:

| Variable | Default | Meaning |
|---|---|---|
| `JETSTREAM_URL` | `wss://jetstream2.us-east.bsky.network/subscribe` | Jetstream endpoint |
| `KEYWORDS` | `golang,kubernetes` (the ConfigMap adds `bluesky`) | Comma-separated, case-insensitive |
| `ENGAGEMENT_THRESHOLD` | `25` | Likes + reposts on one post within `WINDOW` that raise an alert |
| `FOLLOW_THRESHOLD` | `10` | New follows of one account within `WINDOW` that raise an alert |
| `WINDOW` | `1m` | Go duration used by both counters |
| `QUEUE_SIZE` | `1000` | Events each lane can hold before it starts dropping |
| `LOG_LEVEL` | `info` | `debug` also logs every retraction |

An invalid value (for example `QUEUE_SIZE=abc` or `WINDOW=-1m`) stops startup with an error line and exit code 1. Nothing falls back to a default silently.

## Assumptions

- Log lines stand in for the real downstream work (a push notification, an alerting webhook, a cleanup job).
- Dropping events when a lane is overloaded is acceptable for this workload (notifications and rolling counts, not a ledger), as long as every drop is counted and visible. The reader never waits on a lane.
- The server-side `wantedCollections` filter asks only for posts, likes, reposts and follows, so the delete lane sees deletes of those four. Jetstream always sends `identity` and `account` events; they arrive and are ignored.
- Keyword matching is a case-insensitive substring match (`go` would match `good`). Counters use a tumbling window on the event's `time_us`, so a burst that straddles a window boundary can be missed.
- Post text is never logged. Notifications carry the author DID, the record key and the matched keyword only.
- One replica. The resume cursor lives in memory: a reconnect inside the process resumes from the last `time_us` minus 5 s, but a pod restart starts from live. See [DESIGN.md](../DESIGN.md) for why and what production would change.
