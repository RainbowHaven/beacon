# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=0.0.0
RUN COMMIT="$(git rev-parse --short=7 HEAD 2>/dev/null || echo unknown)" \
 && DATE="$(git show -s --format=%cs HEAD 2>/dev/null || date -u +%Y-%m-%d)" \
 && CGO_ENABLED=0 go build \
      -ldflags "-s -w -X github.com/magiconair/beacon/internal/version.Version=${VERSION} -X github.com/magiconair/beacon/internal/version.Commit=${COMMIT} -X github.com/magiconair/beacon/internal/version.Date=${DATE}" \
      -o /out/beacon ./cmd/beacon

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/beacon /app/beacon
EXPOSE 8080
USER nobody
ENTRYPOINT ["/app/beacon"]
CMD ["server"]
