# Документация Personal Site

`AGENTS.md` — краткий входной контракт. Этот каталог содержит подробные правила, которые нужно читать **по мере необходимости**, а не все сразу.

## Порядок работы

1. Прочитать [`AGENTS.md`](../AGENTS.md) и [соглашения проекта](frontend-conventions.md).
2. Найти подходящий раздел ниже и сверить его с фактическим устройством Personal Site.
3. Реализовать задачу без посторонних изменений; проверить поведение, TypeScript, ESLint и сборку.

При конфликте приоритет имеют текущая задача, `AGENTS.md` и `frontend-conventions.md`. Часть материалов импортирована из **Soobota Frontend** и пока носит справочный характер: упоминания её сервисов, окружения, `tasks/`, CI/CD, версионирования и деплоя на `soobota.com` **не описывают Personal Site**.

## Карта документов

| Документ | Когда читать |
| --- | --- |
| [frontend-conventions.md](frontend-conventions.md) | Всегда: локальные требования к функциям, файлам и структуре |
| [00-overview.md](00-overview.md) | Обзор подхода к документации (перенесён из Soobota) |
| [01-context-loading.md](01-context-loading.md) | Как собирать контекст задачи |
| [02-task-workflow.md](02-task-workflow.md) | Планирование и этапы работы; пути `tasks/` — только если созданы в этом проекте |
| [03-fsd.md](03-fsd.md) | Слои и импорты FSD; для маршрутов руководствоваться `src/app` |
| [04-components.md](04-components.md) | Компоненты, props, типы, декомпозиция |
| [05-typescript.md](05-typescript.md) | TypeScript и типизация |
| [06-nextjs.md](06-nextjs.md) | Next.js, RSC, Server/Client Components |
| [07-performance.md](07-performance.md) | Производительность, рендеры, бандл |
| [08-error-handling.md](08-error-handling.md) | Обработка ошибок и обратная связь |
| [error-notifications.md](error-notifications.md) | Единые уведомления Toast, API-перехватчик и сообщения для сотрудников |
| [09-testing-and-verification.md](09-testing-and-verification.md) | Проверка изменений; использовать реальные скрипты `pnpm` проекта |
| [page-driven-scroll.md](page-driven-scroll.md) | Переиспользуемая прокрутка страницы для закреплённых таблиц и календарей |
| [11-ui-context-risks.md](11-ui-context-risks.md) | UI-состояния, overlays, hover, риски регрессий |

## Быстрый выбор по задаче

- **UI, Tailwind CSS 4, shadcn/ui и Figma:** `frontend-conventions.md`, `04-components.md`, `11-ui-context-risks.md`.
- **Архитектура:** `03-fsd.md`, `06-nextjs.md`.
- **Хуки и типы:** `05-typescript.md`, `06-nextjs.md`.
- **Оптимизация:** `07-performance.md`.
- **Проверка и сдача:** `09-testing-and-verification.md`; основные команды — `pnpm exec tsc --noEmit`, `pnpm run lint`, `pnpm run build`.
