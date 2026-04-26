FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata curl
WORKDIR /app

COPY luner /luner

RUN echo "# Default config: mount your config.yaml at runtime" > /app/README.txt && \
    echo "# Example: docker run -v ./config.yaml:/app/config.yaml:ro ..." >> /app/README.txt

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/luner"]

CMD ["-config", "config.yaml"]