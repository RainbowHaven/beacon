# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/beacon ./cmd/beacon

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/beacon /app/beacon
EXPOSE 8080
USER nobody
ENTRYPOINT ["/app/beacon"]
