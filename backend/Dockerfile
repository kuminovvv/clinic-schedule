FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/clinic-api ./cmd/api

FROM alpine:3.22
RUN apk add --no-cache tzdata && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/clinic-api ./clinic-api
COPY migrations ./migrations
USER app
EXPOSE 8080
ENTRYPOINT ["./clinic-api"]
