# 03. Feature-Sliced Design

## Структура слоев

Снизу вверх:

1. `shared`
2. `entities`
3. `features`
4. `widgets`
5. `pages`
6. `app`

## Правило импортов

Слой может импортировать только слои ниже себя.

Правильно:

```ts
import { Button } from "shared/ui";
import { useUser } from "entities/user";
import { LoginForm } from "features/auth";
```

Неправильно:

```ts
import { Header } from "widgets/header";
import { ProductCard } from "widgets/product";
```

## Где живет что

- `app/` — routing Next.js и app-level composition
- `src/app/` — shell, providers, app logic
- `src/pages/` — page composition
- `src/widgets/` — крупные блоки
- `src/features/` — сценарии пользователя
- `src/entities/` — бизнес-сущности
- `src/shared/` — generic code

## Публичный API

Каждый slice должен иметь `index.ts`.

Агент должен:

- экспортировать публичный API из `index.ts`;
- не плодить deep-imports без необходимости;
- при создании нового slice сразу создавать публичную точку входа.
