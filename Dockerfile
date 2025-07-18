# ============================================================
# 🔨 Build stage: сборка backend + frontend
# ============================================================
FROM golang:1.22.4-bullseye AS build

# Установим Node.js
RUN curl -fsSL https://deb.nodesource.com/setup_18.x | bash - && \
    apt-get install -y nodejs && npm install -g npm

WORKDIR /app
COPY . .

# Установим зависимости и соберем frontend
WORKDIR /app/webapp
RUN npm install && npm run build

# Собираем backend
WORKDIR /app/server
RUN go build -o ../bin/mattermost ./cmd/mattermost

# ============================================================
# 📦 Final stage: минимальный образ
# ============================================================
FROM mattermost/mattermost-team-edition:10.11.0 AS final

COPY --from=build /app/bin/mattermost /mattermost/bin/mattermost
COPY --from=build /app/webapp/dist /mattermost/client
COPY config /mattermost/config

ENTRYPOINT ["/mattermost/bin/mattermost"]
