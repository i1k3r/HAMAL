FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/hamal ./cmd/lan-drop

FROM debian:bookworm-slim
# 10001:10001 stays the owner of /data in the image so that new named volumes
# start out owned by it, exactly as with earlier images. The container starts as
# root only so docker-entrypoint.sh can pick the right non-root UID:GID for the
# mounted data (see that file); the server itself always runs without root,
# dropped with setpriv from util-linux, which is already part of the base image.
RUN groupadd --gid 10001 hamal \
    && useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin hamal \
    && mkdir -p /data \
    && chown -R 10001:10001 /data
WORKDIR /app
COPY --from=build /out/hamal /app/hamal
COPY --chmod=755 docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
EXPOSE 7700
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/usr/local/bin/docker-entrypoint.sh", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
