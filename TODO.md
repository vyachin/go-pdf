# TODO по результатам code review

## Актуальное ревью — 24 ноября 2026 года

Проверен весь проект: CLI, публичный пакет `pdfgen`, HTML/CSS, профиль, чтение ресурсов, шрифты, текст, layout и пагинация, PDF, debug raster, тесты, документация, OpenSpec и CI. Начальный снимок [60acb04](https://github.com/vyachin/go-pdf/commit/60acb0471f2a8725c006a03467ee25af92c52f73) совпадал с GitHub `main`. Во время работы появились коммиты с очисткой прежнего TODO и публичным API; окончательные проверки выполнены на отдельном снимке [ef0e0d4](https://github.com/vyachin/go-pdf/commit/ef0e0d4c77296d401453a0333a3ef4aceaec1271). Изменения других работ сохранены. В рамках этого ревью изменён только `TODO.md`.

Ниже **31 подтверждённая проблема: две P1 и 29 P2**, а также **три новых предложения P3**. Некоторые пункты детализируют ранее открытый ENG-02, а не представляют независимые новые классы ошибок. Прежние ARCH-01 и ENG-09 также остаются актуальными. Сначала исправлять REV-01/02, затем ограничения ресурсов, ложный успех валидации и потерю содержимого. P1 означает риск доступности процесса; P2 — неверное поведение поддерживаемой функции; P3 — улучшение сопровождения или полноты проверок.

Все перечисленные воспроизведения проверены на финальном снимке. Для REV-02 проверены принятие опасной геометрии и путь до неограниченной аллокации; саму аллокацию на десятки гигабайт намеренно не запускали. Геометрические значения ниже получены с комплектным `testdata/fonts/DejaVuSans.ttf`; другие шрифты могут менять числа, сохраняя дефект. Ссылки на строки относятся к проверенному снимку; при изменениях ориентироваться также на функцию.

### Доступность, ограничения и отмена

- [x] **REV-01 · P1: ограничить расширение GPOS Coverage до выделения памяти.**

  **Код:** [internal/font/gpos.go:195](/Users/vyachin/Projects/go-pdf/internal/font/gpos.go:195), `parseCoverage` и вызывающий `parsePairPos`.
  **Воспроизведение:** Coverage format 2 с 16 повторёнными диапазонами `0..65535`: всего **100 байт** превращаются в **1 048 576** элементов `uint16`. Полный путь `Font.Parse` также принимает шрифт с такой **132-байтной GPOS**. Проверка `maxKerningGlyphs` выполняется позже и относится к ClassDef, поэтому не защищает Coverage. При 65535 диапазонах объём превышает четыре миллиарда элементов ещё до учёта перевыделений среза.
  **Исправление / приёмка:** проверять суммарное число glyph ID, порядок и пересечения диапазонов до `append`; ограничивать общий бюджет GPOS, включая повторные lookup/subtable. Повреждённую необязательную таблицу отклонять или игнорировать без большой аллокации. Проверять именно полный путь разбора шрифта.

- [x] **REV-02 · P1: ограничить площадь выходного растра и промежуточных масштабированных изображений.**

  **Код:** [internal/backend/raster/raster.go:64](/Users/vyachin/Projects/go-pdf/internal/backend/raster/raster.go:64), `Render`, `drawImage`; [internal/engine/convert.go:173](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go:173).
  **Воспроизведение:** `<style>@page{size:100000pt 100000pt;margin:0}</style><p>A</p>` проходит `Validate`, в том числе с `RenderPNG:true`. При 96 DPI `Render` попытается создать RGBA примерно **133333 × 133333**, то есть **71,1 GB**. `MaxImagePixels` ограничивает исходное изображение, но не страницу и не буфер `scaled` в `drawImage`: маленький PNG с огромным CSS-размером создаёт аналогичную проблему.
  **Исправление / приёмка:** проверять размеры и безопасное произведение до `image.NewRGBA`, возвращать `resource/limit`; масштабировать только видимую область. Опасный сценарий должен завершаться диагностикой, не panic/OOM. Сам OOM при ревью не запускался.

- [x] **REV-03 · P2: применять `MaxPages` при создании страницы, а не после всей раскладки.**

  **Код:** [internal/engine/convert.go:125](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go:125), [internal/layout/paginate.go:22](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:22), `flow.newPage`.
  **Воспроизведение:** 200 абзацев с `break-after:page`, затем ещё один абзац, при `Limits{MaxPages:1}` дают ошибку **«produced 201 pages, the limit is 1»**: все страницы уже созданы. Внутренний `maxIterations` не считает страницы, созданные вынужденными разрывами внутри одного `placeBlock`.
  **Исправление / приёмка:** передавать лимит в layout и проверять в единой точке создания страницы, включая forced breaks. Прерывать работу у границы лимита. Это оставшаяся часть ENG-02.

- [x] **REV-04 · P2: соблюдать `MaxFontBytes` для всех шрифтов и ограничивать чтение до аллокации.**

  **Код:** [internal/engine/fonts.go:85](/Users/vyachin/Projects/go-pdf/internal/engine/fonts.go:85), `newFontRegistry`, `loadSystemFallback`; [internal/fsx/fsx.go:62](/Users/vyachin/Projects/go-pdf/internal/fsx/fsx.go:62), `Root.Read`.
  **Воспроизведение:** `Convert` для `<p>A</p>` с комплектным `FallbackFont` и `Limits{MaxFontBytes:10}` успешно выдаёт PDF; то же подтверждено через публичный `pdfgen.Convert`. Для `@font-face` проверка есть, но только после полного `Read`/`io.ReadAll`. Системный fallback также не проверяется.
  **Исправление / приёмка:** единое ограниченное чтение всех шрифтов, проверка максимум `limit+1` байт до разбора. Аналогичные бюджеты нужны для внешних CSS и закодированных изображений; CLI читает HTML целиком до `MaxInputBytes`. Одного `Stat` недостаточно для ограничения растущего файла. Часть ENG-02.

- [x] **REV-05 · P2: завершить поддержку отмены в `Validate`, подготовке и записи PDF.**

  **Код:** [internal/engine/engine.go:94](/Users/vyachin/Projects/go-pdf/internal/engine/engine.go:94), `Validate`; [internal/engine/convert.go:35](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go:35), `prepare`, `Convert`; [pdfgen/pdfgen.go:248](/Users/vyachin/Projects/go-pdf/pdfgen/pdfgen.go:248).
  **Воспроизведение:** валидация 15000 `<div>A</div>` с отменой через 1 ms заканчивается при `ctx.Err()==context.Canceled`, но возвращает `err=nil` и пустую диагностику. Публичный `pdfgen.Validate` ведёт себя так же. У `Convert` уже есть проверки между стадиями и в части layout, однако парсеры, построение дерева, чтение ресурсов, `writer.Bytes` и растр не получают context; после записи нет финальной проверки.
  **Исправление / приёмка:** передавать отмену в дорогие циклы и ограниченное чтение; гарантировать возврат context error при отмене после старта. Проверять длительную подготовку, layout и сериализацию отдельно. Не считать уже отменённый context достаточным тестом. ENG-02 исправлен лишь частично.

- [x] **REV-06 · P2: закрыть оставшиеся обходы числовых границ CSS.**

  **Код:** [internal/css/token.go:249](/Users/vyachin/Projects/go-pdf/internal/css/token.go:249), `tokenizeNumber`; [internal/css/value.go:106](/Users/vyachin/Projects/go-pdf/internal/css/value.go:106), `ParseLengthToken`, `Length.Resolve`; [internal/css/style.go:110](/Users/vyachin/Projects/go-pdf/internal/css/style.go:110), `LineHeightMP`.
  **Воспроизведение:** `ParseLengthString("1e999pt")` возвращает **0 pt без ошибки**, потому что ошибка `ParseFloat` заменяется нулём. `1e100em` принимается с переполненным значением; `line-height:1e100` также принимается. Даже допустимый по текущей проверке `2000000%` при базе `200000` mp разрешается в **−294967296 mp** из-за сужения результата в `int32`.
  **Исправление / приёмка:** сохранять ошибки числового токена; проверять конечность и диапазон для em, безразмерных множителей, процентов и каждого вычисленного значения. Сложения геометрии и масштабирование тоже должны проверять переполнение. Уже исправленный абсолютный `1e100pt` не закрывает эти пути.

- [x] **REV-07 · P2: убрать квадратичный поиск конца HTML entity.**

  **Код:** [internal/html/token.go:302](/Users/vyachin/Projects/go-pdf/internal/html/token.go:302), `decodeEntities`.
  **Воспроизведение:** строка из `&` без `;`. Для каждого символа `strings.IndexByte(s[i:], ';')` просматривает весь оставшийся хвост, хотя затем разрешены только короткие entity. На финальном снимке 8192 / 32768 / 131072 байт заняли примерно **0,57 / 9,91 / 132,36 ms**. Это замеры конкретного запуска, а квадратичная сложность следует непосредственно из цикла. Лимит HTML 8 MiB и отсутствие отмены внутри парсера оставляют дорогой путь открытым.
  **Исправление / приёмка:** искать `;` только в разрешённом коротком окне либо разбирать entity однопроходно. Сохранить неизвестные entity и позиции; добавить benchmark для увеличивающихся входов и бюджет операций.

### Валидация и шрифты

- [x] **REV-08 · P2: не сообщать успешную полную валидацию для отсутствующих изображений и глифов.**

  **Код:** [internal/engine/engine.go:94](/Users/vyachin/Projects/go-pdf/internal/engine/engine.go:94), `Validate`; [internal/engine/convert.go:35](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go:35), `prepare`; [pdfgen/pdfgen.go:20](/Users/vyachin/Projects/go-pdf/pdfgen/pdfgen.go:20).
  **Воспроизведение:** `<p>A</p><img src="missing.png">` и `<p>&#x10ffff;</p>` с комплектным шрифтом дают `Validate: err=nil, diagnostics=[]`, затем `Convert` возвращает соответственно `img/load` и `text/glyph`. Проблема подтверждена и через публичный API. Это противоречит обещанию одинаковых ошибок Validate/Convert.
  **Исправление / приёмка:** вынести доступность изображений, форматы и проверку необходимых глифов в общую подготовку без пагинации. Либо явно разделить структурную проверку и полную проверку с разными обещаниями API/CLI. Добавить сравнение диагностики двух entry point для отсутствующего/повреждённого изображения, CMYK и отсутствующего символа.

- [x] **REV-09 · P2: отклонять TrueType без обязательной таблицы `glyf`.**

  **Код:** [internal/font/font.go:98](/Users/vyachin/Projects/go-pdf/internal/font/font.go:98), `Parse`; [internal/font/subset.go](/Users/vyachin/Projects/go-pdf/internal/font/subset.go).
  **Воспроизведение:** в копии комплектного TTF изменить только тег `glyf` в directory на `gone`. `font.Parse` возвращает успех, `GlyphIndex('A')` по-прежнему находится, но `GlyphPath` возвращает ноль контуров. Вместо ошибки получается шрифт с отсутствующими очертаниями. Основная спецификация `text-shaping` прямо требует ошибку при отсутствии `glyf`.
  **Исправление / приёмка:** проверять обязательные таблицы и согласованность `loca`/`glyf`, диапазоны и монотонность offsets до использования. Повреждённый обязательный шрифт должен отклоняться, а не превращаться в пустой subset.

- [x] **REV-10 · P2: проверять `cmap` glyph ID до приведения к `uint16` и не считать `.notdef` найденным символом.**

  **Код:** [internal/font/font.go:169](/Users/vyachin/Projects/go-pdf/internal/font/font.go:169), `GlyphIndex`; [internal/font/font.go:379](/Users/vyachin/Projects/go-pdf/internal/font/font.go:379), `cmapSubtable.lookup`; [internal/text/shape.go:41](/Users/vyachin/Projects/go-pdf/internal/text/shape.go:41).
  **Воспроизведение:** format 12 group для `A` со `startGID=0` возвращает `(0,true)`, а со `startGID=70000` — **`(4464,true)`**. Полученный ID не сверяется с `numGlyphs`; Shape доверяет `ok`. Это обходит гарантию ошибки отсутствующего глифа и может подменять символ другим очертанием. Ноль означает отсутствующий символ согласно [OpenType cmap](https://learn.microsoft.com/en-us/typography/opentype/spec/cmap).
  **Исправление / приёмка:** проверять результат в широком типе: `0 < gid < numGlyphs`; валидировать groups и исключить wraparound. Аналогично проверить ноль после `idDelta` в format 4. Ошибка должна называть исходный символ.

- [x] **REV-11 · P2: учитывать неявный класс 0 в GPOS PairPos.**

  **Код:** [internal/font/gpos.go:294](/Users/vyachin/Projects/go-pdf/internal/font/gpos.go:294), `Kerning`, `classOf`.
  **Воспроизведение:** корректный PairPos format 2: Coverage `[65]`, ClassDef1 `{65:1}`, ClassDef2 `{86:1}`, запись `(1,0)=-50`. Для пары `(65,87)` движок возвращает **0**, хотя второй глиф по умолчанию принадлежит классу 0. Проверено через разбор самой PairPos, а не только искусственно заполненные поля.
  **Исправление / приёмка:** отсутствие записи ClassDef означает класс 0, а не отсутствие кернинга; при этом первый глиф всё ещё обязан принадлежать Coverage. Правило определено в [OpenType Class Definition](https://learn.microsoft.com/en-us/typography/opentype/spec/chapter2#class-definition-table). Проверить неявный класс для обеих сторон и глиф вне Coverage.

- [x] **REV-12 · P2: исправить выбор ближайшего начертания по `font-weight`.**

  **Код:** [internal/engine/fonts.go:157](/Users/vyachin/Projects/go-pdf/internal/engine/fonts.go:157), `fontRegistry.pick`.
  **Воспроизведение:** зарегистрировать семейство с весами **100, затем 400** и запросить **500**. Выбирается 100: оба кандидата получают одинаковый score за принадлежность группе `<600`, после чего выигрывает порядок регистрации. Наличие более подходящего 400 не учитывается.
  **Исправление / приёмка:** отделить выбор наклона от алгоритма веса и реализовать определённый порядок fallback для весов, включая особенности диапазона 400–500. Порядок `@font-face` не должен заставлять выбирать 100 вместо доступного 400.

### Конечный PDF

- [x] **REV-13 · P2: сохранять PNG alpha на цветном фоне или диагностировать ограничение профиля.**

  **Код:** [internal/pdf/writer.go:522](/Users/vyachin/Projects/go-pdf/internal/pdf/writer.go:522), `imageObject`, ветка PNG.
  **Воспроизведение:** прозрачный 1×1 PNG в `<div style="background-color:red"><img src="transparent.png" style="width:20pt;height:20pt"></div>`. Debug raster показывает красный фон; независимый Poppler для конечного PDF даёт белый прямоугольник. Пиксель `(85,85)` при 96 DPI: debug **(65535,0,0)**, PDF **(65535,65535,65535)**. Диагностики нет: writer заранее смешивает PNG с белым.
  **Исправление / приёмка:** писать alpha как PDF soft mask либо отклонять alpha PNG с понятным ограничением профиля. Проверять итоговый PDF независимым renderer на цветном фоне, включая частичную прозрачность. Исправление «flattened once» для белого фона эту ошибку не закрывает.

- [x] **REV-14 · P2: вычислять `/Count` закладок по всем видимым потомкам.**

  **Код:** [internal/pdf/outline.go:54](/Users/vyachin/Projects/go-pdf/internal/pdf/outline.go:54), `writeOutline`, также root `/Count` в конце функции.
  **Воспроизведение:** цепочка `h1 → h2 → h3`: root получает `/Count 1`, h1 тоже `/Count 1`. При положительных Count все эти уровни открыты, поэтому должны быть **3** у root и **2** у h1. Сейчас считается только число прямых детей; структура может неверно отображаться у потребителя PDF. Семантика описана в [PDF 32000, таблицы 152–153](https://opensource.adobe.com/dc-acrobat-sdk-docs/standards/pdfstandards/pdf/PDF32000_2008.pdf).
  **Исправление / приёмка:** рекурсивно рассчитывать количество видимых потомков с учётом открытого/закрытого состояния. Проверять три уровня, несколько соседних веток и дерево с пропущенными уровнями заголовков.

- [x] **REV-15 · P2: переносить `<meta name="keywords">` в PDF.**

  **Код:** [internal/engine/convert.go:153](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go:153), создание `pdf.Options`; [internal/pdf/writer.go](/Users/vyachin/Projects/go-pdf/internal/pdf/writer.go), `Options` и Info dictionary.
  **Воспроизведение:** `<meta name="keywords" content="invoice,test"><p>A</p>` успешно конвертируется, но `/Keywords` в PDF отсутствует. В `pdf.Options` нет соответствующего поля, хотя основная спецификация `pdf-output` требует копировать author, description **и keywords**.
  **Исправление / приёмка:** провести keywords через подготовку и writer с корректной Unicode-строкой. Проверить наличие, точное значение, кириллицу и отсутствие meta.

### Каскад, скрытое содержимое и таблицы

- [x] **REV-16 · P2: различать происхождение UA и авторских CSS-правил в каскаде.**

  **Код:** [internal/css/cascade.go:27](/Users/vyachin/Projects/go-pdf/internal/css/cascade.go:27), `NewCascade`, `Compute`; [internal/engine/convert.go:71](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go:71).
  **Воспроизведение:** `<style>*{padding:0}</style><table><tr><td>A</td></tr></table>`. Авторский reset проигрывает более специфичному UA `td { padding:1mm }`: `A` начинается в **59528 mp** вместо **56693 mp**. Сначала должен учитываться origin, затем specificity внутри origin; одно лишь добавление UA sheet в начало списка этого не обеспечивает.
  **Исправление / приёмка:** представить origin явно либо применять UA как начальный слой перед авторским каскадом. Проверить `*` против UA для padding, margin и font-size, не ломая author `!important` и inline styles.

- [x] **REV-17 · P2: соблюдать `display:none` на `html` и `body`.**

  **Код:** [internal/layout/box.go:230](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:230), `Build`.
  **Воспроизведение:** `<body style="display:none">PRIVATE</body>` и `<style>html{display:none}</style><p>PRIVATE</p>` оба рисуют `PRIVATE`. Для обычных потомков проверка DisplayNone есть, для корневых элементов — нет: после Compute безусловно строится body.
  **Исправление / приёмка:** не строить/не рисовать дерево скрытого корня и его ресурсы; пустой результат обрабатывать согласованно. Проверить оба корня, в том числе скрытые изображения и заголовки, отсутствие текста и закладок.

- [x] **REV-18 · P2: не превращать игнорируемый `<script>` в печатный текст через CSS.**

  **Код:** [internal/layout/box.go:255](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:255), `buildBlocks`; [internal/css/ua.go:11](/Users/vyachin/Projects/go-pdf/internal/css/ua.go:11).
  **Воспроизведение:** `<body><style>script{display:block}</style><script>SECRET</script><p>VISIBLE</p></body>` рисует `SECRET`, хотя диагностика `html/ignored-script` и документация обещают игнорирование. Скрытие обеспечено только переопределяемым UA CSS. Исполнение JavaScript здесь не происходит; ошибка именно в выводе его исходного текста.
  **Исправление / приёмка:** исключать игнорируемые служебные узлы из layout независимо от CSS. Проверить script в body, принудительный display и отсутствие текста при обычном и strict запуске.

- [x] **REV-19 · P2: применять ширину, горизонтальные margins, фон и borders блока внутри ячейки.**

  **Код:** [internal/layout/table.go:338](/Users/vyachin/Projects/go-pdf/internal/layout/table.go:338), `paintStaticBlock`.
  **Воспроизведение:** `<table><tr><td><div style="width:30pt;margin:0 0 0 10pt;border:1pt solid red;background-color:blue">A</div></td></tr></table>`. В операциях нет ни красной рамки, ни синего фона. `b.width=width` игнорирует заданную ширину, горизонтальные margins не используются; ветка контейнера с anonymous block вообще не рисует декорации.
  **Исправление / приёмка:** общий расчёт used geometry и декораций для обычного и статического пути. Тот же div внутри/вне td должен сохранять content width 30 pt, margin 10 pt, фон и четыре стороны рамки. Это конкретное подтверждение ARCH-01.

- [x] **REV-20 · P2: сохранять маркеры списков внутри ячеек.**

  **Код:** [internal/layout/table.go:338](/Users/vyachin/Projects/go-pdf/internal/layout/table.go:338), `paintStaticBlock`; [internal/layout/paint.go:144](/Users/vyachin/Projects/go-pdf/internal/layout/paint.go:144), `paintListMarker`.
  **Воспроизведение:** `<table><tr><td><ol><li>FIRST</li><li>SECOND</li></ol></td></tr></table>` даёт текст **`FIRSTSECOND`** без `1.` и `2.`. Статический путь не вызывает отрисовку ListItem marker.
  **Исправление / приёмка:** использовать общий путь размещения маркера и первой строки. Проверить ol/ul, `start`, вложенные списки и длинный маркер внутри td.

- [x] **REV-21 · P2: регистрировать ID и закладки блочных элементов в таблицах.**

  **Код:** [internal/layout/table.go:338](/Users/vyachin/Projects/go-pdf/internal/layout/table.go:338), `paintStaticBlock`; [internal/layout/paginate.go:207](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:207), регистрация в обычном пути.
  **Воспроизведение:** `<table><tr><td><h2 id="target">TITLE</h2></td></tr></table>` рисует TITLE, но `Anchors["target"]` и outline entry отсутствуют. Добавление перед таблицей ссылки с `a::after{content:target-counter(attr(href),page)}` даёт **`PAGE0TITLE`** и ложное предупреждение о неизвестном ID.
  **Исправление / приёмка:** общий учёт anchors/headings в момент фактического размещения бокса, включая td и вложенные блоки. Оглавление должно показывать реальную страницу, а закладка вести к заголовку. Не ограничиваться уже существующим тестом inline ID.

- [x] **REV-22 · P2: не терять ненаследуемые стили строк таблицы.**

  **Код:** [internal/layout/box.go:419](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:419), `buildTable`; [internal/layout/table.go](/Users/vyachin/Projects/go-pdf/internal/layout/table.go), `rowHeight`, `paintRowAt`.
  **Воспроизведение:** `<table><tr style="background-color:red;height:60pt"><td>A</td></tr></table><p>AFTER</p>` не рисует красный фон и не резервирует 60 pt. `rowStyle` вычисляется для наследования в ячейки, но сам Row не сохраняет style, и собственная геометрия/фон строки теряются.
  **Исправление / приёмка:** хранить и применять row style, проверять высоту и фон; явно определить/валидировать остальные поддерживаемые свойства на tr и row groups. Принятые свойства не должны молча исчезать.

- [ ] **REV-23 · P2: ограничивать rowspan фактическим числом оставшихся строк при расчёте высоты.**

  **Код:** [internal/layout/table.go:128](/Users/vyachin/Projects/go-pdf/internal/layout/table.go:128), `rowHeight`; `paintRowAt`.
  **Воспроизведение:** `<table><tr><td rowspan="2">A<br>B<br>C</td><td>Z</td></tr></table><p>AFTER</p>`. Единственная строка резервирует половину высоты ячейки, но рисуются все три строки текста. Baseline AFTER — **92267 mp**, C — **99467 mp**: следующий блок перекрывает содержимое таблицы.
  **Исправление / приёмка:** сначала нормализовать spans по реальной сетке и row group, затем распределять достаточную высоту по реально существующим строкам. Проверить span длиннее таблицы, последнюю строку, несколько перекрывающихся spans и соседний блок.

### Строки, переносы, якоря и CLI

- [ ] **REV-24 · P2: удалять pending space при автоматическом переносе строки.**

  **Код:** [internal/layout/lines.go:265](/Users/vyachin/Projects/go-pdf/internal/layout/lines.go:265), `buildLines`, также ветка imagePiece.
  **Воспроизведение:** `<p style="width:30pt;text-align:right">AAA BBB</p>`. Пробел добавляется до проверки, помещается ли BBB. Затем flush сохраняет его на конце первой строки: правые границы слов **82879** и **86693 mp**, разница ровно **3814 mp** — ширина пробела.
  **Исправление / приёмка:** проверять размещение слова вместе с pending separator, при переносе исключать separator из прежней строки и её Natural. Проверить right/center/justify, картинку после пробела и `pre`. Исправленное схлопывание между inline nodes само по себе не закрывает переносы.

- [ ] **REV-25 · P2: не разрешать перенос слова только из-за границы inline-узла.**

  **Код:** [internal/layout/lines.go:94](/Users/vyachin/Projects/go-pdf/internal/layout/lines.go:94), `collectPieces`, `buildLines`.
  **Воспроизведение:** `<p style="width:35pt">ab<span>cd</span>ef</p>` разрывает единственное слово `abcdef`: ab и ef получают baselines **67832** и **82232 mp**. Каждый текстовый узел превращается в отдельный wordPiece, а перед каждым таким piece разрешён wrap.
  **Исправление / приёмка:** хранить разрешённые точки разрыва отдельно от границ style/text run. Inline-граница без пробела или дефиса не должна добавлять новую точку переноса; при смене начертания сохранить несколько glyph runs в одном слове.

- [ ] **REV-26 · P2: не вычитать `text-indent` дважды при выравнивании.**

  **Код:** [internal/layout/lines.go:202](/Users/vyachin/Projects/go-pdf/internal/layout/lines.go:202), `ensureIndent`; [internal/layout/paint.go:77](/Users/vyachin/Projects/go-pdf/internal/layout/paint.go:77), `paintLineAt`.
  **Воспроизведение:** при `width:200pt;text-align:right` добавление `text-indent:20pt` сдвигает AAA **на 20000 mp влево**, хотя правая граница должна остаться общей. Natural уже включает indent, затем painter повторно уменьшает доступную ширину.
  **Исправление / приёмка:** согласовать определение Natural и Indent для line builder/painter. Проверить right/center/justify, первую и последующие строки, а также отрицательный indent.

- [ ] **REV-27 · P2: переносить inline-якорь вместе с текстом, которому он принадлежит.**

  **Код:** [internal/layout/lines.go:223](/Users/vyachin/Projects/go-pdf/internal/layout/lines.go:223), anchorPiece и последующий flush; [internal/layout/paginate.go:377](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:377), регистрация line anchors.
  **Воспроизведение:** `<p style="width:30pt">AAA <a id="dest">BBB</a></p>`. BBB переходит на вторую строку, но dest остаётся у первой: anchor Y **56693**, AAA baseline **67832**, BBB baseline **82232 mp**. Если перенос совпадает с границей страницы, target-counter может указывать предыдущую страницу.
  **Исправление / приёмка:** привязывать pending anchor к первой фактически размещённой позиции его содержимого. Проверить wrap на строку/страницу, вложенные span и пустые anchors.

- [ ] **REV-28 · P2: не создавать пустую последнюю страницу из завершающего `break-after:page`.**

  **Код:** [internal/layout/paginate.go:288](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:288), `afterBlock`.
  **Воспроизведение:** `<p style="break-after:page">A</p>` даёт **две страницы**, вторая без содержимого. Страница создаётся немедленно, хотя следующего бокса нет; это меняет page count, стоимость печати и `counter(pages)`.
  **Исправление / приёмка:** отложить forced break до появления следующего содержимого либо корректно убрать только неиспользованную завершающую страницу. Проверить footer counters и разрыв между двумя реальными блоками.

- [ ] **REV-29 · P2: обрабатывать строку выше страницы по политике overflow, а не как отсутствие прогресса.**

  **Код:** [internal/layout/paginate.go:330](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:330), `placeText`, и `f.empty=false` в `placeBlock`.
  **Воспроизведение:** страница **200×50 pt** с нулевыми margins и `<p style="font-size:100pt">A</p>` возвращает `layout/no-progress`, A не рисуется. До проверки строки страница уже помечена непустой, поэтому ветка предупреждения/клиппинга для первой слишком высокой строки недостижима.
  **Исправление / приёмка:** различать вход в контейнер и фактически занятое место. Первая слишком высокая строка должна один раз продвинуть cursor и дать предусмотренную профилем диагностику overflow; strict может превратить её в ошибку. Проверить также слишком высокую строку таблицы и block image.

- [ ] **REV-30 · P2: одинаково разбирать пробелы в `colspan`/`rowspan` при валидации и layout.**

  **Код:** [internal/profile/profile.go:71](/Users/vyachin/Projects/go-pdf/internal/profile/profile.go:71), `ValidateSheets`; [internal/layout/box.go:484](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:484), `spanAttr`.
  **Воспроизведение:** `<table><tr><td colspan=" 2 ">A</td></tr></table>` проходит Validate, затем Convert возвращает `html/invalid-span`. Валидатор использует `strings.TrimSpace`, builder передаёт исходную строку в `strconv.Atoi`.
  **Исправление / приёмка:** один общий parser span attributes и одинаковая нормализация. Проверить оба атрибута с пробелами, без них, с неверным числом и на границах 1..100.

- [ ] **REV-31 · P2: поддержать документированный размер `-page 210x297mm`.**

  **Код:** [internal/engine/convert.go:185](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go:185), `ParsePageSize`; [pdfgen/pdfgen.go:42](/Users/vyachin/Projects/go-pdf/pdfgen/pdfgen.go:42), `Options.PageSize`.
  **Воспроизведение:** `ParsePageSize("210x297mm")` возвращает **«page size expects two lengths»**. При разбиении по `x` ширина `210` не получает единицу из правой части и отклоняется. Синтаксис обещан README, CLI help и теперь публичным API.
  **Исправление / приёмка:** реализовать общий суффикс единицы либо везде обещать только реально поддерживаемый синтаксис. Предпочтительно поддержать `210x297mm`; тестировать также `210mmx297mm`, дробные размеры, пробелы и неверную единицу через реальный CLI и публичный API.

### Дополнительные улучшения

- [ ] **REV-32 · P3: проверять грамматику и диапазон отдельно для каждого CSS-свойства.**

  **Код:** [internal/css/props.go](/Users/vyachin/Projects/go-pdf/internal/css/props.go), `parseInteger`, `parseSingleLength`, `parseBoxShorthand`.
  Проверка показала, что `orphans:1.5` и `widows:1.5` принимаются с усечением до 1. Один parser длины также допускает отрицательные padding/width и `auto` в местах с другой грамматикой: `<div style="padding:-10pt;border:-2pt solid red;width:-5pt">A</div>` проходит профиль и выводит текст левее page margin. Добавить ограничения конкретного свойства после общего разбора, сохранив разрешённые отрицательные margin/text-indent. Возвращать ошибку значения вместо неявного усечения.

- [ ] **REV-33 · P3: сделать тесты бюджетов шрифтов проверками реальных защищаемых путей.**

  **Код:** [internal/font/gpos_test.go:65](/Users/vyachin/Projects/go-pdf/internal/font/gpos_test.go:65), `TestClassDefRangeIsBounded`; [internal/font/font_test.go:166](/Users/vyachin/Projects/go-pdf/internal/font/font_test.go:166), `FuzzParse`.
  `TestClassDefRangeIsBounded` разбирает ClassDef и вызывает Kerning пустой таблицы, но **не вызывает `parsePairPos` с его защитой**. Такой тест проходит после удаления соответствующего guard и не обнаруживает REV-01. Добавить валидные seed-шрифты с мутациями directory/GPOS/cmap, контролируемый бюджет и полный `Font.Parse → Shape → Subset`. Тест бюджета должен падать при удалении проверяемого ограничения.

- [ ] **REV-34 · P3: расширить CI проверкой гонок и ограниченным запуском fuzz.**

  **Код:** [.github/workflows/ci.yml](/Users/vyachin/Projects/go-pdf/.github/workflows/ci.yml), [pdfgen/example_test.go](/Users/vyachin/Projects/go-pdf/pdfgen/example_test.go).
  CI уже проверяет минимальный Go из go.mod, vet, pdfcpu и Poppler. Добавить отдельный `go test -race ./...`, ограниченные fuzz-запуски с полезными seeds и проверку импортируемости публичного API из отдельного модуля. Регулярно проверять актуальный Go вместе с минимальным; производительные benchmarks оставить отдельно от функциональных тестов. Проверку конечного изображения довести по ENG-09 до содержательных assertions, а не только наличия краски.

**Уточнение к прежним улучшениям:** ARCH-01 подтверждён REV-19/20/21/22. ENG-09 частично реализован: `TestIndependentRendererSeesContent` уже вызывает Poppler и pdftotext, но проверяет только длину текста, размер страницы и грубую долю тёмных пикселей. Нужны эталонный текст/координаты и цветовые области, чтобы обнаруживать в том числе REV-13. Не требуется повторно добавлять уже существующий smoke test.

### Выполненные проверки и ограничения ревью

- `go test ./...` прошёл на исходной рабочей копии.
- На финальном снимке прошли `go vet ./...` и `PDF_REQUIRE_VALIDATOR=1 PDFCPU=/tmp/gobin/pdfcpu go test -race ./... -skip '^TestReview' -count=1`, включая новый публичный пакет. Гонок не обнаружено. `TestReview` — только временные проверки данного ревью; штатных тестов с этим префиксом нет. PDF_PERF не задавался: тесты временных бюджетов сами пропускаются по конфигурации проекта.
- Во временной копии выполнены **46 сценариев ревью: 8 прошли, 38 выявили расхождения**. Это число сценариев, не число независимых ошибок: повторные проверки внутреннего/публичного API и несколько проверок одного дефекта считаются отдельно. Негативные assertions намеренно падают; они не добавлены в штатный набор.
- PNG alpha проверен на **конечном PDF** через установленный `pdftoppm` 26.05.0. Независимое извлечение текста в этом окружении не запускалось: `pdftotext` отсутствует; штатный Poppler smoke test поэтому пропускается локально. Обязательный pdfcpu запускался отдельно и в race suite.
- Установленный `staticcheck` не смог разобрать export data используемого Go **1.27.4**: `export data version 5 is greater than maximum supported version 2`. Это ограничение локального инструмента; успешный staticcheck не заявляется. Минимальный Go 1.24 проверяется конфигурацией CI, но локально на нём ревью не запускалось.
- Временные исходники проверок и [машиночитаемый отчёт](/private/var/folders/hb/f9ck2zzx4353ph3s0502x2wr0000gn/T/go-pdf-review-final-07e7cblh/review-results.jsonl) лежат в `/private/var/folders/hb/f9ck2zzx4353ph3s0502x2wr0000gn/T/go-pdf-review-final-07e7cblh`. Это временное локальное подтверждение; входы и критерии исправления сохранены выше. При реализации перенести соответствующие сценарии в постоянные regression tests.
- Не заявляется полное соответствие браузерному CSS, PDF/A или PDF/UA. Пункты относятся к принятому профилю, собственной спецификации или обещанному поведению API. Разрешение `@font-face` относительно явно заданного BaseDir/FontDir проверено и не объявляется ошибкой только потому, что отличается от обычной CSS URL-базы.

## История прежних проверок

Ниже сохранены предыдущие записи. Их снимки, числа и утверждения об исправлениях относятся к соответствующим проходам; актуальные результаты приведены выше. Например, абсолютный `1e100pt` уже отклоняется, отмена Convert частично реализована, публичный пакет теперь существует. Эти изменения не закрывают отдельные сценарии текущего ревью.

Дата последней проверки: **24 ноября 2026 года**. Проверены CLI, HTML/CSS-парсеры, валидация, ресурсы, шрифты, текст, таблицы, пагинация, PDF, отладочный растр, тесты, документация и CI. Начальный снимок этого прохода — `a8dd72c`, финальный зафиксированный снимок — [2bdb8ff](https://github.com/vyachin/go-pdf/commit/2bdb8ff75940487715daa91cfa283d235586fb77). Локальный `main` в начале проверки совпадал с GitHub. **Добавлены 19 новых подтверждённых ошибок и три предложения по улучшению** — раздел «Дополнение: полное ревью текущего проекта» в конце файла.

Код в рамках этого ревью не исправлялся. Проверочные тесты создавались в отдельной временной копии; закрытые задачи удалены из списка после проверки исправлений. Во время проверки другие изменения продолжали поступать в `main`; результаты привязаны к указанным снимкам.

Приоритеты: **P1** — падение процесса, нарушение целостности документа или существенный риск для доступности; **P2** — неверное поведение поддерживаемой функции; **P3** — улучшение API, сопровождения или документации. Сначала закрыть задачи безопасности, затем корректность PDF и раскладки.

Ниже перечислены только открытые задачи; исправленные удалены после проверки.

## Улучшения для отдельной проработки

- [ ] **ENG-02 · P2: ввести бюджеты ресурсов и поддержать отмену во всех дорогих стадиях.**

  **Код:** [internal/engine/convert.go:21](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go:21), [internal/engine/engine.go:62](/Users/vyachin/Projects/go-pdf/internal/engine/engine.go:62); парсеры, layout и загрузчики ресурсов.
  `ctx.Err()` проверяется только на входе. Предусмотреть отмену во время парсинга, обходов, пагинации и записи; ограничить размер HTML/CSS/шрифтов/изображений, глубину DOM, размер таблиц, число страниц и общий объём операций. Лимиты должны возвращать диагностируемую ошибку до опасной аллокации. Для CLI использовать `signal.NotifyContext` из стандартной библиотеки. Проверять отмену после старта работы, а не только уже отменённый context.

Штатные тесты исходного состояния `go test ./...` прошли. На обновлённом отдельном снимке штатные тесты и `go vet ./...` также прошли. Дополнительные проверки ревью размещены вне рабочего дерева: 20 проверок, из них 16 выявили ошибки; некоторые проверки относятся к одной задаче выше. Проверки не добавлялись в исходники и не изменяют корпус.

При реализации переносить каждый подтверждённый сценарий в постоянный regression-тест. Успешный прогон нынешних тестов не закрывает перечисленные дефекты: большинство из них текущими проверками не обнаруживается.

## Review addendum — 2026-11-24

This addendum preserves the evidence from the 2026-11-24 review. Its findings are closed and removed from the checklist; source code was not changed by that review.

The first review snapshot exactly matches commit [02dffa5](https://github.com/vyachin/go-pdf/commit/02dffa5cb7bfa4aa2c2b113ff25fbb30bad3e10e). All findings below were reproduced again against a separate snapshot of `705497a`, after the concurrent CSS and flow fixes. File links below refer to that later state. The six earlier tasks CSS-01, CSS-03, CSS-04, CSS-05, LAY-03 and LAY-04 now pass their targeted checks and are not duplicated here.

The evidence below is kept for the still-open ENG-02 and for the review trail. A successful existing test suite does not close the open tasks: each needs a regression test for the specific reproduction.

**Additional evidence for existing ENG-02:** cancellation one millisecond after starting a 1500-paragraph conversion left `ctx.Err()==context.Canceled`, but conversion returned `err=nil` and a 38433-byte PDF. Thread cancellation through parsing, resource loading, layout and writing rather than checking only on entry. For resource limits, also cover span dimensions (SEC-05), expanded GPOS ranges and decoded image pixel counts before their expensive loops/allocations.

- The existing suite passed on the original checkout and on the later `705497a` snapshot. In snapshots containing the additional probes, `go test ./... -skip '^TestReview'` runs the original suite.
- `go vet ./...` passed on both review snapshots.
- The initial snapshot had **36 review checks: 32 failed, four passed**. After rechecking `705497a`, **26 failed and ten passed**; six previously reported CSS/flow cases were fixed concurrently. These are scenario counts, not distinct-defect counts: CMYK and nested structure have more than one check, and cancellation updates existing ENG-02. The 26 failures correspond to the 23 defects reported by that review plus those overlaps and the existing cancellation issue.
- Both review snapshots passed `go test -race ./... -skip 'TestReview|TestPerformanceBudget'`; no race was reported. The unrestricted race run failed the elapsed-time budget described in ENG-07. An early run against actively edited working files hit an intermediate syntax error; it was not treated as a stable project defect.
- The real Arial font was available, and `/tmp/gobin/pdfcpu` existed, so the existing font-dependent tests and their independent PDF validation ran. Structural validation alone does not verify color reproduction, glyph positions or visual completeness.
- Temporary probes and full machine-readable results are outside the working tree: [initial results](/private/tmp/go-pdf-review-uv291h3c/review-results.jsonl), [latest results](/private/tmp/go-pdf-review-current-dacig99u/review-results.jsonl). The four `review_probe_test.go` files in each snapshot contain the executable reproductions. These temporary paths are local review evidence; the reproductions and acceptance criteria above are the durable handoff.

## Дополнение: полное ревью текущего проекта — 24 ноября 2026 года

Из **19 подтверждённых ошибок уровня P2** этого прохода открытых не осталось; ниже — два сохранившихся предложения уровня P3. Ошибки были воспроизведены на отдельном снимке `2bdb8ff`; для проверок текста и геометрии использовался настоящий Arial. Ссылки на строки соответствуют проверенному снимку; при последующих изменениях ориентироваться также на имя функции.

### Предложения по улучшению

- [ ] **ARCH-01 · P3: объединить расчёт геометрии и правила потока для обычных блоков и ячеек.**

  **Код:** [internal/layout/paginate.go](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go), `measure`/`placeBlock`; [internal/layout/table.go](/Users/vyachin/Projects/go-pdf/internal/layout/table.go), `paintStaticBlock`/`paintStaticTable`.
  Разные реализации уже расходятся по margins (LAY-07), изображениям (LAY-17), ширине и декорациям. Выделить общий расчёт used-размеров и статическую раскладку содержимого; пагинация должна фрагментировать рассчитанные боксы, а рисование — использовать их геометрию. Делать постепенно через сравнение координат одинакового блока внутри и вне ячейки, без полной переписи движка.

- [ ] **ENG-09 · P3: добавить независимое визуальное и текстовое сравнение в CI.**

  **Код:** [.github/workflows/ci.yml](/Users/vyachin/Projects/go-pdf/.github/workflows/ci.yml), [internal/engine/corpus_test.go](/Users/vyachin/Projects/go-pdf/internal/engine/corpus_test.go), [internal/pdfdebug/extract.go](/Users/vyachin/Projects/go-pdf/internal/pdfdebug/extract.go).
  Обязательный `pdfcpu` уже полезен, но проверяет структуру PDF; собственный растр получает операции layout до PDF-сериализации и не доказывает правильность конечного файла. Добавить небольшой фиксированный корпус конечных PDF, растеризацию независимым движком и независимое извлечение текста. Закрепить версии шрифтов/инструментов; включить alpha, CMYK/rejection, несколько шрифтов, Unicode-алиасы и номера страниц. Проверять координаты/области с допуском, сохраняя сравнения текста и графа объектов; растеризаторы нужны только в CI, а не в runtime библиотеки.

- `go vet ./...` прошёл на исходном и финальном снимках.
- Первый обычный `go test ./...` на `a8dd72c` упал только на **TestPerformanceBudget**: одна страница заняла **107,32525 мс** при лимите **50 мс**. Это дополняет ENG-07: нестабильность проявилась даже без `-race` при параллельной нагрузке. Функциональные пакеты прошли; падение нельзя скрывать формулировкой «все тесты зелёные».
- На финальном снимке `2bdb8ff` прошёл полный функциональный прогон с гонками: `go test -race ./... -skip '^TestReview|^TestPerformanceBudget$'`. Гонок не обнаружено. `TestReview` — только временные проверки этого ревью, отсутствующие в рабочем проекте; тест производительности исключён явно по ENG-07.
- Корпуса `internal/engine` и `internal/mpdfcompat` отдельно прошли с обязательным независимым валидатором: `PDF_REQUIRE_VALIDATOR=1 PDFCPU=/tmp/gobin/pdfcpu go test ./internal/engine ./internal/mpdfcompat -run 'TestConvertProducesValidPDF|TestCorpus' -count=1`. На финальном снимке корпус также прошёл в составе race-прогона с доступным pdfcpu. Наличие этих проходов не опровергает ошибки внешнего вида и текстового слоя выше.
- `go test ./internal/mpdfcompat -short -count=1` прошёл. `openspec validate --specs` принял все **5** основных спецификаций; предупреждения касались формулировок нормативных требований на русском.
- Проверки путей, геометрии, alpha, структуры ToUnicode, валидации и отмены запускались в отдельной временной копии. Временные негативные проверки намеренно падают, фиксируя расхождение с ожидаемым поведением; они не добавлены в штатный набор проекта. При исправлении задачи перенести её сценарий в постоянный regression-тест.
- **ENG-02 повторно подтверждён:** отмена через 1 мс оставляет `ctx.Err()==context.Canceled`, но конвертация возвращает `err=nil` и **230953 байта PDF**. Также `ParseLengthString("1e100pt")` принимает число без ошибки и на проверенной платформе превращает его в `2147483647` mp. Проверять конечность/диапазон до приведения к `int32`, затем контролировать арифметику размеров и лимиты декодирования/растеризации. Опасные OOM-сценарии намеренно не запускались.
- Исправления прежних HTML-01, HTML-02, PDF-04, PDF-05, PDF-06, FONT-01, FONT-02, FONT-03, CSS-06 и CSS-08 поступили отдельными изменениями в `main` во время этого прохода. Они учтены в финальном снимке и не включены в 19 новых ошибок. Остальные ранее открытые задачи сохраняют свои идентификаторы.
- Не проводилась проверка соответствия PDF/A, PDF/UA или полного браузерного CSS: проект этого не обещает. Новые ошибки относятся к принятому профилю, заявленному API или собственным гарантиям проекта.
