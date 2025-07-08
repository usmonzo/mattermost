FROM mattermost/mattermost-team-edition:latest

# Очистим, чтобы не было конфликта
RUN rm -rf /mattermost

# Копируем весь твой проект внутрь
COPY . /mattermost

# Устанавливаем зависимости (для webapp и server)
WORKDIR /mattermost

# Пересобрать фронтенд (если нужно)
RUN cd webapp && npm install && npm run build

# Собрать backend (если ты пересобираешь сервер)
# Если у тебя уже собранный бинари — можно пропустить
RUN make build-linux

# Стартовая команда по умолчанию
ENTRYPOINT ["/mattermost/bin/mattermost"]
