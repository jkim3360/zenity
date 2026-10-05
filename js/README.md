# jetstream-router (JavaScript)

The same design as the [Go service](../go/README.md), ported file for file to Node so the two can be read side by side. It has the same four lanes, the same log lines and the same six tests.

| JS | Go | What's in it |
|---|---|---|
| `index.js` | `main.go` | Config, wiring, shutdown and periodic stats |
| `stream.js` | `stream.go` | WebSocket reader, collection filter, reconnect and resume |
| `router.js` | `router.go` | `routeOf`, the lanes and the dispatcher |
| `handlers.js` | `handlers.go` | The four handlers, the window counter and keyword matching |
| `log.js` | `log/slog` | JSON log lines with the same fields as Go |
| `router.test.js`, `handlers.test.js` | `router_test.go`, `handlers_test.go` | The same six tests |

## Prerequisites

Node 18.17 or later to run it directly. Docker, [kind](https://kind.sigs.k8s.io/) and `kubectl` for the cluster.

## Run it directly

From this `js/` folder:

```sh
npm install
npm test
KEYWORDS=the npm start
```

Ctrl-C stops reading, lets each lane finish its queue and logs the final stats. Node exits 0, but npm then reports the Ctrl-C as its own exit code, which is normal for npm.

## Run it on kind

From this `js/` folder:

1. Create a cluster. Skip this if you already made one with the Go README, since both apps can share it:
   ```sh
   kind create cluster --name jetstream
   ```
2. Build the image and load it into the cluster:
   ```sh
   docker build -t jetstream-router-js:dev .
   kind load docker-image jetstream-router-js:dev --name jetstream
   ```
3. Deploy it:
   ```sh
   kubectl apply -f k8s/deployment.yaml
   kubectl rollout status deployment/jetstream-router-js
   ```
4. Watch the logs. You'll see `connected` right after startup, keyword notifications for `bluesky` within seconds, and engagement alerts within about a minute:
   ```sh
   kubectl logs -f deployment/jetstream-router-js
   ```
   To see only the alerts, or the latest lane stats:
   ```sh
   kubectl logs deployment/jetstream-router-js | grep -E 'keyword notification|engagement alert|follow burst'
   kubectl logs deployment/jetstream-router-js | grep 'lane stats' | tail -4
   ```
5. Optional: watch a common word. Config is read at startup, so update the ConfigMap and restart. The old pod logs its final stats on the way out, then the new pod's notifications stream in (Ctrl-C to stop):
   ```sh
   kubectl patch configmap jetstream-router-js --type merge -p '{"data":{"KEYWORDS":"the"}}'
   kubectl rollout restart deployment/jetstream-router-js
   kubectl rollout status deployment/jetstream-router-js
   kubectl logs -f deployment/jetstream-router-js | grep 'keyword notification'
   ```
6. Clean up. Deleting the cluster also removes the Go app if it's running there:
   ```sh
   kubectl delete -f k8s/deployment.yaml
   kind delete cluster --name jetstream
   ```

To run the image without Kubernetes: `docker run --rm -e KEYWORDS=the jetstream-router-js:dev`.

## Configuration

Configuration works the same as in Go (see the [Go config table](../go/README.md#configuration)), except that the window is `WINDOW_SECONDS`, a whole number of seconds that defaults to `60`. A bad value stops startup with an error and exit code 1.

## Differences from Go

- **One thread.** Each lane is an async loop instead of a goroutine, so the lanes take turns rather than running in parallel. Each worker yields after every event, so the socket keeps getting read. That's plenty at live rates, but under a flood JS drops far more than Go: 66% vs 1.3% in my flood test.
- **`WINDOW_SECONDS`** instead of a Go duration like `1m`.
- **A bigger image:** about 240 MB on `node:22-alpine`, against about 15 MB for Go on distroless. It runs as the image's `node` user (uid 1000).

The assumptions are the same as Go's: see [Assumptions](../go/README.md#assumptions).
