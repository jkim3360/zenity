# AI.md

## Setup

- **Tool:** Claude Code (CLI), in auto mode for the build.
- **Model:** Claude Opus 5.5.
- **Config:** no `CLAUDE.md`, rules files, custom skills or MCP servers. The plan file (`tmp/PLAN.md`) did that job instead: the acceptance criteria, design, file layout, milestones and sandbox rules.
- **Sandbox:** the agent could only read and write inside the project folder. Go ran in the `golang:1.25-alpine` container because my local Go is older, and tool caches and the kubeconfig lived in `tmp/`. The only side effects outside the folder were Docker images and a temporary kind cluster.

## How I worked

1. **Plan first.** I ran plan mode on the brief and turned it into `tmp/PLAN.md`: 22 acceptance criteria (the brief's plus my own requirements), the design, the file layout and a verification plan.
2. **Then one build run.** A single `/goal` built the Go service and then the JS port, and tested both: unit tests, a scripted fake stream, a flood, bad config, the live stream, and kind deployments that followed the READMEs exactly. It fixed what the tests found and recorded every result in a test report.

## The prompts

**Planning request:**

> - As lightweight as possible. No cruft or unnecessary logic. Follow Go conventions in `go/` and JS conventions in `js/`.
> - Build it in Go + Kubernetes (the submission), then build the same design in JavaScript in `js/`, file for file, so I can read it in the language I know best.
> - A one-sentence comment above every function saying what it does.
> - Take the acceptance criteria from the brief and use them as the test standard.
> - Run it for real, locally and on kind, to confirm events flow and the logs are right.
> - Read and write only inside `<project folder>`.
> - End with a plain-language summary anyone can follow.

**Build prompt** (verbatim apart from the folder path):

> /goal Build the Zenity take-home in `<project folder>` as specified in tmp/PLAN.md (brief: tmp/Zenity_Senior_SWE_Take-Home_Candidate.pdf): a Go service in go/ and a file-for-file JavaScript port in js/ that read Bluesky Jetstream over one WebSocket, route each event by kind/collection/operation to four independent bounded lanes (post keyword notification, like/repost engagement alert, follow burst, delete retraction) with drop-and-count backpressure, wantedCollections filtering and time_us resume, each with focused tests, a Dockerfile and kind manifests, plus README.md, DESIGN.md and AI.md. Keep it lightweight with no cruft, follow Go and JS conventions, and put a one-sentence comment above every function. Follow the plan's sandbox rules exactly: read and write only inside this directory; Docker images, containers and one temporary kind cluster are the only outside side effects; Go runs only in the golang:1.25-alpine container; npm cache and KUBECONFIG live in tmp/. Done when every acceptance criterion AC1-AC22 in tmp/PLAN.md is marked PASS in tmp/test-report.md with the command and output that proves it; go test, go vet, gofmt and npm test pass; Go and JS give identical results on the scripted fake-stream scenario, including the exact resume cursor; both versions ran against the live stream and were deployed to kind by following their READMEs verbatim, with all four lanes processing events and alerts visible in the logs; the work is in milestone commits in a local git repo with a clean working tree and no remote; no kind cluster is left running; and the final message gives a plain-language summary.

## Decisions

Made in the plan: four lanes, each a bounded queue with one worker; drop and count instead of blocking; fixed windows; a resume cursor kept in memory and rewound 5 seconds; one replica; filtering at the source; fail-fast config; no post text in the logs.

Made during the build, and covered by tests:
- Skip a message that can't be decoded instead of dropping the connection. Otherwise the 5-second replay would hit the same bad message on every reconnect.
- Base the windows on each event's own timestamp instead of the clock.
- Count failed events as processed, so queued + processed + dropped always adds up.
- Reject a bad `JETSTREAM_URL` or an empty keyword list at startup.
- Turn off the service-account token on the pod.

## Where the agent helped

- Writing both versions and keeping them in sync, down to the log fields. The scripted test gives identical results in Go and JS.
- I am most comfortable with Javascript, so I created a JS version of the task.
- Building the checks alongside the code, including a fake Jetstream server and README steps run exactly as written.
- Catching mistakes that only show up when you run things. One README step counted notifications before the new pod had connected, and another said `npm start` exits 0 on Ctrl-C when npm actually reports 130. Both were fixed.
- Measuring instead of assuming. The filter numbers in DESIGN.md come from comparing the filtered and unfiltered feeds side by side.

## Where I steered it

- I set the scope up front: the smallest thing that answers the brief, plus a JS port so I could read the design in the language I know best.
- I turned the brief into acceptance criteria and required real runs, live and on kind, before anything counted as done.
- I settled the big tradeoffs in the plan (drop vs block, windowing, one replica), so the agent implemented them rather than chose them.
- I kept it sandboxed to one folder, with Go in a container and all tool state in `tmp/`.
