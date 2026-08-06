FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /out/cluster-monitor ./...

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/cluster-monitor /cluster-monitor
EXPOSE 8080
ENTRYPOINT ["/cluster-monitor"]
