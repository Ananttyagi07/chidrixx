# syntax=docker/dockerfile:1

FROM golang:1.25-bookworm AS build
WORKDIR /src/agent
COPY agent/go.mod agent/go.sum ./
RUN go mod download
COPY agent/ ./
RUN CGO_ENABLED=0 go build -o /out/kharcha ./cmd/kharcha

# k3s bundles a working `kubectl` (as a symlink to the k3s multi-call
# binary) — reusing it avoids a separate network fetch of a standalone
# kubectl release just for this image. A production image would ship a
# slimmer, purpose-built kubectl binary instead.
FROM rancher/k3s:v1.35.5-k3s1 AS kubectl

FROM debian:12-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/kharcha /app/kharcha
COPY --from=kubectl /bin/k3s /usr/local/bin/k3s
RUN ln -s /usr/local/bin/k3s /usr/local/bin/kubectl
ENV PATH="/usr/local/bin:${PATH}"

COPY bpf/flow_cgroup.o /app/bpf/flow_cgroup.o
COPY pricebook/ /app/pricebook/

ENTRYPOINT ["/app/kharcha"]
CMD ["-bpf-object=/app/bpf/flow_cgroup.o", "-pricebook=/app/pricebook/aws.yaml", "-html-out=/app/report.html"]
