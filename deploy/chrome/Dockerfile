FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium \
    socat \
    fonts-freefont-ttf \
    fonts-dejavu-core \
    libnss3 libgbm1 libasound2 libdrm2 libxkbcommon0 \
    libx11-6 libxss1 libxrandr2 libxdamage1 libxfixes3 libxcomposite1 \
    x11-utils \
    dbus \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd -r chrome \
    && useradd -r -g chrome -G audio,video chrome \
    && mkdir -p /home/chrome \
    && chown chrome:chrome /home/chrome

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

USER chrome
WORKDIR /home/chrome

ENTRYPOINT ["/entrypoint.sh"]
