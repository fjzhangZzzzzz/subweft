# syntax=docker/dockerfile:1
# Application-only image: supply the independently verified Mihomo binary as read-only mount.
FROM debian:bookworm-slim@sha256:99c0b25c46d0214d5a2091092e7908907dcaecd6a962cde0ac700acee222b6f3 AS runtime-base
COPY output/container/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN printf 'deb [check-valid-until=no] https://snapshot.debian.org/archive/debian/20261027T000000Z bookworm main\ndeb [check-valid-until=no] https://snapshot.debian.org/archive/debian-security/20261027T000000Z bookworm-security main\n' > /etc/apt/sources.list \
 && rm -f /etc/apt/sources.list.d/debian.sources \
 && apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates bubblewrap util-linux \
 && chmod u-s /usr/bin/bwrap \
 && dpkg-query -W > /usr/share/subweft-runtime-packages.txt \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir -p /var/lib/subweft /run/subweft /opt/subweft \
 && chown 10001:10001 /var/lib/subweft /run/subweft
COPY --chmod=0555 output/container/subweft /usr/local/bin/subweft
USER 10001:10001
ENV SUBWEFT_LISTEN=0.0.0.0:8080 SUBWEFT_VALIDATION_DIR=/run/subweft/validation SUBWEFT_MIHOMO_PATH=/opt/subweft/mihomo
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/subweft"]
CMD ["serve","--data-dir","/var/lib/subweft","--key-file","/run/secrets/master.key"]

# Disposable CI target only; never used for the product image.
FROM runtime-base AS acceptance
USER root
RUN apt-get update && apt-get install -y --no-install-recommends python3 && rm -rf /var/lib/apt/lists/*
USER 10001:10001

FROM runtime-base AS runtime
