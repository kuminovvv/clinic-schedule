# Правила Personal Site (Frontend)

`AGENTS.md` — точка входа, подробные правила находятся в документации:

1. **Перед задачей** прочитайте [карту документации](docs/README.md) и [локальные соглашения](docs/frontend-conventions.md).
2. **По задаче** откройте только подходящие разделы `docs/` (компоненты, FSD, Next.js, производительность, тестирование и т. д.).
3. **Перед завершением** проверьте затронутое поведение и выполните `pnpm exec tsc --noEmit`, `pnpm run lint`, `pnpm run build` (либо явно укажите, что не удалось проверить).

Главные инварианты: стрелочные функции; один React-компонент на `.tsx`; не более 250 строк в исходном файле; FSD, App Router в `src/app`; **Tailwind CSS 4 + shadcn/ui** для оформления компонентов (не создавать CSS Modules); переиспользование общих UI-компонентов без незапрошенного изменения дизайна. Подробности и исключения — в [frontend-conventions.md](docs/frontend-conventions.md).

**Приоритет:** требования текущей задачи → `AGENTS.md` и `docs/frontend-conventions.md` → тематические документы. Документация частично перенесена из Soobota: её специфичные инструкции по деплою, инфраструктуре и продуктовым сценариям **не применяются** к Personal Site без отдельного подтверждения.

<!-- BEGIN:nextjs-agent-rules -->

## This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` (resolved from this file's directory; in monorepos the `next` package may not be visible from the repo root) before writing any code. Heed deprecation notices.

This block is written and re-added by `next dev` — verify at `node_modules/next/dist/server/lib/generate-agent-files.js`. Removing it from a diff only re-creates the uncommitted change; committing it with your work keeps the tree clean.

<!-- END:nextjs-agent-rules -->
