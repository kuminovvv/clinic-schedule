# Прокрутка содержимого через страницу

`usePageDrivenScroll` — переиспользуемый механизм для экранов, где блок остаётся закреплённым, а его содержимое прокручивается **полосой прокрутки документа**. Например: недельный календарь и каталог процедур.

Расположение: `src/shared/lib/hooks/use-page-driven-scroll.ts`. Хук не зависит от доменной модели, не создаёт DOM и не управляет состоянием записей.

## Контракт

```tsx
const {trackRef, panelRef, viewportRef, contentRef, viewportHeight, panelStyle} =
  usePageDrivenScroll({
    topOffset: 64,             // расстояние от верхней границы окна до sticky-панели, px
    bottomGap: 16,             // свободное пространство снизу, px; по умолчанию 0
    heightMode: "fit-viewport", // "fixed" по умолчанию
    trailingScrollSpace: 0,    // дополнительный путь прокрутки, px
    enabled: true,             // по умолчанию true
  })
```

Нужная структура DOM:

```tsx
<div ref={trackRef} className="relative shrink-0">
  <section ref={panelRef} style={panelStyle} className="sticky flex min-h-0 flex-col overflow-hidden">
    <div className="shrink-0">Непрокручиваемая шапка</div>
    <div ref={viewportRef} className="min-h-0 flex-1 overflow-hidden">
      <div ref={contentRef}>Длинный список или сетка</div>
    </div>
  </section>
</div>
```

`trackRef` занимает в документе высоту панели плюс скрытую часть содержимого; `panelRef` фиксируется через `position: sticky`; `viewportRef` обрезает содержимое; `contentRef` смещается с помощью `translateY`. Хук наблюдает за изменениями размеров через `ResizeObserver`, а при размонтировании освобождает подписки. `viewportHeight` нужен только потребителям, которые вычисляют собственную геометрию, например календарной сетке.

Режимы высоты:

- **`fixed`** — высота `100svh - topOffset - bottomGap`, независимо от первоначальной позиции блока. Используется в календаре.
- **`fit-viewport`** — высота вычисляется по фактическому положению блока: до закрепления он заканчивается выше нижней границы экрана, после закрепления расширяется. Используется в каталоге.

Важные инварианты:

- `topOffset` должен соответствовать высоте фиксированной шапки; применяется автоматически через `panelStyle`.
- Не устанавливать `overflow-y-auto` или `overflow-y-scroll` на `viewportRef`: прокруткой управляет документ.
- Над `panelRef` не должно быть предка с `overflow: hidden/auto`, меняющего sticky-контейнер. `overflow-hidden` допустим на самой панели.
- `contentRef` должен охватывать всю движущуюся область; если интерфейс сам позиционирует элементы, как календарь, ссылку можно передать его сетке.
- Для нового экрана меняют только параметры и композицию элементов; обработчики `window.scroll` и расчёт высот не копируют.

Регрессия: `python scripts/check-page-driven-scroll.py` при запущенном фронтенде на `http://localhost:3000`. Требуются Python Playwright и Chrome.
