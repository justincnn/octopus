FROM alpine:3.20
WORKDIR /app
COPY octopus-bin /app/octopus
RUN mkdir -p /app/data
EXPOSE 8080
VOLUME ["/app/data"]
CMD ["/app/octopus", "start"]
