FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
COPY web/ web/
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server && \
    CGO_ENABLED=0 go build -trimpath -o /out/db ./cmd/db

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/ /app/
USER app
EXPOSE 8080
CMD ["/app/server"]
