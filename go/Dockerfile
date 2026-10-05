FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /jetstream-router .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /jetstream-router /jetstream-router
ENTRYPOINT ["/jetstream-router"]
