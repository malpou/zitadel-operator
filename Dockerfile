FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /zitadel-operator ./cmd/zitadel-operator

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /zitadel-operator /zitadel-operator
ENTRYPOINT ["/zitadel-operator"]
