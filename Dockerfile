FROM golang:1.20 AS build-stage

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /server

# Deploy the application binary into a lean image
FROM gcr.io/distroless/base-debian11 AS build-release-stage

WORKDIR /

COPY --from=build-stage /server /server

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/server"]