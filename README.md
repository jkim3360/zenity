# Bluesky Jetstream event router

A small Go service that reads the live Bluesky Jetstream, works out what each event is, and sends it to the right worker. Each event type gets its own lane, which is a bounded queue with one worker:

- **Posts:** a notification when a post mentions a keyword.
- **Likes and reposts:** an alert when one post gets a burst of engagement.
- **Follows:** an alert when one account gets a burst of new followers.
- **Deletes:** handled on their own separate path.

If one lane falls behind, it drops and counts its own overflow, and the others keep going.

| Folder | What's there |
|---|---|
| [`go/`](go/README.md) | The submission: the Go service, tests, Dockerfile and kind manifest |
| [`js/`](js/README.md) | The same design in Node, file for file |
| [`DESIGN.md`](DESIGN.md) | How it works, why, and what I'd change for production |
| [`AI.md`](AI.md) | How I used AI to build it |

## Quick start (Go on kind)

You need Docker, [kind](https://kind.sigs.k8s.io/) and `kubectl`.

```sh
cd go
kind create cluster --name jetstream
docker build -t jetstream-router:dev .
kind load docker-image jetstream-router:dev --name jetstream
kubectl apply -f k8s/deployment.yaml
kubectl rollout status deployment/jetstream-router
kubectl logs -f deployment/jetstream-router
```

Within a minute you'll see keyword notifications, engagement alerts, and each lane's stats every 10 seconds. To clean up, run `kubectl delete -f k8s/deployment.yaml` and `kind delete cluster --name jetstream`. The full steps and config are in [go/README.md](go/README.md).
