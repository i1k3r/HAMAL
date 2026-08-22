FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/hamal ./cmd/lan-drop

FROM debian:bookworm-slim
RUN groupadd --gid 10001 hamal \
    && useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin hamal \
    && mkdir -p /data \
    && chown -R 10001:10001 /data
WORKDIR /app
COPY --from=build /out/hamal /app/hamal
USER 10001:10001
EXPOSE 7700
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/app/hamal", "healthcheck"]
ENTRYPOINT ["/app/hamal"]
