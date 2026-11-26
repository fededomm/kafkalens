# UI
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npx ng build

# server binary with the UI embedded (browser mode: no Wails, no CGo), cross-compiled: no emulation for arm64
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /kafkalens . && mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /kafkalens /kafkalens
COPY --from=build --chown=65532:65532 /data /data
# clusters.json and mcp-token live in /data
ENV KAFKALENS_SERVER_ADDR=0.0.0.0:8093 \
    KAFKALENS_SERVER_OPENBROWSER=false \
    KAFKALENS_CLUSTERS=/data/clusters.json \
    KAFKALENS_LOG_LEVEL=info
VOLUME /data
EXPOSE 8093
ENTRYPOINT ["/kafkalens"]
