FROM golang:1.23-alpine AS build

WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/sqltojson .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /out/sqltojson /usr/local/bin/sqltojson

EXPOSE 8080

ENTRYPOINT ["sqltojson"]
