# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.27-alpine AS build

WORKDIR /src

# Dependencies first so a source-only change does not re-download the module cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
# CGO is off: the SQLite driver is pure Go, so the result runs on a bare image.
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/sysvo/knot/internal/api.Version=${VERSION}" \
      -o /out/knot ./cmd/knot

# ---- runtime ----
FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 -h /data knot \
 && mkdir -p /data/uploads \
 && chown -R knot:knot /data

COPY --from=build /out/knot /usr/local/bin/knot

# A directory created here (not at runtime) is what makes a freshly created
# volume inherit the right ownership for the unprivileged user.
WORKDIR /data
USER knot

EXPOSE 8080

ENTRYPOINT ["knot"]
CMD ["serve"]
