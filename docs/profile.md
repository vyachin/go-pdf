# CSS-профиль

Профиль — закрытый белый список. Всё, что в него не входит, отклоняется валидацией с координатой в исходнике. Актуальный список всегда доступен командой:

```bash
pdfgen validate -print-profile
```

## Элементы

```
html head body div p h1 h2 h3 h4 h5 h6
ul ol li table thead tbody tfoot tr td th
img span br hr strong em a title meta style
```

Неизвестный элемент — ошибка `html/unsupported-element`. Поддерживаются атрибуты `id`, `class`, `style`, `src`, `colspan`, `rowspan`, `start` (для `ol`), `name`/`content` (для `meta`).

## Свойства

| Свойство | Наследуется | Значения |
|---|---|---|
| `display` | нет | `block`, `inline`, `inline-block`, `list-item`, `table`, `table-row`, `table-cell`, `none` |
| `width`, `height` | нет | длина или `auto` |
| `margin`, `padding` | нет | 1–4 длины |
| `border` | нет | `[width] [solid\|none] [color]` |
| `border-collapse` | нет | `collapse`, `separate` |
| `background-color` | нет | цвет |
| `color` | да | цвет |
| `font-family` | да | список семейств |
| `font-size` | да | длина, `em`, `%` |
| `font-weight` | да | `100`–`900`, `normal`, `bold` |
| `font-style` | да | `normal`, `italic` |
| `line-height` | да | число, длина, `normal` |
| `text-align` | да | `left`, `right`, `center`, `justify` |
| `text-indent` | да | длина |
| `white-space` | да | `normal`, `pre` |
| `vertical-align` | нет | `baseline`, `top`, `middle`, `bottom` |
| `break-before`, `break-after` | нет | `auto`, `page`, `avoid` |
| `break-inside` | нет | `auto`, `page`, `avoid` |
| `page-break-before/after/inside` | нет | устаревшие синонимы `break-*` |
| `orphans`, `widows` | да | целое ≥ 1 |
| `list-style-type` | да | `disc`, `decimal` |

## Единицы

`px`, `pt`, `mm`, `cm`, `in`, `%`, `em`. Всё остальное (`rem`, `vw`, `vh`, `ch`) — ошибка.

## Селекторы

Имя элемента, класс, идентификатор, комбинации, потомок (пробел), прямой ребёнок (`>`), перечисление через запятую.

Псевдоклассы и атрибутные селекторы отклоняются. Единственное исключение — `a::after` для сгенерированного содержимого:

```css
a.toc::after { content: " →" target-counter(attr(href), page) }
```

Внутри `a::after` разрешено только свойство `content` со строками, `counter(page)`, `counter(pages)` и `target-counter(attr(href), page)`. Вне `a::after` свойства `content` нет.

## At-правила

```css
@page {
  size: A4;          /* A4, A5, Letter или два размера */
  margin: 20mm 15mm; /* 1–4 длины */
}

@page {
  @top-center    { content: "Счёт № 42" }
  @bottom-center { content: "Стр. " counter(page) " из " counter(pages) }
}

@font-face {
  font-family: "PT Serif";
  src: url(fonts/pt-serif.ttf);  /* только локальный TrueType */
}

@media print { /* или @media all */ }
```

Margin boxes: `top-left`, `top-center`, `top-right`, `bottom-left`, `bottom-center`, `bottom-right`.
Значения `content`: строки в кавычках, `counter(page)`, `counter(pages)`, `target-counter(attr(href), page)`.

## Чего нет и почему

| Не поддерживается | Причина |
|---|---|
| `flex`, `grid` | другой движок раскладки; для документов хватает блоков и таблиц |
| `float`, `position: absolute/fixed` | ломает пагинацию и требует другого алгоритма |
| градиенты, тени, скругления, прозрачность | не нужны в печатных документах, усложняют вывод |
| `hyphens: auto` | требует словарей переносов; в планах на v2 |
| JavaScript | движок статический, документ не исполняется |
| CFF/OTF и веб-шрифты по URL | см. `docs/fonts.md` |
| `table-layout: auto` | алгоритм авто-раскладки таблиц несоразмерен пользе |
| прочие псевдоэлементы (`::before`, `::first-line`) | сгенерированное содержимое нужно только для оглавления |

## Диагностика

Каждая ошибка содержит файл, строку, столбец и путь элемента:

```
invoice.html:12:5: error[css/unsupported-value]: display: unsupported display value "flex" (at body > div.invoice > p)
```

Коды: `css/unsupported-property`, `css/unsupported-value`, `css/unsupported-selector`, `css/unsupported-at-rule`, `css/unsupported-media`, `css/unsupported-margin-box`, `css/incomplete-font-face`, `html/unsupported-element`, `text/glyph`, `layout/overflow`.
