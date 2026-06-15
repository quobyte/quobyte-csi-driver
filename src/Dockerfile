
ARG TARGETOS
ARG TARGETARCH
ARG APP_NAME=quobyte-csi

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG APP_NAME

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# TODO(venkat): strip debug symbols to reduce the image size
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -o /app/${APP_NAME} cmd/main.go

FROM alpine:latest AS root-prep
ARG APP_NAME
RUN mkdir /final_root/; \
    cd /final_root; \
    mkdir -p var tmp run bin; \
    ln -s var/run run; \
    chmod -R 755 var tmp run bin
COPY --from=builder /app/${APP_NAME} /final_root/bin/quobyte-csi

FROM scratch
ARG APP_NAME
COPY --from=root-prep /final_root/ /
COPY --from=builder /app/${APP_NAME} /bin/quobyte-csi
ENTRYPOINT ["/bin/quobyte-csi"]
