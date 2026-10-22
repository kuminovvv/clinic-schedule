# Clinic Schedule

Проект состоит из двух приложений:

- frontend/ — интерфейс Next.js.
- backend/ — Go API, миграции PostgreSQL и локальные скрипты запуска.

Общий docker-compose.yml расположен в корне проекта.

## Production и автоматический деплой

Production-версия запускается отдельным `docker-compose.prod.yml` с
PostgreSQL, Go API, Next.js и Caddy (HTTPS + авторизация). Сборка образов
происходит в GitHub Actions, а новый тег `vX.Y.Z` запускает SSH-деплой на VPS.
Пошаговая первоначальная настройка сервера, секреты GitHub и резервное
копирование описаны в [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

Не запускайте production через `pnpm dev` и не публикуйте наружу порты БД/API.

## Запуск через Docker

Из корня clinic-schedule/:

    docker compose up --build -d
    cd frontend
    pnpm install
    pnpm dev

По умолчанию Go API доступен на http://127.0.0.1:8080.
Адрес для Next.js задаётся в frontend/.env.local (BACKEND_INTERNAL_URL).

## Запуск без Docker (Windows)

Из корня clinic-schedule/:

    .\backend\setup-local.ps1   # только при первой настройке базы
    .\backend\run-local.ps1     # Go API, отдельное окно PowerShell
    cd frontend
    pnpm dev                     # Next.js, отдельное окно PowerShell

Go API читает настройки и пароль БД из backend/.env.local, фронтенд — из frontend/.env.local.
Действующие локальные файлы конфигурации не перезаписываются при переносе.

## Проверка

    cd backend
    go test ./...
    cd ../frontend
    pnpm exec tsc --noEmit
    pnpm run build

Подробности интерфейса: [frontend/README.md](frontend/README.md).
