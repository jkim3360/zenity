# jetstream-router (Go)

Reads the live Bluesky Jetstream over one WebSocket, works out each event's type from `kind`, `commit.collection` and `commit.operation`, and routes it to one of four lanes. A lane is a bounded queue with its own worker. If a lane falls behind, it drops and counts its own overflow without slowing the others down.

| Lane | Events | What it does (logged as a JSON line) |
|---|---|---|
| `post` | New posts | `keyword notification` when a post mentions a keyword |
| `engagement` | New likes and reposts | `engagement alert` when one post reaches `ENGAGEMENT_THRESHOLD` within `WINDOW` |
| `follow` | New follows | `follow burst` when one account reaches `FOLLOW_THRESHOLD` new followers within `WINDOW` |
| `delete` | Any delete | `retraction` (only at debug level; always counted in `lane stats`) |

Every 10 seconds, and again on shutdown, each lane logs `lane stats` with how many events are queued, processed and dropped.

## Prerequisites

Docker, [kind](https://kind.sigs.k8s.io/) and `kubectl`. You don't need Go installed, because tests and builds run in the `golang:1.25-alpine` image.

## Run it on kind

From this `go/` folder:

1. Run the tests. They run in the Go image as your user, so no root-owned files are left behind:
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
4. Deploy it:
   ```sh
   kubectl apply -f k8s/deployment.yaml
   kubectl rollout status deployment/jetstream-router
   ```
5. Watch the logs. You'll see `connected` right after startup, keyword notifications for `bluesky` within seconds, and engagement alerts within about a minute:
   ```sh
   kubectl logs -f deployment/jetstream-router
   ```
   To see only the alerts, or the latest lane stats:
   ```sh
   kubectl logs deployment/jetstream-router | grep -E 'keyword notification|engagement alert|follow burst'
   kubectl logs deployment/jetstream-router | grep 'lane stats' | tail -4
   ```
6. Optional: watch a common word. Config is read at startup, so update the ConfigMap and restart. The old pod logs its final stats on the way out, then the new pod's notifications stream in (Ctrl-C to stop):
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

After the `docker build` in step 3:

```sh
docker run --rm -e KEYWORDS=the jetstream-router:dev
```

Ctrl-C stops reading, lets each lane finish its queue, logs the final stats and exits 0. If you have Go 1.25 installed, `KEYWORDS=the go run .` does the same.

## Configuration

These are set in the ConfigMap in `k8s/deployment.yaml`:

| Variable | Default | What it does |
|---|---|---|
| `JETSTREAM_URL` | `wss://jetstream2.us-east.bsky.network/subscribe` | Where to connect |
| `KEYWORDS` | `golang,kubernetes` (the ConfigMap adds `bluesky`) | Comma-separated, case-insensitive |
| `ENGAGEMENT_THRESHOLD` | `25` | Likes + reposts on one post within the window to raise an alert |
| `FOLLOW_THRESHOLD` | `10` | New follows of one account within the window to raise an alert |
| `WINDOW` | `1m` | Window for both counters, as a Go duration |
| `QUEUE_SIZE` | `1000` | How many events each lane can hold before it starts dropping |
| `LOG_LEVEL` | `info` | `debug` also logs every retraction |

A bad value, like `QUEUE_SIZE=abc` or `WINDOW=-1m`, stops startup with an error and exit code 1. Nothing quietly falls back to a default.

## Assumptions

- Log lines stand in for the real downstream work, such as a push notification, a webhook or a cleanup job.
- Dropping events under overload is fine for notifications and rolling counts, as long as every drop is counted. The reader never waits on a lane.
- I only ask Jetstream for posts, likes, reposts and follows, so the delete lane only sees deletes of those. Jetstream also always sends `identity` and `account` events, and those are ignored.
- Keyword matching is a simple case-insensitive substring match, so `go` would also match `good`. Counters use fixed windows based on each event's timestamp, so a burst that straddles two windows can be missed.
- Post text is never logged. A notification only includes the author's DID, the record key and the matched keyword.
- It runs as a single replica. The resume point is kept in memory, so after a dropped connection it picks up 5 seconds before the last event it saw, but a pod restart starts from live. [DESIGN.md](../DESIGN.md) covers why, and what I'd change for production.
