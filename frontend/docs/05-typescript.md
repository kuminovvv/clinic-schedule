# 05. TypeScript

## 1. Общие принципы

Типы должны объяснять контракт, а не маскировать неясность. 

### Дисциплина типизации:
- **Никаких `any`**: Не прятать проблемы с типами через `any`. При крайней необходимости использовать `unknown` с последующим сужением типа.
- **Запрет Type Assertion (`as`)**: Не использовать приведение типов через `as` без крайней необходимости. Вместо `const value = data as Type` используйте явную проверку типов или guard-функции.
- **Проверка типов**: Всегда проверяйте изменения через локальный компилятор: `npx tsc --noEmit`.

---

## 2. Булевые пропсы и переменные

Всегда используйте префиксы, явно отражающие семантику:

| Префикс | Семантика | Пример |
|---------|-----------|--------|
| `is` | Текущее состояние / статус | `isLoading`, `isVisible`, `isOwnPost` |
| `has` | Наличие чего-либо | `hasError`, `hasData`, `hasMedicalDisclaimer` |
| `should` | Ожидаемое поведение / триггер действия | `shouldAnimate`, `shouldSuppressClick` |
| `can` | Разрешение или возможность | `canEdit`, `canDelete`, `canOpenPost` |

```tsx
// ✅ Правильно
export type PostProps = {
  isBookmarked?: boolean;
  hasVideo?: boolean;
  shouldShowModeration?: boolean;
  canEdit?: boolean;
};

// ❌ Запрещено
export type PostProps = {
  bookmarked?: boolean; // → isBookmarked
  video?: boolean;      // → hasVideo
  showModeration?: boolean; // → shouldShowModeration
};
```

---

## 3. Type vs Interface

Используйте `type` по умолчанию для всех определений в проекте:

```tsx
// ✅ Правильно: type
export type UserProps = {
  id: string;
  name: string;
};

// ❌ Нежелательно: interface
export interface UserProps {
  id: string;
  name: string;
}
```
*Исключение:* Использование `interface` допускается только при необходимости расширения (`extends`) сторонних типов из внешних библиотек.

---

## 4. Канонический паттерн типизации хуков

```ts
export type UseSomethingProps = {
  postId?: string;
  initialViewsCount?: number;
}

export type UseSomethingResult = {
  viewsCount: number;
  handleIncrementAction: () => void;
}

export type UseSomethingHook = (
  props: UseSomethingProps
) => UseSomethingResult
```

---

## 5. Использование служебных типов TypeScript

Для переиспользования и модификации существующих типов используйте встроенные Utility Types вместо создания дублирующих типов вручную:

* `Pick<Type, Keys>` — выбрать определенный набор полей.
* `Omit<Type, Keys>` — исключить определенный набор полей.
* `Partial<Type>` — сделать все свойства опциональными.
* `Required<Type>` — сделать все свойства обязательными.
* `Record<Keys, Type>` — описать словарь (map).
* `ReturnType<Type>` — получить тип возвращаемого значения функции.

---

## 6. Generic-типы

Используйте понятные описательные имена для generic-параметров вместо одиночных букв вроде `T` (если контекст не слишком простой):

```ts
// ✅ Правильно
export type ApiResponse<TData> = {
  data: TData;
  error?: string;
}

// ❌ Нежелательно
export type ApiResponse<T> = {
  data: T;
}
```
