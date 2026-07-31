# syntax=docker/dockerfile:1

FROM golang:1.25-bookworm AS build
WORKDIR /src/agent
COPY agent/go.mod agent/go.sum ./
RUN go mod download
COPY agent/ ./
RUN CGO_ENABLED=0 go build -o /out/loadgen ./cmd/loadgen

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/loadgen /loadgen
ENTRYPOINT ["/loadgen"]
