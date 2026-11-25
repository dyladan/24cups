# syntax=docker/dockerfile:1
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS server
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /24cups ./cmd/24cups \
 && mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /24cups /24cups
# The database lives here; owned by the image's nonroot user (65532) so a
# fresh named volume inherits writable permissions.
COPY --from=server --chown=65532:65532 /data /data
VOLUME /data
ENV DB_PATH=/data/advent.db ADDR=:8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/24cups", "healthcheck"]
ENTRYPOINT ["/24cups"]
