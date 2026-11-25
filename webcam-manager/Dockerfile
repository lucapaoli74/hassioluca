# Facoltativo, per NAS o server Linux. Su Windows usa il Setup.exe.
# La ricerca delle telecamere funziona solo con la rete dell'host:
#   docker run -d --name webcam-manager --network host --restart unless-stopped \
#     -v /srv/webcam-manager:/data webcam-manager
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /webcam-manager .

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates tzdata
ENV TZ=Europe/Rome
COPY --from=build /webcam-manager /usr/local/bin/webcam-manager
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["webcam-manager", "-data", "/data"]
