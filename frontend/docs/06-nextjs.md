# 06. Next.js & React Server Components (RSC)

Данный проект построен на **Next.js (App Router)** с разделением на Server Components и Client Components.

## 1. Архитектура Server & Client Components

По умолчанию все компоненты в `app/` и `src/` являются **Server Components (RSC)**.

### Когда использовать Client Components (`'use client'`):
* Использование хуков состояния или эффектов (`useState`, `useEffect`, `useReducer`, `useContext`).
* Использование Browser API (например, `window`, `document`, `localStorage`).
* Использование обработчиков событий (например, `onClick`, `onChange` в DOM-элементах).
* Интеграция библиотек, работающих только на клиенте.

### Правило локализации `'use client'`:
* Опускайте `'use client'` как можно ниже по дереву компонентов.
* **Критически важно:** Не ставьте `'use client'` на файлы хуков, типов, хелперов или промежуточных подкомпонентов, если они импортируются исключительно другими клиентскими компонентами. Это предотвратит ложные срабатывания компилятора Next.js о сериализуемости пропсов (**TS71007**).
* Директива `'use client'` должна находиться только на уровне «входных файлов» (entry points) клиентских виджетов/фич (например, `post-card.tsx`).

---

## 2. Передача пропсов и сериализуемость (TS71007)

При передаче данных через границу «Server → Client» все пропсы должны быть сериализуемыми. 
Функции не могут передаваться напрямую из серверных компонентов в клиентские, за исключением Server Actions.

### Правило именования коллбэков (`*Action`):
Если компонент экспортируется из файла с `'use client'`, любые функции в его пропсах (коллбэки, обработчики) **обязаны иметь суффикс `Action`** (например, `onLikeAction`, `setIsRepostDialogOpenAction`). В противном случае Next.js компилятор выдаст ошибку **TS71007**:
> *Props must be serializable for components in the "use client" entry file. "setSomething" is a function that's not a Server Action.*

```tsx
// ✅ Правильно: суффикс Action
type PostCardProps = {
  postId: string;
  onBookmarkAction: () => void;
  setIsDialogOpenAction: (open: boolean) => void;
};
```

---

## 3. Data Fetching & Производительность на сервере

### Параллельная загрузка данных (Promise.all)
Для избежания водопадов (Waterfalls) независимые запросы данных на сервере должны выполняться параллельно через `Promise.all`:

```tsx
// ✅ Правильно: запросы запускаются параллельно
export default async function ChannelPage({ params }: { params: { slug: string } }) {
  const [channel, posts] = await Promise.all([
    fetchChannel(params.slug),
    fetchChannelPosts(params.slug),
  ]);
  
  return (
    <ChannelLayout channel={channel}>
      <PostList posts={posts} />
    </ChannelLayout>
  );
}
```

### Дедупликация запросов через `React.cache()`
Если одни и те же данные требуются в разных компонентах в рамках одного серверного рендеринга (одного HTTP-запроса), используйте `React.cache()` для мемоизации функций выборки:

```ts
import { cache } from 'react'

// ✅ Запрос выполнится только один раз за рендер страницы
export const getViewer = cache(async () => {
  return await UsersService.getMe();
});
```

### Разделение Suspense-границ (Streaming)
Для тяжелых или медленных запросов оборачивайте подкомпоненты в `<Suspense />`, чтобы страница рендерилась инкрементально:

```tsx
export default async function Page() {
  return (
    <div>
      <Header />
      <Suspense fallback={<SidebarSkeleton />}>
        <AsyncSidebar />
      </Suspense>
      <Suspense fallback={<FeedSkeleton />}>
        <AsyncFeed />
      </Suspense>
    </div>
  );
}
```

---

## 4. Server Actions (Серверные Действия)

Server Actions используются для мутаций данных и вызываются с клиента, но исполняются на сервере.

### Правила безопасности:
* **Авторизация**: Каждый Server Action должен проверять сессию/токен пользователя перед выполнением логики.
* **Валидация**: Входные параметры действия должны строго валидироваться (например, с использованием `zod`).

```ts
'use server'

import { z } from 'zod'

const schema = z.object({
  postId: z.string().uuid(),
})

export async function deletePostAction(formData: FormData) {
  const session = await getSession();
  if (!session) throw new Error("Unauthorized");

  const data = schema.parse({
    postId: formData.get('postId'),
  });

  await db.post.delete({ where: { id: data.postId } });
}
```

---

## 5. Оптимизация бандла

* **Импорт тяжелых библиотек**: Используйте `next/dynamic` для ленивой загрузки тяжелых клиентских компонентов (редакторы, карты, графики), чтобы они не раздували основной бандл:
  ```tsx
  import dynamic from 'next/dynamic'

  const RichTextEditor = dynamic(() => import('@/shared/ui/RichTextEditor'), {
    ssr: false,
    loading: () => <p>Загрузка редактора...</p>
  });
  ```
* **Использование `experimental.optimizePackageImports`**: Конфигурируйте автоматический тришейкинг тяжелых библиотек (например, иконок) в `next.config.js` для предотвращения попадания лишнего кода в бандл.
