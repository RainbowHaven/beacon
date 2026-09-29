# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# railway up does not upload .git, so commit/date usually come from version.stamp
# (written by CI) or Railway's RAILWAY_GIT_COMMIT_SHA build arg.
ARG VERSION=0.0.0
ARG GIT_COMMIT=
ARG GIT_DATE=
ARG RAILWAY_GIT_COMMIT_SHA=
RUN set -eu; \
  V="${VERSION}"; \
  C="${GIT_COMMIT}"; \
  D="${GIT_DATE}"; \
  if [ -f version.stamp ]; then \
    # shellcheck disable=SC1091
    . ./version.stamp; \
    V="${VERSION:-$V}"; \
    C="${COMMIT:-$C}"; \
    D="${DATE:-$D}"; \
  fi; \
  if [ -z "$C" ] && [ -n "${RAILWAY_GIT_COMMIT_SHA}" ]; then C="${RAILWAY_GIT_COMMIT_SHA}"; fi; \
  if [ -z "$C" ] && git rev-parse --short=7 HEAD >/dev/null 2>&1; then C="$(git rev-parse --short=7 HEAD)"; fi; \
  if [ -z "$D" ] && git show -s --format=%cs HEAD >/dev/null 2>&1; then D="$(git show -s --format=%cs HEAD)"; fi; \
  : "${V:=0.0.0}"; \
  : "${C:=unknown}"; \
  : "${D:=$(date -u +%Y-%m-%d)}"; \
  C="$(printf '%s' "$C" | cut -c1-7)"; \
  echo "beacon build version=v${V} commit=${C} date=${D}"; \
  CGO_ENABLED=0 go build \
    -ldflags "-s -w -X github.com/magiconair/beacon/internal/version.Version=${V} -X github.com/magiconair/beacon/internal/version.Commit=${C} -X github.com/magiconair/beacon/internal/version.Date=${D}" \
    -o /out/beacon ./cmd/beacon

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/beacon /app/beacon
EXPOSE 8080
USER nobody
ENTRYPOINT ["/app/beacon"]
CMD ["server"]
