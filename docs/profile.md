# CSS-профиль

Профиль — закрытый белый список. Всё, что в него не входит, отклоняется валидацией с координатой в исходнике. Актуальный список всегда доступен командой:

```bash
pdfgen validate -print-profile
```

## Элементы

```
html head body div p h1 h2 h3 h4 h5 h6
ul ol li table thead tbody tfoot tr td th
img span br hr strong b em i a title meta style link
```

`<script>` допускается, но игнорируется с предупреждением `html/ignored-script`: код не исполняется и не рендерится. В строгом режиме (`-strict`) это ошибка.

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

При `border-collapse: collapse` общая линия сетки рисуется один раз: внешнюю рамку даёт `border` самой таблицы (если он задан), внутренние границы — левая и верхняя границы ячейки справа или снизу. Конфликт разных толщин и цветов решается в пользу этой одной границы; приоритеты CSS (`cell > row > table`) не моделируются.

`auto` допустим только у `width` и `height`. У `margin`, `padding`, `text-indent` и `line-height` это ошибка профиля: `margin: 0 auto` не центрирует блок, а раньше молча выравнивал его влево.

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
  size: A4;              /* A4, A5, Letter, два размера или landscape/portrait */
  margin: 20mm 15mm;     /* 1–4 длины */
}

@page { size: A4 landscape }  /* 297 x 210 мм */
@page { size: landscape }     /* ориентация на размер из -page (по умолчанию A4) */
@page { size: portrait }

@page {
  @top-center    { content: "Счёт № 42" }
  @bottom-center { content: "Стр. " counter(page) " из " counter(pages) }
}

@font-face {
  font-family: "PT Serif";
  src: url(fonts/pt-serif.ttf);  /* только локальный TrueType */
}

/* Внешняя таблица стилей: только локальный файл */
<link rel="stylesheet" href="styles/print.css">
<link rel="stylesheet" href="styles/screen.css" media="print">

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
| внешний CSS по URL | движок не ходит в сеть; `<link>` принимает только локальный файл |
| `table-layout: auto` | алгоритм авто-раскладки таблиц несоразмерен пользе |
| `size: 210mm 297mm landscape` | грамматика CSS не разрешает ориентацию вместе с длинами |
| прочие псевдоэлементы (`::before`, `::first-line`) | сгенерированное содержимое нужно только для оглавления |

## Внешние таблицы стилей

`<link rel="stylesheet">` подключает локальный файл. Правила:

- путь разрешается относительно `BaseDir` запуска; при заданном `BaseDir` выход за его пределы запрещён;
- `media` у `<link>` и `<style>` со значением `print`, `all` или пустым применяет лист, иное значение (в том числе список без `print`) пропускает его без ошибки;
- `rel`, отличный от `stylesheet` (`icon`, `canonical`), игнорируется молча;
- `http(s)://` и `data:` отклоняются: удалённые стили не загружаются;
- порядок каскада — порядок документа, поэтому `<style>`, `<link>`, `<style>` применяются именно в этой последовательности;
- ошибка внутри подключённого файла указывает файл, строку и столбец в нём.

## Диагностика

Каждая ошибка содержит файл, строку, столбец и путь элемента:

```
invoice.html:12:5: error[css/unsupported-value]: display: unsupported display value "flex" (at body > div.invoice > p)
```

Коды: `css/unsupported-property`, `css/unsupported-value`, `css/unsupported-selector`, `css/unsupported-at-rule`, `css/unsupported-media`, `css/unsupported-margin-box`, `css/incomplete-font-face`, `html/unsupported-element`, `text/glyph`, `layout/overflow`.
