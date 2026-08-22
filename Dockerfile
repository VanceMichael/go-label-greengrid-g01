FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/greengrid ./cmd/server

FROM debian:bookworm-slim
RUN useradd --system --create-home greengrid
WORKDIR /app
COPY --from=build /out/greengrid /app/greengrid
RUN mkdir -p /var/lib/greengrid && chown -R greengrid:greengrid /var/lib/greengrid
USER greengrid
ENV GREENGRID_ADDR=:8080 GREENGRID_DB=/var/lib/greengrid/greengrid.db
EXPOSE 8080
ENTRYPOINT ["/app/greengrid"]
